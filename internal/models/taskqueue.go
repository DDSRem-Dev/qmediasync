package models

import (
	"Q115-STRM/internal/helpers"
	"context"
	"log"
	"sync"
	"time"
)

// Task 定义任务接口
type Task interface {
	Execute() error
	Name() string
}

// QueueProcessor 队列处理器
type QueueProcessor struct {
	channel     chan Task      // 主要任务通道
	buffer      []Task         // 缓冲队列，存放无法添加到channel的任务
	workerCount int            // 工作协程数量
	mu          sync.RWMutex   // 保护缓冲区的锁
	wg          sync.WaitGroup // 等待组
	ctx         context.Context
	cancel      context.CancelFunc
	bufferFull  chan struct{} // 缓冲区满信号
}

// Config 队列处理器配置
type TaskQueueConfig struct {
	ChannelCapacity int // 通道容量
	WorkerCount     int // 工作协程数量
}

// DefaultConfig 默认配置
func DefaultTaskQueueConfig() *TaskQueueConfig {
	return &TaskQueueConfig{
		ChannelCapacity: 10,
		WorkerCount:     3,
	}
}

// NewQueueProcessor 创建新的队列处理器
func NewQueueProcessor(config *TaskQueueConfig) *QueueProcessor {
	if config == nil {
		config = DefaultTaskQueueConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	qp := &QueueProcessor{
		channel:     make(chan Task, config.ChannelCapacity),
		buffer:      make([]Task, 0),
		workerCount: config.WorkerCount,
		ctx:         ctx,
		cancel:      cancel,
		bufferFull:  make(chan struct{}, 1),
	}

	// 启动工作协程
	qp.wg.Add(qp.workerCount)
	for i := 0; i < qp.workerCount; i++ {
		go qp.worker(i)
	}

	// 启动缓冲区监控协程
	go qp.bufferMonitor()

	return qp
}

// Submit 提交任务到队列
func (qp *QueueProcessor) Submit(task Task) bool {
	select {
	case qp.channel <- task:
		helpers.AppLogger.Infof("任务 %s 已添加到channel", task.Name())
		return true
	default:
		// channel已满，尝试添加到缓冲区
		return qp.addToBuffer(task)
	}
}

// SubmitWithTimeout 带超时的任务提交
func (qp *QueueProcessor) SubmitWithTimeout(task Task, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case qp.channel <- task:
		helpers.AppLogger.Infof("任务 %s 已添加到channel", task.Name())
		return true
	case <-ctx.Done():
		// 超时，尝试添加到缓冲区
		return qp.addToBuffer(task)
	}
}

// addToBuffer 添加任务到缓冲区
func (qp *QueueProcessor) addToBuffer(task Task) bool {
	qp.mu.Lock()
	defer qp.mu.Unlock()

	qp.buffer = append(qp.buffer, task)
	helpers.AppLogger.Infof("任务 %s 已添加到缓冲区，当前缓冲区大小: %d", task.Name(), len(qp.buffer))

	return true
}

// bufferMonitor 监控缓冲区，尝试将缓冲区任务移入channel
func (qp *QueueProcessor) bufferMonitor() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-qp.ctx.Done():
			return
		case <-qp.bufferFull:
			// 缓冲区满信号，立即处理
			qp.tryDrainBuffer()
		case <-ticker.C:
			// 定期尝试处理缓冲区
			qp.tryDrainBuffer()
		}
	}
}

// tryDrainBuffer 尝试从缓冲区取出任务放入channel
func (qp *QueueProcessor) tryDrainBuffer() {
	qp.mu.Lock()
	defer qp.mu.Unlock()

	if len(qp.buffer) == 0 {
		return
	}

	// 尝试将缓冲区任务移入channel
	for len(qp.buffer) > 0 {
		select {
		case qp.channel <- qp.buffer[0]:
			helpers.AppLogger.Infof("从缓冲区移出任务 %s 到channel", qp.buffer[0].Name())
			qp.buffer = qp.buffer[1:]
		default:
			// channel已满，停止尝试
			return
		}
	}
}

// worker 工作协程
func (qp *QueueProcessor) worker(id int) {
	defer qp.wg.Done()

	helpers.AppLogger.Infof("工作协程 %d 启动", id)

	for {
		select {
		case <-qp.ctx.Done():
			helpers.AppLogger.Infof("工作协程 %d 退出", id)
			return
		case task, ok := <-qp.channel:
			if !ok {
				return
			}
			qp.processTask(task, id)
		}
	}
}

// processTask 处理任务
func (qp *QueueProcessor) processTask(task Task, workerID int) {
	defer func() {
		if r := recover(); r != nil {
			helpers.AppLogger.Infof("工作协程 %d 处理任务 %s 时发生panic: %v", workerID, task.Name(), r)
		}
	}()

	start := time.Now()
	helpers.AppLogger.Infof("工作协程 %d 开始处理任务: %s", workerID, task.Name())

	err := task.Execute()
	if err != nil {
		helpers.AppLogger.Infof("工作协程 %d 处理任务 %s 失败: %v", workerID, task.Name(), err)
	} else {
		helpers.AppLogger.Infof("工作协程 %d 完成任务: %s, 耗时: %v", workerID, task.Name(), time.Since(start))
	}
}

// Stats 获取队列统计信息
func (qp *QueueProcessor) Stats() map[string]interface{} {
	qp.mu.RLock()
	defer qp.mu.RUnlock()

	return map[string]interface{}{
		"channel_length": len(qp.channel),
		"channel_cap":    cap(qp.channel),
		"buffer_length":  len(qp.buffer),
		"worker_count":   qp.workerCount,
	}
}

// Stop 停止队列处理器
func (qp *QueueProcessor) Stop() {
	log.Println("正在停止队列处理器...")
	qp.cancel()
	qp.wg.Wait()
	close(qp.channel)
	log.Println("队列处理器已停止")
}

// 示例任务实现
type ExampleTask struct {
	name string
	dur  time.Duration
}

func (t *ExampleTask) Execute() error {
	time.Sleep(t.dur)
	return nil
}

func (t *ExampleTask) Name() string {
	return t.name
}

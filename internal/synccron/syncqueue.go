package synccron

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/scrape"
	"fmt"
	"sync"
)

type SyncTaskType string

const (
	SyncTaskTypeStrm   SyncTaskType = "STRM同步"
	SyncTaskTypeScrape SyncTaskType = "刮削整理"
)

// 同步任务处理器
type SyncProcessor struct {
	taskChan chan struct {
		id       uint
		taskType SyncTaskType
	}
	running         bool
	waitingQueue    map[string]uint // 等待队列（syncID集合）
	currentTaskId   uint            // 当前正在运行的任务ID，0表示无任务
	currentTaskType SyncTaskType    // 当前正在运行的任务类型
	currentTaskLock sync.Mutex
	Lock            sync.Mutex
	scrapeInstance  *scrape.Scrape // 正在运行的刮削目录
	strmSync        *models.Sync   // 正在运行的STRM同步任务
}

func (sp *SyncProcessor) resetCurrentTask(taskId uint, taskType SyncTaskType) bool {
	sp.currentTaskLock.Lock()
	defer sp.currentTaskLock.Unlock()
	sp.currentTaskId = taskId
	sp.currentTaskType = taskType
	return true
}

// 处理同步任务的协程
func (sp *SyncProcessor) process() {
	// helpers.AppLogger.Debug("同步任务处理协程已启动")
	for sp.running {
		task, ok := <-sp.taskChan
		if !ok {
			break
		}
		sp.Lock.Lock()
		// 如果等待队列中不存在，则表示任务已被取消，跳过处理
		if _, exists := sp.waitingQueue[fmt.Sprintf("%d-%s", task.id, task.taskType)]; !exists {
			helpers.AppLogger.Infof("同步任务已被取消，跳过处理: 类型=%s, ID=%d", task.taskType, task.id)
			sp.Lock.Unlock()
			continue
		}
		sp.Lock.Unlock()
		sp.resetCurrentTask(task.id, task.taskType)
		// 从等待队列移除
		sp.Lock.Lock()
		delete(sp.waitingQueue, fmt.Sprintf("%d-%s", task.id, task.taskType))
		sp.Lock.Unlock()
		helpers.AppLogger.Infof("开始处理同步任务: 类型=%s, ID=%d", task.taskType, task.id)
		sp.handleSyncTask(task.id, task.taskType)
		// 任务处理完毕，清空当前任务
		sp.resetCurrentTask(0, "")
	}
	helpers.AppLogger.Info("同步任务处理协程已停止")
}

// 处理单个同步任务
func (sp *SyncProcessor) handleSyncTask(id uint, taskType SyncTaskType) {
	if taskType == SyncTaskTypeStrm {
		// 从数据库获取同步任务
		syncPath := models.GetSyncPathById(id)
		if syncPath == nil {
			helpers.AppLogger.Errorf("获取同步目录失败， ID=%d", id)
			return
		}

		helpers.AppLogger.Infof("开始执行同步任务: 同步目录ID=%d", id)
		// 创建一个新的同步任务
		sp.strmSync = syncPath.CreateSyncTask()
		if sp.strmSync == nil {
			helpers.AppLogger.Error("创建同步任务失败")
			return
		}
		defer func() {
			sp.strmSync = nil
		}()
		// time.Sleep(5 * time.Minute)
		// 执行同步任务（单协程并发1）
		if success := sp.strmSync.Start(); success {
			helpers.AppLogger.Infof("同步任务执行成功: 同步目录ID=%d", id)
		}
	}
	if taskType == SyncTaskTypeScrape {
		// 从数据库获取刮削任务
		scrapePath := models.GetScrapePathByID(id)
		if scrapePath == nil {
			helpers.AppLogger.Errorf("获取刮削目录失败， ID=%d", id)
			return
		}

		helpers.AppLogger.Infof("开始执行刮削任务: 刮削目录ID=%d", id)
		// 执行刮削任务（单协程并发1）
		sp.scrapeInstance = scrape.NewScrape(scrapePath)
		defer func() {
			sp.scrapeInstance = nil
		}()
		if success := sp.scrapeInstance.Start(); success {
			helpers.AppLogger.Infof("刮削任务执行成功: 刮削目录ID=%d", id)
		}
	}
}

var sync115Processor *SyncProcessor
var openlistSyncProcessor *SyncProcessor
var localSyncProcessor *SyncProcessor

// 初始化同步任务处理器
func InitSyncProcessor(sourceType models.SourceType) *SyncProcessor {
	switch sourceType {
	case models.SourceType115:
		if sync115Processor != nil {
			return sync115Processor
		}
		sync115Processor = &SyncProcessor{
			taskChan: make(chan struct {
				id       uint
				taskType SyncTaskType
			}, 10), // 缓冲队列，最多20个待处理任务
			running:         true,
			waitingQueue:    make(map[string]uint),
			currentTaskId:   0,
			currentTaskType: "",
			currentTaskLock: sync.Mutex{},
			Lock:            sync.Mutex{},
		}
		// 启动处理协程
		go sync115Processor.process()
		helpers.AppLogger.Info("115任务处理器已启动")

		return sync115Processor
	case models.SourceTypeOpenList:
		if openlistSyncProcessor != nil {
			return openlistSyncProcessor
		}
		openlistSyncProcessor = &SyncProcessor{
			taskChan: make(chan struct {
				id       uint
				taskType SyncTaskType
			}, 10), // 缓冲队列，最多20个待处理任务
			running:         true,
			waitingQueue:    make(map[string]uint),
			currentTaskId:   0,
			currentTaskLock: sync.Mutex{},
			Lock:            sync.Mutex{},
		}
		// 启动处理协程
		go openlistSyncProcessor.process()
		helpers.AppLogger.Info("OpenList任务处理器已启动")
		return openlistSyncProcessor
	case models.SourceTypeLocal:
		if localSyncProcessor != nil {
			return localSyncProcessor
		}
		localSyncProcessor = &SyncProcessor{
			taskChan: make(chan struct {
				id       uint
				taskType SyncTaskType
			}, 10), // 缓冲队列，最多20个待处理任务
			running:         true,
			waitingQueue:    make(map[string]uint),
			currentTaskId:   0,
			currentTaskLock: sync.Mutex{},
			Lock:            sync.Mutex{},
		}
		// 启动处理协程
		go localSyncProcessor.process()
		helpers.AppLogger.Info("Local任务处理器已启动")
		return localSyncProcessor
	default:
		return nil
	}

}

// 添加同步任务到队列
func AddSyncTask(id uint, taskType SyncTaskType) error {
	// 先取任务
	var syncPath *models.SyncPath
	var scrapePath *models.ScrapePath
	var processor *SyncProcessor
	switch taskType {
	case SyncTaskTypeStrm:
		syncPath = models.GetSyncPathById(id)
		processor = InitSyncProcessor(syncPath.SourceType)
	case SyncTaskTypeScrape:
		scrapePath = models.GetScrapePathByID(id)
		processor = InitSyncProcessor(scrapePath.SourceType)
	default:
		return fmt.Errorf("未知的同步任务类型: %s", taskType)
	}
	var err string
	processor.Lock.Lock()
	// 判断是否已在等待队列或正在运行
	key := fmt.Sprintf("%d-%s", id, taskType)
	if _, exists := processor.waitingQueue[key]; exists {
		err = fmt.Sprintf("同步任务已在等待队列: 类型=%s, ID=%d", taskType, id)
	}
	if processor.currentTaskId == id && processor.currentTaskType == taskType {
		err = fmt.Sprintf("同步任务已在运行: 类型=%s, ID=%d", taskType, id)
	}
	// 判断队列是否已满
	if len(processor.taskChan) >= cap(processor.taskChan) {
		err = fmt.Sprintf("同步任务队列已满，无法添加任务: 类型=%s, ID=%d", taskType, id)
	}
	processor.Lock.Unlock()
	if err != "" {
		helpers.AppLogger.Error(err)
		return fmt.Errorf("%s", err)
	}
	// 加入等待队列
	processor.Lock.Lock()
	processor.waitingQueue[key] = id
	processor.taskChan <- struct {
		id       uint
		taskType SyncTaskType
	}{id, taskType}
	processor.Lock.Unlock()
	// 如果队列没有执行则触发一次执行
	helpers.AppLogger.Debugf("当前同步任务队列长度: %d/%d, 当前执行的任务ID：%d， 任务类型：%s", len(processor.taskChan), cap(processor.taskChan), processor.currentTaskId, processor.currentTaskType)
	processor.currentTaskLock.Lock()
	defer processor.currentTaskLock.Unlock()
	if processor.currentTaskId == 0 {
		go processor.process()
	} else {
		helpers.AppLogger.Infof("同步任务已添加到等待队列: 类型=%s, ID=%d", taskType, id)
	}

	return nil
}

// 停止某个任务
func StopSyncTask(id uint, taskType SyncTaskType) error {
	// 先取任务
	var syncPath *models.SyncPath
	var scrapePath *models.ScrapePath
	var processor *SyncProcessor
	switch taskType {
	case SyncTaskTypeStrm:
		syncPath = models.GetSyncPathById(id)
		processor = InitSyncProcessor(syncPath.SourceType)
	case SyncTaskTypeScrape:
		scrapePath = models.GetScrapePathByID(id)
		processor = InitSyncProcessor(scrapePath.SourceType)
	default:
		return fmt.Errorf("未知的同步任务类型: %s", taskType)
	}
	key := fmt.Sprintf("%d-%s", id, taskType)
	if _, exists := processor.waitingQueue[key]; exists {
		delete(processor.waitingQueue, key)
		helpers.AppLogger.Infof("同步任务已从等待队列移除: 类型=%s, ID=%d", taskType, id)
	}
	if processor.currentTaskId == id && processor.currentTaskType == taskType {
		processor.currentTaskId = 0
		processor.currentTaskType = ""
		// 找到对应类型的实例，终止
		if taskType == SyncTaskTypeStrm {
			processor.strmSync.Driver.Stop()
			processor.strmSync = nil
		}
		if taskType == SyncTaskTypeScrape && processor.scrapeInstance != nil {
			// 终止刮削任务
			processor.scrapeInstance.Stop()
		}
		helpers.AppLogger.Infof("同步任务已从运行队列移除: 类型=%s, ID=%d", taskType, id)

	}
	return nil
}

func CheckTaskIsRunning(id uint, taskType SyncTaskType) int {
	// 先取任务
	var syncPath *models.SyncPath
	var scrapePath *models.ScrapePath
	var processor *SyncProcessor
	switch taskType {
	case SyncTaskTypeStrm:
		syncPath = models.GetSyncPathById(id)
		processor = InitSyncProcessor(syncPath.SourceType)
	case SyncTaskTypeScrape:
		scrapePath = models.GetScrapePathByID(id)
		processor = InitSyncProcessor(scrapePath.SourceType)
	default:
		return 0
	}
	key := fmt.Sprintf("%d-%s", id, taskType)
	processor.Lock.Lock()
	defer processor.Lock.Unlock()
	if _, exists := processor.waitingQueue[key]; exists {
		return 1
	}
	processor.currentTaskLock.Lock()
	defer processor.currentTaskLock.Unlock()
	if processor.currentTaskId == id && processor.currentTaskType == taskType {
		return 2
	}
	return 0
}

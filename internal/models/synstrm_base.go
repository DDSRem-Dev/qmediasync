package models

import (
	"Q115-STRM/internal/helpers"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncDriver interface {
	DoSync() error
	Init()
	Stop()
}

type SyncDriverBase struct {
	Sync                *Sync
	IsFullSync          bool
	strmTasks           chan *SyncFile
	downloadTasks       chan *SyncFile
	deleteTasks         chan *SyncFile
	preNetFiles         map[string]netFile // 上次查询到的网盘文件哈希映射，key是文件ID，value是本地文件路径
	preNetFileMu        sync.RWMutex       // 保护preNetFiles的锁
	currentNetFiles     map[string]bool    // 当前查询到的网盘文件哈希映射，key是文件ID，value是文件是否存在
	currentNetFileTasks chan string        // 当前查询到的网盘文件ID任务队列，用于处理文件下载
	pathBuffer          []string           // 如果pathTasks满了，先放入这里
	mu                  sync.RWMutex       // 保护缓冲区的锁
	wg                  sync.WaitGroup
	pathTasks           chan string
}

// 触发同步任务停止
func (s *SyncDriverBase) Stop() {
	select {
	case <-s.Sync.ctx.Done():
		return
	default:
		s.Sync.ctxCancel()
		s.Sync.Logger.Warn("停止同步任务")
		return
	}
}

func (s *SyncDriverBase) Init() {
}

func (s *SyncDriverBase) CheckIsRunning() bool {
	select {
	case <-s.Sync.ctx.Done():
		return false
	default:
		return true
	}
}

func (s *SyncDriverBase) InitPreNetFiles() {
	s.preNetFiles = make(map[string]netFile)
	// 先把数据库中的文件ID和本地文件路径映射到preNetFiles中
	offset := 0
	limit := 1000
	for {
		syncFiles, _ := GetSyncFiles(offset, limit, s.Sync.SyncPathId)
		if len(syncFiles) == 0 {
			break
		}
		for _, file := range syncFiles {
			s.preNetFiles[file.FileId] = netFile{
				localPath: file.LocalFilePath,
				uploaded:  file.Uploaded,
			}
			// s.Sync.Logger.Infof("预初始化，加入缓存，文件ID: %s, 本地路径: %s", file.FileId, file.LocalFilePath)
		}
		offset += limit
		if len(syncFiles) < limit {
			break
		}
	}
}

func (s *SyncDriverBase) StartCurrentNetFileWork() {
	// 启动一个协程记录本次查到的所有fileid
	s.currentNetFileTasks = make(chan string, 10)
	s.currentNetFiles = make(map[string]bool)
	// 启动一个协程记录本次查到的所有fileid
	// s.Sync.Logger.Infof("开始处理当前查询到的所有文件ID，共 %d 个文件", len(s.currentNetFileTasks))
	for fileId := range s.currentNetFileTasks {
		if !s.CheckIsRunning() {
			return
		}
		s.currentNetFiles[fileId] = true
	}
	s.currentNetFileTasks = nil // 销毁currentNetFileTasks，等待GC释放内存
	s.currentNetFiles = nil     // 销毁currentNetFiles，等待GC释放内存
}

func (s *SyncDriverBase) StartStrmWork(strmTasks chan *SyncFile, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range strmTasks {
		file.Sync = s.Sync
		file.SyncPath = s.Sync.SyncPath
		if file.ProcessStrmFile() {
			continue
		}
		// 如果失败，说明创建失败，需要删除file记录
		s.Sync.Logger.Errorf("创建strm文件失败: fileId=%d, 删除对应的数据库记录，下次重试。", file.ID)
		DeleteFileByPickCode(file.PickCode)
		// 删除本地文件
		s.DeleteLocalFile(file.LocalFilePath)
	}
}

func (s *SyncDriverBase) StartDownloadWork(downloadTasks chan *SyncFile, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range downloadTasks {
		// 如果本地文件存在,则不添加下载任务
		if helpers.PathExists(file.LocalFilePath) {
			s.Sync.Logger.Infof("本地文件已存在，无需下载: fileId=%d, pickCode=%s, localFilePath=%s", file.ID, file.PickCode, file.LocalFilePath)
			continue
		}
		file.Sync = s.Sync
		file.SyncPath = s.Sync.SyncPath
		err := AddDownloadTaskFromSyncFile(file)
		if err != nil {
			s.Sync.Logger.Errorf("从SyncFiles生成下载任务失败: fileId=%d, %v", file.ID, err)
			continue
		}
		s.Sync.NewMeta++
		s.Sync.Logger.Infof("添加下载任务: fileId=%d, 目标文件：%s", file.ID, file.LocalFilePath)
	}
}

func (s *SyncDriverBase) StartDeleteWork(deleteTasks chan *SyncFile, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range deleteTasks {
		if !file.Uploaded {
			s.Sync.Logger.Infof("文件未上传完成，无需删除: fileId=%d, pickCode=%s", file.ID, file.PickCode)
			continue
		}
		// 如果是元数据且未开启元数据删除或者未开启元数据下载则跳过
		if file.IsMeta && (s.Sync.SyncPath.GetUploadMeta() != 2 || s.Sync.SyncPath.GetDownloadMeta() == 0) {
			s.Sync.Logger.Infof("元数据删除未开启，无需删除: fileId=%d, pickCode=%s", file.ID, file.PickCode)
			continue
		}
		os.Remove(file.LocalFilePath)

		DeleteFileByPickCode(file.PickCode)
		// 删除本地文件
		s.DeleteLocalFile(file.LocalFilePath)
		s.Sync.Logger.Infof("网盘已删除该文件，删除本地文件和数据库记录: fileId=%d, pickCode=%s", file.ID, file.PickCode)
	}
}

func (st *SyncDriverBase) checkAndDeleteDir(fullPath string) {
	// 只能删除空目录
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		st.Sync.Logger.Errorf("读取目录失败: %v", err)
		return
	}
	if len(entries) > 0 {
		st.Sync.Logger.Infof("目录不为空，无法删除: %s", fullPath)
		return
	}
	if st.Sync.SyncPath.GetDeleteDir() != 1 {
		st.Sync.Logger.Infof("设置不删除空目录，无法删除: %s", fullPath)
		return
	}
	if err := os.Remove(fullPath); err != nil {
		st.Sync.Logger.Errorf("[删除文件夹] %s 结果：失败，错误 %v", fullPath, err)
	} else {
		st.Sync.Logger.Infof("[删除文件夹] %s 结果：成功", fullPath)
	}
}

func (s *SyncDriverBase) DeleteLocalFile(filePath string) {
	if filePath == "" {
		return
	}
	os.Remove(filePath)
	s.checkAndDeleteDir(filepath.Dir(filePath))
}

// 返回本地文件的上传状态
func (s *SyncDriverBase) GetExistsNetFileAndUploadStatus() map[string]bool {
	netFilesByLocalPath := make(map[string]bool)
	offset := 0
	limit := 1000
	for {
		syncFiles, _ := GetSyncFiles(offset, limit, s.Sync.SyncPathId)
		if len(syncFiles) == 0 {
			break
		}
		for _, file := range syncFiles {
			netFilesByLocalPath[file.LocalFilePath] = file.Uploaded
		}
		offset += limit
		if len(syncFiles) < limit {
			break
		}
	}
	return netFilesByLocalPath
}

func (s *SyncDriverBase) ProcessNetFiles() error {
	// 检查任务是否已被停止
	if !s.CheckIsRunning() {
		return errors.New("手动停止任务")
	}
	var wg sync.WaitGroup
	// 启动一个strm生成工作协程
	wg.Add(1)
	s.strmTasks = make(chan *SyncFile, 10)
	go s.StartStrmWork(s.strmTasks, &wg)
	// 启动一个下载生成工作协程
	wg.Add(1)
	s.downloadTasks = make(chan *SyncFile, 10)
	go s.StartDownloadWork(s.downloadTasks, &wg)
	// 启动一个删除工作协程
	wg.Add(1)
	s.deleteTasks = make(chan *SyncFile, 10)
	go s.StartDeleteWork(s.deleteTasks, &wg)
	// 从SyncFiles分页取数据，每次1000条
	offset := 0
	limit := 1000
	s.Sync.Logger.Infof("预缓存的文件数量: %d", len(s.currentNetFiles))
mainloop:
	for {
		// 检查任务是否已被停止
		if !s.CheckIsRunning() {
			break
		}
		files, err := GetSyncFiles(offset, limit, s.Sync.SyncPathId)
		if err != nil {
			s.Sync.Logger.Errorf("从SyncFiles分页取数据失败: offset=%d, limit=%d, %v", offset, limit, err)
			break
		}
		if len(files) == 0 {
			s.Sync.Logger.Infof("分页取数据完成: offset=%d, limit=%d, 数据为空", offset, limit)
			break
		} else {
			s.Sync.Logger.Infof("分页取数据完成: offset=%d, limit=%d, 数据条数: %d", offset, limit, len(files))
		}
	fileloop:
		// 处理文件
		for _, file := range files {
			// 检查任务是否已被停止
			if !s.CheckIsRunning() {
				break mainloop
			}
			if _, ok := s.currentNetFiles[file.FileId]; !ok {
				// 触发删除流程
				s.Sync.Logger.Infof("文件ID %s => %s 在网盘已被删除，触发删除流程", file.FileId, file.LocalFilePath)
				s.deleteTasks <- file
				continue fileloop
			}
			// 检查本地文件是否存在
			if !helpers.PathExists(file.LocalFilePath) {
				// 本地文件不存在
				if file.IsMeta {
					// 元文件，触发下载流程
					s.Sync.Logger.Infof("文件ID %s => %s 为元文件，触发下载流程", file.FileId, file.LocalFilePath)
					s.downloadTasks <- file
					continue fileloop
				}
			}
			if file.IsVideo {
				// 触发strm流程，检查是否更新
				// s.Sync.Logger.Infof("文件ID %s => %s 为视频文件，触发strm生成流程", file.FileId, file.LocalFilePath)
				s.strmTasks <- file
				continue fileloop
			}
			// if file.IsMeta {
			// 	// 检查文件大小是否相同
			// 	localFileInfo, err := os.Stat(file.LocalFilePath)
			// 	if err != nil {
			// 		s.Sync.Logger.Errorf("获取本地文件信息失败: %v", err)
			// 		continue fileloop
			// 	}
			// 	if file.MTime > localFileInfo.ModTime().Unix() {
			// 		s.Sync.Logger.Infof("文件ID %s => %s 本地文件修改时间 %d 早与115网盘文件修改时间 %d，重新下载", file.FileId, file.FileName, localFileInfo.ModTime().Unix(), file.MTime)
			// 		// 先删除本地文件，然后触发下载
			// 		if err := os.Remove(file.LocalFilePath); err != nil {
			// 			s.Sync.Logger.Errorf("删除本地文件失败: %v", err)
			// 			continue fileloop
			// 		}
			// 		// 文件修改时间不同，触发下载流程
			// 		s.downloadTasks <- file
			// 		continue fileloop
			// 	}
			// }
		}
		offset += limit
	}
	// 结束其他任务队列
	close(s.strmTasks)
	close(s.downloadTasks)
	close(s.deleteTasks)
	wg.Wait() // 等待所有任务结束
	return nil
}

// 递归查询本地文件
func (s *SyncDriverBase) CompareLocalFiles() error {
	// 检查任务是否已被停止
	if !s.CheckIsRunning() {
		return errors.New("手动停止任务")
	}
	s.Sync.UpdateSubStatus(SyncSubStatusProcessLocalFileList)
	// 先把数据库中的文件ID和本地文件路径映射到preNetFiles中
	netFilesByLocalPath := s.GetExistsNetFileAndUploadStatus()
	rootDir := s.Sync.GetBaseDir()
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if !s.CheckIsRunning() {
			return errors.New("手动停止任务")
		}
		if err != nil || path == "." || strings.Contains(path, ".verysync") || strings.Contains(path, ".deletedByTMM") {
			// 跳过根目录本身
			// 跳过微力同步和TMM的临时文件夹中的文件
			return nil
		}
		relPath, _ := filepath.Rel(s.Sync.LocalPath, path) // 相对路径，例如：电影/华语电影/哪吒.mkv
		if (relPath == "." || relPath == "") && info.IsDir() {
			// 如果是根目录，则跳过
			return nil
		}
		// 不处理本地目录
		if info.IsDir() {
			metaFileName := filepath.Join(path, ".meta")
			os.Remove(metaFileName) // 删除.meta文件
			return nil
		}

		ext := filepath.Ext(info.Name())
		isVideo := ext == ".strm"
		isMeta := s.Sync.IsValidMetaExt(info.Name())
		// 检查当前文件在数据库是否存在
		uploaded, ok := netFilesByLocalPath[path]
		upload := false
		if ok {
			if uploaded {
				// 文件已上传，跳过
				return nil
			}
		} else {
			// 文件不存在，删除strm
			if isVideo {
				// 删除
				s.Sync.Logger.Infof("本地strm文件 %s 在数据库中不存在，需要删除", path)
				os.Remove(path)
				// 检查是否需要删除整个目录
				s.checkAndDeleteDir(filepath.Dir(path))
				return nil
			}
		}
		if isMeta {
			// 上传元数据
			switch s.Sync.SyncPath.GetUploadMeta() {
			case int(SyncTreeItemMetaActionUpload):
				upload = true
			case int(SyncTreeItemMetaActionDelete):
				// 删除
				s.Sync.Logger.Infof("本地元数据文件 %s 在数据库中不存在，需要删除", path)
				os.Remove(path)
				s.checkAndDeleteDir(filepath.Dir(path))
				return nil
			default:
				// 保留
				return nil
			}
		}
		// 这是要上传的
		if !upload {
			return nil
		}
		// 生成一个syncFile
		parentPath := filepath.Dir(relPath)
		syncFile, syncErr := AddSyncFile(s.Sync.SyncPath, relPath, parentPath, info.Name(), parentPath, path, info.Size(), relPath, "", isMeta, isVideo, "", false, true, info.ModTime().Unix())
		if syncErr != nil {
			s.Sync.Logger.Errorf("保存文件 %s 失败: %v", relPath, syncErr)
			return nil
		}
		// 加入上传队列
		taskErr := AddUploadTaskFromSyncFile(syncFile)
		if taskErr != nil {
			s.Sync.Logger.Errorf("创建上传任务失败: %v", taskErr)
			return nil
		}
		s.Sync.NewUpload++
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SyncDriverBase) addPathToTasks(path string) {
	select {
	case <-s.Sync.ctx.Done():
		return
	case s.pathTasks <- path:
		s.wg.Add(1)
		s.Sync.Logger.Infof("任务 %s 已添加到channel", path)
	default:
		// 超时，尝试添加到缓冲区
		s.addToBuffer(path)
	}
}

// bufferMonitor 监控缓冲区，尝试将缓冲区任务移入channel
func (s *SyncDriverBase) bufferMonitor(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.Sync.ctx.Done():
			// 任务停止，清空buffer
			s.mu.Lock()
			// 根据buffer长度操作wg
			s.wg.Add(-len(s.pathBuffer))
			s.pathBuffer = nil
			s.mu.Unlock()
			ticker.Stop()
			return
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			// 定期尝试处理缓冲区
			s.tryDrainBuffer()
		}
	}
}

// tryDrainBuffer 尝试从缓冲区取出任务放入channel
func (s *SyncDriverBase) tryDrainBuffer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.CheckIsRunning() {
		return
	}
	if len(s.pathBuffer) == 0 {
		return
	}

	// 尝试将缓冲区任务移入channel
	for len(s.pathBuffer) > 0 {
		select {
		case <-s.Sync.ctx.Done():
			return
		case s.pathTasks <- s.pathBuffer[0]:
			helpers.AppLogger.Infof("从缓冲区移出任务 %s 到任务处理队列", s.pathBuffer[0])
			s.pathBuffer = s.pathBuffer[1:]
		default:
			// channel已满，停止尝试
			return
		}
	}
}

// addToBuffer 添加任务到缓冲区
func (s *SyncDriverBase) addToBuffer(task string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wg.Add(1)
	s.pathBuffer = append(s.pathBuffer, task)
	s.Sync.Logger.Infof("任务 %s 已添加到缓冲区，当前缓冲区大小: %d", task, len(s.pathBuffer))
}

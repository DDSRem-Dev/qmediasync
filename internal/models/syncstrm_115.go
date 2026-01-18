package models

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/v115open"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type preloadPathTask struct {
	pathId string
	path   string
	depth  int
}
type netFile struct {
	localPath string
	uploaded  bool
}
type SyncDriver115 struct {
	SyncDriverBase
	client           *v115open.OpenClient
	path115Tasks     chan *SyncFile
	path115TaskIds   map[string]bool      // 已经加入补全路径的任务ID，用来去重不会重复处理一样的ID
	path115TaskIdsMu sync.Mutex           // 保护path115TaskIds的锁
	pathMap          map[string]string    // 只在网盘文件处理阶段使用，用完销毁，存放的是文件ID到文件路径的映射
	preloadPathTasks chan preloadPathTask // 只在全量同步时，预缓存文件夹结构阶段使用，用完关闭，10s超时
	pathBuffer       []preloadPathTask    // 如果pathTasks满了，先放入这里
	pathBufferMu     sync.RWMutex         // 保护缓冲区的锁
	pathBufferWg     sync.WaitGroup
	pathMapTasks     chan []string     // 路径映射任务队列，用于处理路径映射
	mapMu            sync.Mutex        // 路径映射任务队列的互斥锁
	excludePaths     map[string]string // 排除的路径
	excludePathsMu   sync.RWMutex      // 排除的路径的锁
}

func NewSyncDriver115(sync *Sync, client *v115open.OpenClient) *SyncDriver115 {
	return &SyncDriver115{
		SyncDriverBase: SyncDriverBase{
			Sync:       sync,
			IsFullSync: sync.IsFullSync,
		},
		client: client,
	}
}

func (s *SyncDriver115) DoSync() error {
	s.excludePaths = make(map[string]string)
	s.path115TaskIds = make(map[string]bool)
	// 先删除以前的缓存文件，在local_path下面的.fileid_2_map.json文件
	filePath := filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, ".fileid_2_localpath_map.json")
	os.Remove(filePath)
	// 检查源目录是否存在
	detail, detailErr := s.client.GetFsDetailByPath(s.Sync.ctx, s.Sync.RemotePath)
	if detailErr != nil || (detail != nil && detail.FileId == "") {
		return fmt.Errorf("源目录 %s 不存在", s.Sync.RemotePath)
	}
	s.Sync.BaseCid = detail.FileId // 通过目录查ID
	// 检查是否有数据库记录，如果一条记录都没有默认触发全量同步
	syncFiles, _ := GetSyncFiles(0, 1, s.Sync.SyncPathId)
	if len(syncFiles) == 0 {
		s.IsFullSync = true
	}
	s.Sync.Logger.Infof("是否全量同步：%v", s.IsFullSync)
	s.path115Tasks = make(chan *SyncFile, 1150)
	s.pathMapTasks = make(chan []string, 100)
	defer func() {
		if s.pathMap != nil {
			s.pathMap = nil
		}
		if s.preNetFiles != nil {
			s.preNetFiles = nil
		}
		if s.pathMapTasks != nil {
			// 	_, ok := <-s.pathMapTasks
			// 	if ok {
			// 		// 关闭路径映射任务队列
			close(s.pathMapTasks)
			// 	}
		}
		// if s.path115Tasks != nil {
		// 	_, ok := <-s.path115Tasks
		// 	if ok {
		// 		// 关闭路径映射任务队列
		// close(s.path115Tasks)
		// 	}
		// }
		if s.pathBuffer != nil {
			s.pathBuffer = nil
		}
		s.Sync.Logger.Infof("所有同步用的通道都已关闭")
	}()
	go func() {
		for pathArr := range s.pathMapTasks {
			s.mapMu.Lock()
			s.pathMap[pathArr[0]] = pathArr[1]
			s.mapMu.Unlock()
		}
	}()
	if !s.IsFullSync {
		s.InitPathMap()
		s.InitPreNetFiles()
	} else {
		s.pathMap = make(map[string]string)
		s.preNetFiles = make(map[string]netFile)
	}
	if !s.CheckIsRunning() {
		return errors.New("手动停止通知")
	}
	go s.StartCurrentNetFileWork()
	if fileTreeErr := s.GetNetFileFiles(); fileTreeErr != nil {
		s.Sync.Logger.Warnf("获取115文件树失败: %v", fileTreeErr)
		return fileTreeErr
	}
	if !s.CheckIsRunning() {
		return errors.New(s.Sync.FailReason)
	}
	// 开始处理本次获取到的数据
	if processErr := s.ProcessNetFiles(); processErr != nil {
		return processErr
	}
	if !s.CheckIsRunning() {
		return errors.New(s.Sync.FailReason)
	}
	// 重新查询一次文件树，更新netFiles
	if compareErr := s.CompareLocalFiles(); compareErr != nil {
		return compareErr
	}
	return nil
}

func (s *SyncDriver115) InitPathMap() {
	if s.pathMap != nil {
		s.pathMap = nil // 先释放之前的指针，等待GC回收
	}
	s.pathMap = make(map[string]string)
	// 从数据库中把目录ID和本地文件路径映射到s.pathMap中
	offset := 0
	limit := 1000
	for {
		syncPathes, _ := Get115Pathes(offset, limit, s.Sync.SyncPathId)
		if len(syncPathes) == 0 {
			break
		}
		for _, path := range syncPathes {
			s.pathMap[path.FileId] = path.Path
		}
		offset += limit
		if len(syncPathes) < limit {
			break
		}
	}
}

func (s *SyncDriver115) PreloadDirTree(firstFile v115open.File) {
	// 查询文件夹ID的详情，获取路径数组，反转数组，一级一级查询文件夹，直到s.Sync.BaseCID
	// 启动多个协程，并发查
	// 先查询详情
	detail, err := s.client.GetFsDetailByCid(s.Sync.ctx, firstFile.FileId)
	if err != nil {
		s.Sync.Logger.Errorf("获取115文件夹详情失败: %v", err)
		return
	}
	pstr := ""
	found := false
	lastRemotePathPart := filepath.Base(s.Sync.RemotePath)
	depth := 0
	max := len(detail.Paths) - 1
	for i, path := range detail.Paths {
		// 一级一级查询文件夹，直到s.Sync.BaseCID
		if path.FileId == s.Sync.BaseCid {
			found = true
		}
		if !found {
			continue
		}
		if path.Name == lastRemotePathPart {
			pstr = ""
		} else {
			pstr = filepath.Join(pstr, path.Name)
		}
		if i <= max {
			depth++
		}
		if pstr != "" {
			// 入库
			AddSync115Path(s.Sync.SyncPathId, path.FileId, path.Name, pstr, filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, pstr), true)
		}
		// s.Sync.Logger.Infof("文件夹ID %s 名称：%s 路径：%s 需要预缓存子文件夹", path.FileId, path.Name, pstr)
	}
	s.Sync.Logger.Infof("预缓存文件夹深度1 %d", depth)
	lastName := ""
	if depth > 0 {
		lastName = detail.Paths[max].Name
	}
	detail = nil // 释放内存，等待GC回收
	if strings.Contains(strings.ToLower(lastName), "season") {
		s.Sync.Logger.Infof("最后一级是季文件夹，去掉一级: %s", lastName)
		depth--
	}
	if depth <= 0 {
		// 没有要预缓存的内容
		s.Sync.Logger.Warn("没有要预缓存的内容")
		return
	}
	s.Sync.Logger.Infof("预缓存文件夹深度2 %d", depth)
	// 启动多个协程，并发查，总共查 depth 层
	// 跟s.Sync.BaseCid开始查
	// 使用WaitGroup等待所有工作协程完成
	s.preloadPathTasks = make(chan preloadPathTask, SettingsGlobal.FileDetailThreads*5)
	// 启动buffer to task
	pathBufferCtx, pathBufferCtxCancel := context.WithCancel(context.Background())
	go s.bufferMonitor(pathBufferCtx)
	s.pathBufferWg = sync.WaitGroup{}
	// 将根目录加入预加载队列
	s.addPathToTasks(preloadPathTask{
		pathId: s.Sync.BaseCid,
		path:   "",
		depth:  1,
	})
	s.Sync.Logger.Infof("预加载目录任务队列已加入根目录 %s", s.Sync.BaseCid)
	// 启动工作协程，改造成使用wg控制结束，每次往队列增加任务就wg.Add(1)，每个任务完成就wg.Done()
	for i := 1; i <= SettingsGlobal.FileDetailThreads; i++ {
		go s.preloadDirByPathId(i, depth)
	}
	go func() {
		<-s.Sync.ctx.Done()
		for range s.preloadPathTasks {
			s.pathBufferWg.Done()
		}
	}()
	s.pathBufferWg.Wait()
	close(s.preloadPathTasks)
	pathBufferCtxCancel()
}

func (s *SyncDriver115) preloadDirByPathId(taskIndex int, maxDepth int) {
mainloop:
	for {
		select {
		case <-s.Sync.ctx.Done():
			return
		case task, ok := <-s.preloadPathTasks:
			if !ok {
				// 任务停止，清空buffer
				s.pathBufferMu.Lock()
				// 检查缓冲区是否为空
				if len(s.pathBuffer) == 0 {
					s.Sync.Logger.Infof("预加载目录任务队列 %d 完成退出，因为队列已空或关闭", taskIndex)
					s.pathBufferMu.Unlock()
					return
				}
				s.pathBufferMu.Unlock()
				s.Sync.Logger.Infof("预加载目录任务队列 %d 休息200毫秒，因为缓冲区不为空", taskIndex)
				time.Sleep(200 * time.Millisecond)
				continue mainloop
			}
			s.Sync.Logger.Infof("预加载目录任务队列 %d 开始处理目录 %s, 路径 %s, 深度 %d", taskIndex, task.pathId, task.path, task.depth)
			// 循环查找目录下的文件夹
			offset := 0
			limit := 1150
			waitSavePath := []*Sync115Path{}
		pageloop:
			for {
				// 查询目录下的文件夹和文件列表
				// 文件直接处理，文件夹继续加入队列
				resp, err := s.client.GetFsList(s.Sync.ctx, task.pathId, true, false, true, offset, limit)
				if err != nil {
					if strings.Contains(err.Error(), "context canceled") {
						s.pathBufferWg.Done()
						s.Sync.Logger.Infof("预加载目录任务队列 %d 上下文已取消，任务 %s", taskIndex, task.pathId)
						return
					}
					s.Sync.Logger.Errorf("预加载目录任务队列 %d 获取115文件夹列表失败: 父文件夹ID=%s, 路径：%s, offset=%d, limit=%d, %v", taskIndex, task.pathId, task.path, offset, limit, err)
					break pageloop
				}
				// s.Sync.Logger.Infof("预加载目录任务队列 %d 获取115文件夹列表成功: 父文件夹ID=%s, 路径：%s, offset=%d, limit=%d, count=%d", taskIndex, task.pathId, task.path, offset, limit, resp.Count)
			fileloop:
				for _, file := range resp.Data {
					if file.FileCategory == v115open.TypeFile {
						// 跳过文件，只处理文件夹
						continue fileloop
					}
					// 入库并添加缓存
					cp := filepath.Join(task.path, file.FileName)
					// 检查是否排除
					isExclude := s.Sync.IsExcludeName(file.FileName)
					if isExclude {
						s.Sync.Logger.Infof("文件夹 %s 被排除", file.FileName)
						// 加入排除路径
						s.excludePathsMu.Lock()
						s.excludePaths[file.FileId] = cp
						s.excludePathsMu.Unlock()
						continue fileloop
					}
					s.pathMapTasks <- []string{file.FileId, cp}
					pathObj, _ := AddSync115Path(s.Sync.SyncPathId, file.FileId, file.FileName, cp, filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, cp), false)
					s.Sync.Logger.Infof("预加载目录任务队列 %d 入库115路径: %s -> %s", taskIndex, file.FileId, filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, cp))
					waitSavePath = append(waitSavePath, pathObj)
					if len(waitSavePath) >= 100 {
						// 批量入库
						if err := db.Db.Create(waitSavePath).Error; err != nil {
							s.Sync.Logger.Errorf("预加载目录任务队列 %d 批量入库115路径失败: %v", taskIndex, err)
						}
						waitSavePath = []*Sync115Path{}
					}
					// s.Sync.Logger.Infof("预加载目录任务队列 %d 添加缓存: %s -> %s", taskIndex, file.FileId, cp)
					// 加入处理队列
					if task.depth+1 < maxDepth {
						s.Sync.Logger.Infof("预加载目录任务队列 %d 加入预加载目录任务: %s, 路径 %s, 深度 %d", taskIndex, file.FileId, cp, task.depth+1)
						s.addPathToTasks(preloadPathTask{
							pathId: file.FileId,
							path:   cp,
							depth:  task.depth + 1,
						})
					}
				}
				offset++
				if offset*limit >= resp.Count {
					break pageloop
				}
			}
			if len(waitSavePath) > 0 {
				// 批量入库
				if err := db.Db.Create(waitSavePath).Error; err != nil {
					s.Sync.Logger.Errorf("预加载目录任务队列 %d 批量入库115路径失败: %v", taskIndex, err)
				}
			}
			s.pathBufferWg.Done()
		}
	}
}

func (s *SyncDriver115) addPathToTasks(path preloadPathTask) {
	select {
	case <-s.Sync.ctx.Done():
		return // 任务停止
	case s.preloadPathTasks <- path:
		s.pathBufferWg.Add(1)
		// s.pathWgCount++
		s.Sync.Logger.Infof("任务 %s 已添加到目录预加载队列", path.pathId)
	default:
		// 超时，尝试添加到缓冲区
		s.addToBuffer(path)
	}
}

// bufferMonitor 监控缓冲区，尝试将缓冲区任务移入channel
func (s *SyncDriver115) bufferMonitor(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.Sync.ctx.Done():
			// 任务停止，清空buffer
			s.pathBufferMu.Lock()
			// 根据buffer长度操作wg
			s.pathBufferWg.Add(-len(s.pathBuffer))
			s.pathBuffer = []preloadPathTask{}
			s.pathBufferMu.Unlock()
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 定期尝试处理缓冲区
			s.tryDrainBuffer()
		}
	}
}

// tryDrainBuffer 尝试从缓冲区取出任务放入channel
func (s *SyncDriver115) tryDrainBuffer() {
	s.pathBufferMu.Lock()
	defer s.pathBufferMu.Unlock()

	if len(s.pathBuffer) == 0 {
		return
	}

	// 尝试将缓冲区任务移入channel
	for len(s.pathBuffer) > 0 {
		select {
		case <-s.Sync.ctx.Done():
			return // 任务停止
		case s.preloadPathTasks <- s.pathBuffer[0]:
			s.Sync.Logger.Infof("从缓冲区移出任务 %s 到任务处理队列", s.pathBuffer[0].path)
			s.pathBuffer = s.pathBuffer[1:]
		default:
			// channel已满，停止尝试
			return
		}
	}
}

// addToBuffer 添加任务到缓冲区
func (s *SyncDriver115) addToBuffer(task preloadPathTask) {
	s.pathBufferMu.Lock()
	defer s.pathBufferMu.Unlock()
	s.pathBufferWg.Add(1)
	s.pathBuffer = append(s.pathBuffer, task)
	s.Sync.Logger.Infof("任务 %s 已添加到缓冲区，当前缓冲区大小: %d", task.path, len(s.pathBuffer))
}

// 调用v115.GetFsList获取115网盘文件列表
// offset和limit参数用于分页获取文件列表
func (s *SyncDriver115) Get115FilesTotal(offset int, limit int) ([]v115open.File, int, error) {
	resp, err := s.client.GetFsList(s.Sync.ctx, s.Sync.BaseCid, false, false, false, offset, limit)
	if err != nil {
		s.Sync.Logger.Errorf("获取115网盘文件列表失败: offset=%d, limit=%d, %v", offset, limit, err)
		return nil, 0, err
	}
	if len(resp.Data) == 0 {
		s.Sync.Logger.Errorf("获取115网盘文件列表失败: offset=%d, limit=%d, 数据为空", offset, limit)
		return nil, 0, errors.New("获取115网盘文件列表失败, 数据为空")
	}
	return resp.Data, resp.Count, nil
}

func (s *SyncDriver115) GetNetFileFiles() error {
	s.Sync.UpdateSubStatus(SyncSubStatusProcessNetFileList)
	limit := 1150
	offset := 0
	var total, count int
	var listErr error
	var fileList []v115open.File
	// 先请求一次，拿到总数和第一个文件
	fileList, total, listErr = s.Get115FilesTotal(0, 1)
	s.Sync.Total = total
	s.Sync.UpdateTotal()
	if listErr != nil {
		return listErr
	}
	if len(fileList) == 0 {
		// 说明没有任何数据，不处理
		s.Sync.Logger.Infof("获取115网盘文件列表, offset=%d, count=%d, total=%d", offset, count, total)
		return nil
	}
	if s.IsFullSync {
		// 先查询fileList[0]的父文件夹，遍历他的所有子文件夹，将其路径保存到数据库
		s.PreloadDirTree(fileList[0])
	}
	if !s.CheckIsRunning() {
		s.Sync.Logger.Infof("GetNetFileFiles 手动停止任务")
		return errors.New("手动停止任务")
	}
	// 计算分页
	pageCount := total / limit
	if total%limit != 0 {
		pageCount++
	}
	// 创建任务和结果通道
	tasks := make(chan int, pageCount)
	// 工作协程数量
	numWorkers := SettingsGlobal.FileDetailThreads
	// 使用WaitGroup等待所有工作协程完成
	var wg sync.WaitGroup
	// 启动工作协程
	for i := 1; i <= numWorkers; i++ {
		wg.Go(func() {
			s.GetNetFileWorker(i, tasks)
		})
	}
	// 提交任务
	for i := 0; i < pageCount; i++ {
		offset := i * limit
		tasks <- offset
	}
	close(tasks) // 关闭任务通道，表示没有更多任务
	wg.Wait()
	s.Sync.Logger.Infof("完成文件列表查询，开始处理网盘文件，共 %d 个文件", total)
	return nil
}

// 调用v115.GetFsList获取115网盘文件列表
// offset和limit参数用于分页获取文件列表
func (s *SyncDriver115) GetNetFileWorker(id int, tasks <-chan int) {
	limit := 1150
	waitSaveFile := []*SyncFile{}
mainloop:
	for {
		select {
		case <-s.Sync.ctx.Done():
			// 任务已结束，不保存数据
			return
		case offset, ok := <-tasks:
			if !ok {
				// 任务通道已关闭，退出循环
				break mainloop
			}
			resp, err := s.client.GetFsList(s.Sync.ctx, s.Sync.BaseCid, false, false, false, offset, limit)
			if err != nil {
				s.Sync.Logger.Errorf("获取115网盘文件列表失败: offset=%d, limit=%d, %v", offset, limit, err)
				s.Sync.FailReason = err.Error()
				s.Stop()
				return
			}
			if len(resp.Data) == 0 {
				s.Sync.Logger.Errorf("获取115网盘文件列表失败: offset=%d, limit=%d, 数据为空", offset, limit)
				continue
			}
			i := offset
		fileloop:
			for _, file := range resp.Data {
				i++
				if file.Aid != "1" {
					s.Sync.Logger.Infof("文件 %s 已放入回收站或删除，跳过", file.FileName)
					continue fileloop
				}
				isVideo := s.Sync.IsValidVideoExt(file.FileName)
				isMeta := s.Sync.IsValidMetaExt(file.FileName)
				s.Sync.Logger.Infof("扫描到文件 %s => %s 视频:%v 元数据:%v", file.FileName, file.FileId, isVideo, isMeta)
				if file.FileCategory == v115open.TypeFile && !isVideo && !isMeta {
					s.Sync.Logger.Infof("非视频或元数据文件不需要处理: %s", file.FileName)
					continue fileloop // 如果不需要处理，则跳过
				}
				if isVideo && file.FileSize < s.Sync.SyncPath.GetMinVideoSize()*1024*1024 {
					s.Sync.Logger.Infof("视频文件%s大小%d小于%d最小要求，不需要处理", file.FileName, file.FileSize, s.Sync.SyncPath.GetMinVideoSize()*1024*1024)
					continue fileloop
				}
				if isMeta && s.Sync.SyncPath.GetDownloadMeta() == 0 {
					// 如果是元数据文件且设置为不下载，则跳过检查（代表着不上传）
					s.Sync.Logger.Infof("网盘元数据文件 %s 由于关闭了元数据下载所以不需要处理", file.FileName)
					continue fileloop
				}
				// 使用fileid查询文件是否存在，不存在则需要入库
				exists := false
				var existsNetFile netFile
				ok := false
				if existsNetFile, ok = s.preNetFiles[file.FileId]; ok {
					exists = true
					// 如果是existsNetFile.localPath以.iso.strm结尾，则说明是ISO生成的strm文件, 删除数据库记录，标记为不存在
					if strings.HasSuffix(existsNetFile.localPath, ".iso.strm") {
						oldLocalPath := existsNetFile.localPath
						err := db.Db.Exec("DELETE FROM sync_files WHERE file_id = ?", file.FileId).Error
						if err != nil {
							s.Sync.Logger.Errorf("删除ISO文件 %s 数据库记录失败: %v", oldLocalPath, err)
						}
						exists = false
						s.preNetFileMu.Lock()
						delete(s.preNetFiles, file.FileId)
						s.preNetFileMu.Unlock()
						s.Sync.Logger.Infof("文件 %s 之前是ISO生成的strm文件，删除数据库记录，标记为不存在，重新处理", oldLocalPath)
					}
				}
				isExclude := s.Sync.IsExcludeName(file.FileName)
				if isExclude {
					s.Sync.Logger.Infof("文件 %s 被排除", file.FileName)
					// 如果本地文件存在则删除
					if exists && helpers.PathExists(existsNetFile.localPath) {
						DeleteFileByPickCode(file.PickCode)
						s.DeleteLocalFile(existsNetFile.localPath)
					}
					continue fileloop
				}
				// 加入当前查询到的文件ID任务队列
				s.currentNetFileTasks <- file.FileId
				if !exists {
					// 不存在就入库
					path := ""
					localFilePath := ""
					newFile, _ := AddSyncFile(s.Sync.SyncPath, file.FileId, file.Pid, file.FileName, path, localFilePath, file.FileSize, file.PickCode, file.Sha1, isMeta, isVideo, "", true, false, file.Ptime)
					waitSaveFile = append(waitSaveFile, newFile)
					// 每10个一批入库
					if len(waitSaveFile) >= 10 {
						// 批量入库
						tx := db.Db.Create(waitSaveFile)
						err := tx.Error
						rowsAffected := tx.RowsAffected
						if err != nil {
							s.Sync.Logger.Errorf("批量入库 %d 个文件失败, 影响 %d 行, %v", len(waitSaveFile), rowsAffected, err)
						} else {
							// s.Sync.Logger.Infof("批量入库 %d 个文件, 影响 %d 行", len(waitSaveFile), rowsAffected)
						}
						waitSaveFile = []*SyncFile{}
					}
					s.Sync.Logger.Debugf("文件 %s 加入处理列表，因为不在数据库中", newFile.FileName)
				} else {
					// s.Sync.Logger.Debugf("文件 %s 跳过处理，因为已在数据库中", file.FileName)
				}
			}
		}
	}
	// 最后将未入库的全部入库
	if len(waitSaveFile) > 0 {
		// 批量入库
		db.Db.Create(waitSaveFile)
		waitSaveFile = nil
		// s.Sync.Logger.Infof("批量入库 %d 个文件", len(waitSaveFile))
	}
	// s.Sync.Logger.Infof("工作线程 %d 完成查询文件列表任务", id)
}

// 处理网盘文件
// 从SyncFile表中获取所有文件
// 先拿FileId去SyncFileId表中查询，如果不存在说明网盘删除了该文件，触发删除流程
// 如果存在，检查本地文件是否存在，不存在则触发下载或者生成strm流程
// 如果存在，检查是否需要更新strm
// 如果路径为空，则先补全路径
func (s *SyncDriver115) ProcessNetFiles() error {
	if !s.CheckIsRunning() {
		return errors.New("手动停止任务")
	}
	var wg sync.WaitGroup
	// 启动一个strm生成工作协程
	wg.Add(1)
	s.strmTasks = make(chan *SyncFile, 100)
	go s.StartStrmWork(s.strmTasks, &wg)
	// 启动一个下载生成工作协程
	wg.Add(1)
	s.downloadTasks = make(chan *SyncFile, 100)
	go s.StartDownloadWork(s.downloadTasks, &wg)
	// 启动一个删除工作协程
	wg.Add(1)
	s.deleteTasks = make(chan *SyncFile, 100)
	go s.StartDeleteWork(s.deleteTasks, &wg)
	// 启动SettingsGlobal.FileDetailThreads个补充路径协程
	var pathWg sync.WaitGroup
	for i := 0; i < SettingsGlobal.FileDetailThreads; i++ {
		// 在限速器控制下执行StartPathWork
		go s.StartPathWorkWithLimiter(s.path115Tasks, &pathWg)
	}
	// 从SyncFile分页取数据，每次1000条
	total, terr := GetSyncFileTotal(s.Sync.SyncPathId)
	if terr != nil {
		s.Sync.Logger.Errorf("从数据库查询文件总数失败: %v", terr)
		return terr
	}
	limit := 1000
	// 计算页数
	// 计算分页
	pageCount := total / limit
	if total%limit != 0 {
		pageCount++
	}
	dbTasks := make(chan int, pageCount)
	// 启动工作协程
	var dbWg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		dbWg.Add(1)
		go func(dbTasks chan int) {
			defer func() {
				defer dbWg.Done()
			}()
		mainloop:
			for {
				select {
				case <-s.Sync.ctx.Done():
					// 任务已结束，不保存数据
					break mainloop
				case page, ok := <-dbTasks:
					if !ok {
						// 通道已关闭，退出循环
						break mainloop
					}
					offset := page * limit
					s.Sync.Logger.Infof("工作线程 %d 开始处理115网盘文件任务，本次查询 %d 页，偏移量 %d", i, pageCount, offset)
					files, err := GetSyncFiles(offset, limit, s.Sync.SyncPathId)
					if err != nil {
						s.Sync.Logger.Errorf("从115网盘文件表查询数据失败: offset=%d, limit=%d, %v", offset, limit, err)
						break mainloop
					}
					if len(files) == 0 {
						s.Sync.Logger.Infof("从115网盘文件表查询数据完成: offset=%d, limit=%d, 数据为空", offset, limit)
						break mainloop
					}
				fileloop:
					for _, file := range files {
						s.Sync.Logger.Infof("文件ID %s => %s 开始处理", file.FileId, file.FileName)
						// 检查网盘文件是否存在
						if _, ok := s.currentNetFiles[file.FileId]; !ok {
							// 网盘文件不存在，进入删除流程
							s.deleteTasks <- file
							s.Sync.Logger.Infof("文件ID %s => %s 加入删除处理列表，因为在115网盘文件表中不存在", file.FileId, file.FileName)
							continue fileloop
						}
						// 检查目录是否被排除
						s.excludePathsMu.RLock()
						excludePath, ok := s.excludePaths[file.ParentId]
						s.excludePathsMu.RUnlock()
						if ok {
							s.Sync.Logger.Infof("文件ID %s => %s 路径 %s 被排除了，跳过4", file.FileId, file.FileName, excludePath)
							continue fileloop
						}
						if file.LocalFilePath == "" {
							// 检查是否已经加入过处理列表
							s.path115TaskIdsMu.Lock()
							if _, ok := s.path115TaskIds[file.ParentId]; ok {
								s.path115TaskIdsMu.Unlock()
								// 需要等待路径补全完成后继续后续流程
								s.Sync.Logger.Infof("文件夹ID %s 路径补充未完成，等待路径补全完成后会自动处理该文件", file.ParentId)
								continue fileloop
							} else {
								s.path115TaskIds[file.ParentId] = true
								s.path115TaskIdsMu.Unlock()
								// 触发补充路径流程
								pathWg.Add(1)
								s.path115Tasks <- file
								s.Sync.Logger.Infof("文件夹ID %s 路径补充已加入处理列表", file.ParentId)
							}
							// s.Sync.Logger.Infof("文件ID %s => %s 路径补充开始，父路径ID：%s", file.FileId, file.FileName, file.ParentId)
							continue fileloop
						} else {
							isExclude := s.CheckPathExclude(file.FileId, file.FileName, file.LocalFilePath)
							if !isExclude {
								s.Sync.Logger.Infof("文件ID %s 名称：%s 路径 %s，被排除了，跳过5", file.FileId, file.FileName, file.LocalFilePath)
								continue fileloop
							}
						}
						// 检查本地文件是否存在
						if !helpers.PathExists(file.LocalFilePath) {
							// 本地文件不存在
							if file.IsMeta {
								// 元文件，触发下载流程
								s.downloadTasks <- file
								continue fileloop
							}
						}
						if file.IsVideo {
							// 触发strm流程，检查是否更新
							s.strmTasks <- file
							continue fileloop
						}
						// if file.IsMeta {
						// 检查文件大小是否相同
						// localFileInfo, err := os.Stat(file.LocalFilePath)
						// if err != nil {
						// 	s.Sync.Logger.Errorf("获取本地文件信息失败: %v", err)
						// 	continue fileloop
						// }
						// if file.FileSize != localFileInfo.Size() {
						// 	s.Sync.Logger.Infof("文件ID %s => %s 本地文件大小 %d 与115网盘文件大小 %d 不同，重新下载", file.FileId, file.FileName, localFileInfo.Size(), file.FileSize)
						// 	// 先删除本地文件，然后触发下载
						// 	if err := os.Remove(file.LocalFilePath); err != nil {
						// 		s.Sync.Logger.Errorf("删除本地文件失败: %v", err)
						// 		continue fileloop
						// 	}
						// 	// 文件大小不同，触发下载流程
						// 	s.downloadTasks <- file
						// 	continue fileloop
						// }
						// 检查修改时间是否相同
						// if file.MTime > localFileInfo.ModTime().Unix() {
						// 	s.Sync.Logger.Infof("文件ID %s => %s 本地文件修改时间 %d 早与115网盘文件修改时间 %d，重新下载", file.FileId, file.FileName, localFileInfo.ModTime().Unix(), file.MTime)
						// 	// 先删除本地文件，然后触发下载
						// 	if err := os.Remove(file.LocalFilePath); err != nil {
						// 		s.Sync.Logger.Errorf("删除本地文件失败: %v", err)
						// 		continue fileloop
						// 	}
						// 	// 文件修改时间不同，触发下载流程
						// 	s.downloadTasks <- file
						// 	continue fileloop
						// }
						// }
					}
				}
			}
			// s.Sync.Logger.Infof("工作线程 %d 完成处理115网盘文件任务", i)
		}(dbTasks)
	}
	// 添加任务
	for i := 0; i <= pageCount; i++ {
		dbTasks <- i
	}
	close(dbTasks)
	// s.Sync.Logger.Infof("所有115网盘文件任务已添加，等待处理完成")
	dbWg.Wait()
	// s.Sync.Logger.Infof("所有115网盘文件任务处理完成")
	// 需要先完成路径填充，因为路径填充有可能给其他任务队列添加任务
	pathWg.Wait()
	close(s.path115Tasks)
	// s.Sync.Logger.Infof("所有115网盘文件路径补充任务处理完成")
	// 结束其他任务队列
	close(s.strmTasks)
	close(s.downloadTasks)
	close(s.deleteTasks)
	wg.Wait() // 等待所有任务结束
	s.Sync.Logger.Infof("所有115网盘文件任务处理完成")
	return nil
}

// StartPathWorkWithLimiter 在限速器控制下执行路径补充工作
// tokenBucket: 令牌桶限速器，控制任务执行速率
func (s *SyncDriver115) StartPathWorkWithLimiter(path115Tasks chan *SyncFile, wg *sync.WaitGroup) {
mainloop:
	for {
		select {
		case <-s.Sync.ctx.Done():
			// 任务已结束，不保存数据
			break mainloop
		case file, ok := <-path115Tasks:
			if !ok {
				// 通道已关闭，退出循环
				s.Sync.Logger.Infof("路径补充任务通道已关闭，退出循环")
				break mainloop
			}
			// 检查是否存在
			s.mapMu.Lock()
			path, ok := s.pathMap[file.ParentId]
			s.mapMu.Unlock()
			if !ok {
				// 补充路径
				isExclude := false
				path, isExclude = s.FillPath(file)
				// s.Sync.Logger.Infof("文件ID %s => %s 补充路径完成，路径：%s，目录ID: %s", file.FileId, file.FileName, path, file.ParentId)
				if path == "" && isExclude {
					// 路径为空，跳过
					wg.Done()
					continue mainloop
				}
			}
			file.Sync = s.Sync
			file.SyncPath = s.Sync.SyncPath
			file.Path = path
			file.MakeFullLocalPath()
			// s.Sync.Logger.Infof("文件ID %s => %s 路径补充完成，路径：%s", file.FileId, file.LocalFilePath, file.Path)
			err := file.Save()
			if err != nil {
				s.Sync.Logger.Errorf("文件ID %s => %s 路径补充保存失败，错误：%v", file.FileId, file.LocalFilePath, err)
				wg.Done() // 任务失败，也需要调用Done
				continue mainloop
			}
			if !helpers.PathExists(file.LocalFilePath) {
				// 本地文件不存在
				if file.IsMeta {
					// 元文件，触发下载流程
					// s.Sync.Logger.Infof("文件ID %s => %s 为元文件，触发下载流程", file.FileId, file.LocalFilePath)
					s.downloadTasks <- file
				}
			}
			if file.IsVideo {
				// 触发strm流程，检查是否更新
				// s.Sync.Logger.Infof("文件ID %s => %s 为视频文件，触发strm生成流程", file.FileId, file.LocalFilePath)
				s.strmTasks <- file
			}
			// 查询所有同一个父目录但是没有补齐路径的文件
			files, err := GetSyncFilesByPathIdAndLocalPathEmpty(file.ParentId)
			if err != nil {
				s.Sync.Logger.Errorf("查询父目录 %s 下没有补齐路径的文件失败，错误：%v", file.ParentId, err)
				wg.Done() // 任务失败，也需要调用Done
				continue mainloop
			}
			// 遍历所有没有补齐路径的文件，触发补充路径流程
			for _, f := range files {
				f.Sync = s.Sync
				f.SyncPath = s.Sync.SyncPath
				f.Path = path
				f.MakeFullLocalPath()
				err := f.Save()
				if err != nil {
					s.Sync.Logger.Errorf("文件ID %s => %s 路径补充保存失败，错误：%v", f.FileId, f.LocalFilePath, err)
					wg.Done() // 任务失败，也需要调用Done
					continue mainloop
				}
				if !helpers.PathExists(f.LocalFilePath) {
					// 本地文件不存在
					if f.IsMeta {
						// 元文件，触发下载流程
						// s.Sync.Logger.Infof("文件ID %s => %s 为元文件，触发下载流程", file.FileId, file.LocalFilePath)
						s.downloadTasks <- f
					}
				}
				if f.IsVideo {
					// 触发strm流程，检查是否更新
					// s.Sync.Logger.Infof("文件ID %s => %s 为视频文件，触发strm生成流程", f.FileId, f.LocalFilePath)
					s.strmTasks <- f
				}
			}
			wg.Done() // 每个任务完成后调用Done
		}
	}
}

func (s *SyncDriver115) FillPath(file *SyncFile) (string, bool) {
	// 检查目录是否被排除
	s.excludePathsMu.RLock()
	excludePath, ok := s.excludePaths[file.ParentId]
	s.excludePathsMu.RUnlock()
	if ok {
		s.Sync.Logger.Infof("文件ID %s => %s 路径 %s 被排除了，跳过6", file.FileId, file.FileName, excludePath)
		return "", true
	}
	if file.ParentId == s.Sync.BaseCid {
		// 根目录文件，直接返回空路径
		s.Sync.Logger.Infof("文件ID %s => %s 为根目录下的文件，直接返回空路径", file.FileId, file.LocalFilePath)
		return "", false
	}
	// 获取完整路径
	detail, err := s.client.GetFsDetailByCid(s.Sync.ctx, file.ParentId)
	if err != nil {
		s.Sync.Logger.Infof("查询路径详情时出错，路径ID:%s，错误：%v", file.ParentId, err)
		return "", false
	}
	// s.Sync.Logger.Infof("文件夹ID %s 名称：%s 获取路径，路径数组：%+v", file.ParentId, detail.FileName, detail.Paths)
	if strings.Contains(detail.FileName, "****") {
		s.Sync.Logger.Infof("文件夹ID %s 名称：%s 包含 **** 号，跳过", file.ParentId, detail.FileName)
		return "", false
	}
	pathStr := ""
	foundBase := false
	lastRemotePathPart := filepath.Base(s.Sync.RemotePath)
	isExclude := false // 如果为true，则后续所有路径全部排除
	for _, p := range detail.Paths {
		if p.FileId == s.Sync.BaseCid {
			foundBase = true
			continue
		}
		if !foundBase || p.Name == lastRemotePathPart {
			continue
		}
		if pathStr == "" {
			pathStr = p.Name
		} else {
			pathStr = filepath.Join(pathStr, p.Name)
		}
		if isExclude {
			// 加入排除路径
			s.excludePathsMu.Lock()
			s.excludePaths[p.FileId] = pathStr
			s.excludePathsMu.Unlock()
		} else {
			if !s.CheckPathExclude(p.FileId, p.Name, pathStr) {
				// 加入排除路径
				s.excludePathsMu.Lock()
				s.excludePaths[p.FileId] = pathStr
				s.excludePathsMu.Unlock()
				isExclude = true
			}
		}
		// 检查是否存在该路径
		// if !CheckSyncPathIdExists(p.FileId) {
		// 直接写入数据库
		s.mapMu.Lock()
		if _, ok := s.pathMap[p.FileId]; ok {
			s.mapMu.Unlock()
			continue
		}
		if !isExclude {
			s.pathMapTasks <- []string{p.FileId, pathStr}
			AddSync115Path(s.Sync.SyncPathId, p.FileId, p.Name, pathStr, filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, pathStr), true)
		}
		s.mapMu.Unlock()
	}
	if pathStr == "" {
		pathStr = detail.FileName
	} else {
		pathStr = filepath.Join(pathStr, detail.FileName)
	}
	if isExclude {
		s.Sync.Logger.Infof("文件夹ID %s => %s被排除，跳过处理7", file.ParentId, pathStr)
		return "", true
	}
	if !s.CheckPathExclude(file.ParentId, detail.FileName, pathStr) {
		// 加入排除路径
		s.excludePathsMu.Lock()
		s.excludePaths[file.ParentId] = pathStr
		s.excludePathsMu.Unlock()
		s.Sync.Logger.Infof("文件夹ID %s => %s被排除，跳过处理8", file.ParentId, pathStr)
		return "", true
	}
	s.pathMapTasks <- []string{file.ParentId, pathStr}
	// 直接写入数据库
	AddSync115Path(s.Sync.SyncPathId, file.ParentId, detail.FileName, pathStr, filepath.Join(s.Sync.LocalPath, s.Sync.RemotePath, pathStr), true)
	return pathStr, false
}

func (s *SyncDriver115) CheckPathExclude(pathFileId, pathFileName, pathStr string) bool {
	isExclude := false
	// 检查pathStr中是否某一级被排除
	parts := strings.SplitSeq(pathStr, string(os.PathSeparator))
	for part := range parts {
		if s.Sync.IsExcludeName(part) {
			isExclude = true
			s.Sync.Logger.Infof("路径 %s 中包含被排除的目录 %s， 跳过处理2", pathStr, part)
			break
		}
	}
	if !isExclude {
		isExclude = s.Sync.IsExcludeName(pathFileName)
		if isExclude {
			s.Sync.Logger.Infof("路径 %s 中包含被排除的文件名 %s，跳过处理3", pathStr, pathFileName)
		}
	}
	if !isExclude {
		return true
	}
	// 删除这个目录开头的所有记录
	DeleteExcludePathFile(pathStr, s.Sync.SyncPathId, false)
	// 删除目录
	DeletePathByFileId(pathFileId)
	return false
}

var uploadDirNames = []string{
	"extrafanart",
	"exfanarts",
	"extrafanarts",
	"extras",
	"specials",
	"shorts",
	"scenes",
	"featurettes",
	"behind the scenes",
	"trailers",
	"interviews",
}

// 在处理本地文件files表，每次取1000个文件来进行对比
// 对比的逻辑是根据file_id字段去sync_files表查找，看是否有对应的记录
// 如果不存在则判定为删除，如果存在（前面已经处理过，这里忽略）
func (s *SyncDriver115) CompareLocalFiles() error {
	if !s.CheckIsRunning() {
		// 任务已被停止
		return errors.New("手动停止任务")
	}
	s.Sync.UpdateSubStatus(SyncSubStatusProcessLocalFileList)
	netFilesByLocalPath := s.GetExistsNetFileAndUploadStatus()
	rootDir := s.Sync.GetBaseDir()
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if !s.CheckIsRunning() {
			// 任务已被停止
			return errors.New("手动停止任务")
		}
		if err != nil || path == "." || strings.Contains(path, ".verysync") || strings.Contains(path, ".deletedByTMM") {
			// 跳过根目录本身
			// 跳过微力同步和TMM的临时文件夹中的文件
			return nil
		}
		if s.Sync.IsValidMetaExt(info.Name()) && s.Sync.SyncPath.GetDownloadMeta() == 0 {
			// 如果是元数据文件且设置为不下载，则跳过检查（代表着不上传）
			s.Sync.Logger.Infof("本地元数据文件 %s 由于关闭了元数据下载所以不需要处理", info.Name())
			return nil
		}
		// 不处理本地目录
		if info.IsDir() {
			metaFileName := filepath.Join(path, ".meta")
			os.Remove(metaFileName) // 删除.meta文件
			// 如果目录是空的则删除目录
			dirEntries, rerr := os.ReadDir(path)
			if rerr != nil {
				s.Sync.Logger.Errorf("读取目录 %s 失败: %v", path, err)
				return nil
			}
			if len(dirEntries) == 0 {
				os.Remove(path)
			}
			return nil
		}
		if info.Name() == ".fileid_2_localpath_map.json" {
			os.Remove(path)
			return nil
		}
		ext := filepath.Ext(info.Name())
		isVideo := ext == ".strm"
		isMeta := s.Sync.IsValidMetaExt(info.Name())
		// 检查当前文件在数据库是否存在
		// db115File := GetFileByLocalFilePath(path)
		// 检查是否在netFilesByLocalPath中
		upload := false
		uploaded, ok := netFilesByLocalPath[path]
		if ok {
			if uploaded {
				// 文件存在且已上传，跳过
				s.Sync.Logger.Infof("本地文件 %s 在数据库中存在且已上传，跳过", path)
				return nil
			}
		} else {
			if isVideo {
				// 删除
				s.Sync.Logger.Infof("本地strm文件 %s 在数据库中不存在，需要删除", path)
				s.DeleteLocalFile(path)
				return nil
			}
		}
		if isMeta {
			// 上传元数据
			switch s.Sync.SyncPath.GetUploadMeta() {
			case int(SyncTreeItemMetaActionUpload):
				upload = true
				s.Sync.Logger.Infof("本地元数据文件 %s 在数据库中不存在，设置为上传", path)
			case int(SyncTreeItemMetaActionDelete):
				// 删除
				s.Sync.Logger.Infof("本地元数据文件 %s 在数据库中不存在，设置为删除", path)
				s.DeleteLocalFile(path)
				return nil
			case int(SyncTreeItemMetaActionKeep):
				// 保留
				s.Sync.Logger.Infof("本地元数据文件 %s 在数据库中不存在，设置为保留", path)
				return nil
			}
		}
		// 这是要上传的
		if !upload {
			s.Sync.Logger.Infof("本地文件 %s 在数据库中不存在且不上传", path)
			return nil
		}
		// 检查父目录是否存在
		parentPath := filepath.Dir(path)
		s.Sync.Logger.Infof("开始查找文件 %s 的父目录是否存在: %s", info.Name(), parentPath)
		db115Path := GetPathByLocalPath(parentPath)
		parentId := ""
		if db115Path == nil {
			s.Sync.Logger.Infof("父目录 %s 在数据库中没有记录，检查是否可以创建并上传文件", parentPath)
			// 父目录不存在，检查父目录的父目录
			parentFoldName := filepath.Base(parentPath)
			if !slices.Contains(uploadDirNames, parentFoldName) {
				s.Sync.Logger.Infof("目录 %s 不是允许上传的目录，跳过", parentFoldName)
				// 父目录不是允许上传的目录，跳过
				return nil
			}
			// 检查父目录的父目录
			parentParentPath := filepath.Dir(parentPath)
			s.Sync.Logger.Infof("开始查找文件 %s 的父目录的父目录是否存在: %s", info.Name(), parentParentPath)
			db115ParentPath := GetPathByLocalPath(parentParentPath)
			if db115ParentPath == nil {
				// 父目录的父目录不存在，删除
				os.Remove(path)
				s.Sync.Logger.Infof("父目录 %s 在网盘中不存在，将本地文件标记为删除：%s", parentPath, path)
				s.checkAndDeleteDir(parentPath)
				return nil
			} else {
				s.Sync.Logger.Infof("父目录 %s 的父目录 %s 在数据库中存在，文件ID %s", parentPath, parentParentPath, db115ParentPath.FileId)
			}
			// 创建父目录
			parentId, err = s.client.MkDir(s.Sync.ctx, db115ParentPath.FileId, parentFoldName)
			if err != nil {
				s.Sync.Logger.Errorf("创建目录 %s 失败: %v", parentFoldName, err)
				return nil
			} else {
				s.Sync.Logger.Infof("成功新建父目录 %s，文件ID %s", parentFoldName, parentId)
			}
			// 保存数据
			_, err = AddSync115Path(s.Sync.SyncPathId, parentId, parentFoldName, filepath.Join(db115ParentPath.Path, parentFoldName), parentPath, true)
			if err != nil {
				s.Sync.Logger.Errorf("保存目录 %s 失败: %v", parentFoldName, err)
				return nil
			} else {
				s.Sync.Logger.Infof("成功保存父目录 %s 到数据库", parentFoldName)
			}
		} else {
			s.Sync.Logger.Infof("父目录 %s 存在，文件ID %s", parentPath, db115Path.FileId)
			parentId = db115Path.FileId
		}
		// 生成一个文件
		db115File, err := AddSyncFile(s.Sync.SyncPath, "", parentId, info.Name(), db115Path.Path, path, info.Size(), "", "", isMeta, isVideo, "", false, true, info.ModTime().Unix())
		if err != nil {
			s.Sync.Logger.Errorf("保存文件 %s 失败: %v", path, err)
			return nil
		}
		// 加入上传队列
		taskErr := AddUploadTaskFromSyncFile(db115File)
		if taskErr != nil {
			s.Sync.Logger.Errorf("创建上传任务失败: %v", taskErr)
			return nil
		}
		s.Sync.NewUpload++
		return nil
	})
	netFilesByLocalPath = nil // 销毁netFilesByLocalPath，等待GC释放内存
	if err != nil {
		s.Sync.Logger.Errorf("遍历本地文件夹出错: %v", err)
		return err
	}
	return nil
}

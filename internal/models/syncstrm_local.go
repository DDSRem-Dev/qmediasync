package models

import (
	"Q115-STRM/internal/db"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type SyncDriverLocal struct {
	SyncDriverBase
}

func NewSyncDriverLocal(sync *Sync) *SyncDriverLocal {
	return &SyncDriverLocal{
		SyncDriverBase: SyncDriverBase{
			Sync: sync,
		},
	}
}

func (s *SyncDriverLocal) DoSync() error {
	if runtime.GOOS != "windows" && !strings.HasPrefix(s.Sync.RemotePath, "/") {
		// 处理Linux/Mac下的绝对路径
		s.Sync.RemotePath = "/" + s.Sync.RemotePath
	}
	go s.StartCurrentNetFileWork()
	defer func() {
		if s.currentNetFileTasks != nil {
			close(s.currentNetFileTasks)
		}
	}()
	s.InitPreNetFiles() // 先初始化赏赐查询到的所有文件到缓存，供查询使用，提高效率
	if getErr := s.GetNetFileFiles(); getErr != nil {
		return getErr
	}
	if compareErr := s.CompareLocalFiles(); compareErr != nil {
		return compareErr
	}
	return nil
}

// 查询源文件数
// 从s.Sync.RemotePath开始递归读取文件列表
func (s *SyncDriverLocal) GetNetFileFiles() error {
	// 检查任务是否已被停止
	if !s.CheckIsRunning() {
		return errors.New("手动停止任务")
	}
	// 检查来源目录下是否有文件，如果为空则报错
	// 查询文件列表
	fileList, err := os.ReadDir(s.Sync.BaseCid)
	if err != nil {
		s.Sync.Logger.Errorf("来源路径 %s 读取文件列表失败，请检查是否挂载失效 %v", s.Sync.BaseCid, err)
		return fmt.Errorf("来源路径 %s 读取文件列表失败，请检查是否挂载失效 %v", s.Sync.BaseCid, err)
	}
	if len(fileList) == 0 {
		s.Sync.Logger.Infof("来源路径 %s 为空，请检查是否挂载失效", s.Sync.BaseCid)
		return fmt.Errorf("来源路径 %s 为空，请检查是否挂载失效", s.Sync.BaseCid)
	}
	s.Sync.UpdateSubStatus(SyncSubStatusProcessNetFileList)
	s.pathTasks = make(chan string, SettingsGlobal.FileDetailThreads)
	ctx, ctxCancel := context.WithCancel(context.Background())
	// 启动buffer to task
	go s.bufferMonitor(ctx)
	s.wg = sync.WaitGroup{}
	// 加入根目录
	s.addPathToTasks(s.Sync.BaseCid)
	s.Sync.Logger.Infof("将根目录 %s 加入路径队列", s.Sync.RemotePath)
	for i := 0; i < SettingsGlobal.FileDetailThreads; i++ {
		// 在限速器控制下执行StartPathWork
		go s.startPathWorkWithLimiter(i)
	}
	go func() {
		<-s.Sync.ctx.Done()
		time.Sleep(2 * time.Second) // 休息两秒，让监控协程完成
		for range s.pathTasks {
			s.wg.Done()
		}
	}()
	s.wg.Wait()
	for len(s.pathBuffer) > 0 {
		time.Sleep(100 * time.Microsecond) // 休息10微秒，等待其他任务处理完成
		s.Sync.Logger.Infof("路径队列中还有 %d 个路径未处理", len(s.pathBuffer))
	}
	close(s.pathTasks)
	ctxCancel()
	// 处理完以后启开始处理数据库数据
	// 开始处理本次获取到的数据
	s.Sync.UpdateTotal()
	s.ProcessNetFiles()
	return nil
}

func (s *SyncDriverLocal) startPathWorkWithLimiter(taskIndex int) {
mainloop:
	for {
		select {
		case <-s.Sync.ctx.Done():
			return
		case path, ok := <-s.pathTasks:
			if !ok {
				time.Sleep(100 * time.Microsecond) // 休息10微秒，等待其他任务处理完成
				return
			}
			// 查询目录下的文件夹和文件列表
			// 文件直接处理，文件夹继续加入队列
			waitSaveFile := []*SyncFile{}
			s.Sync.Logger.Infof("任务 %d 开始处理目录 %s,等待开关放行", taskIndex, path)
			// 检查文件夹是否被排除
			if !s.CheckPathExclude(path) {
				s.wg.Done()
				s.Sync.Logger.Infof("文件夹 %s 被排除，不需要处理", path)
				continue mainloop
			}
			// 查询文件列表
			fileList, err := os.ReadDir(path)
			if err != nil {
				s.wg.Done()
				s.Sync.Logger.Errorf("获取本地文件列表失败: %v", err)
				continue mainloop
			}
			if len(fileList) == 0 {
				s.wg.Done()
				s.Sync.Logger.Infof("本地文件列表为空: %v", fileList)
				continue mainloop
			}
			s.Sync.Logger.Infof("任务 %d 处理目录 %s, 文件数量: %d", taskIndex, path, len(fileList))
		fileloop:
			for _, file := range fileList {
				if !s.CheckIsRunning() {
					s.wg.Done()
					s.Sync.Logger.Infof("任务队列 %d 上下文已取消，任务 %s", taskIndex, path)
					return
				}
				newPath := filepath.Join(path, file.Name())
				s.Sync.Logger.Infof("任务 %d 处理文件或目录 %s", taskIndex, newPath)
				s.Sync.Total++
				// 检查是否视频或元数据
				isVideo := s.Sync.IsValidVideoExt(file.Name())
				isMeta := s.Sync.IsValidMetaExt(file.Name())
				if isMeta && s.Sync.SyncPath.GetDownloadMeta() == 0 {
					// 如果是元数据文件且设置为不下载，则跳过检查（代表着不上传）
					s.Sync.Logger.Infof("本地元数据文件 %s 由于关闭了元数据下载所以不需要处理", file.Name())
					continue fileloop
				}
				if !file.IsDir() && !isVideo && !isMeta {
					s.Sync.Logger.Warnf("非视频或元数据文件不需要处理: %s", file.Name())
					continue fileloop
				}
				// 检查视频大小是否合规
				stat, err := os.Stat(newPath)
				if err != nil {
					s.Sync.Logger.Errorf("获取文件 %s 大小失败: %v", newPath, err)
					continue fileloop
				}
				size := stat.Size()
				if isVideo && size < s.Sync.SyncPath.GetMinVideoSize()*1024*1024 {
					s.Sync.Logger.Warnf("视频文件 %s 大小%d 没有达到最小大小: %d", newPath, size, s.Sync.SyncPath.GetMinVideoSize()*1024*1024)
					continue fileloop
				}
				// 查看名称是否被排除
				if !s.CheckPathExclude(newPath) {
					s.Sync.Logger.Infof("文件 %s 被排除，不需要处理", newPath)
					continue fileloop
				}

				if file.IsDir() {
					s.Sync.Logger.Infof("任务 %d 将文件夹 %s 加入路径任务队列", taskIndex, newPath)
					s.addPathToTasks(newPath)
					continue fileloop
				}
				// 本次记录入库
				s.currentNetFileTasks <- newPath
				ok := false
				if _, ok = s.preNetFiles[newPath]; ok {
					continue fileloop
				}
				// 不存在则新建文件记录
				// 新建文件
				relPath, rerr := filepath.Rel(s.Sync.RemotePath, path)
				if rerr != nil {
					s.Sync.Logger.Errorf("获取本地文件 %s 相对路径失败: %v", file.Name(), rerr)
					continue fileloop
				}
				localFilePath := s.Sync.SyncPath.MakeFullLocalPath(relPath, file.Name())
				syncFile, _ := AddSyncFile(s.Sync.SyncPath, newPath, path, file.Name(), path, localFilePath, size, newPath, "", isMeta, isVideo, "", true, false, stat.ModTime().Unix())
				waitSaveFile = append(waitSaveFile, syncFile)
				if len(waitSaveFile) >= 100 {
					s.Sync.Logger.Infof("1 任务 %d 处理目录 %s, 已处理 %d 个文件，开始保存到数据库", taskIndex, path, len(waitSaveFile))
					db.Db.Create(waitSaveFile)
					waitSaveFile = []*SyncFile{}
				}
				s.Sync.Logger.Debugf("文件 %s 加入处理列表，因为不在数据库中", syncFile.FileName)
			}
			if len(waitSaveFile) > 0 {
				s.Sync.Logger.Infof("2 任务 %d 处理目录 %s, 已处理 %d 个文件，开始保存到数据库", taskIndex, path, len(waitSaveFile))
				db.Db.Create(waitSaveFile)
			}
			s.wg.Done()
		}
	}

}

func (s *SyncDriverLocal) CheckPathExclude(pathStr string) bool {
	isExclude := false
	// 检查pathStr中是否某一级被排除
	parts := strings.SplitSeq(pathStr, string(os.PathSeparator))
	for part := range parts {
		if s.Sync.IsExcludeName(part) {
			isExclude = true
			s.Sync.Logger.Infof("路径 %s 中包含被排除的目录 %s，已设置的排除字符数组:%+v 跳过处理2", pathStr, part, s.Sync.SyncPath.GetExcludeNameArr())
			break
		}
	}
	if !isExclude {
		return true
	}
	// 删除这个目录开头的所有记录
	DeleteExcludePathFile(pathStr, s.Sync.SyncPathId, false)
	return false
}

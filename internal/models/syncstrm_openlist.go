package models

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/openlist"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncDriverOpenList struct {
	SyncDriverBase
	client *openlist.Client
}

func NewSyncDriverOpenList(sync *Sync, client *openlist.Client) *SyncDriverOpenList {
	return &SyncDriverOpenList{
		SyncDriverBase: SyncDriverBase{
			Sync: sync,
		},
		client: client,
	}
}

func (s *SyncDriverOpenList) DoSync() error {
	// 先检查源目录是否存在
	_, detailErr := s.client.FileDetail(s.Sync.BaseCid)
	if detailErr != nil {
		return fmt.Errorf("源目录ID %s 不存在", s.Sync.BaseCid)
	}
	s.InitPreNetFiles() // 先初始化上次查询到的所有文件到缓存，供查询使用，提高效率
	if !s.CheckIsRunning() {
		return errors.New("手动停止通知")
	}
	go s.StartCurrentNetFileWork()
	defer func() {
		if s.currentNetFileTasks != nil {
			close(s.currentNetFileTasks)
		}
	}()
	if !s.CheckIsRunning() {
		return errors.New("手动停止通知")
	}
	err := s.GetNetFileFiles()
	if err != nil {
		s.Sync.Logger.Errorf("获取源文件列表失败: %v", err)
		return err
	}
	err = s.CompareLocalFiles()
	if err != nil {
		s.Sync.Logger.Errorf("对比本地文件失败: %v", err)
		return err
	}
	return nil
}

// 查询源文件数
// 从s.Sync.RemotePath开始递归读取文件列表
func (s *SyncDriverOpenList) GetNetFileFiles() error {
	// 检查任务是否已被停止
	s.Sync.UpdateSubStatus(SyncSubStatusProcessNetFileList)
	s.pathTasks = make(chan string, SettingsGlobal.FileDetailThreads)
	ctx, ctxCancel := context.WithCancel(context.Background())
	// 启动buffer to task
	go s.bufferMonitor(ctx)
	// 加入根目录
	s.wg = sync.WaitGroup{}
	s.addPathToTasks(s.Sync.BaseCid)
	// 启动一个协程处理目录
	s.Sync.Logger.Infof("开始处理目录 %s, 开启 %d 个任务", s.Sync.RemotePath, SettingsGlobal.FileDetailThreads)
	for i := 0; i < SettingsGlobal.FileDetailThreads; i++ {
		// 在限速器控制下执行StartPathWork
		go s.startPathWorkWithLimiter(i)
	}
	go func() {
		<-s.Sync.ctx.Done()
		for range s.pathTasks {
			s.wg.Done()
		}
	}()
	s.wg.Wait()
	close(s.pathTasks)
	ctxCancel()
	// 处理完以后启开始处理数据库数据
	// 开始处理本次获取到的数据
	s.Sync.UpdateTotal()
	s.ProcessNetFiles()
	return nil
}

func (s *SyncDriverOpenList) startPathWorkWithLimiter(taskIndex int) {
	// 查询目录下的文件夹和文件列表
	// 文件直接处理，文件夹继续加入队列
	waitSaveFile := []*SyncFile{}
	for {
		select {
		case <-s.Sync.ctx.Done():
			return
		case path, ok := <-s.pathTasks:
			if !ok {
				s.Sync.Logger.Infof("任务 %d 处理目录完成", taskIndex)
				return
			}
			s.Sync.Logger.Infof("任务 %d 开始处理目录 %s", taskIndex, path)
			// 检查文件夹是否被排除
			if !s.CheckPathExclude(path) {
				s.wg.Done()
				s.Sync.Logger.Infof("文件夹 %s 被排除，不需要处理", path)
				continue
			}
			page := 1
			pageSize := 50 // 每次取1000条
		pageloop:
			for {
				s.Sync.Logger.Infof("任务 %d 开始处理目录 %s, 第 %d 页, 每页 %d 条", taskIndex, path, page, pageSize)
				resp, err := s.client.FileList(s.Sync.ctx, path, page, pageSize)
				if err != nil {
					if strings.Contains(err.Error(), "context canceled") {
						s.wg.Done()
						s.Sync.Logger.Infof("任务队列 %d 上下文已取消，任务 %s", taskIndex, path)
						return
					}
					s.Sync.Logger.Errorf("获取openlist文件列表失败: %v", err)
					break pageloop
				}
				// helpers.AppLogger.Infof("openlist文件列表: %+v", resp)
				if resp.Total == 0 {
					s.Sync.Logger.Infof("openlist文件列表为空: %v", resp)
					break pageloop
				}
				// relPath, _ := filepath.Rel(s.Sync.RemotePath, path)
			fileloop:
				for _, file := range resp.Content {
					s.Sync.Total++
					// 检查是否视频或元数据
					isVideo := s.Sync.IsValidVideoExt(file.Name)
					isMeta := s.Sync.IsValidMetaExt(file.Name)
					if isMeta && s.Sync.SyncPath.GetDownloadMeta() == 0 {
						// 如果是元数据文件且设置为不下载，则跳过检查（代表着不上传）
						s.Sync.Logger.Infof("网盘元数据文件 %s 由于关闭了元数据下载所以不需要处理", file.Name)
						continue fileloop
					}
					if !file.IsDir && !isVideo && !isMeta {
						s.Sync.Logger.Warnf("非视频或元数据文件不需要处理: %s", file.Name)
						continue fileloop
					}
					// 检查视频大小是否合规
					if isVideo && file.Size < s.Sync.SyncPath.GetMinVideoSize()*1024*1024 {
						s.Sync.Logger.Warnf("视频文件 %s 大小%d 没有达到最小大小: %d", file.Name, file.Size, s.Sync.SyncPath.GetMinVideoSize()*1024*1024)
						continue fileloop
					}
					remoteFilePath := filepath.Join(path, file.Name)
					if file.IsDir {
						s.Sync.Logger.Infof("准备将文件夹 %s 加入路径队列", remoteFilePath)
						s.addPathToTasks(remoteFilePath)
						s.Sync.Logger.Infof("已将将文件夹 %s 加入路径队列", remoteFilePath)
						continue fileloop
					}
					// 查看名称是否被排除
					if !s.CheckPathExclude(remoteFilePath) {
						s.Sync.Logger.Infof("文件 %s 被排除，不需要处理", file.Name)
						continue fileloop
					}
					s.Sync.Logger.Infof("文件 %s 不是文件夹，加入处理列表", file.Name)
					s.currentNetFileTasks <- remoteFilePath
					ok := false
					if _, ok = s.preNetFiles[remoteFilePath]; ok {
						continue fileloop
					}
					// 新建文件
					localFilePath := s.Sync.SyncPath.MakeFullLocalPath(path, file.Name)
					// 将ISO 8601格式的日期字符串转换为时间戳
					t, err := time.Parse(time.RFC3339, file.Modified)
					var mtime int64
					if err != nil {
						s.Sync.Logger.Errorf("解析时间格式失败: %v, 时间字符串: %s", err, file.Modified)
						mtime = 0 // 错误时使用默认值
					} else {
						mtime = t.Unix() // 转换为Unix时间戳（秒）
					}
					syncFile, _ := AddSyncFile(s.Sync.SyncPath, remoteFilePath, path, file.Name, path, localFilePath, file.Size, remoteFilePath, "", isMeta, isVideo, file.Sign, true, false, mtime)
					waitSaveFile = append(waitSaveFile, syncFile)
					if len(waitSaveFile) >= 10 {
						s.Sync.Logger.Infof("任务 %d 处理目录 %s, 已处理 %d 个文件，开始保存到数据库", taskIndex, path, len(waitSaveFile))
						// 批量入库
						tx := db.Db.Create(waitSaveFile)
						err := tx.Error
						rowsAffected := tx.RowsAffected
						if err != nil {
							s.Sync.Logger.Errorf("批量入库 %d 个文件失败, 影响 %d 行, %v", len(waitSaveFile), rowsAffected, err)
						} else {
							s.Sync.Logger.Infof("批量入库 %d 个文件, 影响 %d 行", len(waitSaveFile), rowsAffected)
						}
						waitSaveFile = []*SyncFile{}
					}
					s.Sync.Logger.Debugf("文件 %s 加入处理列表，因为不在数据库中", syncFile.FileName)
				}
				if resp.Total <= int64(page*pageSize) {
					s.Sync.Logger.Infof("任务 %d 第 %d 页文件列表查询完成，共 %d 条记录", taskIndex, page, resp.Total)
					break pageloop
				}
				page++
			}
			if len(waitSaveFile) > 0 {
				s.Sync.Logger.Infof("2 任务 %d 处理目录 %s, 已处理 %d 个文件，开始保存到数据库", taskIndex, path, len(waitSaveFile))
				db.Db.Create(waitSaveFile)
				waitSaveFile = []*SyncFile{}
			}
			s.wg.Done()
		}
	}
}

func (s *SyncDriverOpenList) CheckPathExclude(pathStr string) bool {
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

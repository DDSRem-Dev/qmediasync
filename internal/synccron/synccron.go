package synccron

import (
	"Q115-STRM/internal/db"
	embyclientrestgo "Q115-STRM/internal/embyclient-rest-go"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/notificationmanager"
	"Q115-STRM/internal/scrape"
	"Q115-STRM/internal/v115open"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

var GlobalCron *cron.Cron
var embySyncRunning int32

// IsEmbySyncRunning 检查是否有Emby同步任务正在运行
func IsEmbySyncRunning() bool {
	return atomic.LoadInt32(&embySyncRunning) == 1
}

type playbackInfoResponse struct {
	MediaSources []struct {
		Path             string `json:"Path"`
		MediaAttachments []struct {
			Path string `json:"Path"`
		} `json:"MediaAttachments"`
	} `json:"MediaSources"`
}

type embySyncTask struct {
	LibraryId   string
	LibraryName string
	Item        embyclientrestgo.BaseItemDtoV2
}

func StartSyncCron() {
	// 查询所有同步目录
	syncPaths, _ := models.GetSyncPathList(1, 10000000, true)
	if len(syncPaths) == 0 {
		// helpers.AppLogger.Info("没有找到同步目录")
		return
	}
	for _, syncPath := range syncPaths {
		// 将同步目录ID添加到处理队列，而不是直接执行
		if err := AddSyncTask(syncPath.ID, SyncTaskTypeStrm); err != nil {
			helpers.AppLogger.Errorf("将同步任务添加到队列失败: %s", err.Error())
			continue
		} else {
			helpers.AppLogger.Infof("创建同步任务成功并已添加到执行队列，同步目录ID: %d，同步目录:%s", syncPath.ID, syncPath.RemotePath)
		}
	}
}

// 开始刮削整理任务
func StartScrapeCron() {
	// 查询所有刮削目录
	scrapePaths := models.GetScrapePathes()
	if len(scrapePaths) == 0 {
		helpers.AppLogger.Info("没有找到刮削目录")
		return
	}
	for _, scrapePath := range scrapePaths {
		if !scrapePath.EnableCron {
			continue
		}
		// 将刮削目录ID添加到处理队列，而不是直接执行
		if err := AddSyncTask(scrapePath.ID, SyncTaskTypeScrape); err != nil {
			helpers.AppLogger.Errorf("将刮削任务添加到队列失败: %s", err.Error())
			continue
		} else {
			helpers.AppLogger.Infof("创建刮削任务成功并已添加到执行队列，刮削目录ID: %d，刮削目录:%s，目标目录：%s", scrapePath.ID, scrapePath.SourcePath, scrapePath.DestPath)
		}
	}
}

func Refresh115AccessToken() {
	// 刷新115的访问凭证
	// 取所有115类型的账号
	accounts, _ := models.GetAllAccount()
	now := time.Now().Unix()
	for _, account := range accounts {
		if account.SourceType == models.SourceType115 && account.RefreshToken != "" {
			// helpers.AppLogger.Infof("当前时间: %d, 过期时间：%d", now, account.TokenExpiriesTime-3600)
			if account.TokenExpiriesTime-3600 > now {
				// helpers.AppLogger.Infof("115账号token未过期，账号ID: %d, 115用户名：%s， 过期时间：%s", account.ID, account.Username, time.Unix(account.TokenExpiriesTime-3600, 0).Format("2006-01-02 15:04:05"))
				continue
			}
			helpers.AppLogger.Infof("开始刷新115账号token，账号ID: %d, 115用户名：%s", account.ID, account.Username)
			// 刷新115的访问凭证
			client := account.Get115Client(true)
			tokenData, err := client.RefreshToken(account.RefreshToken)
			if err != nil {
				helpers.AppLogger.Errorf("刷新115访问凭证失败: %s", err.Error())
				// 清空token
				account.ClearToken(err.Error())
				ctx := context.Background()
				notif := &models.Notification{
					Type:      models.SystemAlert,
					Title:     "🔐 115开放平台访问凭证已失效",
					Content:   fmt.Sprintf("账号ID：%d\n用户名：%s\n请重新授权\n⏰ 时间: %s", int(account.ID), account.Username, time.Now().Format("2006-01-02 15:04:05")),
					Timestamp: time.Now(),
					Priority:  models.HighPriority,
				}
				if notificationmanager.GlobalEnhancedNotificationManager != nil {
					if err := notificationmanager.GlobalEnhancedNotificationManager.SendNotification(ctx, notif); err != nil {
						helpers.AppLogger.Errorf("发送访问凭证失效通知失败: %v", err)
					}
				}
				continue
			}
			// 更新账号的token
			if suc := account.UpdateToken(tokenData.AccessToken, tokenData.RefreshToken, tokenData.ExpiresIn); !suc {
				helpers.AppLogger.Errorf("更新115账号token失败")
				continue
			}
			// 更新其他客户端的token
			v115open.UpdateToken(account.ID, tokenData.AccessToken, tokenData.RefreshToken)
			// 刷新成功，更新账号的token
			helpers.AppLogger.Infof("刷新115账号token成功，账号ID: %d", account.ID)
		}
	}
}

var EmbyMediaInfoStart bool = false

func StartParseEmbyMediaInfo() {
	if EmbyMediaInfoStart {
		helpers.AppLogger.Info("Emby库同步任务已在运行")
		return
	}
	if models.GlobalEmbyConfig.EmbyUrl == "" || models.GlobalEmbyConfig.EmbyApiKey == "" {
		helpers.AppLogger.Info("Emby Url或ApiKey为空，无法同步emby库来提取视频信息")
		return
	}
	EmbyMediaInfoStart = true
	defer func() {
		EmbyMediaInfoStart = false
	}()
	// 放入协程运行
	go func() {
		tasks := embyclientrestgo.ProcessLibraries(models.GlobalEmbyConfig.EmbyUrl, models.GlobalEmbyConfig.EmbyApiKey, []string{})
		helpers.AppLogger.Infof("Emby库收集媒体信息已完成，共发现 %d 个影视剧需要提取媒体信息", len(tasks))
		for _, itemTask := range tasks {
			task := models.AddDownloadTaskFromEmbyMedia(itemTask["url"], itemTask["item_id"], itemTask["item_name"])
			if task == nil {
				helpers.AppLogger.Errorf("添加Emby媒体信息提取任务失败: Emby ItemID: %s, 名称: %s", itemTask["item_id"], itemTask["item_name"])
				continue
			}
			helpers.AppLogger.Infof("Emby媒体信息提取已加入操作队列: Emby ItemID: %s, 名称: %s", itemTask["item_id"], itemTask["item_name"])
		}
	}()
}

// PerformEmbySync 全量同步Emby媒体，使用PlaybackInfo提取pickcode并关联同步文件
func PerformEmbySync() (int, error) {
	// 检查是否已有任务在运行，避免并发执行
	if IsEmbySyncRunning() {
		helpers.AppLogger.Warnf("Emby同步任务已在运行，跳过本次定时执行")
		return 0, nil
	}
	config, cerr := models.GetEmbyConfig()
	if config.EmbyUrl == "" || config.EmbyApiKey == "" {
		return 0, errors.New("Emby Url或ApiKey为空")
	}
	if cerr != nil || config.SyncEnabled != 1 {
		return 0, errors.New("Emby同步未启用")
	}
	if !atomic.CompareAndSwapInt32(&embySyncRunning, 0, 1) {
		return 0, errors.New("Emby同步任务已在运行")
	}
	defer atomic.StoreInt32(&embySyncRunning, 0)

	if config == nil {
		var err error
		config, err = models.GetEmbyConfig()
		if err != nil {
			return 0, err
		}
	}
	if config.SyncEnabled != 1 {
		return 0, errors.New("未启用Emby同步")
	}
	if config.EmbyUrl == "" || config.EmbyApiKey == "" {
		return 0, errors.New("Emby配置不完整")
	}

	client := embyclientrestgo.NewClient(config.EmbyUrl, config.EmbyApiKey)
	users, err := client.GetUsersWithAllLibrariesAccess()
	if err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, errors.New("没有找到可访问全部媒体库的Emby用户")
	}

	libs, err := client.GetAllMediaLibraries()
	if err != nil {
		return 0, err
	}
	if len(libs) == 0 {
		return 0, errors.New("未获取到任何Emby媒体库")
	}
	if err := models.UpsertEmbyLibraries(libs); err != nil {
		helpers.AppLogger.Warnf("保存媒体库信息失败: %v", err)
	}

	// 准备并发池
	workerCount := 50
	jobs := make(chan embySyncTask, workerCount*2)
	var wg sync.WaitGroup
	var mu sync.Mutex
	validItemIds := make([]string, 0, 256)
	var processed int64
	// clientHttp := &http.Client{Timeout: 30 * time.Second}

	worker := func() {
		defer wg.Done()
		for task := range jobs {
			pickCode, mediaPath, err := extractPickCode(task.Item.MediaSources)
			// pickCode, mediaPath := "", ""
			if err != nil {
				// helpers.AppLogger.Warnf("从MediaSource中查询PickCode失败 item=%s name=%s path=%s err=%v", task.Item.Id, task.Item.Name, mediaPath, err)
				// 没有pickcode不入库
				continue
			}
			itemData, _ := json.Marshal(task.Item)
			pathStr := mediaPath
			if pathStr == "" {
				pathStr = task.Item.Path
			}
			mediaItem := &models.EmbyMediaItem{
				ItemId:            task.Item.Id,
				ServerId:          "",
				Name:              task.Item.Name,
				Type:              task.Item.Type,
				ParentId:          task.Item.ParentId,
				SeriesId:          task.Item.SeriesId,
				SeasonId:          task.Item.SeasonId,
				SeasonName:        task.Item.SeasonName,
				SeriesName:        task.Item.SeriesName,
				LibraryId:         task.LibraryId,
				Path:              pathStr,
				PickCode:          pickCode,
				MediaSourcePath:   mediaPath,
				IndexNumber:       task.Item.IndexNumber,
				ParentIndexNumber: task.Item.ParentIndexNumber,
				ProductionYear:    task.Item.ProductionYear,
				PremiereDate:      task.Item.PremiereDate,
				DateCreated:       task.Item.DateCreated,
				DateModified:      task.Item.DateModified,
				IsFolder:          task.Item.IsFolder,
				EmbyData:          string(itemData),
			}
			if err := models.CreateOrUpdateEmbyMediaItem(mediaItem); err != nil {
				helpers.AppLogger.Errorf("保存Emby媒体项失败 id=%s name=%s err=%v", task.Item.Id, task.Item.Name, err)
				continue
			}
			mu.Lock()
			validItemIds = append(validItemIds, task.Item.Id)
			mu.Unlock()
			atomic.AddInt64(&processed, 1)
			if pickCode != "" {
				if sf := models.GetFileByPickCode(pickCode); sf != nil {
					if err := models.CreateEmbyMediaSyncFile(task.Item.Id, sf.ID, pickCode); err != nil {
						helpers.AppLogger.Warnf("关联SyncFile失败 item=%s pickcode=%s err=%v", task.Item.Id, pickCode, err)
					}
					models.CreateOrUpdateEmbyLibrarySyncPath(task.LibraryId, sf.SyncPathId, task.LibraryName)
				}
			}
		}
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}

	for _, lib := range libs {
		items, gerr := client.GetMediaItemsByLibraryID(lib.ID)
		if gerr != nil {
			helpers.AppLogger.Warnf("获取媒体库%s失败: %v", lib.Name, gerr)
			continue
		}
		for _, item := range items {
			jobs <- embySyncTask{LibraryId: lib.ID, LibraryName: lib.Name, Item: item}
		}
	}
	close(jobs)
	wg.Wait()

	if processed > 0 {
		if err := models.CleanupOrphanedEmbyMediaItems(validItemIds); err != nil {
			helpers.AppLogger.Warnf("清理过期Emby媒体项失败: %v", err)
		}
	}
	if err := models.UpdateLastSyncTime(); err != nil {
		helpers.AppLogger.Warnf("更新Emby最后同步时间失败: %v", err)
	}
	helpers.AppLogger.Infof("Emby同步完成，处理 %d 个项目", processed)
	return int(processed), nil
}

func extractPickCode(ms []embyclientrestgo.MediaSource) (string, string, error) {
	code := ""
	pathStr := ""
	for _, src := range ms {
		code = extractPickCodeFromPath(src.Path)
		pathStr = src.Path
		if code != "" {
			return code, pathStr, nil
		}

	}
	return code, pathStr, errors.New("未从Item.MediaSource.Path中解析到pickcode")
}

func extractPickCodeFromPath(path string) string {
	if path == "" {
		return ""
	}
	if u, err := url.Parse(path); err == nil {
		if code := u.Query().Get("pickcode"); code != "" {
			return code
		}
		if code := u.Query().Get("pick_code"); code != "" {
			return code
		}
	}
	if m := regexp.MustCompile(`(?i)pickcode[=/]([A-Za-z0-9]+)`).FindStringSubmatch(path); len(m) > 1 {
		return m[1]
	}
	matches := regexp.MustCompile(`([A-Za-z0-9]{12,32})`).FindAllString(path, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}

func StartClearDownloadUploadTasks() {
	helpers.AppLogger.Info("开始清除3天前的上传任务")
	models.ClearExpireUploadTasks()
	helpers.AppLogger.Info("开始清除3天前的下载任务")
	models.ClearExpireDownloadTasks()
}

var RollBackCronStart bool = false

func StartScrapeRollbackCron() {
	if RollBackCronStart {
		helpers.AppLogger.Info("刮削回滚任务已在运行")
		return
	}
	RollBackCronStart = true
	defer func() {
		RollBackCronStart = false
	}()
	go func() {
		limit := 10
		offset := 0
		for {
			// 从数据库中获取所有状态为回滚中的记录
			var mediaFiles []*models.ScrapeMediaFile
			err := db.Db.Where("status = ?", models.ScrapeMediaStatusRollbacking).Limit(limit).Offset(offset).Find(&mediaFiles).Error
			if err != nil {
				helpers.AppLogger.Errorf("获取刮削失败的媒体文件失败: %v", err)
				return
			}
			if len(mediaFiles) == 0 {
				// helpers.AppLogger.Info("没有刮削失败的媒体文件")
				return
			}
			helpers.AppLogger.Infof("获取到 %d 个刮削失败的媒体文件", len(mediaFiles))
			// 遍历所有媒体文件，进行回滚操作
			for _, mediaFile := range mediaFiles {
				scrapePath := models.GetScrapePathByID(mediaFile.ScrapePathId)
				scrape := scrape.NewScrape(scrapePath)
				err := scrape.Rollback(mediaFile)
				if err != nil {
					helpers.AppLogger.Errorf("回滚媒体文件 %s 失败: %v", mediaFile.Name, err)
				} else {
					helpers.AppLogger.Infof("成功回滚媒体文件 %s", mediaFile.Name)
				}
			}
			// 每次处理完休息10秒
			time.Sleep(10 * time.Second)
		}
	}()

}

// 初始化定时任务
func InitCron() {
	if GlobalCron != nil {
		GlobalCron.Stop()
	}
	GlobalCron = cron.New()
	GlobalCron.AddFunc("0 1 * * *", func() {
		StartClearDownloadUploadTasks()
	})
	GlobalCron.AddFunc(models.SettingsGlobal.Cron, func() {
		// helpers.AppLogger.Info("启动115网盘同步任务")
		StartSyncCron()
	})
	GlobalCron.AddFunc("0 0 * * *", func() {
		// 每天0点清理过期的同步记录
		// helpers.AppLogger.Info("清理过期的同步记录")
		models.ClearExpiredSyncRecords(1) // 保留3天内的记录
	})
	GlobalCron.AddFunc("*/5 * * * *", func() {
		// helpers.AppLogger.Info("定时刷新115的访问凭证")
		Refresh115AccessToken()
	})
	GlobalCron.AddFunc("*/13 * * * *", func() {
		// helpers.AppLogger.Info("启动刮削任务")
		StartScrapeCron()
	})
	if config, err := models.GetEmbyConfig(); err == nil {
		if config.EmbyApiKey != "" && config.EmbyUrl != "" {
			GlobalCron.AddFunc(config.SyncCron, func() {
				if _, err := PerformEmbySync(); err != nil {
					helpers.AppLogger.Errorf("Emby同步失败: %v", err)
				}
			})
		}
	}
	GlobalCron.AddFunc("*/2 * * * *", func() {
		// helpers.AppLogger.Info("启动刮削回滚任务")
		StartScrapeRollbackCron()
	})
	// 添加备份定时任务和超时检查
	StartBackupCron()
	GlobalCron.AddFunc("*/1 * * * *", func() {
		// 每分钟检查一次维护模式和备份任务是否超时
		CheckMaintenanceModeTimeout()
	})
	GlobalCron.Start()
}

// StartBackupCron 启动备份定时任务
func StartBackupCron() {
	// 获取备份配置
	backupConfig := &models.BackupConfig{}
	if err := db.Db.First(backupConfig).Error; err != nil {
		// 没有配置，不启动定时备份
		return
	}

	if backupConfig.BackupEnabled != 1 || backupConfig.BackupCron == "" {
		// 未启用自动备份
		return
	}

	GlobalCron.AddFunc(backupConfig.BackupCron, func() {
		// 检查是否已有运行中的备份任务
		runningTask := &models.BackupTask{}
		if err := db.Db.Where("status = ?", "running").First(runningTask).Error; err == nil {
			helpers.AppLogger.Warnf("已有备份任务正在运行中，跳过本次定时备份")
			return
		}

		// 创建备份任务
		task := &models.BackupTask{
			Status:        "running",
			Progress:      0,
			BackupType:    "auto",
			CreatedReason: "定时自动备份",
			CurrentStep:   "准备备份...",
			StartTime:     time.Now().Unix(),
		}

		if err := db.Db.Create(task).Error; err != nil {
			helpers.AppLogger.Errorf("创建定时备份任务失败: %v", err)
			return
		}

		// 异步执行备份
		go performBackup(task, backupConfig)
	})
}

// CheckMaintenanceModeTimeout 检查维护模式和备份任务是否超时
func CheckMaintenanceModeTimeout() {
	// 检查维护模式是否超时（1小时）
	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err == nil {
		if config.MaintenanceMode == 1 {
			elapsed := time.Now().Unix() - config.MaintenanceModeTime
			if elapsed > 3600 { // 1小时 = 3600秒
				helpers.AppLogger.Warnf("维护模式已超时(%d秒)，自动退出维护模式", elapsed)
				db.Db.Model(config).Updates(map[string]interface{}{
					"maintenance_mode":      0,
					"maintenance_mode_time": 0,
				})
			}
		}
	}

	// 检查备份任务是否超时（1小时）
	task := &models.BackupTask{}
	if err := db.Db.Where("status = ?", "running").First(task).Error; err == nil {
		elapsed := time.Now().Unix() - task.StartTime
		if elapsed > 3600 { // 1小时 = 3600秒
			helpers.AppLogger.Warnf("备份任务已超时(%d秒)，标记为超时失败", elapsed)
			db.Db.Model(task).Updates(map[string]interface{}{
				"status":         "timeout",
				"end_time":       time.Now().Unix(),
				"failure_reason": "备份任务执行超过1小时，自动超时",
			})
			// 清理临时文件
			if task.FilePath != "" {
				backupDir := filepath.Join(helpers.RootDir, "config", "backups")
				os.Remove(filepath.Join(backupDir, task.FilePath))
				os.Remove(filepath.Join(backupDir, task.FilePath+".gz"))
			}
		}
	}
}

// performBackup 执行备份操作
func performBackup(task *models.BackupTask, config *models.BackupConfig) {
	// 这个函数由 controllers/database.go 中定义
	// 这里仅作为接口，具体实现在控制器层
}

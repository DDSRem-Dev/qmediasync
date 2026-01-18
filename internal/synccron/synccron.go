package synccron

import (
	"Q115-STRM/internal/db"
	embyclientrestgo "Q115-STRM/internal/embyclient-rest-go"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/scrape"
	"Q115-STRM/internal/v115open"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var GlobalCron *cron.Cron

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
				helpers.GlobalNotificationManager.SendSystemNotification("115开放平台访问凭证已失效，请重新授权", fmt.Sprintf("账号ID：%d, 115用户名：%s", int(account.ID), account.Username))
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
	if models.SettingsGlobal.EmbyUrl == "" || models.SettingsGlobal.EmbyApiKey == "" {
		helpers.AppLogger.Info("Emby Url或ApiKey为空，无法同步emby库来提取视频信息")
		return
	}
	EmbyMediaInfoStart = true
	defer func() {
		EmbyMediaInfoStart = false
	}()
	// 放入协程运行
	go func() {
		tasks := embyclientrestgo.ProcessLibraries(models.SettingsGlobal.EmbyUrl, models.SettingsGlobal.EmbyApiKey, []string{})
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
	GlobalCron.AddFunc("*/2 * * * *", func() {
		// helpers.AppLogger.Info("启动刮削回滚任务")
		StartScrapeRollbackCron()
	})
	GlobalCron.Start()
}

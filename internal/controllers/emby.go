package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/notificationmanager"
	"Q115-STRM/internal/synccron"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type EmbyEvent struct {
	Title    string `json:"Title"`
	Date     string `json:"Date"`
	Event    string `json:"Event"`
	Severity string `json:"Severity"`
	Server   struct {
		Name    string `json:"Name"`
		ID      string `json:"Id"`
		Version string `json:"Version"`
	} `json:"Server"`
	Item struct {
		Name              string            `json:"Name"`
		ID                string            `json:"Id"`
		Type              string            `json:"Type"`
		IsFolder          bool              `json:"IsFolder"`
		FileName          string            `json:"FileName"`
		Path              string            `json:"Path"`
		Overview          string            `json:"Overview"`
		SeriesName        string            `json:"SeriesName"`
		SeasonName        string            `json:"SeasonName"`
		SeriesId          string            `json:"SeriesId"`
		SeasonId          string            `json:"SeasonId"`
		IndexNumber       int               `json:"IndexNumber"`
		ParentIndexNumber int               `json:"ParentIndexNumber"`
		ProductionYear    int               `json:"ProductionYear"`
		Genres            []string          `json:"Genres"`
		ImageTags         map[string]string `json:"ImageTags"`
	} `json:"Item"`
}

var refreshLibraryLock bool = false
var refreshLibraryLockMu = sync.Mutex{}

func Webhook(ctx *gin.Context) {
	// 将请求的body内容完整打印到日志
	var body []byte
	if ctx.Request.Body != nil {
		body, _ = io.ReadAll(ctx.Request.Body)
		helpers.AppLogger.Infof("emby webhook body: %s", string(body))
	}
	if body == nil || models.GlobalEmbyConfig.EmbyUrl == "" || models.GlobalEmbyConfig.EmbyApiKey == "" {
		ctx.JSON(http.StatusOK, gin.H{
			"message": "webhook",
		})
		return
	}
	// 处理 body内容，解析成json
	var event EmbyEvent
	// 如果解析失败，记录错误日志并返回
	err := json.Unmarshal(body, &event)
	if err != nil {
		helpers.AppLogger.Errorf("emby webhook bind json error: %v", err)
		ctx.JSON(http.StatusOK, gin.H{
			"message": "webhook",
		})
		return
	}
	if event.Event == "library.new" {
		// 新入库通知
		// 触发通知
		go func() {
			ctx := context.Background()
			// 拼接Content内容
			content := ""
			imagePath := ""
			id := event.Item.ID
			switch event.Item.Type {
			case "Movie":
				content = fmt.Sprintf("电影名称：%s\n简介：%s\n流派：%s\n⏰ 入库时间: %s", event.Item.Name, event.Item.Overview, strings.Join(event.Item.Genres, ", "), time.Now().Format("2006-01-02 15:04:05"))
			case "Episode":
				content = fmt.Sprintf("电视剧名称：%s\n简介：%s\n流派：%s\n入库季集：S%dE%d\n集主题：%s\n⏰ 入库时间: %s", event.Item.SeriesName, event.Item.Overview, strings.Join(event.Item.Genres, ", "), event.Item.ParentIndexNumber, event.Item.IndexNumber, event.Item.Name, time.Now().Format("2006-01-02 15:04:05"))
				id = event.Item.SeriesId
			default:
				// 只有电影和集会发送通知
				return
			}
			if event.Item.ImageTags != nil {
				if tag, ok := event.Item.ImageTags["Primary"]; ok {
					imageUrl := fmt.Sprintf("%s/emby/Items/%s/Images/Primary?tag=%s&api_key=%s", models.GlobalEmbyConfig.EmbyUrl, id, tag, models.GlobalEmbyConfig.EmbyApiKey)
					// 将图片下载/tmp目录，作为通知图片
					posterPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.jpg", event.Item.ID))
					derr := helpers.DownloadFile(imageUrl, posterPath, "Q115-STRM")
					if derr != nil {
						helpers.AppLogger.Errorf("下载Emby海报失败: %v", derr)
					} else {
						imagePath = posterPath
					}
				}
			}
			notif := &models.Notification{
				Type:      models.MediaAdded,
				Title:     "📚 Emby媒体入库通知",
				Content:   content,
				Timestamp: time.Now(),
				Priority:  models.NormalPriority,
			}
			if imagePath != "" {
				notif.Image = imagePath
			}
			if notificationmanager.GlobalEnhancedNotificationManager != nil {
				if err := notificationmanager.GlobalEnhancedNotificationManager.SendNotification(ctx, notif); err != nil {
					helpers.AppLogger.Errorf("发送媒体入库通知失败: %v", err)
				}
			}
			// 删除临时图片文件
			if imagePath != "" {
				os.Remove(imagePath)
			}
		}()
		// 触发媒体信息提取
		if models.GlobalEmbyConfig != nil && models.GlobalEmbyConfig.EnableExtractMediaInfo == 1 {
			go func() {
				// 获取Emby地址和Emby Api Key
				url := fmt.Sprintf("%s/emby/Items/%s/PlaybackInfo?api_key=%s", models.GlobalEmbyConfig.EmbyUrl, event.Item.ID, models.GlobalEmbyConfig.EmbyApiKey)
				models.AddDownloadTaskFromEmbyMedia(url, event.Item.ID, event.Item.Name)
				if err != nil {
					helpers.AppLogger.Errorf("触发Emby信息提取失败 错误: %v", err)
				}
			}()
		} else {
			helpers.AppLogger.Infof("Emby媒体信息提取功能未启用，跳过媒体信息提取")
		}
		// 1分钟后同步一次Emby媒体库
		go func() {
			refreshLibraryLockMu.Lock()
			if refreshLibraryLock {
				refreshLibraryLockMu.Unlock()
				helpers.AppLogger.Infof("Emby媒体库刷新锁定中，跳过本次刷新")
				return
			}
			refreshLibraryLock = true
			refreshLibraryLockMu.Unlock()
			defer func() {
				refreshLibraryLockMu.Lock()
				refreshLibraryLock = false
				refreshLibraryLockMu.Unlock()
			}()
			time.Sleep(1 * time.Minute)
			synccron.PerformEmbySync()
		}()
	}
	if event.Event == "library.deleted" {
		// 删除媒体通知
		// 仅记录关键信息，不做其他处理
		helpers.AppLogger.Infof("Emby媒体已删除 %+v", event.Item)
		// 触发通知
		go func() {
			ctx := context.Background()
			content := ""
			switch event.Item.Type {
			case "Movie":
				content = fmt.Sprintf("电影名称：%s\n⏰ 删除时间: %s", event.Item.Name, time.Now().Format("2006-01-02 15:04:05"))
			case "Episode":
				content = fmt.Sprintf("电视剧名称：%s\n删除季集：S%dE%d\n⏰ 删除时间: %s", event.Item.SeriesName, event.Item.ParentIndexNumber, event.Item.IndexNumber, time.Now().Format("2006-01-02 15:04:05"))
			default:
				// 只有电影和集会发送通知
				return
			}
			notif := &models.Notification{
				Type:      models.MediaRemoved,
				Title:     "🗑️ Emby媒体删除通知",
				Content:   content,
				Timestamp: time.Now(),
				Priority:  models.NormalPriority,
			}
			if notificationmanager.GlobalEnhancedNotificationManager != nil {
				if err := notificationmanager.GlobalEnhancedNotificationManager.SendNotification(ctx, notif); err != nil {
					helpers.AppLogger.Errorf("发送媒体删除通知失败: %v", err)
				}
			}
		}()
		// 触发联动删除
		if models.GlobalEmbyConfig != nil && models.GlobalEmbyConfig.EnableExtractMediaInfo == 1 {
			switch event.Item.Type {
			case "Movie":
				// 电影：在网盘中将视频文件的父目录一起删除
				// 查找Item.Id对应的SyncFileId
				models.DeleteNetdiskMovieByEmbyItemId(event.Item.ID)
			case "Episode":
				// 集：删除视频文件+元数据（nfo、封面)
				// 查找Item.Id对应的SyncFileId
				models.DeleteNetdiskEpisodeByEmbyItemId(event.Item.ID)
			case "Season":
				// 季：先检查视频文件的父目录，如果父目录是季文件夹则删除该文件夹；如果父目录是有tvshow的目录则仅删除季下所有集对应的视频文件+元数据（nfo、封面)
				// 查找EmbyMediaItem.SeasonId = item.Id的记录，取其中一条的ItemId对应的SyncFileId的SyncFile.Path作为季目录来处理
				models.DeleteNetdiskSeasonByItemId(event.Item.ID)
			case "Series":
				// 剧：在网盘中将tvshow.nfo的父目录删除
				// 查找EmbyMediaItem.SeriesId = item.Id的记录，取其中一条的ItemId对应的SyncFileId的SyncFile.Path作为季目录来处理
				models.DeleteNetdiskTvshowByItemId(event.Item.ID)
			default:
			}
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "webhook",
	})
}

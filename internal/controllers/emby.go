package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
		Name     string `json:"Name"`
		ID       string `json:"Id"`
		Type     string `json:"Type"`
		IsFolder bool   `json:"IsFolder"`
		FileName string `json:"FileName"`
		Path     string `json:"Path"`
	} `json:"Item"`
}

func Webhook(ctx *gin.Context) {
	// 将请求的body内容完整打印到日志
	var body []byte
	if ctx.Request.Body != nil {
		body, _ = io.ReadAll(ctx.Request.Body)
		helpers.AppLogger.Infof("emby webhook body: %s", string(body))
	}
	if body == nil || models.SettingsGlobal.EmbyUrl == "" || models.SettingsGlobal.EmbyApiKey == "" {
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
		// 触发媒体信息提取
		go func() {
			// 获取Emby地址和Emby Api Key
			url := fmt.Sprintf("%s/emby/Items/%s/PlaybackInfo?api_key=%s", models.SettingsGlobal.EmbyUrl, event.Item.ID, models.SettingsGlobal.EmbyApiKey)
			models.AddDownloadTaskFromEmbyMedia(url, event.Item.ID, event.Item.Name)
			if err != nil {
				helpers.AppLogger.Errorf("触发Emby信息提取失败 错误: %v", err)
			}
		}()
		// 触发通知
		go func() {
			helpers.GlobalNotificationManager.SendSystemNotification("Emby媒体入库通知", fmt.Sprintf("新入库媒体名称：%s", event.Item.Name))
		}()
	}
	if event.Event == "library.deleted" {
		// 删除媒体通知
		// 仅记录关键信息，不做其他处理
		helpers.AppLogger.Infof("Emby媒体已删除，当前版本仅通知不执行删除 %+v", event.Item)
		// 触发通知
		go func() {
			helpers.GlobalNotificationManager.SendSystemNotification("Emby媒体删除通知", fmt.Sprintf("已删除媒体名称：%s", event.Item.Name))
		}()
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "webhook",
	})
}

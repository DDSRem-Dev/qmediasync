package controllers

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetEmbyConfig 获取Emby配置
func GetEmbyConfig(c *gin.Context) {
	config, err := models.GetEmbyConfig()
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusOK, APIResponse[any]{
			Code:    Success,
			Message: "获取Emby配置成功",
			Data:    gin.H{"exists": false},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取Emby配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "获取Emby配置成功",
		Data:    gin.H{"exists": true, "config": config},
	})
}

type updateEmbyConfigRequest struct {
	EmbyUrl                 string `json:"emby_url"`
	EmbyApiKey              string `json:"emby_api_key"`
	EnableDeleteNetdisk     int    `json:"enable_delete_netdisk"`
	EnableRefreshLibrary    int    `json:"enable_refresh_library"`
	EnableMediaNotification int    `json:"enable_media_notification"`
	EnableExtractMediaInfo  int    `json:"enable_extract_media_info"`
	SyncEnabled             int    `json:"sync_enabled"`
	SyncCron                string `json:"sync_cron"`
}

// UpdateEmbyConfig 更新Emby配置
func UpdateEmbyConfig(c *gin.Context) {
	var req updateEmbyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error()})
		return
	}

	config, err := models.GetEmbyConfig()
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询Emby配置失败: " + err.Error()})
		return
	}
	isNew := err == gorm.ErrRecordNotFound

	syncEnabled := req.SyncEnabled
	syncCron := req.SyncCron
	if isNew {
		if syncEnabled == 0 {
			syncEnabled = 1
		}
		if syncCron == "" {
			syncCron = "*/5 * * * *"
		}
	}

	if syncEnabled == 1 && syncCron != "" {
		next := helpers.GetNextTimeByCronStr(syncCron, 1)
		if len(next) == 0 {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "cron表达式格式无效"})
			return
		}
	}

	if isNew {
		config = &models.EmbyConfig{}
		config.EmbyUrl = req.EmbyUrl
		config.EmbyApiKey = req.EmbyApiKey
		config.EnableDeleteNetdisk = req.EnableDeleteNetdisk
		config.EnableRefreshLibrary = req.EnableRefreshLibrary
		config.EnableMediaNotification = req.EnableMediaNotification
		config.EnableExtractMediaInfo = req.EnableExtractMediaInfo
		config.SyncEnabled = syncEnabled
		config.SyncCron = syncCron
		if err := db.Db.Create(config).Error; err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "创建Emby配置失败: " + err.Error()})
			return
		}
	} else {
		updates := map[string]interface{}{
			"emby_url":                  req.EmbyUrl,
			"emby_api_key":              req.EmbyApiKey,
			"enable_delete_netdisk":     req.EnableDeleteNetdisk,
			"enable_refresh_library":    req.EnableRefreshLibrary,
			"enable_media_notification": req.EnableMediaNotification,
			"enable_extract_media_info": req.EnableExtractMediaInfo,
			"sync_enabled":              syncEnabled,
			"sync_cron":                 syncCron,
		}
		if err := config.Update(updates); err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新Emby配置失败: " + err.Error()})
			return
		}
	}

	// 同步旧配置结构，保持兼容
	// models.GlobalEmbyConfig.EmbyUrl = req.EmbyUrl
	// models.GlobalEmbyConfig.EmbyApiKey = req.EmbyApiKey

	// 重新加载cron
	synccron.InitCron()

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "Emby配置更新成功"})
}

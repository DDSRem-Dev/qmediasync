package controllers

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StartEmbySync 手动触发同步
func StartEmbySync(c *gin.Context) {
	// 检查是否已有任务在运行
	if synccron.IsEmbySyncRunning() {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "已有Emby同步任务正在运行，请稍候"})
		return
	}

	go func() {
		if _, err := synccron.PerformEmbySync(); err != nil {
			helpers.AppLogger.Warnf("Emby同步失败: %v", err)
		}
	}()
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "Emby同步任务已启动"})
}

// GetEmbySyncStatus 同步状态
func GetEmbySyncStatus(c *gin.Context) {
	config, err := models.GetEmbyConfig()
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "尚未配置Emby", Data: gin.H{"exists": false}})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取配置失败: " + err.Error()})
		return
	}
	total, _ := models.GetEmbyMediaItemsCount()
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "获取同步状态成功",
		Data:    gin.H{"last_sync_time": config.LastSyncTime, "total_items": total, "sync_enabled": config.SyncEnabled},
	})
}

// GetEmbyMediaItems 分页查询
func GetEmbyMediaItems(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	libraryId := c.Query("library_id")
	itemType := c.Query("type")
	items, total, err := models.GetEmbyMediaItemsPaginated(page, pageSize, libraryId, itemType)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取Emby媒体项失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "获取Emby媒体项成功",
		Data:    gin.H{"total": total, "items": items},
	})
}

// GetEmbyLibrarySyncPaths 获取关联
func GetEmbyLibrarySyncPaths(c *gin.Context) {
	var relations []models.EmbyLibrarySyncPath
	if err := db.Db.Find(&relations).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取关联失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取成功", Data: relations})
}

// UpdateEmbyLibrarySyncPath 创建或更新关联（去重）
func UpdateEmbyLibrarySyncPath(c *gin.Context) {
	var req struct {
		LibraryId   string `json:"library_id" binding:"required"`
		SyncPathId  uint   `json:"sync_path_id" binding:"required"`
		LibraryName string `json:"library_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error()})
		return
	}
	if err := models.CreateOrUpdateEmbyLibrarySyncPath(req.LibraryId, req.SyncPathId, req.LibraryName); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新关联失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新成功"})
}

// DeleteEmbyLibrarySyncPath 删除关联
func DeleteEmbyLibrarySyncPath(c *gin.Context) {
	libraryId := c.Query("library_id")
	if libraryId == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "library_id不能为空"})
		return
	}
	if err := db.Db.Where("library_id = ?", libraryId).Delete(&models.EmbyLibrarySyncPath{}).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除成功"})
}

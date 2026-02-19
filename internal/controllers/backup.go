package controllers

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type BackupCreateRequest struct {
	Reason string `json:"reason"`
}

type BackupRestoreRequest struct {
	RecordID uint `json:"record_id"`
}

type BackupConfigUpdateRequest struct {
	BackupEnabled   int    `json:"backup_enabled"`
	BackupCron      string `json:"backup_cron"`
	BackupRetention int    `json:"backup_retention"`
	BackupMaxCount  int    `json:"backup_max_count"`
	BackupCompress  int    `json:"backup_compress"`
}

func CreateBackup(c *gin.Context) {
	var req BackupCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "手动备份"
	}

	service := models.GetBackupService()
	if service.IsRunning() {
		c.JSON(http.StatusConflict, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份任务正在运行中",
			Data:    nil,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), models.DefaultBackupTimeout)
	defer cancel()

	result, err := service.CreateBackup(ctx, models.BackupTypeManual, req.Reason)
	if err != nil {
		helpers.AppLogger.Errorf("创建手动备份失败: %v", err)
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("创建备份失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "备份创建成功",
		Data:    result.Record,
	})
}

func GetBackupList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	backupType := c.DefaultQuery("type", "all")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	service := models.GetBackupService()
	records, total, err := service.GetBackupRecords(page, pageSize, backupType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("获取备份列表失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[map[string]interface{}]{
		Code:    Success,
		Message: "success",
		Data: map[string]interface{}{
			"list":      records,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

func GetBackupRecord(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "无效的备份记录ID",
			Data:    nil,
		})
		return
	}

	var record models.BackupRecord
	if err := db.Db.First(&record, id).Error; err != nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份记录不存在",
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[models.BackupRecord]{
		Code:    Success,
		Message: "success",
		Data:    record,
	})
}

func DeleteBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "无效的备份记录ID",
			Data:    nil,
		})
		return
	}

	service := models.GetBackupService()
	if err := service.DeleteBackup(uint(id), true); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("删除备份失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "备份已删除",
		Data:    nil,
	})
}

func RestoreFromBackup(c *gin.Context) {
	var req BackupRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "请求参数无效",
			Data:    nil,
		})
		return
	}

	if req.RecordID == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "请指定要恢复的备份记录ID",
			Data:    nil,
		})
		return
	}

	service := models.GetBackupService()
	if service.IsRunning() {
		c.JSON(http.StatusConflict, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份或恢复任务正在运行中",
			Data:    nil,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), models.MaxBackupTimeout)
	defer cancel()

	if err := service.RestoreBackup(ctx, req.RecordID, ""); err != nil {
		helpers.AppLogger.Errorf("恢复备份失败: %v", err)
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("恢复备份失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "数据恢复成功",
		Data:    nil,
	})
}

func UploadAndRestore(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "请上传备份文件",
			Data:    nil,
		})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".sql" && ext != ".zip" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "仅支持.sql和.zip格式的备份文件",
			Data:    nil,
		})
		return
	}

	service := models.GetBackupService()
	if service.IsRunning() {
		c.JSON(http.StatusConflict, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份或恢复任务正在运行中",
			Data:    nil,
		})
		return
	}

	tempDir := filepath.Join(helpers.ConfigDir, "backups", "temp")
	os.MkdirAll(tempDir, 0755)
	tempPath := filepath.Join(tempDir, fmt.Sprintf("upload_%d%s", time.Now().UnixNano(), ext))

	dst, err := os.Create(tempPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: "保存上传文件失败",
			Data:    nil,
		})
		return
	}

	_, err = io.Copy(dst, file)
	dst.Close()
	if err != nil {
		os.Remove(tempPath)
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: "保存上传文件失败",
			Data:    nil,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), models.MaxBackupTimeout)
	defer cancel()

	err = service.RestoreBackup(ctx, 0, tempPath)
	os.Remove(tempPath)

	if err != nil {
		helpers.AppLogger.Errorf("上传恢复失败: %v", err)
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("恢复失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "数据恢复成功",
		Data:    nil,
	})
}

func DownloadBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "无效的备份记录ID",
			Data:    nil,
		})
		return
	}

	var record models.BackupRecord
	if err := db.Db.First(&record, id).Error; err != nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份记录不存在",
			Data:    nil,
		})
		return
	}

	if record.FilePath == "" {
		c.JSON(http.StatusNotFound, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份文件路径为空",
			Data:    nil,
		})
		return
	}

	if _, err := os.Stat(record.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, APIResponse[any]{
			Code:    BadRequest,
			Message: "备份文件不存在",
			Data:    nil,
		})
		return
	}

	fileName := filepath.Base(record.FilePath)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Type", "application/octet-stream")
	c.File(record.FilePath)
}

func GetBackupConfig(c *gin.Context) {
	service := models.GetBackupService()
	config := service.GetBackupConfig()

	c.JSON(http.StatusOK, APIResponse[models.BackupConfig]{
		Code:    Success,
		Message: "success",
		Data:    *config,
	})
}

func UpdateBackupConfig(c *gin.Context) {
	var req BackupConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{
			Code:    BadRequest,
			Message: "请求参数无效",
			Data:    nil,
		})
		return
	}

	service := models.GetBackupService()
	config := service.GetBackupConfig()

	if req.BackupCron != "" {
		config.BackupCron = req.BackupCron
	}
	if req.BackupRetention > 0 {
		config.BackupRetention = req.BackupRetention
	}
	if req.BackupMaxCount >= 0 {
		config.BackupMaxCount = req.BackupMaxCount
	}
	if req.BackupCompress >= 0 {
		config.BackupCompress = req.BackupCompress
	}
	config.BackupEnabled = req.BackupEnabled

	if err := service.UpdateBackupConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("更新配置失败: %v", err),
			Data:    nil,
		})
		return
	}

	if config.BackupEnabled == 1 && config.BackupCron != "" {
		synccron.InitCron()
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "配置已更新",
		Data:    nil,
	})
}

func GetBackupStatus(c *gin.Context) {
	service := models.GetBackupService()
	status := service.GetBackupStatus()

	c.JSON(http.StatusOK, APIResponse[map[string]interface{}]{
		Code:    Success,
		Message: "success",
		Data:    status,
	})
}

func CancelBackup(c *gin.Context) {
	service := models.GetBackupService()

	if !service.IsRunning() {
		c.JSON(http.StatusOK, APIResponse[any]{
			Code:    Success,
			Message: "没有正在运行的备份任务",
			Data:    nil,
		})
		return
	}

	if err := service.CancelBackup(); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{
			Code:    BadRequest,
			Message: fmt.Sprintf("取消备份失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "已发送取消信号",
		Data:    nil,
	})
}

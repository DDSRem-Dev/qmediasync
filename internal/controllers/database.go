package controllers

import (
	"Q115-STRM/internal/backup"
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// getDatabaseType 获取当前数据库类型
func getDatabaseType() string {
	// TODO: 根据配置或连接信息返回数据库类型
	// 当前默认返回 "postgres"
	// 未来可以从配置文件或环境变量读取
	return "postgres"
}

// GetBackupConfig 获取备份配置
func GetBackupConfig(c *gin.Context) {
	config := &models.BackupConfig{}
	result := db.Db.First(config)
	if result.Error == gorm.ErrRecordNotFound {
		// 配置不存在，返回空配置
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份配置成功", Data: map[string]interface{}{
			"exists": false,
		}})
		return
	}
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取备份配置失败: " + result.Error.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份配置成功", Data: map[string]interface{}{
		"exists": true,
		"config": config,
	}})
}

// UpdateBackupConfig 更新备份配置
func UpdateBackupConfig(c *gin.Context) {
	type updateBackupConfigRequest struct {
		BackupEnabled   int    `json:"backup_enabled"`   // 是否启用自动备份
		BackupCron      string `json:"backup_cron"`      // 备份cron表达式
		BackupPath      string `json:"backup_path"`      // 备份存储路径
		BackupRetention int    `json:"backup_retention"` // 备份保留天数
		BackupMaxCount  int    `json:"backup_max_count"` // 最多保留的备份数量
		BackupCompress  int    `json:"backup_compress"`  // 是否压缩备份
	}

	var req updateBackupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	// 验证cron表达式
	if req.BackupEnabled == 1 && req.BackupCron != "" {
		times := helpers.GetNextTimeByCronStr(req.BackupCron, 1)
		if len(times) == 0 {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "cron表达式格式无效", Data: nil})
			return
		}

		// 检查最小间隔是否满足1小时要求
		if !validateCronInterval(req.BackupCron) {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份任务定时最小间隔为1小时", Data: nil})
			return
		}
	}

	config := &models.BackupConfig{}
	result := db.Db.First(config)
	if result.Error == gorm.ErrRecordNotFound {
		// 创建新的配置记录
		config = &models.BackupConfig{
			BackupEnabled:   req.BackupEnabled,
			BackupCron:      req.BackupCron,
			BackupPath:      req.BackupPath,
			BackupRetention: req.BackupRetention,
			BackupMaxCount:  req.BackupMaxCount,
			BackupCompress:  req.BackupCompress,
		}
		if err := db.Db.Create(config).Error; err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "创建备份配置失败: " + err.Error(), Data: nil})
			return
		}
	} else if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询备份配置失败: " + result.Error.Error(), Data: nil})
		return
	} else {
		// 更新现有配置
		updateData := map[string]interface{}{
			"backup_enabled":   req.BackupEnabled,
			"backup_cron":      req.BackupCron,
			"backup_path":      req.BackupPath,
			"backup_retention": req.BackupRetention,
			"backup_max_count": req.BackupMaxCount,
			"backup_compress":  req.BackupCompress,
		}
		if err := db.Db.Model(config).Updates(updateData).Error; err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新备份配置失败: " + err.Error(), Data: nil})
			return
		}
	}

	// 重新初始化定时任务
	synccron.InitCron()

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份配置更新成功", Data: nil})
}

// StartBackupTask 启动备份任务
func StartBackupTask(c *gin.Context) {
	type startBackupRequest struct {
		Reason string `json:"reason"` // 备份原因
	}

	var req startBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	// 检查是否已有运行中的备份任务（内存）
	if models.GetCurrentBackupTask() != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "已有备份任务正在运行中，请等待完成", Data: nil})
		return
	}

	// 获取备份配置
	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置，请先配置", Data: nil})
		return
	}

	// 创建运行时备份任务（内存）
	task := &models.RuntimeBackupTask{
		ID:            models.NewBackupTaskID(),
		Status:        "running",
		Progress:      0,
		BackupType:    "manual",
		CreatedReason: req.Reason,
		CurrentStep:   "准备备份...",
		StartTime:     time.Now().Unix(),
	}
	models.SetCurrentBackupTask(task)

	// 异步执行备份
	go performBackup(config)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份任务已启动", Data: map[string]interface{}{
		"task_id": task.ID,
	}})
}

// CancelBackupTask 取消备份任务
func CancelBackupTask(c *gin.Context) {
	type cancelBackupRequest struct {
		TaskID uint `json:"task_id" binding:"required"` // 任务ID
	}

	var req cancelBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	task := models.GetCurrentBackupTask()
	if task == nil || task.ID != req.TaskID {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "任务不存在", Data: nil})
		return
	}

	if task.Status != "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "只能取消运行中的任务", Data: nil})
		return
	}

	// 标记任务为已取消（内存）
	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Status = "cancelled"
		t.EndTime = time.Now().Unix()
		t.FailureReason = "用户取消"
	})

	// 清理临时文件
	if task.FilePath != "" {
		config := &models.BackupConfig{}
		db.Db.First(config)
		if config.ID > 0 {
			backupDir := filepath.Join(helpers.RootDir, config.BackupPath)
			os.Remove(filepath.Join(backupDir, task.FilePath))
			os.Remove(filepath.Join(backupDir, task.FilePath+".gz"))
		}
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份任务已取消", Data: nil})
}

// GetBackupProgress 查询备份进度
func GetBackupProgress(c *gin.Context) {
	// 从内存获取当前备份任务
	task := models.GetCurrentBackupTask()
	if task == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "没有运行中的备份任务", Data: map[string]interface{}{
			"running": false,
		}})
		return
	}

	// 计算已耗时间
	now := time.Now().Unix()
	elapsedSeconds := now - task.StartTime
	estimatedSeconds := task.EstimatedSeconds
	if estimatedSeconds == 0 {
		estimatedSeconds = 3600 // 默认1小时
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取进度成功", Data: map[string]interface{}{
		"task_id":           task.ID,
		"running":           task.Status == "running",
		"status":            task.Status,
		"progress":          task.Progress,
		"elapsed_seconds":   elapsedSeconds,
		"estimated_seconds": estimatedSeconds,
		"current_step":      task.CurrentStep,
		"processed_tables":  task.ProcessedTables,
		"total_tables":      task.TotalTables,
	}})
}

// RestoreDatabase 恢复数据库
func RestoreDatabase(c *gin.Context) {
	// 获取上传的备份文件
	file, err := c.FormFile("backup_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "获取上传文件失败: " + err.Error(), Data: nil})
		return
	}

	// 验证文件大小（限制为1GB）
	if file.Size > 1024*1024*1024 {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件过大，最大支持1GB", Data: nil})
		return
	}

	// 验证文件扩展名
	ext := filepath.Ext(file.Filename)
	if ext != ".sql" && ext != ".gz" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "只支持.sql或.sql.gz格式的备份文件", Data: nil})
		return
	}

	// 检查是否已有运行中的备份任务
	if models.GetCurrentBackupTask() != nil && models.GetCurrentBackupTask().Status == "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "有备份任务正在运行中，无法进行恢复", Data: nil})
		return
	}

	// 检查是否已有运行中的恢复任务
	if models.GetCurrentRestoreTask() != nil && models.GetCurrentRestoreTask().Status == "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "已有恢复任务正在运行中，请等待完成", Data: nil})
		return
	}

	// 保存临时文件
	tempDir := filepath.Join(helpers.RootDir, "config", "backups", "temp")
	os.MkdirAll(tempDir, 0755)
	tempFilePath := filepath.Join(tempDir, file.Filename)

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "保存上传文件失败: " + err.Error(), Data: nil})
		return
	}

	// 验证备份文件完整性
	if !validateBackupFile(tempFilePath) {
		os.Remove(tempFilePath)
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件格式无效或已损坏", Data: nil})
		return
	}

	// 创建内存中的恢复任务
	task := &models.RuntimeRestoreTask{
		ID:               models.NewRestoreTaskID(),
		Status:           "running",
		Progress:         0,
		CurrentStep:      "准备恢复...",
		SourceFile:       file.Filename,
		StartTime:        time.Now().Unix(),
		EstimatedSeconds: 3600,
	}
	models.SetCurrentRestoreTask(task)

	// 异步执行恢复
	go performRestore(tempFilePath)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "恢复任务已启动", Data: map[string]interface{}{
		"task_id": task.ID,
	}})
}

// ListBackups 列出所有备份文件
func ListBackups(c *gin.Context) {
	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置", Data: nil})
		return
	}

	backupDir := filepath.Join(helpers.RootDir, config.BackupPath)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份列表成功", Data: []interface{}{}})
		return
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "读取备份目录失败: " + err.Error(), Data: nil})
		return
	}

	var backups []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// 跳过临时文件
		if filepath.Ext(filename) != ".sql" && filepath.Ext(filename) != ".gz" {
			continue
		}

		info, _ := entry.Info()
		fileSize := info.Size()
		modTime := info.ModTime().Unix()

		// 从数据库查询备份记录
		record := &models.BackupRecord{}
		db.Db.Where("file_path = ?", filename).First(record)

		backup := map[string]interface{}{
			"filename":       filename,
			"file_size":      fileSize,
			"modified_time":  modTime,
			"backup_type":    record.BackupType,
			"created_reason": record.CreatedReason,
			"table_count":    record.TableCount,
			"database_size":  record.DatabaseSize,
		}
		backups = append(backups, backup)
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份列表成功", Data: backups})
}

// DeleteBackup 删除单个备份文件
func DeleteBackup(c *gin.Context) {
	filename := c.Query("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "filename参数不能为空", Data: nil})
		return
	}

	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置", Data: nil})
		return
	}

	backupPath := filepath.Join(helpers.RootDir, config.BackupPath, filename)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件不存在", Data: nil})
		return
	}

	if err := os.Remove(backupPath); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除备份文件失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份文件已删除", Data: nil})
}

// DeleteBackupRecord 删除备份记录（同时删除对应的备份文件）
func DeleteBackupRecord(c *gin.Context) {
	type deleteBackupRecordRequest struct {
		RecordID uint `json:"record_id" binding:"required"` // 记录ID
	}

	var req deleteBackupRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	record := &models.BackupRecord{}
	if err := db.Db.First(record, req.RecordID).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份记录不存在", Data: nil})
		return
	}

	config := &models.BackupConfig{}
	db.Db.First(config)

	// 删除备份文件
	backupPath := filepath.Join(helpers.RootDir, config.BackupPath, record.FilePath)
	os.Remove(backupPath)
	os.Remove(backupPath + ".gz")

	// 删除数据库记录
	if err := db.Db.Delete(record).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除备份记录失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份记录及相关文件已删除", Data: nil})
}

// GetBackupRecords 获取备份历史记录
func GetBackupRecords(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "10")

	var count int64
	var records []*models.BackupRecord

	result := db.Db.Model(&models.BackupRecord{}).Count(&count)
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询记录数失败: " + result.Error.Error(), Data: nil})
		return
	}

	result = db.Db.Order("created_at DESC").Offset((toInt(page) - 1) * toInt(pageSize)).Limit(toInt(pageSize)).Find(&records)
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询备份记录失败: " + result.Error.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份记录成功", Data: map[string]interface{}{
		"total":   count,
		"records": records,
	}})
}

// performBackup 执行备份操作
func performBackup(config *models.BackupConfig) {
	defer func() {
		if r := recover(); r != nil {
			helpers.AppLogger.Errorf("备份任务发生异常: %v", r)
			models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("%v", r)
				t.EndTime = time.Now().Unix()
			})
		}
	}()

	// 确保备份目录存在
	backupDir := filepath.Join(helpers.RootDir, config.BackupPath)
	os.MkdirAll(backupDir, 0755)

	// 生成备份文件名
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	sqlFileName := fmt.Sprintf("backup_%s.sql", timestamp)
	sqlFilePath := filepath.Join(backupDir, sqlFileName)

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.FilePath = sqlFileName
		t.CurrentStep = "正在导出数据库..."
	})

	// 使用通用备份方法导出数据库
	tableCount, dbSize, err := ExportDatabaseToSQL(sqlFilePath)
	if err != nil {
		helpers.AppLogger.Errorf("数据库备份失败: %v", err)
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.Status = "failed"
			t.FailureReason = err.Error()
			t.EndTime = time.Now().Unix()
		})
		return
	}

	// 获取文件大小
	fileInfo, _ := os.Stat(sqlFilePath)
	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.FileSize = fileInfo.Size()
		t.Progress = 70
	})

	// 压缩备份
	if config.BackupCompress == 1 {
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.CurrentStep = "正在压缩备份文件..."
		})

		gzFilePath := sqlFilePath + ".gz"
		if err := compressFile(sqlFilePath, gzFilePath); err != nil {
			helpers.AppLogger.Errorf("压缩备份失败: %v", err)
			models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
				t.Status = "failed"
				t.FailureReason = "压缩失败: " + err.Error()
				t.EndTime = time.Now().Unix()
			})
			os.Remove(sqlFilePath)
			return
		}

		// 删除原始SQL文件，使用压缩后的文件
		os.Remove(sqlFilePath)
		gzInfo, _ := os.Stat(gzFilePath)
		compressedSize := gzInfo.Size()
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.FilePath = sqlFileName + ".gz"
			if t.FileSize > 0 {
				t.CompressionRatio = float64(compressedSize) / float64(t.FileSize)
			}
			t.FileSize = compressedSize
		})
	}

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Progress = 90
		t.CurrentStep = "正在保存备份记录..."
	})

	// 获取最终任务状态用于保存记录
	finalTask := models.GetCurrentBackupTask()

	// 保存备份记录
	duration := time.Now().Unix() - finalTask.StartTime
	record := &models.BackupRecord{
		Status:           "completed",
		FilePath:         finalTask.FilePath,
		FileSize:         finalTask.FileSize,
		TableCount:       tableCount,
		DatabaseSize:     dbSize,
		BackupType:       "manual",
		CreatedReason:    finalTask.CreatedReason,
		BackupDuration:   duration,
		CompressionRatio: finalTask.CompressionRatio,
		IsCompressed:     config.BackupCompress,
		CompletedAt:      time.Now().Unix(),
	}

	if err := db.Db.Create(record).Error; err != nil {
		helpers.AppLogger.Errorf("保存备份记录失败: %v", err)
	}

	// 清理旧备份文件
	cleanupOldBackups(config)

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Status = "completed"
		t.Progress = 100
		t.CurrentStep = "备份完成"
		t.EndTime = time.Now().Unix()
	})

	helpers.AppLogger.Infof("备份任务完成，文件: %s", finalTask.FilePath)

	// 清除内存中的任务（可选，或者保留一段时间供查询）
	// models.ClearCurrentBackupTask()
}

// performRestore 执行恢复操作
// performRestore 执行恢复操作
func performRestore(backupFilePath string) {
	defer func() {
		if r := recover(); r != nil {
			helpers.AppLogger.Errorf("恢复任务发生异常: %v", r)
			models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("%v", r)
				t.EndTime = time.Now().Unix()
			})
		}
		// 恢复上传/下载队列运行
		if models.GlobalUploadQueue != nil {
			models.GlobalUploadQueue.Restart()
		}
		if models.GlobalDownloadQueue != nil {
			models.GlobalDownloadQueue.Restart()
		}
		// 重启定时任务
		synccron.InitCron()

	}()

	// 暂停所有定时任务
	helpers.AppLogger.Info("恢复开始：暂停所有定时任务")
	if synccron.GlobalCron != nil {
		synccron.GlobalCron.Stop()
	}

	// 暂停上传/下载队列，避免恢复期间产生写入
	if models.GlobalUploadQueue != nil {
		helpers.AppLogger.Info("恢复开始：暂停上传队列")
		models.GlobalUploadQueue.Stop()
	}
	if models.GlobalDownloadQueue != nil {
		helpers.AppLogger.Info("恢复开始：暂停下载队列")
		models.GlobalDownloadQueue.Stop()
	}

	// 获取数据库连接和驱动程序
	sqlDB, err := db.Db.DB()
	if err != nil {
		helpers.AppLogger.Errorf("获取数据库连接失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("获取数据库连接失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		os.Remove(backupFilePath)
		return
	}

	// 创建数据库驱动程序
	dbType := getDatabaseType()
	factory := backup.NewDriverFactory(dbType, sqlDB)
	driver := factory.CreateDriver()

	// 步骤1: 清空所有表数据
	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.CurrentStep = "正在清空表数据..."
		t.Progress = 10
	})
	helpers.AppLogger.Info("正在清空表数据...")

	if err := driver.TruncateAllTables(); err != nil {
		helpers.AppLogger.Errorf("清空表失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("清空表失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		os.Remove(backupFilePath)
		return
	}

	// 步骤2: 导入数据
	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.CurrentStep = "正在导入数据..."
		t.Progress = 50
	})
	helpers.AppLogger.Info("正在导入数据...")

	file, err := os.Open(backupFilePath)
	if err != nil {
		helpers.AppLogger.Errorf("打开备份文件失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("打开备份文件失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		os.Remove(backupFilePath)
		return
	}
	defer file.Close()

	// 如果是gz压缩文件，进行解压
	var reader io.Reader = file
	if filepath.Ext(backupFilePath) == ".gz" {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			helpers.AppLogger.Errorf("打开压缩文件失败: %v", err)
			models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("打开压缩文件失败: %v", err)
				t.EndTime = time.Now().Unix()
			})
			os.Remove(backupFilePath)
			return
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	if err := driver.ImportFromSQL(reader); err != nil {
		helpers.AppLogger.Errorf("导入数据失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("导入数据失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		os.Remove(backupFilePath)
		return
	}

	// 恢复成功，清理临时文件
	os.Remove(backupFilePath)

	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.Status = "completed"
		t.Progress = 100
		t.EndTime = time.Now().Unix()
		t.Progress = 100
	})
	helpers.AppLogger.Info("数据库恢复完成")
}

// GetRestoreProgress 查询恢复进度
// GetRestoreProgress 查询恢复进度
func GetRestoreProgress(c *gin.Context) {
	// 从内存中读取恢复任务状态
	task := models.GetCurrentRestoreTask()

	if task == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "没有运行中的恢复任务", Data: map[string]interface{}{
			"running": false,
		}})
		return
	}

	// 计算已耗时间
	now := time.Now().Unix()
	elapsedSeconds := now - task.StartTime
	estimatedSeconds := task.EstimatedSeconds
	if estimatedSeconds == 0 {
		estimatedSeconds = 3600 // 默认1小时
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取恢复进度成功", Data: map[string]interface{}{
		"task_id":           task.ID,
		"running":           task.Status == "running",
		"status":            task.Status,
		"progress":          task.Progress,
		"elapsed_seconds":   elapsedSeconds,
		"estimated_seconds": estimatedSeconds,
		"current_step":      task.CurrentStep,
		"source_file":       task.SourceFile,
		"rollback_file":     task.RollbackFile,
	}})
}

// validateBackupFile 验证备份文件的完整性
func validateBackupFile(filePath string) bool {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	// 对于.sql或.sql.gz文件进行简单的格式检查
	// 这里可以添加更复杂的验证逻辑，如使用pg_restore --list等
	ext := filepath.Ext(filePath)
	if ext == ".gz" {
		// 对于.gz文件，检查magic number（0x1f8b）
		file, err := os.Open(filePath)
		if err != nil {
			return false
		}
		defer file.Close()

		header := make([]byte, 2)
		if _, err := file.Read(header); err != nil {
			return false
		}
		return header[0] == 0x1f && header[1] == 0x8b
	}

	return true
}

// cleanupOldBackups 清理旧备份文件
func cleanupOldBackups(config *models.BackupConfig) {
	backupDir := filepath.Join(helpers.RootDir, config.BackupPath)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		helpers.AppLogger.Errorf("读取备份目录失败: %v", err)
		return
	}

	var backupFiles []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if filepath.Ext(filename) != ".sql" && filepath.Ext(filename) != ".gz" {
			continue
		}

		info, _ := entry.Info()
		backupFiles = append(backupFiles, info)
	}

	// 按修改时间排序（从新到旧）
	for i := 0; i < len(backupFiles); i++ {
		for j := i + 1; j < len(backupFiles); j++ {
			if backupFiles[i].ModTime().Before(backupFiles[j].ModTime()) {
				backupFiles[i], backupFiles[j] = backupFiles[j], backupFiles[i]
			}
		}
	}

	now := time.Now()
	maxCount := config.BackupMaxCount
	retentionDays := config.BackupRetention

	// 删除超过数量限制的备份
	if len(backupFiles) > maxCount {
		for i := maxCount; i < len(backupFiles); i++ {
			filePath := filepath.Join(backupDir, backupFiles[i].Name())
			if err := os.Remove(filePath); err != nil {
				helpers.AppLogger.Warnf("删除备份文件失败: %s, %v", backupFiles[i].Name(), err)
			} else {
				helpers.AppLogger.Infof("已删除超期备份: %s", backupFiles[i].Name())
			}
		}
	}

	// 删除超过保留天数的备份
	for _, fileInfo := range backupFiles {
		if now.Sub(fileInfo.ModTime()) > time.Duration(retentionDays)*24*time.Hour {
			filePath := filepath.Join(backupDir, fileInfo.Name())
			if err := os.Remove(filePath); err != nil {
				helpers.AppLogger.Warnf("删除备份文件失败: %s, %v", fileInfo.Name(), err)
			} else {
				helpers.AppLogger.Infof("已删除超期备份: %s", fileInfo.Name())
			}
		}
	}
}

// validateCronInterval 验证cron表达式的最小间隔是否至少为1小时
func validateCronInterval(cronExpr string) bool {
	times := helpers.GetNextTimeByCronStr(cronExpr, 2)
	if len(times) < 2 {
		return false
	}

	// 计算两次执行之间的时间差
	interval := times[1].Sub(times[0])
	return interval >= 1*time.Hour
}

// compressFile 压缩文件为.gz格式
func compressFile(sourceFile, targetFile string) error {
	source, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer target.Close()

	gzipWriter := gzip.NewWriter(target)
	defer gzipWriter.Close()

	_, err = io.Copy(gzipWriter, source)
	return err
}

func toInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

// ExportDatabaseToSQL 导出整个数据库到SQL文件，返回表数量和数据库大小
func ExportDatabaseToSQL(filePath string) (int, int64, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	// 获取当前数据库连接
	sqlDB, err := db.Db.DB()
	if err != nil {
		return 0, 0, fmt.Errorf("获取数据库连接失败: %v", err)
	}

	// 获取所有表名
	var tables []string
	if err := db.Db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return 0, 0, fmt.Errorf("获取表列表失败: %v", err)
	}

	if len(tables) == 0 {
		return 0, 0, fmt.Errorf("数据库中没有表")
	}

	tableCount := len(tables)

	// 获取数据库大小
	var dbSize int64
	if err := db.Db.Raw("SELECT pg_database_size(current_database())").Scan(&dbSize).Error; err != nil {
		helpers.AppLogger.Warnf("获取数据库大小失败: %v，使用备份文件大小", err)
		dbSize = 0
	}

	// 写入数据库导出头
	file.WriteString("-- Database backup\n")
	file.WriteString(fmt.Sprintf("-- Generated at %s\n", time.Now().Format("2006-01-02 15:04:05")))
	file.WriteString(fmt.Sprintf("-- Tables: %d\n\n", tableCount))

	// 对每个表进行导出
	for _, tableName := range tables {
		// 导出表数据
		rows, err := sqlDB.Query(fmt.Sprintf("SELECT * FROM \"%s\"", tableName))
		if err != nil {
			helpers.AppLogger.Warnf("读取表%s失败: %v", tableName, err)
			continue
		}

		// 获取列信息
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			helpers.AppLogger.Warnf("获取表%s列信息失败: %v", tableName, err)
			continue
		}

		// 构建INSERT语句
		if len(columns) > 0 {
			// 写入表信息
			file.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))

			// 构建列名部分
			columnNames := make([]string, len(columns))
			for i, col := range columns {
				columnNames[i] = fmt.Sprintf("\"%s\"", col)
			}
			columnPart := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES ", tableName, strings.Join(columnNames, ", "))

			// 遍历所有行
			for rows.Next() {
				values := make([]interface{}, len(columns))
				valuePtrs := make([]interface{}, len(columns))
				for i := range columns {
					valuePtrs[i] = &values[i]
				}

				if err := rows.Scan(valuePtrs...); err != nil {
					helpers.AppLogger.Warnf("读取表%s数据失败: %v", tableName, err)
					continue
				}

				// 构建VALUES部分
				valueParts := make([]string, len(values))
				for i, val := range values {
					if val == nil {
						valueParts[i] = "NULL"
					} else if b, ok := val.([]byte); ok {
						// 处理字节数组（文本类型）
						escapedStr := strings.ReplaceAll(string(b), "'", "''")
						valueParts[i] = fmt.Sprintf("'%s'", escapedStr)
					} else if t, ok := val.(time.Time); ok {
						// 处理时间类型 - 转换为ISO 8601格式
						valueParts[i] = fmt.Sprintf("'%s'", t.Format(time.RFC3339Nano))
					} else {
						// 检查是否是time.Time类型（通过反射）
						valType := reflect.TypeOf(val)
						if valType != nil && valType.String() == "time.Time" {
							// 使用反射获取time.Time值
							if t, ok := val.(time.Time); ok {
								valueParts[i] = fmt.Sprintf("'%s'", t.Format(time.RFC3339Nano))
								continue
							}
						}

						// 其他类型直接转换为字符串
						valStr := fmt.Sprintf("%v", val)
						if _, err := strconv.ParseFloat(valStr, 64); err != nil {
							// 不是数字，需要转义引号
							escapedStr := strings.ReplaceAll(valStr, "'", "''")
							valueParts[i] = fmt.Sprintf("'%s'", escapedStr)
						} else {
							// 是数字，直接使用
							valueParts[i] = valStr
						}
					}
				}

				// 完整INSERT语句
				insertStmt := fmt.Sprintf("%s(%s);\n", columnPart, strings.Join(valueParts, ", "))
				file.WriteString(insertStmt)
			}
		}
		rows.Close()
		file.WriteString("\n")
	}

	helpers.AppLogger.Infof("数据库已导出到: %s (表数: %d, 数据库大小: %d bytes)", filePath, tableCount, dbSize)
	return tableCount, dbSize, nil
}

// ImportDatabaseFromSQL 从SQL文件恢复数据库
func ImportDatabaseFromSQL(filePath string) error {
	// 验证文件存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在: %s", filePath)
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %v", err)
	}
	defer file.Close()

	// 如果是gz压缩文件，进行解压
	var reader io.Reader = file
	if filepath.Ext(filePath) == ".gz" {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("打开压缩文件失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	// 读取SQL内容
	content := make([]byte, 0)
	buffer := make([]byte, 1024*1024) // 1MB缓冲
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			content = append(content, buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取备份文件失败: %v", err)
		}
	}

	// 获取数据库连接
	sqlDB, err := db.Db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %v", err)
	}

	// 禁用外键约束
	if _, err := sqlDB.Exec("SET session_replication_role = replica"); err != nil {
		helpers.AppLogger.Warnf("禁用外键约束失败: %v", err)
	}

	// 执行恢复SQL
	sqlStr := string(content)
	if _, err := sqlDB.Exec(sqlStr); err != nil {
		// 恢复外键约束
		sqlDB.Exec("SET session_replication_role = default")
		return fmt.Errorf("执行恢复SQL失败: %v", err)
	}

	// 恢复外键约束
	if _, err := sqlDB.Exec("SET session_replication_role = default"); err != nil {
		helpers.AppLogger.Warnf("恢复外键约束失败: %v", err)
	}

	helpers.AppLogger.Infof("数据库已从备份恢复: %s", filePath)
	return nil
}

// resetAllSequences 重置所有序列为max(id)+1
func resetAllSequences(sqlDB *sql.DB) error {
	// 获取所有表及其主键信息
	var tables []string
	rows, err := sqlDB.Query(`
		SELECT table_name FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		tables = append(tables, tableName)
	}

	// 为每个表的主键序列设置正确的值
	for _, tableName := range tables {
		// 查找表的主键列
		var pkCol string
		err := sqlDB.QueryRow(fmt.Sprintf(`
			SELECT a.attname FROM pg_index i 
			JOIN pg_attribute a ON a.attrelid = i.indrelid 
			AND a.attnum = ANY(i.indkey) 
			WHERE i.indrelid = '%s'::regclass AND i.indisprimary
		`, tableName)).Scan(&pkCol)

		if err != nil && err != sql.ErrNoRows {
			helpers.AppLogger.Warnf("查询表%s主键失败: %v", tableName, err)
			continue
		}

		if pkCol == "" {
			continue // 没有主键
		}

		// 获取主键的最大值
		var maxId int64
		err = sqlDB.QueryRow(fmt.Sprintf(`SELECT COALESCE(MAX("%s"), 0) FROM "%s"`, pkCol, tableName)).Scan(&maxId)
		if err != nil && err != sql.ErrNoRows {
			helpers.AppLogger.Warnf("查询表%s的最大ID失败: %v", tableName, err)
			continue
		}

		// 获取对应的序列名
		var seqName string
		err = sqlDB.QueryRow(fmt.Sprintf(`
			SELECT pg_get_serial_sequence('%s', '%s')
		`, tableName, pkCol)).Scan(&seqName)

		if err != nil || seqName == "" {
			continue // 没有关联的序列
		}

		// 重置序列为 max(id) + 1
		nextVal := maxId + 1
		if _, err := sqlDB.Exec(fmt.Sprintf(`ALTER SEQUENCE "%s" RESTART WITH %d`, seqName, nextVal)); err != nil {
			helpers.AppLogger.Warnf("重置序列%s失败: %v", seqName, err)
		} else {
			helpers.AppLogger.Infof("序列%s已重置为%d", seqName, nextVal)
		}
	}

	return nil
}

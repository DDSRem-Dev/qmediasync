package models

import (
	"sync"
)

// RuntimeBackupTask 运行时备份任务信息（内存存储）
type RuntimeBackupTask struct {
	ID               uint    `json:"id"`
	Status           string  `json:"status"`            // 任务状态：running/completed/cancelled/timeout/failed
	Progress         int     `json:"progress"`          // 进度百分比（0-100）
	ElapsedSeconds   int64   `json:"elapsed_seconds"`   // 已耗时间（秒）
	EstimatedSeconds int64   `json:"estimated_seconds"` // 预计总耗时（秒）
	CurrentStep      string  `json:"current_step"`      // 当前操作步骤描述
	TotalTables      int     `json:"total_tables"`      // 总表数
	ProcessedTables  int     `json:"processed_tables"`  // 已处理表数
	FilePath         string  `json:"file_path"`         // 备份文件路径
	FileSize         int64   `json:"file_size"`         // 备份文件大小（字节）
	DatabaseSize     int64   `json:"database_size"`     // 数据库大小（字节）
	BackupType       string  `json:"backup_type"`       // 备份类型：manual/auto
	CreatedReason    string  `json:"created_reason"`    // 创建原因
	FailureReason    string  `json:"failure_reason"`    // 失败原因
	StartTime        int64   `json:"start_time"`        // 开始时间戳
	EndTime          int64   `json:"end_time"`          // 结束时间戳
	CompressionRatio float64 `json:"compression_ratio"` // 压缩比例
}

// RuntimeRestoreTask 运行时恢复任务信息（内存存储）
type RuntimeRestoreTask struct {
	ID               uint   `json:"id"`
	Status           string `json:"status"`            // 任务状态：running/completed/cancelled/timeout/failed
	Progress         int    `json:"progress"`          // 进度百分比（0-100）
	ElapsedSeconds   int64  `json:"elapsed_seconds"`   // 已耗时间（秒）
	EstimatedSeconds int64  `json:"estimated_seconds"` // 预计总耗时（秒）
	CurrentStep      string `json:"current_step"`      // 当前操作步骤描述
	SourceFile       string `json:"source_file"`       // 恢复源文件路径
	RollbackFile     string `json:"rollback_file"`     // 创建的回滚备份文件路径
	FailureReason    string `json:"failure_reason"`    // 失败原因
	StartTime        int64  `json:"start_time"`        // 开始时间戳
	EndTime          int64  `json:"end_time"`          // 结束时间戳
}

var (
	// 当前运行的备份任务
	currentBackupTask *RuntimeBackupTask
	backupTaskMutex   sync.RWMutex
	backupTaskIDSeq   uint = 0

	// 当前运行的恢复任务
	currentRestoreTask *RuntimeRestoreTask
	restoreTaskMutex   sync.RWMutex
	restoreTaskIDSeq   uint = 0
)

// SetCurrentBackupTask 设置当前备份任务
func SetCurrentBackupTask(task *RuntimeBackupTask) {
	backupTaskMutex.Lock()
	defer backupTaskMutex.Unlock()
	currentBackupTask = task
}

// GetCurrentBackupTask 获取当前备份任务
func GetCurrentBackupTask() *RuntimeBackupTask {
	backupTaskMutex.RLock()
	defer backupTaskMutex.RUnlock()
	return currentBackupTask
}

// ClearCurrentBackupTask 清除当前备份任务
func ClearCurrentBackupTask() {
	backupTaskMutex.Lock()
	defer backupTaskMutex.Unlock()
	currentBackupTask = nil
}

// UpdateBackupTask 更新备份任务字段
func UpdateBackupTask(updater func(*RuntimeBackupTask)) {
	backupTaskMutex.Lock()
	defer backupTaskMutex.Unlock()
	if currentBackupTask != nil {
		updater(currentBackupTask)
	}
}

// NewBackupTaskID 生成新的备份任务ID
func NewBackupTaskID() uint {
	backupTaskMutex.Lock()
	defer backupTaskMutex.Unlock()
	backupTaskIDSeq++
	return backupTaskIDSeq
}

// SetCurrentRestoreTask 设置当前恢复任务
func SetCurrentRestoreTask(task *RuntimeRestoreTask) {
	restoreTaskMutex.Lock()
	defer restoreTaskMutex.Unlock()
	currentRestoreTask = task
}

// GetCurrentRestoreTask 获取当前恢复任务
func GetCurrentRestoreTask() *RuntimeRestoreTask {
	restoreTaskMutex.RLock()
	defer restoreTaskMutex.RUnlock()
	return currentRestoreTask
}

// ClearCurrentRestoreTask 清除当前恢复任务
func ClearCurrentRestoreTask() {
	restoreTaskMutex.Lock()
	defer restoreTaskMutex.Unlock()
	currentRestoreTask = nil
}

// UpdateRestoreTask 更新恢复任务字段
func UpdateRestoreTask(updater func(*RuntimeRestoreTask)) {
	restoreTaskMutex.Lock()
	defer restoreTaskMutex.Unlock()
	if currentRestoreTask != nil {
		updater(currentRestoreTask)
	}
}

// NewRestoreTaskID 生成新的恢复任务ID
func NewRestoreTaskID() uint {
	restoreTaskMutex.Lock()
	defer restoreTaskMutex.Unlock()
	restoreTaskIDSeq++
	return restoreTaskIDSeq
}

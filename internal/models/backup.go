package models

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

var (
	PauseSyncQueuesFunc  func()
	ResumeSyncQueuesFunc func()
)

const (
	BackupStatusPending   = "pending"
	BackupStatusRunning   = "running"
	BackupStatusCompleted = "completed"
	BackupStatusFailed    = "failed"
	BackupStatusCancelled = "cancelled"
	BackupStatusTimeout   = "timeout"

	BackupTypeManual = "manual"
	BackupTypeAuto   = "auto"

	DefaultBackupRetention = 7
	DefaultBackupMaxCount  = 10
	DefaultBackupTimeout   = 30 * time.Minute
	MaxBackupTimeout       = 2 * time.Hour
)

var (
	GlobalBackupService *BackupService
	backupMutex         sync.Mutex
)

type BackupService struct {
	db             *gorm.DB
	config         *BackupConfig
	backupDir      string
	isRunning      bool
	runningMutex   sync.RWMutex
	cancelFunc     context.CancelFunc
	timeout        time.Duration
	statusChangeMu sync.Mutex
}

// BackupConfig 备份配置
type BackupConfig struct {
	BaseModel
	BackupEnabled   int    `json:"backup_enabled" gorm:"default:0"`    // 是否启用自动备份，0表示禁用，1表示启用
	BackupCron      string `json:"backup_cron"`                        // 备份cron表达式
	BackupPath      string `json:"backup_path"`                        // 备份存储路径
	BackupRetention int    `json:"backup_retention" gorm:"default:7"`  // 备份保留天数
	BackupMaxCount  int    `json:"backup_max_count" gorm:"default:10"` // 最多保留的备份数量
	BackupCompress  int    `json:"backup_compress" gorm:"default:1"`   // 是否压缩备份，0表示不压缩，1表示压缩
}

func (*BackupConfig) TableName() string {
	return "backup_config"
}

// BackupRecord 备份记录（历史记录）
type BackupRecord struct {
	BaseModel
	TaskID           uint    `json:"task_id"`           // 关联的任务ID
	Status           string  `json:"status"`            // 任务状态：completed/cancelled/timeout/failed
	FilePath         string  `json:"file_path"`         // 备份文件路径
	FileSize         int64   `json:"file_size"`         // 备份文件大小（字节）
	DatabaseSize     int64   `json:"database_size"`     // 数据库大小（字节）
	TableCount       int     `json:"table_count"`       // 表数量
	BackupDuration   int64   `json:"backup_duration"`   // 备份耗时（秒）
	BackupType       string  `json:"backup_type"`       // 备份类型：manual/auto
	CreatedReason    string  `json:"created_reason"`    // 创建原因
	FailureReason    string  `json:"failure_reason"`    // 失败原因
	CompressionRatio float64 `json:"compression_ratio"` // 压缩比例
	IsCompressed     int     `json:"is_compressed"`     // 是否已压缩，0表示否，1表示是
	CompletedAt      int64   `json:"completed_at"`      // 完成时间戳
}

func (*BackupRecord) TableName() string {
	return "backup_record"
}

func UpdateBackupCronConfig(newCron string, enabled bool) error {
	config := GetOrCreateBackupConfig()
	config.BackupCron = newCron
	if enabled {
		config.BackupEnabled = 1
	} else {
		config.BackupEnabled = 0
	}

	if err := db.Db.Save(config).Error; err != nil {
		return err
	}

	if GetBackupService() != nil {
		GetBackupService().config = config
	}

	return nil
}

func GetBackupCronNextTimes(count int) []time.Time {
	config := GetOrCreateBackupConfig()
	if config.BackupCron == "" {
		return nil
	}

	schedule, err := ParseCron(config.BackupCron)
	if err != nil {
		return nil
	}

	now := time.Now()
	times := make([]time.Time, 0, count)
	next := now

	for i := 0; i < count; i++ {
		next = schedule.Next(next)
		times = append(times, next)
	}

	return times
}

func ParseCron(cronExpr string) (Schedule, error) {
	return &cronSchedule{expr: cronExpr}, nil
}

type Schedule interface {
	Next(time.Time) time.Time
}

type cronSchedule struct {
	expr string
}

func (s *cronSchedule) Next(t time.Time) time.Time {
	return t.Add(time.Hour)
}

func InitBackupService() *BackupService {
	if GlobalBackupService != nil {
		return GlobalBackupService
	}

	config := GetOrCreateBackupConfig()
	backupDir := filepath.Join(helpers.ConfigDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		helpers.AppLogger.Errorf("创建备份目录失败: %v", err)
	}

	GlobalBackupService = &BackupService{
		db:        db.Db,
		config:    config,
		backupDir: backupDir,
		timeout:   DefaultBackupTimeout,
	}

	helpers.AppLogger.Infof("备份服务已初始化，备份目录: %s", backupDir)
	return GlobalBackupService
}

func GetBackupService() *BackupService {
	if GlobalBackupService == nil {
		return InitBackupService()
	}
	return GlobalBackupService
}

func GetOrCreateBackupConfig() *BackupConfig {
	var config BackupConfig
	if err := db.Db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			config = BackupConfig{
				BackupEnabled:   0,
				BackupCron:      "0 3 * * *",
				BackupRetention: DefaultBackupRetention,
				BackupMaxCount:  DefaultBackupMaxCount,
				BackupCompress:  1,
			}
			if err := db.Db.Create(&config).Error; err != nil {
				helpers.AppLogger.Errorf("创建默认备份配置失败: %v", err)
				return &config
			}
			helpers.AppLogger.Info("已创建默认备份配置")
		} else {
			helpers.AppLogger.Errorf("获取备份配置失败: %v", err)
			return &BackupConfig{
				BackupRetention: DefaultBackupRetention,
				BackupMaxCount:  DefaultBackupMaxCount,
				BackupCompress:  1,
			}
		}
	}
	return &config
}

func (s *BackupService) IsRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.isRunning
}

func (s *BackupService) SetRunning(running bool) {
	s.runningMutex.Lock()
	defer s.runningMutex.Unlock()
	s.isRunning = running
}

type BackupResult struct {
	Record   *BackupRecord
	FilePath string
	Error    error
}

func (s *BackupService) CreateBackup(ctx context.Context, backupType string, reason string) (*BackupResult, error) {
	backupMutex.Lock()
	defer backupMutex.Unlock()

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), DefaultBackupTimeout)
		defer cancel()
	}

	if s.IsRunning() {
		return nil, fmt.Errorf("备份任务正在运行中")
	}

	s.SetRunning(true)
	defer s.SetRunning(false)

	record := &BackupRecord{
		Status:        BackupStatusRunning,
		BackupType:    backupType,
		CreatedReason: reason,
	}

	if err := db.Db.Create(record).Error; err != nil {
		helpers.AppLogger.Errorf("创建备份记录失败: %v", err)
		return nil, err
	}

	startTime := time.Now()
	result := &BackupResult{Record: record}

	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("备份过程发生异常: %v", r)
			helpers.AppLogger.Errorf(errMsg)
			result.Error = fmt.Errorf("%s", errMsg)
			s.updateRecordFailure(record, errMsg)
		}
	}()

	helpers.AppLogger.Infof("开始%s备份，备份记录ID: %d", backupType, record.ID)

	if err := s.pauseAllQueues(); err != nil {
		helpers.AppLogger.Warnf("暂停队列时出现警告: %v", err)
	}
	defer s.resumeAllQueues()

	tables, err := s.getAllTables()
	if err != nil {
		result.Error = err
		s.updateRecordFailure(record, fmt.Sprintf("获取表列表失败: %v", err))
		return result, err
	}
	record.TableCount = len(tables)

	var sqlContent bytes.Buffer
	sqlContent.WriteString(s.generateSQLHeader())

	for _, table := range tables {
		select {
		case <-ctx.Done():
			s.updateRecordFailure(record, "备份被取消")
			result.Error = fmt.Errorf("备份被取消")
			return result, result.Error
		default:
		}

		tableSQL, err := s.exportTableSQL(table)
		if err != nil {
			helpers.AppLogger.Warnf("导出表 %s 失败: %v", table, err)
			continue
		}
		sqlContent.WriteString(tableSQL)
	}

	timestamp := time.Now().Format("20060102_150405")
	var fileName string
	if s.config.BackupCompress == 1 {
		fileName = fmt.Sprintf("backup_%s_%s.sql.zip", backupType, timestamp)
	} else {
		fileName = fmt.Sprintf("backup_%s_%s.sql", backupType, timestamp)
	}
	filePath := filepath.Join(s.backupDir, fileName)

	var finalSize int64
	if s.config.BackupCompress == 1 {
		finalSize, err = s.writeCompressedFile(filePath, sqlContent.Bytes())
	} else {
		finalSize, err = s.writeFile(filePath, sqlContent.Bytes())
	}

	if err != nil {
		result.Error = err
		s.updateRecordFailure(record, fmt.Sprintf("写入备份文件失败: %v", err))
		return result, err
	}

	record.FilePath = filePath
	record.FileSize = finalSize
	record.DatabaseSize = int64(sqlContent.Len())
	if record.DatabaseSize > 0 {
		record.CompressionRatio = float64(finalSize) / float64(record.DatabaseSize)
	}
	record.IsCompressed = s.config.BackupCompress
	record.BackupDuration = int64(time.Since(startTime).Seconds())
	record.Status = BackupStatusCompleted
	record.CompletedAt = time.Now().Unix()

	if err := db.Db.Save(record).Error; err != nil {
		helpers.AppLogger.Errorf("更新备份记录失败: %v", err)
	}

	result.FilePath = filePath
	helpers.AppLogger.Infof("备份完成，文件: %s, 大小: %d 字节, 耗时: %d 秒",
		filePath, finalSize, record.BackupDuration)

	go s.cleanupOldBackups()

	return result, nil
}

func (s *BackupService) RestoreBackup(ctx context.Context, recordID uint, uploadFilePath string) error {
	backupMutex.Lock()
	defer backupMutex.Unlock()

	if s.IsRunning() {
		return fmt.Errorf("备份或恢复任务正在运行中")
	}

	s.SetRunning(true)
	defer s.SetRunning(false)

	var record *BackupRecord
	var filePath string

	if uploadFilePath != "" {
		filePath = uploadFilePath
		record = &BackupRecord{
			Status:     BackupStatusRunning,
			BackupType: "restore_upload",
		}
		db.Db.Create(record)
	} else {
		if err := db.Db.First(&record, recordID).Error; err != nil {
			return fmt.Errorf("备份记录不存在: %v", err)
		}
		filePath = record.FilePath
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在: %s", filePath)
	}

	helpers.AppLogger.Infof("开始恢复数据，备份文件: %s", filePath)

	_, err := s.CreateBackup(ctx, BackupTypeManual, "恢复前自动备份")
	if err != nil {
		helpers.AppLogger.Warnf("创建恢复前备份失败: %v", err)
	}

	if err := s.pauseAllQueues(); err != nil {
		helpers.AppLogger.Warnf("暂停队列时出现警告: %v", err)
	}
	defer s.resumeAllQueues()

	var sqlContent []byte
	if strings.HasSuffix(filePath, ".zip") {
		sqlContent, err = s.readCompressedFile(filePath)
	} else {
		sqlContent, err = os.ReadFile(filePath)
	}

	if err != nil {
		s.updateRecordFailure(record, fmt.Sprintf("读取备份文件失败: %v", err))
		return fmt.Errorf("读取备份文件失败: %v", err)
	}

	if err := s.executeRestoreSQL(ctx, string(sqlContent)); err != nil {
		s.updateRecordFailure(record, fmt.Sprintf("执行恢复SQL失败: %v", err))
		return fmt.Errorf("执行恢复SQL失败: %v", err)
	}

	record.Status = BackupStatusCompleted
	record.CompletedAt = time.Now().Unix()
	db.Db.Save(record)

	helpers.AppLogger.Infof("数据恢复完成")
	return nil
}

func (s *BackupService) getAllTables() ([]string, error) {
	var tables []string

	if helpers.GlobalConfig.Db.Engine == helpers.DbEnginePostgres {
		err := db.Db.Raw(`
			SELECT tablename FROM pg_tables 
			WHERE schemaname = 'public'
		`).Scan(&tables).Error
		return tables, err
	}

	err := db.Db.Raw(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`).Scan(&tables).Error
	return tables, err
}

func (s *BackupService) generateSQLHeader() string {
	var header strings.Builder
	header.WriteString("-- QMediaSync Database Backup\n")
	header.WriteString(fmt.Sprintf("-- Generated at: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	header.WriteString(fmt.Sprintf("-- Database Engine: %s\n", helpers.GlobalConfig.Db.Engine))
	header.WriteString("-- \n\n")

	if helpers.GlobalConfig.Db.Engine == helpers.DbEnginePostgres {
		header.WriteString("SET statement_timeout = 0;\n")
		header.WriteString("SET lock_timeout = 0;\n")
		header.WriteString("SET client_encoding = 'UTF8';\n\n")
	} else {
		header.WriteString("PRAGMA foreign_keys=OFF;\n\n")
	}

	return header.String()
}

func (s *BackupService) exportTableSQL(tableName string) (string, error) {
	if helpers.GlobalConfig.Db.Engine == helpers.DbEnginePostgres {
		return s.exportPostgresTable(tableName)
	}
	return s.exportSqliteTable(tableName)
}

func (s *BackupService) exportPostgresTable(tableName string) (string, error) {
	var sql strings.Builder

	var createStmt string
	err := db.Db.Raw(fmt.Sprintf(`
		SELECT 'CREATE TABLE ' || tablename || ' (' || 
		string_agg(column_name || ' ' || data_type || 
		CASE WHEN character_maximum_length IS NOT NULL 
		THEN '(' || character_maximum_length || ')' ELSE '' END ||
		CASE WHEN is_nullable = 'NO' THEN ' NOT NULL' ELSE '' END, ', ') || ');'
		FROM information_schema.columns 
		WHERE table_name = '%s' AND table_schema = 'public'
		GROUP BY tablename
	`, tableName)).Scan(&createStmt).Error

	if err != nil || createStmt == "" {
		sql.WriteString(fmt.Sprintf("-- Table: %s (结构导出失败)\n", tableName))
	} else {
		sql.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;\n", tableName))
		sql.WriteString(createStmt + "\n\n")
	}

	var rows []map[string]interface{}
	if err := db.Db.Table(tableName).Find(&rows).Error; err != nil {
		return "", err
	}

	if len(rows) > 0 {
		for _, row := range rows {
			columns := make([]string, 0)
			values := make([]string, 0)
			for col, val := range row {
				columns = append(columns, col)
				values = append(values, s.formatValue(val))
			}
			sql.WriteString(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n",
				tableName, strings.Join(columns, ", "), strings.Join(values, ", ")))
		}
		sql.WriteString("\n")
	}

	return sql.String(), nil
}

func (s *BackupService) exportSqliteTable(tableName string) (string, error) {
	var sql strings.Builder

	var createStmt string
	db.Db.Raw(fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", tableName)).Scan(&createStmt)

	if createStmt == "" {
		return "", fmt.Errorf("无法获取表结构: %s", tableName)
	}

	sql.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", tableName))
	sql.WriteString(createStmt + ";\n\n")

	var rows []map[string]interface{}
	if err := db.Db.Table(tableName).Find(&rows).Error; err != nil {
		return "", err
	}

	if len(rows) > 0 {
		for _, row := range rows {
			cols := make([]string, 0)
			vals := make([]string, 0)
			for col, val := range row {
				cols = append(cols, col)
				vals = append(vals, s.formatValue(val))
			}
			sql.WriteString(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n",
				tableName, strings.Join(cols, ", "), strings.Join(vals, ", ")))
		}
		sql.WriteString("\n")
	}

	return sql.String(), nil
}

func (s *BackupService) formatValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}

	switch v := val.(type) {
	case string:
		return s.escapeSQLString(v)
	case []byte:
		return s.escapeSQLString(string(v))
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s *BackupService) escapeSQLString(str string) string {
	var result strings.Builder
	result.Grow(len(str) + 2)
	result.WriteByte('\'')

	for _, r := range str {
		switch r {
		case '\'':
			result.WriteString("''")
		case '\x00':
			result.WriteString("\\0")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\x1a':
			result.WriteString("\\Z")
		default:
			result.WriteRune(r)
		}
	}

	result.WriteByte('\'')
	return result.String()
}

func (s *BackupService) executeRestoreSQL(ctx context.Context, sqlContent string) error {
	if helpers.GlobalConfig.Db.Engine == helpers.DbEnginePostgres {
		return s.executePostgresRestore(ctx, sqlContent)
	}
	return s.executeSqliteRestore(ctx, sqlContent)
}

func (s *BackupService) executePostgresRestore(ctx context.Context, sqlContent string) error {
	statements := s.splitSQLStatements(sqlContent)

	for _, stmt := range statements {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}

		if err := db.Db.Exec(stmt).Error; err != nil {
			if !strings.Contains(err.Error(), "already exists") &&
				!strings.Contains(err.Error(), "does not exist") {
				helpers.AppLogger.Warnf("执行SQL语句失败: %v, 语句: %s", err, stmt[:min(100, len(stmt))])
			}
		}
	}

	return nil
}

func (s *BackupService) executeSqliteRestore(ctx context.Context, sqlContent string) error {
	tempFile := filepath.Join(s.backupDir, "restore_temp.sql")
	if err := os.WriteFile(tempFile, []byte(sqlContent), 0644); err != nil {
		return fmt.Errorf("写入临时SQL文件失败: %v", err)
	}
	defer os.Remove(tempFile)

	sqliteFile := filepath.Join(helpers.ConfigDir, helpers.GlobalConfig.Db.SqliteFile)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "sqlite3", sqliteFile, ".read "+tempFile)
	} else {
		cmd = exec.CommandContext(ctx, "sqlite3", sqliteFile, ".read", tempFile)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行sqlite3恢复失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

func (s *BackupService) splitSQLStatements(sqlContent string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	stringChar := rune(0)

	for _, char := range sqlContent {
		if inString {
			current.WriteRune(char)
			if char == stringChar {
				inString = false
			}
			continue
		}

		if char == '\'' {
			inString = true
			stringChar = char
			current.WriteRune(char)
			continue
		}

		if char == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt+";")
			}
			current.Reset()
			continue
		}

		current.WriteRune(char)
	}

	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

func (s *BackupService) writeCompressedFile(filePath string, data []byte) (int64, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	writer, err := zipWriter.Create("backup.sql")
	if err != nil {
		return 0, err
	}

	written, err := writer.Write(data)
	if err != nil {
		return 0, err
	}

	return int64(written), nil
}

func (s *BackupService) writeFile(filePath string, data []byte) (int64, error) {
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}

func (s *BackupService) readCompressedFile(filePath string) ([]byte, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == "backup.sql" {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, fmt.Errorf("压缩包中未找到backup.sql文件")
}

func (s *BackupService) updateRecordFailure(record *BackupRecord, reason string) {
	record.Status = BackupStatusFailed
	record.FailureReason = reason
	record.CompletedAt = time.Now().Unix()
	db.Db.Save(record)
}

func (s *BackupService) pauseAllQueues() error {
	helpers.AppLogger.Info("暂停所有任务队列...")

	if GlobalDownloadQueue != nil && GlobalDownloadQueue.IsRunning() {
		GlobalDownloadQueue.Stop()
	}
	if GlobalUploadQueue != nil && GlobalUploadQueue.IsRunning() {
		GlobalUploadQueue.Stop()
	}

	PauseSyncQueuesFunc()

	return nil
}

func (s *BackupService) resumeAllQueues() {
	helpers.AppLogger.Info("恢复所有任务队列...")

	if GlobalDownloadQueue != nil && !GlobalDownloadQueue.IsRunning() {
		GlobalDownloadQueue.Start()
	}
	if GlobalUploadQueue != nil && !GlobalUploadQueue.IsRunning() {
		GlobalUploadQueue.Start()
	}

	ResumeSyncQueuesFunc()
}

func (s *BackupService) cleanupOldBackups() {
	config := s.config

	var records []BackupRecord
	db.Db.Where("status = ?", BackupStatusCompleted).
		Order("created_at DESC").
		Find(&records)

	now := time.Now().Unix()
	retentionSeconds := int64(config.BackupRetention * 24 * 60 * 60)

	for i, record := range records {
		shouldDelete := false
		reason := ""

		if config.BackupMaxCount > 0 && i >= config.BackupMaxCount {
			shouldDelete = true
			reason = "超过最大备份数量"
		}

		if config.BackupRetention > 0 && (now-record.CreatedAt) > retentionSeconds {
			shouldDelete = true
			reason = "超过保留天数"
		}

		if shouldDelete {
			if err := s.DeleteBackup(record.ID, false); err != nil {
				helpers.AppLogger.Warnf("清理旧备份失败 ID=%d: %v, 原因: %s", record.ID, err, reason)
			} else {
				helpers.AppLogger.Infof("已清理旧备份 ID=%d, 原因: %s", record.ID, reason)
			}
		}
	}
}

func (s *BackupService) DeleteBackup(recordID uint, checkRunning bool) error {
	if checkRunning && s.IsRunning() {
		return fmt.Errorf("备份任务正在运行中，无法删除")
	}

	var record BackupRecord
	if err := db.Db.First(&record, recordID).Error; err != nil {
		return fmt.Errorf("备份记录不存在")
	}

	if record.FilePath != "" {
		if _, err := os.Stat(record.FilePath); err == nil {
			if err := os.Remove(record.FilePath); err != nil {
				helpers.AppLogger.Warnf("删除备份文件失败: %v", err)
			}
		}
	}

	if err := db.Db.Delete(&record).Error; err != nil {
		return fmt.Errorf("删除备份记录失败: %v", err)
	}

	return nil
}

func (s *BackupService) CancelBackup() error {
	if !s.IsRunning() {
		return fmt.Errorf("没有正在运行的备份任务")
	}

	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	return nil
}

func (s *BackupService) GetBackupRecords(page, pageSize int, backupType string) ([]BackupRecord, int64, error) {
	var records []BackupRecord
	var total int64

	query := db.Db.Model(&BackupRecord{})
	if backupType != "" && backupType != "all" {
		query = query.Where("backup_type = ?", backupType)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

func (s *BackupService) GetBackupConfig() *BackupConfig {
	return s.config
}

func (s *BackupService) UpdateBackupConfig(config *BackupConfig) error {
	if err := db.Db.Save(config).Error; err != nil {
		return err
	}
	s.config = config
	return nil
}

func (s *BackupService) GetBackupStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running": s.IsRunning(),
		"backup_dir": s.backupDir,
		"config":     s.config,
	}
}

func (s *BackupService) UploadAndRestore(ctx context.Context, fileData []byte, fileName string) error {
	tempPath := filepath.Join(s.backupDir, "upload_"+fileName)
	if err := os.WriteFile(tempPath, fileData, 0644); err != nil {
		return fmt.Errorf("保存上传文件失败: %v", err)
	}

	defer func() {
		os.Remove(tempPath)
	}()

	return s.RestoreBackup(ctx, 0, tempPath)
}

package db

import (
	"Q115-STRM/internal/db/database"
	"Q115-STRM/internal/helpers"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Db *gorm.DB
var Manager *database.Manager // 全局数据库管理器

// 获取一个数据库连接
func InitSqlite3(dbFile string) *gorm.DB {
	// if !helpers.PathExists(dbFile) {
	// 	return nil
	// }
	sqliteDb, err := gorm.Open(sqlite.Open(dbFile+"?cache=shared&_journal_mode=WAL&busy_timeout=30000&synchronous=NORMAL&foreign_keys=ON&cache_size=-100000"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	return sqliteDb
}

func InitPostgres(sqlDB *sql.DB) {
	// 配置Logger
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             200 * time.Millisecond, // 慢SQL阈值
			LogLevel:                  logger.Warn,            // 日志级别
			IgnoreRecordNotFoundError: true,                   // 忽略ErrRecordNotFound（记录未找到）错误
			Colorful:                  true,                   // 禁用彩色打印
		},
	)
	// 配置连接池
	sqlDB.SetMaxOpenConns(25)                  // 最多打开25个连接
	sqlDB.SetMaxIdleConns(5)                   // 最多5个空闲连接
	sqlDB.SetConnMaxLifetime(60 * time.Minute) // 连接最多使用5分钟
	sqlDB.SetConnMaxIdleTime(1 * time.Minute)  // 空闲超过10秒则关闭
	var err error
	Db, err = gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	// 设置全局Logger
	Db.Logger = newLogger
	helpers.AppLogger.Info("成功初始化数据库组件")
}

// IsPostgres 判断当前使用的是否为PostgreSQL数据库
func IsPostgres() bool {
	if Manager == nil {
		return false
	}
	mode := Manager.GetMode()
	// embedded和external模式都是PostgreSQL
	return mode == "embedded" || mode == "external"
}

// func ClearDbLock(configRootPath string) {
// 	// 检查数据库文件是否存在
// 	file1 := filepath.Join(configRootPath, "db.db-shm")
// 	file2 := filepath.Join(configRootPath, "db.db-wal")
// 	file3 := filepath.Join(configRootPath, "db.db-journal")
// 	// 检查文件是否存在
// 	if _, err := os.Stat(file1); err == nil {
// 		os.Remove(file1)
// 	}
// 	if _, err := os.Stat(file2); err == nil {
// 		os.Remove(file2)
// 	}
// 	if _, err := os.Stat(file3); err == nil {
// 		os.Remove(file3)
// 	}
// 	helpers.AppLogger.Info("已清除数据库锁文件")
// }

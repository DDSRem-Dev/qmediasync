package db

import (
	"Q115-STRM/internal/db/database"
	"Q115-STRM/internal/helpers"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/glebarez/sqlite"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Db *gorm.DB

// 获取一个数据库连接
func InitSqlite3(dbFile string) *gorm.DB {
	// if !helpers.PathExists(dbFile) {
	// 	return nil
	// }
	sqliteDb, err := gorm.Open(sqlite.Open(dbFile+"?mode=wal&cache=shared&_journal_mode=WAL&busy_timeout=30000&synchronous=NORMAL&foreign_keys=ON&cache_size=-100000"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	sqlDB, dbError := sqliteDb.DB()
	if dbError != nil {
		return nil
	}

	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	sqlDB.SetMaxIdleConns(10)

	// SetMaxOpenConns 设置打开数据库连接的最大数量。
	sqlDB.SetMaxOpenConns(100)
	return sqliteDb
}

// 连接外部PostgreSQL数据库
func ConnectPostgres(dbConfig *database.Config) error {
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

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.DBName, dbConfig.SSLMode)
	helpers.AppLogger.Infof("连接数据库: %s", connStr)
	sqlDB, cerr := sql.Open("postgres", connStr)
	if cerr != nil {
		helpers.AppLogger.Errorf("连接数据库失败: %v", cerr)
		return cerr
	}
	// 配置连接池
	sqlDB.SetMaxOpenConns(dbConfig.MaxOpenConns) // 最多打开25个连接
	sqlDB.SetMaxIdleConns(dbConfig.MaxIdleConns) // 最多5个空闲连接
	sqlDB.SetConnMaxLifetime(60 * time.Minute)   // 连接最多使用60分钟
	sqlDB.SetConnMaxIdleTime(1 * time.Minute)    // 空闲超过1分钟则关闭
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

	return nil
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
	return helpers.GlobalConfig.Db.Engine == helpers.DbEnginePostgres
}

// getPostgresBinaryPath 获取PostgreSQL二进制路径
func GetPostgresBinaryPath(embeddedBasePath string) string {
	if helpers.IsRunningInDocker() {
		return "" // Docker 容器中的路径
	}
	// 根据平台返回二进制路径
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var binDir string
	switch goos {
	case "windows":
		binDir = filepath.Join(embeddedBasePath, "windows", goarch, "bin")
	default:
		return ""
	}
	return binDir
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

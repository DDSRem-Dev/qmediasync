package backup

import (
	"database/sql"
	"io"
)

// DatabaseDriver 数据库驱动接口
type DatabaseDriver interface {
	// 获取所有表名
	GetAllTables() ([]string, error)

	// 清空所有表数据（保留表结构）
	TruncateAllTables() error

	// 禁用约束
	DisableConstraints() error

	// 启用约束
	EnableConstraints() error

	// 导出数据到SQL
	ExportToSQL(writer io.Writer) (tableCount int, dbSize int64, err error)

	// 从SQL导入数据
	ImportFromSQL(reader io.Reader) error

	// 获取数据库大小
	GetDatabaseSize() (int64, error)
}

// DriverFactory 驱动工厂
type DriverFactory struct {
	dbType string // "postgres", "sqlite", 等
	sqlDB  *sql.DB
}

func NewDriverFactory(dbType string, sqlDB *sql.DB) *DriverFactory {
	return &DriverFactory{
		dbType: dbType,
		sqlDB:  sqlDB,
	}
}

func (f *DriverFactory) CreateDriver() DatabaseDriver {
	switch f.dbType {
	case "postgres":
		return NewPostgresDriver(f.sqlDB)
	case "sqlite":
		return NewSQLiteDriver(f.sqlDB)
	default:
		return NewPostgresDriver(f.sqlDB) // 默认使用PostgreSQL
	}
}

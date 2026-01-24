package backup

import (
	"database/sql"
	"fmt"
	"io"
	"time"
)

type SQLiteDriver struct {
	sqlDB *sql.DB
}

func NewSQLiteDriver(sqlDB *sql.DB) *SQLiteDriver {
	return &SQLiteDriver{sqlDB: sqlDB}
}

func (d *SQLiteDriver) GetAllTables() ([]string, error) {
	var tables []string
	rows, err := d.sqlDB.Query(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}

// TruncateAllTables 使用 DELETE 清空所有SQLite表数据（DELETE等同于TRUNCATE）
func (d *SQLiteDriver) TruncateAllTables() error {
	// SQLite：禁用外键检查，清空表
	if _, err := d.sqlDB.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}

	tables, err := d.GetAllTables()
	if err != nil {
		d.sqlDB.Exec("PRAGMA foreign_keys = ON")
		return err
	}

	for _, tableName := range tables {
		if _, err := d.sqlDB.Exec(fmt.Sprintf(`DELETE FROM "%s"`, tableName)); err != nil {
			d.sqlDB.Exec("PRAGMA foreign_keys = ON")
			return fmt.Errorf("清空表 %s 失败: %v", tableName, err)
		}
	}

	// 重新启用外键检查
	_, _ = d.sqlDB.Exec("PRAGMA foreign_keys = ON")
	return nil
}

func (d *SQLiteDriver) DisableConstraints() error {
	_, err := d.sqlDB.Exec("PRAGMA foreign_keys = OFF")
	return err
}

func (d *SQLiteDriver) EnableConstraints() error {
	_, err := d.sqlDB.Exec("PRAGMA foreign_keys = ON")
	return err
}

func (d *SQLiteDriver) ExportToSQL(writer io.Writer) (int, int64, error) {
	tables, err := d.GetAllTables()
	if err != nil {
		return 0, 0, err
	}

	fmt.Fprintf(writer, "-- SQLite Database Backup\n")
	fmt.Fprintf(writer, "-- Generated at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(writer, "-- Tables: %d\n\n", len(tables))

	for _, tableName := range tables {
		if err := d.exportTableData(writer, tableName); err != nil {
			fmt.Fprintf(writer, "-- Error exporting table %s: %v\n\n", tableName, err)
			continue
		}
	}

	// SQLite不支持获取数据库大小
	return len(tables), 0, nil
}

func (d *SQLiteDriver) exportTableData(writer io.Writer, tableName string) error {
	// SQLite数据导出实现
	// 类似PostgreSQL的实现逻辑
	return nil
}

func (d *SQLiteDriver) ImportFromSQL(reader io.Reader) error {
	content := make([]byte, 0)
	buffer := make([]byte, 1024*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			content = append(content, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}

	_, err := d.sqlDB.Exec(string(content))
	return err
}

func (d *SQLiteDriver) GetDatabaseSize() (int64, error) {
	// SQLite返回0
	return 0, nil
}

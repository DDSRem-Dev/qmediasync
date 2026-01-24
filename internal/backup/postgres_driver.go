package backup

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

type PostgresDriver struct {
	sqlDB *sql.DB
}

func NewPostgresDriver(sqlDB *sql.DB) *PostgresDriver {
	return &PostgresDriver{sqlDB: sqlDB}
}

func (d *PostgresDriver) GetAllTables() ([]string, error) {
	var tables []string
	rows, err := d.sqlDB.Query(`
		SELECT table_name FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
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

// TruncateAllTables 使用 TRUNCATE 清空所有表数据（保留表结构）
func (d *PostgresDriver) TruncateAllTables() error {
	tables, err := d.GetAllTables()
	if err != nil {
		return err
	}

	if len(tables) == 0 {
		return nil
	}

	// PostgreSQL：TRUNCATE 需要在禁用外键约束的情况下执行
	// 或使用 CASCADE 选项自动处理外键依赖
	for _, tableName := range tables {
		if _, err := d.sqlDB.Exec(fmt.Sprintf(`TRUNCATE TABLE "%s" CASCADE`, tableName)); err != nil {
			return fmt.Errorf("清空表 %s 失败: %v", tableName, err)
		}
	}
	return nil
}

func (d *PostgresDriver) DisableConstraints() error {
	_, err := d.sqlDB.Exec("SET session_replication_role = replica")
	return err
}

func (d *PostgresDriver) EnableConstraints() error {
	_, err := d.sqlDB.Exec("SET session_replication_role = default")
	return err
}

func (d *PostgresDriver) ExportToSQL(writer io.Writer) (int, int64, error) {
	tables, err := d.GetAllTables()
	if err != nil {
		return 0, 0, err
	}

	// 写入头信息
	fmt.Fprintf(writer, "-- PostgreSQL Database Backup\n")
	fmt.Fprintf(writer, "-- Generated at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(writer, "-- Tables: %d\n\n", len(tables))

	// 导出每个表的数据（表结构已存在）
	for _, tableName := range tables {
		if err := d.exportTableData(writer, tableName); err != nil {
			// 记录错误但继续处理其他表
			fmt.Fprintf(writer, "-- Error exporting table %s: %v\n\n", tableName, err)
			continue
		}
	}

	dbSize, _ := d.GetDatabaseSize()
	return len(tables), dbSize, nil
}

func (d *PostgresDriver) exportTableData(writer io.Writer, tableName string) error {
	rows, err := d.sqlDB.Query(fmt.Sprintf(`SELECT * FROM "%s"`, tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	if len(columns) == 0 {
		return nil
	}

	fmt.Fprintf(writer, "-- Table: %s\n", tableName)

	// 构建INSERT语句前缀
	columnNames := make([]string, len(columns))
	for i, col := range columns {
		columnNames[i] = fmt.Sprintf(`"%s"`, col)
	}
	insertPrefix := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES `, tableName, strings.Join(columnNames, ", "))

	// 导出数据
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// 构建值
		valueParts := make([]string, len(values))
		for i, val := range values {
			valueParts[i] = d.formatValue(val)
		}

		fmt.Fprintf(writer, "%s(%s);\n", insertPrefix, strings.Join(valueParts, ", "))
	}

	fmt.Fprint(writer, "\n")
	return nil
}

func (d *PostgresDriver) formatValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	if b, ok := val.([]byte); ok {
		escapedStr := strings.ReplaceAll(string(b), "'", "''")
		return fmt.Sprintf("'%s'", escapedStr)
	}
	if t, ok := val.(time.Time); ok {
		return fmt.Sprintf("'%s'", t.Format(time.RFC3339Nano))
	}

	valStr := fmt.Sprintf("%v", val)
	// 尝试转换为数字
	if _, err := fmt.Sscanf(valStr, "%f", new(float64)); err == nil {
		return valStr
	}
	// 字符串：转义单引号
	escapedStr := strings.ReplaceAll(valStr, "'", "''")
	return fmt.Sprintf("'%s'", escapedStr)
}

func (d *PostgresDriver) ImportFromSQL(reader io.Reader) error {
	// 读取SQL内容
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

func (d *PostgresDriver) GetDatabaseSize() (int64, error) {
	var size int64
	err := d.sqlDB.QueryRow("SELECT pg_database_size(current_database())").Scan(&size)
	return size, err
}

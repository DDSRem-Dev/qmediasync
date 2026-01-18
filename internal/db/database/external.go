package database

import (
	"Q115-STRM/internal/helpers"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "gorm.io/driver/postgres"
)

type ExternalManager struct {
	config       *Config
	db           *sql.DB
	process      *os.Process
	userSwitcher *UserSwitcher
}

func NewExternalManager(config *Config) *ExternalManager {
	return &ExternalManager{
		config:       config,
		userSwitcher: NewUserSwitcher(),
	}
}

func (m *ExternalManager) Start(ctx context.Context) error {
	helpers.AppLogger.Info("连接外部 PostgreSQL...")
	if !m.config.External {
		// 准备数据目录
		if err := m.prepareDataDir(); err != nil {
			return err
		}

		// 初始化数据库
		if err := m.initDatabase(); err != nil {
			return err
		}

		// 启动 PostgreSQL 进程
		if err := m.startPostgresProcess(); err != nil {
			return err
		}
		// 等待数据库可用
		if err := m.waitForPostgres(ctx); err != nil {
			return err
		}
	}
	// 连接数据库
	return m.connectToDB()
}

func (m *ExternalManager) Stop() error {
	helpers.AppLogger.Info("停止 Docker 环境中的 PostgreSQL...")

	if m.process != nil {
		// 使用 pg_ctl 优雅停止，使用qms用户执行
		m.userSwitcher.RunCommandAsUser(
			fmt.Sprintf("pg_ctl stop -D %s -m fast", m.config.DataDir),
		)
		time.Sleep(2 * time.Second)
	}

	if m.db != nil {
		m.db.Close()
	}

	return nil
}

func (m *ExternalManager) GetDB() *sql.DB {
	return m.db
}

func (m *ExternalManager) InitDataDir() {
	configDir := filepath.Dir(filepath.Dir(m.config.DataDir))
	// 更改configDir的目录所有者
	exec.Command("chown", "-R", "qms:qms", configDir).Run() // 设置目录所有者为qms:qms
	exec.Command("chmod", "777", configDir).Run()           // 设置目录权限为777
	postgresRoot := filepath.Dir(m.config.DataDir)
	if !helpers.PathExists(postgresRoot) {
		os.MkdirAll(postgresRoot, 0755)      // 如果没有config目录则创建
		os.Chown(postgresRoot, 12331, 12331) // 设置目录所有者为12331:12331
		helpers.AppLogger.Infof("创建Postgres目录 %s 成功", postgresRoot)
	} else {
		// os.Chown(postgresRoot, 12331, 12331)                       // 设置目录所有者为12331:12331
		exec.Command("chown", "-R", "qms:qms", postgresRoot).Run() // 设置目录所有者为qms:qms
		exec.Command("chmod", "-R", "750", postgresRoot).Run()     // 设置目录权限为750
		helpers.AppLogger.Infof("设置Postgres目录 %s 权限为750成功", postgresRoot)
	}
	dataDir := filepath.Join(postgresRoot, "data")
	if !helpers.PathExists(dataDir) {
		os.MkdirAll(dataDir, 0750)      // 如果没有data目录则创建
		os.Chown(dataDir, 12331, 12331) // 设置目录所有者为12331:12331
		helpers.AppLogger.Infof("创建Postgres数据目录 %s 成功", dataDir)
	} else {
		exec.Command("chown", "-R", "qms:qms", dataDir).Run() // 设置目录所有者为qms:qms
		exec.Command("chmod", "-R", "750", dataDir).Run()     // 设置目录权限为750
		helpers.AppLogger.Infof("设置Postgres数据目录 %s 权限为750成功", dataDir)
	}
	postmasterFile := filepath.Join(dataDir, "postmaster.pid")
	if helpers.PathExists(postmasterFile) {
		os.Remove(postmasterFile)
		helpers.AppLogger.Infof("删除Postgres postmaster.pid 文件 %s 成功", postmasterFile)
	}
	logDir := filepath.Join(postgresRoot, "log")
	if helpers.PathExists(logDir) {
		os.RemoveAll(logDir)
		helpers.AppLogger.Infof("删除Postgres日志目录 %s 成功", logDir)
	}
	os.MkdirAll(logDir, 0755)      // 如果没有log目录则创建
	os.Chown(logDir, 12331, 12331) // 设置目录所有者为12331:12331
	helpers.AppLogger.Infof("创建Postgres日志目录 %s 成功", logDir)
	tmpDir := filepath.Join(postgresRoot, "tmp")
	if helpers.PathExists(tmpDir) {
		os.RemoveAll(tmpDir)
		helpers.AppLogger.Infof("删除Postgres临时目录 %s 成功", tmpDir)
	}
	os.MkdirAll(tmpDir, 0755)      // 如果没有tmp目录则创建
	os.Chown(tmpDir, 12331, 12331) // 设置目录所有者为12331:12331
	helpers.AppLogger.Infof("创建Postgres临时目录 %s 成功", tmpDir)
}
func (m *ExternalManager) prepareDataDir() error {
	m.InitDataDir() // 初始化数据目录
	// 检查是否已经初始化
	pgVersionFile := filepath.Join(m.config.DataDir, "PG_VERSION")
	if helpers.PathExists(pgVersionFile) {
		return nil
	}
	// 使用 qms 用户初始化数据库
	output, err := m.userSwitcher.RunCommandAsUser("initdb", "-D", m.config.DataDir, "-U", m.config.User, "--encoding=UTF8", "--locale=C", "--auth=trust")
	if err != nil {
		return fmt.Errorf("数据库用户初始化失败: %v, 输出: %s", err, output)
	}
	helpers.AppLogger.Info("数据库初始化完成")
	return nil
}

// 添加路径处理函数
func (m *ExternalManager) formatPathForPostgres(path string) string {
	if runtime.GOOS == "windows" {
		// Windows 中 PostgreSQL 配置需要正斜杠或双反斜杠
		// 将路径转换为 Windows 可识别的格式
		path = filepath.Clean(path)

		// 方法1: 使用正斜杠（推荐，跨平台兼容）
		path = strings.ReplaceAll(path, "\\", "/")

		// 或者方法2: 使用双反斜杠
		// path = strings.ReplaceAll(path, "\\", "\\\\")

		// 如果路径包含空格，确保正确转义
		if strings.Contains(path, " ") {
			path = "\"" + path + "\""
		}
	}
	return path
}

func (m *ExternalManager) initDatabase() error {
	// 检测操作系统并选择合适的共享内存类型
	sharedMemoryType := m.getSharedMemoryType()
	// 配置 postgresql.conf
	confPath := filepath.Join(m.config.DataDir, "postgresql.conf")
	confContent := fmt.Sprintf(`
# 基本配置
listen_addresses = '%s'
port = %d
max_connections = 100
shared_buffers = 128MB
dynamic_shared_memory_type = %s
unix_socket_directories = '%s'

# 日志配置
log_destination = 'stderr'
logging_collector = on
log_directory = '%s'
log_filename = 'postgres.log'
log_file_mode = 0644
log_rotation_age = 1d
log_rotation_size = 100MB
log_truncate_on_rotation = on
log_min_error_statement = error
log_min_duration_statement = -1
log_checkpoints = on
log_connections = on
log_disconnections = on
log_duration = on
log_line_prefix = '%%t [%%p]: [%%l-1] user=%%u,db=%%d,app=%%a,client=%%h '
log_timezone = 'UTC'
log_autovacuum_min_duration = 0

# 性能相关
wal_level = replica
max_wal_senders = 10
checkpoint_timeout = 10min
checkpoint_completion_target = 0.9

# 内存配置
work_mem = 4MB
maintenance_work_mem = 64MB
effective_cache_size = 1GB

# 其他优化
max_worker_processes = 8
max_parallel_workers_per_gather = 2
max_parallel_workers = 8
`, m.config.Host, m.config.Port, sharedMemoryType, m.formatPathForPostgres(m.config.DataDir), m.formatPathForPostgres(m.config.LogDir))

	if err := os.WriteFile(confPath, []byte(strings.TrimSpace(confContent)), 0750); err != nil {
		return fmt.Errorf("写入 postgresql.conf 失败: %v", err)
	}
	// 改变所有者
	os.Chown(confPath, 12331, 12331) // 设置文件所有者为12331:12331
	// 配置 pg_hba.conf（保持不变）
	hbaPath := filepath.Join(m.config.DataDir, "pg_hba.conf")
	hbaContent := `
# PostgreSQL Client Authentication Configuration File
local   all             all                                     trust
host    all             all             127.0.0.1/32            trust
host    all             all             ::1/128                 trust
`
	if err := os.WriteFile(hbaPath, []byte(strings.TrimSpace(hbaContent)), 0750); err != nil {
		return fmt.Errorf("写入 pg_hba.conf 失败: %v", err)
	}
	// 改变所有者
	os.Chown(hbaPath, 12331, 12331) // 设置文件所有者为12331:12331

	helpers.AppLogger.Infof("PostgreSQL 配置完成，共享内存类型: %s", sharedMemoryType)
	return nil
}

func (m *ExternalManager) startPostgresProcess() error {
	tmpPath := filepath.Join(filepath.Dir(m.config.DataDir), "tmp")

	postgreStdLog := filepath.Join(m.config.LogDir, "postgres-console.log")
	// 使用su命令在后台以qms用户启动PostgreSQL
	// 使用pg_ctl启动PostgreSQL
	// cmd, err := m.userSwitcher.RunCommandAsUserWithEnv(
	// 	postgreStdLog,
	// 	"qms",
	// 	map[string]string{
	// 		"PGDATA": m.config.DataDir,
	// 		"PGPORT": fmt.Sprintf("%d", m.config.Port),
	// 	},
	// 	fmt.Sprintf("postgres -D $PGDATA -k %s -c unix_socket_directories='%s' 2>&1 & echo $!", tmpPath, tmpPath),
	// )
	cmd, err := m.userSwitcher.RunCommandAsUserWithEnv(
		postgreStdLog,
		map[string]string{
			"PGDATA": m.config.DataDir,
			"PGPORT": fmt.Sprintf("%d", m.config.Port),
		},
		fmt.Sprintf("pg_ctl start -D $PGDATA -o \"-k %s -c unix_socket_directories='%s'\" 2>&1 & echo $!", tmpPath, tmpPath),
	)
	// pg_ctl start -D /app/config/postgres/data -o "-k /app/config/postgres/tmp -c unix_socket_directories='/app/config/postgres/tmp'"
	// su - qms -c "postgres -D /app/config/postgres/data -k /app/config/postgres/tmp -c unix_socket_directories='/app/config/postgres/tmp'"
	if err != nil {
		helpers.AppLogger.Errorf("启动 PostgreSQL 失败: %v", err)
		return fmt.Errorf("启动 PostgreSQL 失败: %v", err)
	}
	m.process = cmd.Process
	// m.process = process
	helpers.AppLogger.Infof("PostgreSQL 进程已启动 (PID: %d)", m.process.Pid)

	return nil
}

func (m *ExternalManager) HealthCheck() error {
	if m.db == nil {
		return fmt.Errorf("数据库未连接")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return m.db.PingContext(ctx)
}

func (m *ExternalManager) Backup(ctx context.Context, backupPath string) error {
	// 在 Docker 模式下，备份通过 exec 进入容器执行
	helpers.AppLogger.Infof("Docker 模式备份到: %s", backupPath)
	// 实际实现会调用 docker exec 执行 pg_dump
	return nil
}

func (m *ExternalManager) Restore(ctx context.Context, backupPath string) error {
	// 在 Docker 模式下，恢复通过 exec 进入容器执行
	helpers.AppLogger.Infof("Docker 模式从备份恢复: %s", backupPath)
	// 实际实现会调用 docker exec 执行 psql
	return nil
}

func (m *ExternalManager) waitForPostgres(ctx context.Context) error {
	helpers.AppLogger.Infof("等待 PostgreSQL 在 %s:%d 启动...", m.config.Host, m.config.Port)

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("等待 PostgreSQL 启动超时")
		case <-ticker.C:
			cmd := exec.Command("pg_isready", "-h", m.config.Host, "-p",
				fmt.Sprintf("%d", m.config.Port), "-U", m.config.User)
			if err := cmd.Run(); err == nil {
				helpers.AppLogger.Info("PostgreSQL 已就绪")
				return nil
			} else {
				helpers.AppLogger.Infof("PostgreSQL 启动中... 错误: %v", err)
			}
		}
	}
}

func (m *ExternalManager) connectToDB() error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		m.config.Host, m.config.Port, m.config.User, m.config.Password, m.config.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %v", err)
	}

	// 测试连接
	if derr := db.Ping(); derr != nil {
		db.Close()
		return fmt.Errorf("数据库连接测试失败: %v", derr)
	}

	m.db = db

	// 创建应用数据库
	if cerr := m.createAppDatabase(); cerr != nil {
		return cerr
	}

	// 重新连接到应用数据库
	connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		m.config.Host, m.config.Port, m.config.User, m.config.Password, m.config.DBName, m.config.SSLMode)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("连接到应用数据库失败: %v", err)
	}

	m.db = db
	helpers.AppLogger.Info("成功连接到嵌入式数据库")

	return nil
}

// 根据操作系统选择合适的共享内存类型
func (m *ExternalManager) getSharedMemoryType() string {
	return "sysv"
}

func (m *ExternalManager) createAppDatabase() error {
	var exists bool
	err := m.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pg_database WHERE datname = $1
		)`, m.config.DBName).Scan(&exists)

	if err != nil {
		return fmt.Errorf("检查数据库存在性失败: %v", err)
	}

	if !exists {
		helpers.AppLogger.Infof("创建数据库: %s", m.config.DBName)
		_, err = m.db.Exec(fmt.Sprintf("CREATE DATABASE %s", m.config.DBName))
		if err != nil {
			helpers.AppLogger.Errorf("创建数据库失败: %v\n", err)
		}
		helpers.AppLogger.Info("数据库创建成功")
	}

	return nil
}

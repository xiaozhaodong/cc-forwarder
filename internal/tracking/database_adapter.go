package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DatabaseAdapter 定义数据库操作接口
// 抽象SQLite和MySQL的差异，让上层代码无需关心具体实现
type DatabaseAdapter interface {
	// 基础连接管理
	Open() error
	Close() error
	Ping(ctx context.Context) error

	// 获取数据库连接
	GetDB() *sql.DB
	GetReadDB() *sql.DB
	GetWriteDB() *sql.DB

	// 事务支持
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

	// 数据库初始化
	InitSchema() error

	// SQL语法适配 - 处理SQLite和MySQL的语法差异
	BuildInsertOrReplaceQuery(table string, columns []string, values []string) string
	BuildDateTimeNow() string
	BuildLimitOffset(limit, offset int) string

	// 数据库特定操作
	VacuumDatabase(ctx context.Context) error
	GetDatabaseStats(ctx context.Context) (*DatabaseStats, error)

	// 连接统计
	GetConnectionStats() ConnectionStats

	// 类型标识
	GetDatabaseType() string
}

// DatabaseConfig 统一数据库配置结构
type DatabaseConfig struct {
	// 数据库类型
	Type string `yaml:"type"` // "sqlite" | "mysql"

	// SQLite配置（向后兼容）
	DatabasePath string `yaml:"database_path,omitempty"`

	// MySQL配置
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Database string `yaml:"database,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// 连接池配置（简化版，适合本地使用）
	MaxOpenConns    int           `yaml:"max_open_conns,omitempty"`
	MaxIdleConns    int           `yaml:"max_idle_conns,omitempty"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime,omitempty"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time,omitempty"`

	// MySQL特定配置
	Charset  string `yaml:"charset,omitempty"`
	Timezone string `yaml:"timezone,omitempty"`
}

// ConnectionStats 连接池统计信息
type ConnectionStats struct {
	OpenConnections int           `json:"open_connections"`
	IdleConnections int           `json:"idle_connections"`
	InUseConnections int          `json:"in_use_connections"`
	WaitCount       int64         `json:"wait_count"`
	WaitDuration    time.Duration `json:"wait_duration"`
	MaxLifetime     time.Duration `json:"max_lifetime"`
}

// NewDatabaseAdapter 数据库适配器工厂函数
func NewDatabaseAdapter(config DatabaseConfig) (DatabaseAdapter, error) {
	// 确定数据库类型
	dbType := getDatabaseType(config)

	switch dbType {
	case "sqlite":
		return NewSQLiteAdapter(config)
	case "mysql":
		return NewMySQLAdapter(config)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// getDatabaseType 从配置推断数据库类型
func getDatabaseType(config DatabaseConfig) string {
	// 1. 优先使用明确配置的类型
	if config.Type != "" {
		return config.Type
	}

	// 2. 根据配置内容推断类型
	if config.Host != "" || config.Database != "" {
		return "mysql"
	}

	// 3. 默认为SQLite（向后兼容）
	return "sqlite"
}

// setDefaultConfig 设置数据库配置默认值
func setDefaultConfig(config *DatabaseConfig) {
	switch config.Type {
	case "mysql":
		// MySQL默认配置
		if config.Port == 0 {
			config.Port = 3306
		}
		if config.MaxOpenConns == 0 {
			config.MaxOpenConns = 10 // 本地使用足够
		}
		if config.MaxIdleConns == 0 {
			config.MaxIdleConns = 5
		}
		if config.ConnMaxLifetime == 0 {
			config.ConnMaxLifetime = time.Hour
		}
		if config.ConnMaxIdleTime == 0 {
			config.ConnMaxIdleTime = 10 * time.Minute
		}
		if config.Charset == "" {
			config.Charset = "utf8mb4"
		}
		if config.Timezone == "" {
			config.Timezone = "Asia/Shanghai"
		}
	case "sqlite", "":
		// SQLite配置保持原有逻辑
		if config.DatabasePath == "" {
			config.DatabasePath = "data/usage.db"
		}
	}
}
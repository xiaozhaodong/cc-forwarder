package tracking

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed mysql_schema.sql
var mysqlSchemaFS embed.FS

// MySQLAdapter MySQL数据库适配器实现
type MySQLAdapter struct {
	config DatabaseConfig
	db     *sql.DB
	logger *slog.Logger
}

// NewMySQLAdapter 创建MySQL适配器实例
func NewMySQLAdapter(config DatabaseConfig) (*MySQLAdapter, error) {
	// 设置默认配置
	setDefaultConfig(&config)

	adapter := &MySQLAdapter{
		config: config,
		logger: slog.Default(),
	}

	return adapter, nil
}

// Open 建立MySQL数据库连接
func (m *MySQLAdapter) Open() error {
	dsn, err := m.buildDSN()
	if err != nil {
		return fmt.Errorf("failed to build DSN: %w", err)
	}

	m.logger.Info("正在连接MySQL数据库",
		"host", m.config.Host,
		"database", m.config.Database,
		"charset", m.config.Charset)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(m.config.MaxOpenConns)
	db.SetMaxIdleConns(m.config.MaxIdleConns)
	db.SetConnMaxLifetime(m.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(m.config.ConnMaxIdleTime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping MySQL database: %w", err)
	}

	// 尝试设置会话时区，使用降级策略
	m.trySetSessionTimezone(ctx, db)

	// 诊断时区设置结果
	m.diagnoseTimezoneSettings(ctx, db)

	m.db = db
	m.logger.Info("✅ MySQL数据库连接成功",
		"max_open_conns", m.config.MaxOpenConns,
		"max_idle_conns", m.config.MaxIdleConns)

	return nil
}

// buildDSN 构建MySQL连接字符串
func (m *MySQLAdapter) buildDSN() (string, error) {
	if m.config.Host == "" {
		return "", fmt.Errorf("MySQL host is required")
	}
	if m.config.Database == "" {
		return "", fmt.Errorf("MySQL database name is required")
	}
	if m.config.Username == "" {
		return "", fmt.Errorf("MySQL username is required")
	}

	// 构建基础DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		m.config.Username,
		m.config.Password,
		m.config.Host,
		m.config.Port,
		m.config.Database)

	// 添加参数
	params := url.Values{}
	params.Add("charset", m.config.Charset)
	params.Add("parseTime", "true")

	// 时区处理：对于DSN连接，优先使用最兼容的格式
	timezone := m.getDSNCompatibleTimezone()
	if timezone != "" {
		params.Add("loc", timezone)
	}

	// MySQL 5.7/8.0兼容性参数
	params.Add("sql_mode", "'STRICT_TRANS_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO'")
	params.Add("timeout", "30s")
	params.Add("readTimeout", "30s")
	params.Add("writeTimeout", "30s")

	dsn += "?" + params.Encode()
	return dsn, nil
}

// Close 关闭数据库连接
func (m *MySQLAdapter) Close() error {
	if m.db != nil {
		m.logger.Info("正在关闭MySQL数据库连接")
		return m.db.Close()
	}
	return nil
}

// Ping 测试数据库连接
func (m *MySQLAdapter) Ping(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("database not connected")
	}
	return m.db.PingContext(ctx)
}

// GetDB 获取数据库连接（单一连接，不需要读写分离）
func (m *MySQLAdapter) GetDB() *sql.DB {
	return m.db
}

// GetReadDB 获取读数据库连接（与写连接相同）
func (m *MySQLAdapter) GetReadDB() *sql.DB {
	return m.db
}

// GetWriteDB 获取写数据库连接（与读连接相同）
func (m *MySQLAdapter) GetWriteDB() *sql.DB {
	return m.db
}

// BeginTx 开始事务
func (m *MySQLAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return m.db.BeginTx(ctx, opts)
}

// InitSchema 初始化MySQL数据库Schema
func (m *MySQLAdapter) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.logger.Info("正在初始化MySQL数据库Schema")

	// 执行MySQL版本的Schema
	schema := m.getMySQLSchema()

	// 分割SQL语句执行
	statements := m.splitSQLStatements(schema)

	for i, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}

		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			m.logger.Error("执行Schema语句失败",
				"statement_index", i,
				"error", err,
				"sql", stmt[:min(100, len(stmt))])
			return fmt.Errorf("failed to execute schema statement %d: %w", i, err)
		}
	}

	m.logger.Info("✅ MySQL数据库Schema初始化完成")
	return nil
}

// BuildInsertOrReplaceQuery 构建插入或更新查询（MySQL语法）
func (m *MySQLAdapter) BuildInsertOrReplaceQuery(table string, columns []string, values []string) string {
	// MySQL使用 ON DUPLICATE KEY UPDATE 语法
	columnsStr := strings.Join(columns, ", ")
	valuesStr := strings.Join(values, ", ")

	// 构建更新部分，对start_time字段进行特殊处理
	var updateParts []string
	for _, col := range columns {
		if col != "id" && col != "request_id" { // 跳过主键和唯一键
			if col == "start_time" {
				// 对start_time使用COALESCE，只在原值为NULL时才更新
				updateParts = append(updateParts, fmt.Sprintf("%s = COALESCE(%s, VALUES(%s))", col, col, col))
			} else {
				updateParts = append(updateParts, fmt.Sprintf("%s = VALUES(%s)", col, col))
			}
		}
	}

	updateStr := strings.Join(updateParts, ", ")

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
		table, columnsStr, valuesStr, updateStr)
}

// BuildDateTimeNow 返回当前时间函数（支持微秒精度）
func (m *MySQLAdapter) BuildDateTimeNow() string {
	return "NOW(6)"
}

// BuildLimitOffset 构建分页查询
func (m *MySQLAdapter) BuildLimitOffset(limit, offset int) string {
	if limit <= 0 {
		return ""
	}
	if offset <= 0 {
		return fmt.Sprintf(" LIMIT %d", limit)
	}
	return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
}

// VacuumDatabase MySQL没有VACUUM操作，执行OPTIMIZE TABLE
func (m *MySQLAdapter) VacuumDatabase(ctx context.Context) error {
	m.logger.Info("正在优化MySQL表结构")

	tables := []string{"request_logs", "usage_summary"}
	for _, table := range tables {
		query := fmt.Sprintf("OPTIMIZE TABLE %s", table)
		if _, err := m.db.ExecContext(ctx, query); err != nil {
			m.logger.Warn("表优化失败", "table", table, "error", err)
			// 不返回错误，因为OPTIMIZE TABLE失败不是致命问题
		}
	}

	m.logger.Info("✅ MySQL表结构优化完成")
	return nil
}

// GetDatabaseStats 获取MySQL数据库统计信息
func (m *MySQLAdapter) GetDatabaseStats(ctx context.Context) (*DatabaseStats, error) {
	stats := &DatabaseStats{}

	// 获取请求记录总数
	err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_logs").Scan(&stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests count: %w", err)
	}

	// 获取汇总记录总数
	err = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_summary").Scan(&stats.TotalSummaries)
	if err != nil {
		return nil, fmt.Errorf("failed to get total summaries count: %w", err)
	}

	// 获取最早和最新的记录时间
	var earliestStr, latestStr sql.NullString
	err = m.db.QueryRowContext(ctx, "SELECT MIN(start_time), MAX(start_time) FROM request_logs").Scan(&earliestStr, &latestStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get record time range: %w", err)
	}

	if earliestStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", earliestStr.String); err == nil {
			stats.EarliestRecord = &t
		}
	}
	if latestStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", latestStr.String); err == nil {
			stats.LatestRecord = &t
		}
	}

	// 获取数据库大小（MySQL特有查询）
	var dataLength, indexLength sql.NullInt64
	query := `SELECT
		SUM(data_length) as data_length,
		SUM(index_length) as index_length
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`

	err = m.db.QueryRowContext(ctx, query).Scan(&dataLength, &indexLength)
	if err == nil {
		stats.DatabaseSize = dataLength.Int64 + indexLength.Int64
	}

	// 获取总成本
	err = m.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(total_cost_usd), 0) FROM request_logs WHERE total_cost_usd > 0").Scan(&stats.TotalCostUSD)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}

	return stats, nil
}

// GetConnectionStats 获取连接池统计信息
func (m *MySQLAdapter) GetConnectionStats() ConnectionStats {
	if m.db == nil {
		return ConnectionStats{}
	}

	dbStats := m.db.Stats()
	return ConnectionStats{
		OpenConnections:  dbStats.OpenConnections,
		IdleConnections:  dbStats.Idle,
		InUseConnections: dbStats.InUse,
		WaitCount:        dbStats.WaitCount,
		WaitDuration:     dbStats.WaitDuration,
		MaxLifetime:      m.config.ConnMaxLifetime,
	}
}


// GetDatabaseType 返回数据库类型标识
func (m *MySQLAdapter) GetDatabaseType() string {
	return "mysql"
}

// getMySQLSchema 获取MySQL数据库Schema
func (m *MySQLAdapter) getMySQLSchema() string {
	schema, err := mysqlSchemaFS.ReadFile("mysql_schema.sql")
	if err != nil {
		m.logger.Error("读取MySQL Schema文件失败", "error", err)
		// 返回硬编码的基础schema作为fallback
		return m.getFallbackSchema()
	}

	if len(schema) == 0 {
		m.logger.Warn("MySQL Schema文件为空，使用fallback schema")
		return m.getFallbackSchema()
	}

	m.logger.Debug("成功读取MySQL Schema", "length", len(schema))
	return string(schema)
}

// getFallbackSchema 获取硬编码的基础Schema
func (m *MySQLAdapter) getFallbackSchema() string {
	return `
-- MySQL基础Schema（fallback版本）
CREATE TABLE IF NOT EXISTS request_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    request_id VARCHAR(255) UNIQUE NOT NULL COMMENT 'req-xxxxxxxx',
    client_ip VARCHAR(45) COMMENT '客户端IP',
    user_agent TEXT COMMENT '客户端User-Agent',
    method VARCHAR(10) DEFAULT 'POST' COMMENT 'HTTP方法',
    path VARCHAR(255) DEFAULT '/v1/messages' COMMENT '请求路径',
    start_time DATETIME(6) NOT NULL COMMENT '请求开始时间',
    end_time DATETIME(6) COMMENT '请求完成时间',
    duration_ms BIGINT COMMENT '总耗时(毫秒)',
    endpoint_name VARCHAR(255) COMMENT '端点名称',
    group_name VARCHAR(255) COMMENT '组名称',
    status VARCHAR(50) DEFAULT 'pending' COMMENT '请求状态',
    retry_count INT DEFAULT 0 COMMENT '重试次数',
    http_status_code INT COMMENT 'HTTP状态码',
    is_streaming BOOLEAN DEFAULT FALSE COMMENT '是否为流式请求',
    model_name VARCHAR(255) COMMENT '模型名称',
    input_tokens BIGINT DEFAULT 0 COMMENT '输入Token数量',
    output_tokens BIGINT DEFAULT 0 COMMENT '输出Token数量',
    cache_creation_tokens BIGINT DEFAULT 0 COMMENT '缓存创建Token数量',
    cache_read_tokens BIGINT DEFAULT 0 COMMENT '缓存读取Token数量',
    input_cost_usd DECIMAL(10, 8) DEFAULT 0.00000000 COMMENT '输入成本(美元)',
    output_cost_usd DECIMAL(10, 8) DEFAULT 0.00000000 COMMENT '输出成本(美元)',
    cache_creation_cost_usd DECIMAL(10, 8) DEFAULT 0.00000000 COMMENT '缓存创建成本(美元)',
    cache_read_cost_usd DECIMAL(10, 8) DEFAULT 0.00000000 COMMENT '缓存读取成本(美元)',
    total_cost_usd DECIMAL(10, 8) DEFAULT 0.00000000 COMMENT '总成本(美元)',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间(API兼容)',
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间(API兼容)',
    INDEX idx_request_id (request_id),
    INDEX idx_start_time (start_time),
    INDEX idx_model_name (model_name),
    INDEX idx_endpoint_group (endpoint_name, group_name),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='请求记录主表';

CREATE TABLE IF NOT EXISTS usage_summary (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    date DATE NOT NULL COMMENT '统计日期',
    model_name VARCHAR(255) DEFAULT '' COMMENT '模型名称',
    endpoint_name VARCHAR(255) DEFAULT '' COMMENT '端点名称',
    group_name VARCHAR(255) DEFAULT '' COMMENT '组名称',
    request_count BIGINT DEFAULT 0 COMMENT '请求总数',
    success_count BIGINT DEFAULT 0 COMMENT '成功请求数',
    error_count BIGINT DEFAULT 0 COMMENT '错误请求数',
    total_input_tokens BIGINT DEFAULT 0 COMMENT '总输入Token数',
    total_output_tokens BIGINT DEFAULT 0 COMMENT '总输出Token数',
    total_cache_creation_tokens BIGINT DEFAULT 0 COMMENT '总缓存创建Token数',
    total_cache_read_tokens BIGINT DEFAULT 0 COMMENT '总缓存读取Token数',
    total_cost_usd DECIMAL(12, 8) DEFAULT 0.00000000 COMMENT '总成本(美元)',
    avg_duration_ms DECIMAL(10, 2) DEFAULT 0.00 COMMENT '平均耗时(毫秒)',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间',
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间',
    UNIQUE KEY uk_summary (date, model_name, endpoint_name, group_name),
    INDEX idx_date (date),
    INDEX idx_model_date (model_name, date),
    INDEX idx_endpoint_date (endpoint_name, date),
    INDEX idx_group_date (group_name, date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='使用统计汇总表';
`
}

// splitSQLStatements 分割SQL语句 - 修复版本
func (m *MySQLAdapter) splitSQLStatements(schema string) []string {
	var result []string

	// 按行分割，然后重新组装SQL语句
	lines := strings.Split(schema, "\n")
	var currentStatement strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过空行和注释行
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		// 添加到当前语句
		currentStatement.WriteString(line)
		currentStatement.WriteString(" ")

		// 如果行以分号结尾，表示语句结束
		if strings.HasSuffix(line, ";") {
			stmt := strings.TrimSpace(currentStatement.String())
			if stmt != "" {
				result = append(result, stmt)
				m.logger.Debug("分割SQL语句", "length", len(stmt), "preview", stmt[:min(80, len(stmt))])
			}
			currentStatement.Reset()
		}
	}

	// 处理最后一个未完成的语句（如果有）
	if currentStatement.Len() > 0 {
		stmt := strings.TrimSpace(currentStatement.String())
		if stmt != "" {
			result = append(result, stmt)
		}
	}

	m.logger.Info("SQL语句分割完成", "总数", len(result))
	return result
}


// trySetSessionTimezone 根据配置设置会话时区，确保一致性
func (m *MySQLAdapter) trySetSessionTimezone(ctx context.Context, db *sql.DB) {
	// 使用配置文件中指定的时区
	// 支持多种格式: "Asia/Shanghai", "+08:00", "Local", "UTC" 等
	configuredTimezone := strings.TrimSpace(m.config.Timezone)
	if configuredTimezone == "" {
		configuredTimezone = "Asia/Shanghai" // 默认值
	}

	m.logger.Debug("准备设置MySQL会话时区",
		"configured_timezone", configuredTimezone,
		"config_source", "database.timezone")

	// 执行多重时区设置，确保生效
	timezoneCommands := []string{
		fmt.Sprintf("SET time_zone = '%s'", configuredTimezone),
		fmt.Sprintf("SET @@session.time_zone = '%s'", configuredTimezone),
	}

	successCount := 0
	for i, cmd := range timezoneCommands {
		if _, err := db.ExecContext(ctx, cmd); err != nil {
			m.logger.Debug("时区设置命令失败",
				"command_index", i+1,
				"command", cmd,
				"error", err.Error())
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		m.logger.Info("✅ MySQL会话时区设置完成",
			"timezone", configuredTimezone,
			"success_commands", successCount,
			"total_commands", len(timezoneCommands))
	} else {
		m.logger.Warn("⚠️  所有时区设置命令都失败，可能出现时区不一致",
			"target_timezone", configuredTimezone)
	}

	// 验证最终时区设置
	m.verifyAndLogFinalTimezone(ctx, db, configuredTimezone)
}


// verifyAndLogFinalTimezone 验证并记录最终的时区设置结果
func (m *MySQLAdapter) verifyAndLogFinalTimezone(ctx context.Context, db *sql.DB, expectedTimezone string) {
	var currentTimezone string
	if err := db.QueryRowContext(ctx, "SELECT @@session.time_zone").Scan(&currentTimezone); err != nil {
		m.logger.Warn("无法验证最终时区设置", "error", err)
		return
	}

	// 测试时区是否真的生效：检查NOW()和UTC的时差
	var mysqlNow, mysqlUTC, timeDiff string
	query := "SELECT NOW(), UTC_TIMESTAMP(), TIMESTAMPDIFF(HOUR, UTC_TIMESTAMP(), NOW())"
	if err := db.QueryRowContext(ctx, query).Scan(&mysqlNow, &mysqlUTC, &timeDiff); err != nil {
		m.logger.Warn("无法测试时区效果", "error", err)
		return
	}

	m.logger.Info("🔍 时区设置验证结果",
		"expected", expectedTimezone,
		"actual_session_tz", currentTimezone,
		"mysql_now", mysqlNow,
		"mysql_utc", mysqlUTC,
		"hour_offset", timeDiff)

	// 检查是否达到预期的8小时偏移
	switch timeDiff {
	case "8":
		m.logger.Info("✅ 时区设置完全正确: MySQL使用Asia/Shanghai时区 (+8小时)")
	case "0":
		m.logger.Warn("⚠️  MySQL时区仍为UTC: 可能出现时间不一致问题")
	default:
		m.logger.Info("ℹ️  MySQL时区偏移", "hours", timeDiff)
	}
}

// getDSNCompatibleTimezone 获取DSN连接兼容的时区格式
func (m *MySQLAdapter) getDSNCompatibleTimezone() string {
	timezone := strings.TrimSpace(m.config.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai" // 默认时区
	}
	// 直接返回配置的时区，MySQL时区表已验证支持
	return timezone
}

// diagnoseTimezoneSettings 诊断时区设置，帮助调试时区不一致问题
func (m *MySQLAdapter) diagnoseTimezoneSettings(ctx context.Context, db *sql.DB) {
	// 查询MySQL时区设置
	var sessionTZ, globalTZ, systemTZ string

	row := db.QueryRowContext(ctx, "SELECT @@session.time_zone, @@global.time_zone, @@system_time_zone")
	if err := row.Scan(&sessionTZ, &globalTZ, &systemTZ); err != nil {
		m.logger.Debug("无法查询MySQL时区设置", "error", err)
		return
	}

	// 查询当前MySQL时间
	var mysqlNow, mysqlUTC string
	row2 := db.QueryRowContext(ctx, "SELECT NOW(), UTC_TIMESTAMP()")
	if err := row2.Scan(&mysqlNow, &mysqlUTC); err != nil {
		m.logger.Debug("无法查询MySQL当前时间", "error", err)
		return
	}

	// 获取Go应用当前时间
	goNow := time.Now()
	goUTC := goNow.UTC()

	// 输出诊断信息
	m.logger.Info("🔍 时区诊断信息",
		"config_timezone", m.config.Timezone,
		"mysql_session_tz", sessionTZ,
		"mysql_global_tz", globalTZ,
		"mysql_system_tz", systemTZ,
		"mysql_now", mysqlNow,
		"mysql_utc", mysqlUTC,
		"go_now", goNow.Format("2006-01-02 15:04:05 -07:00"),
		"go_utc", goUTC.Format("2006-01-02 15:04:05"))

	// 计算时区偏移
	_, goOffset := goNow.Zone()
	goOffsetHours := float64(goOffset) / 3600

	m.logger.Info("📊 时区偏移分析",
		"go_timezone_name", goNow.Location().String(),
		"go_offset_hours", goOffsetHours,
		"expected_mysql_offset", "+08:00 for Asia/Shanghai")
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
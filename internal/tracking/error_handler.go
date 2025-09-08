package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// ErrorHandler handles errors and provides recovery mechanisms
type ErrorHandler struct {
	tracker *UsageTracker
	logger  *slog.Logger
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(tracker *UsageTracker, logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{
		tracker: tracker,
		logger:  logger,
	}
}

// HandleDatabaseError handles database-related errors with recovery attempts
func (eh *ErrorHandler) HandleDatabaseError(err error, operation string) bool {
	if err == nil {
		return true
	}

	eh.logger.Error("Database operation failed", 
		"operation", operation, 
		"error", err.Error())

	// 尝试诊断错误类型
	switch {
	case isDiskSpaceError(err):
		return eh.handleDiskSpaceError()
	case isDatabaseCorruptionError(err):
		return eh.handleCorruptionError()
	case isDatabaseLockedError(err):
		return eh.handleLockError()
	case isConnectionError(err):
		return eh.handleConnectionError()
	default:
		eh.logger.Warn("Unknown database error type", "error", err)
		return false
	}
}

// handleDiskSpaceError handles disk space issues
func (eh *ErrorHandler) handleDiskSpaceError() bool {
	eh.logger.Warn("Disk space error detected, attempting cleanup...")
	
	// 尝试清理旧数据
	if eh.tracker != nil {
		if err := eh.tracker.cleanupOldRecords(); err != nil {
			eh.logger.Error("Emergency cleanup failed", "error", err)
			return false
		}
		eh.logger.Info("Emergency cleanup completed")
		return true
	}
	
	return false
}

// handleCorruptionError handles database corruption
func (eh *ErrorHandler) handleCorruptionError() bool {
	eh.logger.Error("Database corruption detected, attempting recovery...")
	
	if eh.tracker == nil || eh.tracker.db == nil {
		return false
	}
	
	// 尝试数据库完整性检查
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	var integrityResult string
	err := eh.tracker.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult)
	if err != nil {
		eh.logger.Error("Integrity check failed", "error", err)
		return false
	}
	
	if integrityResult != "ok" {
		eh.logger.Error("Database integrity compromised", "result", integrityResult)
		
		// 尝试备份和重建
		return eh.attemptDatabaseRestore()
	}
	
	eh.logger.Info("Database integrity check passed")
	return true
}

// handleLockError handles database lock issues
func (eh *ErrorHandler) handleLockError() bool {
	eh.logger.Warn("Database lock detected, waiting for release...")
	
	// 等待锁释放
	for i := 0; i < 10; i++ {
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		
		// 尝试简单查询测试连接
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		var count int
		err := eh.tracker.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master").Scan(&count)
		cancel()
		
		if err == nil {
			eh.logger.Info("Database lock released", "attempts", i+1)
			return true
		}
	}
	
	eh.logger.Error("Database lock timeout exceeded")
	return false
}

// handleConnectionError handles connection issues
func (eh *ErrorHandler) handleConnectionError() bool {
	eh.logger.Warn("Database connection error, attempting reconnection...")
	
	if eh.tracker == nil || eh.tracker.config == nil {
		return false
	}
	
	// 尝试重新连接
	return eh.attemptReconnection()
}

// attemptReconnection attempts to reconnect to the database
func (eh *ErrorHandler) attemptReconnection() bool {
	for attempt := 1; attempt <= 3; attempt++ {
		eh.logger.Info("Attempting database reconnection", "attempt", attempt)
		
		// 关闭现有连接
		if eh.tracker.db != nil {
			eh.tracker.db.Close()
		}
		
		// 尝试重新打开数据库 - 使用更安全的配置
		db, err := sql.Open("sqlite", eh.tracker.config.DatabasePath+"?_journal_mode=WAL&_synchronous=FULL&_cache_size=10000&_foreign_keys=1&_busy_timeout=30000")
		if err != nil {
			eh.logger.Error("Reconnection failed", "attempt", attempt, "error", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		
		// 测试连接
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		
		if err != nil {
			eh.logger.Error("Connection test failed", "attempt", attempt, "error", err)
			db.Close()
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		
		// 重新设置连接参数
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)
		
		eh.tracker.db = db
		eh.logger.Info("Database reconnection successful")
		return true
	}
	
	eh.logger.Error("All reconnection attempts failed")
	return false
}

// attemptDatabaseRestore attempts to restore the database from backup
func (eh *ErrorHandler) attemptDatabaseRestore() bool {
	if eh.tracker == nil || eh.tracker.config == nil {
		return false
	}
	
	dbPath := eh.tracker.config.DatabasePath
	backupPath := dbPath + ".backup"
	corruptedPath := dbPath + ".corrupted." + time.Now().Format("20060102-150405")
	
	// 检查是否有备份文件
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		eh.logger.Error("No backup file found for restoration", "backup_path", backupPath)
		return false
	}
	
	eh.logger.Info("Attempting database restoration from backup", 
		"backup_path", backupPath, 
		"corrupted_path", corruptedPath)
	
	// 移动损坏的数据库文件
	if err := os.Rename(dbPath, corruptedPath); err != nil {
		eh.logger.Error("Failed to move corrupted database", "error", err)
		return false
	}
	
	// 复制备份文件
	if err := copyFile(backupPath, dbPath); err != nil {
		eh.logger.Error("Failed to restore from backup", "error", err)
		// 尝试恢复原文件
		os.Rename(corruptedPath, dbPath)
		return false
	}
	
	// 尝试重新连接到恢复的数据库
	if !eh.attemptReconnection() {
		eh.logger.Error("Failed to connect to restored database")
		return false
	}
	
	eh.logger.Info("Database successfully restored from backup")
	return true
}

// CreateBackup creates a backup of the current database
func (eh *ErrorHandler) CreateBackup() error {
	if eh.tracker == nil || eh.tracker.db == nil {
		return fmt.Errorf("tracker or database not initialized")
	}
	
	dbPath := eh.tracker.config.DatabasePath
	backupPath := dbPath + ".backup"
	tempBackupPath := backupPath + ".tmp"
	
	// 创建备份目录
	if err := os.MkdirAll(filepath.Dir(tempBackupPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	
	// 使用SQLite的备份API
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// 创建临时备份
	backupDB, err := sql.Open("sqlite", tempBackupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup database: %w", err)
	}
	defer backupDB.Close()
	
	// 简化的备份方案：导出和导入数据
	if err := eh.performSimpleBackup(ctx, backupDB); err != nil {
		os.Remove(tempBackupPath)
		return fmt.Errorf("backup operation failed: %w", err)
	}
	
	// 原子性地移动备份文件
	if err := os.Rename(tempBackupPath, backupPath); err != nil {
		os.Remove(tempBackupPath)
		return fmt.Errorf("failed to finalize backup: %w", err)
	}
	
	eh.logger.Info("Database backup created successfully", "backup_path", backupPath)
	return nil
}

// performSimpleBackup performs a simple backup by recreating schema and copying data
func (eh *ErrorHandler) performSimpleBackup(ctx context.Context, backupDB *sql.DB) error {
	// 首先在备份数据库中创建表结构
	schemaSQL := `
	CREATE TABLE IF NOT EXISTS request_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT UNIQUE NOT NULL,
		client_ip TEXT,
		user_agent TEXT,
		method TEXT DEFAULT 'POST',
		path TEXT DEFAULT '/v1/messages',
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		duration_ms INTEGER,
		endpoint_name TEXT,
		group_name TEXT,
		model_name TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		http_status_code INTEGER,
		retry_count INTEGER DEFAULT 0,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		cache_creation_tokens INTEGER DEFAULT 0,
		cache_read_tokens INTEGER DEFAULT 0,
		input_cost_usd REAL DEFAULT 0,
		output_cost_usd REAL DEFAULT 0,
		cache_creation_cost_usd REAL DEFAULT 0,
		cache_read_cost_usd REAL DEFAULT 0,
		total_cost_usd REAL DEFAULT 0,
		created_at DATETIME DEFAULT (datetime('now', 'localtime')),
		updated_at DATETIME DEFAULT (datetime('now', 'localtime'))
	);
	
	CREATE TABLE IF NOT EXISTS usage_summary (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL,
		model_name TEXT NOT NULL,
		endpoint_name TEXT NOT NULL,
		group_name TEXT,
		request_count INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		error_count INTEGER DEFAULT 0,
		total_input_tokens INTEGER DEFAULT 0,
		total_output_tokens INTEGER DEFAULT 0,
		total_cache_creation_tokens INTEGER DEFAULT 0,
		total_cache_read_tokens INTEGER DEFAULT 0,
		total_cost_usd REAL DEFAULT 0,
		avg_duration_ms REAL DEFAULT 0,
		created_at DATETIME DEFAULT (datetime('now', 'localtime')),
		updated_at DATETIME DEFAULT (datetime('now', 'localtime')),
		UNIQUE(date, model_name, endpoint_name, group_name)
	);`
	
	if _, err := backupDB.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("failed to create backup schema: %w", err)
	}
	
	// 复制请求日志数据
	rows, err := eh.tracker.db.QueryContext(ctx, `
		SELECT request_id, 
		       COALESCE(client_ip, '') as client_ip, 
		       COALESCE(user_agent, '') as user_agent, 
		       COALESCE(method, 'POST') as method, 
		       COALESCE(path, '/v1/messages') as path, 
		       start_time, end_time, duration_ms,
			   COALESCE(endpoint_name, '') as endpoint_name, 
			   COALESCE(group_name, '') as group_name, 
			   COALESCE(model_name, '') as model_name, 
			   status, http_status_code, retry_count,
			   input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			   input_cost_usd, output_cost_usd, cache_creation_cost_usd, cache_read_cost_usd, 
			   total_cost_usd, created_at, updated_at
		FROM request_logs ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("failed to read request logs: %w", err)
	}
	defer rows.Close()
	
	stmt, err := backupDB.PrepareContext(ctx, `
		INSERT INTO request_logs (
			request_id, client_ip, user_agent, method, path, start_time, end_time, duration_ms,
			endpoint_name, group_name, model_name, status, http_status_code, retry_count,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			input_cost_usd, output_cost_usd, cache_creation_cost_usd, cache_read_cost_usd,
			total_cost_usd, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()
	
	recordCount := 0
	for rows.Next() {
		var r RequestDetail
		err := rows.Scan(
			&r.RequestID, &r.ClientIP, &r.UserAgent, &r.Method, &r.Path,
			&r.StartTime, &r.EndTime, &r.DurationMs, &r.EndpointName, &r.GroupName,
			&r.ModelName, &r.Status, &r.HTTPStatusCode, &r.RetryCount,
			&r.InputTokens, &r.OutputTokens, &r.CacheCreationTokens, &r.CacheReadTokens,
			&r.InputCostUSD, &r.OutputCostUSD, &r.CacheCreationCostUSD, &r.CacheReadCostUSD,
			&r.TotalCostUSD, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to scan record: %w", err)
		}
		
		_, err = stmt.ExecContext(ctx,
			r.RequestID, r.ClientIP, r.UserAgent, r.Method, r.Path,
			r.StartTime, r.EndTime, r.DurationMs, r.EndpointName, r.GroupName,
			r.ModelName, r.Status, r.HTTPStatusCode, r.RetryCount,
			r.InputTokens, r.OutputTokens, r.CacheCreationTokens, r.CacheReadTokens,
			r.InputCostUSD, r.OutputCostUSD, r.CacheCreationCostUSD, r.CacheReadCostUSD,
			r.TotalCostUSD, r.CreatedAt, r.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert record: %w", err)
		}
		
		recordCount++
	}
	
	eh.logger.Info("Backup completed", "records_copied", recordCount)
	return nil
}

// Error type detection helper functions
func isDiskSpaceError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "no space left") || 
		   contains(errStr, "disk full") || 
		   contains(errStr, "SQLITE_FULL")
}

func isDatabaseCorruptionError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "SQLITE_CORRUPT") ||
		   contains(errStr, "database disk image is malformed") ||
		   contains(errStr, "file is not a database")
}

func isDatabaseLockedError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "SQLITE_BUSY") ||
		   contains(errStr, "SQLITE_LOCKED") ||
		   contains(errStr, "database is locked")
}

func isConnectionError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "connection") ||
		   contains(errStr, "no such file") ||
		   contains(errStr, "unable to open database")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		   (len(s) > len(substr) && 
		   (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr ||
		   containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RestoreFromBackup 从备份恢复数据库
func (eh *ErrorHandler) RestoreFromBackup() error {
	if eh.tracker == nil || eh.tracker.config == nil {
		return fmt.Errorf("tracker not initialized")
	}
	
	dbPath := eh.tracker.config.DatabasePath
	backupPath := dbPath + ".backup"
	
	// 检查备份文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}
	
	eh.logger.Info("Restoring database from backup", "backup_path", backupPath)
	
	// 关闭当前数据库连接
	if eh.tracker.db != nil {
		eh.tracker.db.Close()
	}
	
	// 复制备份文件到原位置
	if err := copyFile(backupPath, dbPath); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}
	
	// 重新建立数据库连接
	if !eh.attemptReconnection() {
		return fmt.Errorf("failed to reconnect after restore")
	}
	
	eh.logger.Info("Database successfully restored from backup")
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := sourceFile.Read(buf)
		if n > 0 {
			if _, writeErr := destFile.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}
	
	return destFile.Sync()
}
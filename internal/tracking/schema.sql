-- Usage Tracking Database Schema
-- 使用跟踪系统数据库结构
-- 创建时间: 2025-09-04

-- 请求记录主表
CREATE TABLE IF NOT EXISTS request_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT UNIQUE NOT NULL,        -- req-xxxxxxxx
    
    -- 请求基本信息
    client_ip TEXT,                         -- 客户端IP
    user_agent TEXT,                        -- 客户端User-Agent
    method TEXT DEFAULT 'POST',             -- HTTP方法
    path TEXT DEFAULT '/v1/messages',       -- 请求路径
    
    -- 时间信息
    start_time DATETIME NOT NULL,           -- 请求开始时间
    end_time DATETIME,                      -- 请求完成时间
    duration_ms INTEGER,                    -- 总耗时(毫秒)
    
    -- 转发信息
    endpoint_name TEXT,                     -- 使用的端点名称
    group_name TEXT,                        -- 所属组名
    model_name TEXT,                        -- Claude模型名称
    is_streaming BOOLEAN DEFAULT FALSE,     -- 是否为流式请求
    
    -- 状态信息
    status TEXT NOT NULL DEFAULT 'pending', -- pending/success/error/suspended/timeout
    http_status_code INTEGER,               -- HTTP状态码
    retry_count INTEGER DEFAULT 0,          -- 重试次数
    
    -- Token统计
    input_tokens INTEGER DEFAULT 0,        -- 输入token数
    output_tokens INTEGER DEFAULT 0,       -- 输出token数
    cache_creation_tokens INTEGER DEFAULT 0, -- 缓存创建token数
    cache_read_tokens INTEGER DEFAULT 0,   -- 缓存读取token数
    
    -- 成本计算（包含缓存）
    input_cost_usd REAL DEFAULT 0,         -- 输入token成本
    output_cost_usd REAL DEFAULT 0,        -- 输出token成本
    cache_creation_cost_usd REAL DEFAULT 0, -- 缓存创建成本
    cache_read_cost_usd REAL DEFAULT 0,    -- 缓存读取成本
    total_cost_usd REAL DEFAULT 0,         -- 总成本
    
    -- 审计字段（统一使用带时区格式）
    created_at DATETIME DEFAULT (datetime('now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (datetime('now', 'localtime') || '+08:00')
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_request_logs_request_id ON request_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_start_time ON request_logs(start_time);
CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_endpoint ON request_logs(endpoint_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_group ON request_logs(group_name);

-- 使用统计汇总表 (可选，用于快速查询)
CREATE TABLE IF NOT EXISTS usage_summary (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,                    -- YYYY-MM-DD
    model_name TEXT NOT NULL,
    endpoint_name TEXT NOT NULL,
    group_name TEXT,
    
    request_count INTEGER DEFAULT 0,       -- 请求总数
    success_count INTEGER DEFAULT 0,       -- 成功请求数
    error_count INTEGER DEFAULT 0,         -- 失败请求数
    
    total_input_tokens INTEGER DEFAULT 0,
    total_output_tokens INTEGER DEFAULT 0,
    total_cache_creation_tokens INTEGER DEFAULT 0,
    total_cache_read_tokens INTEGER DEFAULT 0,
    total_cost_usd REAL DEFAULT 0,
    
    avg_duration_ms REAL DEFAULT 0,        -- 平均响应时间
    
    created_at DATETIME DEFAULT (datetime('now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (datetime('now', 'localtime') || '+08:00'),
    
    UNIQUE(date, model_name, endpoint_name, group_name)
);

-- 汇总表索引
CREATE INDEX IF NOT EXISTS idx_usage_summary_date ON usage_summary(date);
CREATE INDEX IF NOT EXISTS idx_usage_summary_model ON usage_summary(model_name);
CREATE INDEX IF NOT EXISTS idx_usage_summary_endpoint ON usage_summary(endpoint_name);
CREATE INDEX IF NOT EXISTS idx_usage_summary_group ON usage_summary(group_name);

-- 触发器：自动更新 updated_at 时间戳（统一使用带时区格式）
CREATE TRIGGER IF NOT EXISTS update_request_logs_timestamp
    AFTER UPDATE ON request_logs
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE request_logs SET updated_at = datetime('now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_usage_summary_timestamp
    AFTER UPDATE ON usage_summary
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE usage_summary SET updated_at = datetime('now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;
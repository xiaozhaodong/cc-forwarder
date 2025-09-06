-- 数据库时区统一迁移脚本
-- 将所有时间字段统一为带时区格式

-- 重命名现有表为备份表
ALTER TABLE request_logs RENAME TO request_logs_old;
ALTER TABLE usage_summary RENAME TO usage_summary_old;

-- 删除旧的触发器
DROP TRIGGER IF EXISTS update_request_logs_timestamp;
DROP TRIGGER IF EXISTS update_usage_summary_timestamp;

-- 创建新的表结构（带时区的created_at和updated_at）
CREATE TABLE request_logs (
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

CREATE TABLE usage_summary (
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

-- 迁移 request_logs 数据，转换 created_at 和 updated_at 为带时区格式
INSERT INTO request_logs (
    id, request_id, client_ip, user_agent, method, path,
    start_time, end_time, duration_ms,
    endpoint_name, group_name, model_name,
    status, http_status_code, retry_count,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
    input_cost_usd, output_cost_usd, cache_creation_cost_usd, cache_read_cost_usd, total_cost_usd,
    created_at, updated_at
) SELECT 
    id, request_id, client_ip, user_agent, method, path,
    start_time, end_time, duration_ms,
    endpoint_name, group_name, model_name,
    status, http_status_code, retry_count,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
    input_cost_usd, output_cost_usd, cache_creation_cost_usd, cache_read_cost_usd, total_cost_usd,
    -- 转换时间为带时区格式
    created_at || '+08:00' as created_at,
    updated_at || '+08:00' as updated_at
FROM request_logs_old;

-- 迁移 usage_summary 数据
INSERT INTO usage_summary (
    id, date, model_name, endpoint_name, group_name,
    request_count, success_count, error_count,
    total_input_tokens, total_output_tokens, total_cache_creation_tokens, total_cache_read_tokens,
    total_cost_usd, avg_duration_ms,
    created_at, updated_at
) SELECT 
    id, date, model_name, endpoint_name, group_name,
    request_count, success_count, error_count,
    total_input_tokens, total_output_tokens, total_cache_creation_tokens, total_cache_read_tokens,
    total_cost_usd, avg_duration_ms,
    -- 转换时间为带时区格式
    created_at || '+08:00' as created_at,
    updated_at || '+08:00' as updated_at
FROM usage_summary_old;

-- 重新创建索引
CREATE INDEX idx_request_logs_request_id ON request_logs(request_id);
CREATE INDEX idx_request_logs_start_time ON request_logs(start_time);
CREATE INDEX idx_request_logs_status ON request_logs(status);
CREATE INDEX idx_request_logs_model ON request_logs(model_name);
CREATE INDEX idx_request_logs_endpoint ON request_logs(endpoint_name);
CREATE INDEX idx_request_logs_group ON request_logs(group_name);

CREATE INDEX idx_usage_summary_date ON usage_summary(date);
CREATE INDEX idx_usage_summary_model ON usage_summary(model_name);
CREATE INDEX idx_usage_summary_endpoint ON usage_summary(endpoint_name);
CREATE INDEX idx_usage_summary_group ON usage_summary(group_name);

-- 重新创建触发器（统一使用带时区格式）
CREATE TRIGGER update_request_logs_timestamp
    AFTER UPDATE ON request_logs
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE request_logs SET updated_at = datetime('now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

CREATE TRIGGER update_usage_summary_timestamp
    AFTER UPDATE ON usage_summary
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE usage_summary SET updated_at = datetime('now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- 验证迁移结果
.print "Migration completed. Verifying results..."
SELECT 'request_logs count:' as info, COUNT(*) as count FROM request_logs;
SELECT 'usage_summary count:' as info, COUNT(*) as count FROM usage_summary;

-- 显示时间格式示例
.print "Time format examples:"
SELECT 'Old format example:', created_at FROM request_logs_old LIMIT 1;
SELECT 'New format example:', created_at FROM request_logs LIMIT 1;
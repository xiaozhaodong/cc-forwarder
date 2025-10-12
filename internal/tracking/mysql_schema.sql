-- MySQL Database Schema for cc-forwarder
-- 兼容MySQL 5.7和8.0版本
-- 创建时间: 2025-09-24
-- 字符集: utf8mb4 (支持emoji和中文)

-- 请求记录主表
CREATE TABLE IF NOT EXISTS request_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    request_id VARCHAR(255) UNIQUE NOT NULL COMMENT 'req-xxxxxxxx',

    -- 请求基本信息
    client_ip VARCHAR(45) COMMENT '客户端IP',
    user_agent TEXT COMMENT '客户端User-Agent',
    method VARCHAR(10) DEFAULT 'POST' COMMENT 'HTTP方法',
    path VARCHAR(255) DEFAULT '/v1/messages' COMMENT '请求路径',

    -- 时间信息（API兼容字段）
    start_time DATETIME(6) NOT NULL COMMENT '请求开始时间（微秒精度）',
    end_time DATETIME(6) COMMENT '请求完成时间（微秒精度）',
    duration_ms BIGINT COMMENT '总耗时(毫秒)',


    -- 转发信息
    endpoint_name VARCHAR(255) COMMENT '使用的端点名称',
    group_name VARCHAR(255) COMMENT '所属组名',
    model_name VARCHAR(255) COMMENT 'Claude模型名称',
    is_streaming BOOLEAN DEFAULT FALSE COMMENT '是否为流式请求',

    -- 状态信息 (v3.5.0更新: 生命周期状态与错误原因分离 - 2025-09-28)
    status VARCHAR(50) NOT NULL DEFAULT 'pending' COMMENT '生命周期状态: pending/forwarding/processing/retry/suspended/completed/failed/cancelled',
    http_status_code INT COMMENT 'HTTP状态码',
    retry_count INT DEFAULT 0 COMMENT '重试次数',

    -- 失败信息 (状态机重构新增字段 v3.5.0 - 2025-09-28)
    failure_reason VARCHAR(50) COMMENT '当前失败原因类型: rate_limited/server_error/network_error/timeout/empty_response/invalid_response',
    last_failure_reason TEXT COMMENT '最后一次失败的详细错误信息',

    -- 取消信息 (状态机重构新增字段 v3.5.0 - 2025-09-28)
    cancel_reason VARCHAR(255) COMMENT '取消原因(取消时间使用end_time字段)',

    -- Token统计
    input_tokens BIGINT DEFAULT 0 COMMENT '输入token数',
    output_tokens BIGINT DEFAULT 0 COMMENT '输出token数',
    cache_creation_tokens BIGINT DEFAULT 0 COMMENT '缓存创建token数',
    cache_read_tokens BIGINT DEFAULT 0 COMMENT '缓存读取token数',

    -- 成本计算（包含缓存）
    input_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '输入token成本',
    output_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '输出token成本',
    cache_creation_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '缓存创建成本',
    cache_read_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '缓存读取成本',
    total_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '总成本',

    -- API兼容的审计字段
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间(API兼容)',
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间(API兼容)',

    -- 索引
    INDEX idx_request_id (request_id),
    INDEX idx_start_time (start_time),
    INDEX idx_status (status),
    INDEX idx_model_name (model_name),
    INDEX idx_endpoint_name (endpoint_name),
    INDEX idx_group_name (group_name),
    INDEX idx_failure_reason (failure_reason),
    INDEX idx_created_at (created_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='请求日志记录表';

-- 使用统计汇总表 (用于快速查询)
CREATE TABLE IF NOT EXISTS usage_summary (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    date DATE NOT NULL COMMENT 'YYYY-MM-DD',
    model_name VARCHAR(255) NOT NULL COMMENT '模型名称',
    endpoint_name VARCHAR(255) NOT NULL COMMENT '端点名称',
    group_name VARCHAR(255) COMMENT '组名',

    request_count INT DEFAULT 0 COMMENT '请求总数',
    success_count INT DEFAULT 0 COMMENT '成功请求数',
    error_count INT DEFAULT 0 COMMENT '失败请求数',

    total_input_tokens BIGINT DEFAULT 0 COMMENT '总输入tokens',
    total_output_tokens BIGINT DEFAULT 0 COMMENT '总输出tokens',
    total_cache_creation_tokens BIGINT DEFAULT 0 COMMENT '总缓存创建tokens',
    total_cache_read_tokens BIGINT DEFAULT 0 COMMENT '总缓存读取tokens',
    total_cost_usd DECIMAL(10,6) DEFAULT 0 COMMENT '总成本',

    avg_duration_ms DECIMAL(10,2) DEFAULT 0 COMMENT '平均响应时间',

    -- API兼容的审计字段
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) COMMENT '创建时间(API兼容)',
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) COMMENT '更新时间(API兼容)',


    -- 唯一约束
    UNIQUE KEY unique_summary (date, model_name, endpoint_name, group_name),

    -- 索引
    INDEX idx_date (date),
    INDEX idx_model_name (model_name),
    INDEX idx_endpoint_name (endpoint_name),
    INDEX idx_group_name (group_name),
    INDEX idx_created_at (created_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='使用统计汇总表';
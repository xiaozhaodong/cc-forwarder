/**
 * requestsConstants - 常量定义
 * 文件描述: 定义请求管理相关的常量和配置
 * 创建时间: 2025-09-20 18:03:21
 *
 * 包含的常量:
 * - 请求状态: pending, forwarding, streaming, processing, completed, error, retry, cancelled
 * - 页面大小: 20, 50, 100, 200 (默认50)
 * - HTTP方法和颜色映射
 * - API端点配置
 * - 错误消息定义
 * - 筛选器选项配置
 */

// 请求状态配置
export const REQUEST_STATUS = {
    PENDING: 'pending',
    FORWARDING: 'forwarding',
    PROCESSING: 'processing',
    COMPLETED: 'completed',
    ERROR: 'error',
    RETRY: 'retry',
    CANCELLED: 'cancelled',
    TIMEOUT: 'timeout',
    SUSPENDED: 'suspended',
    NETWORK_ERROR: 'network_error',
    AUTH_ERROR: 'auth_error',
    RATE_LIMITED: 'rate_limited',
    STREAM_ERROR: 'stream_error'
};

// 状态显示配置
export const STATUS_CONFIG = {
    [REQUEST_STATUS.PENDING]: {
        label: '等待中',
        type: 'pending',
        icon: '⏳',
        color: '#3b82f6'
    },
    [REQUEST_STATUS.FORWARDING]: {
        label: '转发中',
        type: 'forwarding',
        icon: '📤',
        color: '#3b82f6'
    },
    [REQUEST_STATUS.PROCESSING]: {
        label: '处理中',
        type: 'processing',
        icon: '⚙️',
        color: '#f97316'
    },
    [REQUEST_STATUS.COMPLETED]: {
        label: '已完成',
        type: 'success',
        icon: '✅',
        color: '#10b981'
    },
    [REQUEST_STATUS.ERROR]: {
        label: '失败',
        type: 'error',
        icon: '✖️',
        color: '#ef4444',
        detailLabel: '请求错误'
    },
    [REQUEST_STATUS.RETRY]: {
        label: '重试中',
        type: 'retry',
        icon: '🔄',
        color: '#f59e0b'
    },
    [REQUEST_STATUS.CANCELLED]: {
        label: '已取消',
        type: 'cancelled',
        icon: '🚫',
        color: '#374151'
    },
    [REQUEST_STATUS.TIMEOUT]: {
        label: '超时',
        type: 'timeout',
        icon: '⏰',
        color: '#6b7280'
    },
    [REQUEST_STATUS.SUSPENDED]: {
        label: '挂起',
        type: 'suspended',
        icon: '⏸️',
        color: '#6b7280'
    },
    [REQUEST_STATUS.NETWORK_ERROR]: {
        label: '失败',
        type: 'error',
        icon: '✖️',
        color: '#ef4444',
        detailLabel: '网络错误'
    },
    [REQUEST_STATUS.AUTH_ERROR]: {
        label: '失败',
        type: 'error',
        icon: '✖️',
        color: '#ef4444',
        detailLabel: '认证错误'
    },
    [REQUEST_STATUS.RATE_LIMITED]: {
        label: '失败',
        type: 'error',
        icon: '✖️',
        color: '#ef4444',
        detailLabel: '限流错误'
    },
    [REQUEST_STATUS.STREAM_ERROR]: {
        label: '失败',
        type: 'error',
        icon: '✖️',
        color: '#ef4444',
        detailLabel: '流式错误'
    }
};

// HTTP方法配置
export const HTTP_METHODS = {
    GET: 'GET',
    POST: 'POST',
    PUT: 'PUT',
    DELETE: 'DELETE',
    PATCH: 'PATCH',
    HEAD: 'HEAD',
    OPTIONS: 'OPTIONS'
};

// HTTP方法颜色配置
export const METHOD_COLORS = {
    [HTTP_METHODS.GET]: '#10b981',
    [HTTP_METHODS.POST]: '#3b82f6',
    [HTTP_METHODS.PUT]: '#f59e0b',
    [HTTP_METHODS.DELETE]: '#ef4444',
    [HTTP_METHODS.PATCH]: '#8b5cf6',
    [HTTP_METHODS.HEAD]: '#6b7280',
    [HTTP_METHODS.OPTIONS]: '#06b6d4'
};

// 筛选器状态选项
export const FILTER_STATUS_OPTIONS = [
    { value: '', label: '全部状态' },
    { value: 'completed', label: '已完成' },
    { value: 'error', label: '失败' },
    { value: 'pending', label: '等待中' },
    { value: 'processing', label: '处理中' },
    { value: 'retry', label: '重试中' },
    { value: 'cancelled', label: '已取消' },
    { value: 'timeout', label: '超时' },
    { value: 'suspended', label: '挂起' }
];

// 分页配置
export const PAGINATION_CONFIG = {
    DEFAULT_PAGE_SIZE: 50,                    // 默认页面大小改为50
    PAGE_SIZE_OPTIONS: [20, 50, 100, 200],   // 支持的页面大小选项
    MAX_PAGE_BUTTONS: 7
};

// 表格列配置
export const TABLE_COLUMNS = {
    REQUEST_ID: 'requestId',
    STATUS: 'status',
    METHOD: 'method',
    ENDPOINT: 'endpoint',
    DURATION: 'duration',
    TOKENS: 'tokens',
    TIMESTAMP: 'timestamp',
    ACTIONS: 'actions'
};

// API端点配置
export const API_ENDPOINTS = {
    REQUESTS: '/api/v1/usage/requests',
    REQUEST_DETAIL: '/api/v1/usage/requests/{id}',
    MODELS: '/api/v1/usage/models',
    ENDPOINTS: '/api/v1/endpoints',
    GROUPS: '/api/v1/groups',
    STREAM: '/api/v1/stream',
    STATS: '/api/v1/usage/stats',
    SUMMARY: '/api/v1/usage/summary'
};

// 错误消息
export const ERROR_MESSAGES = {
    FETCH_FAILED: '获取数据失败',
    NETWORK_ERROR: '网络连接错误',
    INVALID_RESPONSE: '响应数据格式错误',
    REQUEST_TIMEOUT: '请求超时',
    SERVER_ERROR: '服务器错误',
    UNKNOWN_ERROR: '未知错误'
};

// 加载状态
export const LOADING_STATES = {
    IDLE: 'idle',
    LOADING: 'loading',
    SUCCESS: 'success',
    ERROR: 'error'
};

// 获取状态配置的帮助函数
export const getStatusConfig = (status) => {
    return STATUS_CONFIG[status] || {
        label: status || 'Unknown',
        type: 'unknown',
        icon: '❓',
        color: '#6b7280'
    };
};

// 获取详情页面显示的状态标签（显示具体错误类型）
export const getDetailStatusLabel = (status) => {
    const config = STATUS_CONFIG[status];
    return (config && config.detailLabel) ? config.detailLabel : (config ? config.label : status || 'Unknown');
};

// 获取方法颜色的帮助函数
export const getMethodColor = (method) => {
    return METHOD_COLORS[method?.toUpperCase()] || METHOD_COLORS[HTTP_METHODS.POST];
};
/**
 * 组相关常量定义文件
 *
 * 负责：
 * - 定义组管理相关的常量值
 * - 统一管理配置参数
 * - 提供默认值和限制值
 * - 状态和类型枚举定义
 *
 * 功能特性：
 * - 集中管理常量值
 * - 类型安全的枚举定义
 * - 配置参数统一管理
 * - 易于维护和修改
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

// ============================================================================
// 组状态相关常量
// ============================================================================

/**
 * 组状态枚举
 */
export const GROUP_STATUS = {
    ACTIVE: 'active',
    PAUSED: 'paused',
    COOLDOWN: 'cooldown',
    UNKNOWN: 'unknown'
};

/**
 * 组健康状态枚举
 */
export const GROUP_HEALTH = {
    HEALTHY: 'healthy',
    UNHEALTHY: 'unhealthy',
    UNKNOWN: 'unknown'
};

/**
 * 组操作类型枚举
 */
export const GROUP_OPERATIONS = {
    ACTIVATE: 'activate',
    PAUSE: 'pause',
    FORCE_ACTIVATE: 'forceActivate',
    REFRESH: 'refresh'
};

/**
 * 组状态对应的中文描述
 */
export const GROUP_STATUS_LABELS = {
    [GROUP_STATUS.ACTIVE]: '活跃',
    [GROUP_STATUS.PAUSED]: '暂停',
    [GROUP_STATUS.COOLDOWN]: '冷却',
    [GROUP_STATUS.UNKNOWN]: '未知'
};

/**
 * 组健康状态对应的中文描述
 */
export const GROUP_HEALTH_LABELS = {
    [GROUP_HEALTH.HEALTHY]: '健康',
    [GROUP_HEALTH.UNHEALTHY]: '异常',
    [GROUP_HEALTH.UNKNOWN]: '未知'
};

/**
 * 组操作对应的中文描述
 */
export const GROUP_OPERATION_LABELS = {
    [GROUP_OPERATIONS.ACTIVATE]: '激活',
    [GROUP_OPERATIONS.PAUSE]: '暂停',
    [GROUP_OPERATIONS.FORCE_ACTIVATE]: '强制激活',
    [GROUP_OPERATIONS.REFRESH]: '刷新'
};

// ============================================================================
// 组状态图标和颜色配置
// ============================================================================

/**
 * 组状态图标映射
 */
export const GROUP_STATUS_ICONS = {
    [GROUP_STATUS.ACTIVE]: '✅',
    [GROUP_STATUS.PAUSED]: '⏸️',
    [GROUP_STATUS.COOLDOWN]: '🧊',
    [GROUP_STATUS.UNKNOWN]: '❓'
};

/**
 * 组健康状态图标映射
 */
export const GROUP_HEALTH_ICONS = {
    [GROUP_HEALTH.HEALTHY]: '💚',
    [GROUP_HEALTH.UNHEALTHY]: '💔',
    [GROUP_HEALTH.UNKNOWN]: '❓'
};

/**
 * 组状态颜色配置
 */
export const GROUP_STATUS_COLORS = {
    [GROUP_STATUS.ACTIVE]: {
        primary: '#10b981',
        background: '#f0fdf4',
        border: '#bbf7d0',
        text: '#065f46'
    },
    [GROUP_STATUS.PAUSED]: {
        primary: '#6b7280',
        background: '#f9fafb',
        border: '#e5e7eb',
        text: '#374151'
    },
    [GROUP_STATUS.COOLDOWN]: {
        primary: '#ef4444',
        background: '#fef2f2',
        border: '#fecaca',
        text: '#7f1d1d'
    },
    [GROUP_STATUS.UNKNOWN]: {
        primary: '#6b7280',
        background: '#f9fafb',
        border: '#e5e7eb',
        text: '#374151'
    }
};

/**
 * 组健康状态颜色配置
 */
export const GROUP_HEALTH_COLORS = {
    [GROUP_HEALTH.HEALTHY]: {
        primary: '#10b981',
        background: '#f0fdf4',
        border: '#bbf7d0',
        text: '#065f46'
    },
    [GROUP_HEALTH.UNHEALTHY]: {
        primary: '#ef4444',
        background: '#fef2f2',
        border: '#fecaca',
        text: '#7f1d1d'
    },
    [GROUP_HEALTH.UNKNOWN]: {
        primary: '#6b7280',
        background: '#f9fafb',
        border: '#e5e7eb',
        text: '#374151'
    }
};

// ============================================================================
// 组优先级相关常量
// ============================================================================

/**
 * 组优先级级别
 */
export const GROUP_PRIORITY_LEVELS = {
    HIGHEST: 1,
    HIGH: 2,
    MEDIUM: 3,
    LOW: 4,
    LOWEST: 5
};

/**
 * 组优先级标签
 */
export const GROUP_PRIORITY_LABELS = {
    [GROUP_PRIORITY_LEVELS.HIGHEST]: '最高',
    [GROUP_PRIORITY_LEVELS.HIGH]: '高',
    [GROUP_PRIORITY_LEVELS.MEDIUM]: '中',
    [GROUP_PRIORITY_LEVELS.LOW]: '低',
    [GROUP_PRIORITY_LEVELS.LOWEST]: '最低'
};

/**
 * 组优先级颜色
 */
export const GROUP_PRIORITY_COLORS = {
    [GROUP_PRIORITY_LEVELS.HIGHEST]: '#dc2626',
    [GROUP_PRIORITY_LEVELS.HIGH]: '#f59e0b',
    [GROUP_PRIORITY_LEVELS.MEDIUM]: '#3b82f6',
    [GROUP_PRIORITY_LEVELS.LOW]: '#6b7280',
    [GROUP_PRIORITY_LEVELS.LOWEST]: '#9ca3af'
};

// ============================================================================
// 配置和限制常量
// ============================================================================

/**
 * 组数据刷新配置
 */
export const REFRESH_CONFIG = {
    // SSE连接失败时的备用刷新间隔（毫秒）
    FALLBACK_INTERVAL: 15000,
    // 操作后延迟刷新时间（毫秒）
    OPERATION_DELAY: 500,
    // 自动重试次数
    MAX_RETRY_COUNT: 3,
    // 重试间隔（毫秒）
    RETRY_INTERVAL: 2000
};

/**
 * UI配置常量
 */
export const UI_CONFIG = {
    // 组卡片最小宽度
    CARD_MIN_WIDTH: 350,
    // 卡片间距
    CARD_GAP: 20,
    // 组网格响应式断点
    BREAKPOINTS: {
        MOBILE: 768,
        TABLET: 1024,
        DESKTOP: 1200
    },
    // 动画持续时间
    ANIMATION_DURATION: {
        FAST: 200,
        NORMAL: 300,
        SLOW: 500
    }
};

/**
 * 操作超时配置
 */
export const OPERATION_TIMEOUTS = {
    // 激活操作超时（毫秒）
    ACTIVATE: 10000,
    // 暂停操作超时（毫秒）
    PAUSE: 10000,
    // 强制激活操作超时（毫秒）
    FORCE_ACTIVATE: 15000,
    // 数据加载超时（毫秒）
    LOAD_DATA: 10000
};

/**
 * 对话框类型枚举
 */
export const DIALOG_TYPES = {
    INFO: 'info',
    WARNING: 'warning',
    DANGER: 'danger',
    SUCCESS: 'success'
};

/**
 * 对话框默认配置
 */
export const DIALOG_DEFAULTS = {
    [DIALOG_TYPES.INFO]: {
        icon: 'ℹ️',
        confirmButtonColor: '#3b82f6'
    },
    [DIALOG_TYPES.WARNING]: {
        icon: '⚠️',
        confirmButtonColor: '#f59e0b'
    },
    [DIALOG_TYPES.DANGER]: {
        icon: '⚠️',
        confirmButtonColor: '#ef4444'
    },
    [DIALOG_TYPES.SUCCESS]: {
        icon: '✅',
        confirmButtonColor: '#10b981'
    }
};

// ============================================================================
// 数据验证常量
// ============================================================================

/**
 * 组名称验证规则
 */
export const GROUP_NAME_VALIDATION = {
    // 最小长度
    MIN_LENGTH: 1,
    // 最大长度
    MAX_LENGTH: 50,
    // 允许的字符正则表达式
    PATTERN: /^[a-zA-Z0-9_-]+$/,
    // 错误消息
    MESSAGES: {
        REQUIRED: '组名称不能为空',
        TOO_SHORT: '组名称至少需要1个字符',
        TOO_LONG: '组名称不能超过50个字符',
        INVALID_CHARS: '组名称只能包含字母、数字、下划线和连字符'
    }
};

/**
 * 优先级验证规则
 */
export const PRIORITY_VALIDATION = {
    // 最小值
    MIN_VALUE: 1,
    // 最大值
    MAX_VALUE: 100,
    // 默认值
    DEFAULT_VALUE: 5,
    // 错误消息
    MESSAGES: {
        INVALID_TYPE: '优先级必须是数字',
        OUT_OF_RANGE: '优先级必须在1-100之间',
        REQUIRED: '优先级不能为空'
    }
};

// ============================================================================
// SSE事件类型常量
// ============================================================================

/**
 * SSE事件类型
 */
export const SSE_EVENT_TYPES = {
    GROUP: 'group',
    ENDPOINT: 'endpoint',
    SYSTEM: 'system',
    ERROR: 'error'
};

/**
 * 组相关SSE事件子类型
 */
export const GROUP_SSE_EVENTS = {
    STATUS_CHANGED: 'status_changed',
    HEALTH_CHANGED: 'health_changed',
    COOLDOWN_STARTED: 'cooldown_started',
    COOLDOWN_ENDED: 'cooldown_ended',
    ACTIVATED: 'activated',
    PAUSED: 'paused',
    FORCE_ACTIVATED: 'force_activated'
};

// ============================================================================
// 默认值常量
// ============================================================================

/**
 * 组数据默认值
 */
export const GROUP_DEFAULTS = {
    name: '',
    active: false,
    healthy: false,
    in_cooldown: false,
    group_priority: GROUP_PRIORITY_LEVELS.MEDIUM,
    endpoints_count: 0,
    healthy_endpoints: 0,
    cooldown_remaining: 0,
    force_activation_available: false
};

/**
 * 统计数据默认值
 */
export const STATS_DEFAULTS = {
    total: 0,
    active: 0,
    paused: 0,
    cooldown: 0,
    healthy: 0,
    unhealthy: 0,
    activePercentage: 0,
    healthyPercentage: 0
};

/**
 * 挂起请求默认配置
 */
export const SUSPENDED_REQUESTS_DEFAULTS = {
    current: 0,
    max: 100,
    warningThreshold: 70,
    dangerThreshold: 90
};

// ============================================================================
// 本地存储键名常量
// ============================================================================

/**
 * 本地存储键名
 */
export const STORAGE_KEYS = {
    // 组页面配置
    GROUPS_PAGE_CONFIG: 'groupsPageConfig',
    // 组列表排序偏好
    GROUPS_SORT_PREFERENCE: 'groupsSortPreference',
    // 组筛选偏好
    GROUPS_FILTER_PREFERENCE: 'groupsFilterPreference',
    // 确认对话框偏好
    CONFIRM_DIALOG_PREFERENCE: 'confirmDialogPreference'
};

// ============================================================================
// 导出所有常量的集合对象（可选）
// ============================================================================

/**
 * 所有常量的集合对象
 */
export const GROUP_CONSTANTS = {
    STATUS: GROUP_STATUS,
    HEALTH: GROUP_HEALTH,
    OPERATIONS: GROUP_OPERATIONS,
    PRIORITY_LEVELS: GROUP_PRIORITY_LEVELS,
    DIALOG_TYPES,
    SSE_EVENT_TYPES,
    GROUP_SSE_EVENTS,
    REFRESH_CONFIG,
    UI_CONFIG,
    OPERATION_TIMEOUTS,
    DEFAULTS: {
        GROUP: GROUP_DEFAULTS,
        STATS: STATS_DEFAULTS,
        SUSPENDED_REQUESTS: SUSPENDED_REQUESTS_DEFAULTS
    },
    VALIDATION: {
        GROUP_NAME: GROUP_NAME_VALIDATION,
        PRIORITY: PRIORITY_VALIDATION
    },
    STORAGE_KEYS
};
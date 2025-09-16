/**
 * 组状态助手函数集合
 *
 * 负责：
 * - 组状态判断和计算逻辑
 * - 组数据处理和转换
 * - 状态样式和颜色计算
 * - 组排序和筛选逻辑
 *
 * 功能特性：
 * - 纯函数设计，易于测试
 * - 完整的状态判断逻辑
 * - 灵活的数据处理方法
 * - 一致的错误处理
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * 判断组是否处于活跃状态
 * @param {Object} group - 组对象
 * @returns {boolean} 是否活跃
 */
export const isGroupActive = (group) => {
    return !!(group && group.active && !group.in_cooldown);
};

/**
 * 判断组是否处于暂停状态
 * @param {Object} group - 组对象
 * @returns {boolean} 是否暂停
 */
export const isGroupPaused = (group) => {
    return !!(group && !group.active && !group.in_cooldown);
};

/**
 * 判断组是否处于冷却状态
 * @param {Object} group - 组对象
 * @returns {boolean} 是否冷却
 */
export const isGroupInCooldown = (group) => {
    return !!(group && group.in_cooldown);
};

/**
 * 判断组是否健康
 * @param {Object} group - 组对象
 * @returns {boolean} 是否健康
 */
export const isGroupHealthy = (group) => {
    return !!(group && group.healthy);
};

/**
 * 获取组的状态文本描述
 * @param {Object} group - 组对象
 * @returns {string} 状态描述
 */
export const getGroupStatusText = (group) => {
    if (!group) return '未知';

    if (group.in_cooldown) return '冷却中';
    if (group.active && group.healthy) return '活跃·健康';
    if (group.active && !group.healthy) return '活跃·异常';
    if (!group.active) return '已暂停';

    return '未知';
};

/**
 * 获取组状态对应的颜色
 * @param {Object} group - 组对象
 * @returns {Object} 颜色配置对象
 */
export const getGroupStatusColor = (group) => {
    if (!group) {
        return {
            primary: '#6b7280',
            background: '#f9fafb',
            border: '#e5e7eb'
        };
    }

    if (group.in_cooldown) {
        return {
            primary: '#ef4444',
            background: '#fef2f2',
            border: '#fecaca'
        };
    }

    if (group.active && group.healthy) {
        return {
            primary: '#10b981',
            background: '#f0fdf4',
            border: '#bbf7d0'
        };
    }

    if (group.active && !group.healthy) {
        return {
            primary: '#f59e0b',
            background: '#fffbeb',
            border: '#fde68a'
        };
    }

    // 暂停状态
    return {
        primary: '#6b7280',
        background: '#f9fafb',
        border: '#e5e7eb'
    };
};

/**
 * 获取组状态图标
 * @param {Object} group - 组对象
 * @returns {string} 状态图标
 */
export const getGroupStatusIcon = (group) => {
    if (!group) return '❓';

    if (group.in_cooldown) return '🧊';
    if (group.active && group.healthy) return '✅';
    if (group.active && !group.healthy) return '⚠️';
    if (!group.active) return '⏸️';

    return '❓';
};

/**
 * 计算组的健康度百分比
 * @param {Object} group - 组对象
 * @returns {number} 健康度百分比 (0-100)
 */
export const getGroupHealthPercentage = (group) => {
    if (!group || !group.endpoints_count || group.endpoints_count === 0) {
        return 0;
    }

    const healthyCount = group.healthy_endpoints || 0;
    const totalCount = group.endpoints_count;

    return ((healthyCount / totalCount) * 100).toFixed(1);
};

/**
 * 格式化冷却剩余时间
 * @param {number} seconds - 剩余秒数
 * @returns {string} 格式化的时间字符串
 */
export const formatCooldownTime = (seconds) => {
    if (!seconds || seconds <= 0) return '00:00';

    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;

    if (hours > 0) {
        return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    } else {
        return `${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }
};

/**
 * 判断组是否可以被激活
 * @param {Object} group - 组对象
 * @returns {boolean} 是否可以激活
 */
export const canActivateGroup = (group) => {
    return !!(group && !group.active && !group.in_cooldown);
};

/**
 * 判断组是否可以被暂停
 * @param {Object} group - 组对象
 * @returns {boolean} 是否可以暂停
 */
export const canPauseGroup = (group) => {
    return !!(group && group.active);
};

/**
 * 判断组是否可以强制激活
 * @param {Object} group - 组对象
 * @returns {boolean} 是否可以强制激活
 */
export const canForceActivateGroup = (group) => {
    return !!(group && group.force_activation_available && !group.active);
};

/**
 * 按优先级排序组列表
 * @param {Array} groups - 组列表
 * @param {boolean} ascending - 是否升序排序
 * @returns {Array} 排序后的组列表
 */
export const sortGroupsByPriority = (groups, ascending = true) => {
    if (!Array.isArray(groups)) return [];

    return [...groups].sort((a, b) => {
        const priorityA = a.group_priority || 0;
        const priorityB = b.group_priority || 0;

        return ascending ? priorityA - priorityB : priorityB - priorityA;
    });
};

/**
 * 按状态排序组列表
 * @param {Array} groups - 组列表
 * @returns {Array} 排序后的组列表（活跃 > 暂停 > 冷却）
 */
export const sortGroupsByStatus = (groups) => {
    if (!Array.isArray(groups)) return [];

    return [...groups].sort((a, b) => {
        // 状态优先级：活跃(3) > 暂停(2) > 冷却(1)
        const getStatusPriority = (group) => {
            if (group.in_cooldown) return 1;
            if (!group.active) return 2;
            if (group.active) return 3;
            return 0;
        };

        const priorityA = getStatusPriority(a);
        const priorityB = getStatusPriority(b);

        return priorityB - priorityA; // 降序排列
    });
};

/**
 * 筛选指定状态的组
 * @param {Array} groups - 组列表
 * @param {string} status - 状态类型 ('active', 'paused', 'cooldown', 'healthy', 'unhealthy')
 * @returns {Array} 筛选后的组列表
 */
export const filterGroupsByStatus = (groups, status) => {
    if (!Array.isArray(groups)) return [];

    switch (status) {
        case 'active':
            return groups.filter(group => isGroupActive(group));
        case 'paused':
            return groups.filter(group => isGroupPaused(group));
        case 'cooldown':
            return groups.filter(group => isGroupInCooldown(group));
        case 'healthy':
            return groups.filter(group => isGroupHealthy(group));
        case 'unhealthy':
            return groups.filter(group => !isGroupHealthy(group));
        default:
            return groups;
    }
};

/**
 * 搜索组（按名称）
 * @param {Array} groups - 组列表
 * @param {string} query - 搜索关键词
 * @returns {Array} 搜索结果
 */
export const searchGroups = (groups, query) => {
    if (!Array.isArray(groups) || !query) return groups;

    const lowerQuery = query.toLowerCase().trim();
    return groups.filter(group =>
        group.name && group.name.toLowerCase().includes(lowerQuery)
    );
};

/**
 * 计算组列表统计信息
 * @param {Array} groups - 组列表
 * @returns {Object} 统计信息对象
 */
export const calculateGroupsStats = (groups) => {
    if (!Array.isArray(groups) || groups.length === 0) {
        return {
            total: 0,
            active: 0,
            paused: 0,
            cooldown: 0,
            healthy: 0,
            unhealthy: 0,
            activePercentage: 0,
            healthyPercentage: 0
        };
    }

    const total = groups.length;
    const active = groups.filter(g => isGroupActive(g)).length;
    const paused = groups.filter(g => isGroupPaused(g)).length;
    const cooldown = groups.filter(g => isGroupInCooldown(g)).length;
    const healthy = groups.filter(g => isGroupHealthy(g)).length;
    const unhealthy = groups.filter(g => !isGroupHealthy(g)).length;

    return {
        total,
        active,
        paused,
        cooldown,
        healthy,
        unhealthy,
        activePercentage: total > 0 ? ((active / total) * 100).toFixed(1) : 0,
        healthyPercentage: total > 0 ? ((healthy / total) * 100).toFixed(1) : 0
    };
};

/**
 * 验证组对象的数据完整性
 * @param {Object} group - 组对象
 * @returns {Object} 验证结果
 */
export const validateGroupData = (group) => {
    const errors = [];
    const warnings = [];

    if (!group) {
        errors.push('组对象不能为空');
        return { isValid: false, errors, warnings };
    }

    // 必需字段检查
    if (!group.name || typeof group.name !== 'string') {
        errors.push('组名称必须是非空字符串');
    }

    if (typeof group.active !== 'boolean') {
        errors.push('active 字段必须是布尔值');
    }

    if (typeof group.healthy !== 'boolean') {
        warnings.push('healthy 字段应该是布尔值');
    }

    // 数值字段检查
    if (group.group_priority !== undefined && (!Number.isInteger(group.group_priority) || group.group_priority < 0)) {
        warnings.push('组优先级应该是非负整数');
    }

    if (group.endpoints_count !== undefined && (!Number.isInteger(group.endpoints_count) || group.endpoints_count < 0)) {
        warnings.push('端点数量应该是非负整数');
    }

    if (group.healthy_endpoints !== undefined && (!Number.isInteger(group.healthy_endpoints) || group.healthy_endpoints < 0)) {
        warnings.push('健康端点数量应该是非负整数');
    }

    // 逻辑一致性检查
    if (group.healthy_endpoints > group.endpoints_count) {
        warnings.push('健康端点数量不应该超过总端点数量');
    }

    return {
        isValid: errors.length === 0,
        errors,
        warnings
    };
};

/**
 * 获取组的优先级标签
 * @param {number} priority - 优先级数值
 * @returns {Object} 优先级标签信息
 */
export const getGroupPriorityLabel = (priority) => {
    if (priority === undefined || priority === null) {
        return { text: '未设置', color: '#6b7280' };
    }

    if (priority <= 1) {
        return { text: '最高', color: '#dc2626' };
    } else if (priority <= 3) {
        return { text: '高', color: '#f59e0b' };
    } else if (priority <= 5) {
        return { text: '中', color: '#3b82f6' };
    } else {
        return { text: '低', color: '#6b7280' };
    }
};
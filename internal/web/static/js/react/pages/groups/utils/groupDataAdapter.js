/**
 * 组数据适配器 - API数据结构兼容性处理
 *
 * 完全基于迁移计划设计，确保新React组件与现有工具函数的兼容性
 *
 * 功能特性：
 * - API数据结构到传统结构的双向转换
 * - 向后兼容性保证
 * - 数据完整性验证和修复
 * - 类型安全的数据处理
 * - 错误处理和降级策略
 *
 * 数据结构映射：
 * API结构 → 传统结构
 * - is_active → active
 * - total_endpoints → endpoints_count
 * - healthy_endpoints → healthy_endpoints (保持不变)
 * - in_cooldown → in_cooldown (保持不变)
 * - priority → group_priority
 * - _computed_health_status → healthy (计算布尔值)
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

import { GROUP_DEFAULTS } from './groupConstants.js';

/**
 * 将API数据结构转换为传统组件期望的数据结构
 * @param {Object} apiGroup - API响应的组数据
 * @returns {Object} 传统格式的组数据
 */
export const adaptAPIGroupToLegacy = (apiGroup) => {
  if (!apiGroup) {
    return { ...GROUP_DEFAULTS };
  }

  // 计算健康状态（基于健康端点数量）
  const isHealthy = apiGroup.total_endpoints > 0 &&
                   apiGroup.healthy_endpoints > 0 &&
                   apiGroup.healthy_endpoints === apiGroup.total_endpoints;

  return {
    // 保持原有字段
    name: apiGroup.name || '',
    in_cooldown: Boolean(apiGroup.in_cooldown),
    cooldown_remaining: apiGroup.cooldown_remaining || '0s',
    force_activation_available: Boolean(apiGroup.can_force_activate),

    // API字段映射
    active: Boolean(apiGroup.is_active),
    healthy: isHealthy,
    group_priority: apiGroup.priority || GROUP_DEFAULTS.group_priority,
    endpoints_count: apiGroup.total_endpoints || 0,
    healthy_endpoints: apiGroup.healthy_endpoints || 0,

    // 新增字段（保持API原始数据）
    is_active: Boolean(apiGroup.is_active),
    is_force_activated: Boolean(apiGroup.is_force_activated),
    total_endpoints: apiGroup.total_endpoints || 0,
    unhealthy_endpoints: apiGroup.unhealthy_endpoints || 0,
    can_activate: Boolean(apiGroup.can_activate),
    can_pause: Boolean(apiGroup.can_pause),
    can_resume: Boolean(apiGroup.can_resume),
    can_force_activate: Boolean(apiGroup.can_force_activate),
    force_activation_time: apiGroup.force_activation_time || '',
    priority: apiGroup.priority || GROUP_DEFAULTS.group_priority,
    _computed_health_status: apiGroup._computed_health_status || '未知'
  };
};

/**
 * 将传统数据结构转换为API期望的数据结构
 * @param {Object} legacyGroup - 传统格式的组数据
 * @returns {Object} API格式的组数据
 */
export const adaptLegacyGroupToAPI = (legacyGroup) => {
  if (!legacyGroup) {
    return {};
  }

  return {
    name: legacyGroup.name || '',
    priority: legacyGroup.group_priority || legacyGroup.priority || GROUP_DEFAULTS.group_priority,
    is_active: Boolean(legacyGroup.active || legacyGroup.is_active),
    in_cooldown: Boolean(legacyGroup.in_cooldown),
    cooldown_remaining: legacyGroup.cooldown_remaining || '0s',
    total_endpoints: legacyGroup.endpoints_count || legacyGroup.total_endpoints || 0,
    healthy_endpoints: legacyGroup.healthy_endpoints || 0,
    unhealthy_endpoints: legacyGroup.unhealthy_endpoints ||
                        Math.max(0, (legacyGroup.endpoints_count || 0) - (legacyGroup.healthy_endpoints || 0)),
    can_activate: Boolean(legacyGroup.can_activate),
    can_pause: Boolean(legacyGroup.can_pause),
    can_resume: Boolean(legacyGroup.can_resume),
    can_force_activate: Boolean(legacyGroup.can_force_activate || legacyGroup.force_activation_available),
    is_force_activated: Boolean(legacyGroup.is_force_activated),
    force_activation_time: legacyGroup.force_activation_time || '',
    _computed_health_status: legacyGroup._computed_health_status ||
                           (legacyGroup.healthy ? '健康' : '异常')
  };
};

/**
 * 批量适配API组数据列表
 * @param {Array} apiGroups - API响应的组数据列表
 * @returns {Array} 适配后的组数据列表
 */
export const adaptAPIGroupList = (apiGroups) => {
  if (!Array.isArray(apiGroups)) {
    return [];
  }

  return apiGroups.map(adaptAPIGroupToLegacy);
};

/**
 * 验证并修复组数据的完整性
 * @param {Object} group - 组数据对象
 * @returns {Object} 修复后的组数据
 */
export const validateAndRepairGroupData = (group) => {
  if (!group || typeof group !== 'object') {
    return { ...GROUP_DEFAULTS };
  }

  const repairedGroup = { ...group };

  // 修复字符串字段
  if (!repairedGroup.name || typeof repairedGroup.name !== 'string') {
    repairedGroup.name = GROUP_DEFAULTS.name;
  }

  // 修复布尔字段
  repairedGroup.is_active = Boolean(repairedGroup.is_active || repairedGroup.active);
  repairedGroup.in_cooldown = Boolean(repairedGroup.in_cooldown);
  repairedGroup.is_force_activated = Boolean(repairedGroup.is_force_activated);

  // 修复数值字段
  repairedGroup.priority = Math.max(1, parseInt(repairedGroup.priority || repairedGroup.group_priority) || GROUP_DEFAULTS.group_priority);
  repairedGroup.total_endpoints = Math.max(0, parseInt(repairedGroup.total_endpoints || repairedGroup.endpoints_count) || 0);
  repairedGroup.healthy_endpoints = Math.max(0, parseInt(repairedGroup.healthy_endpoints) || 0);
  repairedGroup.unhealthy_endpoints = Math.max(0, parseInt(repairedGroup.unhealthy_endpoints) || 0);

  // 逻辑一致性修复
  if (repairedGroup.healthy_endpoints > repairedGroup.total_endpoints) {
    repairedGroup.healthy_endpoints = repairedGroup.total_endpoints;
  }

  if (repairedGroup.total_endpoints === 0) {
    repairedGroup.healthy_endpoints = 0;
    repairedGroup.unhealthy_endpoints = 0;
  } else if (repairedGroup.unhealthy_endpoints === 0) {
    repairedGroup.unhealthy_endpoints = repairedGroup.total_endpoints - repairedGroup.healthy_endpoints;
  }

  // 修复状态字符串
  if (!repairedGroup._computed_health_status) {
    if (repairedGroup.total_endpoints === 0) {
      repairedGroup._computed_health_status = '无端点';
    } else if (repairedGroup.healthy_endpoints === repairedGroup.total_endpoints) {
      repairedGroup._computed_health_status = '健康';
    } else if (repairedGroup.healthy_endpoints > 0) {
      repairedGroup._computed_health_status = '部分健康';
    } else {
      repairedGroup._computed_health_status = '异常';
    }
  }

  // 修复操作权限字段
  repairedGroup.can_activate = Boolean(repairedGroup.can_activate);
  repairedGroup.can_pause = Boolean(repairedGroup.can_pause);
  repairedGroup.can_resume = Boolean(repairedGroup.can_resume);
  repairedGroup.can_force_activate = Boolean(repairedGroup.can_force_activate);

  return repairedGroup;
};

/**
 * 创建组状态CSS类名（兼容传统样式）
 * @param {Object} group - 组数据对象
 * @returns {string} CSS类名字符串
 */
export const getCompatibleGroupStatusClass = (group) => {
  if (!group) return 'inactive';

  if (group.in_cooldown) return 'cooldown';
  if (group.is_force_activated) return 'force-activated';
  if (group.is_active || group.active) {
    const healthyCount = group.healthy_endpoints || 0;
    return healthyCount > 0 ? 'active' : 'active-unhealthy';
  }
  return 'inactive';
};

/**
 * 获取组的用户友好状态描述
 * @param {Object} group - 组数据对象
 * @returns {string} 状态描述文本
 */
export const getGroupStatusDescription = (group) => {
  if (!group) return '未知状态';

  if (group.in_cooldown) {
    return `冷却中 (剩余: ${group.cooldown_remaining || '计算中...'})`;
  }

  if (group.is_force_activated) {
    return '应急激活状态';
  }

  if (group.is_active || group.active) {
    const healthyCount = group.healthy_endpoints || 0;
    const totalCount = group.total_endpoints || group.endpoints_count || 0;

    if (totalCount === 0) {
      return '活跃 (无端点)';
    } else if (healthyCount === totalCount) {
      return '活跃·健康';
    } else if (healthyCount > 0) {
      return `活跃·部分健康 (${healthyCount}/${totalCount})`;
    } else {
      return '活跃·异常';
    }
  }

  return '已暂停';
};

/**
 * 检查组是否处于可操作状态
 * @param {Object} group - 组数据对象
 * @param {string} operation - 操作类型 ('activate', 'pause', 'resume', 'force_activate')
 * @returns {boolean} 是否可操作
 */
export const canPerformGroupOperation = (group, operation) => {
  if (!group) return false;

  switch (operation) {
    case 'activate':
      return Boolean(group.can_activate) && !group.in_cooldown && !group.is_active;
    case 'pause':
      return Boolean(group.can_pause) && (group.is_active || group.active);
    case 'resume':
      return Boolean(group.can_resume);
    case 'force_activate':
      return Boolean(group.can_force_activate) && !group.in_cooldown;
    default:
      return false;
  }
};

/**
 * 计算组的健康度得分（0-100）
 * @param {Object} group - 组数据对象
 * @returns {number} 健康度得分
 */
export const calculateGroupHealthScore = (group) => {
  if (!group) return 0;

  const totalEndpoints = group.total_endpoints || group.endpoints_count || 0;
  const healthyEndpoints = group.healthy_endpoints || 0;

  if (totalEndpoints === 0) return 0;

  return Math.round((healthyEndpoints / totalEndpoints) * 100);
};

/**
 * 获取组的优先级标签和颜色
 * @param {Object} group - 组数据对象
 * @returns {Object} 包含text、color、icon的对象
 */
export const getGroupPriorityInfo = (group) => {
  const priority = group?.priority || group?.group_priority || 5;

  if (priority === 1) {
    return { text: '高', color: '#ef4444', icon: '🔥' };
  } else if (priority === 2) {
    return { text: '中高', color: '#f59e0b', icon: '⚡' };
  } else if (priority === 3) {
    return { text: '中', color: '#3b82f6', icon: '📋' };
  } else if (priority <= 5) {
    return { text: '低', color: '#6b7280', icon: '📌' };
  } else {
    return { text: '最低', color: '#9ca3af', icon: '📎' };
  }
};

/**
 * 合并多个组数据源（用于SSE更新）
 * @param {Object} currentGroup - 当前组数据
 * @param {Object} updateData - 更新数据
 * @returns {Object} 合并后的组数据
 */
export const mergeGroupData = (currentGroup, updateData) => {
  if (!currentGroup) return validateAndRepairGroupData(updateData);
  if (!updateData) return currentGroup;

  const merged = {
    ...currentGroup,
    ...updateData
  };

  // 确保关键字段的一致性
  merged.is_active = Boolean(merged.is_active || merged.active);
  merged.active = merged.is_active; // 向后兼容

  // 重新计算健康状态
  if (merged.total_endpoints > 0) {
    merged.healthy = merged.healthy_endpoints === merged.total_endpoints;
  } else {
    merged.healthy = false;
  }

  return validateAndRepairGroupData(merged);
};

export default {
  adaptAPIGroupToLegacy,
  adaptLegacyGroupToAPI,
  adaptAPIGroupList,
  validateAndRepairGroupData,
  getCompatibleGroupStatusClass,
  getGroupStatusDescription,
  canPerformGroupOperation,
  calculateGroupHealthScore,
  getGroupPriorityInfo,
  mergeGroupData
};
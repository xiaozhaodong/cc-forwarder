/**
 * GroupCard 组件 - 组管理界面核心卡片组件
 *
 * 完全基于迁移计划设计，匹配后端API数据结构和传统组管理功能
 *
 * 功能特性：
 * - 完整API数据结构支持（name, priority, is_active, in_cooldown等）
 * - 传统CSS类名保持（.group-info-card, .group-card-header等）
 * - 组状态可视化和动画效果
 * - 操作按钮集成（激活、暂停、恢复、应急激活）
 * - 冷却倒计时和应急激活信息显示
 * - 可访问性支持（aria-label等）
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

import React from 'react';
import GroupDetails from './GroupDetails.jsx';
import GroupActions from './GroupActions.jsx';

// 组状态助手函数 - 活跃组始终保持active样式
const getGroupStatusClass = (group) => {
  if (group.in_cooldown) return 'cooldown';
  if (group.is_force_activated) return 'force-activated';
  if (group.is_active) {
    return 'active'; // 活跃组无论健康状态如何都保持active样式
  }
  return 'inactive';
};

/**
 * GroupCard 组件 - 单个组卡片
 *
 * @param {Object} props - 组件属性
 * @param {Object} props.group - 组数据对象（匹配API响应结构）
 * @param {Function} props.onActivate - 激活组回调函数
 * @param {Function} props.onPause - 暂停组回调函数
 * @param {Function} props.onResume - 恢复组回调函数
 * @param {Function} props.onForceActivate - 应急激活组回调函数
 * @returns {JSX.Element} 组卡片JSX元素
 */
const GroupCard = ({
  group,
  onActivate,
  onPause,
  onResume,
  onForceActivate,
  className = ''
}) => {
  // 确保group对象有默认值，匹配API数据结构
  const groupData = {
    name: '',
    priority: 0,
    is_active: false,
    in_cooldown: false,
    cooldown_remaining: '0s',
    total_endpoints: 0,
    healthy_endpoints: 0,
    unhealthy_endpoints: 0,
    can_activate: false,
    can_pause: false,
    can_resume: false,
    can_force_activate: false,
    is_force_activated: false,
    force_activation_time: '',
    _computed_health_status: '未知',
    ...group
  };

  const statusClass = getGroupStatusClass(groupData);

  // 计算健康状态描述（匹配原版SSE逻辑）
  const getComputedHealthStatus = () => {
    if (groupData.healthy_endpoints === 0) {
      return '无健康端点';
    } else if (groupData.healthy_endpoints < groupData.total_endpoints) {
      return '部分健康';
    } else {
      return null; // 所有端点健康，使用原始状态
    }
  };

  const computedHealthStatus = getComputedHealthStatus();
  const displayStatus = computedHealthStatus || groupData.status ||
    (groupData.in_cooldown ? '冷却中' : (groupData.is_active ? '活跃' : '未激活'));

  return (
    <div
      className={`group-info-card ${statusClass} ${className}`}
      data-group-name={groupData.name}
      role="article"
      aria-label={`组 ${groupData.name} 信息卡片`}
    >
      <div className="group-card-header">
        <h3 className="group-name">{groupData.name}</h3>
        <span className={`group-status ${statusClass}`}>
          {displayStatus}
          {groupData.is_force_activated ? ' ⚡' : ''}
        </span>
      </div>

      <GroupDetails group={groupData} />

      <GroupActions
        group={groupData}
        onActivate={onActivate}
        onPause={onPause}
        onResume={onResume}
        onForceActivate={onForceActivate}
      />

      {groupData.in_cooldown && (
        <div className="group-cooldown-info">
          🕐 冷却剩余时间: {groupData.cooldown_remaining}
        </div>
      )}

      {groupData.is_force_activated && (
        <div className="group-force-activation-info">
          ⚡ 应急激活 - {groupData.force_activation_time || '时间未知'}
        </div>
      )}
    </div>
  );
};

export default GroupCard;
export { getGroupStatusClass };
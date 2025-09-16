/**
 * 状态指示器组件
 *
 * 负责：
 * - 根据端点状态显示健康、不健康、未检测状态
 * - 使用与原版本完全一致的CSS类名和HTML结构
 * - 提供视觉化的状态指示（颜色圆点 + 状态文本）
 * - 实时更新状态显示
 *
 * 创建日期: 2025-09-15 23:47:50
 * @author Claude Code Assistant
 *
 * 实现逻辑：
 * - 复用endpointsManager.js中的状态判断逻辑
 * - 保持与原版本相同的HTML结构：<span class="status-indicator ${statusClass}"></span>${statusText}
 * - 支持三种状态：未检测、健康、不健康
 */

import React from 'react';

/**
 * 状态指示器组件
 * @param {Object} props 组件属性
 * @param {Object} props.endpoint 端点数据对象，包含 never_checked 和 healthy 字段
 * @returns {JSX.Element} 状态指示器JSX元素
 */
const StatusIndicator = ({ endpoint }) => {
    // 根据 endpoint 对象计算状态类名和文本
    // 复用 endpointsManager.js 中的状态判断逻辑
    let statusClass, statusText;

    // 根据never_checked字段决定状态显示
    if (endpoint.never_checked) {
        statusClass = 'status-never-checked';
        statusText = '未检测';
    } else if (endpoint.healthy) {
        statusClass = 'status-healthy';
        statusText = '健康';
    } else {
        statusClass = 'status-unhealthy';
        statusText = '不健康';
    }

    return (
        <>
            <span className={`status-indicator ${statusClass}`}></span>
            {statusText}
        </>
    );
};

export default StatusIndicator;
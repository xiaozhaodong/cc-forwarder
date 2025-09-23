/**
 * GroupsGrid 组件 - 极简版本，完全匹配原版
 *
 * 只保留原版功能：
 * - 简单的组卡片列表显示
 * - 基本的加载和错误状态
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

import { useCallback } from 'react';
import GroupCard from './GroupCard.jsx';

/**
 * GroupsGrid组件 - 组卡片列表容器
 * 完全匹配原版groupsManager.js的简单实现
 *
 * @param {Object} props - 组件属性
 * @param {Array} props.groups - 组列表数据
 * @param {boolean} props.loading - 加载状态
 * @param {string} props.error - 错误信息
 * @param {Function} props.onActivate - 激活组回调
 * @param {Function} props.onPause - 暂停组回调
 * @param {Function} props.onResume - 恢复组回调
 * @param {Function} props.onForceActivate - 强制激活组回调
 * @returns {JSX.Element} 网格容器JSX元素
 */
const GroupsGrid = ({
    groups = [],
    loading = false,
    error = null,
    onActivate,
    onPause,
    onResume,
    onForceActivate
}) => {
    // 操作回调包装
    const handleGroupAction = useCallback((action, groupName) => {
        switch (action) {
            case 'activate':
                onActivate?.(groupName);
                break;
            case 'pause':
                onPause?.(groupName);
                break;
            case 'resume':
                onResume?.(groupName);
                break;
            case 'forceActivate':
                onForceActivate?.(groupName);
                break;
            default:
                console.warn(`未知的组操作: ${action}`);
        }
    }, [onActivate, onPause, onResume, onForceActivate]);

    // 加载状态
    if (loading) {
        return <p>加载中...</p>;
    }

    // 错误状态
    if (error) {
        return <div className="error">❌ 加载组信息失败: {error}</div>;
    }

    // 空状态
    if (!groups || groups.length === 0) {
        return <div className="info">📦 没有配置的组</div>;
    }

    // 渲染组卡片列表
    return (
        <div className="group-info-cards" id="group-info-cards">
            {groups.map((group) => (
                <GroupCard
                    key={group.name}
                    group={group}
                    onActivate={() => handleGroupAction('activate', group.name)}
                    onPause={() => handleGroupAction('pause', group.name)}
                    onResume={() => handleGroupAction('resume', group.name)}
                    onForceActivate={() => handleGroupAction('forceActivate', group.name)}
                />
            ))}
        </div>
    );
};

export default GroupsGrid;
/**
 * GroupStatus 组件 - 组状态指示器（匹配原版结构）
 *
 * 创建日期: 2025-09-16
 */

const GroupStatus = ({
    group,
    showText = true
}) => {
    const getStatusConfig = () => {
        if (group.in_cooldown) {
            return { text: '冷却中', className: 'cooldown' };
        }
        if (group.is_force_activated) {
            return { text: '应急激活 ⚡', className: 'force-activated' };
        }
        if (group.is_active && group.healthy_endpoints > 0) {
            return { text: '活跃·健康', className: 'active' };
        }
        if (group.is_active && group.healthy_endpoints === 0) {
            return { text: '活跃·异常', className: 'active' };
        }
        return { text: '已暂停', className: 'inactive' };
    };

    const statusConfig = getStatusConfig();

    return (
        <span className={`group-status ${statusConfig.className}`} role="status" aria-label={`组状态: ${statusConfig.text}`}>
            {showText && statusConfig.text}
        </span>
    );
};

export default GroupStatus;

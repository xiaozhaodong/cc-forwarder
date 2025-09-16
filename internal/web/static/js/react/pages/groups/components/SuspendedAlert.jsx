/**
 * 挂起请求警告组件 - 极简版本，完全匹配原版
 *
 * 只显示简单的文字提示，没有任何额外功能：
 * - 无进度条
 * - 无详情按钮
 * - 无模态框
 * - 只有基本的显示/隐藏逻辑
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * 挂起请求警告组件
 * 完全匹配原版groupsManager.js的updateGroupSuspendedAlert实现
 *
 * @param {Object} props - 组件属性
 * @param {number} props.totalSuspendedRequests - 总挂起请求数
 * @param {Object} props.groupSuspendedCounts - 各组挂起请求数
 * @returns {JSX.Element|null} 警告横幅JSX元素或null
 */
const SuspendedAlert = ({
    totalSuspendedRequests = 0,
    groupSuspendedCounts = {}
}) => {
    // 如果没有挂起请求，不显示
    if (totalSuspendedRequests <= 0) {
        return null;
    }

    // 生成消息文本，完全匹配原版逻辑
    let message = `当前有 ${totalSuspendedRequests} 个挂起请求等待处理`;

    const suspendedGroups = Object.entries(groupSuspendedCounts)
        .filter(([group, count]) => count > 0)
        .map(([group, count]) => `${group}(${count})`)
        .join(', ');

    if (suspendedGroups) {
        message += `，涉及组: ${suspendedGroups}`;
    }

    return (
        <div className="alert-banner" id="group-suspended-alert" style={{ display: 'flex' }}>
            <div className="alert-icon">⏸️</div>
            <div className="alert-content">
                <div className="alert-title">挂起请求通知</div>
                <div className="alert-message" id="suspended-alert-message">
                    {message}
                </div>
            </div>
            <button
                className="alert-close"
                onClick={() => {
                    const alertBanner = document.getElementById('group-suspended-alert');
                    if (alertBanner) {
                        alertBanner.style.display = 'none';
                    }
                }}
            >
                ×
            </button>
        </div>
    );
};

export default SuspendedAlert;
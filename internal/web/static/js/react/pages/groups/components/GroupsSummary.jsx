/**
 * 组统计概要组件 - 极简版本，完全匹配原版
 *
 * 只显示原版的3个指标：
 * - 总组数
 * - 活跃组数
 * - 健康端点总数
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * 组统计概要组件
 * 完全匹配原版groupsManager.js的summaryHtml
 *
 * @param {Object} props - 组件属性
 * @param {Object} props.data - 组数据对象
 * @returns {JSX.Element} 统计概要JSX元素
 */
const GroupsSummary = ({ data }) => {
    // 确保data存在，避免null引用错误
    if (!data) {
        return (
            <div className="groups-summary">
                <div className="summary-item">
                    <div className="summary-value">-</div>
                    <div className="summary-label">总组数</div>
                </div>
                <div className="summary-item">
                    <div className="summary-value">-</div>
                    <div className="summary-label">活跃组数</div>
                </div>
                <div className="summary-item">
                    <div className="summary-value">-</div>
                    <div className="summary-label">健康端点</div>
                </div>
            </div>
        );
    }

    const totalGroups = data.total_groups || 0;
    const activeGroups = data.active_groups || 0;
    const healthyEndpoints = data.groups ?
        data.groups.reduce((sum, g) => sum + (g.healthy_endpoints || 0), 0) : 0;

    return (
        <div className="groups-summary">
            <div className="summary-item">
                <div className="summary-value">{totalGroups}</div>
                <div className="summary-label">总组数</div>
            </div>
            <div className="summary-item">
                <div className="summary-value">{activeGroups}</div>
                <div className="summary-label">活跃组数</div>
            </div>
            <div className="summary-item">
                <div className="summary-value">{healthyEndpoints}</div>
                <div className="summary-label">健康端点</div>
            </div>
        </div>
    );
};

export default GroupsSummary;
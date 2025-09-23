/**
 * GroupDetails 组件 - 组详细信息展示（简化版本）
 *
 * 完全匹配原版HTML结构和CSS类名
 *
 * 数据映射：
 * - group.priority: 组优先级
 * - group.total_endpoints: 端点总数
 * - group.healthy_endpoints: 健康端点数
 * - group.unhealthy_endpoints: 不健康端点数
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * GroupDetails 组件 - 组详细信息
 *
 * @param {Object} props - 组件属性
 * @param {Object} props.group - 组数据对象（匹配API响应结构）
 * @returns {JSX.Element} 组详情JSX元素
 */
const GroupDetails = ({
  group
}) => {
  // 确保数据有默认值
  const {
    priority = 0,
    total_endpoints: totalEndpoints = 0,
    healthy_endpoints: healthyEndpoints = 0,
    unhealthy_endpoints: unhealthyEndpoints = 0
  } = group;

  return (
    <div className="group-details">
      <div className="group-detail-item">
        <div className="group-detail-label">优先级</div>
        <div className="group-detail-value group-priority">{priority}</div>
      </div>

      <div className="group-detail-item">
        <div className="group-detail-label">端点总数</div>
        <div className="group-detail-value">{totalEndpoints}</div>
      </div>

      <div className="group-detail-item">
        <div className="group-detail-label">健康端点</div>
        <div className="group-detail-value group-endpoints-count">{healthyEndpoints}</div>
      </div>

      <div className="group-detail-item">
        <div className="group-detail-label">不健康端点</div>
        <div className="group-detail-value group-unhealthy-count">{unhealthyEndpoints}</div>
      </div>
    </div>
  );
};

export default GroupDetails;
/**
 * 端点行组件 (单行数据)
 *
 * 负责：
 * - 渲染单个端点的数据行
 * - 显示端点的基本信息(名称、URL、状态等)
 * - 集成状态指示器、优先级编辑器和操作按钮
 * - 处理行级别的交互事件
 * - 与原版本endpointsManager.js完全一致的HTML表格结构
 *
 * 创建日期: 2025-09-15 23:47:50
 * 完整实现日期: 2025-09-16
 * @author Claude Code Assistant
 */

import { useRef } from 'react';
import StatusIndicator from './StatusIndicator.jsx';
import PriorityEditor from './PriorityEditor.jsx';
import ActionButtons from './ActionButtons.jsx';

/**
 * 端点行组件
 * @param {Object} props 组件属性
 * @param {Object} props.endpoint 端点数据对象，包含所有端点信息
 * @param {Function} props.onUpdatePriority 优先级更新回调函数 (endpointName, newPriority) => Promise
 * @param {Function} props.onHealthCheck 手动健康检测回调函数 (endpointName) => Promise
 * @returns {JSX.Element} 端点表格行JSX元素
 */
const EndpointRow = ({
    endpoint,
    onUpdatePriority,
    onHealthCheck
}) => {
    // 创建ref用于PriorityEditor和ActionButtons之间的通信
    const priorityEditorRef = useRef(null);

    // 数据验证
    if (!endpoint) {
        console.warn('EndpointRow: endpoint 数据为空');
        return null;
    }

    // 格式化组信息显示
    const formatGroupInfo = (endpoint) => {
        const group = endpoint.group || 'default';
        const groupPriority = endpoint.group_priority || 0;
        return `${group} (${groupPriority})`;
    };

    // 安全地获取端点数据，提供默认值
    const safeEndpoint = {
        name: endpoint.name || 'unknown',
        url: endpoint.url || '-',
        priority: endpoint.priority || 1,
        group: endpoint.group || 'default',
        group_priority: endpoint.group_priority || 0,
        response_time: endpoint.response_time || '-',
        last_check: endpoint.last_check || '-',
        healthy: endpoint.healthy || false,
        never_checked: endpoint.never_checked || false,
        ...endpoint
    };

    return (
        <tr>
            {/* 第1列：状态指示器 */}
            <td>
                <StatusIndicator endpoint={safeEndpoint} />
            </td>

            {/* 第2列：端点名称 */}
            <td>{safeEndpoint.name}</td>

            {/* 第3列：端点URL */}
            <td>{safeEndpoint.url}</td>

            {/* 第4列：优先级编辑器 */}
            <td>
                <PriorityEditor
                    ref={priorityEditorRef}
                    priority={safeEndpoint.priority}
                    endpointName={safeEndpoint.name}
                    onUpdate={onUpdatePriority}
                />
            </td>

            {/* 第5列：组信息 (组名 + 组优先级) */}
            <td>{formatGroupInfo(safeEndpoint)}</td>

            {/* 第6列：响应时间 */}
            <td>{safeEndpoint.response_time}</td>

            {/* 第7列：最后检查时间 */}
            <td>{safeEndpoint.last_check}</td>

            {/* 第8列：操作按钮 */}
            <td>
                <ActionButtons
                    endpoint={safeEndpoint}
                    onHealthCheck={onHealthCheck}
                    priorityEditorRef={priorityEditorRef}
                />
            </td>
        </tr>
    );
};

export default EndpointRow;
/**
 * RequestRow - 请求行组件
 * 文件描述: 显示单个请求记录的表格行（12列数据布局）
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';
import StatusBadge from './StatusBadge.jsx';
import {
    formatDuration,
    formatTimestamp,
    formatRequestId,
    formatModelName,
    formatEndpoint,
    formatStreamingIcon,
    formatCost
} from '../utils/requestsFormatter.jsx';

const RequestRow = ({ request, onClick }) => {
    // 处理行点击
    const handleClick = () => {
        if (onClick) {
            onClick(request);
        }
    };

    // 格式化Token数量 - 直接显示原始数字，不做单位转换
    const formatTokenCount = (tokens) => {
        if (!tokens || tokens === 0) return '0';  // 零值显示 "0" 而不是 "N/A"
        const num = parseInt(tokens);
        if (isNaN(num)) return 'N/A';
        return num.toString();  // 直接返回原始数字，与原版保持一致
    };

    return (
        <tr className="request-row" onClick={handleClick}>
            {/* 1. 请求ID */}
            <td className="request-id">
                <span className="id-text">
                    {formatStreamingIcon(request.isStreaming)}{' '}
                    {formatRequestId(request.requestId)}
                </span>
            </td>

            {/* 2. 时间 */}
            <td className="timestamp">
                {formatTimestamp(request.timestamp)}
            </td>

            {/* 3. 状态 */}
            <td className="status">
                <StatusBadge status={request.status} />
            </td>

            {/* 4. 模型 */}
            <td className="model">
                <span className="model-name">
                    {formatModelName(request.model)}
                </span>
            </td>

            {/* 5. 端点 */}
            <td className="endpoint">
                <span className="endpoint-name">{request.endpoint}</span>
            </td>

            {/* 6. 组 */}
            <td className="group">
                <span className="group-name">{request.group}</span>
            </td>

            {/* 7. 耗时 */}
            <td className="duration">
                {formatDuration(request.duration)}
            </td>

            {/* 8. 输入Tokens */}
            <td className="input-tokens">
                {formatTokenCount(request.inputTokens)}
            </td>

            {/* 9. 输出Tokens */}
            <td className="output-tokens">
                {formatTokenCount(request.outputTokens)}
            </td>

            {/* 10. 缓存创建Tokens */}
            <td className="cache-creation-tokens">
                {formatTokenCount(request.cacheCreationTokens)}
            </td>

            {/* 11. 缓存读取Tokens */}
            <td className="cache-read-tokens">
                {formatTokenCount(request.cacheReadTokens)}
            </td>

            {/* 12. 成本 */}
            <td className="cost">
                {formatCost(request.cost)}
            </td>
        </tr>
    );
};

export default RequestRow;
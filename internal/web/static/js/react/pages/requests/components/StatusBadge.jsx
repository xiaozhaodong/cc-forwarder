/**
 * StatusBadge - 状态徽章组件
 * 文件描述: 显示请求状态的彩色徽章
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';
import { getStatusConfig } from '../utils/requestsConstants.jsx';

const StatusBadge = ({ status }) => {
    const statusConfig = getStatusConfig(status);

    if (!statusConfig) {
        return (
            <span
                className="status-badge status-unknown"
                aria-label={`状态: ${status || '未知'}`}
                title={`状态: ${status || '未知'}`}
            >
                {status || 'unknown'}
            </span>
        );
    }

    return (
        <span
            className={`status-badge status-${statusConfig.type}`}
            aria-label={`状态: ${statusConfig.label}`}
            title={`状态: ${statusConfig.label}`}
        >
            <span className="status-icon" aria-hidden="true">{statusConfig.icon}</span>
            <span className="status-text">{statusConfig.label}</span>
        </span>
    );
};

export default StatusBadge;
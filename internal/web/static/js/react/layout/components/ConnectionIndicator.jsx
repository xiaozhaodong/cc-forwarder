// 连接状态指示器组件
import React from 'react';
import useSSE from '../../hooks/useSSE.jsx';

const ConnectionIndicator = () => {
    const { connectionStatus, connect } = useSSE();

    // 状态配置
    const statusConfig = {
        connected: {
            icon: '🟢',
            title: '已连接 - 实时数据更新中',
            className: 'connected'
        },
        reconnecting: {
            icon: '🟡',
            title: '重连中 - 正在尝试重新连接',
            className: 'connecting'
        },
        error: {
            icon: '🔴',
            title: '连接错误 - 数据可能不是最新的',
            className: 'error'
        },
        failed: {
            icon: '🔴',
            title: '连接失败 - 点击重连',
            className: 'error'
        },
        disconnected: {
            icon: '⚪',
            title: '未连接 - 点击重连',
            className: 'disconnected'
        }
    };

    const currentStatus = statusConfig[connectionStatus] || statusConfig.disconnected;

    const handleClick = () => {
        if (['error', 'failed', 'disconnected'].includes(connectionStatus)) {
            console.log('🔄 [ConnectionIndicator] 手动重连');
            connect();
        }
    };

    return (
        <div
            className={`connection-indicator ${currentStatus.className}`}
            title={currentStatus.title}
            onClick={handleClick}
            style={{ cursor: ['error', 'failed', 'disconnected'].includes(connectionStatus) ? 'pointer' : 'help' }}
        >
            {currentStatus.icon}
        </div>
    );
};

export default ConnectionIndicator;
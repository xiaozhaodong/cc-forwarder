// 连接统计详情组件 - 匹配原始Utils.generateConnectionsStats
// 2025-09-15 16:45:37

import React from 'react';

const ConnectionDetails = ({ data, isInitialized }) => {
    console.log('ConnectionDetails props:', { data, isInitialized });
    const { connections } = data;

    // 添加调试信息
    console.log('🔍 [ConnectionDetails] 接收到的data:', data);
    console.log('🔍 [ConnectionDetails] connections数据:', connections);
    console.log('🔍 [ConnectionDetails] isInitialized:', isInitialized);

    if (data.loading || !isInitialized) {
        return <p>加载中...</p>;
    }

    if (data.error) {
        return (
            <p style={{ color: '#ef4444' }}>
                错误: {data.error}
            </p>
        );
    }

    // 生成与原始Utils.generateConnectionsStats完全一致的HTML结构
    return (
        <div className="stats-grid">
            <div className="stat-item">
                <div className="stat-value">{connections.total_requests || 0}</div>
                <div className="stat-label">总请求数</div>
            </div>
            <div className="stat-item">
                <div className="stat-value">{connections.active_connections || 0}</div>
                <div className="stat-label">活跃连接</div>
            </div>
            <div className="stat-item">
                <div className="stat-value">{connections.successful_requests || 0}</div>
                <div className="stat-label">成功请求</div>
            </div>
            <div className="stat-item">
                <div className="stat-value">{connections.failed_requests || 0}</div>
                <div className="stat-label">失败请求</div>
            </div>
            <div className="stat-item">
                <div className="stat-value">{connections.average_response_time || '0s'}</div>
                <div className="stat-label">平均响应时间</div>
            </div>
            <div className="stat-item">
                <div className="stat-value">
                    {(() => {
                        const tokens = connections.total_tokens || 0;
                        if (tokens >= 1000000) {
                            return (tokens / 1000000).toFixed(1) + 'M';
                        } else if (tokens >= 1000) {
                            return (tokens / 1000).toFixed(1) + 'K';
                        } else {
                            return tokens.toString();
                        }
                    })()}
                </div>
                <div className="stat-label">Token使用量</div>
            </div>
        </div>
    );
};

export default ConnectionDetails;
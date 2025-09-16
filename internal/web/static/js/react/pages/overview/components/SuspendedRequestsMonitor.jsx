// 挂起请求监控组件 - 匹配原始HTML结构和功能
// 2025-09-15 16:45:37

import React from 'react';

const SuspendedRequestsMonitor = ({ data }) => {
    const suspendedData = data.connections.suspended || {};
    const suspendedConnections = data.connections.suspended_connections || [];
    const suspendedCount = suspendedData.suspended_requests || 0;
    const suspendedRate = suspendedData.success_rate || 0;

    if (data.loading) {
        return <p>加载中...</p>;
    }

    return (
        <div>
            {/* 警告横幅 - 有挂起时显示 */}
            {suspendedCount > 0 && (
                <div className="alert-banner warning">
                    <div className="alert-icon">⏸️</div>
                    <div className="alert-content">
                        <div className="alert-title">检测到挂起请求</div>
                        <div className="alert-message">
                            当前有 {suspendedCount} 个请求正在等待处理，系统正在尝试自动恢复
                        </div>
                    </div>
                </div>
            )}

            {/* 挂起请求状态标题 - 与原始HTML一致 */}
            <h4>⏸️ 挂起请求状态</h4>

            {/* 挂起请求统计卡片 - 与原始updateSuspendedStats匹配 */}
            <div id="suspended-stats" className="cards">
                <div className="card">
                    <h5>当前挂起</h5>
                    <p id="current-suspended">{suspendedData.suspended_requests || 0}</p>
                </div>

                <div className="card">
                    <h5>历史总数</h5>
                    <p id="total-suspended">{suspendedData.total_suspended_requests || 0}</p>
                </div>

                <div className="card">
                    <h5>成功恢复</h5>
                    <p id="successful-suspended">{suspendedData.successful_suspended_requests || 0}</p>
                </div>

                <div className="card">
                    <h5>超时失败</h5>
                    <p id="timeout-suspended">{suspendedData.timeout_suspended_requests || 0}</p>
                </div>

                <div className="card">
                    <h5>成功率</h5>
                    <p id="suspended-success-rate-detail">{suspendedRate.toFixed(1)}%</p>
                </div>

                <div className="card">
                    <h5>平均挂起时间</h5>
                    <p id="avg-suspended-time">{suspendedData.average_suspended_time || '0ms'}</p>
                </div>
            </div>

            {/* 当前挂起的连接列表标题 - 与原始HTML一致 */}
            <div id="suspended-connections-section">
                <h4>当前挂起的连接</h4>
                <div id="suspended-connections-table">
                    {suspendedConnections.length === 0 ? (
                        <p>无挂起连接</p>
                    ) : (
                        <div className="suspended-connections-list">
                            {suspendedConnections.map((conn, index) => (
                                <div key={index} className="suspended-connection-item">
                                    <div className="connection-header">
                                        <span className="connection-id">{conn.id}</span>
                                        <span className="suspended-time">{conn.suspended_for}</span>
                                    </div>
                                    <div className="connection-details">
                                        <div>端点: {conn.endpoint || '未知'}</div>
                                        <div>组: {conn.group || '未知'}</div>
                                        <div>模型: {conn.model || '未知'}</div>
                                        <div>状态: {conn.status || '挂起中'}</div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
};

export default SuspendedRequestsMonitor;
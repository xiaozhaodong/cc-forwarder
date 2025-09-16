// 状态卡片网格组件 - 使用原始HTML结构和CSS类名
// 2025-09-15 16:45:37

import React from 'react';

const StatusCardsGrid = ({ data }) => {
    console.log('StatusCardsGrid data:', data);
    const { status, endpoints, connections, groups } = data;

    // 挂起数据处理 - 匹配原始数据结构
    const suspendedData = connections.suspended || {};
    const suspendedCount = suspendedData.suspended_requests || 0;
    const totalSuspended = suspendedData.total_suspended_requests || 0;
    const suspendedRate = suspendedData.success_rate || 0;

    // 活动组信息 - 匹配原始逻辑
    const activeGroup = groups.groups ?
        groups.groups.find(group => group.is_active) : null;
    const activeGroupText = activeGroup ?
        `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)` :
        '无活跃组';

    return (
        <div className="cards">
            <div className="card">
                <h3>🚀 服务状态</h3>
                <p id="server-status">
                    {status.status === 'running' ? '🟢 运行中' : '🔴 已停止'}
                </p>
            </div>

            <div className="card">
                <h3>⏱️ 运行时间</h3>
                <p id="uptime">{status.uptime || '加载中...'}</p>
            </div>

            <div className="card">
                <h3>📡 端点数量</h3>
                <p id="endpoint-count">{endpoints.total || 0}</p>
            </div>

            <div className="card">
                <h3>🔗 总请求数</h3>
                <p id="total-requests">{connections.total_requests || 0}</p>
            </div>

            <div className="card">
                <h3>⏸️ 挂起请求</h3>
                <p id="suspended-requests">
                    {suspendedCount} / {totalSuspended}
                </p>
                <small id="suspended-success-rate" className={suspendedRate > 80 ? 'text-muted' : 'text-warning'}>
                    成功率: {suspendedRate.toFixed(1)}%
                </small>
            </div>

            <div className="card">
                <h3>🔄 当前活动组</h3>
                <p id="active-group">{activeGroupText}</p>
                {groups.total_suspended_requests > 0 && (
                    <small id="group-suspended-info" className="text-warning">
                        {groups.total_suspended_requests} 个挂起请求
                    </small>
                )}
            </div>
        </div>
    );
};

export default StatusCardsGrid;
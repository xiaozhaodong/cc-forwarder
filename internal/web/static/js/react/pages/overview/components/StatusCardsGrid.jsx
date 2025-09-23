// 状态卡片网格组件 - 使用原始HTML结构和CSS类名
// 2025-09-15 16:45:37
// 2025-09-22 更新：移除挂起请求显示，优化为单行布局

import React from 'react';

const StatusCardsGrid = ({ data }) => {
    console.log('StatusCardsGrid data:', data);
    const { status, endpoints, connections, groups } = data;

    // 活动组信息 - 匹配原始逻辑
    const activeGroup = groups.groups ?
        groups.groups.find(group => group.is_active) : null;
    const activeGroupText = activeGroup ?
        `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)` :
        '无活跃组';

    return (
        <>
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
                    <h3>🔄 当前活动组</h3>
                    <p id="active-group">{activeGroupText}</p>
                </div>
            </div>

            <style dangerouslySetInnerHTML={{
                __html: `
                .cards {
                    display: grid;
                    grid-template-columns: repeat(5, 1fr);
                    gap: 16px;
                    margin-bottom: 24px;
                }

                /* 响应式布局 */
                @media (max-width: 1200px) {
                    .cards {
                        grid-template-columns: repeat(3, 1fr);
                    }
                }

                @media (max-width: 768px) {
                    .cards {
                        grid-template-columns: repeat(2, 1fr);
                        gap: 12px;
                    }
                }

                @media (max-width: 480px) {
                    .cards {
                        grid-template-columns: 1fr;
                        gap: 8px;
                    }
                }
                `
            }} />
        </>
    );
};

export default StatusCardsGrid;
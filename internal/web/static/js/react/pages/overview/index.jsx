// 概览页面主组件 - 页面入口
// 2025-09-15 17:17:04

import React from 'react';
import useOverviewData from './hooks/useOverviewData.jsx';
import StatusCardsGrid from './components/StatusCardsGrid.jsx';
import ConnectionDetails from './components/ConnectionDetails.jsx';
import SuspendedRequestsMonitor from './components/SuspendedRequestsMonitor.jsx';
import CollapsibleSection from '../../components/ui/CollapsibleSection.jsx';

const OverviewPage = () => {
    const { data, refresh, isInitialized } = useOverviewData();

    // 错误状态渲染
    if (data.error) {
        return (
            <div style={{
                textAlign: 'center',
                padding: '48px 24px',
                color: '#ef4444'
            }}>
                <div style={{ fontSize: '48px', marginBottom: '16px' }}>
                    ❌
                </div>
                <h3 style={{ margin: '0 0 8px 0' }}>
                    数据加载失败
                </h3>
                <p style={{ margin: '0 0 16px 0' }}>
                    {data.error}
                </p>
                <button
                    onClick={refresh}
                    style={{
                        padding: '8px 16px',
                        backgroundColor: '#ef4444',
                        color: 'white',
                        border: 'none',
                        borderRadius: '4px',
                        cursor: 'pointer'
                    }}
                >
                    重试
                </button>
            </div>
        );
    }

    // 主要内容渲染 - 与原始版本结构完全一致
    return (
        <React.Fragment>
            {/* 状态卡片网格 - 直接使用原始结构，无额外标题 */}
            <StatusCardsGrid data={data} />

            {/* 连接统计详情 - 可折叠区域 */}
            <CollapsibleSection
                id="connection-details"
                title="🔗 连接统计详情"
                defaultExpanded={true}
            >
                <ConnectionDetails data={data} isInitialized={isInitialized} />
            </CollapsibleSection>

            {/* 挂起请求监控 - 可折叠区域，有挂起时自动展开 */}
            <CollapsibleSection
                id="suspended-monitoring"
                title="⏸️ 挂起请求监控"
                defaultExpanded={(data.connections.suspended?.suspended_requests || 0) > 0}
            >
                <SuspendedRequestsMonitor data={data} />
            </CollapsibleSection>
        </React.Fragment>
    );
};

export default OverviewPage;
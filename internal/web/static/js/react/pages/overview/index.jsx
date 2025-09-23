// 概览页面主组件 - 页面入口
// 2025-09-15 17:17:04
// 2025-09-22 更新：集成ChartsPanel图表融合方案

import React, { useState, useCallback } from 'react';
import useOverviewData from './hooks/useOverviewData.jsx';
import StatusCardsGrid from './components/StatusCardsGrid.jsx';
import ConnectionDetails from './components/ConnectionDetails.jsx';
import ChartsPanel from './components/ChartsPanel.jsx';
import CollapsibleSection from '../../components/ui/CollapsibleSection.jsx';

const OverviewPage = () => {
    const { data, refresh, isInitialized } = useOverviewData();

    // 图表时间范围状态管理
    const [chartTimeRange, setChartTimeRange] = useState(30); // 默认30分钟

    // 处理图表时间范围变化
    const handleTimeRangeChange = useCallback((newTimeRange) => {
        setChartTimeRange(newTimeRange);
    }, []);

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

    // 主要内容渲染 - 包含图表融合方案
    return (
        <React.Fragment>
            {/* 状态卡片网格 - 直接使用原始结构，无额外标题 */}
            <StatusCardsGrid data={data} />

            {/* 图表监控面板 - 包含两个独立折叠栏 */}
            <ChartsPanel
                timeRange={chartTimeRange}
                onTimeRangeChange={handleTimeRangeChange}
            />

            {/* 连接统计详情 - 可折叠区域 */}
            <CollapsibleSection
                id="connection-details"
                title="🔗 连接统计详情"
                defaultExpanded={true}
            >
                <ConnectionDetails data={data} isInitialized={isInitialized} />
            </CollapsibleSection>

        </React.Fragment>
    );
};

export default OverviewPage;
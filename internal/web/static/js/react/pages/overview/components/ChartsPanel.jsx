// ChartsPanel组件 - 概览页面图表面板
// 实现2×2重点图表网格 + 可折叠详细监控区域
// 创建时间: 2025-09-22

import React, { useState, useCallback } from 'react';
import ChartContainer from '../../charts/components/ChartContainer.jsx';
import CollapsibleSection from '../../../components/ui/CollapsibleSection.jsx';

const ChartsPanel = ({ timeRange, onTimeRangeChange }) => {
    // 时间范围选项配置
    const timeRangeOptions = [
        { value: 30, label: '30分钟', selected: true },
        { value: 60, label: '1小时', selected: false },
        { value: 180, label: '3小时', selected: false },
        { value: 360, label: '6小时', selected: false },
        { value: 720, label: '12小时', selected: false },
        { value: 1440, label: '24小时', selected: false }
    ];

    // 处理时间范围变化
    const handleTimeRangeChange = useCallback((newTimeRange) => {
        onTimeRangeChange?.(newTimeRange);
    }, [onTimeRangeChange]);

    // 组件挂载时添加样式
    React.useEffect(() => {
        addStylesToHead();
    }, []);

    return (
        <CollapsibleSection
            id="charts-monitoring"
            title="📊 图表监控"
            defaultExpanded={true}
        >
            {/* 重点图表区域 - 2×2网格布局 */}
            <div className="charts-primary-grid">
                {/* 第一行 */}
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="endpointCosts"
                        title="💰 当日端点Token使用成本"
                        timeRangeOptions={null} // 无时间范围选择
                        hasExport={true}
                        exportFilename="endpoint_costs.png"
                    />
                </div>
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="requestTrend"
                        title="📈 请求趋势"
                        timeRangeOptions={timeRangeOptions}
                        hasExport={true}
                        timeRange={timeRange}
                        onTimeRangeChange={handleTimeRangeChange}
                        exportFilename="request_trend.png"
                    />
                </div>

                {/* 第二行 */}
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="tokenUsage"
                        title="🎯 Token使用分布"
                        timeRangeOptions={null} // 无时间范围选择
                        hasExport={true}
                        exportFilename="token_usage.png"
                    />
                </div>
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="endpointHealth"
                        title="🏥 端点健康状态"
                        timeRangeOptions={null} // 无时间范围选择
                        hasExport={true}
                        exportFilename="endpoint_health.png"
                    />
                </div>
            </div>

            {/* 详细监控区域 - 2列网格布局 */}
            <div className="charts-detail-grid">
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="responseTime"
                        title="⚡ 响应时间"
                        timeRangeOptions={timeRangeOptions}
                        hasExport={true}
                        timeRange={timeRange}
                        onTimeRangeChange={handleTimeRangeChange}
                        exportFilename="response_time.png"
                    />
                </div>
                <div className="chart-grid-item">
                    <ChartContainer
                        chartType="connectionActivity"
                        title="🔌 连接活动"
                        timeRangeOptions={timeRangeOptions}
                        hasExport={true}
                        timeRange={timeRange}
                        onTimeRangeChange={handleTimeRangeChange}
                        exportFilename="connection_activity.png"
                    />
                </div>
            </div>
        </CollapsibleSection>
    );
};

// 添加样式到文档头部
const addStylesToHead = () => {
    const styleId = 'charts-panel-styles';
    if (!document.getElementById(styleId)) {
        const style = document.createElement('style');
        style.id = styleId;
        style.textContent = `
            /* ChartsPanel 组件样式 - 优化版本 */
            .charts-panel {
                padding: 20px 0;
            }

            .charts-primary-section {
                margin-bottom: 30px;
            }

            .charts-section-title {
                margin: 0 0 20px 0;
                font-size: 20px;
                font-weight: 600;
                color: var(--text-color, #374151);
                display: flex;
                align-items: center;
                gap: 8px;
            }

            /* 重点图表2×2网格布局 */
            .charts-primary-grid {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 20px;
                margin-bottom: 20px;
            }

            /* 详细图表2列网格布局 */
            .charts-detail-grid {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 20px;
                margin-top: 20px;
            }

            .chart-grid-item {
                background: var(--card-bg, white);
                border-radius: 12px;
                border: 1px solid var(--border-color, #e5e7eb);
                overflow: hidden;
                box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
                transition: all 0.3s ease;
                min-height: 350px; /* 桌面端默认高度 */
            }

            .chart-grid-item:hover {
                box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
                transform: translateY(-2px);
            }

            /* 图表容器内部样式调整 */
            .chart-grid-item .chart-container {
                height: 100%;
                box-shadow: none;
                border: none;
                border-radius: 0;
                margin: 0;
                background: transparent;
            }

            .chart-grid-item .chart-canvas {
                height: calc(100% - 80px); /* 减去header高度 */
                min-height: 270px;
            }

            /* 响应式设计 - 精确断点 */

            /* 桌面端 (≥1200px) - 2×2网格，350px高度 */
            @media (min-width: 1200px) {
                .charts-primary-grid {
                    grid-template-columns: 1fr 1fr;
                    gap: 20px;
                }

                .charts-detail-grid {
                    grid-template-columns: 1fr 1fr;
                    gap: 20px;
                }

                .chart-grid-item {
                    min-height: 350px;
                }

                .chart-grid-item .chart-canvas {
                    height: 300px;
                }
            }

            /* 平板端 (768px-1199px) - 2×2网格，320px高度 */
            @media (min-width: 768px) and (max-width: 1199px) {
                .charts-primary-grid {
                    grid-template-columns: 1fr 1fr;
                    gap: 20px;
                }

                .charts-detail-grid {
                    grid-template-columns: 1fr 1fr;
                    gap: 20px;
                }

                .chart-grid-item {
                    min-height: 320px;
                }

                .chart-grid-item .chart-canvas {
                    height: 270px;
                }

                .charts-section-title {
                    font-size: 18px;
                }
            }

            /* 移动端 (<768px) - 单列布局，280px高度 */
            @media (max-width: 767px) {
                .charts-panel {
                    padding: 15px 0;
                }

                .charts-primary-grid,
                .charts-detail-grid {
                    grid-template-columns: 1fr;
                    gap: 15px;
                }

                .chart-grid-item {
                    min-height: 280px;
                }

                .chart-grid-item .chart-canvas {
                    height: 230px;
                }

                .charts-section-title {
                    font-size: 16px;
                    margin-bottom: 15px;
                }

                .charts-primary-section {
                    margin-bottom: 25px;
                }
            }

            /* 超小屏幕优化 */
            @media (max-width: 480px) {
                .charts-panel {
                    padding: 10px 0;
                }

                .charts-primary-grid,
                .charts-detail-grid {
                    gap: 12px;
                }

                .chart-grid-item {
                    min-height: 260px;
                }

                .chart-grid-item .chart-canvas {
                    height: 210px;
                }

                .charts-section-title {
                    font-size: 14px;
                    margin-bottom: 12px;
                }
            }

            /* 加载状态和过渡动画 */
            .chart-grid-item.loading {
                opacity: 0.7;
                pointer-events: none;
            }

            .chart-grid-item.loading::after {
                content: '';
                position: absolute;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                width: 24px;
                height: 24px;
                border: 2px solid var(--border-color, #e5e7eb);
                border-top: 2px solid var(--primary-color, #2563eb);
                border-radius: 50%;
                animation: chartLoadingRotate 1s linear infinite;
                z-index: 10;
            }

            @keyframes chartLoadingRotate {
                0% { transform: translate(-50%, -50%) rotate(0deg); }
                100% { transform: translate(-50%, -50%) rotate(360deg); }
            }

            /* 折叠区域内的图表样式调整 */
            .collapsible-section .charts-detail-grid {
                margin-top: 0;
            }
        `;
        document.head.appendChild(style);
    }
};

export default ChartsPanel;

// 注册到全局组件
window.ReactComponents = window.ReactComponents || {};
window.ReactComponents.ChartsPanel = ChartsPanel;

// 兼容模块化导入
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ChartsPanel;
} else if (typeof window !== 'undefined') {
    window.ChartsPanel = ChartsPanel;
}
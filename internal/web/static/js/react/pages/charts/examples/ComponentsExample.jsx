// 图表UI组件使用示例
// 这个文件展示了如何使用新创建的图表UI组件

import React, { useState } from 'react';
import { TimeRangeSelector, ExportButton, ChartContainer } from './components/index.jsx';
import { useChartExport } from './hooks/index.jsx';

/**
 * 图表UI组件使用示例
 */
const ChartComponentsExample = () => {
    const [timeRange, setTimeRange] = useState(30);
    const { exportChart } = useChartExport();

    return (
        <div style={{ padding: '20px' }}>
            <h2>📊 图表UI组件使用示例</h2>

            {/* 1. 独立使用TimeRangeSelector */}
            <div style={{ marginBottom: '20px' }}>
                <h3>时间范围选择器</h3>
                <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
                    <span>当前选择：{timeRange}分钟</span>
                    <TimeRangeSelector
                        value={timeRange}
                        onChange={setTimeRange}
                        chartType="requestTrend"
                    />
                </div>
            </div>

            {/* 2. 独立使用ExportButton */}
            <div style={{ marginBottom: '20px' }}>
                <h3>导出按钮</h3>
                <div style={{ display: 'flex', gap: '10px' }}>
                    <ExportButton
                        chartName="requestTrend"
                        filename="请求趋势图.png"
                        title="导出请求趋势图"
                    />
                    <ExportButton
                        chartName="responseTime"
                        filename="响应时间图.png"
                        title="导出响应时间图"
                    />
                </div>
            </div>

            {/* 3. 使用完整的ChartContainer */}
            <div style={{ marginBottom: '20px' }}>
                <h3>完整图表容器</h3>
                <ChartContainer
                    chartType="requestTrend"
                    title="请求趋势"
                    timeRange={timeRange}
                    onTimeRangeChange={setTimeRange}
                    exportFilename="请求趋势图.png"
                    hasExport={true}
                />
            </div>

            {/* 4. 多个图表示例 */}
            <div>
                <h3>多个图表示例</h3>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))', gap: '20px' }}>
                    <ChartContainer
                        chartType="tokenUsage"
                        title="Token使用分布"
                        timeRangeOptions={null} // 无时间选择器
                        exportFilename="Token使用图.png"
                    />
                    <ChartContainer
                        chartType="endpointHealth"
                        title="端点健康状态"
                        timeRangeOptions={null} // 无时间选择器
                        exportFilename="端点健康图.png"
                    />
                </div>
            </div>
        </div>
    );
};

export default ChartComponentsExample;
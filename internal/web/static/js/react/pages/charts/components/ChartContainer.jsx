// ChartContainer组件 - 精确复制现有HTML结构和CSS类名
// 🎯 确保100%样式一致性，使用完全相同的CSS类名和DOM结构

import React, { useState, useEffect, useCallback } from 'react';
import TimeRangeSelector from './TimeRangeSelector.jsx';
import ExportButton from './ExportButton.jsx';
import ActualChart from './ActualChart.jsx';
import { getChartConfig } from '../utils/chartConfigs.jsx';

const ChartContainer = ({
    chartType,
    title,
    timeRangeOptions,
    hasExport = true,
    loading = false,
    timeRange,
    onTimeRangeChange,
    exportFilename
}) => {
    const [chartConfig, setChartConfig] = useState(null);
    const [chartInstance, setChartInstance] = useState(null);

    // 解析默认时间范围
    const resolveDefaultRange = useCallback(
        () => timeRangeOptions?.find(opt => opt.selected)?.value ?? timeRangeOptions?.[0]?.value ?? null,
        [timeRangeOptions]
    );

    // 本地状态管理选中的时间范围
    const [selectedRange, setSelectedRange] = useState(() =>
        typeof timeRange === 'number' ? timeRange : resolveDefaultRange()
    );

    // 同步外部timeRange变化
    useEffect(() => {
        if (typeof timeRange === 'number') {
            setSelectedRange(timeRange);
        } else if (selectedRange == null) {
            setSelectedRange(resolveDefaultRange());
        }
    }, [timeRange, resolveDefaultRange, selectedRange]);

    // 处理时间范围变化
    const handleRangeChange = useCallback(
        (minutes) => {
            setSelectedRange(minutes);
            onTimeRangeChange?.(minutes);
        },
        [onTimeRangeChange]
    );

    // 获取图表配置
    useEffect(() => {
        try {
            const config = getChartConfig(chartType);
            setChartConfig(config);
        } catch (error) {
            console.error(`❌ 加载图表配置失败 (${chartType}):`, error);
        }
    }, [chartType]);

    // 图表准备就绪回调
    const handleChartReady = (chartInstance, type) => {
        setChartInstance(chartInstance);
        console.log(`📊 图表实例就绪: ${type}`);
    };

    return (
        <div className="chart-container">
            <div className="chart-header">
                <div className="chart-title">{title}</div>
                <div className="chart-controls">
                    {timeRangeOptions && (
                        <TimeRangeSelector
                            value={selectedRange}
                            onChange={handleRangeChange}
                            chartType={chartType}
                            options={timeRangeOptions}
                        />
                    )}
                    {hasExport && (
                        <ExportButton
                            chartName={chartType}
                            filename={exportFilename || `${title}.png`}
                        />
                    )}
                </div>
            </div>
            <div className="chart-canvas">
                {/* 实际的Chart.js图表组件 */}
                {chartConfig && (
                    <ActualChart
                        chartType={chartType}
                        chartConfig={chartConfig}
                        onChartReady={handleChartReady}
                    />
                )}
                {loading && (
                    <div className="chart-loading">加载中...</div>
                )}
            </div>
        </div>
    );
};

// 导出组件
export default ChartContainer;

window.ReactComponents = window.ReactComponents || {};
window.ReactComponents.ChartContainer = ChartContainer;

// 兼容模块化导入
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ChartContainer;
} else if (typeof window !== 'undefined') {
    window.ChartContainer = ChartContainer;
}
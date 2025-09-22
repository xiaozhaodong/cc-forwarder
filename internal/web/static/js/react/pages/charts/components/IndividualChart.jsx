import React, { useEffect, useRef, useState } from 'react';
import { Chart as ChartJS } from 'chart.js';
import { chartConfigs } from '../utils/chartConfigs.jsx';
import { dataFetchers } from '../utils/chartDataService.jsx';
import { useSSECharts } from '../hooks/useSSECharts.jsx';
import { useChartGlobals } from '../hooks/useChartGlobals.jsx';

// 单个图表组件
// 集成Chart.js配置、数据获取和SSE更新功能
const IndividualChart = ({
    chartType,
    title,
    timeRangeOptions,
    hasExport = true,
    className = "",
    updateInterval = 30000 // 30秒更新间隔，作为SSE的备用机制
}) => {
    const canvasRef = useRef(null);
    const chartRef = useRef(null);
    const updateIntervalRef = useRef(null);

    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    // 使用全局配置和SSE功能
    const { isChartAvailable, showErrorState, showChartDisabledMessage } = useChartGlobals();
    const { registerChart, unregisterChart, updateTimeRange, exportChart, isSSEEnabled } = useSSECharts();

    // 获取图表配置和数据获取器
    const chartConfig = chartConfigs[chartType];
    const dataFetcher = dataFetchers[chartType];

    // 创建图表
    const createChart = async () => {
        if (!isChartAvailable() || !canvasRef.current || !chartConfig || !dataFetcher) {
            if (!isChartAvailable()) {
                showChartDisabledMessage(`#${chartType}Chart + .chart-loading`);
            } else {
                setError('图表配置或数据获取器不可用');
            }
            setLoading(false);
            return;
        }

        try {
            setLoading(true);
            setError(null);

            // 获取初始数据
            const data = await dataFetcher();

            // 创建Chart.js实例
            const ctx = canvasRef.current.getContext('2d');
            const chart = new ChartJS(ctx, {
                type: chartConfig.type,
                data: data,
                options: chartConfig.options
            });

            chartRef.current = chart;

            // 注册到SSE管理器
            registerChart(chartType, chart);

            setLoading(false);
            console.log(`📊 图表创建成功: ${chartType}`);
        } catch (error) {
            console.error(`创建图表失败 (${chartType}):`, error);
            setError('图表创建失败');
            setLoading(false);
        }
    };

    // 更新图表数据
    const updateChart = async () => {
        if (!chartRef.current || !dataFetcher) return;

        try {
            const newData = await dataFetcher();
            chartRef.current.data = newData;
            chartRef.current.update('none'); // 无动画更新
        } catch (error) {
            console.error(`更新图表数据失败 (${chartType}):`, error);
        }
    };

    // 处理时间范围变更
    const handleTimeRangeChange = (event) => {
        const minutes = parseInt(event.target.value);
        updateTimeRange(chartType, minutes);
    };

    // 处理导出
    const handleExport = () => {
        const filename = `${chartType}_${new Date().getTime()}.png`;
        exportChart(chartType, filename);
    };

    // 开始定时更新 (作为SSE的备用机制)
    const startRealTimeUpdates = () => {
        if (updateIntervalRef.current) {
            clearInterval(updateIntervalRef.current);
        }

        updateIntervalRef.current = setInterval(async () => {
            // 只有在没有SSE连接时才使用定时更新
            if (!isSSEEnabled()) {
                await updateChart();
            }
        }, updateInterval);
    };

    // 组件挂载时创建图表
    useEffect(() => {
        createChart();

        return () => {
            // 清理定时器
            if (updateIntervalRef.current) {
                clearInterval(updateIntervalRef.current);
            }

            // 注销图表
            if (chartRef.current) {
                unregisterChart(chartType);
                chartRef.current = null;
            }
        };
    }, [chartType]);

    // 开始实时更新
    useEffect(() => {
        if (!loading && !error) {
            startRealTimeUpdates();
        }

        return () => {
            if (updateIntervalRef.current) {
                clearInterval(updateIntervalRef.current);
            }
        };
    }, [loading, error, updateInterval]);

    return (
        <div className={`chart-container ${className}`}>
            <div className="chart-header">
                <div className="chart-title">{title}</div>
                <div className="chart-controls">
                    {timeRangeOptions && (
                        <select
                            className="chart-controls select"
                            onChange={handleTimeRangeChange}
                            disabled={loading}
                        >
                            {timeRangeOptions.map(option => (
                                <option
                                    key={option.value}
                                    value={option.value}
                                    defaultSelected={option.selected}
                                >
                                    {option.label}
                                </option>
                            ))}
                        </select>
                    )}
                    {hasExport && (
                        <button
                            className="chart-controls button"
                            title="导出图片"
                            onClick={handleExport}
                            disabled={loading || !chartRef.current}
                        >
                            📷
                        </button>
                    )}
                </div>
            </div>
            <div className="chart-canvas">
                <canvas
                    ref={canvasRef}
                    id={`${chartType}Chart`}
                    style={{ display: loading || error ? 'none' : 'block' }}
                />
                {loading && (
                    <div className="chart-loading">加载图表中...</div>
                )}
                {error && (
                    <div className="chart-loading" style={{ color: '#ef4444' }}>
                        {error}
                    </div>
                )}
            </div>
        </div>
    );
};

export default IndividualChart;
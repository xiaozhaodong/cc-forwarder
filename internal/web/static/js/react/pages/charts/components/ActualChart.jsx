// 实际图表组件 - 使用Chart.js渲染图表
import React, { useEffect, useRef, useState } from 'react';
import { fetchChartData } from '../utils/chartDataService.jsx';

const ActualChart = ({ chartType, chartConfig, data, onChartReady }) => {
    const canvasRef = useRef(null);
    const chartRef = useRef(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!canvasRef.current || !chartConfig) return;

        const initChart = async () => {
            try {
                setLoading(true);
                setError(null);

                // 检查Chart.js是否可用
                if (typeof window.Chart === 'undefined') {
                    throw new Error('Chart.js库未加载');
                }

                // 如果已存在图表实例，先销毁
                if (chartRef.current) {
                    chartRef.current.destroy();
                    chartRef.current = null;
                }

                // 获取数据
                let chartData = data;
                if (!chartData) {
                    // 从数据服务获取数据
                    chartData = await fetchChartData(chartType);
                }

                // 创建Chart.js实例
                const ctx = canvasRef.current.getContext('2d');
                chartRef.current = new window.Chart(ctx, {
                    type: chartConfig.type,
                    data: chartData,
                    options: chartConfig.options
                });

                // 通知父组件图表已准备就绪
                if (onChartReady) {
                    onChartReady(chartRef.current, chartType);
                }

                // 注册到全局chartManager（兼容现有系统）
                if (window.chartManager && window.chartManager.charts) {
                    window.chartManager.charts.set(chartType, chartRef.current);
                }

                setLoading(false);
                console.log(`✅ 图表渲染成功: ${chartType}`);

            } catch (err) {
                console.error(`❌ 图表渲染失败 (${chartType}):`, err);
                console.error(`❌ 详细错误信息:`, err.stack);
                console.error(`❌ 图表配置:`, chartConfig);
                console.error(`❌ 图表数据:`, chartData);
                console.error(`❌ Canvas元素:`, canvasRef.current);
                console.error(`❌ Chart.js可用性:`, typeof window.Chart);
                setError(`${err.message} - 请查看控制台获取详细信息`);
                setLoading(false);
            }
        };

        initChart();

        // 清理函数
        return () => {
            if (chartRef.current) {
                chartRef.current.destroy();
                chartRef.current = null;
            }
        };
    }, [chartType, chartConfig, data]);

    // 组件卸载时清理
    useEffect(() => {
        return () => {
            if (chartRef.current) {
                chartRef.current.destroy();
            }
        };
    }, []);

    if (error) {
        return (
            <div className="chart-loading" style={{ display: 'block', color: '#ef4444' }}>
                图表加载失败: {error}
            </div>
        );
    }

    return (
        <>
            <canvas ref={canvasRef} id={`${chartType}Chart`}></canvas>
            {loading && (
                <div className="chart-loading" style={{ display: 'block' }}>
                    加载中...
                </div>
            )}
        </>
    );
};

export default ActualChart;
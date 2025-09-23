import { useEffect, useRef, useCallback } from 'react';
import { chartTypeMapping } from '../utils/chartConfigs.jsx';

// SSE图表更新 Hook
// 精确复制自 internal/web/static/js/charts.js 的SSE事件处理逻辑
export const useSSECharts = () => {
    const chartsRef = useRef(new Map());
    const isDestroyedRef = useRef(false);
    const sseEnabledRef = useRef(false);

    // 映射图表类型到内部名称 - 精确复制原始映射逻辑
    const mapChartTypeToName = useCallback((chartType) => {
        return chartTypeMapping[chartType] || chartType;
    }, []);

    // 处理SSE图表更新事件 - 精确复制原始处理逻辑
    const handleSSEChartUpdate = useCallback((data) => {
        if (isDestroyedRef.current || !data.chart_type) return;

        const chartName = mapChartTypeToName(data.chart_type);
        const chart = chartsRef.current.get(chartName);

        if (chart && data.data) {
            // 使用平滑动画更新图表 - 精确复制原始更新机制
            chart.data = data.data;
            chart.update('active');
            console.log(`📊 SSE更新图表: ${chartName}`);
        }
    }, [mapChartTypeToName]);

    // 设置SSE事件监听器 - 精确复制原始事件监听机制
    const setupSSEListener = useCallback(() => {
        // 监听来自主WebInterface的SSE事件
        const eventHandler = (event) => {
            handleSSEChartUpdate(event.detail);
        };

        document.addEventListener('chartUpdate', eventHandler);

        // 返回清理函数
        return () => {
            document.removeEventListener('chartUpdate', eventHandler);
        };
    }, [handleSSEChartUpdate]);

    // 注册图表实例
    const registerChart = useCallback((chartName, chartInstance) => {
        if (!isDestroyedRef.current) {
            chartsRef.current.set(chartName, chartInstance);
            console.log(`📊 注册图表: ${chartName}`);
        }
    }, []);

    // 注销图表实例
    const unregisterChart = useCallback((chartName) => {
        const chart = chartsRef.current.get(chartName);
        if (chart) {
            chart.destroy();
            chartsRef.current.delete(chartName);
            console.log(`📊 注销图表: ${chartName}`);
        }
    }, []);

    // 获取图表实例
    const getChart = useCallback((chartName) => {
        return chartsRef.current.get(chartName);
    }, []);

    // 启用SSE更新 - 精确复制原始启用逻辑
    const enableSSEUpdates = useCallback(() => {
        sseEnabledRef.current = true;
        console.log('📊 启用SSE图表更新');
    }, []);

    // 禁用SSE更新 - 精确复制原始禁用逻辑
    const disableSSEUpdates = useCallback(() => {
        sseEnabledRef.current = false;
        console.log('📊 禁用SSE图表更新，切换到定时更新');
    }, []);

    // 检查SSE是否启用
    const isSSEEnabled = useCallback(() => {
        return sseEnabledRef.current;
    }, []);

    // 更新单个图表的时间范围 - 精确复制原始时间范围更新逻辑
    const updateTimeRange = useCallback(async (chartName, minutes) => {
        const chart = chartsRef.current.get(chartName);
        if (!chart) return;

        let newData;
        try {
            switch (chartName) {
                case 'requestTrend':
                    const response1 = await fetch(`/api/v1/chart/request-trends?minutes=${minutes}`);
                    if (!response1.ok) throw new Error(`HTTP ${response1.status}`);
                    newData = await response1.json();
                    break;
                case 'responseTime':
                    const response2 = await fetch(`/api/v1/chart/response-times?minutes=${minutes}`);
                    if (!response2.ok) throw new Error(`HTTP ${response2.status}`);
                    newData = await response2.json();
                    break;
                case 'connectionActivity':
                    const response3 = await fetch(`/api/v1/chart/connection-activity?minutes=${minutes}`);
                    if (!response3.ok) throw new Error(`HTTP ${response3.status}`);
                    newData = await response3.json();
                    break;
                default:
                    return;
            }

            chart.data = newData;
            chart.update('active');

            // 更新图表标题 - 精确复制原始标题更新逻辑
            const titleText = chart.options.plugins.title.text;
            const baseTitleText = titleText.split('(')[0].trim();
            chart.options.plugins.title.text = `${baseTitleText} (最近${minutes}分钟)`;
            chart.update();

        } catch (error) {
            console.error(`更新图表时间范围失败 (${chartName}):`, error);
        }
    }, []);

    // 导出图表为PNG - 精确复制原始导出逻辑
    const exportChart = useCallback((chartName, filename) => {
        const chart = chartsRef.current.get(chartName);
        if (!chart) {
            console.error(`图表 ${chartName} 不存在`);
            return;
        }

        try {
            // 创建下载链接
            const url = chart.toBase64Image('image/png', 1.0);
            const link = document.createElement('a');
            link.download = filename || `${chartName}_${new Date().getTime()}.png`;
            link.href = url;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);

            console.log(`📊 图表已导出: ${filename}`);
        } catch (error) {
            console.error('导出图表失败:', error);
            alert('导出图表失败，请稍后重试');
        }
    }, []);

    // 销毁所有图表 - 精确复制原始销毁逻辑
    const destroyCharts = useCallback(() => {
        isDestroyedRef.current = true;

        // 销毁所有图表实例
        chartsRef.current.forEach(chart => {
            chart.destroy();
        });
        chartsRef.current.clear();

        console.log('📊 图表管理器已销毁');
    }, []);

    // 初始化SSE监听器
    useEffect(() => {
        const cleanup = setupSSEListener();

        return () => {
            cleanup();
            destroyCharts();
        };
    }, [setupSSEListener, destroyCharts]);

    return {
        // 图表管理
        registerChart,
        unregisterChart,
        getChart,

        // SSE控制
        enableSSEUpdates,
        disableSSEUpdates,
        isSSEEnabled,

        // 图表操作
        updateTimeRange,
        exportChart,
        destroyCharts,

        // 事件处理
        handleSSEChartUpdate,
        mapChartTypeToName
    };
};
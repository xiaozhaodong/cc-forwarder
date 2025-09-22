import { useCallback } from 'react';

/**
 * 图表导出Hook
 * 精确复制现有导出功能逻辑
 */
const useChartExport = () => {
    /**
     * 导出图表为PNG图片
     * @param {string} chartName - 图表名称
     * @param {string} filename - 文件名
     * @param {Object} chartInstance - Chart.js图表实例 (可选，如果不传则从全局chartManager获取)
     */
    const exportChart = useCallback((chartName, filename, chartInstance = null) => {
        try {
            let chart = chartInstance;

            // 如果没有传入chart实例，尝试从全局chartManager获取
            if (!chart && window.chartManager) {
                chart = window.chartManager.charts?.get(chartName);
            }

            if (!chart) {
                console.error(`图表 ${chartName} 不存在`);
                // 显示错误信息 - 与原始实现保持一致
                if (window.webInterface?.showError) {
                    window.webInterface.showError(`图表 ${chartName} 不存在`);
                }
                return false;
            }

            // 使用Chart.js的toBase64Image方法导出图片
            // 与原始charts.js中的实现完全一致
            const url = chart.toBase64Image('image/png', 1.0);

            // 创建下载链接
            const link = document.createElement('a');
            link.download = filename || `${chartName}_${new Date().getTime()}.png`;
            link.href = url;

            // 临时添加到DOM并触发下载
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);

            console.log(`📷 图表导出成功: ${chartName} -> ${link.download}`);
            return true;
        } catch (error) {
            console.error('导出图表失败:', error);

            // 显示错误信息
            if (window.webInterface?.showError) {
                window.webInterface.showError('导出图表失败: ' + error.message);
            }
            return false;
        }
    }, []);

    /**
     * 批量导出多个图表
     * @param {Array} charts - 图表配置数组 [{chartName, filename, chartInstance?}]
     */
    const exportMultipleCharts = useCallback((charts) => {
        let successCount = 0;

        charts.forEach(({chartName, filename, chartInstance}) => {
            const success = exportChart(chartName, filename, chartInstance);
            if (success) successCount++;
        });

        console.log(`📷 批量导出完成: ${successCount}/${charts.length} 个图表导出成功`);
        return successCount;
    }, [exportChart]);

    return {
        exportChart,
        exportMultipleCharts
    };
};

export default useChartExport;
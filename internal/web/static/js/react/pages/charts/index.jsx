// React图表页面主组件
// 📊 完全复制现有HTML结构和CSS类名，确保100%样式一致性

import ChartContainer from './components/ChartContainer.jsx';

const ChartsPage = () => {

    // 时间范围选择处理器示例
    const handleTimeRangeChange = (chartType) => (minutes) => {
        console.log(`📊 ${chartType} 图表时间范围变更为: ${minutes}分钟`);
        // 这里会调用实际的图表更新逻辑
        if (window.chartManager) {
            window.chartManager.updateTimeRange(chartType, minutes);
        }
    };

    // 图表配置示例 - 端点Token成本和健康状态优先显示
    const chartConfigs = [
        {
            chartType: 'endpointCosts',
            title: '💰 当日端点Token使用成本',
            hasTimeRange: false,
            exportFilename: '端点Token成本图.png'
        },
        {
            chartType: 'endpointHealth',
            title: '端点健康状态',
            hasTimeRange: false,
            exportFilename: '端点健康图.png'
        },
        {
            chartType: 'tokenUsage',
            title: 'Token使用分布',
            hasTimeRange: false,
            exportFilename: 'Token使用图.png'
        },
        {
            chartType: 'requestTrend',
            title: '请求趋势',
            hasTimeRange: true,
            exportFilename: '请求趋势图.png',
            timeRangeOptions: [
                { value: 15, label: '15分钟' },
                { value: 30, label: '30分钟', selected: true },
                { value: 60, label: '1小时' },
                { value: 180, label: '3小时' }
            ]
        },
        {
            chartType: 'responseTime',
            title: '响应时间',
            hasTimeRange: true,
            exportFilename: '响应时间图.png',
            timeRangeOptions: [
                { value: 15, label: '15分钟' },
                { value: 30, label: '30分钟', selected: true },
                { value: 60, label: '1小时' },
                { value: 180, label: '3小时' }
            ]
        },
        {
            chartType: 'connectionActivity',
            title: '连接活动',
            hasTimeRange: true,
            exportFilename: '连接活动图.png',
            timeRangeOptions: [
                { value: 30, label: '30分钟' },
                { value: 60, label: '1小时', selected: true },
                { value: 180, label: '3小时' },
                { value: 360, label: '6小时' }
            ]
        }
    ];

    return (
        <div className="section">
            <h2>📈 数据可视化</h2>
            <div className="charts-grid">
                {chartConfigs.map(config => (
                    <ChartContainer
                        key={config.chartType}
                        chartType={config.chartType}
                        title={config.title}
                        timeRangeOptions={config.hasTimeRange ? config.timeRangeOptions : null}
                        hasExport={true}
                        exportFilename={config.exportFilename}
                        onTimeRangeChange={handleTimeRangeChange(config.chartType)}
                    />
                ))}
            </div>
        </div>
    );
};

// 导出组件
export default ChartsPage;

window.ReactComponents = window.ReactComponents || {};
window.ReactComponents.ChartsPage = ChartsPage;

// 兼容模块化导入
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ChartsPage;
} else if (typeof window !== 'undefined') {
    window.ChartsPage = ChartsPage;
}
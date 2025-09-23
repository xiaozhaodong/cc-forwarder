// 图表数据获取服务
// 精确复制自 internal/web/static/js/charts.js 的数据获取逻辑

// 获取空图表数据 - 内联函数避免循环依赖
const getEmptyChartData = (labels, datasetLabels) => {
    const colors = [
        'rgba(59, 130, 246, 1)', // blue
        'rgba(16, 185, 129, 1)', // green
        'rgba(239, 68, 68, 1)',  // red
        'rgba(245, 158, 11, 1)', // amber
        'rgba(139, 92, 246, 1)', // purple
        'rgba(236, 72, 153, 1)', // pink
    ];

    return {
        labels: [],
        datasets: datasetLabels.map((label, index) => ({
            label: label,
            data: [],
            borderColor: colors[index % colors.length],
            backgroundColor: colors[index % colors.length].replace('1)', '0.1)'),
            fill: false
        }))
    };
};

// 获取请求趋势数据 - 精确复制原始逻辑
export const fetchRequestTrendData = async () => {
    try {
        const response = await fetch('/api/v1/chart/request-trends?minutes=30');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('获取请求趋势数据失败:', error);
        return getEmptyChartData(['时间'], ['总请求数', '成功请求', '失败请求']);
    }
};

// 获取响应时间数据 - 精确复制原始逻辑
export const fetchResponseTimeData = async () => {
    try {
        const response = await fetch('/api/v1/chart/response-times?minutes=30');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('获取响应时间数据失败:', error);
        return getEmptyChartData(['时间'], ['平均响应时间', '最小响应时间', '最大响应时间']);
    }
};

// 获取Token使用数据 - 精确复制原始逻辑
export const fetchTokenUsageData = async () => {
    try {
        const response = await fetch('/api/v1/tokens/usage');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const tokenData = await response.json();

        const current = tokenData.current;
        return {
            labels: ['输入Token', '输出Token', '缓存创建Token', '缓存读取Token'],
            datasets: [{
                data: [
                    current.input_tokens,
                    current.output_tokens,
                    current.cache_creation_tokens,
                    current.cache_read_tokens
                ],
                backgroundColor: [
                    '#3b82f6',
                    '#10b981',
                    '#f59e0b',
                    '#8b5cf6'
                ],
                borderColor: [
                    '#2563eb',
                    '#059669',
                    '#d97706',
                    '#7c3aed'
                ],
                borderWidth: 2
            }]
        };
    } catch (error) {
        console.error('获取Token使用数据失败:', error);
        return {
            labels: ['输入Token', '输出Token', '缓存创建Token', '缓存读取Token'],
            datasets: [{
                data: [0, 0, 0, 0],
                backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6']
            }]
        };
    }
};

// 获取端点健康状态数据 - 精确复制原始逻辑
export const fetchEndpointHealthData = async () => {
    try {
        const response = await fetch('/api/v1/chart/endpoint-health');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('获取端点健康状态数据失败:', error);
        return {
            labels: ['健康端点', '不健康端点'],
            datasets: [{
                data: [0, 0],
                backgroundColor: ['#10b981', '#ef4444']
            }]
        };
    }
};

// 获取连接活动数据 - 精确复制原始逻辑
export const fetchConnectionActivityData = async () => {
    try {
        const response = await fetch('/api/v1/chart/connection-activity?minutes=60');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('获取连接活动数据失败:', error);
        return getEmptyChartData(['时间'], ['连接数']);
    }
};

// 获取端点性能数据 - 精确复制原始逻辑
export const fetchEndpointPerformanceData = async () => {
    try {
        const response = await fetch('/api/v1/endpoints/performance');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const perfData = await response.json();

        const endpoints = perfData.endpoints || [];
        const labels = endpoints.map(ep => ep.name);
        const responseTimeData = endpoints.map(ep => ({
            x: ep.avg_response_time,
            endpointData: ep
        }));

        // 根据健康状态设置颜色 - 精确复制原始颜色逻辑
        const backgroundColors = endpoints.map(ep =>
            ep.healthy ? '#10b981' : '#ef4444'
        );

        return {
            labels: labels,
            datasets: [{
                label: '平均响应时间',
                data: responseTimeData,
                backgroundColor: backgroundColors,
                borderColor: backgroundColors,
                borderWidth: 1
            }]
        };
    } catch (error) {
        console.error('获取端点性能数据失败:', error);
        return {
            labels: [],
            datasets: [{
                label: '平均响应时间',
                data: [],
                backgroundColor: []
            }]
        };
    }
};

// 获取悬停趋势数据 (如果有相应的API端点)
export const fetchSuspendedTrendData = async () => {
    try {
        // 注意：这个API端点可能需要根据实际后端实现调整
        const response = await fetch('/api/v1/chart/suspended-trends?minutes=30');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('获取悬停趋势数据失败:', error);
        return getEmptyChartData(['时间'], ['悬停请求数']);
    }
};

// 获取端点成本数据 - 新增函数
export const fetchEndpointCostsData = async () => {
    try {
        const response = await fetch('/api/v1/chart/endpoint-costs');
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();

        // ✅ 检查数据是否为空
        if (!data.labels || data.labels.length === 0) {
            return {
                labels: ['暂无数据'],
                datasets: [
                    {
                        label: 'Token使用量',
                        data: [0],
                        backgroundColor: ['rgba(156, 163, 175, 0.5)'],
                        borderColor: ['#9ca3af'],
                        borderWidth: 1,
                        yAxisID: 'tokens',
                        type: 'bar'
                    },
                    {
                        label: '成本 (USD)',
                        data: [0],
                        backgroundColor: 'transparent',
                        borderColor: '#9ca3af',
                        borderWidth: 2,
                        pointBackgroundColor: ['#9ca3af'],
                        pointBorderColor: ['#9ca3af'],
                        pointRadius: 4,
                        yAxisID: 'cost',
                        type: 'line',
                        fill: false,
                        tension: 0.4
                    }
                ]
            };
        }

        return data;
    } catch (error) {
        console.error('获取端点成本数据失败:', error);
        return {
            labels: ['加载失败'],
            datasets: [
                {
                    label: 'Token使用量',
                    data: [0],
                    backgroundColor: ['rgba(239, 68, 68, 0.3)'],
                    borderColor: ['#ef4444'],
                    borderWidth: 1,
                    yAxisID: 'tokens',
                    type: 'bar'
                },
                {
                    label: '成本 (USD)',
                    data: [0],
                    backgroundColor: 'transparent',
                    borderColor: '#ef4444',
                    borderWidth: 2,
                    pointBackgroundColor: ['#ef4444'],
                    pointBorderColor: ['#ef4444'],
                    pointRadius: 4,
                    yAxisID: 'cost',
                    type: 'line',
                    fill: false,
                    tension: 0.4
                }
            ]
        };
    }
};

// 数据获取函数映射
export const dataFetchers = {
    requestTrend: fetchRequestTrendData,
    responseTime: fetchResponseTimeData,
    tokenUsage: fetchTokenUsageData,
    endpointHealth: fetchEndpointHealthData,
    connectionActivity: fetchConnectionActivityData,
    endpointPerformance: fetchEndpointPerformanceData,
    suspendedTrend: fetchSuspendedTrendData,
    endpointCosts: fetchEndpointCostsData
};

// 批量获取所有图表数据
export const fetchAllChartsData = async () => {
    const results = {};

    try {
        const fetchPromises = Object.entries(dataFetchers).map(async ([chartName, fetcher]) => {
            try {
                const data = await fetcher();
                return [chartName, data];
            } catch (error) {
                console.error(`获取${chartName}数据失败:`, error);
                return [chartName, null];
            }
        });

        const responses = await Promise.all(fetchPromises);

        responses.forEach(([chartName, data]) => {
            results[chartName] = data;
        });

        console.log('📊 批量获取图表数据完成');
        return results;
    } catch (error) {
        console.error('批量获取图表数据失败:', error);
        return results;
    }
};

// 通用图表数据获取函数 - ActualChart组件使用
export const fetchChartData = async (chartType) => {
    const fetcher = dataFetchers[chartType];
    if (!fetcher) {
        console.warn(`⚠️ 未找到数据获取函数: ${chartType}`);
        return getEmptyChartData(['时间'], ['数据']);
    }

    try {
        const data = await fetcher();
        console.log(`✅ 数据获取成功: ${chartType}`);
        return data;
    } catch (error) {
        console.error(`❌ 数据获取失败: ${chartType}`, error);
        return getEmptyChartData(['时间'], ['数据']);
    }
};
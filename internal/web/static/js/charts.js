// Chart.js 数据可视化组件

class ChartManager {
    constructor() {
        this.charts = new Map();
        this.updateIntervals = new Map();
        this.isDestroyed = false;
        this.sseEnabled = false;
        
        // 检查Chart.js是否可用
        if (typeof Chart === 'undefined' || window.chartLoadFailed) {
            console.warn('Chart.js不可用，图表功能将被禁用');
            this.chartDisabled = true;
            return;
        }
        
        // Chart.js 默认配置
        Chart.defaults.responsive = true;
        Chart.defaults.maintainAspectRatio = false;
        Chart.defaults.plugins.legend.display = true;
        Chart.defaults.font.family = '"Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif';
        Chart.defaults.font.size = 12;
        
        // 设置中文
        Chart.defaults.locale = 'zh-CN';
        
        // 注册SSE事件监听器
        this.setupSSEListener();
    }

    // 设置SSE事件监听器
    setupSSEListener() {
        // 监听来自主WebInterface的SSE事件
        document.addEventListener('chartUpdate', (event) => {
            this.handleSSEChartUpdate(event.detail);
        });
    }

    // 处理SSE图表更新事件
    handleSSEChartUpdate(data) {
        if (this.isDestroyed || !data.chart_type) return;
        
        const chartName = this.mapChartTypeToName(data.chart_type);
        const chart = this.charts.get(chartName);
        
        if (chart && data.data) {
            // 使用平滑动画更新图表
            chart.data = data.data;
            chart.update('active');
            console.log(`📊 SSE更新图表: ${chartName}`);
        }
    }

    // 映射图表类型到内部名称
    mapChartTypeToName(chartType) {
        const mapping = {
            'request_trends': 'requestTrend',
            'response_times': 'responseTime',
            'token_usage': 'tokenUsage',
            'endpoint_health': 'endpointHealth',
            'connection_activity': 'connectionActivity',
            'endpoint_performance': 'endpointPerformance'
        };
        return mapping[chartType] || chartType;
    }

    // 初始化所有图表
    async initializeCharts() {
        if (this.isDestroyed || this.chartDisabled) {
            console.log('图表功能已禁用，跳过初始化');
            this.showChartDisabledMessage();
            return;
        }
        
        try {
            // 显示加载状态
            this.showLoadingState();
            
            // 初始化请求趋势图
            await this.createRequestTrendChart();
            
            // 初始化响应时间图
            await this.createResponseTimeChart();
            
            // 初始化Token使用饼图
            await this.createTokenUsageChart();
            
            // 初始化端点健康状态图
            await this.createEndpointHealthChart();
            
            // 初始化连接活动图
            await this.createConnectionActivityChart();
            
            // 初始化端点性能对比图
            await this.createEndpointPerformanceChart();
            
            // 隐藏加载状态
            this.hideLoadingState();
            
            // 开始实时更新（作为SSE的备用机制）
            this.startRealTimeUpdates();
            
            console.log('📊 所有图表初始化完成');
        } catch (error) {
            console.error('图表初始化失败:', error);
            this.showErrorState('图表初始化失败');
        }
    }

    // 显示加载状态
    showLoadingState() {
        document.querySelectorAll('.chart-loading').forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = '加载图表中...';
            loading.style.color = '#6b7280';
        });
    }

    // 隐藏加载状态
    hideLoadingState() {
        document.querySelectorAll('.chart-loading').forEach(loading => {
            loading.style.display = 'none';
        });
    }

    // 显示错误状态
    showErrorState(message) {
        document.querySelectorAll('.chart-loading').forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = message;
            loading.style.color = '#ef4444';
        });
    }

    // 显示图表禁用消息
    showChartDisabledMessage() {
        document.querySelectorAll('.chart-loading').forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = '图表功能暂不可用 (Chart.js加载失败)';
            loading.style.color = '#6b7280';
        });
    }

    // 创建请求趋势图
    async createRequestTrendChart() {
        const canvas = document.getElementById('requestTrendChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchRequestTrendData();

        const chart = new Chart(ctx, {
            type: 'line',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    x: {
                        title: {
                            display: true,
                            text: '时间'
                        },
                        grid: {
                            display: true,
                            color: 'rgba(0,0,0,0.1)'
                        }
                    },
                    y: {
                        title: {
                            display: true,
                            text: '请求数量'
                        },
                        beginAtZero: true,
                        grid: {
                            display: true,
                            color: 'rgba(0,0,0,0.1)'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: '请求趋势 (最近30分钟)',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        position: 'top'
                    },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1
                    }
                },
                interaction: {
                    intersect: false,
                    mode: 'index'
                },
                elements: {
                    line: {
                        tension: 0.3
                    },
                    point: {
                        radius: 3,
                        hoverRadius: 6
                    }
                },
                animation: {
                    duration: 1000,
                    easing: 'easeInOutQuart'
                }
            }
        });

        this.charts.set('requestTrend', chart);
    }

    // 创建响应时间图
    async createResponseTimeChart() {
        const canvas = document.getElementById('responseTimeChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchResponseTimeData();

        const chart = new Chart(ctx, {
            type: 'line',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    x: {
                        title: {
                            display: true,
                            text: '时间'
                        }
                    },
                    y: {
                        title: {
                            display: true,
                            text: '响应时间 (毫秒)'
                        },
                        beginAtZero: true
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: '响应时间趋势 (最近30分钟)',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        position: 'top'
                    },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1,
                        callbacks: {
                            label: function(context) {
                                return `${context.dataset.label}: ${context.parsed.y}ms`;
                            }
                        }
                    }
                },
                interaction: {
                    intersect: false,
                    mode: 'index'
                },
                elements: {
                    line: {
                        tension: 0.3
                    },
                    point: {
                        radius: 2,
                        hoverRadius: 5
                    }
                },
                animation: {
                    duration: 1000,
                    easing: 'easeInOutQuart'
                }
            }
        });

        this.charts.set('responseTime', chart);
    }

    // 创建Token使用饼图
    async createTokenUsageChart() {
        const canvas = document.getElementById('tokenUsageChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchTokenUsageData();

        const chart = new Chart(ctx, {
            type: 'doughnut',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: {
                        display: true,
                        text: 'Token使用分布',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        position: 'bottom',
                        labels: {
                            usePointStyle: true,
                            padding: 20
                        }
                    },
                    tooltip: {
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1,
                        callbacks: {
                            label: function(context) {
                                const label = context.label || '';
                                const value = context.parsed || 0;
                                const dataset = context.dataset;
                                const total = dataset.data.reduce((a, b) => a + b, 0);
                                const percentage = total > 0 ? ((value / total) * 100).toFixed(1) : '0';
                                return `${label}: ${value.toLocaleString()} (${percentage}%)`;
                            }
                        }
                    }
                },
                cutout: '40%',
                animation: {
                    animateRotate: true,
                    duration: 1500
                }
            }
        });

        this.charts.set('tokenUsage', chart);
    }

    // 创建端点健康状态图
    async createEndpointHealthChart() {
        const canvas = document.getElementById('endpointHealthChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchEndpointHealthData();

        const chart = new Chart(ctx, {
            type: 'pie',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: {
                        display: true,
                        text: '端点健康状态分布',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        position: 'bottom',
                        labels: {
                            usePointStyle: true,
                            padding: 20
                        }
                    },
                    tooltip: {
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1,
                        callbacks: {
                            label: function(context) {
                                const label = context.label || '';
                                const value = context.parsed || 0;
                                const total = context.dataset.data.reduce((a, b) => a + b, 0);
                                const percentage = total > 0 ? ((value / total) * 100).toFixed(1) : '0';
                                return `${label}: ${value} 个 (${percentage}%)`;
                            }
                        }
                    }
                },
                animation: {
                    animateRotate: true,
                    duration: 1500
                }
            }
        });

        this.charts.set('endpointHealth', chart);
    }

    // 创建连接活动图
    async createConnectionActivityChart() {
        const canvas = document.getElementById('connectionActivityChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchConnectionActivityData();

        const chart = new Chart(ctx, {
            type: 'bar',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    x: {
                        title: {
                            display: true,
                            text: '时间'
                        }
                    },
                    y: {
                        title: {
                            display: true,
                            text: '连接数'
                        },
                        beginAtZero: true
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: '连接活动 (最近1小时)',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        display: false
                    },
                    tooltip: {
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1
                    }
                },
                animation: {
                    duration: 1000,
                    easing: 'easeInOutQuart'
                }
            }
        });

        this.charts.set('connectionActivity', chart);
    }

    // 创建端点性能对比图
    async createEndpointPerformanceChart() {
        const canvas = document.getElementById('endpointPerformanceChart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = await this.fetchEndpointPerformanceData();

        const chart = new Chart(ctx, {
            type: 'bar',
            data: data,
            options: {
                responsive: true,
                maintainAspectRatio: false,
                indexAxis: 'y', // 水平条形图
                scales: {
                    x: {
                        title: {
                            display: true,
                            text: '平均响应时间 (毫秒)'
                        },
                        beginAtZero: true
                    },
                    y: {
                        title: {
                            display: true,
                            text: '端点'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: '端点性能对比',
                        font: { size: 16, weight: 'bold' }
                    },
                    legend: {
                        display: false
                    },
                    tooltip: {
                        backgroundColor: 'rgba(255, 255, 255, 0.95)',
                        titleColor: '#1f2937',
                        bodyColor: '#374151',
                        borderColor: '#e5e7eb',
                        borderWidth: 1,
                        callbacks: {
                            afterLabel: function(context) {
                                const endpointData = context.raw.endpointData;
                                if (endpointData) {
                                    return [
                                        `成功率: ${endpointData.success_rate.toFixed(1)}%`,
                                        `总请求数: ${endpointData.total_requests}`,
                                        `健康状态: ${endpointData.healthy ? '健康' : '不健康'}`
                                    ];
                                }
                                return [];
                            }
                        }
                    }
                },
                animation: {
                    duration: 1000,
                    easing: 'easeInOutQuart'
                }
            }
        });

        this.charts.set('endpointPerformance', chart);
    }

    // 获取请求趋势数据
    async fetchRequestTrendData() {
        try {
            const response = await fetch('/api/v1/chart/request-trends?minutes=30');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            return data;
        } catch (error) {
            console.error('获取请求趋势数据失败:', error);
            return this.getEmptyChartData(['时间'], ['总请求数', '成功请求', '失败请求']);
        }
    }

    // 获取响应时间数据
    async fetchResponseTimeData() {
        try {
            const response = await fetch('/api/v1/chart/response-times?minutes=30');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            return data;
        } catch (error) {
            console.error('获取响应时间数据失败:', error);
            return this.getEmptyChartData(['时间'], ['平均响应时间', '最小响应时间', '最大响应时间']);
        }
    }

    // 获取Token使用数据
    async fetchTokenUsageData() {
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
    }

    // 获取端点健康状态数据
    async fetchEndpointHealthData() {
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
    }

    // 获取连接活动数据
    async fetchConnectionActivityData() {
        try {
            const response = await fetch('/api/v1/chart/connection-activity?minutes=60');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            return data;
        } catch (error) {
            console.error('获取连接活动数据失败:', error);
            return this.getEmptyChartData(['时间'], ['连接数']);
        }
    }

    // 获取端点性能数据
    async fetchEndpointPerformanceData() {
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
            
            // 根据健康状态设置颜色
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
    }

    // 获取空图表数据
    getEmptyChartData(labels, datasetLabels) {
        return {
            labels: [],
            datasets: datasetLabels.map((label, index) => ({
                label: label,
                data: [],
                borderColor: this.getColorByIndex(index),
                backgroundColor: this.getColorByIndex(index, 0.1),
                fill: false
            }))
        };
    }

    // 根据索引获取颜色
    getColorByIndex(index, alpha = 1) {
        const colors = [
            `rgba(59, 130, 246, ${alpha})`, // blue
            `rgba(16, 185, 129, ${alpha})`, // green
            `rgba(239, 68, 68, ${alpha})`,  // red
            `rgba(245, 158, 11, ${alpha})`, // amber
            `rgba(139, 92, 246, ${alpha})`, // purple
            `rgba(236, 72, 153, ${alpha})`, // pink
        ];
        return colors[index % colors.length];
    }

    // 开始实时更新
    startRealTimeUpdates() {
        // 每30秒更新一次图表数据（作为SSE的备用机制）
        const updateInterval = setInterval(async () => {
            if (this.isDestroyed) {
                clearInterval(updateInterval);
                return;
            }
            
            // 只有在没有SSE连接时才使用定时更新
            if (!this.sseEnabled) {
                await this.updateAllCharts();
            }
        }, 30000);
        
        this.updateIntervals.set('main', updateInterval);
    }

    // 更新所有图表
    async updateAllCharts() {
        if (this.isDestroyed) return;
        
        try {
            const updatePromises = [];
            
            // 并行更新所有图表
            if (this.charts.has('requestTrend')) {
                updatePromises.push(this.updateRequestTrendChart());
            }
            if (this.charts.has('responseTime')) {
                updatePromises.push(this.updateResponseTimeChart());
            }
            if (this.charts.has('tokenUsage')) {
                updatePromises.push(this.updateTokenUsageChart());
            }
            if (this.charts.has('endpointHealth')) {
                updatePromises.push(this.updateEndpointHealthChart());
            }
            if (this.charts.has('connectionActivity')) {
                updatePromises.push(this.updateConnectionActivityChart());
            }
            if (this.charts.has('endpointPerformance')) {
                updatePromises.push(this.updateEndpointPerformanceChart());
            }
            
            await Promise.all(updatePromises);
            console.log('📊 图表数据更新完成');
        } catch (error) {
            console.error('更新图表数据失败:', error);
        }
    }

    // 更新请求趋势图
    async updateRequestTrendChart() {
        const chart = this.charts.get('requestTrend');
        if (!chart) return;
        
        const newData = await this.fetchRequestTrendData();
        chart.data = newData;
        chart.update('none'); // 无动画更新
    }

    // 更新响应时间图
    async updateResponseTimeChart() {
        const chart = this.charts.get('responseTime');
        if (!chart) return;
        
        const newData = await this.fetchResponseTimeData();
        chart.data = newData;
        chart.update('none');
    }

    // 更新Token使用图
    async updateTokenUsageChart() {
        const chart = this.charts.get('tokenUsage');
        if (!chart) return;
        
        const newData = await this.fetchTokenUsageData();
        chart.data = newData;
        chart.update('none');
    }

    // 更新端点健康状态图
    async updateEndpointHealthChart() {
        const chart = this.charts.get('endpointHealth');
        if (!chart) return;
        
        const newData = await this.fetchEndpointHealthData();
        chart.data = newData;
        chart.update('none');
    }

    // 更新连接活动图
    async updateConnectionActivityChart() {
        const chart = this.charts.get('connectionActivity');
        if (!chart) return;
        
        const newData = await this.fetchConnectionActivityData();
        chart.data = newData;
        chart.update('none');
    }

    // 更新端点性能图
    async updateEndpointPerformanceChart() {
        const chart = this.charts.get('endpointPerformance');
        if (!chart) return;
        
        const newData = await this.fetchEndpointPerformanceData();
        chart.data = newData;
        chart.update('none');
    }

    // 导出图表为PNG
    exportChart(chartName, filename) {
        const chart = this.charts.get(chartName);
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
    }

    // 切换图表的时间范围
    async updateTimeRange(chartName, minutes) {
        const chart = this.charts.get(chartName);
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
            
            // 更新图表标题
            const titleText = chart.options.plugins.title.text;
            const baseTitleText = titleText.split('(')[0].trim();
            chart.options.plugins.title.text = `${baseTitleText} (最近${minutes}分钟)`;
            chart.update();
            
        } catch (error) {
            console.error(`更新图表时间范围失败 (${chartName}):`, error);
        }
    }

    // 启用SSE更新
    enableSSEUpdates() {
        this.sseEnabled = true;
        console.log('📊 启用SSE图表更新');
    }

    // 禁用SSE更新
    disableSSEUpdates() {
        this.sseEnabled = false;
        console.log('📊 禁用SSE图表更新，切换到定时更新');
    }

    // 销毁所有图表
    destroy() {
        this.isDestroyed = true;
        
        // 清除更新定时器
        this.updateIntervals.forEach(interval => {
            clearInterval(interval);
        });
        this.updateIntervals.clear();
        
        // 销毁所有图表实例
        this.charts.forEach(chart => {
            chart.destroy();
        });
        this.charts.clear();
        
        console.log('📊 图表管理器已销毁');
    }
}

// 导出给全局使用
window.ChartManager = ChartManager;
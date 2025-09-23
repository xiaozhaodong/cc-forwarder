// Chart.js 图表配置
// 精确复制自 internal/web/static/js/charts.js 的配置

// 根据索引获取颜色的工具函数
export const getColorByIndex = (index, alpha = 1) => {
    const colors = [
        `rgba(59, 130, 246, ${alpha})`, // blue
        `rgba(16, 185, 129, ${alpha})`, // green
        `rgba(239, 68, 68, ${alpha})`,  // red
        `rgba(245, 158, 11, ${alpha})`, // amber
        `rgba(139, 92, 246, ${alpha})`, // purple
        `rgba(236, 72, 153, ${alpha})`, // pink
    ];
    return colors[index % colors.length];
};

// 获取空图表数据
export const getEmptyChartData = (labels, datasetLabels) => {
    return {
        labels: [],
        datasets: datasetLabels.map((label, index) => ({
            label: label,
            data: [],
            borderColor: getColorByIndex(index),
            backgroundColor: getColorByIndex(index, 0.1),
            fill: false
        }))
    };
};

// 1. 请求趋势图配置
export const requestTrendConfig = {
    type: 'line',
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
};

// 2. 响应时间图配置
export const responseTimeConfig = {
    type: 'line',
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
                        return `${context.dataset.label}: ${context.parsed.y.toFixed(2)}ms`;
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
};

// 3. Token使用饼图配置
export const tokenUsageConfig = {
    type: 'doughnut',
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
};

// 4. 端点健康状态图配置
export const endpointHealthConfig = {
    type: 'pie',
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
};

// 5. 连接活动图配置
export const connectionActivityConfig = {
    type: 'bar',
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
};

// 6. 端点Token使用成本分析图配置
export const endpointCostsConfig = {
    type: 'bar', // 基础类型为条形图，成本数据会覆盖为线图
    options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
            x: {
                title: {
                    display: true,
                    text: '端点'
                },
                grid: {
                    display: true,
                    color: 'rgba(0,0,0,0.1)'
                },
                ticks: {
                    maxRotation: 45,     // 标签最大旋转角度
                    minRotation: 0,      // 标签最小旋转角度
                    autoSkip: false,     // 禁用自动跳过标签
                    maxTicksLimit: 10,   // 最大显示标签数
                    font: {
                        size: 11         // 字体大小
                    },
                    callback: function(value) {
                        // 处理长标签，超过20字符时截断
                        const label = this.getLabelForValue(value);
                        return label.length > 20 ? label.substring(0, 17) + '...' : label;
                    }
                }
            },
            tokens: {
                type: 'linear',
                display: true,
                position: 'left',
                title: {
                    display: true,
                    text: 'Token使用量',
                    color: '#6366f1',
                    font: { weight: 'bold' }
                },
                beginAtZero: true,
                grid: {
                    display: true,
                    color: 'rgba(99, 102, 241, 0.1)'
                },
                ticks: {
                    color: '#6366f1',
                    callback: function(value) {
                        return value.toLocaleString() + ' tokens';
                    }
                }
            },
            cost: {
                type: 'linear',
                display: true,
                position: 'right',
                title: {
                    display: true,
                    text: '成本 (USD)',
                    color: '#dc2626',
                    font: { weight: 'bold' }
                },
                beginAtZero: true,
                grid: {
                    drawOnChartArea: false,
                },
                ticks: {
                    color: '#dc2626',
                    callback: function(value) {
                        return '$' + value.toFixed(2);
                    }
                }
            }
        },
        plugins: {
            title: {
                display: true,
                text: '💰 当日端点Token使用成本',
                font: { size: 16, weight: 'bold' }
            },
            legend: {
                position: 'top',
                labels: {
                    usePointStyle: true,
                    padding: 20
                }
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
                        const label = context.dataset.label || '';
                        const value = context.parsed.y || 0;

                        if (label.includes('Token')) {
                            return `${label}: ${value.toLocaleString()} tokens`;
                        } else if (label.includes('成本')) {
                            return `${label}: $${value.toFixed(4)} USD`;
                        }
                        return `${label}: ${value}`;
                    },
                    afterLabel: function(context) {
                        // 显示额外的端点信息
                        const endpointData = context.raw?.endpointData;
                        if (endpointData) {
                            return [
                                `请求总数: ${endpointData.total_requests}`,
                                `成功率: ${endpointData.success_rate ? endpointData.success_rate.toFixed(1) : '0'}%`,
                                `平均响应时间: ${endpointData.avg_response_time ? endpointData.avg_response_time.toFixed(0) : '0'}ms`
                            ];
                        }
                        return [];
                    }
                }
            }
        },
        interaction: {
            intersect: false,
            mode: 'index'
        },
        animation: {
            duration: 1000,
            easing: 'easeInOutQuart'
        }
    }
};

// 导出所有配置
export const chartConfigs = {
    requestTrend: requestTrendConfig,
    responseTime: responseTimeConfig,
    tokenUsage: tokenUsageConfig,
    endpointHealth: endpointHealthConfig,
    connectionActivity: connectionActivityConfig,
    endpointCosts: endpointCostsConfig
};

// 图表类型映射 (用于SSE事件处理)
export const chartTypeMapping = {
    'request_trends': 'requestTrend',
    'response_times': 'responseTime',
    'token_usage': 'tokenUsage',
    'endpoint_health': 'endpointHealth',
    'connection_activity': 'connectionActivity',
    'endpoint_costs': 'endpointCosts'
};

// 根据图表类型获取配置的主要函数
export const getChartConfig = (chartType) => {
    const config = chartConfigs[chartType];
    if (!config) {
        console.warn(`⚠️ 未找到图表配置: ${chartType}`);
        return null;
    }
    return config;
};

// 获取所有可用的图表类型
export const getAvailableChartTypes = () => {
    return Object.keys(chartConfigs);
};
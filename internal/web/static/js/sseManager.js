// Claude Request Forwarder - SSE管理模块
// 处理Server-Sent Events连接、重连逻辑和事件分发

window.SSEManager = class {
    constructor(webInterface) {
        this.webInterface = webInterface;
        this.connection = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 2000; // 2秒
        this.connectionStatus = 'disconnected';
        
        // 事件处理器映射
        this.eventHandlers = {
            'status': this.handleStatusEvent.bind(this),
            'endpoint': this.handleEndpointEvent.bind(this),
            'group': this.handleGroupEvent.bind(this),
            'connection': this.handleConnectionEvent.bind(this),
            'suspended': this.handleSuspendedEvent.bind(this),
            'request': this.handleRequestEvent.bind(this),
            'log': this.handleLogEvent.bind(this),
            'config': this.handleConfigEvent.bind(this),
            'chart': this.handleChartEvent.bind(this)
        };
    }
    
    // 初始化SSE连接
    init() {
        this.connect();
    }
    
    // 建立SSE连接
    connect() {
        if (this.connection) {
            this.connection.close();
        }

        this.updateConnectionStatus('connecting');
        
        const clientId = Utils.getOrCreateClientId();
        const events = 'status,endpoint,group,connection,log,chart'; // 订阅的事件类型
        
        try {
            this.connection = new EventSource(`/api/v1/stream?client_id=${clientId}&events=${events}`);
            
            this.connection.onopen = () => {
                console.log('📡 SSE连接已建立');
                this.updateConnectionStatus('connected');
                this.reconnectAttempts = 0;
                this.webInterface.stopAutoRefresh(); // 停止定时刷新
            };
            
            this.connection.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    this.handleMessage(data);
                } catch (error) {
                    console.error('解析SSE消息失败:', error, event.data);
                }
            };
            
            // 监听特定事件类型
            Object.keys(this.eventHandlers).forEach(eventType => {
                this.connection.addEventListener(eventType, (event) => {
                    try {
                        const data = JSON.parse(event.data);
                        this.handleMessage(data, eventType);
                    } catch (error) {
                        console.error(`解析${eventType}事件失败:`, error);
                    }
                });
            });
            
            this.connection.onerror = (event) => {
                console.error('❌ SSE连接错误:', event);
                this.updateConnectionStatus('error');
                this.handleError();
            };
            
        } catch (error) {
            console.error('创建SSE连接失败:', error);
            this.updateConnectionStatus('error');
            this.handleError();
        }
    }
    
    // 处理SSE消息
    handleMessage(data, eventType) {
        const type = eventType || data.type;
        const handler = this.eventHandlers[type];
        
        if (handler) {
            handler(data);
        } else {
            console.log('收到未处理的SSE消息:', data);
        }
    }
    
    // 处理SSE连接错误
    handleError() {
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++;
            const delay = this.reconnectDelay * this.reconnectAttempts;
            
            console.log(`🔄 SSE重连尝试 ${this.reconnectAttempts}/${this.maxReconnectAttempts}，${delay}ms后重试`);
            this.updateConnectionStatus('reconnecting');
            
            setTimeout(() => {
                this.connect();
            }, delay);
        } else {
            console.error('❌ SSE重连尝试已达上限，切换到定时刷新模式');
            this.updateConnectionStatus('failed');
            this.webInterface.startAutoRefresh(); // 回退到定时刷新
        }
    }
    
    // 更新连接状态
    updateConnectionStatus(status) {
        this.connectionStatus = status;
        Utils.updateConnectionStatus(status, this.reconnectAttempts, this.maxReconnectAttempts);
    }
    
    // === 事件处理器 ===
    
    // 处理状态事件
    handleStatusEvent(data) {
        if (this.webInterface.currentTab === 'overview') {
            if (data.status) Utils.updateElementText('server-status', 
                data.status === 'running' ? '🟢 运行中' : '🔴 已停止');
            if (data.uptime) Utils.updateElementText('uptime', data.uptime);
        }
    }
    
    // 处理端点事件
    handleEndpointEvent(data) {
        // 始终更新缓存数据
        if (data.endpoints) {
            this.webInterface.cachedData.endpoints = data;
            console.log('[SSE] 端点数据已更新到缓存:', data.endpoints.length, '个端点');
        }
        
        // 如果当前在endpoints标签页，立即更新UI
        if (this.webInterface.currentTab === 'endpoints' && data.endpoints) {
            Utils.updateElementHTML('endpoints-table', 
                this.webInterface.endpointsManager.generateEndpointsTable(data.endpoints));
            this.webInterface.endpointsManager.bindEndpointEvents();
            console.log('[UI] endpoints标签页UI已更新');
        }
        
        // 更新概览页面的端点数量
        if (data.total !== undefined) {
            if (this.webInterface.currentTab === 'overview') {
                Utils.updateElementText('endpoint-count', data.total);
                console.log('[UI] 概览页面端点数量已更新:', data.total);
            }
        }
    }
    
    // 处理组事件
    handleGroupEvent(data) {
        // 始终更新缓存数据
        if (data) {
            this.webInterface.cachedData.groups = data;
            console.log('[SSE] 组数据已更新到缓存');
            
            // 更新挂起请求提示
            this.webInterface.groupsManager.updateGroupSuspendedAlert(data);
            
            // 如果当前在概览标签页，更新活跃组信息
            if (this.webInterface.currentTab === 'overview') {
                const activeGroupElement = document.getElementById('active-group');
                if (activeGroupElement) {
                    const activeGroup = data.groups ? data.groups.find(group => group.is_active) : null;
                    if (activeGroup) {
                        activeGroupElement.textContent = `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)`;
                    } else {
                        activeGroupElement.textContent = '无活跃组';
                    }
                }
            }
        }
        
        // 如果当前在groups标签页，立即更新UI
        if (this.webInterface.currentTab === 'groups') {
            this.webInterface.groupsManager.displayGroups(data);
            console.log('[UI] groups标签页UI已更新');
        }
    }
    
    // 处理连接事件
    handleConnectionEvent(data) {
        // 始终更新缓存数据
        if (data) {
            this.webInterface.cachedData.connections = data;
            console.log('[SSE] 连接数据已更新到缓存');
        }
        
        // 如果当前在connections标签页，立即更新UI
        if (this.webInterface.currentTab === 'connections') {
            Utils.updateElementHTML('connections-stats', Utils.generateConnectionsStats(data));
            console.log('[UI] connections标签页UI已更新');
            
            // 更新挂起请求统计
            if (data.suspended) {
                this.updateSuspendedStats(data.suspended);
            }
            
            // 更新挂起连接列表
            if (data.suspended_connections) {
                this.updateSuspendedConnections(data.suspended_connections);
            }
        }
        
        // 如果在概览页面，更新挂起请求信息
        if (this.webInterface.currentTab === 'overview' && data.suspended) {
            const suspendedElement = document.getElementById('suspended-requests');
            const suspendedRateElement = document.getElementById('suspended-success-rate');
            
            if (suspendedElement) {
                suspendedElement.textContent = `${data.suspended.suspended_requests || 0} / ${data.suspended.total_suspended_requests || 0}`;
            }
            
            if (suspendedRateElement) {
                const rate = data.suspended.success_rate || 0;
                suspendedRateElement.textContent = `成功率: ${rate.toFixed(1)}%`;
                suspendedRateElement.className = rate > 80 ? 'text-muted' : 'text-warning';
            }
        }
        
        // 更新概览页面的请求数
        if (data.total_requests !== undefined) {
            if (this.webInterface.currentTab === 'overview') {
                Utils.updateElementText('total-requests', data.total_requests);
                console.log('[UI] 概览页面请求总数已更新:', data.total_requests);
            }
        }
    }
    
    // 处理挂起事件
    handleSuspendedEvent(data) {
        console.log('[SSE] 收到挂起请求事件数据:', data);
        
        // 如果在连接标签页，更新挂起请求统计
        if (this.webInterface.currentTab === 'connections') {
            if (data.current) {
                this.updateSuspendedStats(data.current);
            }
            if (data.suspended_connections) {
                this.updateSuspendedConnections(data.suspended_connections);
            }
        }
        
        // 在概览页面更新挂起请求统计
        if (this.webInterface.currentTab === 'overview' && data.current) {
            const suspendedElement = document.getElementById('suspended-requests');
            const suspendedRateElement = document.getElementById('suspended-success-rate');
            
            if (suspendedElement) {
                suspendedElement.textContent = `${data.current.suspended_requests || 0} / ${data.current.total_suspended_requests || 0}`;
            }
            
            if (suspendedRateElement) {
                const rate = data.current.success_rate || 0;
                suspendedRateElement.textContent = `成功率: ${rate.toFixed(1)}%`;
                suspendedRateElement.className = rate > 80 ? 'text-muted' : 'text-warning';
            }
        }
        
        // 显示挂起请求通知
        if (data.current && data.current.suspended_requests > 0) {
            Utils.showInfo(`当前有 ${data.current.suspended_requests} 个挂起请求`);
        }
    }
    
    // 处理请求事件
    handleRequestEvent(data) {
        // 始终更新缓存数据
        if (data) {
            this.webInterface.cachedData.requests = data;
            console.log('[SSE] 请求数据已更新到缓存');
        }
        
        // 如果当前在requests标签页，立即更新UI
        if (this.webInterface.currentTab === 'requests') {
            const tbody = document.getElementById('requests-table-body');
            if (tbody && data.requests) {
                tbody.innerHTML = this.webInterface.requestsManager.generateRequestsRows(data.requests);
                this.webInterface.requestsManager.updateRequestsCountInfo(data.total, this.webInterface.requestsState.currentPage);
                this.webInterface.requestsManager.bindRequestsEvents();
                console.log('[UI] requests标签页UI已更新');
            }
        }
    }
    
    // 处理日志事件 (已废弃)
    handleLogEvent(data) {
        console.log('日志功能已被请求追踪功能替代');
    }
    
    // 处理配置事件
    handleConfigEvent(data) {
        Utils.showInfo('配置已更新');
        if (this.webInterface.currentTab === 'config') {
            this.webInterface.loadConfig();
        }
    }
    
    // 处理图表事件
    handleChartEvent(data) {
        // 通知图表管理器处理SSE更新
        if (window.chartManager) {
            try {
                // 启用SSE更新模式
                if (!window.chartManager.sseEnabled) {
                    window.chartManager.enableSSEUpdates();
                }
                
                // 发送图表更新事件到图表管理器
                const chartUpdateEvent = new CustomEvent('chartUpdate', {
                    detail: {
                        chart_type: data.chart_type,
                        data: data.data
                    }
                });
                document.dispatchEvent(chartUpdateEvent);
                
                console.log(`📊 SSE图表数据更新: ${data.chart_type}`);
            } catch (error) {
                console.error('更新图表数据失败:', error);
                // 回退到直接更新模式
                this.updateChartLegacy(data);
            }
        } else {
            console.warn('图表管理器未初始化');
        }
    }
    
    // === 辅助方法 ===
    
    // 更新挂起请求统计
    updateSuspendedStats(suspendedData) {
        const elements = {
            'current-suspended': suspendedData.suspended_requests || 0,
            'total-suspended': suspendedData.total_suspended_requests || 0,
            'successful-suspended': suspendedData.successful_suspended_requests || 0,
            'timeout-suspended': suspendedData.timeout_suspended_requests || 0,
            'suspended-success-rate-detail': `${(suspendedData.success_rate || 0).toFixed(1)}%`,
            'avg-suspended-time': suspendedData.average_suspended_time || '0ms'
        };

        Object.entries(elements).forEach(([id, value]) => {
            Utils.updateElementText(id, value);
        });
    }

    // 更新挂起连接列表
    updateSuspendedConnections(connections) {
        const container = document.getElementById('suspended-connections-table');
        if (!container) return;

        if (connections.length === 0) {
            container.innerHTML = '<p>无挂起连接</p>';
            return;
        }

        let html = '<div class="suspended-connections-list">';
        connections.forEach(conn => {
            html += `
                <div class="suspended-connection-item">
                    <div class="connection-header">
                        <div class="connection-id">${conn.id}</div>
                        <div class="suspended-time">${conn.suspended_time}</div>
                    </div>
                    <div class="connection-details">
                        <div><strong>IP:</strong> ${conn.client_ip}</div>
                        <div><strong>端点:</strong> ${conn.endpoint}</div>
                        <div><strong>方法:</strong> ${conn.method}</div>
                        <div><strong>路径:</strong> ${conn.path}</div>
                        <div><strong>重试次数:</strong> ${conn.retry_count}</div>
                        <div><strong>挂起时间:</strong> ${conn.suspended_at}</div>
                    </div>
                </div>
            `;
        });
        html += '</div>';
        container.innerHTML = html;
    }
    
    // 兼容旧版图表更新（作为备用方案）
    updateChartLegacy(data) {
        if (this.webInterface.currentTab === 'charts' && window.chartManager) {
            try {
                const chartType = data.chart_type;
                const chartData = data.data;
                
                // 根据图表类型更新对应的图表
                const chartName = Utils.mapChartTypeToName(chartType);
                if (chartName && window.chartManager.charts.has(chartName)) {
                    const chart = window.chartManager.charts.get(chartName);
                    chart.data = chartData;
                    chart.update('none'); // 无动画更新，实时性更好
                }
            } catch (error) {
                console.error('兼容模式图表更新失败:', error);
            }
        }
    }
    
    // 关闭连接
    disconnect() {
        if (this.connection) {
            this.connection.close();
            this.connection = null;
            this.updateConnectionStatus('disconnected');
        }
    }
    
    // 获取连接状态
    isConnected() {
        return this.connectionStatus === 'connected';
    }
    
    // 清理资源
    destroy() {
        this.disconnect();
        this.reconnectAttempts = 0;
    }
};

console.log('✅ SSEManager模块已加载');
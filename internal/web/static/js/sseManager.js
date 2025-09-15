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
        
        // 前端实时时间计算 - 行业最佳实践
        this.serverStartTimestamp = null; // 服务器启动时间戳(Unix)
        this.uptimeTimer = null;          // 运行时间计时器
        this.isUptimeActive = false;      // 运行时间计算是否激活

        // 优先级事件统计
        this.stats = {
            eventsReceived: 0,
            eventsByPriority: {
                high: 0,
                normal: 0,
                low: 0
            },
            processingTime: 0
        };
        
        // 事件处理器映射（保留作为备用）
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
        
        // 扩展事件处理器映射，支持优先级处理
        this.priorityEventHandlers = {
            'high': this.handleHighPriorityEvent.bind(this),
            'normal': this.handleNormalPriorityEvent.bind(this), 
            'low': this.handleLowPriorityEvent.bind(this)
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
                    console.error('解析SSE消息失败:', error);
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
        const startTime = performance.now();

        const type = eventType || data.type || 'unknown';
        const priority = data.priority || 'normal';
        const context = data.context || {};

        // 关键调试信息：记录所有接收到的事件
        console.log(`🔍 [SSE] 接收事件: ${type}, 优先级: ${priority}, 数据:`, data);

        // 统计事件接收
        this.stats.eventsReceived++;
        this.stats.eventsByPriority[priority]++;

        // 使用传统事件处理器
        const handler = this.eventHandlers[type];
        if (handler) {
            // 提取实际的业务数据
            const businessData = data.data || data;
            handler(businessData);
        } else {
            console.log('收到未处理的SSE消息:', data);
        }
        
        // 记录处理时间
        const endTime = performance.now();
        this.stats.processingTime += (endTime - startTime);
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
        const oldStatus = this.connectionStatus;
        
        // 调用连接状态变化处理（影响运行时间计算）
        this.handleConnectionStatusChange(status);
        
        // 更新UI连接状态指示器
        Utils.updateConnectionStatus(status, this.reconnectAttempts, this.maxReconnectAttempts);
        
        if (oldStatus !== status) {
            console.log(`📡 SSE连接状态更新: ${oldStatus} → ${status}`);
        }
    }
    
    // === 优先级处理方法 ===
    
    // 高优先级事件处理（立即处理）
    handleHighPriorityEvent(eventType, data, context) {
        // 高优先级事件立即处理，通常是健康状态变化、错误告警
        console.log(`🔥 [高优先级事件] ${eventType}:`, data);
        
        // 显示高优先级通知
        if (context.show_notification !== false) {
            this.showHighPriorityNotification(eventType, data);
        }
        
        // 立即更新相关UI元素
        this.updateUIImmediately(eventType, data);
    }
    
    // 中等优先级事件处理（延迟处理）
    handleNormalPriorityEvent(eventType, data, context) {
        // 中等优先级事件延迟处理，通常是请求完成、常规更新
        console.log(`⚡ [中等优先级事件] ${eventType}:`, data);
        
        // 计划UI更新（1秒后）
        this.scheduleUIUpdate(eventType, data, 1000);
    }
    
    // 低优先级事件处理（批量处理）
    handleLowPriorityEvent(eventType, data, context) {
        // 低优先级事件批量处理，通常是统计数据、图表更新
        console.log(`📊 [低优先级事件] ${eventType}:`, data);
        
        // 批量UI更新
        this.batchUIUpdate(eventType, data);
    }
    
    // 显示高优先级通知
    showHighPriorityNotification(eventType, data) {
        switch (eventType) {
            case 'endpoint':
                if (data.change_type === 'health_changed') {
                    const status = data.healthy ? '🟢 恢复正常' : '🔴 状态异常';
                    Utils.showWarning(`端点 ${data.name} ${status}`);
                }
                break;
            case 'group':
                if (data.change_type === 'active_changed') {
                    Utils.showInfo(`组状态变化: ${data.name} ${data.is_active ? '已激活' : '已停用'}`);
                }
                break;
            case 'connection':
                if (data.change_type === 'error_response') {
                    Utils.showError(`请求失败: ${data.error_message}`);
                }
                break;
            case 'request':
                if (data.status === 'error') {
                    Utils.showError(`请求错误: ${data.request_id}`);
                }
                break;
        }
    }
    
    // 立即更新UI
    updateUIImmediately(eventType, data) {
        // 强制立即刷新缓存数据
        this.webInterface.cachedData[eventType] = data;
        
        // 立即更新当前标签页相关内容
        if (this.webInterface.currentTab === this.getRelevantTab(eventType)) {
            this.forceRefreshCurrentTab();
        }
        
        // 更新概览页面的关键指标
        this.updateOverviewIndicators(eventType, data);
    }
    
    // 计划UI更新
    scheduleUIUpdate(eventType, data, delay) {
        clearTimeout(this.scheduledUpdates?.[eventType]);
        
        if (!this.scheduledUpdates) {
            this.scheduledUpdates = {};
        }
        
        this.scheduledUpdates[eventType] = setTimeout(() => {
            this.webInterface.cachedData[eventType] = data;
            
            if (this.webInterface.currentTab === this.getRelevantTab(eventType)) {
                this.refreshCurrentTab();
            }
            
            delete this.scheduledUpdates[eventType];
        }, delay);
    }
    
    // 批量UI更新
    batchUIUpdate(eventType, data) {
        // 将更新添加到批量队列
        if (!this.batchQueue) {
            this.batchQueue = new Map();
        }
        
        this.batchQueue.set(eventType, data);
        
        // 如果没有批量处理定时器，创建一个
        if (!this.batchTimer) {
            this.batchTimer = setTimeout(() => {
                this.processBatchUIUpdates();
                this.batchTimer = null;
            }, 3000); // 3秒批量处理
        }
    }
    
    // 处理批量UI更新
    processBatchUIUpdates() {
        if (!this.batchQueue || this.batchQueue.size === 0) {
            return;
        }
        
        console.log(`🔄 处理批量UI更新: ${this.batchQueue.size} 个事件`);
        
        // 批量更新缓存数据
        this.batchQueue.forEach((data, eventType) => {
            this.webInterface.cachedData[eventType] = data;
        });
        
        // 如果当前标签页相关，刷新一次
        const currentTab = this.webInterface.currentTab;
        const relevantUpdates = Array.from(this.batchQueue.keys()).filter(eventType => 
            this.getRelevantTab(eventType) === currentTab
        );
        
        if (relevantUpdates.length > 0) {
            this.refreshCurrentTab();
        }
        
        // 清空批量队列
        this.batchQueue.clear();
    }
    
    // 获取事件类型对应的标签页
    getRelevantTab(eventType) {
        const tabMapping = {
            'endpoint': 'endpoints',
            'group': 'groups', 
            'connection': 'connections',
            'request': 'requests',
            'log': 'logs',
            'chart': 'charts',
            'status': 'overview'
        };
        return tabMapping[eventType] || 'overview';
    }
    
    // 强制刷新当前标签页
    forceRefreshCurrentTab() {
        const currentTab = this.webInterface.currentTab;
        console.log(`🔄 强制刷新标签页: ${currentTab}`);
        
        // 调用对应标签页的刷新方法
        switch (currentTab) {
            case 'endpoints':
                if (this.webInterface.endpointsManager) {
                    this.webInterface.endpointsManager.refreshEndpoints();
                }
                break;
            case 'groups':
                if (this.webInterface.groupsManager) {
                    this.webInterface.groupsManager.refreshGroups();
                }
                break;
            case 'requests':
                if (this.webInterface.requestsManager) {
                    this.webInterface.requestsManager.refreshRequests();
                }
                break;
            case 'overview':
                this.webInterface.loadOverview();
                break;
        }
    }
    
    // 普通刷新当前标签页
    refreshCurrentTab() {
        const currentTab = this.webInterface.currentTab;
        
        // 使用缓存数据更新，避免重复API调用
        switch (currentTab) {
            case 'endpoints':
                if (this.webInterface.cachedData.endpoints && this.webInterface.endpointsManager) {
                    this.webInterface.endpointsManager.displayEndpoints(this.webInterface.cachedData.endpoints);
                }
                break;
            case 'groups':
                if (this.webInterface.cachedData.groups && this.webInterface.groupsManager) {
                    this.webInterface.groupsManager.displayGroups(this.webInterface.cachedData.groups);
                }
                break;
            case 'requests':
                if (this.webInterface.cachedData.requests && this.webInterface.requestsManager) {
                    this.webInterface.requestsManager.displayRequests(this.webInterface.cachedData.requests);
                }
                break;
        }
    }
    
    // 更新概览页面关键指标
    updateOverviewIndicators(eventType, data) {
        if (eventType === 'endpoint' && data.total !== undefined) {
            Utils.updateElementText('endpoint-count', data.total);
        }
        if (eventType === 'connection' && data.total_requests !== undefined) {
            Utils.updateElementText('total-requests', data.total_requests);
        }
        if (eventType === 'group' && data.groups) {
            const activeGroup = data.groups.find(g => g.is_active);
            const activeGroupElement = document.getElementById('active-group');
            if (activeGroupElement) {
                activeGroupElement.textContent = activeGroup ? 
                    `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)` : 
                    '无活跃组';
            }
        }
    }
    
    // === 传统事件处理器 ===
    
    // 处理状态事件
    handleStatusEvent(data) {
        // 处理服务器启动时间戳（行业最佳实践）
        if (data.start_timestamp) {
            if (!this.serverStartTimestamp || this.serverStartTimestamp !== data.start_timestamp) {
                console.log('⏰ 收到服务器启动时间戳，开始前端实时计算');
                this.startUptimeCalculation(data.start_timestamp);
            }
        }
        
        // 总是更新服务状态（但不再处理uptime，由前端实时计算）
        if (data.status) {
            Utils.updateElementText('server-status', 
                data.status === 'running' ? '🟢 运行中' : '🔴 已停止');
        }
        
        // 如果当前在overview页面，可能还需要更新其他元素
        if (this.webInterface.currentTab === 'overview') {
            // 可以在这里添加其他overview特有的更新逻辑
        }
    }
    
    // 处理端点事件
    handleEndpointEvent(data) {
        // 处理完整端点列表更新
        if (data.endpoints) {
            this.webInterface.cachedData.endpoints = data;
            
            // 如果当前在endpoints标签页，立即更新UI
            if (this.webInterface.currentTab === 'endpoints') {
                Utils.updateElementHTML('endpoints-table', 
                    this.webInterface.endpointsManager.generateEndpointsTable(data.endpoints));
                this.webInterface.endpointsManager.bindEndpointEvents();
            }
            
            // 更新概览页面的端点数量
            if (data.total !== undefined) {
                if (this.webInterface.currentTab === 'overview') {
                    Utils.updateElementText('endpoint-count', data.total);
                }
            }
            return;
        }
        
        // 处理单个端点状态更新（健康检查后的实时更新）
        if (data.endpoint && (data.change_type === 'status_changed' || data.change_type === 'health_changed')) {
            // 更新缓存中的特定端点数据
            if (this.webInterface.cachedData.endpoints && this.webInterface.cachedData.endpoints.endpoints) {
                const endpoints = this.webInterface.cachedData.endpoints.endpoints;
                const endpointIndex = endpoints.findIndex(ep => ep.name === data.endpoint);
                
                if (endpointIndex !== -1) {
                    // 更新缓存中的端点数据
                    endpoints[endpointIndex].healthy = data.healthy;
                    endpoints[endpointIndex].response_time = data.response_time;
                    endpoints[endpointIndex].last_check = data.last_check;
                    endpoints[endpointIndex].never_checked = data.never_checked || false;
                    
                    // 如果当前在endpoints标签页，实时更新特定行
                    if (this.webInterface.currentTab === 'endpoints') {
                        this.updateEndpointTableRow(data.endpoint, endpoints[endpointIndex]);
                    }
                    
                    // 如果在概览页面，更新相关统计
                    if (this.webInterface.currentTab === 'overview') {
                        this.updateOverviewEndpointStats();
                    }
                }
            }
        }
    }
    
    // 处理组事件
    handleGroupEvent(data) {
        // 🔥 关键调试：记录传统组事件处理器的调用
        console.log(`🔍 [传统处理器] 处理组事件:`, {
            currentTab: this.webInterface.currentTab,
            hasGroups: !!(data && data.groups),
            changeType: data.change_type,
            groupName: data.group,
            data: JSON.parse(JSON.stringify(data))
        });

        // 始终更新缓存数据
        if (data) {
            // 🔥 关键修复：区分完整组数据更新和单个组健康统计更新
            if (data.groups) {
                // 完整组数据更新
                this.webInterface.cachedData.groups = data;
                console.log(`✅ [传统处理器] 完整组数据已更新到缓存`);

                // 更新挂起请求提示
                this.webInterface.groupsManager.updateGroupSuspendedAlert(data);

                // 如果当前在groups标签页，立即更新UI
                if (this.webInterface.currentTab === 'groups') {
                    console.log(`🔄 [传统处理器] 当前在组页面，正在更新完整组UI...`);
                    this.webInterface.groupsManager.displayGroups(data);
                    console.log(`✅ [传统处理器] 组页面完整UI已更新`);
                }
            } else if (data.change_type === 'health_stats_changed' && data.group) {
                // 🔥 新增：单个组健康统计更新处理
                console.log(`🎯 [传统处理器] 处理单个组健康统计更新: ${data.group}`);

                // 更新缓存中的特定组数据
                if (this.webInterface.cachedData.groups && this.webInterface.cachedData.groups.groups) {
                    const groups = this.webInterface.cachedData.groups.groups;
                    const groupIndex = groups.findIndex(g => g.name === data.group);
                    if (groupIndex !== -1) {
                        groups[groupIndex].healthy_endpoints = data.healthy_endpoints;
                        groups[groupIndex].unhealthy_endpoints = data.unhealthy_endpoints;
                        groups[groupIndex].total_endpoints = data.total_endpoints;

                        // 🔥 同时更新计算的健康状态标记，用于tab切换时正确显示
                        if (data.healthy_endpoints === 0) {
                            groups[groupIndex]._computed_health_status = '无健康端点';
                        } else if (data.healthy_endpoints < data.total_endpoints) {
                            groups[groupIndex]._computed_health_status = '部分健康';
                        } else {
                            groups[groupIndex]._computed_health_status = null; // 清除计算状态，使用原始状态
                        }

                        console.log(`✅ [传统处理器] 缓存中的组 ${data.group} 数据已更新`);
                    }
                }

                // 如果当前在组页面，更新特定组卡片
                if (this.webInterface.currentTab === 'groups') {
                    console.log(`🔄 [传统处理器] 当前在组页面，正在更新组卡片...`);
                    this.updateSingleGroupCard(data);
                }
            }

            // 如果当前在概览标签页，更新活跃组信息
            if (this.webInterface.currentTab === 'overview') {
                if (data.groups) {
                    const activeGroup = data.groups.find(group => group.is_active);
                    const activeGroupElement = document.getElementById('active-group');
                    if (activeGroupElement) {
                        if (activeGroup) {
                            activeGroupElement.textContent = `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)`;
                        } else {
                            activeGroupElement.textContent = '无活跃组';
                        }
                        console.log(`✅ [传统处理器] 概览页活跃组信息已更新`);
                    }
                } else if (data.is_active && data.group) {
                    // 单个活跃组的统计更新
                    const activeGroupElement = document.getElementById('active-group');
                    if (activeGroupElement) {
                        activeGroupElement.textContent = `${data.group} (${data.healthy_endpoints}/${data.total_endpoints} 健康)`;
                        console.log(`✅ [传统处理器] 概览页活跃组统计已更新`);
                    }
                }
            }
        } else {
            console.log(`⚠️ [传统处理器] 收到空的组事件数据`);
        }
    }

    // 🔥 新增：传统处理器的单个组卡片更新方法
    updateSingleGroupCard(data) {
        const groupName = data.group;

        // 查找并更新组卡片
        const groupCard = document.querySelector(`[data-group-name="${groupName}"]`);
        if (groupCard) {
            // 更新健康端点数量
            const healthyElement = groupCard.querySelector('.group-endpoints-count');
            if (healthyElement) {
                const oldValue = healthyElement.textContent;
                healthyElement.textContent = data.healthy_endpoints;
                // 添加动画效果提示更新
                if (oldValue !== String(data.healthy_endpoints)) {
                    healthyElement.style.backgroundColor = '#e8f5e8';
                    healthyElement.style.transition = 'background-color 0.5s ease';
                    setTimeout(() => {
                        healthyElement.style.backgroundColor = '';
                    }, 500);
                }
            }

            // 更新不健康端点数量
            const unhealthyElement = groupCard.querySelector('.group-unhealthy-count');
            if (unhealthyElement) {
                const oldValue = unhealthyElement.textContent;
                unhealthyElement.textContent = data.unhealthy_endpoints;
                // 添加动画效果提示更新
                if (oldValue !== String(data.unhealthy_endpoints)) {
                    unhealthyElement.style.backgroundColor = '#ffe8e8';
                    unhealthyElement.style.transition = 'background-color 0.5s ease';
                    setTimeout(() => {
                        unhealthyElement.style.backgroundColor = '';
                    }, 500);
                }
            }

            // 🔥 更新状态文本（但不修改CSS类）
            const groupStatusElement = groupCard.querySelector('.group-status');
            if (groupStatusElement) {
                let statusText;

                // 根据健康端点数量决定状态文本
                if (data.healthy_endpoints === 0) {
                    statusText = '无健康端点';
                } else if (data.healthy_endpoints < data.total_endpoints) {
                    statusText = '部分健康';
                } else {
                    statusText = data.status || '正常';
                }

                // 更新状态文本（不修改CSS类）
                if (groupStatusElement.textContent !== statusText) {
                    groupStatusElement.textContent = statusText;
                    console.log(`📊 [传统处理器] 组 ${groupName} 状态文本更新为: ${statusText}`);
                }
            }

            console.log(`✅ [传统处理器] 组卡片 ${groupName} 已更新 (${data.healthy_endpoints}/${data.total_endpoints} 健康)`);
        } else {
            console.log(`⚠️ [传统处理器] 未找到组卡片: ${groupName}, 可能需要重新加载页面`);
            // 如果找不到卡片，重新加载组页面
            if (this.webInterface.groupsManager) {
                this.webInterface.groupsManager.loadGroups();
            }
        }
    }
    
    // 处理连接事件
    handleConnectionEvent(data) {
        // 始终更新缓存数据
        if (data) {
            this.webInterface.cachedData.connections = data;
            console.log('[SSE] 连接数据已更新到缓存');
        }
        
        // 如果在概览页面，更新连接详情和挂起监控区域
        if (this.webInterface.currentTab === 'overview') {
            // 更新连接详情区域
            this.webInterface.updateConnectionDetails(data);
            
            // 更新挂起请求监控区域
            if (data.suspended || data.suspended_connections) {
                this.webInterface.updateSuspendedMonitoring(
                    data.suspended || {}, 
                    data.suspended_connections || []
                );
            }
            
            // 更新概览页面的挂起请求信息
            if (data.suspended) {
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
                
                // 智能展开逻辑：如果有挂起请求，自动展开监控区域
                if (data.suspended.suspended_requests > 0) {
                    this.webInterface.expandSection('suspended-monitoring');
                }
            }
            
            console.log('[UI] 概览页面连接数据已更新');
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
        
        // 如果在概览页面，更新挂起请求监控区域
        if (this.webInterface.currentTab === 'overview') {
            if (data.current) {
                this.webInterface.updateSuspendedMonitoring(
                    data.current,
                    data.suspended_connections || []
                );
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
            
            // 智能展开逻辑：如果有挂起请求，自动展开监控区域
            if (data.current.suspended_requests > 0) {
                this.webInterface.expandSection('suspended-monitoring');
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
    handleLogEvent(_data) {
        console.log('日志功能已被请求追踪功能替代');
    }
    
    // 处理配置事件
    handleConfigEvent(_data) {
        Utils.showInfo('配置已更新');
        if (this.webInterface.currentTab === 'config') {
            this.webInterface.loadConfig();
        }
    }
    
    // 处理图表事件
    handleChartEvent(data) {
        // 检查是否是新的批量更新事件
        if (data.chart_type === 'batch_update' && data.charts) {
            // 遍历批量更新中的所有图表
            for (const chartType in data.charts) {
                if (Object.hasOwnProperty.call(data.charts, chartType)) {
                    const chartData = data.charts[chartType];
                    
                    // 为每个图表分发一个独立的自定义事件
                    // 这使得各个图表组件无需修改自身逻辑
                    const chartUpdateEvent = new CustomEvent('chartUpdate', {
                        detail: { 
                            chart_type: chartType, // 使用原始的 chart_type
                            data: chartData       // 使用该图表对应的数据
                        }
                    });
                    document.dispatchEvent(chartUpdateEvent);
                }
            }
            console.log(`📊 批量SSE图表数据更新: ${Object.keys(data.charts).length}个图表`);
        } else if (data.chart_type) {
            // 向后兼容，处理旧的单个图表事件（可选，但建议保留）
            const chartUpdateEvent = new CustomEvent('chartUpdate', {
                detail: { 
                    chart_type: data.chart_type, 
                    data: data.data 
                }
            });
            document.dispatchEvent(chartUpdateEvent);
            console.log(`📊 SSE图表数据更新: ${data.chart_type}`);
        } else {
            console.warn('收到未知格式的图表事件:', data);
        }
        
        // 通知图表管理器处理SSE更新
        if (window.chartManager) {
            try {
                // 启用SSE更新模式
                if (!window.chartManager.sseEnabled) {
                    window.chartManager.enableSSEUpdates();
                }
            } catch (error) {
                console.error('启用图表SSE更新失败:', error);
            }
        } else {
            console.warn('图表管理器未初始化');
        }
    }
    
    // === 前端运行时间计算方法（行业最佳实践） ===
    
    // 启动前端实时运行时间计算
    startUptimeCalculation(serverStartTimestamp) {
        if (!serverStartTimestamp) {
            console.warn('⚠️ 无效的服务器启动时间戳，无法计算运行时间');
            return;
        }
        
        this.serverStartTimestamp = serverStartTimestamp;
        this.isUptimeActive = true;
        
        console.log('⏰ 启动前端实时运行时间计算, 服务器启动时间:', new Date(serverStartTimestamp * 1000).toLocaleString());
        
        // 立即计算一次
        this.calculateAndDisplayUptime();
        
        // 每秒更新一次运行时间 - 行业标准做法
        this.uptimeTimer = setInterval(() => {
            if (this.isUptimeActive) {
                this.calculateAndDisplayUptime();
            }
        }, 1000);
    }
    
    // 停止前端运行时间计算（服务器关闭或网络断开）
    stopUptimeCalculation() {
        if (this.uptimeTimer) {
            clearInterval(this.uptimeTimer);
            this.uptimeTimer = null;
        }
        this.isUptimeActive = false;
        
        // 显示离线状态
        Utils.updateElementText('uptime', '⏸️ 离线');
        Utils.updateElementText('server-status', '🔴 已断开');
        console.log('⏸️ 前端运行时间计算已停止（服务器离线）');
    }
    
    // 计算并显示当前运行时间
    calculateAndDisplayUptime() {
        if (!this.serverStartTimestamp) {
            return;
        }
        
        const nowTimestamp = Math.floor(Date.now() / 1000); // 当前Unix时间戳
        const uptimeSeconds = nowTimestamp - this.serverStartTimestamp;
        
        if (uptimeSeconds < 0) {
            console.warn('⚠️ 运行时间计算异常：当前时间早于服务器启动时间');
            return;
        }
        
        const formattedUptime = this.formatUptime(uptimeSeconds);
        
        // 更新UI显示
        Utils.updateElementText('uptime', formattedUptime);
        
        // 确保服务状态显示为运行中（仅当连接正常时）
        if (this.connectionStatus === 'connected') {
            Utils.updateElementText('server-status', '🟢 运行中');
        }
    }
    
    // 格式化运行时间（秒转为友好格式）
    formatUptime(totalSeconds) {
        const days = Math.floor(totalSeconds / 86400);
        const hours = Math.floor((totalSeconds % 86400) / 3600);
        const minutes = Math.floor((totalSeconds % 3600) / 60);
        const seconds = totalSeconds % 60;
        
        let formatted = '';
        if (days > 0) {
            formatted += `${days}天 `;
        }
        if (hours > 0 || days > 0) {
            formatted += `${hours}小时 `;
        }
        if (minutes > 0 || hours > 0 || days > 0) {
            formatted += `${minutes}分钟 `;
        }
        formatted += `${seconds}秒`;
        
        return formatted;
    }
    
    // 处理连接状态变化（影响运行时间计算）
    handleConnectionStatusChange(newStatus) {
        const oldStatus = this.connectionStatus;
        this.connectionStatus = newStatus;
        
        switch (newStatus) {
            case 'connected':
                // 连接恢复时恢复运行时间计算（如果有启动时间戳）
                if (this.serverStartTimestamp && !this.isUptimeActive) {
                    this.startUptimeCalculation(this.serverStartTimestamp);
                    console.log('🔄 连接恢复，重启运行时间计算');
                }
                break;
                
            case 'disconnected':
            case 'error':
            case 'failed':
                // 连接断开时暂停运行时间计算
                if (this.isUptimeActive) {
                    this.stopUptimeCalculation();
                }
                break;
                
            case 'connecting':
            case 'reconnecting':
                // 连接中状态显示但不影响时间计算
                Utils.updateElementText('server-status', '🔄 连接中...');
                break;
        }
        
        if (oldStatus !== newStatus) {
            console.log(`🔄 连接状态变化: ${oldStatus} → ${newStatus}`);
        }
    }
    
    // === 结束前端运行时间计算方法 ===
    
    // 更新端点表格中的特定行（实时更新单个端点）
    updateEndpointTableRow(endpointName, endpointData) {
        const table = document.getElementById('endpoints-table');
        if (!table) return;
        
        // 查找对应的表格行
        const rows = table.querySelectorAll('tbody tr');
        for (let row of rows) {
            const nameCell = row.cells[1]; // 名称在第二列
            if (nameCell && nameCell.textContent.trim() === endpointName) {
                // 更新状态列
                const statusCell = row.cells[0];
                if (statusCell) {
                    let statusClass, statusText;
                    if (endpointData.never_checked) {
                        statusClass = 'status-never-checked';
                        statusText = '未检测';
                    } else if (endpointData.healthy) {
                        statusClass = 'status-healthy';
                        statusText = '健康';
                    } else {
                        statusClass = 'status-unhealthy';
                        statusText = '不健康';
                    }
                    statusCell.innerHTML = `<span class="status-indicator ${statusClass}"></span>${statusText}`;
                }
                
                // 更新响应时间列
                const responseTimeCell = row.cells[5];
                if (responseTimeCell) {
                    responseTimeCell.textContent = endpointData.response_time;
                }
                
                // 更新最后检查时间列（这是用户关心的核心问题）
                const lastCheckCell = row.cells[6];
                if (lastCheckCell) {
                    lastCheckCell.textContent = endpointData.last_check;
                    // 高亮显示刚更新的时间
                    lastCheckCell.style.backgroundColor = '#e8f5e8';
                    lastCheckCell.style.transition = 'background-color 2s ease';
                    setTimeout(() => {
                        lastCheckCell.style.backgroundColor = '';
                    }, 2000);
                }
                
                break;
            }
        }
    }
    
    // 更新概览页面的端点统计
    updateOverviewEndpointStats() {
        if (this.webInterface.cachedData.endpoints && this.webInterface.cachedData.endpoints.endpoints) {
            const endpoints = this.webInterface.cachedData.endpoints.endpoints;
            const healthy = endpoints.filter(ep => ep.healthy && !ep.never_checked).length;
            const total = endpoints.length;
            
            Utils.updateElementText('endpoint-count', total);
            
            // 如果有健康状态指示器，也更新它
            const healthRatio = total > 0 ? (healthy / total * 100).toFixed(1) : 0;
            const healthElement = document.getElementById('endpoint-health-ratio');
            if (healthElement) {
                healthElement.textContent = `${healthy}/${total} (${healthRatio}%)`;
            }
        }
    }
    
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
        
        // 清理前端运行时间计算
        this.stopUptimeCalculation();
        this.serverStartTimestamp = null;
        
        // 清理所有定时器
        if (this.scheduledUpdates) {
            Object.values(this.scheduledUpdates).forEach(timerId => clearTimeout(timerId));
            this.scheduledUpdates = {};
        }
        
        if (this.batchTimer) {
            clearTimeout(this.batchTimer);
            this.batchTimer = null;
        }
        
        // 清理批量队列
        if (this.batchQueue) {
            this.batchQueue.clear();
        }
        
        // 重置统计
        this.stats = {
            eventsReceived: 0,
            eventsByPriority: { high: 0, normal: 0, low: 0 },
            processingTime: 0
        };

        console.log('🧹 SSE管理器资源清理完成（包含运行时间计算）');
    }
    
    
    // 获取性能报告
    getPerformanceReport() {
        const avgProcessingTime = this.stats.eventsReceived > 0 ? 
            (this.stats.processingTime / this.stats.eventsReceived).toFixed(2) : 0;
        
        let report = `SSE管理器状态报告:\n`;
        report += `- 连接状态: ${this.connectionStatus}\n`;
        report += `- 重连次数: ${this.reconnectAttempts}/${this.maxReconnectAttempts}\n`;
        report += `- 已接收事件: ${this.stats.eventsReceived}\n`;
        report += `- 高优先级: ${this.stats.eventsByPriority.high} (${((this.stats.eventsByPriority.high/this.stats.eventsReceived)*100).toFixed(1)}%)\n`;
        report += `- 中优先级: ${this.stats.eventsByPriority.normal} (${((this.stats.eventsByPriority.normal/this.stats.eventsReceived)*100).toFixed(1)}%)\n`;
        report += `- 低优先级: ${this.stats.eventsByPriority.low} (${((this.stats.eventsByPriority.low/this.stats.eventsReceived)*100).toFixed(1)}%)\n`;
        report += `- 平均处理时间: ${avgProcessingTime}ms\n`;
        
        
        return report;
    }
    
};

console.log('✅ SSEManager模块已加载');
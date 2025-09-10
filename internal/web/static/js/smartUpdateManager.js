/**
 * 智能更新管理器 - 基于优先级和上下文的前端更新系统
 * 
 * 功能特性：
 * - 基于事件优先级的智能处理
 * - 批量更新和防抖动机制
 * - 跨标签页的智能UI更新
 * - 性能监控和统计
 * - 内存泄漏防护
 */

class SmartUpdateManager {
    constructor(webInterface) {
        this.webInterface = webInterface;
        
        // 更新队列和优先级处理
        this.updateQueue = new PriorityQueue();
        this.pendingUpdates = new Map();
        this.batchTimer = null;
        this.lastUpdate = new Map();
        
        // 防抖动配置 - 优化为监控系统的实时响应
        this.debounceConfig = {
            high: 0,        // 高优先级立即执行 (健康状态变化、错误响应)
            normal: 100,    // 中优先级100ms延迟 (性能变化、请求完成) - 从1000ms优化
            low: 500        // 低优先级500ms批量处理 (统计信息) - 从3000ms优化
        };
        
        // 性能监控统计
        this.stats = {
            totalUpdates: 0,
            immediateUpdates: 0,
            batchedUpdates: 0,
            droppedUpdates: 0,
            startTime: Date.now()
        };
        
        // UI更新缓存：避免重复DOM操作
        this.uiUpdateCache = new Map();
        this.lastUIUpdate = new Map();
        
        console.log('🧠 智能更新管理器初始化完成');
    }
    
    /**
     * 处理SSE事件的智能更新
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     * @param {Object} context 事件上下文（包含优先级信息）
     */
    handleSSEEvent(eventType, data, context) {
        const priority = this.determinePriority(eventType, data, context);
        const updateKey = `${eventType}-${priority}`;
        
        // 检查是否需要去重
        if (this.shouldSkipUpdate(updateKey, data)) {
            this.stats.droppedUpdates++;
            return;
        }
        
        // 根据优先级处理更新
        switch (priority) {
            case 'high':
                this.updateImmediately(eventType, data);
                this.stats.immediateUpdates++;
                break;
            case 'normal':
                this.scheduleUpdate(eventType, data, this.debounceConfig.normal);
                break;
            case 'low':
                this.batchUpdate(eventType, data);
                this.stats.batchedUpdates++;
                break;
        }
        
        this.stats.totalUpdates++;
        this.recordLastUpdate(updateKey, data);
    }
    
    /**
     * 智能优先级判断
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     * @param {Object} context 事件上下文
     * @returns {string} 优先级 (high/normal/low)
     */
    determinePriority(eventType, data, context) {
        // 基于后端事件上下文判断
        if (context && context.priority !== undefined) {
            switch (context.priority) {
                case 0: return 'high';    // HighPriority - 立即处理
                case 1: return 'normal';  // NormalPriority - 延迟处理
                case 2: return 'low';     // LowPriority - 批量处理
            }
        }
        
        // 基于事件类型和数据内容的智能判断 - 优化为监控系统
        if (eventType === 'endpoint') {
            if (data.change_type === 'health_changed' || data.change_type === 'status_changed') {
                return 'high'; // 端点健康状态变化需要立即更新
            }
            if (data.change_type === 'performance_changed') {
                return 'high'; // 性能变化对监控系统也很重要 - 从normal提升为high
            }
            return 'normal'; // 其他端点信息 - 从low提升为normal
        }
        
        if (eventType === 'connection') {
            if (data.change_type === 'error_response' || data.change_type === 'suspended_change') {
                return 'high'; // 错误响应和挂起变化需要立即处理
            }
            if (data.change_type === 'request_completed') {
                return 'high'; // 请求完成对监控系统重要 - 从normal提升为high
            }
            return 'normal'; // 统计信息 - 从low提升为normal
        }
        
        if (eventType === 'group') {
            if (data.change_type === 'active_changed' || data.change_type === 'state_changed') {
                return 'high'; // 组状态变化立即更新
            }
            return 'normal';
        }
        
        if (eventType === 'request') {
            if (data.change_type === 'status_changed') {
                return 'high'; // 所有状态变化都立即显示 - 从部分normal提升为high
            }
            return 'normal'; // 新请求 - 从low提升为normal
        }
        
        // 新增：重要监控事件的优先级判断
        if (eventType === 'status') {
            return 'high'; // 服务状态更新立即显示
        }
        
        if (eventType === 'chart') {
            return 'high'; // 图表数据更新立即显示，保持实时性
        }
        
        if (eventType === 'suspended') {
            return 'high'; // 挂起请求事件立即显示
        }
        
        if (eventType === 'config') {
            return 'normal'; // 配置更新较少，normal优先级即可
        }
        
        if (eventType === 'log') {
            return 'normal'; // 日志事件normal优先级，避免过于频繁
        }
        
        return 'normal'; // 默认中等优先级
    }
    
    /**
     * 检查是否应该跳过更新（去重逻辑）
     * @param {string} updateKey 更新键
     * @param {Object} data 事件数据
     * @returns {boolean} 是否应该跳过
     */
    shouldSkipUpdate(updateKey, data) {
        const lastData = this.lastUpdate.get(updateKey);
        if (!lastData) return false;
        
        // 检查时间间隔（防止过于频繁的更新）
        const now = Date.now();
        const lastTime = this.lastUIUpdate.get(updateKey) || 0;
        if (now - lastTime < 100) { // 100ms内的重复更新直接跳过
            return true;
        }
        
        // 检查数据是否真的有变化
        try {
            const dataStr = JSON.stringify(data);
            const lastDataStr = JSON.stringify(lastData);
            return dataStr === lastDataStr;
        } catch (e) {
            return false; // 序列化失败时不跳过
        }
    }
    
    /**
     * 立即更新（高优先级）
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     */
    updateImmediately(eventType, data) {
        // 更新缓存数据
        this.webInterface.cachedData[eventType] = data;
        
        // 立即更新UI
        this.updateRelevantUI(eventType, data);
        
        // 清除任何待处理的相同类型更新
        this.clearPendingUpdate(eventType);
        
        console.log(`⚡ 高优先级立即更新: ${eventType}`);
    }
    
    /**
     * 延迟更新（中优先级）
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     * @param {number} delay 延迟时间
     */
    scheduleUpdate(eventType, data, delay) {
        // 更新缓存数据
        this.webInterface.cachedData[eventType] = data;
        
        // 清除之前的定时器
        this.clearPendingUpdate(eventType);
        
        // 设置新的定时器
        const timerId = setTimeout(() => {
            this.updateRelevantUI(eventType, data);
            this.pendingUpdates.delete(eventType);
        }, delay);
        
        this.pendingUpdates.set(eventType, timerId);
        
        console.log(`⏰ 延迟更新调度: ${eventType}, 延迟: ${delay}ms`);
    }
    
    /**
     * 批量更新（低优先级）
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     */
    batchUpdate(eventType, data) {
        // 更新缓存数据
        this.webInterface.cachedData[eventType] = data;
        
        // 添加到批量更新队列
        this.updateQueue.enqueue({
            eventType, 
            data, 
            timestamp: Date.now()
        });
        
        // 如果没有批量处理定时器，创建一个
        if (!this.batchTimer) {
            this.batchTimer = setTimeout(() => {
                this.processBatchUpdates();
                this.batchTimer = null;
            }, this.debounceConfig.low);
        }
        
        console.log(`📦 批量更新入队: ${eventType}`);
    }
    
    /**
     * 处理批量更新
     */
    processBatchUpdates() {
        const updates = [];
        while (!this.updateQueue.isEmpty()) {
            updates.push(this.updateQueue.dequeue());
        }
        
        if (updates.length === 0) return;
        
        // 按事件类型分组，去重（保留最新的）
        const groupedUpdates = new Map();
        updates.forEach(update => {
            groupedUpdates.set(update.eventType, update);
        });
        
        console.log(`🔄 处理批量更新: ${groupedUpdates.size} 个事件类型`);
        
        // 批量处理UI更新
        groupedUpdates.forEach(({eventType, data}) => {
            this.updateRelevantUI(eventType, data);
        });
    }
    
    /**
     * 智能UI更新：不局限于当前标签页
     * @param {string} eventType 事件类型
     * @param {Object} data 事件数据
     */
    updateRelevantUI(eventType, data) {
        const updateKey = `ui-${eventType}`;
        this.lastUIUpdate.set(updateKey, Date.now());
        
        switch (eventType) {
            case 'endpoint':
                this.updateEndpointInfo(data);
                break;
            case 'connection':
                this.updateConnectionInfo(data);
                break;
            case 'group':
                this.updateGroupInfo(data);
                break;
            case 'request':
                this.updateRequestInfo(data);
                break;
            case 'log':
                this.updateLogInfo(data);
                break;
            case 'status':
                this.updateSystemStatus(data);
                break;
            default:
                console.warn(`未知的事件类型: ${eventType}`);
        }
    }
    
    /**
     * 更新端点信息
     * @param {Object} data 端点数据
     */
    updateEndpointInfo(data) {
        // 总是更新概览页指示器
        if (data.endpoints) {
            const healthyCount = data.endpoints.filter(ep => ep.healthy).length;
            const totalCount = data.endpoints.length;
            
            Utils.updateElementText('endpoint-count', totalCount);
            this.updateHealthStatusIcon(healthyCount, totalCount);
        }
        
        // 当前在端点标签页才更新详细内容
        if (this.webInterface.currentTab === 'endpoints' && data.endpoints) {
            const container = document.getElementById('endpoints-table');
            if (container) {
                container.innerHTML = this.webInterface.endpointsManager.generateEndpointsTable(data.endpoints);
                this.webInterface.endpointsManager.bindEndpointEvents();
            }
        }
    }
    
    /**
     * 更新连接信息
     * @param {Object} data 连接数据
     */
    updateConnectionInfo(data) {
        // 更新概览页统计
        if (data.total_requests !== undefined) {
            Utils.updateElementText('total-requests', data.total_requests);
        }
        if (data.active_connections !== undefined) {
            Utils.updateElementText('active-connections', data.active_connections);
        }
        if (data.success_rate !== undefined) {
            Utils.updateElementText('success-rate', `${data.success_rate.toFixed(1)}%`);
        }
        
        // 更新挂起请求监控
        if (data.suspended !== undefined) {
            this.webInterface.updateSuspendedMonitoring(data.suspended, data.suspended_connections || []);
        }
        
        // 更新连接页面详细信息
        if (this.webInterface.currentTab === 'connections' && data.connections) {
            const tbody = document.getElementById('connections-table-body');
            if (tbody) {
                tbody.innerHTML = this.generateConnectionsRows(data.connections);
            }
        }
    }
    
    /**
     * 更新组信息
     * @param {Object} data 组数据
     */
    updateGroupInfo(data) {
        // 总是更新概览页活跃组信息
        const activeGroupElement = document.getElementById('active-group');
        if (activeGroupElement && data.groups) {
            const activeGroup = data.groups.find(group => group.is_active);
            if (activeGroup) {
                activeGroupElement.textContent = `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)`;
                activeGroupElement.className = 'status-active';
            } else {
                activeGroupElement.textContent = '无活跃组';
                activeGroupElement.className = 'status-inactive';
            }
        }
        
        // 当前在组页面才更新详细内容
        if (this.webInterface.currentTab === 'groups' && this.webInterface.groupsManager) {
            this.webInterface.groupsManager.displayGroups(data);
        }
    }
    
    /**
     * 更新请求信息
     * @param {Object} data 请求数据
     */
    updateRequestInfo(data) {
        // 只有在请求页面且影响当前页才更新
        if (this.webInterface.currentTab === 'requests' && this.isCurrentPageAffected(data)) {
            const tbody = document.getElementById('requests-table-body');
            if (tbody && data.requests) {
                tbody.innerHTML = this.webInterface.requestsManager.generateRequestsRows(data.requests);
                this.webInterface.requestsManager.bindRequestsEvents();
            }
        }
        
        // 显示新请求提醒（除非在请求页面）
        if (this.webInterface.currentTab !== 'requests') {
            this.showNewRequestNotification(data);
        }
    }
    
    /**
     * 更新日志信息
     * @param {Object} data 日志数据
     */
    updateLogInfo(data) {
        // 只有在日志页面才更新
        if (this.webInterface.currentTab === 'logs' && data.logs) {
            const container = document.getElementById('logs-container');
            if (container) {
                const logEntries = data.logs.map(log => `
                    <div class="log-entry log-${log.level}">
                        <span class="log-time">${log.timestamp}</span>
                        <span class="log-level">[${log.level}]</span>
                        <span class="log-message">${log.message}</span>
                    </div>
                `).join('');
                container.innerHTML = logEntries;
                
                // 自动滚动到底部
                container.scrollTop = container.scrollHeight;
            }
        }
    }
    
    /**
     * 更新系统状态
     * @param {Object} data 系统状态数据
     */
    updateSystemStatus(data) {
        if (data.version) {
            Utils.updateElementText('system-version', data.version);
        }
        if (data.uptime) {
            Utils.updateElementText('system-uptime', Utils.formatDuration(data.uptime));
        }
        if (data.memory_usage !== undefined) {
            Utils.updateElementText('memory-usage', Utils.formatBytes(data.memory_usage));
        }
    }
    
    /**
     * 生成连接行HTML
     * @param {Array} connections 连接数组
     * @returns {string} HTML字符串
     */
    generateConnectionsRows(connections) {
        return connections.map(conn => `
            <tr class="connection-row">
                <td>${conn.id}</td>
                <td>${conn.client_ip}</td>
                <td>${Utils.formatTimestamp(conn.created_at)}</td>
                <td>${conn.endpoint || '-'}</td>
                <td><span class="status-badge status-${conn.status}">${Utils.getStatusText(conn.status)}</span></td>
                <td>${Utils.formatDuration(conn.duration)}ms</td>
            </tr>
        `).join('');
    }
    
    /**
     * 判断是否影响当前页面
     * @param {Object} requestData 请求数据
     * @returns {boolean} 是否影响当前页面
     */
    isCurrentPageAffected(requestData) {
        // 简化实现：检查是否有新请求或状态变化
        if (requestData.new_requests && requestData.new_requests.length > 0) {
            return true;
        }
        if (requestData.updated_requests && requestData.updated_requests.length > 0) {
            return true;
        }
        return false;
    }
    
    /**
     * 显示新请求通知
     * @param {Object} data 请求数据
     */
    showNewRequestNotification(data) {
        if (data.new_requests_count > 0) {
            Utils.showInfo(`有 ${data.new_requests_count} 个新请求，点击请求页面查看`);
        }
        if (data.failed_requests_count > 0) {
            Utils.showWarning(`有 ${data.failed_requests_count} 个请求失败`);
        }
    }
    
    /**
     * 更新健康状态图标
     * @param {number} healthyCount 健康端点数量
     * @param {number} totalCount 总端点数量
     */
    updateHealthStatusIcon(healthyCount, totalCount) {
        const healthPercent = totalCount > 0 ? (healthyCount / totalCount) * 100 : 0;
        const statusElement = document.getElementById('endpoint-health-status');
        
        if (statusElement) {
            let statusClass, statusText;
            
            if (healthPercent === 100) {
                statusClass = 'status-healthy';
                statusText = '🟢 全部健康';
            } else if (healthPercent > 50) {
                statusClass = 'status-degraded';
                statusText = '🟡 部分异常';
            } else if (healthPercent > 0) {
                statusClass = 'status-unhealthy';  
                statusText = '🔴 状态异常';
            } else {
                statusClass = 'status-critical';
                statusText = '💀 全部离线';
            }
            
            statusElement.className = statusClass;
            statusElement.textContent = statusText;
        }
    }
    
    /**
     * 清除待处理更新
     * @param {string} eventType 事件类型
     */
    clearPendingUpdate(eventType) {
        const timerId = this.pendingUpdates.get(eventType);
        if (timerId) {
            clearTimeout(timerId);
            this.pendingUpdates.delete(eventType);
        }
    }
    
    /**
     * 记录最后更新
     * @param {string} updateKey 更新键
     * @param {Object} data 数据
     */
    recordLastUpdate(updateKey, data) {
        this.lastUpdate.set(updateKey, data);
    }
    
    /**
     * 获取性能统计信息
     * @returns {Object} 统计信息
     */
    getStats() {
        const runtime = Date.now() - this.stats.startTime;
        const stats = {
            ...this.stats,
            runtime: runtime,
            updatesPerSecond: (this.stats.totalUpdates / runtime * 1000).toFixed(2),
            queueSize: this.updateQueue.size(),
            pendingUpdates: this.pendingUpdates.size,
            domUpdates: {
                pendingDOMUpdates: this.pendingDOMUpdates ? this.pendingDOMUpdates.size : 0,
                elementCacheSize: this.elementCache ? this.elementCache.size : 0,
                batchDOMUpdatesProcessed: this.stats.batchDOMUpdatesProcessed || 0
            }
        };

        // 添加性能统计
        if (this.performanceStats && this.performanceStats.size > 0) {
            stats.performance = {};
            this.performanceStats.forEach((perf, updateType) => {
                stats.performance[updateType] = {
                    avgTime: perf.avgTime.toFixed(2) + 'ms',
                    maxTime: perf.maxTime.toFixed(2) + 'ms',
                    minTime: perf.minTime.toFixed(2) + 'ms',
                    count: perf.count
                };
            });
        }

        return stats;
    }
    
    /**
     * 获取性能报告
     * @returns {string} 格式化的性能报告
     */
    getPerformanceReport() {
        const stats = this.getStats();
        let report = `
智能更新管理器性能报告:
- 运行时间: ${Utils.formatDuration(stats.runtime)}
- 总更新数: ${stats.totalUpdates}
- 立即更新: ${stats.immediateUpdates} (${(stats.immediateUpdates/stats.totalUpdates*100).toFixed(1)}%)
- 批量更新: ${stats.batchedUpdates} (${(stats.batchedUpdates/stats.totalUpdates*100).toFixed(1)}%)
- 丢弃更新: ${stats.droppedUpdates} (${(stats.droppedUpdates/stats.totalUpdates*100).toFixed(1)}%)
- 更新频率: ${stats.updatesPerSecond} 次/秒
- 队列大小: ${stats.queueSize}
- 待处理: ${stats.pendingUpdates}

DOM优化统计:
- 批量DOM更新: ${stats.domUpdates.batchDOMUpdatesProcessed}
- 待处理DOM更新: ${stats.domUpdates.pendingDOMUpdates}
- 元素缓存大小: ${stats.domUpdates.elementCacheSize}`;

        // 添加性能统计详情
        if (stats.performance) {
            report += `\n\n性能分析:`;
            Object.entries(stats.performance).forEach(([updateType, perf]) => {
                report += `\n- ${updateType}: 平均${perf.avgTime}, 最大${perf.maxTime}, 执行${perf.count}次`;
            });
        }
        
        return report.trim();
    }
    
    /**
     * 强制处理所有待处理更新
     */
    flushPendingUpdates() {
        // 处理所有待处理的延迟更新
        this.pendingUpdates.forEach((timerId, eventType) => {
            clearTimeout(timerId);
            const data = this.webInterface.cachedData[eventType];
            if (data) {
                this.updateRelevantUI(eventType, data);
            }
        });
        this.pendingUpdates.clear();
        
        // 处理所有批量更新
        if (this.batchTimer) {
            clearTimeout(this.batchTimer);
            this.batchTimer = null;
            this.processBatchUpdates();
        }
        
        console.log('💨 强制处理完所有待处理更新');
    }
    
    /**
     * DOM更新性能优化方法
     */
    optimizedDOMUpdate(element, newContent) {
        // 避免不必要的DOM操作
        if (element.innerHTML === newContent) {
            return false; // 内容未变化，跳过更新
        }
        
        // 使用DocumentFragment进行批量更新
        const fragment = document.createDocumentFragment();
        const tempDiv = document.createElement('div');
        tempDiv.innerHTML = newContent;
        
        while (tempDiv.firstChild) {
            fragment.appendChild(tempDiv.firstChild);
        }
        
        // 一次性替换内容
        element.innerHTML = '';
        element.appendChild(fragment);
        
        return true; // 更新完成
    }

    /**
     * 批量DOM更新队列处理
     */
    processBatchDOMUpdates() {
        const updates = [];
        
        // 收集所有待更新的元素
        this.pendingDOMUpdates.forEach((data, elementId) => {
            const element = document.getElementById(elementId);
            if (element) {
                updates.push({ element, data });
            }
        });
        
        // 使用requestAnimationFrame优化DOM更新
        requestAnimationFrame(() => {
            updates.forEach(({ element, data }) => {
                if (data.content !== undefined) {
                    this.optimizedDOMUpdate(element, data.content);
                } else if (data.text !== undefined) {
                    if (element.textContent !== data.text) {
                        element.textContent = data.text;
                    }
                } else if (data.attributes) {
                    // 批量更新属性
                    Object.entries(data.attributes).forEach(([attr, value]) => {
                        if (element.getAttribute(attr) !== value) {
                            element.setAttribute(attr, value);
                        }
                    });
                }
            });
            
            this.pendingDOMUpdates.clear();
            this.stats.batchDOMUpdatesProcessed++;
        });
    }

    /**
     * 虚拟DOM差异更新
     */
    virtualDOMDiff(oldElement, newContent) {
        // 简化的虚拟DOM差异检测
        const tempDiv = document.createElement('div');
        tempDiv.innerHTML = newContent;
        
        const newElement = tempDiv.firstElementChild;
        if (!newElement) return false;
        
        // 比较属性差异
        const attributeChanges = [];
        if (oldElement.attributes && newElement.attributes) {
            for (let attr of newElement.attributes) {
                if (oldElement.getAttribute(attr.name) !== attr.value) {
                    attributeChanges.push({ name: attr.name, value: attr.value });
                }
            }
        }
        
        // 比较文本内容差异
        const textChanged = oldElement.textContent !== newElement.textContent;
        
        // 应用最小化变更
        if (attributeChanges.length > 0) {
            attributeChanges.forEach(change => {
                oldElement.setAttribute(change.name, change.value);
            });
        }
        
        if (textChanged) {
            oldElement.textContent = newElement.textContent;
        }
        
        return attributeChanges.length > 0 || textChanged;
    }

    /**
     * 智能DOM更新调度
     */
    scheduleDOMUpdate(elementId, updateData, priority = 'normal') {
        if (!this.pendingDOMUpdates) {
            this.pendingDOMUpdates = new Map();
        }
        
        // 合并同一元素的更新
        const existing = this.pendingDOMUpdates.get(elementId);
        if (existing) {
            // 合并更新数据
            this.pendingDOMUpdates.set(elementId, { ...existing, ...updateData });
        } else {
            this.pendingDOMUpdates.set(elementId, updateData);
        }
        
        // 根据优先级调度更新
        if (priority === 'high') {
            // 高优先级立即处理
            requestAnimationFrame(() => {
                const element = document.getElementById(elementId);
                if (element && this.pendingDOMUpdates.has(elementId)) {
                    const data = this.pendingDOMUpdates.get(elementId);
                    if (data.content !== undefined) {
                        this.optimizedDOMUpdate(element, data.content);
                    }
                    this.pendingDOMUpdates.delete(elementId);
                }
            });
        } else {
            // 中低优先级批量处理
            if (!this.domUpdateTimer) {
                this.domUpdateTimer = setTimeout(() => {
                    this.processBatchDOMUpdates();
                    this.domUpdateTimer = null;
                }, priority === 'normal' ? 100 : 500);
            }
        }
    }

    /**
     * 内存优化的元素缓存
     */
    getCachedElement(elementId) {
        if (!this.elementCache) {
            this.elementCache = new Map();
        }
        
        let element = this.elementCache.get(elementId);
        if (!element || !document.contains(element)) {
            element = document.getElementById(elementId);
            if (element) {
                this.elementCache.set(elementId, element);
                
                // 清理无效缓存（防止内存泄漏）
                if (this.elementCache.size > 100) {
                    const entries = Array.from(this.elementCache.entries());
                    entries.slice(0, 20).forEach(([id]) => {
                        if (!document.contains(this.elementCache.get(id))) {
                            this.elementCache.delete(id);
                        }
                    });
                }
            }
        }
        
        return element;
    }

    /**
     * 性能监控增强
     */
    measureUpdatePerformance(updateType, callback) {
        const startTime = performance.now();
        const result = callback();
        const endTime = performance.now();
        
        if (!this.performanceStats) {
            this.performanceStats = new Map();
        }
        
        const stats = this.performanceStats.get(updateType) || { 
            totalTime: 0, 
            count: 0, 
            maxTime: 0,
            minTime: Infinity
        };
        
        const duration = endTime - startTime;
        stats.totalTime += duration;
        stats.count++;
        stats.maxTime = Math.max(stats.maxTime, duration);
        stats.minTime = Math.min(stats.minTime, duration);
        stats.avgTime = stats.totalTime / stats.count;
        
        this.performanceStats.set(updateType, stats);
        
        // 性能警告
        if (duration > 50) {
            console.warn(`⚠️ 慢DOM更新检测: ${updateType} 耗时 ${duration.toFixed(2)}ms`);
        }
        
        return result;
    }
    
    /**
     * 清理资源
     */
    destroy() {
        // 清理所有待处理的定时器
        this.pendingUpdates.forEach(timerId => clearTimeout(timerId));
        this.pendingUpdates.clear();
        
        if (this.batchTimer) {
            clearTimeout(this.batchTimer);
        }
        
        // 清理DOM更新定时器
        if (this.domUpdateTimer) {
            clearTimeout(this.domUpdateTimer);
            this.domUpdateTimer = null;
        }
        
        // 清理缓存
        this.uiUpdateCache.clear();
        this.lastUIUpdate.clear();
        this.lastUpdate.clear();
        
        // 清理DOM相关缓存
        if (this.pendingDOMUpdates) {
            this.pendingDOMUpdates.clear();
        }
        
        if (this.elementCache) {
            this.elementCache.clear();
        }
        
        if (this.performanceStats) {
            this.performanceStats.clear();
        }
        
        // 重置统计
        this.stats = {
            totalUpdates: 0,
            immediateUpdates: 0,
            batchedUpdates: 0,
            droppedUpdates: 0,
            batchDOMUpdatesProcessed: 0,
            startTime: Date.now()
        };
        
        console.log('🧹 智能更新管理器资源清理完成');
    }
}

/**
 * 简单的优先级队列实现
 */
class PriorityQueue {
    constructor() {
        this.items = [];
    }
    
    enqueue(item) {
        this.items.push(item);
    }
    
    dequeue() {
        return this.items.shift();
    }
    
    isEmpty() {
        return this.items.length === 0;
    }
    
    size() {
        return this.items.length;
    }
    
    clear() {
        this.items = [];
    }
}

console.log('✅ SmartUpdateManager模块已加载');
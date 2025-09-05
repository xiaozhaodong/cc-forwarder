// Claude Request Forwarder - 端点管理模块
// 处理端点显示、优先级更新和健康检测

window.EndpointsManager = class {
    constructor(webInterface) {
        this.webInterface = webInterface;
    }
    
    // 加载端点数据
    async loadEndpoints() {
        try {
            const response = await fetch('/api/v1/endpoints');
            const data = await response.json();

            // 更新缓存
            this.webInterface.cachedData.endpoints = data;
            console.log('[API] endpoints数据已加载并缓存');

            const container = document.getElementById('endpoints-table');
            if (data.endpoints && data.endpoints.length > 0) {
                container.innerHTML = this.generateEndpointsTable(data.endpoints);
                this.bindEndpointEvents();
            } else {
                container.innerHTML = '<p>暂无端点数据</p>';
            }
        } catch (error) {
            console.error('加载端点数据失败:', error);
            Utils.showError('端点数据加载失败');
        }
    }
    
    // 生成端点表格HTML
    generateEndpointsTable(endpoints) {
        let html = `
            <table>
                <thead>
                    <tr>
                        <th>状态</th>
                        <th>名称</th>
                        <th>URL</th>
                        <th>优先级</th>
                        <th>组</th>
                        <th>响应时间</th>
                        <th>最后检查</th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody>
        `;

        endpoints.forEach(endpoint => {
            let statusClass, statusText;
            
            // 根据never_checked字段决定状态显示
            if (endpoint.never_checked) {
                statusClass = 'status-never-checked';
                statusText = '未检测';
            } else if (endpoint.healthy) {
                statusClass = 'status-healthy';
                statusText = '健康';
            } else {
                statusClass = 'status-unhealthy';
                statusText = '不健康';
            }
            
            html += `
                <tr>
                    <td>
                        <span class="status-indicator ${statusClass}"></span>
                        ${statusText}
                    </td>
                    <td>${endpoint.name}</td>
                    <td>${endpoint.url}</td>
                    <td>
                        <input type="number" 
                               class="priority-input" 
                               value="${endpoint.priority}" 
                               data-endpoint="${endpoint.name}"
                               min="1">
                    </td>
                    <td>${endpoint.group} (${endpoint.group_priority})</td>
                    <td>${endpoint.response_time}</td>
                    <td>${endpoint.last_check}</td>
                    <td>
                        <button class="btn btn-sm update-priority" data-endpoint="${endpoint.name}">
                            更新
                        </button>
                        <button class="btn btn-sm manual-health-check" data-endpoint="${endpoint.name}" title="手动健康检测">
                            检测
                        </button>
                    </td>
                </tr>
            `;
        });

        html += '</tbody></table>';
        return html;
    }
    
    // 绑定端点事件
    bindEndpointEvents() {
        // 绑定优先级更新按钮事件
        document.querySelectorAll('.update-priority').forEach(button => {
            button.addEventListener('click', async (e) => {
                const endpointName = e.target.dataset.endpoint;
                const priorityInput = document.querySelector(`input[data-endpoint="${endpointName}"]`);
                const newPriority = parseInt(priorityInput.value);

                if (newPriority < 1) {
                    Utils.showError('优先级必须大于等于1');
                    return;
                }

                try {
                    const response = await fetch(`/api/v1/endpoints/${endpointName}/priority`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                        },
                        body: JSON.stringify({ priority: newPriority })
                    });

                    const result = await response.json();
                    if (result.success) {
                        Utils.showSuccess('优先级更新成功');
                        this.loadEndpoints(); // 重新加载端点数据
                    } else {
                        Utils.showError(result.error || '更新失败');
                    }
                } catch (error) {
                    console.error('更新优先级失败:', error);
                    Utils.showError('更新优先级失败');
                }
            });
        });

        // 绑定回车键更新事件
        document.querySelectorAll('.priority-input').forEach(input => {
            input.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    const updateButton = document.querySelector(`.update-priority[data-endpoint="${input.dataset.endpoint}"]`);
                    updateButton.click();
                }
            });
        });

        // 绑定手动健康检测按钮事件
        document.querySelectorAll('.manual-health-check').forEach(button => {
            button.addEventListener('click', async (e) => {
                const endpointName = e.target.dataset.endpoint;
                const originalText = e.target.innerHTML;
                
                // 显示加载状态
                e.target.innerHTML = '检测中...';
                e.target.disabled = true;

                try {
                    const response = await fetch(`/api/v1/endpoints/${endpointName}/health-check`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                        }
                    });

                    const result = await response.json();
                    if (result.success) {
                        const healthText = result.healthy ? '健康' : '不健康';
                        Utils.showSuccess(`手动检测完成 - ${endpointName}: ${healthText}`);
                        this.loadEndpoints(); // 重新加载端点数据
                    } else {
                        Utils.showError(result.error || '手动检测失败');
                    }
                } catch (error) {
                    console.error('手动检测失败:', error);
                    Utils.showError('手动检测失败');
                } finally {
                    // 恢复按钮状态
                    e.target.innerHTML = originalText;
                    e.target.disabled = false;
                }
            });
        });
    }
    
    // 更新端点优先级
    async updateEndpointPriority(endpointName, newPriority) {
        try {
            const response = await fetch(`/api/v1/endpoints/${endpointName}/priority`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ priority: newPriority })
            });

            const result = await response.json();
            if (result.success) {
                Utils.showSuccess(`端点 ${endpointName} 优先级已更新为 ${newPriority}`);
                await this.loadEndpoints(); // 重新加载数据
                return true;
            } else {
                Utils.showError(result.error || '更新失败');
                return false;
            }
        } catch (error) {
            console.error('更新端点优先级失败:', error);
            Utils.showError('更新端点优先级失败');
            return false;
        }
    }
    
    // 执行手动健康检测
    async performHealthCheck(endpointName) {
        try {
            const response = await fetch(`/api/v1/endpoints/${endpointName}/health-check`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                }
            });

            const result = await response.json();
            if (result.success) {
                const healthText = result.healthy ? '健康' : '不健康';
                Utils.showSuccess(`健康检测完成 - ${endpointName}: ${healthText}`);
                await this.loadEndpoints(); // 重新加载数据
                return result.healthy;
            } else {
                Utils.showError(result.error || '健康检测失败');
                return null;
            }
        } catch (error) {
            console.error('健康检测失败:', error);
            Utils.showError('健康检测失败');
            return null;
        }
    }
    
    // 获取端点列表
    getEndpoints() {
        const cachedData = this.webInterface.cachedData.endpoints;
        return cachedData ? cachedData.endpoints || [] : [];
    }
    
    // 根据名称获取端点信息
    getEndpointByName(endpointName) {
        const endpoints = this.getEndpoints();
        return endpoints.find(endpoint => endpoint.name === endpointName);
    }
    
    // 获取健康的端点
    getHealthyEndpoints() {
        const endpoints = this.getEndpoints();
        return endpoints.filter(endpoint => endpoint.healthy && !endpoint.never_checked);
    }
    
    // 获取不健康的端点
    getUnhealthyEndpoints() {
        const endpoints = this.getEndpoints();
        return endpoints.filter(endpoint => !endpoint.healthy && !endpoint.never_checked);
    }
    
    // 获取未检测的端点
    getUncheckedEndpoints() {
        const endpoints = this.getEndpoints();
        return endpoints.filter(endpoint => endpoint.never_checked);
    }
    
    // 按组分组端点
    getEndpointsByGroup() {
        const endpoints = this.getEndpoints();
        const grouped = {};
        
        endpoints.forEach(endpoint => {
            const group = endpoint.group || 'default';
            if (!grouped[group]) {
                grouped[group] = [];
            }
            grouped[group].push(endpoint);
        });
        
        return grouped;
    }
    
    // 获取端点统计信息
    getEndpointsStats() {
        const endpoints = this.getEndpoints();
        if (endpoints.length === 0) return null;
        
        const healthy = endpoints.filter(e => e.healthy && !e.never_checked).length;
        const unhealthy = endpoints.filter(e => !e.healthy && !e.never_checked).length;
        const unchecked = endpoints.filter(e => e.never_checked).length;
        
        return {
            total: endpoints.length,
            healthy,
            unhealthy,
            unchecked,
            healthPercentage: endpoints.length > 0 ? ((healthy / endpoints.length) * 100).toFixed(1) : 0
        };
    }
    
    // 格式化端点状态
    formatEndpointStatus(endpoint) {
        if (endpoint.never_checked) return '未检测';
        return endpoint.healthy ? '健康' : '不健康';
    }
    
    // 获取端点状态样式类
    getEndpointStatusClass(endpoint) {
        if (endpoint.never_checked) return 'status-never-checked';
        return endpoint.healthy ? 'status-healthy' : 'status-unhealthy';
    }
    
    // 批量更新端点优先级
    async updateMultipleEndpointsPriority(updates) {
        const results = [];
        for (const update of updates) {
            const success = await this.updateEndpointPriority(update.name, update.priority);
            results.push({ 
                endpoint: update.name, 
                priority: update.priority, 
                success 
            });
        }
        return results;
    }
    
    // 批量执行健康检测
    async performBatchHealthCheck(endpointNames = null) {
        const endpoints = endpointNames || this.getEndpoints().map(e => e.name);
        const results = [];
        
        for (const endpointName of endpoints) {
            const result = await this.performHealthCheck(endpointName);
            results.push({
                endpoint: endpointName,
                healthy: result
            });
        }
        
        return results;
    }
    
    // 刷新端点数据（强制重新加载）
    async refreshEndpoints() {
        try {
            await this.loadEndpoints();
        } catch (error) {
            console.error('刷新端点数据失败:', error);
            Utils.showError('刷新端点数据失败');
        }
    }
    
    // 搜索端点
    searchEndpoints(query) {
        const endpoints = this.getEndpoints();
        if (!query) return endpoints;
        
        const lowerQuery = query.toLowerCase();
        return endpoints.filter(endpoint => 
            endpoint.name.toLowerCase().includes(lowerQuery) ||
            endpoint.url.toLowerCase().includes(lowerQuery) ||
            endpoint.group.toLowerCase().includes(lowerQuery)
        );
    }
    
    // 按优先级排序端点
    sortEndpointsByPriority(ascending = true) {
        const endpoints = this.getEndpoints();
        return endpoints.sort((a, b) => {
            return ascending ? a.priority - b.priority : b.priority - a.priority;
        });
    }
    
    // 按响应时间排序端点
    sortEndpointsByResponseTime(ascending = true) {
        const endpoints = this.getEndpoints();
        return endpoints.sort((a, b) => {
            const timeA = this.parseResponseTime(a.response_time);
            const timeB = this.parseResponseTime(b.response_time);
            return ascending ? timeA - timeB : timeB - timeA;
        });
    }
    
    // 解析响应时间字符串为毫秒
    parseResponseTime(timeString) {
        if (!timeString || timeString === '-') return Infinity;
        
        const match = timeString.match(/(\d+(?:\.\d+)?)(ms|s|m)/);
        if (!match) return Infinity;
        
        const value = parseFloat(match[1]);
        const unit = match[2];
        
        switch (unit) {
            case 'ms': return value;
            case 's': return value * 1000;
            case 'm': return value * 60000;
            default: return Infinity;
        }
    }
    
    // 获取最快响应的端点
    getFastestEndpoint() {
        const endpoints = this.getHealthyEndpoints();
        if (endpoints.length === 0) return null;
        
        return endpoints.reduce((fastest, current) => {
            const fastestTime = this.parseResponseTime(fastest.response_time);
            const currentTime = this.parseResponseTime(current.response_time);
            return currentTime < fastestTime ? current : fastest;
        });
    }
    
    // 获取最慢响应的端点
    getSlowestEndpoint() {
        const endpoints = this.getHealthyEndpoints();
        if (endpoints.length === 0) return null;
        
        return endpoints.reduce((slowest, current) => {
            const slowestTime = this.parseResponseTime(slowest.response_time);
            const currentTime = this.parseResponseTime(current.response_time);
            return currentTime > slowestTime ? current : slowest;
        });
    }
};

console.log('✅ EndpointsManager模块已加载');
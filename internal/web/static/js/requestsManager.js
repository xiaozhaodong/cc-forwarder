// Claude Request Forwarder - 请求追踪管理模块
// 处理请求列表、筛选、分页、导出和详情显示

window.RequestsManager = class {
    constructor(webInterface) {
        this.webInterface = webInterface;
        
        // 请求追踪页面状态
        this.state = {
            currentPage: 1,
            pageSize: 50,
            totalRequests: 0,
            filters: {
                start_date: '',
                end_date: '',
                status: '',
                model: '',
                endpoint: '',
                group: ''
            },
            sortBy: 'start_time',
            sortOrder: 'desc'
        };
    }
    
    // 加载请求数据
    async loadRequests() {
        try {
            // 构建查询参数
            const params = new URLSearchParams({
                limit: this.state.pageSize.toString(),
                offset: ((this.state.currentPage - 1) * this.state.pageSize).toString(),
                sort_by: this.state.sortBy,
                sort_order: this.state.sortOrder
            });
            
            // 添加筛选参数
            Object.entries(this.state.filters).forEach(([key, value]) => {
                if (value && value.trim() !== '') {
                    params.append(key, value.trim());
                }
            });
            
            const response = await fetch(`/api/v1/usage/requests?${params}`);
            if (!response.ok) {
                throw new Error('获取请求数据失败');
            }
            
            const data = await response.json();
            
            // 更新缓存
            this.webInterface.cachedData.requests = data;
            
            // 更新状态
            this.state.totalRequests = data.total || 0;
            
            const tbody = document.getElementById('requests-table-body');
            if (!tbody) {
                console.error('找不到requests-table-body元素');
                return;
            }
            
            if (data.data && data.data.length > 0) {
                tbody.innerHTML = this.generateRequestsRows(data.data);
                this.bindRequestsEvents();
                // 更新计数信息
                this.updateRequestsCountInfo(data.total, this.state.currentPage);
            } else {
                tbody.innerHTML = '<tr><td colspan="12" class="no-data">🔄 暂无请求数据</td></tr>';
                this.updateRequestsCountInfo(0, this.state.currentPage);
            }
            
            // 同时加载统计数据
            this.loadRequestsStats();
        } catch (error) {
            console.error('加载请求数据失败:', error);
            Utils.showError('请求数据加载失败: ' + error.message);
        }
    }
    
    // 加载模型选项（动态从配置读取）
    async loadModelOptions() {
        try {
            const response = await fetch('/api/v1/usage/models');
            if (!response.ok) {
                console.warn('Failed to load models from config, using default options');
                return;
            }
            
            const result = await response.json();
            if (result.success && result.data) {
                const modelSelect = document.getElementById('model-filter');
                if (modelSelect) {
                    // 清空现有选项（保留"全部模型"选项）
                    const allOption = modelSelect.querySelector('option[value=""]');
                    modelSelect.innerHTML = '';
                    if (allOption) {
                        modelSelect.appendChild(allOption);
                    } else {
                        modelSelect.innerHTML = '<option value="">全部模型</option>';
                    }
                    
                    // 添加从配置读取的模型
                    result.data.forEach(model => {
                        const option = document.createElement('option');
                        option.value = model.model_name;
                        option.textContent = model.display_name || model.model_name;
                        if (this.state.filters.model === model.model_name) {
                            option.selected = true;
                        }
                        modelSelect.appendChild(option);
                    });
                }
            }
        } catch (error) {
            console.warn('Error loading model options:', error);
        }
    }
    
    // 生成请求表格行内容（只生成tbody内的tr元素）
    generateRequestsRows(requests) {
        if (!requests || requests.length === 0) {
            return '<tr><td colspan="12" class="no-data">🔄 暂无请求数据</td></tr>';
        }

        let html = '';
        requests.forEach(request => {
            const status = Utils.formatRequestStatus(request.status);
            const duration = Utils.formatDuration(request.duration_ms);
            const cost = Utils.formatCost(request.total_cost_usd);
            const startTime = new Date(request.start_time).toLocaleString('zh-CN');
            
            // 生成流式图标
            const streamingIcon = request.is_streaming ? '🌊' : '🔄';
            
            html += `
                <tr>
                    <td>
                        <span title="${request.is_streaming ? '流式请求' : '常规请求'}">${streamingIcon}</span>
                        <code class="request-id">${request.request_id}</code>
                    </td>
                    <td class="datetime">${startTime}</td>
                    <td>
                        <span class="status-badge status-${request.status}">${status}</span>
                    </td>
                    <td class="model-name">${request.model_name || '-'}</td>
                    <td class="endpoint-name">${request.endpoint_name || '-'}</td>
                    <td class="group-name">${request.group_name || '-'}</td>
                    <td class="duration">${duration}</td>
                    <td class="input-tokens">${request.input_tokens || 0}</td>
                    <td class="output-tokens">${request.output_tokens || 0}</td>
                    <td class="cache-creation-tokens">${request.cache_creation_tokens || 0}</td>
                    <td class="cache-read-tokens">${request.cache_read_tokens || 0}</td>
                    <td class="cost">${cost}</td>
                    <td class="actions">
                        <button class="btn btn-sm" onclick="window.webInterface.requestsManager.showRequestDetail('${request.request_id}')">
                            查看
                        </button>
                    </td>
                </tr>
            `;
        });
        
        return html;
    }
    
    // 生成完整的请求表格（包含筛选和分页）
    generateRequestsTable(requests, total, currentPage) {
        const startIndex = (currentPage - 1) * this.state.pageSize + 1;
        const endIndex = Math.min(startIndex + requests.length - 1, total);
        
        let html = `
            <div class="requests-header">
                <div class="requests-filters">
                    <form id="requests-filter-form" class="filter-form">
                        <div class="filter-row">
                            <div class="filter-group">
                                <label for="start-date">开始日期:</label>
                                <input type="date" id="start-date" name="start_date" value="${this.state.filters.start_date}">
                            </div>
                            <div class="filter-group">
                                <label for="end-date">结束日期:</label>
                                <input type="date" id="end-date" name="end_date" value="${this.state.filters.end_date}">
                            </div>
                            <div class="filter-group">
                                <label for="status-filter">状态:</label>
                                <select id="status-filter" name="status">
                                    <option value="">全部</option>
                                    <option value="completed" ${this.state.filters.status === 'completed' ? 'selected' : ''}>完成</option>
                                    <option value="processing" ${this.state.filters.status === 'processing' ? 'selected' : ''}>解析中</option>
                                    <option value="forwarding" ${this.state.filters.status === 'forwarding' ? 'selected' : ''}>转发中</option>
                                    <option value="suspended" ${this.state.filters.status === 'suspended' ? 'selected' : ''}>挂起中</option>
                                    <option value="error" ${this.state.filters.status === 'error' ? 'selected' : ''}>失败</option>
                                    <option value="cancelled" ${this.state.filters.status === 'cancelled' ? 'selected' : ''}>取消</option>
                                    <option value="timeout" ${this.state.filters.status === 'timeout' ? 'selected' : ''}>超时</option>
                                </select>
                            </div>
                        </div>
                        <div class="filter-row">
                            <div class="filter-group">
                                <label for="model-filter">模型:</label>
                                <select id="model-filter" name="model">
                                    <option value="">全部模型</option>
                                    <!-- 模型选项将动态加载 -->
                                </select>
                            </div>
                            <div class="filter-group">
                                <label for="endpoint-filter">端点:</label>
                                <input type="text" id="endpoint-filter" name="endpoint" value="${this.state.filters.endpoint}" placeholder="输入端点名称">
                            </div>
                            <div class="filter-group">
                                <label for="group-filter">组:</label>
                                <input type="text" id="group-filter" name="group" value="${this.state.filters.group}" placeholder="输入组名">
                            </div>
                            <div class="filter-group">
                                <button type="submit" class="btn btn-primary">筛选</button>
                                <button type="button" class="btn btn-secondary" onclick="webInterface.requestsManager.resetFilters()">重置</button>
                            </div>
                        </div>
                    </form>
                </div>
                
                <div class="requests-actions">
                    <div class="requests-info">
                        显示 ${startIndex}-${endIndex} / 共 ${total} 条记录
                    </div>
                    <div class="export-actions">
                        <button class="btn btn-outline" onclick="webInterface.requestsManager.exportRequests('csv')">导出CSV</button>
                        <button class="btn btn-outline" onclick="webInterface.requestsManager.exportRequests('json')">导出JSON</button>
                    </div>
                </div>
            </div>
            
            <table class="requests-table">
                <thead>
                    <tr>
                        <th class="sortable" data-sort="request_id">
                            请求ID
                            ${Utils.getSortIcon('request_id', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="start_time">
                            开始时间
                            ${Utils.getSortIcon('start_time', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="status">
                            状态
                            ${Utils.getSortIcon('status', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="model_name">
                            模型
                            ${Utils.getSortIcon('model_name', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="endpoint_name">
                            端点
                            ${Utils.getSortIcon('endpoint_name', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="group_name">
                            组
                            ${Utils.getSortIcon('group_name', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="duration_ms">
                            耗时
                            ${Utils.getSortIcon('duration_ms', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th class="sortable" data-sort="input_tokens">输入Token</th>
                        <th class="sortable" data-sort="output_tokens">输出Token</th>
                        <th class="sortable" data-sort="cache_creation_tokens">缓存创建</th>
                        <th class="sortable" data-sort="cache_read_tokens">缓存读取</th>
                        <th class="sortable" data-sort="total_cost_usd">
                            成本
                            ${Utils.getSortIcon('total_cost_usd', this.state.sortBy, this.state.sortOrder)}
                        </th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody id="requests-table-body">
        `;

        html += this.generateRequestsRows(requests);

        html += `
                </tbody>
            </table>
            
            <div class="pagination">
                ${Utils.generatePagination(total, currentPage, this.state.pageSize, 'requestsManager')}
            </div>
        `;
        
        return html;
    }
    
    // 更新请求计数信息
    updateRequestsCountInfo(total, currentPage) {
        const countInfoElement = document.getElementById('requests-count-info');
        if (countInfoElement) {
            const startIndex = (currentPage - 1) * this.state.pageSize + 1;
            const endIndex = Math.min(startIndex + this.state.pageSize - 1, total);
            countInfoElement.textContent = `显示 ${startIndex}-${endIndex} 条，共 ${total} 条记录`;
        }
        
        // 更新分页控件
        const totalPagesElement = document.getElementById('total-pages');
        const currentPageInput = document.getElementById('current-page-input');
        
        if (totalPagesElement && currentPageInput) {
            const totalPages = Math.ceil(total / this.state.pageSize);
            totalPagesElement.textContent = totalPages;
            currentPageInput.value = currentPage;
            currentPageInput.max = totalPages;
        }
    }
    
    // 绑定请求相关事件
    bindRequestsEvents() {
        // 加载模型选项
        this.loadModelOptions();
        
        // 筛选表单事件 - 先移除之前的事件监听器，避免重复绑定
        const filterForm = document.getElementById('requests-filter-form');
        if (filterForm) {
            // 移除之前可能存在的事件监听器
            const newForm = filterForm.cloneNode(true);
            filterForm.parentNode.replaceChild(newForm, filterForm);
            
            // 重新绑定事件
            newForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.handleRequestsFilter();
            });
        }
        
        // 排序事件 - 也需要避免重复绑定
        document.querySelectorAll('.sortable').forEach(th => {
            // 克隆元素来移除所有事件监听器
            const newTh = th.cloneNode(true);
            th.parentNode.replaceChild(newTh, th);
            
            newTh.addEventListener('click', () => {
                const sortBy = newTh.dataset.sort;
                if (this.state.sortBy === sortBy) {
                    this.state.sortOrder = this.state.sortOrder === 'asc' ? 'desc' : 'asc';
                } else {
                    this.state.sortBy = sortBy;
                    this.state.sortOrder = 'desc';
                }
                this.state.currentPage = 1; // 重置到第一页
                this.loadRequests();
            });
        });
    }
    
    // 处理筛选
    handleRequestsFilter() {
        const formData = new FormData(document.getElementById('requests-filter-form'));
        
        // 更新筛选状态
        this.state.filters = {
            start_date: formData.get('start_date') || '',
            end_date: formData.get('end_date') || '',
            status: formData.get('status') || '',
            model: formData.get('model') || '',
            endpoint: formData.get('endpoint') || '',
            group: formData.get('group') || ''
        };
        
        // 重置到第一页
        this.state.currentPage = 1;
        
        // 重新加载数据
        this.loadRequests();
    }
    
    // 重置筛选条件
    resetFilters() {
        // 清空筛选条件
        this.state.filters = {
            start_date: '',
            end_date: '',
            status: '',
            model: '',
            endpoint: '',
            group: ''
        };
        
        // 重置到第一页
        this.state.currentPage = 1;
        
        // 重新加载数据
        this.loadRequests();
    }
    
    // 切换页面
    changePage(page) {
        this.state.currentPage = page;
        this.loadRequests();
    }
    
    // 显示请求详情
    showRequestDetail(requestId) {
        // 从缓存数据中找到对应的请求
        const requests = this.webInterface.cachedData.requests?.data || [];
        const request = requests.find(r => r.request_id === requestId);
        
        if (!request) {
            Utils.showError('未找到请求详情数据');
            return;
        }
        
        this.displayRequestDetailModal(request);
    }
    
    // 显示请求详情模态框
    displayRequestDetailModal(request) {
        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal-content request-detail-modal">
                <div class="modal-header">
                    <h3>🔍 请求详情</h3>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="detail-section">
                        <div class="detail-section-title">📋 基本信息</div>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <label>请求ID:</label>
                                <code class="detail-value">${request.request_id}</code>
                            </div>
                            <div class="detail-item">
                                <label>状态:</label>
                                <span class="status-badge status-${request.status}">${Utils.formatRequestStatus(request.status)}</span>
                            </div>
                            <div class="detail-item">
                                <label>开始时间:</label>
                                <span class="detail-value">${new Date(request.start_time).toLocaleString('zh-CN')}</span>
                            </div>
                            <div class="detail-item">
                                <label>更新时间:</label>
                                <span class="detail-value">${new Date(request.updated_at).toLocaleString('zh-CN')}</span>
                            </div>
                        </div>
                    </div>

                    <div class="detail-section">
                        <div class="detail-section-title">🌐 网络信息</div>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <label>请求方法:</label>
                                <span class="detail-value method-badge">${request.method || 'POST'}</span>
                            </div>
                            <div class="detail-item">
                                <label>请求路径:</label>
                                <code class="detail-value request-path">${request.path || '/v1/messages'}</code>
                            </div>
                            <div class="detail-item">
                                <label>请求类型:</label>
                                <span class="detail-value" title="${request.is_streaming ? '流式请求 - 实时响应' : '常规请求 - 完整响应'}">
                                    ${request.is_streaming ? '🌊 流式请求' : '🔄 常规请求'}
                                </span>
                            </div>
                            <div class="detail-item">
                                <label>客户端IP:</label>
                                <span class="detail-value">${request.client_ip || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>用户代理:</label>
                                <span class="detail-value user-agent">${request.user_agent || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>重试次数:</label>
                                <span class="detail-value">${request.retry_count || 0}</span>
                            </div>
                        </div>
                    </div>

                    <div class="detail-section">
                        <div class="detail-section-title">🚀 服务信息</div>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <label>模型:</label>
                                <span class="detail-value model-name">${request.model_name || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>端点:</label>
                                <span class="detail-value">${request.endpoint_name || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>组:</label>
                                <span class="detail-value">${request.group_name || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>耗时:</label>
                                <span class="detail-value">${Utils.formatDuration(request.duration_ms)}</span>
                            </div>
                        </div>
                    </div>

                    <div class="detail-section">
                        <div class="detail-section-title">🪙 Token & 成本</div>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <label>输入Token:</label>
                                <span class="detail-value token-count">${request.input_tokens || 0}</span>
                            </div>
                            <div class="detail-item">
                                <label>输出Token:</label>
                                <span class="detail-value token-count">${request.output_tokens || 0}</span>
                            </div>
                            <div class="detail-item">
                                <label>缓存创建Token:</label>
                                <span class="detail-value token-count">${request.cache_creation_tokens || 0}</span>
                            </div>
                            <div class="detail-item">
                                <label>缓存读取Token:</label>
                                <span class="detail-value token-count">${request.cache_read_tokens || 0}</span>
                            </div>
                            <div class="detail-item">
                                <label>总成本:</label>
                                <span class="detail-value cost-value">${Utils.formatCost(request.total_cost_usd)}</span>
                            </div>
                        </div>
                    </div>

                    ${request.error_message ? `
                    <div class="detail-section error-section">
                        <div class="detail-section-title">❌ 错误信息</div>
                        <div class="error-message">${request.error_message}</div>
                    </div>
                    ` : ''}
                </div>
                <div class="modal-footer">
                    <button class="btn btn-primary" onclick="this.closest('.modal-overlay').remove()">关闭</button>
                </div>
            </div>
        `;
        
        document.body.appendChild(modal);
        
        // 点击背景关闭
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.remove();
            }
        });

        // ESC键关闭
        const handleEscape = (e) => {
            if (e.key === 'Escape') {
                modal.remove();
                document.removeEventListener('keydown', handleEscape);
            }
        };
        document.addEventListener('keydown', handleEscape);
    }
    
    // 加载统计概览数据
    async loadRequestsStats() {
        try {
            // 根据当前筛选条件构建查询参数
            const params = new URLSearchParams();
            
            // 根据筛选条件决定时间段
            if (this.state.filters.start_date && this.state.filters.end_date) {
                // 有自定义时间范围时，使用自定义时间
                params.append('start_date', this.state.filters.start_date);
                params.append('end_date', this.state.filters.end_date);
            } else {
                // 没有时间筛选时，获取所有历史数据
                params.append('period', '90d');
            }
            
            // 添加其他筛选参数（排除时间参数）
            Object.entries(this.state.filters).forEach(([key, value]) => {
                if (value && value.trim() !== '' && key !== 'start_date' && key !== 'end_date') {
                    params.append(key, value.trim());
                }
            });
            
            const response = await fetch(`/api/v1/usage/stats?${params}`);
            if (!response.ok) {
                throw new Error('获取统计数据失败');
            }
            
            const result = await response.json();
            if (result.success && result.data) {
                this.updateStatsDisplay(result.data);
            }
        } catch (error) {
            console.error('加载统计数据失败:', error);
            // 不显示错误提示，避免干扰用户
        }
    }
    
    // 更新统计显示面板
    updateStatsDisplay(stats) {
        // 总请求数
        const totalRequestsElement = document.getElementById('total-requests-count');
        if (totalRequestsElement) {
            totalRequestsElement.textContent = stats.total_requests || 0;
        }
        
        // 成功率
        const successRateElement = document.getElementById('success-rate');
        if (successRateElement) {
            const rate = stats.success_rate || 0;
            successRateElement.textContent = `${rate.toFixed(1)}%`;
        }
        
        // 平均响应时间
        const avgResponseTimeElement = document.getElementById('avg-response-time');
        if (avgResponseTimeElement) {
            const avgTime = stats.avg_duration_ms || 0;
            if (avgTime >= 1000) {
                avgResponseTimeElement.textContent = `${(avgTime / 1000).toFixed(1)}s`;
            } else {
                avgResponseTimeElement.textContent = `${Math.round(avgTime)}ms`;
            }
        }
        
        // 总成本 
        const totalCostElement = document.getElementById('total-cost');
        if (totalCostElement) {
            const cost = stats.total_cost_usd || 0;
            totalCostElement.textContent = Utils.formatCost(cost);
        }
        
        // 总Token数 (以百万为单位显示)
        const totalTokensElement = document.getElementById('total-tokens');
        if (totalTokensElement) {
            const tokens = stats.total_tokens || 0;
            totalTokensElement.textContent = Utils.formatTokens(tokens);
        }
        
        // 挂起请求数 (使用统计API返回的数据)
        const suspendedCountElement = document.getElementById('suspended-count');
        if (suspendedCountElement) {
            suspendedCountElement.textContent = stats.suspended_requests || 0;
        }
    }
    
    // 加载挂起请求数
    async loadSuspendedCount() {
        try {
            const response = await fetch('/api/v1/connections');
            if (response.ok) {
                const data = await response.json();
                const suspendedCountElement = document.getElementById('suspended-count');
                if (suspendedCountElement && data.suspended) {
                    suspendedCountElement.textContent = data.suspended.suspended_requests || 0;
                }
            }
        } catch (error) {
            console.error('加载挂起请求数失败:', error);
        }
    }
    
    // 获取当前状态（用于与主接口同步）
    getState() {
        return this.state;
    }
    
    // 设置状态（用于从主接口同步）
    setState(newState) {
        this.state = { ...this.state, ...newState };
    }
    
    // 重置状态
    resetState() {
        this.state = {
            currentPage: 1,
            pageSize: 50,
            totalRequests: 0,
            filters: {
                start_date: '',
                end_date: '',
                status: '',
                model: '',
                endpoint: '',
                group: ''
            },
            sortBy: 'start_time',
            sortOrder: 'desc'
        };
    }
};

// 分页导航全局函数
window.goToFirstPage = function() {
    if (window.webInterface && window.webInterface.requestsManager) {
        window.webInterface.requestsManager.state.currentPage = 1;
        window.webInterface.requestsManager.loadRequests();
    }
};

window.goToPrevPage = function() {
    if (window.webInterface && window.webInterface.requestsManager) {
        const currentPage = window.webInterface.requestsManager.state.currentPage;
        if (currentPage > 1) {
            window.webInterface.requestsManager.state.currentPage = currentPage - 1;
            window.webInterface.requestsManager.loadRequests();
        }
    }
};

window.goToNextPage = function() {
    if (window.webInterface && window.webInterface.requestsManager) {
        const manager = window.webInterface.requestsManager;
        const totalPages = Math.ceil(manager.state.totalRequests / manager.state.pageSize);
        if (manager.state.currentPage < totalPages) {
            manager.state.currentPage = manager.state.currentPage + 1;
            manager.loadRequests();
        }
    }
};

window.goToLastPage = function() {
    if (window.webInterface && window.webInterface.requestsManager) {
        const manager = window.webInterface.requestsManager;
        const totalPages = Math.ceil(manager.state.totalRequests / manager.state.pageSize);
        if (totalPages > 0) {
            manager.state.currentPage = totalPages;
            manager.loadRequests();
        }
    }
};

// 全局函数，供HTML模板调用
window.changePageSize = function() {
    const pageSizeSelect = document.getElementById('page-size-select');
    const newPageSize = parseInt(pageSizeSelect.value);
    
    if (window.webInterface && window.webInterface.requestsManager) {
        window.webInterface.requestsManager.state.pageSize = newPageSize;
        window.webInterface.requestsManager.state.currentPage = 1; // 重置到第一页
        window.webInterface.requestsManager.loadRequests();
    }
};

window.goToPage = function() {
    const currentPageInput = document.getElementById('current-page-input');
    const newPage = parseInt(currentPageInput.value);
    
    if (window.webInterface && window.webInterface.requestsManager && newPage > 0) {
        const totalPages = Math.ceil(window.webInterface.requestsManager.state.totalRequests / window.webInterface.requestsManager.state.pageSize);
        const validPage = Math.max(1, Math.min(newPage, totalPages));
        
        window.webInterface.requestsManager.state.currentPage = validPage;
        currentPageInput.value = validPage; // 确保输入框显示有效页码
        window.webInterface.requestsManager.loadRequests();
    }
};

console.log('✅ RequestsManager模块已加载');
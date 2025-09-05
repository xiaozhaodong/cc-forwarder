// Claude Request Forwarder - 请求追踪管理模块
// 处理请求列表、筛选、分页、导出和详情显示

window.RequestsManager = class {
    constructor(webInterface) {
        this.webInterface = webInterface;
        
        // 请求追踪页面状态
        this.state = {
            currentPage: 1,
            pageSize: 20,
            totalRequests: 0,
            filters: {
                start_date: '',
                end_date: '',
                status: '',
                model: '',
                endpoint: '',
                group: ''
            },
            sortBy: 'created_at',
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
                tbody.innerHTML = '<tr><td colspan="13" class="no-data">📄 暂无请求数据</td></tr>';
                this.updateRequestsCountInfo(0, this.state.currentPage);
            }
        } catch (error) {
            console.error('加载请求数据失败:', error);
            Utils.showError('请求数据加载失败: ' + error.message);
        }
    }
    
    // 生成请求表格行内容（只生成tbody内的tr元素）
    generateRequestsRows(requests) {
        if (!requests || requests.length === 0) {
            return '<tr><td colspan="13" class="no-data">📄 暂无请求数据</td></tr>';
        }

        let html = '';
        requests.forEach(request => {
            const status = Utils.formatRequestStatus(request.status);
            const duration = Utils.formatDuration(request.duration_ms);
            const cost = Utils.formatCost(request.total_cost_usd);
            const createdAt = new Date(request.created_at).toLocaleString('zh-CN');
            
            html += `
                <tr>
                    <td>
                        <code class="request-id">${request.request_id}</code>
                    </td>
                    <td class="datetime">${createdAt}</td>
                    <td>
                        <span class="status-badge status-${request.status}">${status}</span>
                    </td>
                    <td class="model-name">${request.model_name || '-'}</td>
                    <td class="endpoint-name">${request.endpoint_name || '-'}</td>
                    <td class="group-name">${request.group_name || '-'}</td>
                    <td class="duration">${duration}</td>
                    <td class="input-tokens">${request.input_tokens || 0}</td>
                    <td class="output-tokens">${request.output_tokens || 0}</td>
                    <td class="cache-creation-tokens">${request.cache_creation_tokens || '-'}</td>
                    <td class="cache-read-tokens">${request.cache_read_tokens || '-'}</td>
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
                                    <option value="success" ${this.state.filters.status === 'success' ? 'selected' : ''}>成功</option>
                                    <option value="error" ${this.state.filters.status === 'error' ? 'selected' : ''}>失败</option>
                                    <option value="timeout" ${this.state.filters.status === 'timeout' ? 'selected' : ''}>超时</option>
                                </select>
                            </div>
                        </div>
                        <div class="filter-row">
                            <div class="filter-group">
                                <label for="model-filter">模型:</label>
                                <input type="text" id="model-filter" name="model" value="${this.state.filters.model}" placeholder="输入模型名称">
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
                        <th class="sortable" data-sort="created_at">
                            创建时间
                            ${Utils.getSortIcon('created_at', this.state.sortBy, this.state.sortOrder)}
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
                        <th class="sortable" data-sort="total_cost">
                            成本
                            ${Utils.getSortIcon('total_cost', this.state.sortBy, this.state.sortOrder)}
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
                ${Utils.generatePagination(total, currentPage, this.state.pageSize)}
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
    }
    
    // 绑定请求相关事件
    bindRequestsEvents() {
        // 筛选表单事件
        const filterForm = document.getElementById('requests-filter-form');
        if (filterForm) {
            filterForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.handleRequestsFilter();
            });
        }
        
        // 排序事件
        document.querySelectorAll('.sortable').forEach(th => {
            th.addEventListener('click', () => {
                const sortBy = th.dataset.sort;
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
                                <label>创建时间:</label>
                                <span class="detail-value">${new Date(request.created_at).toLocaleString('zh-CN')}</span>
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
                                <label>客户端IP:</label>
                                <span class="detail-value">${request.client_ip || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>用户代理:</label>
                                <span class="detail-value user-agent">${request.user_agent || '-'}</span>
                            </div>
                            <div class="detail-item">
                                <label>HTTP状态码:</label>
                                <span class="detail-value">${request.http_status_code || '-'}</span>
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
    
    // 导出请求数据
    async exportRequests(format) {
        try {
            // 构建查询参数
            const params = new URLSearchParams({
                format: format
            });
            
            // 添加筛选参数
            Object.entries(this.state.filters).forEach(([key, value]) => {
                if (value && value.trim() !== '') {
                    params.append(key, value.trim());
                }
            });
            
            const response = await fetch(`/api/v1/usage/export?${params}`);
            if (!response.ok) {
                throw new Error('导出数据失败');
            }
            
            // 获取文件名
            const contentDisposition = response.headers.get('Content-Disposition');
            let filename = `requests_export.${format}`;
            if (contentDisposition) {
                const filenameMatch = contentDisposition.match(/filename="?(.+)"?/);
                if (filenameMatch) {
                    filename = filenameMatch[1];
                }
            }
            
            // 下载文件
            const blob = await response.blob();
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
            
            Utils.showSuccess(`数据已导出为 ${format.toUpperCase()} 格式`);
        } catch (error) {
            console.error('导出数据失败:', error);
            Utils.showError('导出数据失败: ' + error.message);
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
            pageSize: 20,
            totalRequests: 0,
            filters: {
                start_date: '',
                end_date: '',
                status: '',
                model: '',
                endpoint: '',
                group: ''
            },
            sortBy: 'created_at',
            sortOrder: 'desc'
        };
    }
};

console.log('✅ RequestsManager模块已加载');
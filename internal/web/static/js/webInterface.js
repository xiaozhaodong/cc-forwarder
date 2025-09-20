// Claude Request Forwarder - 核心Web界面模块
// 主要的WebInterface类和标签页管理逻辑

window.WebInterface = class {
    constructor() {
        this.refreshInterval = null;
        this.currentTab = 'overview';
        
        // 数据缓存，用于存储各个标签页的最新数据
        this.cachedData = {
            status: null,
            endpoints: null,
            groups: null,
            connections: null,
            requests: null,
            config: null
        };
        
        // 初始化管理器
        this.sseManager = new SSEManager(this);
        this.requestsManager = new RequestsManager(this);
        // this.groupsManager = new GroupsManager(this); // 组管理已迁移到React，禁用传统组管理器
        // this.endpointsManager = new EndpointsManager(this); // 已迁移到React，禁用传统端点管理器
        
        this.init();
    }

    init() {
        this.bindEvents();
        this.showTab('overview');
        // 立即加载初始数据，不等待SSE连接
        this.loadAllTabsData();
        Utils.createConnectionIndicator();
        
        // 初始化折叠区域状态
        this.initializeCollapsibleSections();
        
        // SSE连接放在最后建立
        this.sseManager.init();
    }

    bindEvents() {
        // 标签页切换事件
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                const tabName = e.target.getAttribute('onclick').match(/'([^']+)'/)[1];
                this.showTab(tabName);
            });
        });
    }

    showTab(tabName) {
        const previousTab = this.currentTab;
        // 隐藏所有标签页内容
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });

        // 移除所有标签页的活动状态
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.classList.remove('active');
        });

        // 显示选中的标签页
        const selectedTab = document.getElementById(tabName);
        if (selectedTab) {
            selectedTab.classList.add('active');
        }

        // 激活对应的标签按钮
        document.querySelectorAll('.nav-tab').forEach(tab => {
            const tabTarget = tab.getAttribute('onclick')?.match(/'([^']+)'/)?.[1];
            if (tabTarget === tabName) {
                tab.classList.add('active');
            }
        });
        // 如果从React组管理页面离开，先卸载React根，避免外部DOM变更导致警告
        if (previousTab === 'groups' && tabName !== 'groups') {
            this.cleanupReactGroupsPage();
        }

        this.currentTab = tabName;

        // 优先使用缓存数据，如果没有缓存则请求API
        this.loadTabDataFromCache(tabName);
    }

    cleanupReactGroupsPage() {
        const groupsContainer = document.getElementById('react-groups-container');
        if (!groupsContainer || !window.ReactComponents) {
            return;
        }

        // 卸载React组件，避免外部DOM操作触发React警告
        window.ReactComponents.unmountComponent(groupsContainer);

        // 恢复占位内容，保持与初始模板一致
        groupsContainer.innerHTML = `
            <div style="text-align: center; padding: 48px 24px; color: #6b7280;">
                <div style="font-size: 24px; margin-bottom: 8px;">⏳</div>
                <p>React组管理页面加载中...</p>
            </div>
        `;
    }
    
    // 加载React组管理页面
    async loadReactGroupsPage() {
        try {
            console.log('🔄 [React组件] 开始加载React组管理页面...');

            // 确保React模块加载器已初始化
            if (!window.ReactModuleLoader || !window.ReactModuleLoader.initialized) {
                await window.ReactModuleLoader.initialize();
            }

            // 加载React组管理页面组件
            const GroupsPageModule = await window.importReactModule('pages/groups/index.jsx');
            const GroupsPage = GroupsPageModule.default || GroupsPageModule;

            if (!GroupsPage) {
                throw new Error('无法获取GroupsPage组件');
            }

            // 获取React组管理页面容器DOM元素
            const groupsContainer = document.getElementById('react-groups-container');
            if (!groupsContainer) {
                throw new Error('找不到react-groups-container容器元素');
            }

            // 使用React组件注册系统的渲染方法（不需要手动卸载，React会自动管理）
            const groupsPageElement = React.createElement(GroupsPage);
            window.ReactComponents.renderComponent(groupsPageElement, groupsContainer);

            console.log('✅ [React组件] React组管理页面加载成功');

        } catch (error) {
            console.error('❌ [React组件] React组管理页面加载失败:', error);

            // 显示错误信息而不是回退到传统管理器
            const groupsContainer = document.getElementById('react-groups-container');
            if (groupsContainer) {
                groupsContainer.innerHTML = `
                    <div style="text-align: center; padding: 48px 24px; color: #ef4444;">
                        <div style="font-size: 48px; margin-bottom: 16px;">❌</div>
                        <h3 style="margin: 0 0 8px 0;">React组件加载失败</h3>
                        <p style="margin: 0; font-size: 14px; color: #6b7280;">${error.message}</p>
                        <button onclick="window.webInterface.loadReactGroupsPage()"
                                style="margin-top: 16px; padding: 8px 16px; border: none; border-radius: 6px; background: #3b82f6; color: white; cursor: pointer;">
                            🔄 重试加载
                        </button>
                    </div>
                `;
            }
        }
    }

    loadTabDataFromCache(tabName) {
        console.log('[Cache] 尝试从缓存加载标签页数据:', tabName);
        
        switch (tabName) {
            case 'overview':
                // 概览页面需要综合数据，总是重新加载
                this.loadOverview();
                break;
            case 'endpoints':
                // 端点页面已迁移到React，不再使用传统的端点管理器
                console.log('端点页面已迁移到React，跳过传统端点管理器初始化');
                break;
            case 'groups':
                // 组页面已迁移到React，使用React组件渲染
                this.loadReactGroupsPage();
                break;
            case 'requests':
                if (this.cachedData.requests) {
                    console.log('[Cache] 使用缓存数据显示requests');
                    const tbody = document.getElementById('requests-table-body');
                    if (tbody && this.cachedData.requests.data) {
                        tbody.innerHTML = this.requestsManager.generateRequestsRows(this.cachedData.requests.data);
                        this.requestsManager.updateRequestsCountInfo(this.cachedData.requests.total, this.requestsManager.state.currentPage);
                        this.requestsManager.bindRequestsEvents();
                    }
                } else {
                    console.log('[Cache] 无缓存数据，请求requests API');
                    this.requestsManager.loadRequests();
                }
                
                // 初始化下拉框
                const self = this;
                setTimeout(function() {
                    if (typeof self.initializeRequestsFilters === 'function') {
                        self.initializeRequestsFilters();
                    }
                }, 100);
                break;
            case 'charts':
                // 图表页面依赖chart.js，使用SSE数据进行实时更新
                if (window.chartManager) {
                    // 图表管理器存在，确保使用SSE更新模式
                    if (!window.chartManager.sseEnabled) {
                        window.chartManager.enableSSEUpdates();
                    }
                    // 触发图表数据刷新
                    window.chartManager.refreshAllCharts();
                }
                break;
            case 'config':
                // 配置页面已迁移到React，跳过传统配置数据加载
                console.log('配置页面已迁移到React，跳过传统配置数据加载');
                break;
            default:
                // 后备方案，使用原有逻辑
                this.loadTabData(tabName);
        }
    }

    loadAllTabsData() {
        // 并行加载所有标签页数据，加快初始显示速度
        Promise.all([
            this.loadOverview(),
            // this.endpointsManager.loadEndpoints(), // 端点页面已迁移到React
            // this.groupsManager.loadGroups(), // 组页面已迁移到React
            this.requestsManager.loadRequests()
            // this.loadConfig() // 配置页面已迁移到React
        ]).catch(error => {
            console.error('加载初始数据失败:', error);
        });
    }

    loadTabData(tabName) {
        switch (tabName) {
            case 'overview':
                this.loadOverview();
                break;
            case 'endpoints':
                // 端点页面已迁移到React，不再使用传统的端点管理器
                console.log('端点页面已迁移到React，跳过传统端点数据加载');
                break;
            case 'groups':
                // 组页面已迁移到React，使用React组件渲染
                this.loadReactGroupsPage();
                break;
            case 'requests':
                this.requestsManager.loadRequests();
                break;
            case 'config':
                // 配置页面已迁移到React，跳过传统配置数据加载
                console.log('配置页面已迁移到React，跳过传统配置数据加载');
                break;
        }
    }

    async loadOverview() {
        try {
            const [statusResponse, endpointsResponse, connectionsResponse, groupsResponse] = await Promise.all([
                fetch('/api/v1/status'),
                fetch('/api/v1/endpoints'),
                fetch('/api/v1/connections'),
                fetch('/api/v1/groups')
            ]);

            const status = await statusResponse.json();
            const endpoints = await endpointsResponse.json();
            const connections = await connectionsResponse.json();
            const groups = await groupsResponse.json();

            // 更新概览卡片
            Utils.updateElementText('server-status', 
                status.status === 'running' ? '🟢 运行中' : '🔴 已停止');
            Utils.updateElementText('uptime', status.uptime);
            Utils.updateElementText('endpoint-count', endpoints.total);
            Utils.updateElementText('total-requests', connections.total_requests);

            // 更新挂起请求信息
            const suspendedData = connections.suspended || {};
            const suspendedElement = document.getElementById('suspended-requests');
            const suspendedRateElement = document.getElementById('suspended-success-rate');
            
            if (suspendedElement) {
                suspendedElement.textContent = `${suspendedData.suspended_requests || 0} / ${suspendedData.total_suspended_requests || 0}`;
            }
            
            if (suspendedRateElement) {
                const rate = suspendedData.success_rate || 0;
                suspendedRateElement.textContent = `成功率: ${rate.toFixed(1)}%`;
                suspendedRateElement.className = rate > 80 ? 'text-muted' : 'text-warning';
            }

            // 更新当前活动组信息
            const activeGroupElement = document.getElementById('active-group');
            const groupSuspendedInfoElement = document.getElementById('group-suspended-info');
            
            if (activeGroupElement) {
                // 从groups数组中找到is_active为true的组
                const activeGroup = groups.groups ? groups.groups.find(group => group.is_active) : null;
                if (activeGroup) {
                    activeGroupElement.textContent = `${activeGroup.name} (${activeGroup.healthy_endpoints}/${activeGroup.total_endpoints} 健康)`;
                } else {
                    activeGroupElement.textContent = '无活跃组';
                }
            }
            
            if (groupSuspendedInfoElement && groups.total_suspended_requests > 0) {
                groupSuspendedInfoElement.textContent = `${groups.total_suspended_requests} 个挂起请求`;
                groupSuspendedInfoElement.style.display = 'block';
            } else if (groupSuspendedInfoElement) {
                groupSuspendedInfoElement.style.display = 'none';
            }

            // 更新连接详情区域
            this.updateConnectionDetails(connections);
            
            // 更新挂起请求监控区域
            this.updateSuspendedMonitoring(connections.suspended || {}, connections.suspended_connections || []);
            
            // 智能展开逻辑：如果有挂起请求，自动展开挂起监控区域
            if (suspendedData.suspended_requests > 0) {
                this.expandSection('suspended-monitoring');
            }

        } catch (error) {
            console.error('加载概览数据失败:', error);
            Utils.showError('概览数据加载失败');
        }
    }

    // 更新连接详情区域
    updateConnectionDetails(connectionsData) {
        const container = document.getElementById('connections-stats');
        if (container) {
            container.innerHTML = Utils.generateConnectionsStats(connectionsData);
        }
    }

    // 更新挂起请求监控区域
    updateSuspendedMonitoring(suspendedData, suspendedConnections) {
        // 更新挂起请求统计
        this.updateSuspendedStats(suspendedData);
        
        // 更新挂起连接列表  
        this.updateSuspendedConnections(suspendedConnections);
    }

    // 折叠/展开区域控制方法
    toggleSection(sectionId) {
        const content = document.getElementById(sectionId + '-content');
        const indicator = document.getElementById(sectionId + '-indicator');
        
        if (!content || !indicator) return;
        
        const isCollapsed = content.classList.contains('collapsed');
        
        if (isCollapsed) {
            // 展开
            content.classList.remove('collapsed');
            content.classList.add('expanded');
            indicator.textContent = '▲';
            indicator.style.transform = 'rotate(180deg)';
            
            // 保存状态到localStorage
            localStorage.setItem(`section-${sectionId}`, 'expanded');
        } else {
            // 折叠
            content.classList.remove('expanded');
            content.classList.add('collapsed');
            indicator.textContent = '▼';
            indicator.style.transform = 'rotate(0deg)';
            
            // 保存状态到localStorage
            localStorage.setItem(`section-${sectionId}`, 'collapsed');
        }
    }

    // 智能展开区域（用于异常情况）
    expandSection(sectionId) {
        const content = document.getElementById(sectionId + '-content');
        const indicator = document.getElementById(sectionId + '-indicator');
        const header = document.getElementById(sectionId + '-section')?.querySelector('.section-header');
        
        if (!content || !indicator) return;
        
        // 展开内容
        content.classList.remove('collapsed');
        content.classList.add('expanded');
        indicator.textContent = '▲';
        indicator.style.transform = 'rotate(180deg)';
        
        // 添加警告样式
        if (header) {
            header.classList.add('has-alerts');
        }
        
        // 保存状态
        localStorage.setItem(`section-${sectionId}`, 'expanded');
    }

    // 初始化折叠区域状态
    initializeCollapsibleSections() {
        const sections = ['connection-details', 'suspended-monitoring'];
        
        sections.forEach(sectionId => {
            const savedState = localStorage.getItem(`section-${sectionId}`);
            const content = document.getElementById(sectionId + '-content');
            const indicator = document.getElementById(sectionId + '-indicator');
            
            if (content && indicator) {
                if (savedState === 'expanded') {
                    content.classList.remove('collapsed');
                    content.classList.add('expanded');
                    indicator.textContent = '▲';
                    indicator.style.transform = 'rotate(180deg)';
                } else {
                    // 默认折叠
                    content.classList.add('collapsed');
                    content.classList.remove('expanded');
                    indicator.textContent = '▼';
                    indicator.style.transform = 'rotate(0deg)';
                }
            }
        });
    }

    async loadConfig() {
        try {
            const response = await fetch('/api/v1/config');
            const data = await response.json();

            const container = document.getElementById('config-display');
            container.innerHTML = Utils.generateConfigDisplay(data);
        } catch (error) {
            console.error('加载配置数据失败:', error);
            Utils.showError('配置数据加载失败');
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

    // 分页控制（委托给requestsManager）
    changePage(page) {
        this.requestsManager.changePage(page);
    }

    startAutoRefresh() {
        // 注意：移除定时刷新机制，改为纯事件驱动架构
        // SSE连接建立后完全依赖事件推送，不需要定时刷新
        if (this.sseManager.isConnected()) {
            this.stopAutoRefresh();
            console.log('✅ SSE已连接，使用事件驱动模式，无需定时刷新');
            return;
        }
        
        // 仅在SSE连接失败时提供回退机制（保留框架但不启动定时器）
        console.log('⚠️ SSE未连接，但定时刷新已禁用，完全依赖事件驱动');
        // 原先的30秒定时刷新机制已移除：
        // this.refreshInterval = setInterval(() => {
        //     if (!this.sseManager.isConnected()) {
        //         this.loadTabData(this.currentTab);
        //     }
        // }, 30000);
    }

    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    // 便捷方法（委托给Utils）
    showSuccess(message) {
        Utils.showSuccess(message);
    }

    showError(message) {
        Utils.showError(message);
    }
    
    showInfo(message) {
        Utils.showInfo(message);
    }

    // 清理资源
    destroy() {
        if (this.sseManager) {
            this.sseManager.destroy();
        }
        this.stopAutoRefresh();
    }

    // 初始化请求页面的筛选下拉框
    async initializeRequestsFilters() {
        try {
            // 检查DOM元素是否存在
            const endpointFilter = document.getElementById('endpoint-filter');
            const groupFilter = document.getElementById('group-filter');
            
            if (!endpointFilter || !groupFilter) {
                setTimeout(() => this.initializeRequestsFilters(), 1000);
                return;
            }
            
            // 并行加载端点和组数据
            const [endpointsResponse, groupsResponse] = await Promise.all([
                fetch('/api/v1/endpoints'),
                fetch('/api/v1/groups')
            ]);
            
            if (!endpointsResponse.ok || !groupsResponse.ok) {
                throw new Error('API请求失败');
            }
            
            const endpointsData = await endpointsResponse.json();
            const groupsData = await groupsResponse.json();
            
            // 填充端点下拉框
            if (endpointsData.endpoints && Array.isArray(endpointsData.endpoints)) {
                endpointFilter.innerHTML = '<option value="all">全部端点</option>';
                endpointsData.endpoints.forEach(endpoint => {
                    const option = document.createElement('option');
                    option.value = endpoint.name;
                    option.textContent = endpoint.name;
                    endpointFilter.appendChild(option);
                });
            }
            
            // 填充组下拉框
            if (groupsData.groups && Array.isArray(groupsData.groups)) {
                groupFilter.innerHTML = '<option value="all">全部组</option>';
                groupsData.groups.forEach(group => {
                    const option = document.createElement('option');
                    option.value = group.name;
                    option.textContent = group.name;
                    groupFilter.appendChild(option);
                });
            }
            
        } catch (error) {
            console.error('初始化筛选下拉框失败:', error);
            // 重试一次
            setTimeout(() => {
                this.initializeRequestsFilters();
            }, 2000);
        }
    }
};

// === 全局函数 ===

// 全局函数用于HTML中的onclick事件
function showTab(tabName) {
    // 添加安全检查和更好的错误处理
    try {
        if (window.webInterface && window.webInterface.showTab) {
            window.webInterface.showTab(tabName);
        } else {
            console.warn('WebInterface not ready yet, retrying in 100ms...');
            // 如果webInterface还没准备好，延迟重试
            setTimeout(() => showTab(tabName), 100);
        }
    } catch (error) {
        console.error('Error in showTab:', error);
    }
}

// 隐藏挂起请求警告
function hideSuspendedAlert() {
    const alertBanner = document.getElementById('group-suspended-alert');
    if (alertBanner) {
        alertBanner.style.display = 'none';
    }
}

// 页面卸载时清理资源
window.addEventListener('beforeunload', () => {
    if (window.webInterface) {
        window.webInterface.destroy();
    }
});

// 立即定义全局showTab函数，防止未定义错误
window.showTab = function(tabName) {
    console.log('📋 切换到标签页:', tabName);
    
    // 如果WebInterface已准备好，使用它
    if (window.webInterface && typeof window.webInterface.showTab === 'function') {
        window.webInterface.showTab(tabName);
        return;
    }
    
    // 否则提供基本的标签页切换功能
    try {
        // 隐藏所有标签页内容
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });
        
        // 移除所有导航标签的活跃状态
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        
        // 显示目标标签页内容
        const targetContent = document.getElementById(tabName + '-content') || 
                             document.querySelector(`[data-tab="${tabName}"]`);
        if (targetContent) {
            targetContent.classList.add('active');
        }
        
        // 激活对应的导航标签
        const targetTab = document.querySelector(`[onclick*="${tabName}"]`);
        if (targetTab) {
            targetTab.classList.add('active');
        }
        
        console.log('✅ 基本标签切换完成:', tabName);
        
        // 等待WebInterface准备好后再尝试完整功能
        let retryCount = 0;
        const maxRetries = 50; // 5秒内重试
        const tryAgain = () => {
            if (window.webInterface && typeof window.webInterface.showTab === 'function') {
                console.log('🔄 WebInterface准备就绪，切换到完整功能');
                window.webInterface.showTab(tabName);
            } else if (retryCount < maxRetries) {
                retryCount++;
                setTimeout(tryAgain, 100);
            } else {
                console.warn('⚠️ WebInterface初始化超时，使用基本功能');
            }
        };
        setTimeout(tryAgain, 100);
        
    } catch (error) {
        console.error('❌ 标签切换错误:', error);
    }
};

// 确保函数在页面加载前就可用
console.log('✅ 全局showTab函数已定义');

// 初始化Web界面
document.addEventListener('DOMContentLoaded', () => {
    console.log('🔄 DOM内容已加载，开始初始化WebInterface...');
    try {
        window.webInterface = new WebInterface();
        console.log('✅ WebInterface初始化成功');
        
        // 验证showTab函数是否可用
        if (typeof window.webInterface.showTab === 'function') {
            console.log('✅ showTab方法可用');
        } else {
            console.error('❌ showTab方法不可用');
        }
    } catch (error) {
        console.error('❌ WebInterface初始化失败:', error);
    }
});

// 全局筛选函数 - 用于HTML按钮调用
window.applyFilters = function() {
    if (!window.webInterface || !window.webInterface.requestsManager) {
        console.error('WebInterface或RequestsManager未初始化');
        return;
    }
    
    // 获取筛选条件
    const timeRange = document.getElementById('time-range-filter')?.value;
    const status = document.getElementById('status-filter')?.value;
    const model = document.getElementById('model-filter')?.value;
    const endpoint = document.getElementById('endpoint-filter')?.value;
    const group = document.getElementById('group-filter')?.value;
    
    // 处理时间范围
    let startDate = '', endDate = '';
    if (timeRange === 'custom') {
        startDate = document.getElementById('start-date')?.value || '';
        endDate = document.getElementById('end-date')?.value || '';
    } else if (timeRange && timeRange !== 'all' && timeRange !== '') {
        const now = new Date();
        // 使用本地时间而不是UTC时间
        const formatLocalDateTime = (date) => {
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            const hours = String(date.getHours()).padStart(2, '0');
            const minutes = String(date.getMinutes()).padStart(2, '0');
            const seconds = String(date.getSeconds()).padStart(2, '0');
            return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}+08:00`;
        };
        
        endDate = formatLocalDateTime(now);
        
        switch(timeRange) {
            case '1h':
                startDate = formatLocalDateTime(new Date(now.getTime() - 1 * 60 * 60 * 1000));
                break;
            case '6h':
                startDate = formatLocalDateTime(new Date(now.getTime() - 6 * 60 * 60 * 1000));
                break;
            case '24h':
                startDate = formatLocalDateTime(new Date(now.getTime() - 24 * 60 * 60 * 1000));
                break;
            case '7d':
                startDate = formatLocalDateTime(new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000));
                break;
            case '30d':
                startDate = formatLocalDateTime(new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000));
                break;
        }
    }
    
    // 更新RequestsManager的筛选条件
    window.webInterface.requestsManager.state.filters = {
        start_date: startDate || '',
        end_date: endDate || '', 
        status: status === 'all' ? '' : status,
        model: model === 'all' ? '' : model || '',
        endpoint: endpoint === 'all' ? '' : endpoint || '',
        group: group === 'all' ? '' : group || ''
    };
    
    // 重置到第一页
    window.webInterface.requestsManager.state.currentPage = 1;
    
    // 加载数据和统计信息
    window.webInterface.requestsManager.loadRequests();
};

// 重置筛选条件
window.resetFilters = function() {
    if (!window.webInterface || !window.webInterface.requestsManager) {
        console.error('WebInterface或RequestsManager未初始化');
        return;
    }
    
    // 重置表单元素
    const timeRangeFilter = document.getElementById('time-range-filter');
    const statusFilter = document.getElementById('status-filter');
    const modelFilter = document.getElementById('model-filter');
    const endpointFilter = document.getElementById('endpoint-filter');
    const groupFilter = document.getElementById('group-filter');
    const startDate = document.getElementById('start-date');
    const endDate = document.getElementById('end-date');
    
    if (timeRangeFilter) timeRangeFilter.value = '';
    if (statusFilter) statusFilter.value = 'all';
    if (modelFilter) modelFilter.value = 'all';
    if (endpointFilter) endpointFilter.value = 'all';
    if (groupFilter) groupFilter.value = 'all';
    if (startDate) startDate.value = '';
    if (endDate) endDate.value = '';
    
    // 隐藏自定义时间范围
    const customDateRange = document.getElementById('custom-date-range');
    if (customDateRange) {
        customDateRange.style.display = 'none';
    }
    
    // 重置RequestsManager的筛选条件
    window.webInterface.requestsManager.resetFilters();
};

// 加载并填充端点下拉框
window.loadEndpointOptions = async function() {
    try {
        console.log('🔄 开始加载端点选项...');
        const response = await fetch('/api/v1/endpoints');
        if (!response.ok) {
            throw new Error(`获取端点列表失败: ${response.status} ${response.statusText}`);
        }
        
        const data = await response.json();
        console.log('📡 端点API数据:', data);
        
        const endpointFilter = document.getElementById('endpoint-filter');
        console.log('🎯 端点过滤器元素:', endpointFilter);
        
        if (!endpointFilter) {
            console.error('❌ 找不到endpoint-filter元素');
            return;
        }
        
        if (!data.endpoints || !Array.isArray(data.endpoints)) {
            console.error('❌ 端点数据格式错误:', data);
            return;
        }
        
        // 清除现有选项（保留"全部端点"）
        endpointFilter.innerHTML = '<option value="all">全部端点</option>';
        
        // 添加端点选项
        data.endpoints.forEach(endpoint => {
            const option = document.createElement('option');
            option.value = endpoint.name;
            option.textContent = endpoint.name;
            endpointFilter.appendChild(option);
            console.log(`✅ 添加端点选项: ${endpoint.name}`);
        });
        
        console.log(`✅ 成功加载${data.endpoints.length}个端点选项`);
    } catch (error) {
        console.error('❌ 加载端点选项失败:', error);
    }
};

// 加载并填充组下拉框
window.loadGroupOptions = async function() {
    try {
        console.log('🔄 开始加载组选项...');
        const response = await fetch('/api/v1/groups');
        if (!response.ok) {
            throw new Error(`获取组列表失败: ${response.status} ${response.statusText}`);
        }
        
        const data = await response.json();
        console.log('📡 组API数据:', data);
        
        const groupFilter = document.getElementById('group-filter');
        console.log('🎯 组过滤器元素:', groupFilter);
        
        if (!groupFilter) {
            console.error('❌ 找不到group-filter元素');
            return;
        }
        
        if (!data.groups || !Array.isArray(data.groups)) {
            console.error('❌ 组数据格式错误:', data);
            return;
        }
        
        // 清除现有选项（保留"全部组"）
        groupFilter.innerHTML = '<option value="all">全部组</option>';
        
        // 添加组选项
        data.groups.forEach(group => {
            const option = document.createElement('option');
            option.value = group.name;
            option.textContent = group.name;
            groupFilter.appendChild(option);
            console.log(`✅ 添加组选项: ${group.name}`);
        });
        
        console.log(`✅ 成功加载${data.groups.length}个组选项`);
    } catch (error) {
        console.error('❌ 加载组选项失败:', error);
    }
};

// 初始化筛选下拉框
window.initializeFilterOptions = function() {
    console.log('🔄 初始化筛选下拉框...');
    
    // 检查元素是否存在
    const endpointFilter = document.getElementById('endpoint-filter');
    const groupFilter = document.getElementById('group-filter');
    
    console.log('📋 元素检查:', {
        endpointFilter: !!endpointFilter,
        groupFilter: !!groupFilter
    });
    
    if (!endpointFilter || !groupFilter) {
        console.warn('⚠️ 下拉框元素未找到，延迟重试...');
        // 延迟重试
        setTimeout(() => {
            console.log('🔄 重试初始化筛选下拉框...');
            window.initializeFilterOptions();
        }, 1000);
        return;
    }
    
    // 延迟加载，确保页面已经完全渲染
    setTimeout(async () => {
        console.log('📡 开始并行加载端点和组选项...');
        try {
            await Promise.all([
                window.loadEndpointOptions(),
                window.loadGroupOptions()
            ]);
            console.log('✅ 筛选选项初始化完成');
        } catch (error) {
            console.error('❌ 筛选选项初始化失败:', error);
        }
    }, 500);
};

// 时间范围切换处理
document.addEventListener('DOMContentLoaded', function() {
    const timeRangeFilter = document.getElementById('time-range-filter');
    const customDateRange = document.getElementById('custom-date-range');
    
    if (timeRangeFilter && customDateRange) {
        timeRangeFilter.addEventListener('change', function() {
            if (this.value === 'custom') {
                customDateRange.style.display = 'block';
            } else {
                customDateRange.style.display = 'none';
            }
        });
    }
});

console.log('✅ WebInterface模块已加载');

// Claude Request Forwarder - 工具函数模块
// 通用工具函数，包括格式化、通知、DOM操作等

window.Utils = {
    
    // 格式化请求状态
    formatRequestStatus(status) {
        const statusMap = {
            'forwarding': '转发中',
            'processing': '解析中',  // 🆕 HTTP响应成功，Token解析中
            'completed': '完成',     // 🆕 Token解析和计算完成
            'success': '成功',       // 兼容旧数据
            'suspended': '挂起中',   // 🆕 请求挂起等待组切换
            'error': '失败',
            'timeout': '超时',
            'cancelled': '取消'
        };
        return statusMap[status] || status;
    },
    
    // 格式化持续时间
    formatDuration(ms) {
        if (!ms || ms === 0) return '-';
        if (ms < 1000) return `${ms}ms`;
        if (ms < 60000) return `${(ms / 1000).toFixed(2)}s`;
        const minutes = Math.floor(ms / 60000);
        const seconds = ((ms % 60000) / 1000).toFixed(0);
        return `${minutes}m ${seconds}s`;
    },
    
    // 格式化成本
    formatCost(cost) {
        const numericCost = Number(cost) || 0;
        
        // 始终显示至少2位小数
        if (numericCost === 0) return '$0.00';
        
        // 根据成本大小选择合适的精度
        if (numericCost >= 1) {
            return `$${numericCost.toFixed(2)}`;
        } else if (numericCost >= 0.01) {
            return `$${numericCost.toFixed(4)}`;
        } else if (numericCost >= 0.001) {
            return `$${numericCost.toFixed(5)}`;
        } else {
            return `$${numericCost.toFixed(6)}`;
        }
    },
    
    // 格式化Token数量 (以百万为单位显示)
    formatTokens(tokens) {
        const numericTokens = Number(tokens) || 0;
        if (numericTokens === 0) return '0.00M';
        
        // 转换为百万单位
        const tokensInM = numericTokens / 1000000;
        
        if (tokensInM >= 1) {
            return `${tokensInM.toFixed(2)}M`;
        } else {
            return `${tokensInM.toFixed(3)}M`;
        }
    },
    
    // 生成排序图标
    getSortIcon(field, sortBy, sortOrder) {
        if (sortBy !== field) {
            return '<span class="sort-icon">⇅</span>';
        }
        return sortOrder === 'asc' ? 
            '<span class="sort-icon">↑</span>' : 
            '<span class="sort-icon">↓</span>';
    },
    
    // 生成分页控件HTML
    generatePagination(total, currentPage, pageSize, managerName = 'requestsManager') {
        const totalPages = Math.ceil(total / pageSize);
        if (totalPages <= 1) return '';
        
        let html = '<div class="pagination-controls">';
        html += `<div class="pagination-info">第 ${currentPage} 页，共 ${totalPages} 页，总计 ${total} 条记录</div>`;
        
        // 上一页
        if (currentPage > 1) {
            html += `<button class="btn page-btn" onclick="webInterface.${managerName}.changePage(${currentPage - 1})">‹ 上一页</button>`;
        }
        
        // 页码
        const startPage = Math.max(1, currentPage - 2);
        const endPage = Math.min(totalPages, currentPage + 2);
        
        if (startPage > 1) {
            html += `<button class="btn page-btn" onclick="webInterface.${managerName}.changePage(1)">1</button>`;
            if (startPage > 2) {
                html += '<span class="pagination-ellipsis">...</span>';
            }
        }
        
        for (let i = startPage; i <= endPage; i++) {
            const activeClass = i === currentPage ? 'active' : '';
            html += `<button class="btn page-btn ${activeClass}" onclick="webInterface.${managerName}.changePage(${i})">${i}</button>`;
        }
        
        if (endPage < totalPages) {
            if (endPage < totalPages - 1) {
                html += '<span class="pagination-ellipsis">...</span>';
            }
            html += `<button class="btn page-btn" onclick="webInterface.${managerName}.changePage(${totalPages})">${totalPages}</button>`;
        }
        
        // 下一页
        if (currentPage < totalPages) {
            html += `<button class="btn page-btn" onclick="webInterface.${managerName}.changePage(${currentPage + 1})">下一页 ›</button>`;
        }
        
        html += '</div>';
        return html;
    },
    
    // 创建连接状态指示器
    createConnectionIndicator() {
        const header = document.querySelector('header');
        if (header && !document.getElementById('connection-indicator')) {
            const indicator = document.createElement('div');
            indicator.id = 'connection-indicator';
            indicator.className = 'connection-indicator disconnected';
            indicator.textContent = '⚪';
            indicator.title = '连接状态指示器';
            indicator.style.cssText = `
                position: absolute;
                top: 20px;
                right: 20px;
                font-size: 20px;
                cursor: help;
            `;
            header.appendChild(indicator);
        }
    },
    
    // 获取连接状态信息
    getConnectionStatusInfo(status, reconnectAttempts, maxReconnectAttempts) {
        const statusMap = {
            'connected': { text: '🟢', tooltip: 'SSE连接已建立，实时更新中' },
            'connecting': { text: '🟡', tooltip: '正在连接...' },
            'reconnecting': { text: '🟠', tooltip: `重连中... (${reconnectAttempts}/${maxReconnectAttempts})` },
            'error': { text: '🔴', tooltip: 'SSE连接错误' },
            'failed': { text: '⚫', tooltip: 'SSE连接失败，使用定时刷新' },
            'disconnected': { text: '⚪', tooltip: '未连接' }
        };
        return statusMap[status] || statusMap['disconnected'];
    },
    
    // 更新连接状态显示
    updateConnectionStatus(status, reconnectAttempts, maxReconnectAttempts) {
        const indicator = document.getElementById('connection-indicator');
        if (indicator) {
            const statusInfo = this.getConnectionStatusInfo(status, reconnectAttempts, maxReconnectAttempts);
            indicator.className = `connection-indicator ${status}`;
            indicator.textContent = statusInfo.text;
            indicator.title = statusInfo.tooltip;
        }
    },
    
    // 生成或获取客户端ID
    getOrCreateClientId() {
        let clientId = localStorage.getItem('sse_client_id');
        if (!clientId) {
            clientId = 'client_' + Date.now() + '_' + Math.random().toString(36).substring(2, 11);
            localStorage.setItem('sse_client_id', clientId);
        }
        return clientId;
    },
    
    // 显示通知
    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;
        
        const colors = {
            success: 'var(--success-color, #10b981)',
            error: 'var(--error-color, #ef4444)',
            info: 'var(--info-color, #3b82f6)'
        };
        
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${colors[type]};
            color: white;
            padding: 15px 20px;
            border-radius: 8px;
            z-index: 1000;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            max-width: 400px;
        `;
        
        document.body.appendChild(notification);
        
        const timeout = type === 'error' ? 5000 : 3000;
        setTimeout(() => {
            if (notification.parentNode) {
                document.body.removeChild(notification);
            }
        }, timeout);
    },
    
    // 便捷通知方法
    showSuccess(message) {
        this.showNotification('✅ ' + message, 'success');
    },
    
    showError(message) {
        this.showNotification('❌ ' + message, 'error');
    },
    
    showInfo(message) {
        this.showNotification('ℹ️ ' + message, 'info');
    },
    
    // 生成连接统计HTML
    generateConnectionsStats(data) {
        return `
            <div class="stats-grid">
                <div class="stat-item">
                    <div class="stat-value">${data.total_requests || 0}</div>
                    <div class="stat-label">总请求数</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">${data.active_connections || 0}</div>
                    <div class="stat-label">活跃连接</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">${data.successful_requests || 0}</div>
                    <div class="stat-label">成功请求</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">${data.failed_requests || 0}</div>
                    <div class="stat-label">失败请求</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">${data.average_response_time || '0s'}</div>
                    <div class="stat-label">平均响应时间</div>
                </div>
            </div>
        `;
    },
    
    // 生成配置显示HTML
    generateConfigDisplay(config) {
        let html = '';

        Object.keys(config).forEach(section => {
            if (typeof config[section] === 'object' && config[section] !== null) {
                html += `<div class="config-section">`;
                html += `<h3>${section}</h3>`;
                
                Object.keys(config[section]).forEach(key => {
                    let value = config[section][key];
                    if (typeof value === 'object') {
                        value = JSON.stringify(value, null, 2);
                    }
                    
                    html += `
                        <div class="config-item">
                            <span class="config-key">${key}</span>
                            <span class="config-value">${value}</span>
                        </div>
                    `;
                });
                
                html += `</div>`;
            }
        });

        return html;
    },
    
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
    },
    
    // 更新元素文本内容（安全更新）
    updateElementText(id, text) {
        const element = document.getElementById(id);
        if (element) {
            element.textContent = text;
        }
    },
    
    // 更新元素HTML内容（安全更新）
    updateElementHTML(id, html) {
        const element = document.getElementById(id);
        if (element) {
            element.innerHTML = html;
        }
    },
    
    // 切换元素显示状态
    toggleElementDisplay(id, show) {
        const element = document.getElementById(id);
        if (element) {
            element.style.display = show ? 'block' : 'none';
        }
    },
    
    // 安全的事件处理器绑定
    bindEventSafely(selector, event, handler) {
        const elements = document.querySelectorAll(selector);
        elements.forEach(element => {
            element.addEventListener(event, handler);
        });
    },
    
    // 清理模态框
    removeModal(modalSelector) {
        const modal = document.querySelector(modalSelector);
        if (modal && modal.parentNode) {
            modal.parentNode.removeChild(modal);
        }
    }
};

console.log('✅ Utils模块已加载');
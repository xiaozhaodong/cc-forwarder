package web

// indexHTML contains the complete HTML template for the web interface
const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Claude Request Forwarder - Web界面</title>
    <link rel="stylesheet" href="/static/css/style.css">
    <!-- Chart.js with fallback and timeout -->
    <script>
    window.chartLoadTimeout = setTimeout(() => {
        if (!window.Chart) {
            console.warn('Chart.js CDN loading timeout, charts will be disabled');
            window.chartLoadFailed = true;
        }
    }, 3000);
    </script>
    <script src="/static/js/lib/chart.umd.js"
            onload="clearTimeout(window.chartLoadTimeout); console.log('Chart.js loaded successfully');"
            onerror="window.chartLoadFailed=true; clearTimeout(window.chartLoadTimeout); console.warn('Chart.js local file failed, charts disabled');"></script>
    <style>
        .connection-indicator {
            position: absolute;
            top: 20px;
            right: 20px;
            font-size: 20px;
            cursor: help;
            transition: all 0.3s ease;
        }
        .connection-indicator.connected {
            color: #10b981;
        }
        .connection-indicator.connecting {
            color: #f59e0b;
            animation: pulse 1s infinite;
        }
        .connection-indicator.reconnecting {
            color: #f97316;
            animation: pulse 1.5s infinite;
        }
        .connection-indicator.error {
            color: #ef4444;
        }
        .connection-indicator.failed {
            color: #6b7280;
        }
        .connection-indicator.disconnected {
            color: #9ca3af;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        header {
            position: relative;
        }
        .notification {
            animation: slideIn 0.3s ease-out;
        }
        @keyframes slideIn {
            from {
                transform: translateX(100%);
                opacity: 0;
            }
            to {
                transform: translateX(0);
                opacity: 1;
            }
        }
        
        /* 图表样式 */
        .charts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        .chart-container {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            min-height: 400px;
            position: relative;
        }
        .chart-header {
            display: flex;
            justify-content: between;
            align-items: center;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 1px solid #e5e7eb;
        }
        .chart-title {
            font-size: 18px;
            font-weight: 600;
            color: #1f2937;
        }
        .chart-controls {
            display: flex;
            gap: 10px;
        }
        .chart-controls select {
            padding: 5px 10px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 12px;
            background: white;
        }
        .chart-controls button {
            padding: 5px 10px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 12px;
            background: white;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .chart-controls button:hover {
            background: #f3f4f6;
        }
        .chart-canvas {
            position: relative;
            height: 300px;
            width: 100%;
        }
        .chart-loading {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            color: #6b7280;
            font-size: 14px;
        }
        
        @media (max-width: 768px) {
            .charts-grid {
                grid-template-columns: 1fr;
            }
            .chart-container {
                min-height: 300px;
            }
            .chart-canvas {
                height: 250px;
            }
        }
        
        /* 可折叠区域样式 */
        .collapsible-section {
            background: white;
            border-radius: 12px;
            margin-bottom: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            border: 1px solid #e5e7eb;
            overflow: hidden;
        }
        
        .section-header {
            padding: 15px 20px;
            background: #f8fafc;
            border-bottom: 1px solid #e5e7eb;
            cursor: pointer;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: all 0.2s ease;
            user-select: none;
        }
        
        .section-header:hover {
            background: #f1f5f9;
        }
        
        .section-header h3 {
            margin: 0;
            font-size: 16px;
            font-weight: 600;
            color: #1f2937;
        }
        
        .section-header h4 {
            margin: 0;
            font-size: 14px;
            font-weight: 600;
            color: #1f2937;
        }
        
        .collapse-indicator {
            font-size: 12px;
            color: #6b7280;
            transition: transform 0.3s ease;
            font-weight: bold;
        }
        
        .section-content {
            padding: 20px;
            transition: all 0.3s ease;
            overflow: hidden;
        }
        
        .section-content.collapsed {
            max-height: 0;
            padding: 0 20px;
            opacity: 0;
        }
        
        .section-content.expanded {
            max-height: none;
            opacity: 1;
        }
        
        /* 折叠区域内的卡片样式调整 */
        .collapsible-section .cards {
            margin-bottom: 20px;
        }
        
        .collapsible-section .card h5 {
            font-size: 14px;
            margin-bottom: 5px;
            color: #374151;
        }
        
        .collapsible-section .card h4 {
            font-size: 16px;
            margin: 15px 0 10px 0;
            color: #1f2937;
            border-bottom: 1px solid #e5e7eb;
            padding-bottom: 8px;
        }
        
        /* 智能展开提示 */
        .section-header.has-alerts {
            background: linear-gradient(135deg, #fef3c7, #fde68a);
            border-bottom-color: #f59e0b;
        }
        
        .section-header.has-alerts h3,
        .section-header.has-alerts h4 {
            color: #92400e;
        }
        
        .section-header.has-alerts .collapse-indicator {
            color: #f59e0b;
        }
        
        /* 响应式设计 */
        @media (max-width: 768px) {
            .section-header {
                padding: 12px 15px;
            }
            
            .section-header h3 {
                font-size: 14px;
            }
            
            .section-content {
                padding: 15px;
            }
            
            .section-content.collapsed {
                padding: 0 15px;
            }
        }
        
        /* 挂起请求相关样式 */
        .alert-banner {
            background: linear-gradient(135deg, #fef3c7, #fbbf24);
            border: 2px solid #f59e0b;
            border-radius: 12px;
            padding: 15px;
            margin-bottom: 20px;
            display: flex;
            align-items: center;
            gap: 12px;
            box-shadow: 0 4px 12px rgba(245, 158, 11, 0.2);
            animation: slideInFromTop 0.5s ease-out;
        }
        
        .alert-banner.warning {
            background: linear-gradient(135deg, #fef3c7, #fbbf24);
            border-color: #f59e0b;
        }
        
        .alert-banner.info {
            background: linear-gradient(135deg, #dbeafe, #60a5fa);
            border-color: #3b82f6;
            box-shadow: 0 4px 12px rgba(59, 130, 246, 0.2);
        }
        
        .alert-icon {
            font-size: 24px;
            flex-shrink: 0;
        }
        
        .alert-content {
            flex-grow: 1;
        }
        
        .alert-title {
            font-weight: 600;
            font-size: 16px;
            margin-bottom: 4px;
            color: #1f2937;
        }
        
        .alert-message {
            font-size: 14px;
            color: #4b5563;
            line-height: 1.4;
        }
        
        .alert-close {
            background: none;
            border: none;
            font-size: 20px;
            cursor: pointer;
            color: #6b7280;
            padding: 4px 8px;
            border-radius: 4px;
            transition: all 0.2s ease;
        }
        
        .alert-close:hover {
            background: rgba(0, 0, 0, 0.1);
            color: #1f2937;
        }
        
        @keyframes slideInFromTop {
            from {
                transform: translateY(-20px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }
        
        .suspended-connection-item {
            background: #fef3c7;
            border-left: 4px solid #f59e0b;
            padding: 12px;
            margin-bottom: 8px;
            border-radius: 0 8px 8px 0;
            transition: all 0.2s ease;
        }
        
        .suspended-connection-item:hover {
            background: #fde68a;
            transform: translateX(2px);
        }
        
        .connection-header {
            display: flex;
            justify-content: between;
            align-items: center;
            margin-bottom: 8px;
        }
        
        .connection-id {
            font-weight: 600;
            font-family: monospace;
            font-size: 12px;
            color: #1f2937;
        }
        
        .suspended-time {
            color: #f59e0b;
            font-weight: 500;
            font-size: 12px;
        }
        
        .connection-details {
            font-size: 12px;
            color: #6b7280;
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 8px;
        }
        
        .text-muted {
            color: #6b7280;
            font-size: 12px;
        }
        
        .text-warning {
            color: #f59e0b;
            font-weight: 500;
        }
        
        /* 请求追踪页面样式 */
        .filters-panel {
            background: white;
            border-radius: 12px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            border: 1px solid #e5e7eb;
        }
        
        .filters-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            align-items: end;
        }
        
        .filter-group {
            display: flex;
            flex-direction: column;
            gap: 5px;
        }
        
        .filter-group label {
            font-size: 12px;
            font-weight: 600;
            color: #374151;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .filter-group select,
        .filter-input {
            padding: 8px 12px;
            border: 1px solid #d1d5db;
            border-radius: 8px;
            font-size: 14px;
            background: white;
            transition: border-color 0.2s ease, box-shadow 0.2s ease;
        }
        
        .filter-group select:focus,
        .filter-input:focus {
            outline: none;
            border-color: #3b82f6;
            box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
        }
        
        .filter-actions {
            display: flex;
            gap: 10px;
            grid-column: 1 / -1;
            justify-content: flex-end;
            margin-top: 10px;
        }
        
        .btn {
            padding: 8px 16px;
            border: none;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        
        .btn-primary {
            background: #3b82f6;
            color: white;
        }
        
        .btn-primary:hover {
            background: #2563eb;
            transform: translateY(-1px);
        }
        
        .btn-secondary {
            background: #6b7280;
            color: white;
        }
        
        .btn-secondary:hover {
            background: #4b5563;
        }
        
        .btn-export {
            background: #059669;
            color: white;
        }
        
        .btn-export:hover {
            background: #047857;
        }
        
        .btn-sm {
            padding: 6px 12px;
            font-size: 12px;
        }
        
        /* 统计概览卡片 */
        .stats-overview {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        
        .stats-card {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            border: 1px solid #e5e7eb;
            display: flex;
            align-items: center;
            gap: 15px;
            transition: transform 0.2s ease, box-shadow 0.2s ease;
        }
        
        .stats-card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
        }
        
        .stats-card.success {
            border-left: 4px solid #10b981;
        }
        
        .stats-card.warning {
            border-left: 4px solid #f59e0b;
        }
        
        .stats-card.cost {
            border-left: 4px solid #8b5cf6;
        }
        
        .stat-icon {
            font-size: 28px;
            opacity: 0.8;
        }
        
        .stat-content {
            flex: 1;
        }
        
        .stat-value {
            font-size: 24px;
            font-weight: 700;
            color: #1f2937;
            margin-bottom: 4px;
        }
        
        .stat-label {
            font-size: 12px;
            color: #6b7280;
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        /* 请求表格样式 */
        .requests-table-container {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            border: 1px solid #e5e7eb;
            margin-bottom: 20px;
        }
        
        .table-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 1px solid #e5e7eb;
        }
        
        .table-header h3 {
            margin: 0;
            font-size: 18px;
            font-weight: 600;
            color: #1f2937;
        }
        
        .table-actions {
            display: flex;
            align-items: center;
            gap: 15px;
        }
        
        #requests-count-info {
            font-size: 12px;
            color: #6b7280;
        }
        
        .table-wrapper {
            overflow-x: auto;
            border-radius: 8px;
            border: 1px solid #e5e7eb;
        }
        
        .requests-table {
            width: 100%;
            border-collapse: collapse;
            font-size: 13px;
        }
        
        .requests-table th {
            background: #f8fafc;
            padding: 12px 8px;
            text-align: left;
            font-weight: 600;
            color: #374151;
            border-bottom: 1px solid #e5e7eb;
            white-space: nowrap;
        }
        
        .requests-table th.sortable {
            cursor: pointer;
            user-select: none;
            transition: background-color 0.2s ease;
        }
        
        .requests-table th.sortable:hover {
            background: #f1f5f9;
        }
        
        .requests-table th.sortable:after {
            content: ' ⇅';
            opacity: 0.4;
            font-size: 11px;
        }
        
        .requests-table th.sort-asc:after {
            content: ' ↑';
            opacity: 1;
        }
        
        .requests-table th.sort-desc:after {
            content: ' ↓';
            opacity: 1;
        }
        
        .requests-table td {
            padding: 12px 8px;
            border-bottom: 1px solid #f3f4f6;
            vertical-align: top;
        }
        
        .requests-table tr:hover {
            background: #f9fafb;
        }
        
        .loading-row {
            text-align: center;
            padding: 40px !important;
            color: #6b7280;
        }
        
        .loading-spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #e5e7eb;
            border-top: 2px solid #3b82f6;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-right: 8px;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        /* 状态指示器 */
        .status-indicator {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 4px 8px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .status-indicator.success {
            background: #d1fae5;
            color: #065f46;
        }
        
        .status-indicator.failed {
            background: #fee2e2;
            color: #991b1b;
        }
        
        .status-indicator.timeout {
            background: #fef3c7;
            color: #92400e;
        }
        
        .status-indicator.suspended {
            background: #ede9fe;
            color: #5b21b6;
        }
        
        /* 状态徽章样式 - 用于请求追踪页面 */
        .status-badge {
            display: inline-flex;
            align-items: center;
            padding: 4px 8px;
            border-radius: 12px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            white-space: nowrap;
        }
        
        .status-badge.status-forwarding {
            background: linear-gradient(135deg, #60a5fa, #3b82f6);
            color: white;
            animation: pulse 2s infinite;
        }
        
        .status-badge.status-processing {
            background: linear-gradient(135deg, #fbbf24, #f59e0b);
            color: #92400e;
            animation: pulse 2s infinite;
        }
        
        .status-badge.status-completed {
            background: linear-gradient(135deg, #10b981, #059669);
            color: white;
        }
        
        .status-badge.status-success {
            background: linear-gradient(135deg, #10b981, #059669);
            color: white;
        }
        
        .status-badge.status-error {
            background: linear-gradient(135deg, #ef4444, #dc2626);
            color: white;
        }
        
        .status-badge.status-cancelled {
            background: linear-gradient(135deg, #8b5cf6, #7c3aed);
            color: white;
        }
        
        .status-badge.status-suspended {
            background: linear-gradient(135deg, #6366f1, #4f46e5);
            color: white;
            animation: pulse 2s infinite;
        }
        
        .status-badge.status-timeout {
            background: linear-gradient(135deg, #f97316, #ea580c);
            color: white;
        }
        
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.7; }
        }
        
        /* 分页样式 */
        .pagination-container {
            background: white;
            border-radius: 12px;
            padding: 15px 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            border: 1px solid #e5e7eb;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .pagination-info {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 14px;
            color: #6b7280;
        }
        
        .pagination-info select {
            padding: 4px 8px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 14px;
            background: white;
        }
        
        .pagination-controls {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .btn-pagination {
            padding: 6px 12px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            background: white;
            color: #374151;
            cursor: pointer;
            font-size: 12px;
            transition: all 0.2s ease;
        }
        
        .btn-pagination:hover:not(:disabled) {
            background: #f3f4f6;
            border-color: #9ca3af;
        }
        
        .btn-pagination:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        
        .page-input-group {
            display: flex;
            align-items: center;
            gap: 6px;
            font-size: 14px;
            color: #374151;
        }
        
        .page-input-group input {
            width: 60px;
            padding: 4px 8px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            text-align: center;
            font-size: 14px;
        }
        
        /* 响应式设计 */
        @media (max-width: 1200px) {
            .stats-overview {
                grid-template-columns: repeat(3, 1fr);
            }
        }
        
        @media (max-width: 768px) {
            .filters-grid {
                grid-template-columns: 1fr;
            }
            
            .stats-overview {
                grid-template-columns: repeat(2, 1fr);
            }
            
            .table-header {
                flex-direction: column;
                align-items: flex-start;
                gap: 10px;
            }
            
            .pagination-container {
                flex-direction: column;
                gap: 15px;
                align-items: stretch;
            }
            
            .pagination-controls {
                justify-content: center;
            }
            
            .filter-actions {
                justify-content: stretch;
            }
            
            .filter-actions .btn {
                flex: 1;
                justify-content: center;
            }
        }
        
        @media (max-width: 480px) {
            .stats-overview {
                grid-template-columns: 1fr;
            }
            
            .stats-card {
                padding: 15px;
            }
            
            .stat-value {
                font-size: 20px;
            }
            
            .pagination-controls {
                flex-wrap: wrap;
                gap: 5px;
            }
            
            .btn-pagination {
                font-size: 11px;
                padding: 4px 8px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🌐 Claude Request Forwarder</h1>
            <p>高性能API请求转发器 - Web监控界面</p>
        </header>

        <nav class="nav-tabs">
            <button class="nav-tab active" onclick="showTab('overview')">📊 概览</button>
            <button class="nav-tab" onclick="showTab('charts')">📈 图表</button>
            <button class="nav-tab" onclick="showTab('endpoints')">📡 端点</button>
            <button class="nav-tab" onclick="showTab('groups')">📦 组管理</button>
            <button class="nav-tab" onclick="showTab('requests')">📊 请求追踪</button>
            <button class="nav-tab" onclick="showTab('config')">⚙️ 配置</button>
        </nav>

        <main>
            <!-- 概览标签页 -->
            <div id="overview" class="tab-content active">
                <div class="cards">
                    <div class="card">
                        <h3>🚀 服务状态</h3>
                        <p id="server-status">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>⏱️ 运行时间</h3>
                        <p id="uptime">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>📡 端点数量</h3>
                        <p id="endpoint-count">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>🔗 总请求数</h3>
                        <p id="total-requests">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>⏸️ 挂起请求</h3>
                        <p id="suspended-requests">加载中...</p>
                        <small id="suspended-success-rate" class="text-muted">成功率: --</small>
                    </div>
                    <div class="card">
                        <h3>🔄 当前活动组</h3>
                        <p id="active-group">加载中...</p>
                        <small id="group-suspended-info" class="text-warning"></small>
                    </div>
                </div>
                
                <!-- 连接统计详情区域（可折叠） -->
                <div class="collapsible-section" id="connection-details-section">
                    <div class="section-header" onclick="toggleCollapsible('connection-details')">
                        <h3>🔗 连接统计详情</h3>
                        <span class="collapse-indicator" id="connection-details-indicator">▼</span>
                    </div>
                    <div class="section-content collapsed" id="connection-details-content">
                        <div id="connections-stats">
                            <p>加载中...</p>
                        </div>
                    </div>
                </div>

                <!-- 挂起请求监控区域（可折叠） -->
                <div class="collapsible-section" id="suspended-monitoring-section">
                    <div class="section-header" onclick="toggleCollapsible('suspended-monitoring')">
                        <h3>⏸️ 挂起请求监控</h3>
                        <span class="collapse-indicator" id="suspended-monitoring-indicator">▼</span>
                    </div>
                    <div class="section-content collapsed" id="suspended-monitoring-content">
                        <!-- 挂起请求统计 -->
                        <h4>⏸️ 挂起请求状态</h4>
                        <div id="suspended-stats" class="cards">
                            <div class="card">
                                <h5>当前挂起</h5>
                                <p id="current-suspended">0</p>
                            </div>
                            <div class="card">
                                <h5>历史总数</h5>
                                <p id="total-suspended">0</p>
                            </div>
                            <div class="card">
                                <h5>成功恢复</h5>
                                <p id="successful-suspended">0</p>
                            </div>
                            <div class="card">
                                <h5>超时失败</h5>
                                <p id="timeout-suspended">0</p>
                            </div>
                            <div class="card">
                                <h5>成功率</h5>
                                <p id="suspended-success-rate-detail">0%</p>
                            </div>
                            <div class="card">
                                <h5>平均挂起时间</h5>
                                <p id="avg-suspended-time">0ms</p>
                            </div>
                        </div>
                        
                        <!-- 当前挂起的连接列表 -->
                        <div id="suspended-connections-section">
                            <h4>当前挂起的连接</h4>
                            <div id="suspended-connections-table">
                                <p>无挂起连接</p>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- 图表标签页 -->
            <div id="charts" class="tab-content">
                <div class="section">
                    <h2>📈 数据可视化</h2>
                    <div class="charts-grid">
                        <!-- 请求趋势图 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">请求趋势</div>
                                <div class="chart-controls">
                                    <select id="requestTrendTimeRange" onchange="updateChartTimeRange('requestTrend', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('requestTrend', '请求趋势图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="requestTrendChart"></canvas>
                                <div id="requestTrendLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 响应时间图 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">响应时间</div>
                                <div class="chart-controls">
                                    <select id="responseTimeTimeRange" onchange="updateChartTimeRange('responseTime', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('responseTime', '响应时间图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="responseTimeChart"></canvas>
                                <div id="responseTimeLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- Token使用统计 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">Token使用分布</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('tokenUsage', 'Token使用图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="tokenUsageChart"></canvas>
                                <div id="tokenUsageLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 端点健康状态 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">端点健康状态</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('endpointHealth', '端点健康图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="endpointHealthChart"></canvas>
                                <div id="endpointHealthLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 连接活动 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">连接活动</div>
                                <div class="chart-controls">
                                    <select id="connectionActivityTimeRange" onchange="updateChartTimeRange('connectionActivity', this.value)">
                                        <option value="30">30分钟</option>
                                        <option value="60" selected>1小时</option>
                                        <option value="180">3小时</option>
                                        <option value="360">6小时</option>
                                    </select>
                                    <button onclick="exportChart('connectionActivity', '连接活动图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="connectionActivityChart"></canvas>
                                <div id="connectionActivityLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 端点性能对比 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">端点性能对比</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('endpointPerformance', '端点性能图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="endpointPerformanceChart"></canvas>
                                <div id="endpointPerformanceLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>
                        
                        <!-- 挂起请求趋势 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">挂起请求趋势</div>
                                <div class="chart-controls">
                                    <select id="suspendedTrendTimeRange" onchange="updateChartTimeRange('suspendedTrend', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('suspendedTrend', '挂起请求趋势图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="suspendedTrendChart"></canvas>
                                <div id="suspendedTrendLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- 端点标签页 -->
            <div id="endpoints" class="tab-content">
                <div class="section">
                    <h2>📡 端点状态</h2>
                    <div id="endpoints-table">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>

            <!-- 组管理标签页 -->
            <div id="groups" class="tab-content">
                <div class="section">
                    <h2>📦 组管理</h2>
                    
                    <!-- 挂起请求提示信息 -->
                    <div id="group-suspended-alert" class="alert-banner" style="display: none;">
                        <div class="alert-icon">⏸️</div>
                        <div class="alert-content">
                            <div class="alert-title">挂起请求通知</div>
                            <div class="alert-message" id="suspended-alert-message">有请求正在等待组切换...</div>
                        </div>
                        <button class="alert-close" onclick="hideSuspendedAlert()">×</button>
                    </div>
                    
                    <div class="group-info-cards" id="group-info-cards">
                        <p>加载中...</p>
                    </div>
                    <div class="groups-container" id="groups-container">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>


            <!-- 请求追踪标签页 -->
            <div id="requests" class="tab-content">
                <div class="section">
                    <h2>📊 请求追踪</h2>
                    
                    <!-- 筛选面板 -->
                    <div class="filters-panel">
                        <div class="filters-grid">
                            <div class="filter-group">
                                <label>时间范围:</label>
                                <select id="time-range-filter">
                                    <option value="" selected>全部时间</option>
                                    <option value="1h">最近1小时</option>
                                    <option value="6h">最近6小时</option>
                                    <option value="24h">最近24小时</option>
                                    <option value="7d">最近7天</option>
                                    <option value="30d">最近30天</option>
                                    <option value="custom">自定义</option>
                                </select>
                            </div>
                            
                            <div class="filter-group" id="custom-date-range" style="display: none;">
                                <label>自定义时间:</label>
                                <input type="datetime-local" id="start-date" class="filter-input">
                                <span>至</span>
                                <input type="datetime-local" id="end-date" class="filter-input">
                            </div>
                            
                            <div class="filter-group">
                                <label>状态:</label>
                                <select id="status-filter">
                                    <option value="all">全部状态</option>
                                    <option value="success">成功</option>
                                    <option value="failed">失败</option>
                                    <option value="timeout">超时</option>
                                    <option value="suspended">挂起</option>
                                </select>
                            </div>
                            
                            <div class="filter-group">
                                <label>模型:</label>
                                <select id="model-filter">
                                    <option value="all">全部模型</option>
                                    <!-- 模型选项将通过JavaScript动态加载 -->
                                </select>
                            </div>
                            
                            <div class="filter-group">
                                <label>端点:</label>
                                <select id="endpoint-filter">
                                    <option value="all">全部端点</option>
                                </select>
                            </div>
                            
                            <div class="filter-group">
                                <label>组:</label>
                                <select id="group-filter">
                                    <option value="all">全部组</option>
                                </select>
                            </div>
                            
                            <div class="filter-actions">
                                <button class="btn btn-primary" onclick="applyFilters()">🔍 搜索</button>
                                <button class="btn btn-secondary" onclick="resetFilters()">🔄 重置</button>
                            </div>
                        </div>
                    </div>
                    
                    <!-- 统计概览卡片 -->
                    <div class="stats-overview">
                        <div class="stats-card">
                            <div class="stat-icon">🚀</div>
                            <div class="stat-content">
                                <div class="stat-value" id="total-requests-count">-</div>
                                <div class="stat-label">总请求数</div>
                            </div>
                        </div>
                        
                        <div class="stats-card success">
                            <div class="stat-icon">✅</div>
                            <div class="stat-content">
                                <div class="stat-value" id="success-rate">-</div>
                                <div class="stat-label">成功率</div>
                            </div>
                        </div>
                        
                        <div class="stats-card">
                            <div class="stat-icon">⏱️</div>
                            <div class="stat-content">
                                <div class="stat-value" id="avg-response-time">-</div>
                                <div class="stat-label">平均响应时间</div>
                            </div>
                        </div>
                        
                        <div class="stats-card cost">
                            <div class="stat-icon">💰</div>
                            <div class="stat-content">
                                <div class="stat-value" id="total-cost">-</div>
                                <div class="stat-label">总成本 (USD)</div>
                            </div>
                        </div>
                        
                        <div class="stats-card">
                            <div class="stat-icon">🪙</div>
                            <div class="stat-content">
                                <div class="stat-value" id="total-tokens">-</div>
                                <div class="stat-label">总Token数 (M)</div>
                            </div>
                        </div>
                        
                        <div class="stats-card warning">
                            <div class="stat-icon">⏸️</div>
                            <div class="stat-content">
                                <div class="stat-value" id="suspended-count">-</div>
                                <div class="stat-label">挂起请求数</div>
                            </div>
                        </div>
                    </div>
                    
                    <!-- 请求列表表格 -->
                    <div class="requests-table-container">
                        <div class="table-header">
                            <h3>请求详情列表</h3>
                            <div class="table-actions">
                                <span id="requests-count-info">显示 0-0 条，共 0 条记录</span>
                                <button class="btn btn-sm" onclick="webInterface.requestsManager.loadRequests()">🔄 刷新</button>
                            </div>
                        </div>
                        
                        <div class="table-wrapper">
                            <table class="requests-table">
                                <thead>
                                    <tr>
                                        <th class="sortable" data-sort="request_id">请求ID</th>
                                        <th class="sortable" data-sort="timestamp">时间</th>
                                        <th class="sortable" data-sort="status">状态</th>
                                        <th class="sortable" data-sort="model">模型</th>
                                        <th class="sortable" data-sort="endpoint">端点</th>
                                        <th class="sortable" data-sort="group">组</th>
                                        <th class="sortable" data-sort="duration">耗时</th>
                                        <th class="sortable" data-sort="input_tokens">输入</th>
                                        <th class="sortable" data-sort="output_tokens">输出</th>
                                        <th class="sortable" data-sort="cache_creation_tokens">缓存创建</th>
                                        <th class="sortable" data-sort="cache_read_tokens">缓存读取</th>
                                        <th class="sortable" data-sort="cost">成本</th>
                                    </tr>
                                </thead>
                                <tbody id="requests-table-body">
                                    <tr>
                                        <td colspan="11" class="loading-row">
                                            <div class="loading-spinner"></div>
                                            正在加载请求数据...
                                        </td>
                                    </tr>
                                </tbody>
                            </table>
                        </div>
                    </div>
                    
                    <!-- 分页控制 -->
                    <div class="pagination-container">
                        <div class="pagination-info">
                            <span>每页显示：</span>
                            <select id="page-size-select" onchange="changePageSize()">
                                <option value="20">20</option>
                                <option value="50" selected>50</option>
                                <option value="100">100</option>
                                <option value="200">200</option>
                            </select>
                            <span>条记录</span>
                        </div>
                        
                        <div class="pagination-controls">
                            <button class="btn btn-pagination" onclick="goToFirstPage()" disabled>⏮️ 首页</button>
                            <button class="btn btn-pagination" onclick="goToPrevPage()" disabled>⏪ 上一页</button>
                            
                            <div class="page-input-group">
                                <span>第</span>
                                <input type="number" id="current-page-input" value="1" min="1" onchange="goToPage()">
                                <span>/</span>
                                <span id="total-pages">1</span>
                                <span>页</span>
                            </div>
                            
                            <button class="btn btn-pagination" onclick="goToNextPage()">下一页 ⏩</button>
                            <button class="btn btn-pagination" onclick="goToLastPage()">末页 ⏭️</button>
                        </div>
                    </div>
                </div>
            </div>

            <!-- 配置标签页 -->
            <div id="config" class="tab-content">
                <div class="section">
                    <h2>⚙️ 当前配置</h2>
                    <div id="config-display">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>
        </main>
    </div>

    <script src="/static/js/charts.js"></script>
    <!-- 模块化JavaScript文件 -->
    <script src="/static/js/utils.js"></script>
    <script src="/static/js/sseManager.js"></script>
    <script src="/static/js/requestsManager.js"></script>
    <script src="/static/js/groupsManager.js"></script>
    <script src="/static/js/endpointsManager.js"></script>
    <script src="/static/js/webInterface.js"></script>
    <script>
        // 全局图表管理器
        let chartManager = null;

        // 等待页面完全加载后再扩展功能
        window.addEventListener('load', function() {
            // 等待WebInterface初始化完成
            setTimeout(() => {
                if (window.webInterface) {
                    console.log('📊 扩展图表功能到WebInterface');
                    
                    // 保存原始的showTab方法
                    const originalShowTab = window.webInterface.showTab.bind(window.webInterface);
                    
                    // 扩展showTab方法以支持图表
                    window.webInterface.showTab = function(tabName) {
                        originalShowTab(tabName);
                        
                        // 当切换到图表标签时，确保图表已初始化并更新数据
                        if (tabName === 'charts') {
                            initializeCharts();
                        }
                    };
                    
                    console.log('✅ 图表功能扩展完成');
                } else {
                    console.error('❌ WebInterface未找到，无法扩展图表功能');
                }
            }, 200);
        });

        // 初始化图表
        async function initializeCharts() {
            if (chartManager) {
                return; // 已经初始化过了
            }
            
            try {
                console.log('🔄 开始初始化图表系统...');
                chartManager = new ChartManager();
                await chartManager.initializeCharts();
                
                // 隐藏加载指示器
                document.querySelectorAll('.chart-loading').forEach(loading => {
                    loading.style.display = 'none';
                });
                
                console.log('✅ 图表系统初始化完成');
            } catch (error) {
                console.error('❌ 图表初始化失败:', error);
                if (window.webInterface && window.webInterface.showError) {
                    window.webInterface.showError('图表初始化失败: ' + error.message);
                }
            }
        }

        // 更新图表时间范围
        function updateChartTimeRange(chartName, minutes) {
            if (chartManager) {
                chartManager.updateTimeRange(chartName, parseInt(minutes));
            }
        }

        // 导出图表
        function exportChart(chartName, filename) {
            if (chartManager) {
                chartManager.exportChart(chartName, filename);
            } else {
                window.webInterface?.showError('图表管理器未初始化');
            }
        }

        // 页面卸载时清理图表资源
        window.addEventListener('beforeunload', () => {
            if (chartManager) {
                chartManager.destroy();
            }
        });
        
        // 全局折叠/展开函数
        window.toggleCollapsible = function(sectionId) {
            if (window.webInterface && typeof window.webInterface.toggleSection === 'function') {
                window.webInterface.toggleSection(sectionId);
            } else {
                console.warn('WebInterface未初始化，无法切换折叠状态');
            }
        };
    </script>
</body>
</html>`
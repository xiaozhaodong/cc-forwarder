package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/utils"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/tracking"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFiles embed.FS

// HistoryCollector 负责定期收集历史数据点
type HistoryCollector struct {
	metrics   *middleware.MonitoringMiddleware
	ticker    *time.Ticker
	stopChan  chan struct{}
	logger    *slog.Logger
	running   bool
}

// NewHistoryCollector 创建新的历史数据收集器
func NewHistoryCollector(middleware *middleware.MonitoringMiddleware, logger *slog.Logger) *HistoryCollector {
	return &HistoryCollector{
		metrics:  middleware,
		stopChan: make(chan struct{}),
		logger:   logger,
	}
}

// Start 启动历史数据收集器
func (hc *HistoryCollector) Start() {
	if hc.running {
		return
	}

	hc.running = true
	hc.ticker = time.NewTicker(30 * time.Second) // 每30秒收集一次

	go func() {
		hc.logger.Info("📊 历史数据收集器已启动 (30秒间隔)")

		for {
			select {
			case <-hc.ticker.C:
				hc.collectData()
			case <-hc.stopChan:
				hc.logger.Info("📊 历史数据收集器已停止")
				return
			}
		}
	}()
}

// Stop 停止历史数据收集器
func (hc *HistoryCollector) Stop() {
	if !hc.running {
		return
	}

	hc.running = false
	if hc.ticker != nil {
		hc.ticker.Stop()
	}
	close(hc.stopChan)
}

// collectData 收集历史数据点
func (hc *HistoryCollector) collectData() {
	if hc.metrics != nil {
		hc.metrics.GetMetrics().AddHistoryDataPoints()
	}
}

// WebServer represents the Web UI server
type WebServer struct {
	server              *http.Server
	engine              *gin.Engine
	logger              *slog.Logger
	config              *config.Config
	endpointManager     *endpoint.Manager
	monitoringMiddleware *middleware.MonitoringMiddleware
	usageTracker        *tracking.UsageTracker
	usageAPI           *UsageAPI
	eventManager        *SmartEventManager
	startTime           time.Time
	configPath          string
	historyCollector    *HistoryCollector
}

// NewWebServer creates a new Web UI server
func NewWebServer(cfg *config.Config, endpointManager *endpoint.Manager, monitoringMiddleware *middleware.MonitoringMiddleware, usageTracker *tracking.UsageTracker, logger *slog.Logger, startTime time.Time, configPath string, eventBus events.EventBus) *WebServer {
	// 设置gin为release模式以减少日志输出
	gin.SetMode(gin.ReleaseMode)
	
	engine := gin.New()
	
	// 添加自定义中间件来处理日志
	engine.Use(ginLoggerMiddleware(logger))
	engine.Use(gin.Recovery())
	
	ws := &WebServer{
		engine:              engine,
		logger:              logger,
		config:              cfg,
		endpointManager:     endpointManager,
		monitoringMiddleware: monitoringMiddleware,
		usageTracker:        usageTracker,
		usageAPI:           NewUsageAPI(usageTracker),
		eventManager:        NewSmartEventManager(logger),
		startTime:           startTime,
		configPath:          configPath,
		historyCollector:    NewHistoryCollector(monitoringMiddleware, logger),
	}
	
	// 设置EventBus的SSE适配器
	if eventBus != nil {
		sseAdapter := events.NewSSEAdapter(ws, logger)
		eventBus.SetSSEBroadcaster(sseAdapter)
	}
	
	// 保持兼容性 - 不再设置监控中间件的事件广播器
	// monitoringMiddleware.SetEventBroadcaster(ws)
	
	ws.setupRoutes()
	
	return ws
}

// Start启动Web服务器
func (ws *WebServer) Start() error {
	addr := fmt.Sprintf("%s:%d", ws.config.Web.Host, ws.config.Web.Port)
	
	ws.server = &http.Server{
		Addr:         addr,
		Handler:      ws.engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE连接需要禁用写入超时
		IdleTimeout:  300 * time.Second, // 5分钟空闲超时
	}
	
	ws.logger.Info(fmt.Sprintf("🌐 Web界面启动中... - 地址: %s", addr))
	
	go func() {
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ws.logger.Error(fmt.Sprintf("❌ Web服务器启动失败: %v", err))
		}
	}()
	
	// 给服务器一点启动时间
	time.Sleep(100 * time.Millisecond)

	// 注意：已移除所有定时推送机制，改为纯事件驱动架构
	// 原先移除的函数：
	// - startPeriodicBroadcast(): 15秒定时广播
	// - startHistoryDataCollection(): 30秒数据收集
	// - startChartDataBroadcast(): 60秒图表广播
	// - startStatusUpdateLoop(): 30秒状态更新循环（新移除）
	//
	// 运行时间现在由前端实时计算，无需服务器推送

	// 启动历史数据收集器 (修复请求趋势图表数据问题)
	if ws.historyCollector != nil {
		ws.historyCollector.Start()
	}

	ws.logger.Info(fmt.Sprintf("✅ Web界面启动成功！访问地址: http://%s", addr))
	
	return nil
}

// Stop优雅关闭Web服务器
func (ws *WebServer) Stop(ctx context.Context) error {
	if ws.server == nil {
		return nil
	}
	
	ws.logger.Info("🛑 正在关闭Web服务器...")

	// 停止历史数据收集器
	if ws.historyCollector != nil {
		ws.historyCollector.Stop()
	}

	// 停止事件管理器
	if ws.eventManager != nil {
		ws.eventManager.Stop()
	}
	
	err := ws.server.Shutdown(ctx)
	if err != nil {
		ws.logger.Error(fmt.Sprintf("❌ Web服务器关闭失败: %v", err))
	} else {
		ws.logger.Info("✅ Web服务器已安全关闭")
	}
	
	return err
}

// UpdateConfig更新配置
func (ws *WebServer) UpdateConfig(newConfig *config.Config) {
	ws.config = newConfig
	ws.logger.Info("🔄 Web服务器配置已更新")
	
	// 广播配置更新事件
	ws.BroadcastConfigUpdate(map[string]interface{}{
		"event": "config_updated",
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"config": map[string]interface{}{
			"server": newConfig.Server,
			"web": newConfig.Web,
			"strategy": newConfig.Strategy,
		},
	})
}

// setupRoutes设置路由
func (ws *WebServer) setupRoutes() {
	// 静态文件服务 - 修复embed文件系统路径
	staticFS, _ := fs.Sub(staticFiles, "static")
	ws.engine.StaticFS("/static", http.FS(staticFS))
	
	// 主页面
	ws.engine.GET("/", ws.handleIndex)
	
	// API路由组
	api := ws.engine.Group("/api/v1")
	{
		api.GET("/status", ws.handleStatus)
		api.GET("/endpoints", ws.handleEndpoints)
		api.GET("/connections", ws.handleConnections)
		api.GET("/config", ws.handleConfig)
		api.GET("/requests", ws.handleRequests)
		api.GET("/stream", ws.handleSSE)
		api.POST("/endpoints/:name/priority", ws.handleUpdatePriority)
		api.POST("/endpoints/:name/health-check", ws.handleManualHealthCheck)
		
		// 组管理API
		api.GET("/groups", ws.handleGroups)
		api.POST("/groups/:name/activate", ws.handleActivateGroup)
		api.POST("/groups/:name/pause", ws.handlePauseGroup)
		api.POST("/groups/:name/resume", ws.handleResumeGroup)
		
		// Chart.js 数据可视化 API 端点
		api.GET("/metrics/history", ws.handleMetricsHistory)
		api.GET("/endpoints/performance", ws.handleEndpointPerformance)
		api.GET("/tokens/usage", ws.handleTokenUsage)
		api.GET("/chart/request-trends", ws.handleRequestTrends)
		api.GET("/chart/response-times", ws.handleResponseTimes)
		api.GET("/chart/endpoint-health", ws.handleEndpointHealth)
		api.GET("/chart/connection-activity", ws.handleConnectionActivity)
		
		// 挂起请求相关 API 端点
		api.GET("/suspended/requests", ws.handleSuspendedRequests)
		api.GET("/chart/suspended-trends", ws.handleSuspendedChart)
		
		// 使用跟踪 API 端点
		api.GET("/usage/summary", ws.handleUsageSummary)
		api.GET("/usage/requests", ws.handleUsageRequests)
		api.GET("/usage/stats", ws.handleUsageStats)
		api.GET("/usage/export", ws.handleUsageExport)
		api.GET("/usage/models", ws.handleUsageModelStats)
		api.GET("/usage/endpoints", ws.handleUsageEndpointStats)
		api.GET("/chart/usage-trends", ws.handleUsageChart)
		api.GET("/chart/cost-analysis", ws.handleCostChart)
		api.GET("/chart/endpoint-costs", ws.handleEndpointCosts)
	}
	
	// WebSocket用于实时更新（暂时注释掉，使用SSE代替）
	// ws.engine.GET("/ws", ws.handleWebSocket)
}

// ginLoggerMiddleware创建gin的日志中间件
func ginLoggerMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		
		// 处理请求
		c.Next()
		
		// 计算延迟
		latency := time.Since(start)
		
		// 只记录非静态文件的请求
		if c.Request.Method != "GET" || (!strings.Contains(path, "/static") && path != "/favicon.ico") {
			clientIP := c.ClientIP()
			method := c.Request.Method
			statusCode := c.Writer.Status()
			
			if raw != "" {
				path = path + "?" + raw
			}
			
			// 根据状态码确定日志级别
			if statusCode >= 400 {
				logger.Warn(fmt.Sprintf("🌐 Web请求 %s %s %d %v %s", 
					method, path, statusCode, latency, clientIP))
			} else {
				logger.Debug(fmt.Sprintf("🌐 Web请求 %s %s %d %v %s", 
					method, path, statusCode, latency, clientIP))
			}
		}
	}
}

// 注意：定时推送函数已移除，改为纯事件驱动架构
// 原先移除的函数：
// - startPeriodicBroadcast(): 15秒定时广播
// - startHistoryDataCollection(): 30秒数据收集  
// - startChartDataBroadcast(): 60秒图表广播
// - startStatusUpdateLoop(): 30秒状态更新循环（新移除）
//
// 运行时间现在由前端基于服务器启动时间戳实时计算

// broadcastChartData 聚合广播图表数据，避免事件风暴
func (ws *WebServer) broadcastChartData() {
	metrics := ws.monitoringMiddleware.GetMetrics()
	chartDataMap := make(map[string]interface{})

	// 1. 收集请求趋势数据
	requestHistory := metrics.GetChartDataForRequestHistory(30)
	if len(requestHistory) > 0 {
		labels := make([]string, len(requestHistory))
		totalData := make([]int64, len(requestHistory))
		successData := make([]int64, len(requestHistory))
		failedData := make([]int64, len(requestHistory))
		for i, point := range requestHistory {
			labels[i] = point.Timestamp.Format("15:04")
			totalData[i] = point.Total
			successData[i] = point.Successful
			failedData[i] = point.Failed
		}
		chartDataMap["request_trends"] = map[string]interface{}{
			"labels": labels,
			"datasets": []map[string]interface{}{
				{"label": "总请求数", "data": totalData, "borderColor": "#3b82f6", "backgroundColor": "rgba(59, 130, 246, 0.1)", "fill": true},
				{"label": "成功请求", "data": successData, "borderColor": "#10b981", "backgroundColor": "rgba(16, 185, 129, 0.1)", "fill": true},
				{"label": "失败请求", "data": failedData, "borderColor": "#ef4444", "backgroundColor": "rgba(239, 68, 68, 0.1)", "fill": true},
			},
		}
	}

	// 2. 收集响应时间数据
	responseHistory := metrics.GetChartDataForResponseTime(30)
	if len(responseHistory) > 0 {
		labels := make([]string, len(responseHistory))
		avgData := make([]int64, len(responseHistory))
		minData := make([]int64, len(responseHistory))
		maxData := make([]int64, len(responseHistory))
		for i, point := range responseHistory {
			labels[i] = point.Timestamp.Format("15:04")
			avgData[i] = point.AverageTime.Milliseconds()
			minData[i] = point.MinTime.Milliseconds()
			maxData[i] = point.MaxTime.Milliseconds()
		}
		chartDataMap["response_times"] = map[string]interface{}{
			"labels": labels,
			"datasets": []map[string]interface{}{
				{"label": "平均响应时间", "data": avgData, "borderColor": "#f59e0b", "backgroundColor": "rgba(245, 158, 11, 0.1)", "fill": true},
				{"label": "最小响应时间", "data": minData, "borderColor": "#10b981", "backgroundColor": "rgba(16, 185, 129, 0.1)", "fill": false},
				{"label": "最大响应时间", "data": maxData, "borderColor": "#ef4444", "backgroundColor": "rgba(239, 68, 68, 0.1)", "fill": false},
			},
		}
	}

	// 3. 收集Token使用数据
	tokenStats := metrics.GetTotalTokenStats()
	chartDataMap["token_usage"] = map[string]interface{}{
		"labels": []string{"输入Token", "输出Token", "缓存创建Token", "缓存读取Token"},
		"datasets": []map[string]interface{}{
			{
				"data":            []int64{tokenStats.InputTokens, tokenStats.OutputTokens, tokenStats.CacheCreationTokens, tokenStats.CacheReadTokens},
				"backgroundColor": []string{"#3b82f6", "#10b981", "#f59e0b", "#8b5cf6"},
				"borderColor":     []string{"#2563eb", "#059669", "#d97706", "#7c3aed"},
				"borderWidth":     2,
			},
		},
	}

	// 4. 收集端点健康状态数据
	healthDistribution := metrics.GetEndpointHealthDistribution()
	chartDataMap["endpoint_health"] = map[string]interface{}{
		"labels": []string{"健康端点", "不健康端点"},
		"datasets": []map[string]interface{}{
			{
				"data":            []int{healthDistribution["healthy"], healthDistribution["unhealthy"]},
				"backgroundColor": []string{"#10b981", "#ef4444"},
				"borderColor":     []string{"#059669", "#dc2626"},
				"borderWidth":     2,
			},
		},
	}

	// 5. 收集端点性能数据
	performanceData := metrics.GetEndpointPerformanceData()
	if len(performanceData) > 0 {
		labels := make([]string, len(performanceData))
		responseTimeData := make([]map[string]interface{}, len(performanceData))
		backgroundColors := make([]string, len(performanceData))
		for i, ep := range performanceData {
			labels[i] = ep["name"].(string)
			responseTimeData[i] = map[string]interface{}{"x": ep["avg_response_time"], "endpointData": ep}
			if ep["healthy"].(bool) {
				backgroundColors[i] = "#10b981"
			} else {
				backgroundColors[i] = "#ef4444"
			}
		}
		chartDataMap["endpoint_performance"] = map[string]interface{}{
			"labels": labels,
			"datasets": []map[string]interface{}{
				{"label": "平均响应时间", "data": responseTimeData, "backgroundColor": backgroundColors, "borderColor": backgroundColors, "borderWidth": 1},
			},
		}
	}

	// 6. 收集挂起请求数据
	suspendedHistory := metrics.GetChartDataForSuspendedRequests(30)
	if len(suspendedHistory) > 0 {
		labels := make([]string, len(suspendedHistory))
		suspendedData := make([]int64, len(suspendedHistory))
		successfulData := make([]int64, len(suspendedHistory))
		timeoutData := make([]int64, len(suspendedHistory))
		for i, point := range suspendedHistory {
			labels[i] = point.Timestamp.Format("15:04")
			suspendedData[i] = point.SuspendedRequests
			successfulData[i] = point.SuccessfulSuspendedRequests
			timeoutData[i] = point.TimeoutSuspendedRequests
		}
		chartDataMap["suspended_trends"] = map[string]interface{}{
			"labels": labels,
			"datasets": []map[string]interface{}{
				{"label": "当前挂起请求", "data": suspendedData, "borderColor": "#f59e0b", "backgroundColor": "rgba(245, 158, 11, 0.1)", "fill": true},
				{"label": "成功恢复", "data": successfulData, "borderColor": "#10b981", "backgroundColor": "rgba(16, 185, 129, 0.1)", "fill": false},
				{"label": "超时失败", "data": timeoutData, "borderColor": "#ef4444", "backgroundColor": "rgba(239, 68, 68, 0.1)", "fill": false},
			},
		}
	}

	// 只有在收集到数据时才进行广播
	if len(chartDataMap) > 0 {
		ws.BroadcastBatchChartUpdate(chartDataMap)
	} else {
		ws.logger.Debug("📊 无图表数据更新，跳过广播")
	}

	ws.logger.Debug("📊 图表数据广播完成")
}

// BroadcastChartUpdate 广播图表更新事件
func (ws *WebServer) BroadcastChartUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeChart, data, nil)
	}
}

// BroadcastBatchChartUpdate 批量广播所有图表更新（优化版本，避免事件轰炸）
func (ws *WebServer) BroadcastBatchChartUpdate(chartDataMap map[string]interface{}) {
	if ws.eventManager != nil {
		// 将所有图表数据打包为一个事件，减少事件数量
		batchData := map[string]interface{}{
			"chart_type": "batch_update",  // 批量更新标识
			"charts":     chartDataMap,    // 包含所有图表的数据
			"timestamp":  time.Now().Unix(),
		}
		ws.eventManager.BroadcastEventSmart(EventTypeChart, batchData, nil)
		ws.logger.Debug("📊 批量图表数据广播完成", "charts_count", len(chartDataMap))
	}
}

// broadcastCurrentData 广播当前数据状态
func (ws *WebServer) broadcastCurrentData() {
	if ws.eventManager.GetClientCount() == 0 {
		return // 没有客户端连接，跳过广播
	}
	
	// 广播服务状态
	uptime := time.Since(ws.startTime)
	ws.BroadcastStatusUpdate(map[string]interface{}{
		"status":      "running",
		"uptime":      uptime.String(),
		"start_time":  ws.startTime.Format("2006-01-02 15:04:05"),
		"config_file": ws.configPath,
		"client_count": ws.eventManager.GetClientCount(),
	})
	
	// 广播端点状态
	endpoints := ws.endpointManager.GetEndpoints()
	endpointData := make([]map[string]interface{}, 0, len(endpoints))
	
	for _, ep := range endpoints {
		status := ws.endpointManager.GetEndpointStatus(ep.Config.Name)
		endpointData = append(endpointData, map[string]interface{}{
			"name":           ep.Config.Name,
			"url":            ep.Config.URL,
			"priority":       ep.Config.Priority,
			"group":          ep.Config.Group,
			"group_priority": ep.Config.GroupPriority,
			"healthy":        status.Healthy,
			"response_time":  utils.FormatResponseTime(status.ResponseTime),
			"last_check":     status.LastCheck.Format("2006-01-02 15:04:05"),
			"never_checked":  status.NeverChecked,
			"error":          "", // 暂时设为空字符串
		})
	}
	
	ws.BroadcastEndpointUpdate(map[string]interface{}{
		"endpoints": endpointData,
		"total":     len(endpointData),
	})
	
	// 广播连接统计
	metrics := ws.monitoringMiddleware.GetMetrics()
	stats := metrics.GetMetrics()
	suspendedStats := metrics.GetSuspendedRequestStats()
	suspendedConnections := metrics.GetActiveSuspendedConnections()

	// 统计每个组的挂起请求数量
	groupSuspendedCounts := make(map[string]int)
	for _, conn := range suspendedConnections {
		// 根据endpoint名称查找对应的组
		endpoints := ws.endpointManager.GetEndpoints()
		for _, ep := range endpoints {
			if ep.Config.Name == conn.Endpoint {
				groupSuspendedCounts[ep.Config.Group]++
				break
			}
		}
	}

	// 计算总Token使用量
	totalTokens := stats.TotalTokenUsage.InputTokens +
		stats.TotalTokenUsage.OutputTokens +
		stats.TotalTokenUsage.CacheCreationTokens +
		stats.TotalTokenUsage.CacheReadTokens

	ws.BroadcastConnectionUpdate(map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   len(stats.ActiveConnections),
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": utils.FormatResponseTime(stats.GetAverageResponseTime()),
		"total_tokens":         totalTokens,
		"success_rate":         stats.GetSuccessRate(),
		"suspended":            suspendedStats,
		"group_suspended_counts": groupSuspendedCounts,
		"total_suspended_requests": len(suspendedConnections),
		"max_suspended_requests": ws.config.RequestSuspend.MaxSuspendedRequests,
	})

	// 广播组状态
	groupDetails := ws.endpointManager.GetGroupDetails()

	ws.BroadcastGroupUpdate(map[string]interface{}{
		"groups":                groupDetails["groups"],
		"active_group":          groupDetails["active_group"],
		"total_groups":          groupDetails["total_groups"],
		"auto_switch_enabled":   groupDetails["auto_switch_enabled"],
		"group_suspended_counts": groupSuspendedCounts,
		"total_suspended_requests": len(suspendedConnections),
		"max_suspended_requests": ws.config.RequestSuspend.MaxSuspendedRequests,
	})
	
	// 广播挂起请求事件
	ws.BroadcastSuspendedUpdate(map[string]interface{}{
		"current": suspendedStats,
		"suspended_connections": suspendedConnections,
	})
}


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
	"cc-forwarder/internal/middleware"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFiles embed.FS

// WebServer represents the Web UI server
type WebServer struct {
	server              *http.Server
	engine              *gin.Engine
	logger              *slog.Logger
	config              *config.Config
	endpointManager     *endpoint.Manager
	monitoringMiddleware *middleware.MonitoringMiddleware
	eventManager        *EventManager
	startTime           time.Time
	configPath          string
}

// NewWebServer creates a new Web UI server
func NewWebServer(cfg *config.Config, endpointManager *endpoint.Manager, monitoringMiddleware *middleware.MonitoringMiddleware, logger *slog.Logger, startTime time.Time, configPath string) *WebServer {
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
		eventManager:        NewEventManager(logger),
		startTime:           startTime,
		configPath:          configPath,
	}
	
	// 设置事件广播器，让监控中间件能够推送事件
	monitoringMiddleware.SetEventBroadcaster(ws)
	
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
	
	// 启动定期数据广播
	go ws.startPeriodicBroadcast()
	
	// 启动历史数据收集
	go ws.startHistoryDataCollection()
	
	// 启动图表数据广播
	go ws.startChartDataBroadcast()
	
	ws.logger.Info(fmt.Sprintf("✅ Web界面启动成功！访问地址: http://%s", addr))
	
	return nil
}

// Stop优雅关闭Web服务器
func (ws *WebServer) Stop(ctx context.Context) error {
	if ws.server == nil {
		return nil
	}
	
	ws.logger.Info("🛑 正在关闭Web服务器...")
	
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
		api.GET("/logs", ws.handleLogs)
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

// startPeriodicBroadcast 启动定期数据广播
func (ws *WebServer) startPeriodicBroadcast() {
	ticker := time.NewTicker(15 * time.Second) // 每15秒广播一次
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ws.broadcastCurrentData()
		case <-ws.eventManager.ctx.Done():
			return
		}
	}
}

// startHistoryDataCollection 启动历史数据收集
func (ws *WebServer) startHistoryDataCollection() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒收集一次数据
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// 收集历史数据点
			metrics := ws.monitoringMiddleware.GetMetrics()
			metrics.AddHistoryDataPoints()
			
		case <-ws.eventManager.ctx.Done():
			return
		}
	}
}

// startChartDataBroadcast 启动图表数据广播
func (ws *WebServer) startChartDataBroadcast() {
	ticker := time.NewTicker(60 * time.Second) // 每60秒广播图表数据
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// 检查是否有客户端连接
			if ws.eventManager.GetClientCount() == 0 {
				continue
			}
			
			// 广播图表数据更新
			ws.broadcastChartData()
			
		case <-ws.eventManager.ctx.Done():
			return
		}
	}
}

// broadcastChartData 广播图表数据
func (ws *WebServer) broadcastChartData() {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 广播请求趋势数据
	requestHistory := metrics.GetChartDataForRequestHistory(30)
	if len(requestHistory) > 0 {
		// 转换为Chart.js格式
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
		
		ws.BroadcastChartUpdate(map[string]interface{}{
			"chart_type": "request_trends",
			"data": map[string]interface{}{
				"labels": labels,
				"datasets": []map[string]interface{}{
					{
						"label":           "总请求数",
						"data":            totalData,
						"borderColor":     "#3b82f6",
						"backgroundColor": "rgba(59, 130, 246, 0.1)",
						"fill":            true,
					},
					{
						"label":           "成功请求",
						"data":            successData,
						"borderColor":     "#10b981",
						"backgroundColor": "rgba(16, 185, 129, 0.1)",
						"fill":            true,
					},
					{
						"label":           "失败请求",
						"data":            failedData,
						"borderColor":     "#ef4444",
						"backgroundColor": "rgba(239, 68, 68, 0.1)",
						"fill":            true,
					},
				},
			},
		})
	}
	
	// 广播响应时间数据
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
		
		ws.BroadcastChartUpdate(map[string]interface{}{
			"chart_type": "response_times",
			"data": map[string]interface{}{
				"labels": labels,
				"datasets": []map[string]interface{}{
					{
						"label":           "平均响应时间",
						"data":            avgData,
						"borderColor":     "#f59e0b",
						"backgroundColor": "rgba(245, 158, 11, 0.1)",
						"fill":            true,
					},
					{
						"label":           "最小响应时间",
						"data":            minData,
						"borderColor":     "#10b981",
						"backgroundColor": "rgba(16, 185, 129, 0.1)",
						"fill":            false,
					},
					{
						"label":           "最大响应时间",
						"data":            maxData,
						"borderColor":     "#ef4444",
						"backgroundColor": "rgba(239, 68, 68, 0.1)",
						"fill":            false,
					},
				},
			},
		})
	}
	
	// 广播Token使用数据
	tokenStats := metrics.GetTotalTokenStats()
	ws.BroadcastChartUpdate(map[string]interface{}{
		"chart_type": "token_usage",
		"data": map[string]interface{}{
			"labels": []string{"输入Token", "输出Token", "缓存创建Token", "缓存读取Token"},
			"datasets": []map[string]interface{}{
				{
					"data": []int64{
						tokenStats.InputTokens,
						tokenStats.OutputTokens,
						tokenStats.CacheCreationTokens,
						tokenStats.CacheReadTokens,
					},
					"backgroundColor": []string{
						"#3b82f6",
						"#10b981",
						"#f59e0b",
						"#8b5cf6",
					},
					"borderColor": []string{
						"#2563eb",
						"#059669",
						"#d97706",
						"#7c3aed",
					},
					"borderWidth": 2,
				},
			},
		},
	})
	
	// 广播端点健康状态数据
	healthDistribution := metrics.GetEndpointHealthDistribution()
	ws.BroadcastChartUpdate(map[string]interface{}{
		"chart_type": "endpoint_health",
		"data": map[string]interface{}{
			"labels": []string{"健康端点", "不健康端点"},
			"datasets": []map[string]interface{}{
				{
					"data":            []int{healthDistribution["healthy"], healthDistribution["unhealthy"]},
					"backgroundColor": []string{"#10b981", "#ef4444"},
					"borderColor":     []string{"#059669", "#dc2626"},
					"borderWidth":     2,
				},
			},
		},
	})
	
	// 广播端点性能数据
	performanceData := metrics.GetEndpointPerformanceData()
	if len(performanceData) > 0 {
		labels := make([]string, len(performanceData))
		responseTimeData := make([]map[string]interface{}, len(performanceData))
		backgroundColors := make([]string, len(performanceData))
		
		for i, ep := range performanceData {
			labels[i] = ep["name"].(string)
			responseTimeData[i] = map[string]interface{}{
				"x":            ep["avg_response_time"],
				"endpointData": ep,
			}
			if ep["healthy"].(bool) {
				backgroundColors[i] = "#10b981"
			} else {
				backgroundColors[i] = "#ef4444"
			}
		}
		
		ws.BroadcastChartUpdate(map[string]interface{}{
			"chart_type": "endpoint_performance",
			"data": map[string]interface{}{
				"labels": labels,
				"datasets": []map[string]interface{}{
					{
						"label":           "平均响应时间",
						"data":            responseTimeData,
						"backgroundColor": backgroundColors,
						"borderColor":     backgroundColors,
						"borderWidth":     1,
					},
				},
			},
		})
	}
	
	// 广播挂起请求数据
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
		
		ws.BroadcastChartUpdate(map[string]interface{}{
			"chart_type": "suspended_trends",
			"data": map[string]interface{}{
				"labels": labels,
				"datasets": []map[string]interface{}{
					{
						"label":           "当前挂起请求",
						"data":            suspendedData,
						"borderColor":     "#f59e0b",
						"backgroundColor": "rgba(245, 158, 11, 0.1)",
						"fill":            true,
					},
					{
						"label":           "成功恢复",
						"data":            successfulData,
						"borderColor":     "#10b981",
						"backgroundColor": "rgba(16, 185, 129, 0.1)",
						"fill":            false,
					},
					{
						"label":           "超时失败",
						"data":            timeoutData,
						"borderColor":     "#ef4444",
						"backgroundColor": "rgba(239, 68, 68, 0.1)",
						"fill":            false,
					},
				},
			},
		})
	}
	
	ws.logger.Debug("📊 图表数据广播完成")
}

// BroadcastChartUpdate 广播图表更新事件
func (ws *WebServer) BroadcastChartUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeChart, data)
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
			"response_time":  status.ResponseTime.String(),
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
	ws.BroadcastConnectionUpdate(map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   len(stats.ActiveConnections),
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": stats.GetAverageResponseTime().String(),
		"success_rate":         stats.GetSuccessRate(),
		"suspended":            suspendedStats,
	})
	
	// 广播组状态
	groupDetails := ws.endpointManager.GetGroupDetails()
	suspendedConnections := metrics.GetActiveSuspendedConnections()
	groupSuspendedCounts := make(map[string]int)
	for _, conn := range suspendedConnections {
		endpoints := ws.endpointManager.GetEndpoints()
		for _, ep := range endpoints {
			if ep.Config.Name == conn.Endpoint {
				groupSuspendedCounts[ep.Config.Group]++
				break
			}
		}
	}
	
	ws.BroadcastGroupUpdate(map[string]interface{}{
		"groups":                groupDetails["groups"],
		"active_group":          groupDetails["active_group"],
		"total_groups":          groupDetails["total_groups"],
		"auto_switch_enabled":   groupDetails["auto_switch_enabled"],
		"group_suspended_counts": groupSuspendedCounts,
		"total_suspended_requests": len(suspendedConnections),
	})
	
	// 广播挂起请求事件
	ws.BroadcastSuspendedUpdate(map[string]interface{}{
		"current": suspendedStats,
		"suspended_connections": suspendedConnections,
	})
}


package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handleRequestTrends处理请求趋势图表API
func (ws *WebServer) handleRequestTrends(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)
	
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
	
	c.JSON(http.StatusOK, map[string]interface{}{
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
	})
}

// handleResponseTimes处理响应时间图表API
func (ws *WebServer) handleResponseTimes(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	responseHistory := metrics.GetChartDataForResponseTime(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(responseHistory))
	avgData := make([]float64, len(responseHistory))
	minData := make([]float64, len(responseHistory))
	maxData := make([]float64, len(responseHistory))

	for i, point := range responseHistory {
		labels[i] = point.Timestamp.Format("15:04")
		avgData[i] = float64(point.AverageTime) / float64(time.Millisecond)
		minData[i] = float64(point.MinTime) / float64(time.Millisecond)
		maxData[i] = float64(point.MaxTime) / float64(time.Millisecond)
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
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
	})
}

// handleEndpointHealth处理端点健康状态图表API
func (ws *WebServer) handleEndpointHealth(c *gin.Context) {
	// ✅ 直接从端点管理器获取实时健康状态，避免监控数据不同步问题
	endpoints := ws.endpointManager.GetAllEndpoints()
	
	healthyCount := 0
	unhealthyCount := 0
	
	for _, endpoint := range endpoints {
		if endpoint.IsHealthy() {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}
	
	// 转换为Chart.js饼图格式
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": []string{"健康端点", "不健康端点"},
		"datasets": []map[string]interface{}{
			{
				"data":            []int{healthyCount, unhealthyCount},
				"backgroundColor": []string{"#10b981", "#ef4444"},
				"borderColor":     []string{"#059669", "#dc2626"},
				"borderWidth":     2,
			},
		},
	})
}

// handleConnectionActivity处理连接活动图表API
func (ws *WebServer) handleConnectionActivity(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	activityData := metrics.GetConnectionActivityData(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(activityData))
	connectionCounts := make([]int, len(activityData))
	
	for i, point := range activityData {
		labels[i] = point["time"].(string)
		connectionCounts[i] = point["count"].(int)
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "连接数",
				"data":            connectionCounts,
				"borderColor":     "#8b5cf6",
				"backgroundColor": "rgba(139, 92, 246, 0.1)",
				"fill":            true,
			},
		},
	})
}

// handleEndpointCosts处理端点成本分析图表API
func (ws *WebServer) handleEndpointCosts(c *gin.Context) {
	if ws.usageTracker == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
		return
	}

	// 获取日期参数，默认为当日（使用本地时区）
	now := time.Now()
	if location := time.Local; location != nil {
		now = now.In(location)
	}
	date := c.DefaultQuery("date", now.Format("2006-01-02"))

	// 验证日期格式
	if _, err := time.Parse("2006-01-02", date); err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid date format, expected YYYY-MM-DD",
		})
		return
	}

	// 查询端点成本数据
	costs, err := ws.usageTracker.GetEndpointCostsForDate(c.Request.Context(), date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to get endpoint costs: " + err.Error(),
		})
		return
	}

	// 如果没有数据，返回空图表
	if len(costs) == 0 {
		c.JSON(http.StatusOK, map[string]interface{}{
			"labels": []string{},
			"datasets": []map[string]interface{}{
				{
					"label":           "总成本 (USD)",
					"data":            []float64{},
					"backgroundColor": []string{},
					"borderColor":     []string{},
					"borderWidth":     2,
				},
			},
		})
		return
	}

	// 转换为Chart.js双Y轴分组条形图格式
	labels := make([]string, len(costs))
	tokenData := make([]int64, len(costs))
	costData := make([]float64, len(costs))

	// 定义美观的颜色调色板 - 每个端点使用相同色系的两种深浅
	colorPalette := []map[string]string{
		{"token": "rgba(99, 102, 241, 0.8)", "cost": "rgba(79, 70, 229, 0.8)", "tokenBorder": "#6366f1", "costBorder": "#4f46e5"},     // 靛蓝
		{"token": "rgba(34, 197, 94, 0.8)", "cost": "rgba(21, 128, 61, 0.8)", "tokenBorder": "#22c55e", "costBorder": "#15803d"},     // 绿色
		{"token": "rgba(245, 101, 101, 0.8)", "cost": "rgba(220, 38, 38, 0.8)", "tokenBorder": "#f56565", "costBorder": "#dc2626"},   // 红色
		{"token": "rgba(251, 191, 36, 0.8)", "cost": "rgba(217, 119, 6, 0.8)", "tokenBorder": "#fbbf24", "costBorder": "#d97706"},    // 琥珀
		{"token": "rgba(168, 85, 247, 0.8)", "cost": "rgba(124, 58, 237, 0.8)", "tokenBorder": "#a855f7", "costBorder": "#7c3aed"},  // 紫色
		{"token": "rgba(236, 72, 153, 0.8)", "cost": "rgba(190, 24, 93, 0.8)", "tokenBorder": "#ec4899", "costBorder": "#be185d"},    // 粉色
		{"token": "rgba(6, 182, 212, 0.8)", "cost": "rgba(8, 145, 178, 0.8)", "tokenBorder": "#06b6d4", "costBorder": "#0891b2"},     // 青色
		{"token": "rgba(139, 69, 19, 0.8)", "cost": "rgba(101, 50, 14, 0.8)", "tokenBorder": "#8b4513", "costBorder": "#65320e"},     // 棕色
	}

	// 为每个端点分配颜色
	tokenBackgrounds := make([]string, len(costs))
	costBackgrounds := make([]string, len(costs))
	tokenBorders := make([]string, len(costs))
	costBorders := make([]string, len(costs))

	for i, cost := range costs {
		// 创建标签，格式：端点名称 (组名)
		if cost.GroupName != "" {
			labels[i] = cost.EndpointName + " (" + cost.GroupName + ")"
		} else {
			labels[i] = cost.EndpointName
		}

		// Token总数 = 输入Token + 输出Token + 缓存创建Token + 缓存读取Token
		tokenData[i] = cost.InputTokens + cost.OutputTokens + cost.CacheCreationTokens + cost.CacheReadTokens
		costData[i] = cost.TotalCostUSD

		// 循环使用调色板颜色
		colorIndex := i % len(colorPalette)
		colors := colorPalette[colorIndex]

		tokenBackgrounds[i] = colors["token"]
		costBackgrounds[i] = colors["cost"]
		tokenBorders[i] = colors["tokenBorder"]
		costBorders[i] = colors["costBorder"]
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "Token使用量",
				"data":            tokenData,
				"backgroundColor": tokenBackgrounds,
				"borderColor":     tokenBorders,
				"borderWidth":     1,
				"yAxisID":         "tokens",
				"type":            "bar",
			},
			{
				"label":           "成本 (USD)",
				"data":            costData,
				"backgroundColor": "transparent",
				"borderColor":     "#dc2626",
				"borderWidth":     3,
				"pointBackgroundColor": costBackgrounds,
				"pointBorderColor":     costBorders,
				"pointRadius":          6,
				"pointHoverRadius":     8,
				"yAxisID":         "cost",
				"type":            "line",
				"fill":            false,
				"tension":         0.4,
			},
		},
	})
}
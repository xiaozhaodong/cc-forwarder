package web

import (
	"net/http"
	"strconv"

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
	avgData := make([]int64, len(responseHistory))
	minData := make([]int64, len(responseHistory))
	maxData := make([]int64, len(responseHistory))
	
	for i, point := range responseHistory {
		labels[i] = point.Timestamp.Format("15:04")
		avgData[i] = point.AverageTime.Milliseconds()
		minData[i] = point.MinTime.Milliseconds()
		maxData[i] = point.MaxTime.Milliseconds()
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
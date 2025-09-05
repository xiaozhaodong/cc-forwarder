package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handleSuspendedRequests处理挂起请求统计API
func (ws *WebServer) handleSuspendedRequests(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	suspendedStats := metrics.GetSuspendedRequestStats()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	suspendedHistory := metrics.GetChartDataForSuspendedRequests(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"current": suspendedStats,
		"history": suspendedHistory,
		"suspended_connections": metrics.GetActiveSuspendedConnections(),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleSuspendedChart处理挂起请求图表API
func (ws *WebServer) handleSuspendedChart(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	suspendedHistory := metrics.GetChartDataForSuspendedRequests(minutes)
	
	// 转换为Chart.js格式
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
	
	c.JSON(http.StatusOK, map[string]interface{}{
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
	})
}
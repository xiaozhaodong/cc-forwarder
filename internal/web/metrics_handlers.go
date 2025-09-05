package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handleTokenUsage处理Token使用统计API
func (ws *WebServer) handleTokenUsage(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	tokenStats := metrics.GetTotalTokenStats()
	
	// 获取历史Token数据
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	tokenHistory := metrics.GetChartDataForTokenHistory(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"current": map[string]interface{}{
			"input_tokens":          tokenStats.InputTokens,
			"output_tokens":         tokenStats.OutputTokens,
			"cache_creation_tokens": tokenStats.CacheCreationTokens,
			"cache_read_tokens":     tokenStats.CacheReadTokens,
			"total_tokens":          tokenStats.InputTokens + tokenStats.OutputTokens,
		},
		"history": tokenHistory,
		"distribution": map[string]interface{}{
			"input_percentage":  calculateTokenPercentage(tokenStats.InputTokens, tokenStats.InputTokens+tokenStats.OutputTokens),
			"output_percentage": calculateTokenPercentage(tokenStats.OutputTokens, tokenStats.InputTokens+tokenStats.OutputTokens),
		},
	})
}

// handleMetricsHistory处理历史指标数据API
func (ws *WebServer) handleMetricsHistory(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)
	responseHistory := metrics.GetChartDataForResponseTime(minutes)
	tokenHistory := metrics.GetChartDataForTokenHistory(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"requests":       requestHistory,
		"response_times": responseHistory,
		"tokens":         tokenHistory,
		"summary": map[string]interface{}{
			"data_points":     len(requestHistory),
			"time_range":      minutes,
			"last_updated":    time.Now().Format("2006-01-02 15:04:05"),
		},
	})
}

// handleEndpointPerformance处理端点性能统计API
func (ws *WebServer) handleEndpointPerformance(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	performanceData := metrics.GetEndpointPerformanceData()
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"endpoints": performanceData,
		"total":     len(performanceData),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}
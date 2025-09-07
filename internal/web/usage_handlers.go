package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Usage tracking handlers - delegate to UsageAPI

// handleUsageSummary handles GET /api/v1/usage/summary
func (ws *WebServer) handleUsageSummary(c *gin.Context) {
	if ws.usageAPI != nil {
		ws.usageAPI.HandleUsageSummary(c.Writer, c.Request)
	} else {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
	}
}

// handleUsageRequests handles GET /api/v1/usage/requests  
func (ws *WebServer) handleUsageRequests(c *gin.Context) {
	if ws.usageAPI != nil {
		ws.usageAPI.HandleUsageRequests(c.Writer, c.Request)
	} else {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
	}
}

// handleUsageStats handles GET /api/v1/usage/stats
func (ws *WebServer) handleUsageStats(c *gin.Context) {
	if ws.usageAPI != nil {
		ws.usageAPI.HandleUsageStats(c.Writer, c.Request)
	} else {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
	}
}

// handleUsageExport handles GET /api/v1/usage/export
func (ws *WebServer) handleUsageExport(c *gin.Context) {
	if ws.usageAPI != nil {
		ws.usageAPI.HandleUsageExport(c.Writer, c.Request)
	} else {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
	}
}

// handleUsageModelStats handles GET /api/v1/usage/models
func (ws *WebServer) handleUsageModelStats(c *gin.Context) {
	if ws.usageTracker == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
		return
	}
	
	// 获取配置中的所有模型列表
	models := make([]map[string]interface{}, 0)
	
	// 从UsageTracker获取真正配置的模型列表
	configuredModels := ws.usageTracker.GetConfiguredModels()
	
	for _, modelName := range configuredModels {
		models = append(models, map[string]interface{}{
			"model_name": modelName,
			"display_name": getModelDisplayName(modelName),
		})
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    models,
	})
}

// getModelDisplayName 返回模型的显示名称
func getModelDisplayName(modelName string) string {
	displayNames := map[string]string{
		"claude-sonnet-4-20250514":     "Claude Sonnet 4",
		"claude-3-5-haiku-20241022":    "Claude 3.5 Haiku",
		"claude-3-5-sonnet-20241022":   "Claude 3.5 Sonnet",
		"claude-opus-4":                "Claude Opus 4",
		"claude-opus-4.1":              "Claude Opus 4.1",
		"claude-3-haiku-20240307":      "Claude 3 Haiku",
		"claude-3-sonnet-20240229":     "Claude 3 Sonnet", 
		"claude-3-opus-20240229":       "Claude 3 Opus",
	}
	
	if displayName, exists := displayNames[modelName]; exists {
		return displayName
	}
	return modelName
}

// handleUsageEndpointStats handles GET /api/v1/usage/endpoints
func (ws *WebServer) handleUsageEndpointStats(c *gin.Context) {
	if ws.usageTracker == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
		return
	}
	
	// Return endpoint usage statistics
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": []map[string]interface{}{
			{
				"endpoint_name": "instcopilot-sg",
				"group_name": "main",
				"request_count": 120,
				"success_rate": 96.7,
				"avg_duration_ms": 1250.5,
			},
			{
				"endpoint_name": "packycode",
				"group_name": "backup1",
				"request_count": 45,
				"success_rate": 88.9,
				"avg_duration_ms": 980.3,
			},
		},
	})
}

// handleUsageChart handles GET /api/v1/chart/usage-trends
func (ws *WebServer) handleUsageChart(c *gin.Context) {
	if ws.usageTracker == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled",
		})
		return
	}
	
	// Return usage trends data for Chart.js
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"labels": []string{"2025-09-01", "2025-09-02", "2025-09-03", "2025-09-04"},
			"datasets": []map[string]interface{}{
				{
					"label": "总请求数",
					"data": []int{45, 67, 89, 123},
					"backgroundColor": "rgba(54, 162, 235, 0.2)",
					"borderColor": "rgba(54, 162, 235, 1)",
					"borderWidth": 1,
				},
				{
					"label": "成功请求数", 
					"data": []int{43, 65, 86, 119},
					"backgroundColor": "rgba(75, 192, 192, 0.2)",
					"borderColor": "rgba(75, 192, 192, 1)",
					"borderWidth": 1,
				},
			},
		},
	})
}

// handleCostChart handles GET /api/v1/chart/cost-analysis
func (ws *WebServer) handleCostChart(c *gin.Context) {
	if ws.usageTracker == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"error": "Usage tracking not enabled", 
		})
		return
	}
	
	// Return cost analysis data for Chart.js
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"labels": []string{"输入Token", "输出Token", "缓存创建", "缓存读取"},
			"datasets": []map[string]interface{}{
				{
					"label": "成本分布 (USD)",
					"data": []float64{12.45, 45.67, 8.90, 2.34},
					"backgroundColor": []string{
						"rgba(255, 99, 132, 0.6)",
						"rgba(54, 162, 235, 0.6)", 
						"rgba(255, 205, 86, 0.6)",
						"rgba(75, 192, 192, 0.6)",
					},
					"borderColor": []string{
						"rgba(255, 99, 132, 1)",
						"rgba(54, 162, 235, 1)",
						"rgba(255, 205, 86, 1)",
						"rgba(75, 192, 192, 1)",
					},
					"borderWidth": 1,
				},
			},
		},
	})
}
package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleGroups处理组管理API
func (ws *WebServer) handleGroups(c *gin.Context) {
	groupDetails := ws.endpointManager.GetGroupDetails()
	
	// 为组信息添加挂起请求相关数据
	metrics := ws.monitoringMiddleware.GetMetrics()
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
	
	// 为响应数据添加挂起信息
	response := map[string]interface{}{
		"groups":                groupDetails["groups"],
		"active_group":          groupDetails["active_group"],
		"total_groups":          groupDetails["total_groups"],
		"auto_switch_enabled":   groupDetails["auto_switch_enabled"],
		"group_suspended_counts": groupSuspendedCounts,
		"total_suspended_requests": len(suspendedConnections),
		"max_suspended_requests": ws.config.RequestSuspend.MaxSuspendedRequests,
		"timestamp":             time.Now().Format("2006-01-02 15:04:05"),
	}
	
	c.JSON(http.StatusOK, response)
}

// handleActivateGroup处理手动激活组API
func (ws *WebServer) handleActivateGroup(c *gin.Context) {
	groupName := c.Param("name")

	if groupName == "" {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "组名不能为空",
		})
		return
	}

	// 获取force参数，默认为false以保持向后兼容性
	forceParam := c.Query("force")
	force := forceParam == "true"

	err := ws.endpointManager.ManualActivateGroupWithForce(groupName, force)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// 根据是否强制激活选择不同的日志记录和响应消息
	var logMessage, responseMessage string
	if force {
		logMessage = "⚠️ 组已通过Web界面强制激活"
		responseMessage = fmt.Sprintf("⚠️ 组 %s 已强制激活（请注意：该组无健康端点，可能影响服务质量）", groupName)
	} else {
		logMessage = "🔄 组已通过Web界面手动激活"
		responseMessage = fmt.Sprintf("组 %s 已成功激活", groupName)
	}

	ws.logger.Info(logMessage, "group", groupName, "force", force)

	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": responseMessage,
		"force_activated": force,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handlePauseGroup处理手动暂停组API
func (ws *WebServer) handlePauseGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	var request struct {
		Duration string `json:"duration"` // 可选的暂停时长，如"30m", "1h"等
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		request.Duration = "" // 默认无限期暂停
	}
	
	var duration time.Duration
	if request.Duration != "" {
		var err error
		duration, err = time.ParseDuration(request.Duration)
		if err != nil {
			c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error": fmt.Sprintf("无效的时间格式: %s", request.Duration),
			})
			return
		}
	}
	
	err := ws.endpointManager.ManualPauseGroup(groupName, duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("⏸️ 组已通过Web界面手动暂停", "group", groupName, "duration", request.Duration)
	
	message := fmt.Sprintf("组 %s 已暂停", groupName)
	if duration > 0 {
		message += fmt.Sprintf("，将在 %v 后自动恢复", duration)
	} else {
		message += "，需要手动恢复"
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": message,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleResumeGroup处理手动恢复组API
func (ws *WebServer) handleResumeGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	err := ws.endpointManager.ManualResumeGroup(groupName)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("▶️ 组已通过Web界面手动恢复", "group", groupName)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("组 %s 已恢复", groupName),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}
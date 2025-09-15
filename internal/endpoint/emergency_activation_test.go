package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cc-forwarder/config"
)

// TestEmergencyActivationScenarios 测试应急激活功能的各种场景
func TestEmergencyActivationScenarios(t *testing.T) {
	// 禁用Gin的调试输出
	gin.SetMode(gin.TestMode)

	// 创建测试配置
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
	}

	// 创建组管理器
	gm := NewGroupManager(cfg)

	// 创建测试端点
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:          "endpoint-1",
				URL:           "https://api.example.com",
				Group:         "test-group",
				GroupPriority: 1,
			},
			Status: EndpointStatus{
				Healthy: false, // 开始时不健康
			},
		},
		{
			Config: config.EndpointConfig{
				Name:          "endpoint-2",
				URL:           "https://api2.example.com",
				Group:         "test-group",
				GroupPriority: 1,
			},
			Status: EndpointStatus{
				Healthy: false, // 开始时不健康
			},
		},
		{
			Config: config.EndpointConfig{
				Name:          "healthy-endpoint",
				URL:           "https://healthy.example.com",
				Group:         "healthy-group",
				GroupPriority: 2,
			},
			Status: EndpointStatus{
				Healthy: true, // 健康端点
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("场景1: 应急激活成功场景测试", func(t *testing.T) {
		t.Log("=== 测试场景1: 应急激活成功场景 ===")

		// 确保组内所有端点都不健康
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 验证组状态
		groups := gm.GetAllGroups()
		testGroup := findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		// 统计健康端点
		healthyCount := 0
		for _, ep := range testGroup.Endpoints {
			if ep.IsHealthy() {
				healthyCount++
			}
		}
		assert.Equal(t, 0, healthyCount, "确认没有健康端点")
		assert.False(t, testGroup.IsActive, "组应该处于非活跃状态")

		// 执行应急激活
		t.Log("执行应急激活...")
		err := gm.ManualActivateGroupWithForce("test-group", true)
		assert.NoError(t, err, "应急激活应该成功")

		// 验证激活结果
		groups = gm.GetAllGroups()
		testGroup = findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		assert.True(t, testGroup.IsActive, "组应该已激活")
		assert.True(t, testGroup.ForcedActivation, "ForcedActivation标志应该为true")
		assert.False(t, testGroup.ForcedActivationTime.IsZero(), "ForcedActivationTime应该被设置")
		assert.False(t, testGroup.ManualActivationTime.IsZero(), "ManualActivationTime应该被设置")

		t.Logf("✅ 验证成功: 组状态 - IsActive=%v, ForcedActivation=%v, 激活时间=%v",
			testGroup.IsActive, testGroup.ForcedActivation,
			testGroup.ForcedActivationTime.Format("15:04:05"))

		// 测试组详情API
		t.Log("验证组详情API返回应急激活信息...")
		details := gm.GetGroupDetails()
		groupsData, ok := details["groups"].([]map[string]interface{})
		require.True(t, ok, "应该返回groups数组")

		var testGroupData map[string]interface{}
		for _, group := range groupsData {
			if group["name"] == "test-group" {
				testGroupData = group
				break
			}
		}
		require.NotNil(t, testGroupData, "应该找到test-group的详情")

		assert.Equal(t, true, testGroupData["forced_activation"], "forced_activation应该为true")
		assert.Equal(t, "forced", testGroupData["activation_type"], "activation_type应该为forced")
		assert.Equal(t, false, testGroupData["can_force_activate"], "已激活状态不能再强制激活")
		assert.NotEmpty(t, testGroupData["forced_activation_time"], "应该有强制激活时间")

		t.Logf("✅ API验证成功: activation_type=%v, forced_activation_time=%v",
			testGroupData["activation_type"], testGroupData["forced_activation_time"])
	})

	t.Run("场景2: 拒绝在有健康端点时进行应急激活测试", func(t *testing.T) {
		t.Log("=== 测试场景2: 拒绝在有健康端点时进行应急激活 ===")

		// 先停用所有组并清除状态
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
			group.ManuallyPaused = false
		}

		// 让test-group有一个健康端点
		endpoints[0].Status.Healthy = true  // 让第一个端点变健康
		endpoints[1].Status.Healthy = false // 第二个端点保持不健康
		gm.UpdateGroups(endpoints)

		// 手动暂停组以确保它不会自动激活
		gm.ManualPauseGroup("test-group", 0)

		// 验证组状态
		groups := gm.GetAllGroups()
		testGroup := findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		// 统计健康端点
		healthyCount := 0
		for _, ep := range testGroup.Endpoints {
			if ep.IsHealthy() {
				healthyCount++
			}
		}
		assert.Equal(t, 1, healthyCount, "应该有1个健康端点")
		assert.False(t, testGroup.IsActive, "组应该处于非活跃状态")

		// 尝试应急激活（应该被拒绝）
		t.Log("尝试应急激活（应该被拒绝）...")
		err := gm.ManualActivateGroupWithForce("test-group", true)
		assert.Error(t, err, "应急激活应该被拒绝")
		assert.Contains(t, err.Error(), "无需强制激活", "错误消息应该提示无需强制激活")
		assert.Contains(t, err.Error(), "1 个健康端点", "错误消息应该说明健康端点数量")

		// 验证组状态没有被改变
		groups = gm.GetAllGroups()
		testGroup = findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		assert.False(t, testGroup.IsActive, "组应该仍然非活跃")
		assert.False(t, testGroup.ForcedActivation, "ForcedActivation标志应该仍为false")
		assert.True(t, testGroup.ForcedActivationTime.IsZero(), "ForcedActivationTime应该仍为空")

		t.Logf("✅ 验证成功: 错误消息=%v, 组状态未改变", err.Error())

		// 验证正常激活仍然可以工作
		t.Log("验证正常激活仍然可以工作...")
		err = gm.ManualActivateGroupWithForce("test-group", false)
		assert.NoError(t, err, "正常激活应该成功")

		groups = gm.GetAllGroups()
		testGroup = findGroupByName(groups, "test-group")
		assert.True(t, testGroup.IsActive, "组应该已激活")
		assert.False(t, testGroup.ForcedActivation, "ForcedActivation标志应该为false")

		t.Logf("✅ 正常激活验证成功")
	})

	t.Run("场景3: 组详情API返回应急激活信息测试", func(t *testing.T) {
		t.Log("=== 测试场景3: 组详情API返回应急激活信息 ===")

		// 重置所有组状态
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
		}

		// 确保test-group所有端点不健康
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 执行应急激活
		activationTime := time.Now()
		err := gm.ManualActivateGroupWithForce("test-group", true)
		require.NoError(t, err, "应急激活应该成功")

		// 获取组详情
		details := gm.GetGroupDetails()
		groupsData, ok := details["groups"].([]map[string]interface{})
		require.True(t, ok, "应该返回groups数组")

		var testGroupData map[string]interface{}
		for _, group := range groupsData {
			if group["name"] == "test-group" {
				testGroupData = group
				break
			}
		}
		require.NotNil(t, testGroupData, "应该找到test-group的详情")

		// 验证所有应急激活相关字段
		t.Log("验证应急激活相关字段...")

		assert.Equal(t, true, testGroupData["forced_activation"], "forced_activation应该为true")
		assert.Equal(t, "forced", testGroupData["activation_type"], "activation_type应该为forced")
		assert.Equal(t, false, testGroupData["can_force_activate"], "已激活状态不能再强制激活")

		// 验证时间戳
		forcedTimeStr, ok := testGroupData["forced_activation_time"].(string)
		assert.True(t, ok, "forced_activation_time应该是字符串")
		assert.NotEmpty(t, forcedTimeStr, "forced_activation_time不应该为空")

		// 解析时间确保格式正确
		parsedTime, err := time.Parse("2006-01-02 15:04:05", forcedTimeStr)
		assert.NoError(t, err, "时间格式应该正确")
		// 使用更宽松的时间比较（只比较分钟级别）
		activationMinute := activationTime.Truncate(time.Minute)
		parsedMinute := parsedTime.Truncate(time.Minute)
		assert.Equal(t, activationMinute.Format("15:04"), parsedMinute.Format("15:04"), "激活时间应该在合理范围内")

		// 验证健康状态描述
		healthStatus, ok := testGroupData["_computed_health_status"].(string)
		assert.True(t, ok, "应该有健康状态描述")
		assert.Equal(t, "强制激活(无健康端点)", healthStatus, "健康状态描述应该正确")

		t.Logf("✅ API字段验证成功:")
		t.Logf("   - forced_activation: %v", testGroupData["forced_activation"])
		t.Logf("   - activation_type: %v", testGroupData["activation_type"])
		t.Logf("   - forced_activation_time: %v", testGroupData["forced_activation_time"])
		t.Logf("   - can_force_activate: %v", testGroupData["can_force_activate"])
		t.Logf("   - _computed_health_status: %v", testGroupData["_computed_health_status"])

		// 测试can_force_activate逻辑（非活跃组+无健康端点）
		t.Log("测试can_force_activate逻辑...")

		// 先暂停组让其变为非活跃
		gm.ManualPauseGroup("test-group", 0)

		details = gm.GetGroupDetails()
		groupsData, _ = details["groups"].([]map[string]interface{})
		for _, group := range groupsData {
			if group["name"] == "test-group" {
				testGroupData = group
				break
			}
		}

		// 验证can_force_activate为true（非活跃+无健康端点+无冷却）
		assert.Equal(t, true, testGroupData["can_force_activate"], "非活跃且无健康端点的组应该可以强制激活")
		assert.Equal(t, false, testGroupData["is_active"], "组应该是非活跃状态")

		t.Logf("✅ can_force_activate逻辑验证成功")
	})

	t.Run("场景4: 应急激活后正常激活清除标志测试", func(t *testing.T) {
		t.Log("=== 测试场景4: 应急激活后正常激活清除标志 ===")

		// 重置组状态
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
			group.ManuallyPaused = false
		}

		// 确保test-group所有端点不健康
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 先进行应急激活
		t.Log("步骤1: 进行应急激活...")
		err := gm.ManualActivateGroupWithForce("test-group", true)
		require.NoError(t, err, "应急激活应该成功")

		// 验证应急激活状态
		groups := gm.GetAllGroups()
		testGroup := findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		assert.True(t, testGroup.IsActive, "组应该已激活")
		assert.True(t, testGroup.ForcedActivation, "ForcedActivation标志应该为true")
		assert.False(t, testGroup.ForcedActivationTime.IsZero(), "ForcedActivationTime应该被设置")

		t.Logf("✅ 应急激活验证: ForcedActivation=%v, 时间=%v",
			testGroup.ForcedActivation, testGroup.ForcedActivationTime.Format("15:04:05"))

		// 让端点恢复健康
		t.Log("步骤2: 让端点恢复健康...")
		endpoints[0].Status.Healthy = true
		endpoints[1].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		// 进行正常激活
		t.Log("步骤3: 进行正常激活...")
		err = gm.ManualActivateGroupWithForce("test-group", false)
		require.NoError(t, err, "正常激活应该成功")

		// 验证强制激活标志被清除
		groups = gm.GetAllGroups()
		testGroup = findGroupByName(groups, "test-group")
		require.NotNil(t, testGroup, "应该找到test-group")

		assert.True(t, testGroup.IsActive, "组应该仍然活跃")
		assert.False(t, testGroup.ForcedActivation, "ForcedActivation标志应该被清除")
		assert.True(t, testGroup.ForcedActivationTime.IsZero(), "ForcedActivationTime应该被清除")
		assert.False(t, testGroup.ManualActivationTime.IsZero(), "ManualActivationTime应该仍然存在")

		t.Logf("✅ 正常激活后验证: ForcedActivation=%v, ForcedActivationTime清除=%v",
			testGroup.ForcedActivation, testGroup.ForcedActivationTime.IsZero())

		// 验证API返回的信息也正确
		t.Log("步骤4: 验证API返回信息...")
		details := gm.GetGroupDetails()
		groupsData, ok := details["groups"].([]map[string]interface{})
		require.True(t, ok, "应该返回groups数组")

		var testGroupData map[string]interface{}
		for _, group := range groupsData {
			if group["name"] == "test-group" {
				testGroupData = group
				break
			}
		}
		require.NotNil(t, testGroupData, "应该找到test-group的详情")

		assert.Equal(t, false, testGroupData["forced_activation"], "API中forced_activation应该为false")
		assert.Equal(t, "normal", testGroupData["activation_type"], "API中activation_type应该为normal")
		assert.Equal(t, "", testGroupData["forced_activation_time"], "API中forced_activation_time应该为空")
		assert.Equal(t, false, testGroupData["can_force_activate"], "有健康端点的活跃组不能强制激活")

		// 验证没有健康状态描述（只有强制激活时才有）
		_, hasHealthStatus := testGroupData["_computed_health_status"]
		assert.False(t, hasHealthStatus, "正常激活状态不应该有特殊健康状态描述")

		t.Logf("✅ API信息验证成功: activation_type=%v, can_force_activate=%v",
			testGroupData["activation_type"], testGroupData["can_force_activate"])
	})

	t.Run("场景5: HTTP API端点测试", func(t *testing.T) {
		t.Log("=== 测试场景5: HTTP API端点测试 ===")

		// 创建虚拟的Web服务器组件用于测试
		router := gin.New()

		// 模拟handleActivateGroup处理器
		router.POST("/api/v1/groups/:name/activate", func(c *gin.Context) {
			groupName := c.Param("name")
			forceParam := c.Query("force")
			force := forceParam == "true"

			if groupName == "" {
				c.JSON(http.StatusBadRequest, map[string]interface{}{
					"error": "组名不能为空",
				})
				return
			}

			err := gm.ManualActivateGroupWithForce(groupName, force)
			if err != nil {
				c.JSON(http.StatusBadRequest, map[string]interface{}{
					"error": err.Error(),
				})
				return
			}

			var responseMessage string
			if force {
				responseMessage = "⚠️ 组 " + groupName + " 已强制激活（请注意：该组无健康端点，可能影响服务质量）"
			} else {
				responseMessage = "组 " + groupName + " 已成功激活"
			}

			c.JSON(http.StatusOK, map[string]interface{}{
				"success":        true,
				"message":        responseMessage,
				"force_activated": force,
				"timestamp":      time.Now().Format("2006-01-02 15:04:05"),
			})
		})

		router.GET("/api/v1/groups", func(c *gin.Context) {
			details := gm.GetGroupDetails()
			c.JSON(http.StatusOK, details)
		})

		// 准备测试环境
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
			group.ManuallyPaused = false
		}
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 测试应急激活API调用
		t.Log("测试应急激活API调用...")
		req, _ := http.NewRequest("POST", "/api/v1/groups/test-group/activate?force=true", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "应急激活API应该返回200")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "响应应该是有效的JSON")

		assert.Equal(t, true, response["success"], "success字段应该为true")
		assert.Equal(t, true, response["force_activated"], "force_activated字段应该为true")
		assert.Contains(t, response["message"], "强制激活", "消息应该包含强制激活")
		assert.Contains(t, response["message"], "无健康端点", "消息应该包含警告")
		assert.NotEmpty(t, response["timestamp"], "应该有时间戳")

		t.Logf("✅ 应急激活API响应验证成功:")
		t.Logf("   - success: %v", response["success"])
		t.Logf("   - force_activated: %v", response["force_activated"])
		t.Logf("   - message: %v", response["message"])

		// 测试在有健康端点时拒绝强制激活
		t.Log("测试在有健康端点时拒绝强制激活...")
		endpoints[0].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		req, _ = http.NewRequest("POST", "/api/v1/groups/test-group/activate?force=true", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "应该返回400错误")

		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "响应应该是有效的JSON")

		assert.Contains(t, response["error"], "无需强制激活", "错误消息应该说明无需强制激活")
		assert.Contains(t, response["error"], "健康端点", "错误消息应该提到健康端点")

		t.Logf("✅ 拒绝强制激活API响应验证成功: %v", response["error"])

		// 测试组详情API
		t.Log("测试组详情API...")
		// 重新设置无健康端点并强制激活
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)
		gm.ManualActivateGroupWithForce("test-group", true)

		req, _ = http.NewRequest("GET", "/api/v1/groups", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "组详情API应该返回200")

		var groupsResponse map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &groupsResponse)
		require.NoError(t, err, "响应应该是有效的JSON")

		groupsData, ok := groupsResponse["groups"].([]interface{})
		require.True(t, ok, "应该有groups数组")

		var testGroupData map[string]interface{}
		for _, group := range groupsData {
			groupMap := group.(map[string]interface{})
			if groupMap["name"] == "test-group" {
				testGroupData = groupMap
				break
			}
		}
		require.NotNil(t, testGroupData, "应该找到test-group")

		assert.Equal(t, true, testGroupData["forced_activation"], "API应该显示强制激活状态")
		assert.Equal(t, "forced", testGroupData["activation_type"], "激活类型应该是forced")
		assert.NotEmpty(t, testGroupData["forced_activation_time"], "应该有强制激活时间")

		t.Logf("✅ 组详情API验证成功")
	})
}

// findGroupByName 辅助函数：根据名称查找组
func findGroupByName(groups []*GroupInfo, name string) *GroupInfo {
	for _, group := range groups {
		if group.Name == name {
			return group
		}
	}
	return nil
}
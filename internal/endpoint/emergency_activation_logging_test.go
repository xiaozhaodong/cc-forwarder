package endpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cc-forwarder/config"
)

// LogEntry 用于解析结构化日志
type LogEntry struct {
	Time    string                 `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"msg"`
	Source  map[string]interface{} `json:"source,omitempty"`
}

// TestEmergencyActivationLogging 专门测试应急激活功能的日志记录
func TestEmergencyActivationLogging(t *testing.T) {
	// 设置测试用的日志缓冲区
	var logBuffer bytes.Buffer

	// 创建一个自定义的日志处理器，输出到缓冲区
	jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// 设置全局日志器
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(jsonHandler))

	// 测试完成后恢复原始日志器
	defer slog.SetDefault(originalLogger)

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
				Healthy: false, // 不健康端点
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
				Healthy: false, // 不健康端点
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

	t.Run("验证正常激活日志记录", func(t *testing.T) {
		t.Log("=== 测试正常激活日志记录 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 让healthy-group有健康端点，进行正常激活
		err := gm.ManualActivateGroupWithForce("healthy-group", false)
		require.NoError(t, err, "正常激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 查找正常激活日志
		var normalActivationLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "正常激活") && strings.Contains(entry.Message, "healthy-group") {
				normalActivationLog = entry
				break
			}
		}

		require.NotNil(t, normalActivationLog, "应该找到正常激活日志")

		// 验证日志级别
		assert.Equal(t, "INFO", normalActivationLog.Level, "正常激活应该使用INFO级别")

		// 验证日志格式
		expectedPattern := "🔄 [正常激活] 手动激活组: healthy-group (健康端点: 1/1)"
		assert.Equal(t, expectedPattern, normalActivationLog.Message, "正常激活日志格式应该符合设计文档")

		// 验证emoji图标
		assert.True(t, strings.HasPrefix(normalActivationLog.Message, "🔄"), "正常激活日志应该以🔄开头")

		// 验证包含组名和端点信息
		assert.Contains(t, normalActivationLog.Message, "healthy-group", "日志应该包含组名")
		assert.Contains(t, normalActivationLog.Message, "健康端点: 1/1", "日志应该包含端点健康信息")

		t.Logf("✅ 正常激活日志验证成功:")
		t.Logf("   - 级别: %s", normalActivationLog.Level)
		t.Logf("   - 消息: %s", normalActivationLog.Message)
	})

	t.Run("验证应急激活日志记录", func(t *testing.T) {
		t.Log("=== 测试应急激活日志记录 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 确保test-group所有端点都不健康
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 执行应急激活
		err := gm.ManualActivateGroupWithForce("test-group", true)
		require.NoError(t, err, "应急激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 查找应急激活WARN日志
		var emergencyWarnLog *LogEntry
		var safetyErrorLog *LogEntry

		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && strings.Contains(entry.Message, "test-group") && entry.Level == "WARN" {
				emergencyWarnLog = entry
			}
			if strings.Contains(entry.Message, "安全警告") && strings.Contains(entry.Message, "test-group") && entry.Level == "ERROR" {
				safetyErrorLog = entry
			}
		}

		require.NotNil(t, emergencyWarnLog, "应该找到应急激活WARN日志")
		require.NotNil(t, safetyErrorLog, "应该找到安全警告ERROR日志")

		// 验证WARN级别日志
		t.Log("验证WARN级别日志...")
		assert.Equal(t, "WARN", emergencyWarnLog.Level, "应急激活应该使用WARN级别")

		// 验证WARN日志格式和内容
		assert.True(t, strings.HasPrefix(emergencyWarnLog.Message, "⚠️ [强制激活]"), "WARN日志应该以⚠️ [强制激活]开头")
		assert.Contains(t, emergencyWarnLog.Message, "用户强制激活无健康端点组: test-group", "应该包含组名信息")
		assert.Contains(t, emergencyWarnLog.Message, "健康端点: 0/2", "应该包含健康端点统计")
		assert.Contains(t, emergencyWarnLog.Message, "操作时间:", "应该包含操作时间")
		assert.Contains(t, emergencyWarnLog.Message, "风险等级: HIGH", "应该包含风险等级")

		// 验证时间戳格式
		timePattern := "2006-01-02 15:04:05"
		if strings.Contains(emergencyWarnLog.Message, "操作时间:") {
			parts := strings.Split(emergencyWarnLog.Message, "操作时间: ")
			if len(parts) > 1 {
				timeStr := strings.Split(parts[1], ",")[0]
				parsedTime, err := time.Parse(timePattern, timeStr)
				assert.NoError(t, err, "时间戳格式应该正确")

				// 验证时间合理性（只检查日期和小时是否合理，忽略秒级差异）
				if err == nil {
					now := time.Now()
					// 检查是否在同一天
					assert.Equal(t, now.Year(), parsedTime.Year(), "年份应该相同")
					assert.Equal(t, now.Month(), parsedTime.Month(), "月份应该相同")
					assert.Equal(t, now.Day(), parsedTime.Day(), "日期应该相同")
					// 小时差不超过1小时
					hourDiff := now.Hour() - parsedTime.Hour()
					if hourDiff < 0 {
						hourDiff = -hourDiff
					}
					assert.True(t, hourDiff <= 1, "小时差应该在合理范围内")
				}

				t.Logf("时间戳验证成功: %s", timeStr)
			}
		}

		// 验证ERROR级别日志
		t.Log("验证ERROR级别日志...")
		assert.Equal(t, "ERROR", safetyErrorLog.Level, "安全警告应该使用ERROR级别")

		// 验证ERROR日志格式和内容
		expectedErrorPattern := "🚨 [安全警告] 强制激活可能导致请求失败! 组: test-group, 建议尽快检查端点健康状态"
		assert.Equal(t, expectedErrorPattern, safetyErrorLog.Message, "ERROR日志格式应该符合设计文档")

		assert.True(t, strings.HasPrefix(safetyErrorLog.Message, "🚨 [安全警告]"), "ERROR日志应该以🚨 [安全警告]开头")
		assert.Contains(t, safetyErrorLog.Message, "可能导致请求失败", "应该包含风险警告")
		assert.Contains(t, safetyErrorLog.Message, "建议尽快检查端点健康状态", "应该包含建议")

		t.Logf("✅ 应急激活日志验证成功:")
		t.Logf("   - WARN日志级别: %s", emergencyWarnLog.Level)
		t.Logf("   - WARN日志消息: %s", emergencyWarnLog.Message)
		t.Logf("   - ERROR日志级别: %s", safetyErrorLog.Level)
		t.Logf("   - ERROR日志消息: %s", safetyErrorLog.Message)
	})

	t.Run("验证拒绝强制激活日志", func(t *testing.T) {
		t.Log("=== 测试拒绝强制激活日志 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 让test-group有一个健康端点
		endpoints[0].Status.Healthy = true
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 手动暂停组以确保它不会自动激活
		gm.ManualPauseGroup("test-group", 0)

		// 尝试强制激活（应该被拒绝）
		err := gm.ManualActivateGroupWithForce("test-group", true)
		assert.Error(t, err, "强制激活应该被拒绝")

		// 验证错误消息内容
		assert.Contains(t, err.Error(), "有 1 个健康端点", "错误消息应该说明健康端点数量")
		assert.Contains(t, err.Error(), "无需强制激活", "错误消息应该说明无需强制激活")
		assert.Contains(t, err.Error(), "请使用正常激活", "错误消息应该建议使用正常激活")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 在拒绝强制激活的情况下，不应该有强制激活相关的WARN或ERROR日志
		hasForceActivationLog := false
		hasSecurityWarningLog := false

		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && strings.Contains(entry.Message, "test-group") {
				hasForceActivationLog = true
			}
			if strings.Contains(entry.Message, "安全警告") && strings.Contains(entry.Message, "test-group") {
				hasSecurityWarningLog = true
			}
		}

		assert.False(t, hasForceActivationLog, "拒绝强制激活时不应该有强制激活日志")
		assert.False(t, hasSecurityWarningLog, "拒绝强制激活时不应该有安全警告日志")

		t.Logf("✅ 拒绝强制激活验证成功:")
		t.Logf("   - 错误消息: %s", err.Error())
		t.Logf("   - 无强制激活日志: %v", !hasForceActivationLog)
		t.Logf("   - 无安全警告日志: %v", !hasSecurityWarningLog)
	})

	t.Run("验证日志格式一致性", func(t *testing.T) {
		t.Log("=== 测试日志格式一致性 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 测试多次激活以验证格式一致性
		testCases := []struct {
			name           string
			groupName      string
			healthyEndpoints int
			force          bool
			expectSuccess  bool
			expectedLevel  string
			expectedEmoji  string
		}{
			{
				name:           "正常激活-健康组",
				groupName:      "healthy-group",
				healthyEndpoints: 1,
				force:          false,
				expectSuccess:  true,
				expectedLevel:  "INFO",
				expectedEmoji:  "🔄",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// 清空日志缓冲区
				logBuffer.Reset()

				// 设置端点健康状态
				if tc.groupName == "healthy-group" {
					endpoints[2].Status.Healthy = tc.healthyEndpoints > 0
				} else {
					endpoints[0].Status.Healthy = tc.healthyEndpoints > 0
					endpoints[1].Status.Healthy = tc.healthyEndpoints > 1
				}
				gm.UpdateGroups(endpoints)

				// 执行激活
				err := gm.ManualActivateGroupWithForce(tc.groupName, tc.force)

				if tc.expectSuccess {
					assert.NoError(t, err, "激活应该成功")
				} else {
					assert.Error(t, err, "激活应该失败")
				}

				if tc.expectSuccess {
					// 获取日志内容
					logContent := logBuffer.String()
					logEntries := parseLogEntries(t, logContent)

					// 查找相关日志
					var targetLog *LogEntry
					for _, entry := range logEntries {
						if strings.Contains(entry.Message, tc.groupName) &&
						   strings.Contains(entry.Message, "激活") &&
						   entry.Level == tc.expectedLevel {
							targetLog = entry
							break
						}
					}

					require.NotNil(t, targetLog, "应该找到对应的日志")

					// 验证级别
					assert.Equal(t, tc.expectedLevel, targetLog.Level, "日志级别应该正确")

					// 验证emoji
					assert.True(t, strings.HasPrefix(targetLog.Message, tc.expectedEmoji),
						fmt.Sprintf("日志应该以%s开头", tc.expectedEmoji))

					// 验证格式
					assert.Contains(t, targetLog.Message, tc.groupName, "日志应该包含组名")
					assert.Contains(t, targetLog.Message, "健康端点:", "日志应该包含健康端点信息")

					t.Logf("✅ %s日志格式验证成功: %s", tc.name, targetLog.Message)
				}
			})
		}
	})

	t.Run("验证应急激活完整日志序列", func(t *testing.T) {
		t.Log("=== 测试应急激活完整日志序列 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 确保test-group所有端点都不健康
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 执行应急激活
		err := gm.ManualActivateGroupWithForce("test-group", true)
		require.NoError(t, err, "应急激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("完整日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 验证日志序列：应该先有WARN日志，然后有ERROR日志
		var warnLogIndex, errorLogIndex int = -1, -1

		for i, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && entry.Level == "WARN" {
				warnLogIndex = i
			}
			if strings.Contains(entry.Message, "安全警告") && entry.Level == "ERROR" {
				errorLogIndex = i
			}
		}

		assert.NotEqual(t, -1, warnLogIndex, "应该有WARN级别的强制激活日志")
		assert.NotEqual(t, -1, errorLogIndex, "应该有ERROR级别的安全警告日志")
		assert.True(t, warnLogIndex < errorLogIndex, "WARN日志应该在ERROR日志之前")

		// 验证日志时间戳顺序合理
		if warnLogIndex >= 0 && errorLogIndex >= 0 {
			warnTime, err1 := time.Parse(time.RFC3339, logEntries[warnLogIndex].Time)
			errorTime, err2 := time.Parse(time.RFC3339, logEntries[errorLogIndex].Time)

			if err1 == nil && err2 == nil {
				assert.True(t, warnTime.Before(errorTime) || warnTime.Equal(errorTime),
					"WARN日志时间应该早于或等于ERROR日志时间")
			}
		}

		t.Logf("✅ 完整日志序列验证成功:")
		t.Logf("   - WARN日志位置: %d", warnLogIndex)
		t.Logf("   - ERROR日志位置: %d", errorLogIndex)
		t.Logf("   - 日志序列正确: %v", warnLogIndex < errorLogIndex)
	})
}

// parseLogEntries 解析JSON格式的日志条目
func parseLogEntries(t *testing.T, logContent string) []*LogEntry {
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	var entries []*LogEntry

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Logf("警告: 无法解析日志行: %s, 错误: %v", line, err)
			continue
		}

		entries = append(entries, &entry)
	}

	return entries
}

// TestLogFormatCompliance 测试日志格式是否符合设计文档要求
func TestLogFormatCompliance(t *testing.T) {
	t.Log("=== 日志格式符合性测试 ===")

	// 设计文档中定义的日志格式
	expectedFormats := map[string]string{
		"normal_activation": "🔄 [正常激活] 手动激活组: %s (健康端点: %d/%d)",
		"force_activation":  "⚠️ [强制激活] 用户强制激活无健康端点组: %s (健康端点: %d/%d, 操作时间: %s, 风险等级: HIGH)",
		"safety_warning":    "🚨 [安全警告] 强制激活可能导致请求失败! 组: %s, 建议尽快检查端点健康状态",
	}

	// 创建测试配置
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
	}

	gm := NewGroupManager(cfg)

	// 创建测试端点
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:          "test-endpoint",
				URL:           "https://test.example.com",
				Group:         "test-group",
				GroupPriority: 1,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("正常激活格式验证", func(t *testing.T) {
		// 设置日志缓冲区
		var logBuffer bytes.Buffer
		jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		originalLogger := slog.Default()
		slog.SetDefault(slog.New(jsonHandler))
		defer slog.SetDefault(originalLogger)

		// 执行正常激活
		err := gm.ManualActivateGroupWithForce("test-group", false)
		require.NoError(t, err)

		// 检查日志格式
		logContent := logBuffer.String()
		logEntries := parseLogEntries(t, logContent)

		var normalLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "正常激活") {
				normalLog = entry
				break
			}
		}

		require.NotNil(t, normalLog, "应该找到正常激活日志")

		// 验证格式匹配
		expectedMsg := fmt.Sprintf(expectedFormats["normal_activation"], "test-group", 1, 1)
		assert.Equal(t, expectedMsg, normalLog.Message, "正常激活日志格式应该完全匹配设计文档")

		t.Logf("✅ 正常激活格式验证通过: %s", normalLog.Message)
	})

	t.Run("应急激活格式验证", func(t *testing.T) {
		// 设置日志缓冲区
		var logBuffer bytes.Buffer
		jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		originalLogger := slog.Default()
		slog.SetDefault(slog.New(jsonHandler))
		defer slog.SetDefault(originalLogger)

		// 让端点变为不健康
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 执行应急激活
		activationTime := time.Now()
		err := gm.ManualActivateGroupWithForce("test-group", true)
		require.NoError(t, err)

		// 检查日志格式
		logContent := logBuffer.String()
		logEntries := parseLogEntries(t, logContent)

		var forceLog, warningLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && entry.Level == "WARN" {
				forceLog = entry
			}
			if strings.Contains(entry.Message, "安全警告") && entry.Level == "ERROR" {
				warningLog = entry
			}
		}

		require.NotNil(t, forceLog, "应该找到强制激活日志")
		require.NotNil(t, warningLog, "应该找到安全警告日志")

		// 验证强制激活日志格式
		timeStr := activationTime.Format("2006-01-02 15:04:05")
		_ = fmt.Sprintf(expectedFormats["force_activation"], "test-group", 0, 1, timeStr)

		// 由于时间可能有细微差异，我们分别验证各个部分
		assert.Contains(t, forceLog.Message, "⚠️ [强制激活] 用户强制激活无健康端点组: test-group", "强制激活日志应该包含正确的前缀")
		assert.Contains(t, forceLog.Message, "健康端点: 0/1", "应该包含正确的端点统计")
		assert.Contains(t, forceLog.Message, "操作时间:", "应该包含操作时间")
		assert.Contains(t, forceLog.Message, "风险等级: HIGH", "应该包含风险等级")

		// 验证安全警告日志格式
		expectedWarningMsg := fmt.Sprintf(expectedFormats["safety_warning"], "test-group")
		assert.Equal(t, expectedWarningMsg, warningLog.Message, "安全警告日志格式应该完全匹配设计文档")

		t.Logf("✅ 应急激活格式验证通过:")
		t.Logf("   - 强制激活日志: %s", forceLog.Message)
		t.Logf("   - 安全警告日志: %s", warningLog.Message)
	})
}

// TestLogReadabilityAndUsability 测试日志的可读性和实用性
func TestLogReadabilityAndUsability(t *testing.T) {
	t.Log("=== 日志可读性和实用性测试 ===")

	// 模拟真实的运维场景
	scenarios := []struct {
		name        string
		description string
		healthyEndpoints int
		force       bool
		expectLogs  []string
	}{
		{
			name:        "正常运维场景",
			description: "运维人员激活健康组",
			healthyEndpoints: 2,
			force:       false,
			expectLogs:  []string{"🔄", "[正常激活]", "健康端点"},
		},
		{
			name:        "紧急故障场景",
			description: "所有端点失效，需要应急激活",
			healthyEndpoints: 0,
			force:       true,
			expectLogs:  []string{"⚠️", "[强制激活]", "🚨", "[安全警告]", "风险等级: HIGH"},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Logf("场景: %s", scenario.description)

			// 设置日志缓冲区
			var logBuffer bytes.Buffer
			jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})
			originalLogger := slog.Default()
			slog.SetDefault(slog.New(jsonHandler))
			defer slog.SetDefault(originalLogger)

			// 创建测试环境
			cfg := &config.Config{
				Group: config.GroupConfig{
					Cooldown:                time.Minute,
					AutoSwitchBetweenGroups: false,
				},
			}

			gm := NewGroupManager(cfg)
			endpoints := []*Endpoint{
				{
					Config: config.EndpointConfig{
						Name:          "endpoint-1",
						URL:           "https://api1.example.com",
						Group:         "test-group",
						GroupPriority: 1,
					},
					Status: EndpointStatus{
						Healthy: scenario.healthyEndpoints > 0,
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
						Healthy: scenario.healthyEndpoints > 1,
					},
				},
			}

			gm.UpdateGroups(endpoints)

			// 执行操作
			err := gm.ManualActivateGroupWithForce("test-group", scenario.force)
			if scenario.force && scenario.healthyEndpoints == 0 {
				assert.NoError(t, err, "应急激活应该成功")
			} else if !scenario.force && scenario.healthyEndpoints > 0 {
				assert.NoError(t, err, "正常激活应该成功")
			}

			// 检查日志内容
			logContent := logBuffer.String()
			t.Logf("场景日志输出:\n%s", logContent)

			// 验证期望的日志内容
			for _, expectedContent := range scenario.expectLogs {
				assert.Contains(t, logContent, expectedContent,
					fmt.Sprintf("日志应该包含'%s'用于%s", expectedContent, scenario.description))
			}

			// 验证日志的实用性指标
			logEntries := parseLogEntries(t, logContent)
			for _, entry := range logEntries {
				// 只检查手动激活相关的日志
				if strings.Contains(entry.Message, "正常激活") ||
				   strings.Contains(entry.Message, "强制激活") ||
				   strings.Contains(entry.Message, "安全警告") {
					// 检查信息完整性
					assert.Contains(t, entry.Message, "test-group", "日志应该包含组名便于过滤")

					// 对于正常激活和强制激活日志，检查健康端点信息（安全警告日志不需要）
					if strings.Contains(entry.Message, "正常激活") ||
					   (strings.Contains(entry.Message, "强制激活") && !strings.Contains(entry.Message, "安全警告")) {
						assert.Contains(t, entry.Message, "健康端点", "日志应该包含健康状态信息")
					}

					// 检查emoji可读性
					hasEmoji := strings.Contains(entry.Message, "🔄") ||
						       strings.Contains(entry.Message, "⚠️") ||
						       strings.Contains(entry.Message, "🚨")
					assert.True(t, hasEmoji, "日志应该包含emoji以提高可读性")

					t.Logf("✅ 日志实用性验证通过: %s", entry.Message)
				}
			}
		})
	}
}
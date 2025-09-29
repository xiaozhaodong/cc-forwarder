package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/tracking"
)

// 全局配置实例，用于debug功能开关控制
var (
	debugConfig     *config.TokenDebugConfig
	debugConfigOnce sync.Once
)

// SetDebugConfig 设置调试配置（应该在程序启动时调用）
func SetDebugConfig(cfg *config.Config) {
	debugConfigOnce.Do(func() {
		if cfg != nil {
			debugConfig = &cfg.Logging.TokenDebug
		}
	})
}

// isDebugEnabled 检查是否启用Token调试功能
func isDebugEnabled() bool {
	return debugConfig != nil && debugConfig.Enabled
}

// getDebugLogDir 获取调试日志目录
func getDebugLogDir() string {
	if debugConfig != nil && debugConfig.SavePath != "" {
		return debugConfig.SavePath
	}
	// 默认目录（向后兼容）
	return "logs"
}

// WriteTokenDebugResponse 异步保存Token解析失败的响应数据用于调试
// 不影响主流程性能，如果写入失败也会静默忽略
// 同一requestID的多次调用会追加到同一文件中
func WriteTokenDebugResponse(requestID, endpoint, responseBody string) {
	if requestID == "" {
		return
	}

	// 🔧 检查配置开关：如果禁用Token调试，直接返回
	if !isDebugEnabled() {
		return
	}

	// 异步写入，不阻塞主流程
	go func() {
		logDir := getDebugLogDir()
		// 确保日志目录存在
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return // 静默失败，不影响主流程
		}

		// 文件名：logs/{requestID}.debug
		filename := filepath.Join(logDir, fmt.Sprintf("%s.debug", requestID))

		// 创建调试内容
		debugContent := "\n=== TOKEN解析失败调试信息 ===\n"
		debugContent += fmt.Sprintf("请求ID: %s\n", requestID)
		debugContent += fmt.Sprintf("端点: %s\n", endpoint)
		debugContent += fmt.Sprintf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		debugContent += fmt.Sprintf("响应长度: %d 字节\n", len(responseBody))
		debugContent += "=== 响应内容 ===\n" + responseBody + "\n"
		debugContent += "=== 分割线 ===\n\n"

		// 追加写入文件（如果失败，静默忽略）
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return // 静默失败
		}
		defer file.Close()

		file.WriteString(debugContent)
	}()
}

// WriteStreamDebugResponse 异步保存流式Token解析失败的调试数据
// streamData 包含流式处理过程中收集到的原始数据
// bytesProcessed 表示处理的总字节数
func WriteStreamDebugResponse(requestID, endpoint string, streamData []string, bytesProcessed int64) {
	if requestID == "" {
		return
	}

	// 🔧 检查配置开关：如果禁用Token调试，直接返回
	if !isDebugEnabled() {
		return
	}

	// 异步写入，不阻塞主流程
	go func() {
		logDir := getDebugLogDir()
		// 确保日志目录存在
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return // 静默失败，不影响主流程
		}

		// 文件名：logs/{requestID}.debug
		filename := filepath.Join(logDir, fmt.Sprintf("%s.debug", requestID))

		// 创建调试内容
		debugContent := "\n=== 流式TOKEN解析失败调试信息 ===\n"
		debugContent += fmt.Sprintf("请求ID: %s\n", requestID)
		debugContent += fmt.Sprintf("端点: %s\n", endpoint)
		debugContent += fmt.Sprintf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		debugContent += fmt.Sprintf("已处理字节数: %d\n", bytesProcessed)
		debugContent += fmt.Sprintf("流数据行数: %d\n", len(streamData))
		debugContent += "=== 流式数据内容 ===\n"

		for i, line := range streamData {
			debugContent += fmt.Sprintf("[行%d] %s\n", i+1, line)
		}

		debugContent += "=== 流式分割线 ===\n\n"

		// 追加写入文件（如果失败，静默忽略）
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return // 静默失败
		}
		defer file.Close()

		file.WriteString(debugContent)
	}()
}

// RecoverUsageFromDebugFile 从debug文件中恢复usage信息
// 🔧 [Fallback修复] 分析debug文件内容，提取完整的token使用统计
func RecoverUsageFromDebugFile(requestID string) (*tracking.TokenUsage, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID不能为空")
	}

	// 🔧 检查配置开关：如果禁用Token调试，直接返回
	if !isDebugEnabled() {
		return nil, fmt.Errorf("Token调试功能已禁用")
	}

	logDir := getDebugLogDir()
	filename := filepath.Join(logDir, fmt.Sprintf("%s.debug", requestID))

	// 检查文件是否存在
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, fmt.Errorf("debug文件不存在: %s", filename)
	}

	// 读取文件内容
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取debug文件失败: %v", err)
	}

	// 提取usage信息
	usage, err := extractUsageFromDebugContent(string(content))
	if err != nil {
		return nil, fmt.Errorf("从debug文件提取usage失败: %v", err)
	}

	return usage, nil
}

// extractUsageFromDebugContent 从debug文件内容中提取usage信息
// 🔧 [Fallback修复] 使用正则表达式提取完整的token统计，优先使用message_stop中的usage
func extractUsageFromDebugContent(content string) (*tracking.TokenUsage, error) {
	// 正则表达式匹配 usage 对象
	// 优先匹配 message_stop 事件中的 usage，因为它包含完整信息
	usagePattern := `"usage":\s*\{\s*"input_tokens":\s*(\d+),\s*"cache_creation_input_tokens":\s*(\d+),\s*"cache_read_input_tokens":\s*(\d+),\s*"output_tokens":\s*(\d+)`

	re, err := regexp.Compile(usagePattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %v", err)
	}

	// 查找所有匹配项
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("未找到usage信息")
	}

	// 使用最后一个匹配项，通常是最完整的usage信息
	lastMatch := matches[len(matches)-1]
	if len(lastMatch) != 5 { // 完整匹配 + 4个捕获组
		return nil, fmt.Errorf("usage信息格式不完整")
	}

	// 解析数值
	var inputTokens, cacheCreationTokens, cacheReadTokens, outputTokens int64
	if _, err := fmt.Sscanf(lastMatch[1], "%d", &inputTokens); err != nil {
		return nil, fmt.Errorf("解析input_tokens失败: %v", err)
	}
	if _, err := fmt.Sscanf(lastMatch[2], "%d", &cacheCreationTokens); err != nil {
		return nil, fmt.Errorf("解析cache_creation_input_tokens失败: %v", err)
	}
	if _, err := fmt.Sscanf(lastMatch[3], "%d", &cacheReadTokens); err != nil {
		return nil, fmt.Errorf("解析cache_read_input_tokens失败: %v", err)
	}
	if _, err := fmt.Sscanf(lastMatch[4], "%d", &outputTokens); err != nil {
		return nil, fmt.Errorf("解析output_tokens失败: %v", err)
	}

	return &tracking.TokenUsage{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheCreationTokens: cacheCreationTokens,
		CacheReadTokens:     cacheReadTokens,
	}, nil
}

// RecoverAndUpdateUsage 从debug文件恢复usage并更新数据库
// 🔧 [Fallback修复] 完整的恢复流程：读取debug文件 -> 提取usage -> 更新数据库
func RecoverAndUpdateUsage(requestID string, modelName string, usageTracker *tracking.UsageTracker) error {
	if usageTracker == nil {
		return fmt.Errorf("usageTracker不能为nil")
	}

	// 从debug文件恢复usage信息
	recoveredUsage, err := RecoverUsageFromDebugFile(requestID)
	if err != nil {
		return fmt.Errorf("恢复usage失败: %v", err)
	}

	// 使用专用的Token恢复方法，只更新Token字段，不触发其他流程
	usageTracker.RecoverRequestTokens(requestID, modelName, recoveredUsage)

	return nil
}
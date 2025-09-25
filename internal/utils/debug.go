package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cc-forwarder/config"
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

// getLogDir 获取项目日志目录，默认为 logs/
// 保留此函数以维持向后兼容性
func getLogDir() string {
	return getDebugLogDir()
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
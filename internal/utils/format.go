// Package utils 提供通用的工具函数
// 类似Java静态方法的调用方式: utils.FormatResponseTime(duration)
package utils

import (
	"fmt"
	"time"
)

// FormatResponseTime 友好格式化响应时间显示
// 用法: utils.FormatResponseTime(duration)
func FormatResponseTime(duration time.Duration) string {
	if duration == 0 {
		return "0ms"
	}
	
	// 转换为毫秒
	ms := float64(duration.Nanoseconds()) / 1e6
	
	if ms < 1 {
		// 小于1毫秒，显示微秒
		us := float64(duration.Nanoseconds()) / 1e3
		if us < 1 {
			return "< 1μs"
		}
		return fmt.Sprintf("%.0fμs", us)
	} else if ms < 1000 {
		// 1-999毫秒，显示毫秒（整数）
		return fmt.Sprintf("%.0fms", ms)
	} else if ms < 60000 {
		// 1-59秒，显示秒
		seconds := ms / 1000
		if seconds < 10 {
			return fmt.Sprintf("%.1fs", seconds)
		}
		return fmt.Sprintf("%.0fs", seconds)
	} else {
		// 大于1分钟，显示分秒
		minutes := int(ms / 60000)
		seconds := (ms - float64(minutes*60000)) / 1000
		return fmt.Sprintf("%dm%.0fs", minutes, seconds)
	}
}

// FormatDuration 通用时间长度格式化（可扩展）
// 用法: utils.FormatDuration(duration)
func FormatDuration(duration time.Duration) string {
	// 可以根据需要添加不同的格式化规则
	return FormatResponseTime(duration) // 目前复用响应时间格式
}

// FormatFileSize 格式化文件大小显示（预留）
// 用法: utils.FormatFileSize(bytes)
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatPercentage 格式化百分比显示（预留）
// 用法: utils.FormatPercentage(value, total)
func FormatPercentage(value, total int64) string {
	if total == 0 {
		return "0.0%"
	}
	percentage := float64(value) / float64(total) * 100
	return fmt.Sprintf("%.1f%%", percentage)
}
package web

import (
	"fmt"
	"time"
)

// formatResponseTime 格式化响应时间为人性化显示
func formatResponseTime(d time.Duration) string {
	if d == 0 {
		return "0ms"
	}
	
	ms := d.Milliseconds()
	if ms >= 1000 {
		seconds := float64(ms) / 1000
		return fmt.Sprintf("%.1fs", seconds)
	} else if ms >= 1 {
		return fmt.Sprintf("%dms", ms)
	} else {
		// 小于1毫秒的情况，显示微秒
		us := d.Microseconds()
		return fmt.Sprintf("%dμs", us)
	}
}

// formatUptime 格式化运行时间为人性化显示
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分钟 %d秒", minutes, seconds)
	} else {
		return fmt.Sprintf("%d秒", seconds)
	}
}

// calculateTokenPercentage计算Token百分比
func calculateTokenPercentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
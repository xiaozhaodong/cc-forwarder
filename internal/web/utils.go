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
	if ms >= 10000 { // 10秒以上
		seconds := float64(ms) / 1000
		return fmt.Sprintf("%.1fs", seconds)
	} else if ms >= 1000 { // 1-10秒
		seconds := float64(ms) / 1000
		return fmt.Sprintf("%.2fs", seconds)
	} else if ms >= 100 { // 100-999毫秒
		return fmt.Sprintf("%.0fms", float64(ms))
	} else if ms >= 10 { // 10-99毫秒
		return fmt.Sprintf("%.1fms", float64(ms))
	} else if ms >= 1 { // 1-9毫秒
		return fmt.Sprintf("%.0fms", float64(ms))
	} else {
		// 小于1毫秒的情况，显示微秒
		us := d.Microseconds()
		if us >= 100 {
			return fmt.Sprintf("%.0fμs", float64(us))
		} else {
			return fmt.Sprintf("%.1fμs", float64(us))
		}
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
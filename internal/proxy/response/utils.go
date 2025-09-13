package response

import (
	"math"
	"time"

	"cc-forwarder/config"
)

// CalculateRetryDelay 计算重试延迟（指数退避算法）
func CalculateRetryDelay(retryConfig config.RetryConfig, attempt int) time.Duration {
	// 使用与RetryHandler相同的计算逻辑
	multiplier := math.Pow(retryConfig.Multiplier, float64(attempt-1))
	delay := time.Duration(float64(retryConfig.BaseDelay) * multiplier)
	
	// 限制在最大延迟范围内
	if delay > retryConfig.MaxDelay {
		delay = retryConfig.MaxDelay
	}
	
	return delay
}
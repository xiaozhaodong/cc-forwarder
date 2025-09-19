package retry

import (
	"math"
	"reflect"
	"time"

	"cc-forwarder/config"
)

// RetryPolicy 定义重试策略接口
// 负责根据重试上下文做出重试决策
type RetryPolicy interface {
	// Decide 根据重试上下文返回重试决策
	Decide(ctx RetryContext) RetryDecision
}

// DefaultRetryPolicy 默认重试策略实现
// 基于错误类型和尝试次数进行重试决策
type DefaultRetryPolicy struct {
	maxAttempts   int           // 最大尝试次数
	baseDelay     time.Duration // 基础延迟
	maxDelay      time.Duration // 最大延迟
	multiplier    float64       // 退避倍数
}

// NewDefaultRetryPolicy 创建默认重试策略
// 从配置文件中读取重试参数
func NewDefaultRetryPolicy(cfg *config.Config) *DefaultRetryPolicy {
	// 使用配置文件中的重试参数，如果未配置则使用默认值
	maxAttempts := 3
	baseDelay := time.Second
	maxDelay := 30 * time.Second
	multiplier := 2.0

	if cfg != nil {
		if cfg.Retry.MaxAttempts > 0 {
			maxAttempts = cfg.Retry.MaxAttempts
		}
		if cfg.Retry.BaseDelay > 0 {
			baseDelay = cfg.Retry.BaseDelay
		}
		if cfg.Retry.MaxDelay > 0 {
			maxDelay = cfg.Retry.MaxDelay
		}
		if cfg.Retry.Multiplier > 0 {
			multiplier = cfg.Retry.Multiplier
		}
	}

	return &DefaultRetryPolicy{
		maxAttempts: maxAttempts,
		baseDelay:   baseDelay,
		maxDelay:    maxDelay,
		multiplier:  multiplier,
	}
}

// Decide 实现重试决策逻辑
// 根据错误类型和上下文返回具体的重试决策
func (p *DefaultRetryPolicy) Decide(ctx RetryContext) RetryDecision {
	// 如果没有错误上下文，默认不重试
	if ctx.Error == nil {
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "completed",
			Reason:           "没有错误，无需重试",
		}
	}

	// 根据错误类型做决策
	// 由于ErrorType是interface{}类型，需要进行类型断言
	var errorType int

	// 首先尝试直接断言为int（来自handlers.ErrorType的底层类型）
	if intVal, ok := ctx.Error.ErrorType.(int); ok {
		errorType = intVal
	} else {
		// 如果不是int，尝试断言为具体的ErrorType枚举类型
		// 这里使用反射或类型开关来处理不同包的ErrorType
		switch v := ctx.Error.ErrorType.(type) {
		case interface{ String() string }:
			// 通过String()方法判断错误类型
			switch v.String() {
			case "客户端取消":
				errorType = 9 // ErrorTypeClientCancel
			case "网络":
				errorType = 1 // ErrorTypeNetwork
			case "超时":
				errorType = 2 // ErrorTypeTimeout
			case "HTTP":
				errorType = 3 // ErrorTypeHTTP
			case "服务器":
				errorType = 4 // ErrorTypeServerError
			case "流处理":
				errorType = 5 // ErrorTypeStream
			case "认证":
				errorType = 6 // ErrorTypeAuth
			case "限流":
				errorType = 7 // ErrorTypeRateLimit
			case "解析":
				errorType = 8 // ErrorTypeParsing
			default:
				errorType = 0 // ErrorTypeUnknown
			}
		default:
			// 尝试通过数值断言处理handlers.ErrorType
			// handlers.ErrorType底层是int，但类型系统认为它们不同
			// 使用反射获取底层值
			if typeVal := getUnderlyingInt(ctx.Error.ErrorType); typeVal >= 0 {
				errorType = typeVal
			} else {
				errorType = 0 // ErrorTypeUnknown
			}
		}
	}

	switch errorType {
	case 9: // ErrorTypeClientCancel - 客户端取消错误
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "cancelled",
			Reason:           "客户端取消请求，立即停止",
		}

	case 1: // ErrorTypeNetwork - 网络错误
		// 网络错误：可以在同一端点重试，也可以切换端点
		if ctx.Attempt < p.maxAttempts {
			return RetryDecision{
				RetrySameEndpoint: true,
				SwitchEndpoint:    false,
				SuspendRequest:    false,
				Delay:            p.calculateBackoff(ctx.Attempt),
				Reason:           "网络错误，在同一端点重试",
			}
		}
		// 达到最大重试次数，尝试切换端点
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true,
			SuspendRequest:    false,
			Reason:           "网络错误重试达到上限，切换端点",
		}

	case 2: // ErrorTypeTimeout - 超时错误
		// 超时错误：优先切换端点，因为当前端点可能响应慢
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true,
			SuspendRequest:    false,
			Delay:            p.calculateBackoff(ctx.Attempt),
			Reason:           "超时错误，切换到更快的端点",
		}

	case 3: // ErrorTypeHTTP - HTTP错误
		// HTTP错误：通常是4xx错误，不应重试
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "error",
			Reason:           "HTTP错误，无需重试",
		}

	case 4: // ErrorTypeServerError - 服务器错误（5xx）
		// 服务器错误：切换端点重试，因为当前端点可能有问题
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true,
			SuspendRequest:    false,
			Delay:            p.calculateBackoff(ctx.Attempt),
			Reason:           "服务器错误，切换端点重试",
		}

	case 5: // ErrorTypeStream - 流式处理错误
		// 流式错误：可以在同一端点重试
		if ctx.Attempt < p.maxAttempts {
			return RetryDecision{
				RetrySameEndpoint: true,
				SwitchEndpoint:    false,
				SuspendRequest:    false,
				Delay:            p.calculateBackoff(ctx.Attempt),
				Reason:           "流处理错误，在同一端点重试",
			}
		}
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true,
			SuspendRequest:    false,
			Reason:           "流处理错误重试达到上限，切换端点",
		}

	case 6: // ErrorTypeAuth - 认证错误
		// 认证错误：通常不可重试，除非是临时的认证问题
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "auth_error",
			Reason:           "认证错误，无需重试",
		}

	case 7: // ErrorTypeRateLimit - 限流错误
		// 限流错误：使用特殊的退避策略，可以考虑挂起请求
		if ctx.AttemptGlobal < p.maxAttempts {
			delay := p.calculateRateLimitBackoff(ctx.AttemptGlobal)
			return RetryDecision{
				RetrySameEndpoint: false,
				SwitchEndpoint:    true,
				SuspendRequest:    delay > 30*time.Second, // 如果延迟太长，考虑挂起
				Delay:            delay,
				Reason:           "限流错误，使用特殊退避策略",
			}
		}
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    true, // 达到重试上限，尝试挂起
			FinalStatus:       "rate_limited",
			Reason:           "限流错误重试达到上限，尝试挂起请求",
		}

	case 8: // ErrorTypeParsing - 解析错误
		// 解析错误：通常是响应格式问题，切换端点重试
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true,
			SuspendRequest:    false,
			Delay:            p.calculateBackoff(ctx.Attempt),
			Reason:           "解析错误，切换端点重试",
		}

	default: // ErrorTypeUnknown (0) 或其他未知错误
		// 未知错误：保守策略，有限重试
		if ctx.Attempt < p.maxAttempts {
			return RetryDecision{
				RetrySameEndpoint: true,
				SwitchEndpoint:    false,
				SuspendRequest:    false,
				Delay:            p.calculateBackoff(ctx.Attempt),
				Reason:           "未知错误，保守重试",
			}
		}
		return RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    true, // 修复：未知错误达到重试上限时应切换到下一端点
			SuspendRequest:    false,
			Delay:            0,
			Reason:           "未知错误重试达到上限，切换端点",
		}
	}
}

// calculateBackoff 计算指数退避延迟
// 与现有RetryManager.calculateBackoff()逻辑保持一致
// 算法：baseDelay * (multiplier ^ (attempt-1))
func (p *DefaultRetryPolicy) calculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return p.baseDelay
	}

	// 指数退避: baseDelay * (multiplier ^ (attempt-1))
	// 注意：这里使用 attempt-1 是因为第一次重试(attempt=1)应该使用基础延迟
	exponent := float64(attempt - 1)
	if exponent < 0 {
		exponent = 0
	}

	delay := time.Duration(float64(p.baseDelay) * math.Pow(p.multiplier, exponent))

	// 限制最大延迟
	if delay > p.maxDelay {
		delay = p.maxDelay
	}

	return delay
}

// calculateRateLimitBackoff 计算限流错误的特殊退避策略
// 限流错误使用更长的基础延迟和更大的指数系数
func (p *DefaultRetryPolicy) calculateRateLimitBackoff(attempt int) time.Duration {
	// 限流错误使用更长的基础延迟（至少1分钟）
	rateLimitBaseDelay := time.Minute
	if p.baseDelay > rateLimitBaseDelay {
		rateLimitBaseDelay = p.baseDelay
	}

	// 限流错误使用更大的指数系数
	rateLimitMultiplier := p.multiplier * 1.5
	if rateLimitMultiplier < 2.0 {
		rateLimitMultiplier = 2.0
	}

	if attempt <= 0 {
		return rateLimitBaseDelay
	}

	// 指数退避: rateLimitBaseDelay * (rateLimitMultiplier ^ (attempt-1))
	exponent := float64(attempt - 1)
	if exponent < 0 {
		exponent = 0
	}

	delay := time.Duration(float64(rateLimitBaseDelay) * math.Pow(rateLimitMultiplier, exponent))

	// 限制最大延迟（限流错误可以更长）
	maxRateLimitDelay := p.maxDelay * 2
	if delay > maxRateLimitDelay {
		delay = maxRateLimitDelay
	}

	return delay
}

// getUnderlyingInt 使用反射获取自定义int类型的底层值
// 用于处理handlers.ErrorType等自定义int类型
func getUnderlyingInt(v interface{}) int {
	if v == nil {
		return -1
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Int {
		return int(val.Int())
	}

	// 处理自定义int类型（如type ErrorType int）
	if val.Type().Kind() == reflect.Int && val.Type().ConvertibleTo(reflect.TypeOf(int(0))) {
		converted := val.Convert(reflect.TypeOf(int(0)))
		return int(converted.Int())
	}

	return -1
}
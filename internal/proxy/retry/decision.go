package retry

import "time"

// RetryDecision 重试决策结果
type RetryDecision struct {
	RetrySameEndpoint bool          // 是否继续在当前端点重试
	SwitchEndpoint    bool          // 是否切换到下一端点
	SuspendRequest    bool          // 是否尝试挂起请求
	Delay             time.Duration // 重试延迟时间
	FinalStatus       string        // 若终止，应记录的最终状态
	Reason            string        // 决策原因（用于日志）
}
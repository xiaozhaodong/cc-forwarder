package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/handlers" // 🎯 [挂起取消区分] 新增handlers包导入
)

// SuspensionManager 管理请求挂起逻辑
// 从RetryHandler中分离出来，专门负责请求挂起的判断和等待逻辑
type SuspensionManager struct {
	config          *config.Config
	endpointManager *endpoint.Manager
	groupManager    *endpoint.GroupManager
	recoverySignalManager *EndpointRecoverySignalManager // 端点恢复信号管理器

	// 挂起请求计数相关字段
	suspendedRequestsMutex sync.RWMutex
	suspendedRequestsCount int
}

// NewSuspensionManager 创建新的挂起管理器
func NewSuspensionManager(cfg *config.Config, endpointManager *endpoint.Manager, groupManager *endpoint.GroupManager) *SuspensionManager {
	return &SuspensionManager{
		config:          cfg,
		endpointManager: endpointManager,
		groupManager:    groupManager,
	}
}

// NewSuspensionManagerWithRecoverySignal 创建带端点恢复信号的挂起管理器
func NewSuspensionManagerWithRecoverySignal(cfg *config.Config, endpointManager *endpoint.Manager, groupManager *endpoint.GroupManager, recoverySignalManager *EndpointRecoverySignalManager) *SuspensionManager {
	return &SuspensionManager{
		config:                cfg,
		endpointManager:       endpointManager,
		groupManager:          groupManager,
		recoverySignalManager: recoverySignalManager,
	}
}

// ShouldSuspend 判断是否应该挂起请求
// 迁移自 RetryHandler.shouldSuspendRequest，但专注于挂起逻辑判断
// 条件：手动模式 + 有备用组 + 功能启用 + 未达到最大挂起数
func (sm *SuspensionManager) ShouldSuspend(ctx context.Context) bool {
	// 检查配置是否存在
	if sm.config == nil {
		slog.InfoContext(ctx, "🔍 [挂起检查] 配置为空，不挂起请求")
		return false
	}

	// 检查功能是否启用
	if !sm.config.RequestSuspend.Enabled {
		slog.InfoContext(ctx, "🔍 [挂起检查] 功能未启用，不挂起请求")
		return false
	}

	// 检查是否为手动模式
	if sm.config.Group.AutoSwitchBetweenGroups {
		slog.InfoContext(ctx, "🔍 [挂起检查] 当前为自动切换模式，不挂起请求")
		return false
	}

	// 检查当前挂起请求数量是否超过限制
	sm.suspendedRequestsMutex.RLock()
	currentCount := sm.suspendedRequestsCount
	sm.suspendedRequestsMutex.RUnlock()

	if currentCount >= sm.config.RequestSuspend.MaxSuspendedRequests {
		slog.WarnContext(ctx, fmt.Sprintf("🚫 [挂起限制] 当前挂起请求数 %d 已达到最大限制 %d，不再挂起新请求",
			currentCount, sm.config.RequestSuspend.MaxSuspendedRequests))
		return false
	}

	// 检查groupManager是否存在
	if sm.groupManager == nil {
		slog.InfoContext(ctx, "🔍 [挂起检查] 组管理器为空，不挂起请求")
		return false
	}

	// 检查是否存在可用的备用组
	allGroups := sm.groupManager.GetAllGroups()
	hasAvailableBackupGroups := false
	var availableGroups []string

	slog.InfoContext(ctx, fmt.Sprintf("🔍 [挂起检查] 开始检查可用备用组，总共 %d 个组", len(allGroups)))

	for _, group := range allGroups {
		slog.InfoContext(ctx, fmt.Sprintf("🔍 [挂起检查] 检查组 %s: IsActive=%t, InCooldown=%t",
			group.Name, group.IsActive, sm.groupManager.IsGroupInCooldown(group.Name)))

		// 检查非活跃组且不在冷却期的组
		if !group.IsActive && !sm.groupManager.IsGroupInCooldown(group.Name) {
			// 检查组内是否有健康端点
			healthyCount := 0
			for _, ep := range group.Endpoints {
				if ep.IsHealthy() {
					healthyCount++
				}
			}
			slog.InfoContext(ctx, fmt.Sprintf("🔍 [挂起检查] 组 %s 健康端点数: %d", group.Name, healthyCount))

			if healthyCount > 0 {
				hasAvailableBackupGroups = true
				availableGroups = append(availableGroups, fmt.Sprintf("%s(%d个健康端点)", group.Name, healthyCount))
			}
		}
	}

	if !hasAvailableBackupGroups {
		slog.InfoContext(ctx, "🔍 [挂起检查] 没有可用的备用组，不挂起请求")
		return false
	}

	slog.InfoContext(ctx, fmt.Sprintf("✅ [挂起检查] 满足挂起条件: 手动模式=%t, 功能启用=%t, 当前挂起数=%d/%d, 可用备用组=%v",
		!sm.config.Group.AutoSwitchBetweenGroups, sm.config.RequestSuspend.Enabled,
		currentCount, sm.config.RequestSuspend.MaxSuspendedRequests, availableGroups))

	return true
}

// WaitForGroupSwitch 挂起请求并等待组切换通知
// 迁移自 RetryHandler.waitForGroupSwitch，但移除状态管理部分，只保留挂起等待逻辑
// 返回是否成功切换到新组
func (sm *SuspensionManager) WaitForGroupSwitch(ctx context.Context, connID string) bool {
	// 检查配置和管理器是否存在
	if sm.config == nil {
		slog.InfoContext(ctx, "🔍 [挂起等待] 配置为空，无法挂起请求")
		return false
	}
	if sm.groupManager == nil {
		slog.InfoContext(ctx, "🔍 [挂起等待] 组管理器为空，无法挂起请求")
		return false
	}
	if sm.endpointManager == nil {
		slog.InfoContext(ctx, "🔍 [挂起等待] 端点管理器为空，无法挂起请求")
		return false
	}
	// 增加挂起请求计数
	sm.suspendedRequestsMutex.Lock()
	sm.suspendedRequestsCount++
	currentCount := sm.suspendedRequestsCount
	sm.suspendedRequestsMutex.Unlock()

	// 确保在退出时减少计数
	defer func() {
		sm.suspendedRequestsMutex.Lock()
		sm.suspendedRequestsCount--
		newCount := sm.suspendedRequestsCount
		sm.suspendedRequestsMutex.Unlock()
		slog.InfoContext(ctx, fmt.Sprintf("⬇️ [挂起结束] 连接 %s 请求挂起结束，当前挂起数: %d", connID, newCount))
	}()

	slog.InfoContext(ctx, fmt.Sprintf("⏸️ [请求挂起] 连接 %s 请求已挂起，等待组切换 (当前挂起数: %d)", connID, currentCount))

	// 订阅组切换通知
	groupChangeNotify := sm.groupManager.SubscribeToGroupChanges()
	defer func() {
		// 确保清理订阅，防止内存泄漏
		sm.groupManager.UnsubscribeFromGroupChanges(groupChangeNotify)
		slog.DebugContext(ctx, fmt.Sprintf("🔌 [订阅清理] 连接 %s 组切换通知订阅已清理", connID))
	}()

	// 创建超时context
	timeout := sm.config.RequestSuspend.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second // 默认5分钟
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	slog.InfoContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起超时设置: %v，等待组切换通知...", connID, timeout))

	// 等待组切换通知或超时
	select {
	case newGroupName := <-groupChangeNotify:
		// 收到组切换通知
		slog.InfoContext(ctx, fmt.Sprintf("📡 [组切换通知] 连接 %s 收到组切换通知: %s，验证新组可用性", connID, newGroupName))

		// 验证新激活的组是否有健康端点
		newEndpoints := sm.endpointManager.GetHealthyEndpoints()
		if len(newEndpoints) > 0 {
			slog.InfoContext(ctx, fmt.Sprintf("✅ [切换成功] 连接 %s 新组 %s 有 %d 个健康端点，恢复请求处理",
				connID, newGroupName, len(newEndpoints)))
			return true
		} else {
			slog.WarnContext(ctx, fmt.Sprintf("⚠️ [切换无效] 连接 %s 新组 %s 暂无健康端点，挂起失败",
				connID, newGroupName))
			return false
		}

	case <-timeoutCtx.Done():
		// 挂起超时
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			slog.WarnContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起等待超时 (%v)，停止等待", connID, timeout))
		} else {
			slog.InfoContext(ctx, fmt.Sprintf("🔄 [上下文取消] 连接 %s 挂起期间上下文被取消", connID))
		}
		return false

	case <-ctx.Done():
		// 原始请求被取消
		switch ctxErr := ctx.Err(); {
		case errors.Is(ctxErr, context.Canceled):
			slog.InfoContext(ctx, fmt.Sprintf("❌ [请求取消] 连接 %s 原始请求被客户端取消，结束挂起", connID))
		case errors.Is(ctxErr, context.DeadlineExceeded):
			slog.InfoContext(ctx, fmt.Sprintf("⏰ [请求超时] 连接 %s 原始请求上下文超时，结束挂起", connID))
		default:
			slog.InfoContext(ctx, fmt.Sprintf("❌ [请求异常] 连接 %s 原始请求上下文异常: %v，结束挂起", connID, ctxErr))
		}
		return false
	}
}

// GetSuspendedRequestsCount 返回当前挂起的请求数量
func (sm *SuspensionManager) GetSuspendedRequestsCount() int {
	sm.suspendedRequestsMutex.RLock()
	defer sm.suspendedRequestsMutex.RUnlock()
	return sm.suspendedRequestsCount
}

// UpdateConfig 更新配置
func (sm *SuspensionManager) UpdateConfig(cfg *config.Config) {
	sm.config = cfg
}

// WaitForEndpointRecovery 挂起请求并等待端点恢复或组切换通知
// 新增的端点自愈功能：监听指定端点的恢复信号
// 参数：
//   - ctx: 上下文
//   - connID: 连接ID
//   - failedEndpoint: 失败的端点名称
// 返回：是否成功恢复（端点恢复或组切换）
func (sm *SuspensionManager) WaitForEndpointRecovery(ctx context.Context, connID, failedEndpoint string) bool {
	// 检查配置和管理器是否存在
	if sm.config == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 配置为空，无法挂起请求")
		return false
	}
	if sm.groupManager == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 组管理器为空，无法挂起请求")
		return false
	}
	if sm.endpointManager == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 端点管理器为空，无法挂起请求")
		return false
	}

	// 增加挂起请求计数
	sm.suspendedRequestsMutex.Lock()
	sm.suspendedRequestsCount++
	currentCount := sm.suspendedRequestsCount
	sm.suspendedRequestsMutex.Unlock()

	// 确保在退出时减少计数
	defer func() {
		sm.suspendedRequestsMutex.Lock()
		sm.suspendedRequestsCount--
		newCount := sm.suspendedRequestsCount
		sm.suspendedRequestsMutex.Unlock()
		slog.InfoContext(ctx, fmt.Sprintf("⬇️ [挂起结束] 连接 %s 请求挂起结束，当前挂起数: %d", connID, newCount))
	}()

	slog.InfoContext(ctx, fmt.Sprintf("⏸️ [端点恢复挂起] 连接 %s 请求已挂起，等待端点 %s 恢复或组切换 (当前挂起数: %d)",
		connID, failedEndpoint, currentCount))

	// 创建超时context
	timeout := sm.config.RequestSuspend.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second // 默认5分钟
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	slog.InfoContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起超时设置: %v，等待端点 %s 恢复或组切换...",
		connID, timeout, failedEndpoint))

	// 订阅端点恢复信号（如果有恢复信号管理器）
	var endpointRecoveryCh chan string
	if sm.recoverySignalManager != nil && failedEndpoint != "" {
		endpointRecoveryCh = sm.recoverySignalManager.Subscribe(failedEndpoint)
		defer func() {
			if endpointRecoveryCh != nil {
				sm.recoverySignalManager.Unsubscribe(failedEndpoint, endpointRecoveryCh)
			}
		}()
	}

	// 订阅组切换通知
	groupChangeNotify := sm.groupManager.SubscribeToGroupChanges()
	defer func() {
		// 确保清理订阅，防止内存泄漏
		sm.groupManager.UnsubscribeFromGroupChanges(groupChangeNotify)
		slog.DebugContext(ctx, fmt.Sprintf("🔌 [订阅清理] 连接 %s 组切换通知订阅已清理", connID))
	}()

	// 等待恢复信号：端点恢复 > 组切换 > 超时
	for {
		select {
		case recoveredEndpoint := <-endpointRecoveryCh:
			// 🚀 [优先级1] 端点恢复信号 - 立即重试原端点
			slog.InfoContext(ctx, fmt.Sprintf("🎯 [端点自愈] 连接 %s 端点 %s 已恢复，立即重试原端点",
				connID, recoveredEndpoint))
			return true

		case newGroupName := <-groupChangeNotify:
			// 🔄 [优先级2] 组切换通知
			slog.InfoContext(ctx, fmt.Sprintf("📡 [组切换通知] 连接 %s 收到组切换通知: %s，验证新组可用性",
				connID, newGroupName))

			// 验证新激活的组是否有健康端点
			newEndpoints := sm.endpointManager.GetHealthyEndpoints()
			if len(newEndpoints) > 0 {
				slog.InfoContext(ctx, fmt.Sprintf("✅ [切换成功] 连接 %s 新组 %s 有 %d 个健康端点，恢复请求处理",
					connID, newGroupName, len(newEndpoints)))
				return true
			} else {
				slog.WarnContext(ctx, fmt.Sprintf("⚠️ [切换无效] 连接 %s 新组 %s 暂无健康端点，继续等待",
					connID, newGroupName))
				// 继续等待其他恢复信号
			}

		case <-timeoutCtx.Done():
			// ⏰ [优先级3] 挂起超时
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				slog.WarnContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起等待超时 (%v)，停止等待", connID, timeout))
			} else {
				slog.InfoContext(ctx, fmt.Sprintf("🔄 [上下文取消] 连接 %s 挂起期间上下文被取消", connID))
			}
			return false

		case <-ctx.Done():
			// ❌ [优先级4] 原始请求被取消
			switch ctxErr := ctx.Err(); {
			case errors.Is(ctxErr, context.Canceled):
				slog.InfoContext(ctx, fmt.Sprintf("❌ [请求取消] 连接 %s 原始请求被客户端取消，结束挂起", connID))
			case errors.Is(ctxErr, context.DeadlineExceeded):
				slog.InfoContext(ctx, fmt.Sprintf("⏰ [请求超时] 连接 %s 原始请求上下文超时，结束挂起", connID))
			default:
				slog.InfoContext(ctx, fmt.Sprintf("❌ [请求异常] 连接 %s 原始请求上下文异常: %v，结束挂起", connID, ctxErr))
			}
			return false
		}
	}
}

// WaitForEndpointRecoveryWithResult 🎯 [挂起取消区分] 带结果的端点恢复等待方法
// 功能与WaitForEndpointRecovery相同，但返回详细的结果类型以区分成功、超时、取消
// 这是对现有方法的增强版本，保持向后兼容性
func (sm *SuspensionManager) WaitForEndpointRecoveryWithResult(ctx context.Context, connID, failedEndpoint string) handlers.SuspensionResult {
	// 检查配置和管理器是否存在
	if sm.config == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 配置为空，无法挂起请求")
		return handlers.SuspensionTimeout
	}
	if sm.groupManager == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 组管理器为空，无法挂起请求")
		return handlers.SuspensionTimeout
	}
	if sm.endpointManager == nil {
		slog.InfoContext(ctx, "🔍 [端点恢复等待] 端点管理器为空，无法挂起请求")
		return handlers.SuspensionTimeout
	}

	// 增加挂起请求计数
	sm.suspendedRequestsMutex.Lock()
	sm.suspendedRequestsCount++
	currentCount := sm.suspendedRequestsCount
	sm.suspendedRequestsMutex.Unlock()

	// 确保在退出时减少计数
	defer func() {
		sm.suspendedRequestsMutex.Lock()
		sm.suspendedRequestsCount--
		newCount := sm.suspendedRequestsCount
		sm.suspendedRequestsMutex.Unlock()
		slog.InfoContext(ctx, fmt.Sprintf("⬇️ [挂起结束] 连接 %s 请求挂起结束，当前挂起数: %d", connID, newCount))
	}()

	slog.InfoContext(ctx, fmt.Sprintf("⏸️ [端点恢复挂起] 连接 %s 请求已挂起，等待端点 %s 恢复或组切换 (当前挂起数: %d)",
		connID, failedEndpoint, currentCount))

	// 创建超时context
	timeout := sm.config.RequestSuspend.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second // 默认5分钟
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	slog.InfoContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起超时设置: %v，等待端点 %s 恢复或组切换...",
		connID, timeout, failedEndpoint))

	// 订阅端点恢复信号（如果有恢复信号管理器）
	var endpointRecoveryCh chan string
	if sm.recoverySignalManager != nil && failedEndpoint != "" {
		endpointRecoveryCh = sm.recoverySignalManager.Subscribe(failedEndpoint)
		defer func() {
			if endpointRecoveryCh != nil {
				sm.recoverySignalManager.Unsubscribe(failedEndpoint, endpointRecoveryCh)
			}
		}()
	}

	// 订阅组切换通知
	groupChangeNotify := sm.groupManager.SubscribeToGroupChanges()
	defer func() {
		// 确保清理订阅，防止内存泄漏
		sm.groupManager.UnsubscribeFromGroupChanges(groupChangeNotify)
		slog.DebugContext(ctx, fmt.Sprintf("🔌 [订阅清理] 连接 %s 组切换通知订阅已清理", connID))
	}()

	// 等待恢复信号：端点恢复 > 组切换 > 超时 > 取消
	for {
		select {
		case recoveredEndpoint := <-endpointRecoveryCh:
			// 🚀 [优先级1] 端点恢复信号 - 立即重试原端点
			slog.InfoContext(ctx, fmt.Sprintf("🎯 [端点自愈] 连接 %s 端点 %s 已恢复，立即重试原端点",
				connID, recoveredEndpoint))
			return handlers.SuspensionSuccess

		case newGroupName := <-groupChangeNotify:
			// 🔄 [优先级2] 组切换通知
			slog.InfoContext(ctx, fmt.Sprintf("📡 [组切换通知] 连接 %s 收到组切换通知: %s，验证新组可用性",
				connID, newGroupName))

			// 验证新激活的组是否有健康端点
			newEndpoints := sm.endpointManager.GetHealthyEndpoints()
			if len(newEndpoints) > 0 {
				slog.InfoContext(ctx, fmt.Sprintf("✅ [切换成功] 连接 %s 新组 %s 有 %d 个健康端点，恢复请求处理",
					connID, newGroupName, len(newEndpoints)))
				return handlers.SuspensionSuccess
			} else {
				slog.WarnContext(ctx, fmt.Sprintf("⚠️ [切换无效] 连接 %s 新组 %s 暂无健康端点，继续等待",
					connID, newGroupName))
				// 继续等待其他恢复信号
			}

		case <-timeoutCtx.Done():
			// ⏰ [优先级3] 挂起超时
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				slog.WarnContext(ctx, fmt.Sprintf("⏰ [挂起超时] 连接 %s 挂起等待超时 (%v)，停止等待", connID, timeout))
			} else {
				slog.InfoContext(ctx, fmt.Sprintf("🔄 [上下文取消] 连接 %s 挂起期间超时上下文被取消", connID))
			}
			return handlers.SuspensionTimeout

		case <-ctx.Done():
			// ❌ [优先级4] 原始请求被取消
			switch ctxErr := ctx.Err(); {
			case errors.Is(ctxErr, context.Canceled):
				slog.InfoContext(ctx, fmt.Sprintf("❌ [请求取消] 连接 %s 原始请求被客户端取消，结束挂起", connID))
				return handlers.SuspensionCancelled
			case errors.Is(ctxErr, context.DeadlineExceeded):
				slog.InfoContext(ctx, fmt.Sprintf("⏰ [请求超时] 连接 %s 原始请求上下文超时，结束挂起", connID))
				return handlers.SuspensionTimeout
			default:
				slog.InfoContext(ctx, fmt.Sprintf("❌ [请求异常] 连接 %s 原始请求上下文异常: %v，结束挂起", connID, ctxErr))
				return handlers.SuspensionCancelled
			}
		}
	}
}

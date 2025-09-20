package proxy

import (
	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

// RetryManagerFactory 重试管理器工厂接口
// 用于创建RetryManager实例，支持依赖注入模式
type RetryManagerFactory interface {
	// NewRetryManager 创建新的重试管理器实例
	// 参数：
	//   - cfg: 配置信息
	//   - errorRecovery: 错误恢复管理器
	//   - endpointMgr: 端点管理器
	// 返回：RetryManager实例
	NewRetryManager(cfg *config.Config, errorRecovery *ErrorRecoveryManager, endpointMgr *endpoint.Manager) *RetryManager
}

// SuspensionManagerFactory 挂起管理器工厂接口
// 用于创建SuspensionManager实例，支持依赖注入模式
type SuspensionManagerFactory interface {
	// NewSuspensionManager 创建新的挂起管理器实例
	// 参数：
	//   - cfg: 配置信息
	//   - endpointManager: 端点管理器
	//   - groupManager: 组管理器
	// 返回：SuspensionManager实例
	NewSuspensionManager(cfg *config.Config, endpointManager *endpoint.Manager, groupManager *endpoint.GroupManager) *SuspensionManager
}

// DefaultRetryManagerFactory 默认重试管理器工厂实现
type DefaultRetryManagerFactory struct{}

// NewRetryManager 实现RetryManagerFactory接口
func (f *DefaultRetryManagerFactory) NewRetryManager(cfg *config.Config, errorRecovery *ErrorRecoveryManager, endpointMgr *endpoint.Manager) *RetryManager {
	return NewRetryManager(cfg, errorRecovery, endpointMgr)
}

// DefaultSuspensionManagerFactory 默认挂起管理器工厂实现
type DefaultSuspensionManagerFactory struct{}

// NewSuspensionManager 实现SuspensionManagerFactory接口
func (f *DefaultSuspensionManagerFactory) NewSuspensionManager(cfg *config.Config, endpointManager *endpoint.Manager, groupManager *endpoint.GroupManager) *SuspensionManager {
	return NewSuspensionManager(cfg, endpointManager, groupManager)
}
package endpoint

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"cc-forwarder/config"
)

// GroupInfo represents information about an endpoint group
type GroupInfo struct {
	Name         string
	Priority     int
	IsActive     bool
	CooldownUntil time.Time
	Endpoints    []*Endpoint
	// Manual control states
	ManuallyPaused bool
	ManualActivationTime time.Time
	// Forced activation states
	ForcedActivation bool       // 标记是否为强制激活（无健康端点时激活）
	ForcedActivationTime time.Time // 强制激活时间
}

// GroupManager manages endpoint groups and their cooldown states
type GroupManager struct {
	groups        map[string]*GroupInfo
	config        *config.Config
	mutex         sync.RWMutex
	cooldownDuration time.Duration
	// Group change notification subscribers
	groupChangeSubscribers []chan string
	subscriberMutex        sync.RWMutex
}

// NewGroupManager creates a new group manager
func NewGroupManager(cfg *config.Config) *GroupManager {
	return &GroupManager{
		groups:               make(map[string]*GroupInfo),
		config:               cfg,
		cooldownDuration:     cfg.Group.Cooldown,
		groupChangeSubscribers: make([]chan string, 0),
	}
}

// UpdateConfig updates the group manager configuration
func (gm *GroupManager) UpdateConfig(cfg *config.Config) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	gm.config = cfg
	gm.cooldownDuration = cfg.Group.Cooldown
}

// UpdateGroups rebuilds group information from endpoints
func (gm *GroupManager) UpdateGroups(endpoints []*Endpoint) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	// Clear existing groups but preserve cooldown states
	oldGroups := make(map[string]*GroupInfo)
	for name, group := range gm.groups {
		if !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil) {
			// Preserve cooldown state
			oldGroups[name] = &GroupInfo{
				Name:         group.Name,
				Priority:     group.Priority,
				IsActive:     false,
				CooldownUntil: group.CooldownUntil,
				Endpoints:    nil, // Will be updated
			}
		}
	}
	
	// Rebuild groups from current endpoints
	newGroups := make(map[string]*GroupInfo)
	
	for _, ep := range endpoints {
		groupName := ep.Config.Group
		if groupName == "" {
			groupName = "Default"
		}
		
		if _, exists := newGroups[groupName]; !exists {
			// Check if this group was in cooldown
			var cooldownUntil time.Time
			if oldGroup, hadCooldown := oldGroups[groupName]; hadCooldown {
				cooldownUntil = oldGroup.CooldownUntil
			}
			
			newGroups[groupName] = &GroupInfo{
				Name:         groupName,
				Priority:     ep.Config.GroupPriority,
				IsActive:     false, // Don't auto-activate groups, let updateActiveGroups handle activation
				CooldownUntil: cooldownUntil,
				Endpoints:    make([]*Endpoint, 0),
			}
		}
		
		newGroups[groupName].Endpoints = append(newGroups[groupName].Endpoints, ep)
	}
	
	gm.groups = newGroups
	
	// Update active status based on cooldown timers
	gm.updateActiveGroups()
}

// updateActiveGroups updates which groups are currently active
func (gm *GroupManager) updateActiveGroups() {
	now := time.Now()
	var newlyActivatedGroup string
	
	// Track previous active state to detect changes
	previousActiveGroups := make(map[string]bool)
	for _, group := range gm.groups {
		previousActiveGroups[group.Name] = group.IsActive
	}
	
	// First, check cooldown timers and clear expired cooldowns
	for _, group := range gm.groups {
		if !group.CooldownUntil.IsZero() && now.After(group.CooldownUntil) {
			// Cooldown expired, clear it but don't auto-activate in manual mode
			group.CooldownUntil = time.Time{}
			slog.Info(fmt.Sprintf("🔄 [组管理] 组冷却结束: %s (优先级: %d) - %s", 
				group.Name, group.Priority, 
				map[bool]string{true: "自动激活", false: "等待手动激活"}[gm.config.Group.AutoSwitchBetweenGroups]))
		} else if !group.CooldownUntil.IsZero() && now.Before(group.CooldownUntil) {
			// Still in cooldown
			group.IsActive = false
		}
	}
	
	// Determine which groups should be active based on priority
	// Only auto-activate next group if auto switching is enabled
	if gm.config.Group.AutoSwitchBetweenGroups {
		// Auto mode: automatically activate highest priority available group
		// Get all groups sorted by priority
		sortedGroups := gm.getSortedGroups()
		
		// Find the highest priority group that's not in cooldown and not manually paused
		activeGroupFound := false
		for _, group := range sortedGroups {
			isAvailable := group.CooldownUntil.IsZero() && !group.ManuallyPaused
			if isAvailable {
				if !activeGroupFound {
					wasActive := group.IsActive
					group.IsActive = true
					activeGroupFound = true
					// Check if this group became newly active
					if !wasActive && group.IsActive {
						newlyActivatedGroup = group.Name
					}
				} else {
					group.IsActive = false // Only one group can be active at a time
				}
			} else {
				group.IsActive = false
			}
		}
	} else {
		// Manual mode: Only activate priority 1 group at startup if no groups are active
		// Don't auto-switch between groups during runtime
		currentActiveCount := 0
		for _, group := range gm.groups {
			if group.IsActive {
				currentActiveCount++
			}
		}
		
		// Handle cooldown states first
		for _, group := range gm.groups {
			if !group.CooldownUntil.IsZero() && now.Before(group.CooldownUntil) {
				// Still in cooldown, keep inactive
				group.IsActive = false
			}
		}
		
		// If no groups are active, determine if this is startup or runtime failure
		if currentActiveCount == 0 {
			// Check if this is truly startup (no groups have ever failed) or runtime failure
			isActualStartup := true
			for _, group := range gm.groups {
				if !group.CooldownUntil.IsZero() || group.ManuallyPaused {
					isActualStartup = false
					break
				}
			}
			
			// Determine activation strategy based on startup vs runtime context
			var shouldAutoActivate bool
			if isActualStartup {
				// Always auto-activate priority 1 group at startup for better UX
				shouldAutoActivate = true
				slog.Debug("🚀 [组管理] 检测到系统启动 - 尝试激活优先级1组")
			} else {
				// This is runtime failure - respect manual mode + suspend settings
				if !gm.config.Group.AutoSwitchBetweenGroups && gm.config.RequestSuspend.Enabled {
					shouldAutoActivate = false
					slog.Debug("⏸️ [组管理] 运行时故障且启用挂起 - 不激活其他组，等待挂起处理")
				} else {
					// Manual mode without suspend, or auto mode - allow activation
					shouldAutoActivate = true
					slog.Debug("🔄 [组管理] 运行时故障但未启用挂起 - 尝试激活可用组")
				}
			}
			
			if shouldAutoActivate {
				sortedGroups := gm.getSortedGroups()
				for _, group := range sortedGroups {
					// 关键修复：检查组是否被手动暂停（包括因失败而暂停的组）
					if group.Priority == 1 && group.CooldownUntil.IsZero() && !group.ManuallyPaused {
						// Check if this group has healthy endpoints
						hasHealthyEndpoints := false
						for _, ep := range group.Endpoints {
							if ep.IsHealthy() {
								hasHealthyEndpoints = true
								break
							}
						}
						if hasHealthyEndpoints {
							wasActive := group.IsActive
							group.IsActive = true
							if isActualStartup {
								if gm.config.Group.AutoSwitchBetweenGroups {
									slog.Info(fmt.Sprintf("🚀 [自动模式] 启动时激活优先级1组: %s (有健康端点)", group.Name))
								} else {
									slog.Info(fmt.Sprintf("🚀 [手动模式] 启动时激活优先级1组: %s (有健康端点) - 后续故障将启用挂起", group.Name))
								}
							} else {
								slog.Info(fmt.Sprintf("🔄 [运行时] 激活可用组: %s (优先级: %d, 有健康端点)", group.Name, group.Priority))
							}
							// Check if this group became newly active
							if !wasActive && group.IsActive {
								newlyActivatedGroup = group.Name
							}
							break // Only activate one group
						}
					} else if group.ManuallyPaused {
						// 记录被暂停的组，说明为什么没有激活
						slog.Debug(fmt.Sprintf("⏸️ [手动模式] 跳过已暂停组: %s (优先级: %d) - 等待手动恢复", group.Name, group.Priority))
					}
				}
			}
		}
	}
	
	// Notify subscribers if a group was newly activated
	if newlyActivatedGroup != "" {
		// Check if this is truly a state change (not just the same group remaining active)
		if !previousActiveGroups[newlyActivatedGroup] {
			slog.Debug(fmt.Sprintf("📡 [组通知] 检测到组状态变化: %s 变为活跃", newlyActivatedGroup))
			gm.notifyGroupChange(newlyActivatedGroup)
		}
	}
}

// getSortedGroups returns groups sorted by priority (lower number = higher priority)
func (gm *GroupManager) getSortedGroups() []*GroupInfo {
	groups := make([]*GroupInfo, 0, len(gm.groups))
	for _, group := range gm.groups {
		groups = append(groups, group)
	}
	
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})
	
	return groups
}

// GetActiveGroups returns currently active groups
func (gm *GroupManager) GetActiveGroups() []*GroupInfo {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	gm.updateActiveGroups()
	
	var active []*GroupInfo
	for _, group := range gm.groups {
		if group.IsActive {
			active = append(active, group)
		}
	}
	
	// Sort by priority
	sort.Slice(active, func(i, j int) bool {
		return active[i].Priority < active[j].Priority
	})
	
	return active
}

// GetAllGroups returns all groups
func (gm *GroupManager) GetAllGroups() []*GroupInfo {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	gm.updateActiveGroups()
	
	groups := make([]*GroupInfo, 0, len(gm.groups))
	for _, group := range gm.groups {
		groups = append(groups, group)
	}
	
	// Sort by priority
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})
	
	return groups
}

// SetGroupCooldown sets a group into cooldown mode (only in auto mode)
func (gm *GroupManager) SetGroupCooldown(groupName string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	if group, exists := gm.groups[groupName]; exists {
		// In manual mode, mark group as manually paused to prevent re-activation
		if !gm.config.Group.AutoSwitchBetweenGroups {
			group.IsActive = false
			group.ManuallyPaused = true // 👈 关键修复：防止组被自动重新激活
			slog.Warn(fmt.Sprintf("⚠️ [手动模式] 组 %s 失败已停用并标记为暂停状态，需要手动切换到其他组", groupName))
			slog.Info(fmt.Sprintf("🚫 [组状态] 组 %s 已设置 ManuallyPaused=true，不会被自动重新激活", groupName))
			return
		}
		
		// Auto mode: use cooldown mechanism
		now := time.Now()
		group.CooldownUntil = now.Add(gm.cooldownDuration)
		group.IsActive = false
		
		slog.Warn(fmt.Sprintf("❄️ [自动模式] 组进入冷却状态: %s (冷却时长: %v, 恢复时间: %s)", 
			groupName, gm.cooldownDuration, group.CooldownUntil.Format("15:04:05")))
		
		// Update active groups after cooldown change
		gm.updateActiveGroups()
		
		// Log and notify about next active group
		for _, g := range gm.getSortedGroups() {
			if g.IsActive {
				slog.Info(fmt.Sprintf("🔄 [自动模式] 切换到下一优先级组: %s (优先级: %d)", 
					g.Name, g.Priority))
				// Notify subscribers about the group switch
				gm.notifyGroupChange(g.Name)
				break
			}
		}
	}
}

// IsGroupInCooldown checks if a group is currently in cooldown
func (gm *GroupManager) IsGroupInCooldown(groupName string) bool {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	if group, exists := gm.groups[groupName]; exists {
		return !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil)
	}
	
	return false
}

// GetGroupCooldownRemaining returns remaining cooldown time for a group
func (gm *GroupManager) GetGroupCooldownRemaining(groupName string) time.Duration {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	if group, exists := gm.groups[groupName]; exists {
		if !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil) {
			return group.CooldownUntil.Sub(time.Now())
		}
	}
	
	return 0
}

// ManualActivateGroup manually activates a specific group and deactivates others (compatibility function)
func (gm *GroupManager) ManualActivateGroup(groupName string) error {
	return gm.ManualActivateGroupWithForce(groupName, false)
}

// ManualActivateGroupWithForce manually activates a specific group and deactivates others
// force: 当为true时，即使组内没有健康端点也强制激活
func (gm *GroupManager) ManualActivateGroupWithForce(groupName string, force bool) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}

	// 检查冷却状态（强制激活仍需检查冷却）
	if !targetGroup.CooldownUntil.IsZero() && time.Now().Before(targetGroup.CooldownUntil) {
		remaining := targetGroup.CooldownUntil.Sub(time.Now())
		return fmt.Errorf("组 %s 仍在冷却中，剩余时间: %v", groupName, remaining.Round(time.Second))
	}

	// 检查健康端点
	healthyCount := 0
	totalCount := len(targetGroup.Endpoints)
	for _, ep := range targetGroup.Endpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
	}

	// 核心逻辑：强制激活只能在完全没有健康端点时使用
	if healthyCount == 0 {
		// 没有健康端点的情况
		if !force {
			return fmt.Errorf("组 %s 中没有健康的端点，无法激活。如需强制激活请使用强制模式", groupName)
		}
		// 强制激活：只有在没有健康端点时才允许
		slog.Warn(fmt.Sprintf("⚠️ [强制激活] 用户强制激活无健康端点组: %s (健康端点: %d/%d, 操作时间: %s, 风险等级: HIGH)",
			groupName, healthyCount, totalCount, time.Now().Format("2006-01-02 15:04:05")))
		slog.Error(fmt.Sprintf("🚨 [安全警告] 强制激活可能导致请求失败! 组: %s, 建议尽快检查端点健康状态", groupName))

		// 标记强制激活
		targetGroup.ForcedActivation = true
		targetGroup.ForcedActivationTime = time.Now()
	} else {
		// 有健康端点的情况
		if force {
			// 拒绝在有健康端点时使用强制激活
			return fmt.Errorf("组 %s 有 %d 个健康端点，无需强制激活。请使用正常激活", groupName, healthyCount)
		}
		// 正常激活
		targetGroup.ForcedActivation = false
		targetGroup.ForcedActivationTime = time.Time{}

		slog.Info(fmt.Sprintf("🔄 [正常激活] 手动激活组: %s (健康端点: %d/%d)",
			groupName, healthyCount, totalCount))
	}

	// 停用所有组
	for _, group := range gm.groups {
		group.IsActive = false
		group.ManuallyPaused = false
	}

	// 激活目标组
	targetGroup.IsActive = true
	targetGroup.ManualActivationTime = time.Now()
	targetGroup.CooldownUntil = time.Time{}

	// 通知订阅者
	gm.notifyGroupChange(groupName)

	return nil
}

// ManualActivateGroupCompat 兼容性函数，默认不强制激活
func (gm *GroupManager) ManualActivateGroupCompat(groupName string) error {
	return gm.ManualActivateGroupWithForce(groupName, false)
}

// ManualPauseGroup manually pauses a group (prevents it from being auto-activated)
func (gm *GroupManager) ManualPauseGroup(groupName string, duration time.Duration) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}
	
	// Pause the group
	targetGroup.ManuallyPaused = true
	var switchedToGroup string
	if targetGroup.IsActive {
		targetGroup.IsActive = false
		// Find next available group to activate
		gm.updateActiveGroups()
		// Check which group became active after pausing
		for _, g := range gm.getSortedGroups() {
			if g.IsActive {
				switchedToGroup = g.Name
				break
			}
		}
	}
	
	if duration > 0 {
		// Set a timer to automatically unpause
		go func() {
			time.Sleep(duration)
			gm.mutex.Lock()
			defer gm.mutex.Unlock()
			if targetGroup.ManuallyPaused {
				targetGroup.ManuallyPaused = false
				// Store previous state to check for changes
				prevActiveGroups := make(map[string]bool)
				for _, g := range gm.groups {
					prevActiveGroups[g.Name] = g.IsActive
				}
				gm.updateActiveGroups()
				// Check if any group became newly active
				for _, g := range gm.groups {
					if g.IsActive && !prevActiveGroups[g.Name] {
						gm.notifyGroupChange(g.Name)
						break
					}
				}
				slog.Info(fmt.Sprintf("⏰ [自动恢复] 组 %s 暂停期已结束，重新可用", groupName))
			}
		}()
		slog.Info(fmt.Sprintf("⏸️ [手动暂停] 组 %s 已暂停 %v", groupName, duration))
	} else {
		slog.Info(fmt.Sprintf("⏸️ [手动暂停] 组 %s 已暂停，需要手动恢复", groupName))
	}
	
	// Notify about group switch if another group became active
	if switchedToGroup != "" {
		gm.notifyGroupChange(switchedToGroup)
		slog.Debug(fmt.Sprintf("📡 [组通知] 因暂停组 %s 而切换到组 %s", groupName, switchedToGroup))
	}
	
	return nil
}

// ManualResumeGroup manually resumes a paused group
func (gm *GroupManager) ManualResumeGroup(groupName string) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}
	
	if !targetGroup.ManuallyPaused {
		return fmt.Errorf("组 %s 未处于暂停状态", groupName)
	}
	
	targetGroup.ManuallyPaused = false
	
	// Store previous active groups to detect changes
	prevActiveGroups := make(map[string]bool)
	for _, g := range gm.groups {
		prevActiveGroups[g.Name] = g.IsActive
	}
	
	gm.updateActiveGroups() // Re-evaluate active groups
	
	// Check if any group became newly active
	for _, g := range gm.groups {
		if g.IsActive && !prevActiveGroups[g.Name] {
			gm.notifyGroupChange(g.Name)
			slog.Debug(fmt.Sprintf("📡 [组通知] 因恢复组 %s 而激活组 %s", groupName, g.Name))
			break
		}
	}
	
	slog.Info(fmt.Sprintf("▶️ [手动恢复] 组 %s 已恢复，重新参与自动选择", groupName))
	return nil
}

// GetGroupDetails returns detailed information about all groups
func (gm *GroupManager) GetGroupDetails() map[string]interface{} {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	gm.updateActiveGroups()
	
	result := make(map[string]interface{})
	groupsData := make([]map[string]interface{}, 0, len(gm.groups))
	
	for _, group := range gm.groups {
		healthyCount := 0
		unhealthyCount := 0
		totalEndpoints := len(group.Endpoints)
		
		for _, ep := range group.Endpoints {
			if ep.IsHealthy() {
				healthyCount++
			} else {
				unhealthyCount++
			}
		}
		
		var status string
		var statusColor string
		var cooldownRemaining time.Duration
		
		if group.IsActive {
			status = "活跃"
			statusColor = "success"
		} else if group.ManuallyPaused {
			status = "手动暂停"
			statusColor = "warning"
		} else if !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil) {
			status = "冷却中"
			statusColor = "danger"
			cooldownRemaining = group.CooldownUntil.Sub(time.Now())
		} else if healthyCount == 0 {
			status = "无健康端点"
			statusColor = "danger"
		} else {
			status = "可用"
			statusColor = "secondary"
		}
		
		groupData := map[string]interface{}{
			"name":               group.Name,
			"priority":           group.Priority,
			"is_active":          group.IsActive,
			"status":             status,
			"status_color":       statusColor,
			"total_endpoints":    totalEndpoints,
			"healthy_endpoints":  healthyCount,
			"unhealthy_endpoints": unhealthyCount,
			"manually_paused":    group.ManuallyPaused,
			"in_cooldown":        !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil),
			"cooldown_remaining": cooldownRemaining.Round(time.Second).String(),
			"can_activate":       healthyCount > 0 && !group.IsActive && (group.CooldownUntil.IsZero() || time.Now().After(group.CooldownUntil)),
			"can_pause":          !group.ManuallyPaused,
			"can_resume":         group.ManuallyPaused,
			"forced_activation":      group.ForcedActivation,
			"forced_activation_time": "",
			"activation_type":        "normal",
			"can_force_activate":     healthyCount == 0 && !group.IsActive && (group.CooldownUntil.IsZero() || time.Now().After(group.CooldownUntil)),
		}

		// 添加强制激活时间
		if !group.ForcedActivationTime.IsZero() {
			groupData["forced_activation_time"] = group.ForcedActivationTime.Format("2006-01-02 15:04:05")
		}

		// 设置激活类型
		if group.ForcedActivation {
			groupData["activation_type"] = "forced"
			// 计算健康状态描述
			if healthyCount == 0 {
				groupData["_computed_health_status"] = "强制激活(无健康端点)"
			} else {
				groupData["_computed_health_status"] = "强制激活(已恢复)"
			}
		}
		
		if !group.ManualActivationTime.IsZero() {
			groupData["last_manual_activation"] = group.ManualActivationTime.Format("2006-01-02 15:04:05")
		}
		
		groupsData = append(groupsData, groupData)
	}
	
	// Sort by priority
	sort.Slice(groupsData, func(i, j int) bool {
		return groupsData[i]["priority"].(int) < groupsData[j]["priority"].(int)
	})
	
	result["groups"] = groupsData
	result["total_groups"] = len(groupsData)
	result["active_groups"] = len(gm.GetActiveGroups())
	
	return result
}

// FilterEndpointsByActiveGroups filters endpoints to only include those in active groups
func (gm *GroupManager) FilterEndpointsByActiveGroups(endpoints []*Endpoint) []*Endpoint {
	activeGroups := gm.GetActiveGroups()
	if len(activeGroups) == 0 {
		return nil
	}
	
	// Create a map of active group names for quick lookup
	activeGroupNames := make(map[string]bool)
	for _, group := range activeGroups {
		activeGroupNames[group.Name] = true
	}
	
	// Filter endpoints
	var filtered []*Endpoint
	for _, ep := range endpoints {
		groupName := ep.Config.Group
		if groupName == "" {
			groupName = "Default"
		}
		
		if activeGroupNames[groupName] {
			filtered = append(filtered, ep)
		}
	}
	
	return filtered
}

// SubscribeToGroupChanges subscribes to group change notifications
// Returns a channel that will receive the name of the newly activated group
func (gm *GroupManager) SubscribeToGroupChanges() <-chan string {
	gm.subscriberMutex.Lock()
	defer gm.subscriberMutex.Unlock()
	
	// Create a buffered channel to avoid blocking the sender
	ch := make(chan string, 10)
	gm.groupChangeSubscribers = append(gm.groupChangeSubscribers, ch)
	
	slog.Debug(fmt.Sprintf("📡 [组通知] 新增订阅者，当前订阅者数: %d", len(gm.groupChangeSubscribers)))
	
	return ch
}

// UnsubscribeFromGroupChanges removes a subscriber from group change notifications
func (gm *GroupManager) UnsubscribeFromGroupChanges(ch <-chan string) {
	gm.subscriberMutex.Lock()
	defer gm.subscriberMutex.Unlock()
	
	// Find and remove the channel from subscribers
	for i, subscriber := range gm.groupChangeSubscribers {
		if subscriber == ch {
			// Remove the channel from the slice
			gm.groupChangeSubscribers = append(gm.groupChangeSubscribers[:i], gm.groupChangeSubscribers[i+1:]...)
			// Close the channel to signal unsubscription
			close(subscriber)
			slog.Debug(fmt.Sprintf("📡 [组通知] 移除订阅者，当前订阅者数: %d", len(gm.groupChangeSubscribers)))
			return
		}
	}
}

// notifyGroupChange sends a non-blocking notification to all subscribers
// This method should be called with appropriate locks already held
func (gm *GroupManager) notifyGroupChange(activatedGroupName string) {
	gm.subscriberMutex.RLock()
	defer gm.subscriberMutex.RUnlock()
	
	if len(gm.groupChangeSubscribers) == 0 {
		return
	}
	
	slog.Debug(fmt.Sprintf("📡 [组通知] 广播组切换事件: %s (订阅者数: %d)", 
		activatedGroupName, len(gm.groupChangeSubscribers)))
	
	// Send notification to all subscribers in a non-blocking manner
	for i, subscriber := range gm.groupChangeSubscribers {
		select {
		case subscriber <- activatedGroupName:
			// Successfully sent
		default:
			// Channel is full or closed, log warning
			slog.Warn(fmt.Sprintf("📡 [组通知] 订阅者 #%d 通道已满或已关闭，跳过通知", i))
		}
	}
}
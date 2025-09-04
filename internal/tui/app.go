package tui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	
	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
)

// TUIApp represents the main TUI application
type TUIApp struct {
	app                  *tview.Application
	cfg                  *config.Config
	endpointManager      *endpoint.Manager
	monitoringMiddleware *middleware.MonitoringMiddleware
	startTime            time.Time
	
	// UI components
	pages     *tview.Pages
	tabBar    *tview.TextView
	statusBar *tview.TextView
	
	// Views
	overviewView    *OverviewView
	endpointsView   *EndpointsView
	connectionsView *ConnectionsView
	logsView        *LogsView
	configView      *ConfigView
	
	// State
	currentTab int
	tabs       []Tab
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	configPath string                 // Configuration file path
	
	// Edit mode state for priority editing
	editMode        bool                // Whether we're in edit mode
	tempPriorities  map[string]int      // Temporary priority changes in memory
	isDirty         bool                // Whether there are unsaved changes
	editMutex       sync.RWMutex        // Protects edit mode state
}

// Tab represents a tab in the TUI
type Tab struct {
	Name string
	View tview.Primitive
}

// NewTUIApp creates a new TUI application
func NewTUIApp(cfg *config.Config, endpointManager *endpoint.Manager, monitoringMiddleware *middleware.MonitoringMiddleware, startTime time.Time, configPath string) *TUIApp {
	app := tview.NewApplication()
	ctx, cancel := context.WithCancel(context.Background())
	
	tuiApp := &TUIApp{
		app:                  app,
		cfg:                  cfg,
		endpointManager:      endpointManager,
		monitoringMiddleware: monitoringMiddleware,
		startTime:            startTime,
		ctx:                  ctx,
		cancel:               cancel,
		currentTab:           0,
		running:              false,
		configPath:           configPath,
		tempPriorities:       make(map[string]int),
		editMode:             false,
		isDirty:              false,
	}

	// Create UI components
	tuiApp.setupUI()
	
	return tuiApp
}

// setupUI creates and configures all UI components
func (t *TUIApp) setupUI() {
	// Create main pages container
	t.pages = tview.NewPages()

	// Create views
	t.overviewView = NewOverviewView(t.monitoringMiddleware, t.endpointManager, t.startTime)
	t.endpointsView = NewEndpointsView(t.monitoringMiddleware, t.endpointManager)
	t.endpointsView.SetTUIApp(t)  // Set reference for edit mode functionality
	t.connectionsView = NewConnectionsView(t.monitoringMiddleware, t.endpointManager, t.cfg)
	t.logsView = NewLogsView()
	t.configView = NewConfigView(t.cfg)

	// Define tabs
	t.tabs = []Tab{
		{"Overview", t.overviewView.GetPrimitive()},
		{"Endpoints", t.endpointsView.GetPrimitive()},
		{"Connections", t.connectionsView.GetPrimitive()},
		{"Logs", t.logsView.GetPrimitive()},
		{"Config", t.configView.GetPrimitive()},
	}

	// Create tab bar
	t.tabBar = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	t.tabBar.SetBorder(true).SetTitle(" Navigation ").SetTitleAlign(tview.AlignLeft)
	t.updateTabBar()

	// Create status bar
	t.statusBar = tview.NewTextView().
		SetDynamicColors(false).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft)
	t.statusBar.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)
	t.updateStatusBar()

	// Create main layout
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.createHeaderFlex(), 3, 1, false).
		AddItem(t.tabBar, 3, 1, false).
		AddItem(t.pages, 0, 1, true).
		AddItem(t.statusBar, 3, 1, false)

	// Add pages to the container
	for i, tab := range t.tabs {
		t.pages.AddPage(tab.Name, tab.View, true, i == 0)
	}

	// Set input capture for tab navigation
	t.app.SetInputCapture(t.handleInput)

	// Set root and focus
	t.app.SetRoot(mainFlex, true).SetFocus(t.pages)
}

// createHeaderFlex creates the header section
func (t *TUIApp) createHeaderFlex() *tview.Flex {
	title := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf("[blue::b]🚀 Claude EndPoints Forwarder TUI[white::-]"))

	headerFlex := tview.NewFlex().
		AddItem(tview.NewTextView(), 1, 1, false).
		AddItem(title, 0, 1, false).
		AddItem(tview.NewTextView(), 1, 1, false)
	
	headerFlex.SetBorder(true).SetTitle(" Claude EndPoints Forwarder TUI ").SetTitleAlign(tview.AlignCenter)
	
	return headerFlex
}

// handleInput handles keyboard input for navigation
func (t *TUIApp) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// Handle edit mode specific keys first (only in Endpoints tab)
	if t.currentTab == 1 { // Endpoints tab
		if t.IsInEditMode() {
			// Edit mode is active
			switch event.Key() {
			case tcell.KeyEscape:
				// Exit edit mode without saving
				t.ExitEditMode()
				return nil
			case tcell.KeyCtrlS:
				// Save changes to config
				if err := t.SavePrioritiesToConfig(); err != nil {
					t.AddLog("ERROR", fmt.Sprintf("保存配置失败: %v", err), "TUI")
				}
				return nil
			}
			
			// Handle number keys for priority setting in edit mode
			if event.Rune() >= '1' && event.Rune() <= '9' {
				priority := int(event.Rune() - '0')
				t.setSelectedEndpointPriority(priority)
				return nil
			}
			
			// Handle two-digit priorities (10-99) - requires holding shift for special chars
			// For now, just support 1-9 in edit mode
			
		} else {
			// Normal mode in Endpoints tab
			switch event.Key() {
			case tcell.KeyEnter:
				// Enter edit mode
				t.EnterEditMode()
				return nil
			}
		}
	}
	
	// Handle global navigation keys
	switch event.Key() {
	case tcell.KeyTab:
		// Next tab (but only if not in edit mode)
		if !t.IsInEditMode() {
			t.currentTab = (t.currentTab + 1) % len(t.tabs)
			t.switchToTab(t.currentTab)
			t.updateTabBar()
		}
		return nil
	case tcell.KeyBacktab:
		// Previous tab (but only if not in edit mode)
		if !t.IsInEditMode() {
			t.currentTab = (t.currentTab - 1 + len(t.tabs)) % len(t.tabs)
			t.switchToTab(t.currentTab)
			t.updateTabBar()
		}
		return nil
	case tcell.KeyCtrlC:
		// Quit application
		t.Stop()
		return nil
	}

	// Handle number keys for direct tab access (but not in edit mode)
	if !t.IsInEditMode() && event.Rune() >= '1' && event.Rune() <= '9' {
		tabIndex := int(event.Rune() - '1')
		if tabIndex < len(t.tabs) {
			t.currentTab = tabIndex
			t.switchToTab(t.currentTab)
			t.updateTabBar()
		}
		return nil
	}

	return event
}

// setSelectedEndpointPriority sets priority for the currently selected endpoint
func (t *TUIApp) setSelectedEndpointPriority(priority int) {
	if t.endpointsView == nil {
		return
	}
	
	// Get the currently selected endpoint name
	selectedEndpointName := t.getSelectedEndpointName()
	if selectedEndpointName == "" {
		t.AddLog("WARN", "没有选中的端点", "TUI")
		return
	}
	
	t.SetEndpointPriority(selectedEndpointName, priority)
}

// getSelectedEndpointName returns the name of the currently selected endpoint
func (t *TUIApp) getSelectedEndpointName() string {
	if t.endpointsView == nil {
		return ""
	}
	
	// Use the groupRowMap to get the correct endpoint for the selected row
	rowInfo, exists := t.endpointsView.groupRowMap[t.endpointsView.selectedRow]
	if !exists || rowInfo.IsGroupHeader || rowInfo.Endpoint == nil {
		return "" // Selected row is a group header or invalid
	}
	
	return rowInfo.Endpoint.Config.Name
}

// getSelectedEndpoint returns the selected endpoint object with group context
func (t *TUIApp) getSelectedEndpoint() *endpoint.Endpoint {
	if t.endpointsView == nil {
		return nil
	}
	
	// Use the groupRowMap to get the correct endpoint for the selected row
	rowInfo, exists := t.endpointsView.groupRowMap[t.endpointsView.selectedRow]
	if !exists || rowInfo.IsGroupHeader || rowInfo.Endpoint == nil {
		return nil // Selected row is a group header or invalid
	}
	
	return rowInfo.Endpoint
}

// switchToTab switches to the specified tab
func (t *TUIApp) switchToTab(tabIndex int) {
	if tabIndex >= 0 && tabIndex < len(t.tabs) {
		t.pages.SwitchToPage(t.tabs[tabIndex].Name)
		
		// Force update logs view when switching to it to show any missed logs
		if tabIndex == 3 && t.logsView != nil {
			t.logsView.ForceUpdate()
		}
	}
}

// updateTabBar updates the tab bar display
func (t *TUIApp) updateTabBar() {
	var tabText string
	for i, tab := range t.tabs {
		if i == t.currentTab {
			tabText += fmt.Sprintf(`[black:blue:b] %d: %s [white:black:-] `, i+1, tab.Name)
		} else {
			tabText += fmt.Sprintf(` [gray]%d: %s[white] `, i+1, tab.Name)
		}
	}
	tabText += `   [gray]Tab/Shift+Tab: Navigate  Ctrl+C: Quit[white]`
	t.tabBar.SetText(tabText)
}

// updateStatusBar updates the status bar
func (t *TUIApp) updateStatusBar() {
	metrics := t.monitoringMiddleware.GetMetrics().GetMetrics()
	
	// Basic status text
	statusText := fmt.Sprintf("Requests: %d | Success: %.1f%% | Connections: %d",
		metrics.TotalRequests,
		metrics.GetSuccessRate(),
		len(metrics.ActiveConnections),
	)
	
	// Add edit mode indicator
	if t.IsInEditMode() {
		isDirty := ""
		if t.HasUnsavedChanges() {
			isDirty = " *"
		}
		statusText += fmt.Sprintf(" | [编辑模式%s]", isDirty)
	}
	
	t.statusBar.SetText(statusText)
}

// Run starts the TUI application
func (t *TUIApp) Run() error {
	t.running = true
	
	// Start background refresh routine
	go t.refreshLoop()
	
	// Start the UI
	return t.app.Run()
}

// refreshLoop runs the background refresh routine
func (t *TUIApp) refreshLoop() {
	ticker := time.NewTicker(t.cfg.TUI.UpdateInterval)
	defer ticker.Stop()
	
	// Add a flag to prevent overlapping updates
	updating := false

	for t.running {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			if !t.running || updating {
				continue
			}
			
			updating = true
			// Use QueueUpdateDraw to ensure thread-safe UI updates
			t.app.QueueUpdateDraw(func() {
				defer func() { 
					updating = false 
					t.app.Sync()
				}()
				
				// Check if app is still running before updating
				if !t.running {
					return
				}
				
				// Update endpoint health in metrics first
				t.monitoringMiddleware.UpdateEndpointHealthStatus()
				
				// Update status bar
				t.updateStatusBar()
				
				// Update only the currently active view to reduce UI conflicts
				if t.currentTab >= 0 && t.currentTab < len(t.tabs) {
					switch t.currentTab {
					case 0:
						if t.overviewView != nil {
							t.overviewView.Update()
						}
					case 1:
						if t.endpointsView != nil {
							t.endpointsView.Update()
						}
					case 2:
						if t.connectionsView != nil {
							t.connectionsView.Update()
						}
					case 3:
						// Only update logs view when it's the active tab
						if t.logsView != nil {
							t.logsView.Update()
						}
					case 4:
						if t.configView != nil {
							t.configView.Update()
						}
					}
				}
			})
		}
	}
}

// AddLog adds a log entry to the logs view (thread-safe)
func (t *TUIApp) AddLog(level, message, source string) {
	if t.logsView != nil {
		// Only add log if logs tab is currently active to avoid unnecessary UI updates
		if t.currentTab == 3 {
			t.logsView.AddLog(level, message, source)
		} else {
			// Still add log to buffer but don't trigger UI update
			t.logsView.AddLogSilent(level, message, source)
		}
	}
}

// GetLogsView returns the logs view instance
func (t *TUIApp) GetLogsView() *LogsView {
	return t.logsView
}

// Stop gracefully stops the TUI application
func (t *TUIApp) Stop() {
	t.running = false
	t.cancel()
	t.app.Stop()
}

// IsRunning returns whether the TUI is currently running
func (t *TUIApp) IsRunning() bool {
	return t.running
}

// Edit mode methods for priority editing

// EnterEditMode enters the priority edit mode
func (t *TUIApp) EnterEditMode() {
	t.editMutex.Lock()
	defer t.editMutex.Unlock()
	
	if t.editMode {
		return // Already in edit mode
	}
	
	t.editMode = true
	t.isDirty = false
	
	// Initialize temp priorities with current config values
	// Use endpoint@group keys for same-name endpoints
	for _, endpoint := range t.cfg.Endpoints {
		groupName := endpoint.Group
		if groupName == "" {
			groupName = "Default"
		}
		endpointKey := fmt.Sprintf("%s@%s", endpoint.Name, groupName)
		t.tempPriorities[endpointKey] = endpoint.Priority
	}
	
	// Add log entry
	t.AddLog("INFO", "进入优先级编辑模式", "TUI")
}

// ExitEditMode exits the priority edit mode without saving changes
func (t *TUIApp) ExitEditMode() {
	t.editMutex.Lock()
	defer t.editMutex.Unlock()
	
	if !t.editMode {
		return // Not in edit mode
	}
	
	t.editMode = false
	t.isDirty = false
	
	// Clear temp priorities
	t.tempPriorities = make(map[string]int)
	
	// Add log entry
	t.AddLog("INFO", "退出优先级编辑模式", "TUI")
}

// IsInEditMode returns whether we're currently in edit mode
func (t *TUIApp) IsInEditMode() bool {
	t.editMutex.RLock()
	defer t.editMutex.RUnlock()
	return t.editMode
}

// SetEndpointPriority sets the priority for an endpoint in edit mode
func (t *TUIApp) SetEndpointPriority(endpointName string, priority int) {
	t.editMutex.Lock()
	defer t.editMutex.Unlock()
	
	if !t.editMode {
		return // Not in edit mode
	}
	
	// Validate priority range
	if priority < 1 || priority > 99 {
		t.AddLog("WARN", fmt.Sprintf("优先级超出范围 (1-99): %d", priority), "TUI")
		return
	}
	
	// Get the selected endpoint to determine its group context
	selectedEndpoint := t.getSelectedEndpoint()
	if selectedEndpoint == nil {
		t.AddLog("WARN", "未选中有效的端点", "TUI")
		return
	}
	
	// Create unique key using endpoint name and group for same-name endpoints
	groupName := selectedEndpoint.Config.Group
	if groupName == "" {
		groupName = "Default"
	}
	endpointKey := fmt.Sprintf("%s@%s", endpointName, groupName)
	
	oldPriority := t.tempPriorities[endpointKey]
	if oldPriority == 0 {
		// Get original priority from config
		oldPriority = selectedEndpoint.Config.Priority
	}
	
	t.tempPriorities[endpointKey] = priority
	t.isDirty = true
	
	t.AddLog("INFO", fmt.Sprintf("端点 %s (组: %s) 优先级: %d -> %d", 
		endpointName, groupName, oldPriority, priority), "TUI")
}

// GetEffectivePriority returns the effective priority for an endpoint (temp or config)
// For same-name endpoints, we need the group context to get the right endpoint
func (t *TUIApp) GetEffectivePriority(endpointName string) int {
	t.editMutex.RLock()
	defer t.editMutex.RUnlock()
	
	if t.editMode {
		// Try to find the priority in temp priorities using different possible keys
		// First, try to find by endpoint object if we have group context
		for _, endpoint := range t.cfg.Endpoints {
			if endpoint.Name == endpointName {
				groupName := endpoint.Group
				if groupName == "" {
					groupName = "Default"
				}
				endpointKey := fmt.Sprintf("%s@%s", endpointName, groupName)
				
				if priority, exists := t.tempPriorities[endpointKey]; exists {
					return priority
				}
			}
		}
		
		// Fallback to simple name lookup (for backward compatibility)
		if priority, exists := t.tempPriorities[endpointName]; exists {
			return priority
		}
	}
	
	// Return original config priority
	for _, endpoint := range t.cfg.Endpoints {
		if endpoint.Name == endpointName {
			return endpoint.Priority
		}
	}
	
	return 999 // Default high priority if not found
}

// GetEffectivePriorityForEndpoint returns the effective priority for a specific endpoint object
func (t *TUIApp) GetEffectivePriorityForEndpoint(endpoint *endpoint.Endpoint) int {
	t.editMutex.RLock()
	defer t.editMutex.RUnlock()
	
	if t.editMode {
		groupName := endpoint.Config.Group
		if groupName == "" {
			groupName = "Default"
		}
		endpointKey := fmt.Sprintf("%s@%s", endpoint.Config.Name, groupName)
		
		if priority, exists := t.tempPriorities[endpointKey]; exists {
			return priority
		}
		
		// Fallback to simple name lookup
		if priority, exists := t.tempPriorities[endpoint.Config.Name]; exists {
			return priority
		}
	}
	
	return endpoint.Config.Priority
}

// HasUnsavedChanges returns whether there are unsaved changes
func (t *TUIApp) HasUnsavedChanges() bool {
	t.editMutex.RLock()
	defer t.editMutex.RUnlock()
	return t.editMode && t.isDirty
}

// IsSaveEnabled returns whether saving to config file is enabled
func (t *TUIApp) IsSaveEnabled() bool {
	return t.cfg.TUI.SavePriorityEdits
}

// SavePrioritiesToConfig saves the temporary priorities to the config file
func (t *TUIApp) SavePrioritiesToConfig() error {
	t.editMutex.Lock()
	defer t.editMutex.Unlock()
	
	if !t.editMode || !t.isDirty {
		return nil // Nothing to save
	}
	
	// Apply temp priorities to the config
	for i := range t.cfg.Endpoints {
		endpoint := &t.cfg.Endpoints[i]
		groupName := endpoint.Group
		if groupName == "" {
			groupName = "Default"
		}
		endpointKey := fmt.Sprintf("%s@%s", endpoint.Name, groupName)
		
		if newPriority, exists := t.tempPriorities[endpointKey]; exists {
			endpoint.Priority = newPriority
		}
		// Also check for simple name key for backward compatibility
		if newPriority, exists := t.tempPriorities[endpoint.Name]; exists {
			endpoint.Priority = newPriority
		}
	}
	
	// **关键修复**: 同步配置到EndpointManager
	t.endpointManager.UpdateConfig(t.cfg)
	
	// 检查是否允许保存到配置文件
	if t.cfg.TUI.SavePriorityEdits {
		// 保存到配置文件（保留注释）
		if err := config.SavePriorityConfigWithComments(t.cfg, t.configPath); err != nil {
			t.AddLog("ERROR", fmt.Sprintf("保存配置文件失败: %v", err), "TUI")
			return err
		}
		t.AddLog("INFO", "配置已保存到文件并同步到路由系统，优先级更改已生效", "TUI")
	} else {
		t.AddLog("INFO", "优先级更改已应用到内存（配置文件保存已禁用）", "TUI")
	}
	
	t.isDirty = false
	
	return nil
}

// UpdateConfig updates the TUI configuration when config is reloaded
func (t *TUIApp) UpdateConfig(newCfg *config.Config) {
	t.editMutex.Lock()
	defer t.editMutex.Unlock()
	
	// Update configuration
	oldCfg := t.cfg
	t.cfg = newCfg
	
	// Clear temporary priorities when config changes to prevent stale data
	t.tempPriorities = make(map[string]int)
	t.isDirty = false
	
	// Update endpoint manager with new config
	t.endpointManager.UpdateConfig(newCfg)
	
	// Log configuration update
	t.AddLog("INFO", fmt.Sprintf("配置已重载 - 端点数量: %d -> %d", 
		len(oldCfg.Endpoints), len(newCfg.Endpoints)), "CONFIG")
}
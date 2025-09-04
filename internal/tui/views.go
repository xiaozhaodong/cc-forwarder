package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	
	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/monitor"
)

// OverviewView represents the overview tab
type OverviewView struct {
	container           *tview.Flex
	metricsBox          *tview.TextView
	chartBox            *tview.TextView
	endpointsBox        *tview.TextView
	systemBox           *tview.TextView
	monitoringMiddleware *middleware.MonitoringMiddleware
	endpointManager     *endpoint.Manager
	responseTimeHistory []time.Duration
	lastMetricsHash     string // Track metrics content changes
	lastEndpointsHash   string // Track endpoints content changes  
	lastSystemHash      string // Track system content changes
	startTime           time.Time // App start time for uptime calculation
}

// NewOverviewView creates a new overview view
func NewOverviewView(monitoringMiddleware *middleware.MonitoringMiddleware, endpointManager *endpoint.Manager, startTime time.Time) *OverviewView {
	view := &OverviewView{
		monitoringMiddleware: monitoringMiddleware,
		endpointManager:     endpointManager,
		responseTimeHistory: make([]time.Duration, 0, 60),
		startTime:           startTime,
	}
	view.setupUI()
	return view
}

func (v *OverviewView) setupUI() {
	v.metricsBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(false)
	v.metricsBox.SetBorder(true).SetTitle(" 📊 Request Metrics ").SetTitleAlign(tview.AlignLeft)

	v.chartBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(false)
	v.chartBox.SetBorder(true).SetTitle(" 🪙 Historical Token Usage ").SetTitleAlign(tview.AlignLeft)

	v.endpointsBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(false)
	v.endpointsBox.SetBorder(true).SetTitle(" 🎯 Endpoints Status ").SetTitleAlign(tview.AlignLeft)

	v.systemBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(false)
	v.systemBox.SetBorder(true).SetTitle(" 💻 System Info ").SetTitleAlign(tview.AlignLeft)

	topFlex := tview.NewFlex().
		AddItem(v.metricsBox, 0, 1, false).
		AddItem(v.chartBox, 0, 1, false)

	bottomFlex := tview.NewFlex().
		AddItem(v.endpointsBox, 0, 1, false).
		AddItem(v.systemBox, 0, 1, false)

	v.container = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topFlex, 12, 0, false).   // Increased height for top section (Request Metrics + Historical Token Usage)  
		AddItem(bottomFlex, 0, 1, false)  // Remaining space for bottom (Endpoints Status + System Info)
}

func (v *OverviewView) GetPrimitive() tview.Primitive {
	return v.container
}

func (v *OverviewView) Update() {
	metrics := v.monitoringMiddleware.GetMetrics().GetMetrics()
	
	// Update metrics with content change detection
	avgTime := formatDurationShort(metrics.GetAverageResponseTime())
	successRate := metrics.GetSuccessRate()
	
	// Get token usage statistics with safety checks
	tokenStats := metrics.GetTotalTokenStats()
	totalTokens := tokenStats.InputTokens + tokenStats.OutputTokens
	
	// Ensure non-negative values
	if tokenStats.InputTokens < 0 { tokenStats.InputTokens = 0 }
	if tokenStats.OutputTokens < 0 { tokenStats.OutputTokens = 0 }
	if tokenStats.CacheCreationTokens < 0 { tokenStats.CacheCreationTokens = 0 }
	if tokenStats.CacheReadTokens < 0 { tokenStats.CacheReadTokens = 0 }
	totalTokens = tokenStats.InputTokens + tokenStats.OutputTokens
	
	metricsText := fmt.Sprintf(`[white::b]Total Requests:[white::-] [cyan]%8d[white]
[white::b]Successful:[white::-] [green]%8d[white] ([green]%5.1f%%[white])
[white::b]Failed:[white::-] [red]%8d[white] ([red]%5.1f%%[white])
[white::b]Avg Response Time:[white::-] [cyan]%8s[white]

[yellow::b]🪙 Token Usage[white::-]
[white::b]📥 Input Tokens:[white::-] [cyan]%8d[white]
[white::b]📤 Output Tokens:[white::-] [cyan]%8d[white]
[white::b]🆕 Cache Creation:[white::-] [cyan]%8d[white]
[white::b]📖 Cache Read:[white::-] [cyan]%8d[white]
[white::b]🔢 Total Tokens:[white::-] [magenta]%8d[white]`,
		metrics.TotalRequests,
		metrics.SuccessfulRequests, successRate,
		metrics.FailedRequests, 100-successRate,
		avgTime,
		tokenStats.InputTokens,
		tokenStats.OutputTokens,
		tokenStats.CacheCreationTokens,
		tokenStats.CacheReadTokens,
		totalTokens)

	// Only update metrics if content changed
	if metricsText != v.lastMetricsHash {
		v.lastMetricsHash = metricsText
		v.metricsBox.SetText(metricsText)
	}
	
	// Historical token usage from past connections
	connectionHistory := metrics.ConnectionHistory
	
	// Show token usage for the last 3 connections that have token data
	var chartText strings.Builder
	chartText.WriteString("[yellow::b]🪙 Historical Token Usage[white::-]\n")
	chartText.WriteString("[gray]Past connections with token consumption:[white]\n\n")
	
	// Filter connections that have token usage and get the most recent 3
	var connectionsWithTokens []*monitor.ConnectionInfo
	for i := len(connectionHistory) - 1; i >= 0 && len(connectionsWithTokens) < 3; i-- {
		conn := connectionHistory[i]
		totalTokens := conn.TokenUsage.InputTokens + conn.TokenUsage.OutputTokens + 
					   conn.TokenUsage.CacheCreationTokens + conn.TokenUsage.CacheReadTokens
		if totalTokens > 0 {
			connectionsWithTokens = append(connectionsWithTokens, conn)
		}
	}
	
	if len(connectionsWithTokens) > 0 {
		for i, conn := range connectionsWithTokens {
			totalTokens := conn.TokenUsage.InputTokens + conn.TokenUsage.OutputTokens
			totalCacheTokens := conn.TokenUsage.CacheCreationTokens + conn.TokenUsage.CacheReadTokens
			
			// Format connection info
			clientIP := truncateString(conn.ClientIP, 12)
			endpoint := truncateString(conn.Endpoint, 10)
			if endpoint == "" || endpoint == "unknown" {
				endpoint = "pending"
			}
			
			// Status color
			statusColor := "green"
			statusText := "✓"
			if conn.Status == "failed" {
				statusColor = "red" 
				statusText = "✗"
			}
			
			chartText.WriteString(fmt.Sprintf("%d. [%s]%s[white] [cyan]%-12s[white] -> [yellow]%-10s[white]\n",
				i+1, statusColor, statusText, clientIP, endpoint))
			chartText.WriteString(fmt.Sprintf("   📥[cyan]%4d[white] 📤[cyan]%4d[white] 🆕[cyan]%3d[white] 📖[cyan]%3d[white] 🔢[magenta]%5d[white]\n\n",
				conn.TokenUsage.InputTokens, conn.TokenUsage.OutputTokens,
				conn.TokenUsage.CacheCreationTokens, conn.TokenUsage.CacheReadTokens,
				totalTokens + totalCacheTokens))
		}

		// Fill remaining lines if fewer than 3 connections
		for i := len(connectionsWithTokens); i < 3; i++ {
			chartText.WriteString(fmt.Sprintf("%d. [gray]─[white]\n\n", i+1))
		}
	} else {
		chartText.WriteString("[gray]No connections with token usage yet...\n")
		chartText.WriteString("Token consumption history will appear here\n")
		chartText.WriteString("after processing Claude API requests.\n\n")
		for i := 0; i < 3; i++ {
			chartText.WriteString(fmt.Sprintf("%d. [gray]─[white]\n\n", i+1))
		}
	}
	
	v.chartBox.SetText(chartText.String())
	
	// Endpoints status - maintain consistent formatting with group info
	endpoints := v.endpointManager.GetAllEndpoints()
	var statusText strings.Builder
	
	healthyCount := 0
	for _, ep := range endpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
	}
	
	// Show group summary with active group details
	groupManager := v.endpointManager.GetGroupManager()
	allGroups := groupManager.GetAllGroups()
	activeGroups := groupManager.GetActiveGroups()
	cooledGroupsCount := 0
	for _, group := range allGroups {
		if groupManager.IsGroupInCooldown(group.Name) {
			cooledGroupsCount++
		}
	}
	
	statusText.WriteString(fmt.Sprintf("[white::b]Total:[white::-] [cyan]%3d[white] | [white::b]Healthy:[white::-] [green]%3d[white]\n", len(endpoints), healthyCount))
	
	// Show current active group with priority
	if len(activeGroups) > 0 {
		activeGroup := activeGroups[0] // First active group (highest priority)
		statusText.WriteString(fmt.Sprintf("[white::b]Active Group:[white::-] [green]%s[white] (P:%d) | [cyan]%d[white]总组 ([red]%d冷却[white])\n\n", 
			activeGroup.Name, activeGroup.Priority, len(allGroups), cooledGroupsCount))
	} else {
		statusText.WriteString(fmt.Sprintf("[white::b]Groups:[white::-] [cyan]%2d[white] ([yellow]无活跃[white], [red]%d冷却[white])\n\n", 
			len(allGroups), cooledGroupsCount))
	}
	
	// Always show exactly 5 lines to maintain consistent height (reduced from 6 for group summary)
	for i := 0; i < 5; i++ {
		if i < len(endpoints) {
			ep := endpoints[i]
			status := ep.GetStatus()
			healthIcon := "[red]●[white]"
			if status.Healthy {
				healthIcon = "[green]●[white]"
			}
			
			// Get group info
			groupName := ep.Config.Group
			if groupName == "" {
				groupName = "Default"
			}
			
			// Check group status  
			groupStatusIcon := "[green]◆[white]"
			if groupManager.IsGroupInCooldown(groupName) {
				groupStatusIcon = "[red]◆[white]"
			} else {
				// Check if group is active
				activeGroups := groupManager.GetActiveGroups()
				isActive := false
				for _, group := range activeGroups {
					if group.Name == groupName {
						isActive = true
						break
					}
				}
				if !isActive {
					groupStatusIcon = "[gray]◆[white]"
				}
			}
			
			// Fixed width formatting to prevent jumping, with longer group name display
			statusText.WriteString(fmt.Sprintf("%s %s [cyan]%-8s[white] ([cyan]%-12s[white]) %3dms\n",
				healthIcon,
				groupStatusIcon,
				truncateString(ep.Config.Name, 8),
				truncateString(groupName, 12),
				status.ResponseTime.Milliseconds()))
		} else {
			// Fill empty lines to maintain height
			statusText.WriteString("\n")
		}
	}
	
	if len(endpoints) > 5 {
		statusText.WriteString("[gray]... and more[white]")
	}
	
	// Only update endpoints if content changed
	endpointsContent := statusText.String()
	if endpointsContent != v.lastEndpointsHash {
		v.lastEndpointsHash = endpointsContent
		v.endpointsBox.SetText(endpointsContent)
	}
	
	// System info - fixed width formatting
	uptime := time.Since(v.startTime)
	systemText := fmt.Sprintf(`[white::b]Active Connections:[white::-] [cyan]%6d[white]
[white::b]Total Connections:[white::-] [cyan]%7d[white]
[white::b]Uptime:[white::-] [cyan]%8s[white]`,
		len(metrics.ActiveConnections),
		len(metrics.ActiveConnections)+len(metrics.ConnectionHistory),
		formatUptimeShort(uptime))

	// Only update system info if content changed
	if systemText != v.lastSystemHash {
		v.lastSystemHash = systemText
		v.systemBox.SetText(systemText)
	}
}

// GroupRowInfo tracks information about each row in the grouped table
type GroupRowInfo struct {
	IsGroupHeader bool
	GroupName     string
	Endpoint      *endpoint.Endpoint
}

// EndpointsView represents the endpoints tab
type EndpointsView struct {
	container           *tview.Flex
	table               *tview.Table
	detailBox           *tview.TextView
	monitoringMiddleware *middleware.MonitoringMiddleware
	endpointManager     *endpoint.Manager
	tuiApp              *TUIApp  // Reference to main TUI app for edit mode
	selectedRow         int
	lastDetailHash      string // Track detail content changes
	groupRowMap         map[int]GroupRowInfo // Track which rows are groups vs endpoints
}

func NewEndpointsView(monitoringMiddleware *middleware.MonitoringMiddleware, endpointManager *endpoint.Manager) *EndpointsView {
	view := &EndpointsView{
		monitoringMiddleware: monitoringMiddleware,
		endpointManager:     endpointManager,
		groupRowMap:         make(map[int]GroupRowInfo),
	}
	view.setupUI()
	return view
}

func (v *EndpointsView) setupUI() {
	v.table = tview.NewTable().SetBorders(true).SetSelectable(true, false).
		SetFixed(1, 0) // Fix the header row (row 0) so it stays visible when scrolling
	
	// Update table title based on edit mode
	v.updateTableTitle()
	
	// Set up table selection change handler (auto-update on row change)
	v.table.SetSelectionChangedFunc(func(row, column int) {
		if row > 0 { // Skip header row
			v.selectedRow = row
			v.updateDetails()
		}
	})
	
	v.detailBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	v.detailBox.SetBorder(true).SetTitle(" 📊 Details ").SetTitleAlign(tview.AlignLeft)

	v.container = tview.NewFlex().
		AddItem(v.table, 0, 3, true).
		AddItem(v.detailBox, 0, 2, false)

	// Setup fixed headers
	v.setupTableHeaders()
}

// setupTableHeaders sets up the fixed table headers
func (v *EndpointsView) setupTableHeaders() {
	headers := []string{"Status", "Name", "Priority", "Resp", "Reqs", "Fails"}
	
	for col, header := range headers {
		cell := tview.NewTableCell(fmt.Sprintf("[white::b]%s[white::-]", header)).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		
		// Only Priority column should expand
		if col == 2 { // Priority column
			cell.SetExpansion(1)
		}
		
		v.table.SetCell(0, col, cell)
	}
}

// updateTableTitle updates the table title based on current mode
func (v *EndpointsView) updateTableTitle() {
	var title string
	if v.tuiApp != nil && v.tuiApp.IsInEditMode() {
		isDirty := ""
		if v.tuiApp.HasUnsavedChanges() {
			isDirty = " *"
		}
		
		// Check if saving is enabled
		saveHint := "Ctrl+S to Save"
		if v.tuiApp != nil && !v.tuiApp.IsSaveEnabled() {
			saveHint = "Ctrl+S Save (No File)"
		}
		
		title = fmt.Sprintf(" 🎯 Endpoints [Edit Mode%s - ESC to Exit %s] ", isDirty, saveHint)
	} else {
		title = " 🎯 Endpoints [Enter to Edit / Number Keys for Priority] "
	}
	v.table.SetBorder(true).SetTitle(title).SetTitleAlign(tview.AlignLeft)
}

func (v *EndpointsView) GetPrimitive() tview.Primitive {
	return v.container
}

// SetTUIApp sets the TUIApp reference for edit mode functionality
func (v *EndpointsView) SetTUIApp(app *TUIApp) {
	v.tuiApp = app
}

func (v *EndpointsView) Update() {
	// Update table title first
	v.updateTableTitle()
	
	v.updateTable()
	// Update details for currently selected row
	if v.selectedRow > 0 {
		v.updateDetails()
	} else {
		v.detailBox.SetText("[gray]Select an endpoint to view details[white]\n\n[yellow]Use arrow keys to navigate[white]")
	}
}

// updateTable updates the endpoints table efficiently with grouped format
func (v *EndpointsView) updateTable() {
	endpoints := v.endpointManager.GetAllEndpoints()
	metrics := v.monitoringMiddleware.GetMetrics().GetMetrics()
	
	// Group endpoints by group name
	groupedEndpoints := make(map[string][]*endpoint.Endpoint)
	for _, ep := range endpoints {
		groupName := ep.Config.Group
		if groupName == "" {
			groupName = "Default"
		}
		groupedEndpoints[groupName] = append(groupedEndpoints[groupName], ep)
	}
	
	// Get groups sorted by priority
	groupManager := v.endpointManager.GetGroupManager()
	allGroups := groupManager.GetAllGroups()
	
	// Clear existing table content but preserve headers
	v.table.Clear()
	v.setupTableHeaders()
	
	currentRow := 1 // Start from row 1 (row 0 is headers)
	v.groupRowMap = make(map[int]GroupRowInfo) // Track which rows are groups vs endpoints
	
	for _, group := range allGroups {
		groupEndpoints := groupedEndpoints[group.Name]
		if len(groupEndpoints) == 0 {
			continue
		}
		
		// Add group header row
		v.addGroupHeaderRow(currentRow, group, groupEndpoints)
		v.groupRowMap[currentRow] = GroupRowInfo{IsGroupHeader: true, GroupName: group.Name}
		currentRow++
		
		// Add endpoint rows for this group
		for _, ep := range groupEndpoints {
			v.addEndpointRow(currentRow, ep, metrics)
			v.groupRowMap[currentRow] = GroupRowInfo{IsGroupHeader: false, GroupName: group.Name, Endpoint: ep}
			currentRow++
		}
		
		// Add separator row between groups (empty row)
		if group != allGroups[len(allGroups)-1] { // Don't add separator after last group
			v.addSeparatorRow(currentRow)
			currentRow++
		}
	}
	
	// Auto-select first endpoint row if no row is selected
	if v.selectedRow == 0 && len(endpoints) > 0 {
		// Find first endpoint row (skip group headers)
		for row, info := range v.groupRowMap {
			if !info.IsGroupHeader && info.Endpoint != nil {
				v.table.Select(row, 0)
				v.selectedRow = row
				break
			}
		}
	}
}

// addGroupHeaderRow adds a group header row to the table
func (v *EndpointsView) addGroupHeaderRow(row int, group *endpoint.GroupInfo, groupEndpoints []*endpoint.Endpoint) {
	// Count healthy endpoints in this group
	healthyCount := 0
	for _, ep := range groupEndpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
	}
	
	// Determine group status and color
	groupManager := v.endpointManager.GetGroupManager()
	var groupStatusText, groupColor string
	
	if groupManager.IsGroupInCooldown(group.Name) {
		remaining := groupManager.GetGroupCooldownRemaining(group.Name)
		groupStatusText = fmt.Sprintf("Cooldown %ds", int(remaining.Seconds()))
		groupColor = "[red::b]"
	} else if group.IsActive {
		groupStatusText = "🟢"
		groupColor = "[green::b]"
	} else {
		groupStatusText = "⚫"
		groupColor = "[gray::b]"
	}
	
	// Create multi-line group header with full group name
	groupLine1 := fmt.Sprintf("%s %s P%d[white::-]", groupColor, group.Name, group.Priority)
	groupLine2 := fmt.Sprintf("%s %s %d/%d[white::-]", groupColor, groupStatusText, healthyCount, len(groupEndpoints))
	
	// Set group header cell spanning first 2 columns (Status, Name) with multi-line content
	groupHeaderText := fmt.Sprintf("%s\n%s", groupLine1, groupLine2)
	
	cell := tview.NewTableCell(groupHeaderText).
		SetTextColor(tcell.ColorWhite).
		SetAlign(tview.AlignLeft).
		SetSelectable(false).
		SetExpansion(1)
	
	v.table.SetCell(row, 0, cell)
	
	// Fill remaining columns with empty cells to maintain table structure
	for col := 1; col < 6; col++ {
		emptyCell := tview.NewTableCell("").
			SetSelectable(false)
		v.table.SetCell(row, col, emptyCell)
	}
}

// addEndpointRow adds an endpoint row to the table
func (v *EndpointsView) addEndpointRow(row int, ep *endpoint.Endpoint, metrics *monitor.Metrics) {
	status := ep.GetStatus()
	
	// Status icon
	statusIcon := "🔴"
	if status.Healthy {
		statusIcon = "🟢"
	}
	
	// Get endpoint stats
	endpointStats := metrics.EndpointStats[ep.Config.Name]
	totalReqs := int64(0)
	if endpointStats != nil {
		totalReqs = endpointStats.TotalRequests
	}
	
	// Get effective priority (temp or config)
	effectivePriority := ep.Config.Priority
	if v.tuiApp != nil {
		effectivePriority = v.tuiApp.GetEffectivePriorityForEndpoint(ep)
	}
	
	// Check if this is the highest priority endpoint in the group
	isHighestPriority := false
	if v.tuiApp != nil {
		// Find all endpoints in the same group
		groupName := ep.Config.Group
		if groupName == "" {
			groupName = "Default"
		}
		
		minPriority := 999
		allEndpoints := v.endpointManager.GetAllEndpoints()
		for _, endpoint := range allEndpoints {
			epGroupName := endpoint.Config.Group
			if epGroupName == "" {
				epGroupName = "Default"
			}
			if epGroupName == groupName {
				priority := v.tuiApp.GetEffectivePriorityForEndpoint(endpoint)
				if priority < minPriority {
					minPriority = priority
				}
			}
		}
		isHighestPriority = effectivePriority == minPriority
	}
	
	// Priority text with edit mode indicator
	priorityText := fmt.Sprintf("%d", effectivePriority)
	if v.tuiApp != nil && v.tuiApp.IsInEditMode() {
		priorityText += " [Edit]"
		if isHighestPriority {
			priorityText = fmt.Sprintf("[red::b]%d [Edit][white::-]", effectivePriority)
		}
	} else if isHighestPriority {
		priorityText = fmt.Sprintf("[green::b]%d[white::-]", effectivePriority)
	}
	
	// Set endpoint cells with indentation to show they belong to the group
	// Optimized column widths to prevent group from taking too much space
	cells := []string{
		fmt.Sprintf("  %s", statusIcon),                                    // Indented status
		fmt.Sprintf("  %s", truncateString(ep.Config.Name, 10)),           // Indented name (shorter)
		priorityText,                                                      // Priority
		fmt.Sprintf("%dms", status.ResponseTime.Milliseconds()),           // Response time
		fmt.Sprintf("%d", totalReqs),                                      // Requests
		fmt.Sprintf("%d", status.ConsecutiveFails),                        // Failures
	}
	
	for col, text := range cells {
		cell := tview.NewTableCell(text).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetSelectable(true)
		v.table.SetCell(row, col, cell)
	}
}

// addSeparatorRow adds a separator row between groups
func (v *EndpointsView) addSeparatorRow(row int) {
	for col := 0; col < 7; col++ {
		cell := tview.NewTableCell("").
			SetSelectable(false)
		v.table.SetCell(row, col, cell)
	}
}

// updateDetails updates the detail view for the selected endpoint
func (v *EndpointsView) updateDetails() {
	metrics := v.monitoringMiddleware.GetMetrics().GetMetrics()
	
	// Check if selected row is valid and get the row info
	rowInfo, exists := v.groupRowMap[v.selectedRow]
	if !exists || rowInfo.IsGroupHeader || rowInfo.Endpoint == nil {
		// Selected row is a group header or invalid, show group info or clear
		if exists && rowInfo.IsGroupHeader {
			v.showGroupDetails(rowInfo.GroupName)
		} else {
			v.detailBox.SetText("[gray]Select an endpoint to view details[white]\n\n[yellow]Use arrow keys to navigate[white]")
		}
		return
	}
	
	endpoint := rowInfo.Endpoint
	status := endpoint.GetStatus()
	
	var detailText strings.Builder
	detailText.WriteString(fmt.Sprintf("[blue::b]🎯 %s[white::-]\n", endpoint.Config.Name))
	
	// Group information
	groupName := endpoint.Config.Group
	if groupName == "" {
		groupName = "Default"
	}
	detailText.WriteString(fmt.Sprintf("[yellow::b]📋 Group Info[white::-]\n"))
	detailText.WriteString(fmt.Sprintf("Group: [cyan]%s[white] | Priority: [cyan]%d[white]\n", groupName, endpoint.Config.GroupPriority))
	
	// Basic Info - Use smart URL truncation
	detailText.WriteString("\n[yellow::b]📋 Basic Info[white::-]\n")
	detailText.WriteString(fmt.Sprintf("URL: [cyan]%s[white]\n", smartTruncateURL(endpoint.Config.URL, 35)))
	detailText.WriteString(fmt.Sprintf("Priority: [cyan]%d[white] | Timeout: [cyan]%v[white]\n", 
		endpoint.Config.Priority, endpoint.Config.Timeout))
	
	// Health Status - More compact format
	detailText.WriteString("\n[yellow::b]❤️ Health[white::-]\n")
	healthStatus := "[red]Unhealthy[white]"
	healthIcon := "🔴"
	if status.Healthy {
		healthStatus = "[green]Healthy[white]"
		healthIcon = "🟢"
	}
	detailText.WriteString(fmt.Sprintf("%s %s | [cyan]%dms[white] | Fails: [red]%d[white]\n", 
		healthIcon, healthStatus, status.ResponseTime.Milliseconds(), status.ConsecutiveFails))
	detailText.WriteString(fmt.Sprintf("Last Check: [cyan]%v[white]\n", status.LastCheck.Format("15:04:05")))
	
	// Performance Metrics - Only show if there's data
	if endpointStats := metrics.EndpointStats[endpoint.Config.Name]; endpointStats != nil && endpointStats.TotalRequests > 0 {
		detailText.WriteString("\n[yellow::b]📊 Performance[white::-]\n")
		
		// Compact metrics format
		successRate := float64(endpointStats.SuccessfulRequests) / float64(endpointStats.TotalRequests) * 100
		detailText.WriteString(fmt.Sprintf("Requests: [cyan]%s[white] | Success: [green]%.1f%%[white] | Retries: [yellow]%s[white]\n",
			formatLargeNumber(endpointStats.TotalRequests), successRate, formatLargeNumber(int64(endpointStats.RetryCount))))
		
		// Response time metrics
		avgResponseTime := endpointStats.TotalResponseTime / time.Duration(endpointStats.TotalRequests)
		detailText.WriteString(fmt.Sprintf("Avg: [cyan]%s[white] | Min: [cyan]%s[white] | Max: [cyan]%s[white]\n",
			formatDurationShort(avgResponseTime),
			formatDurationShort(endpointStats.MinResponseTime),
			formatDurationShort(endpointStats.MaxResponseTime)))
		
		// Last used info
		if !endpointStats.LastUsed.IsZero() {
			detailText.WriteString(fmt.Sprintf("Last Used: [cyan]%v[white]\n", endpointStats.LastUsed.Format("15:04:05")))
		}
		
		// Token Usage Metrics - Only show if there's significant token usage
		hasTokens := endpointStats.TokenUsage.InputTokens > 0 || endpointStats.TokenUsage.OutputTokens > 0 || 
					 endpointStats.TokenUsage.CacheCreationTokens > 0 || endpointStats.TokenUsage.CacheReadTokens > 0
		if hasTokens {
			detailText.WriteString("\n[yellow::b]🪙 Tokens[white::-]\n")
			
			// Compact token display
			totalTokens := endpointStats.TokenUsage.InputTokens + endpointStats.TokenUsage.OutputTokens
			totalCacheTokens := endpointStats.TokenUsage.CacheCreationTokens + endpointStats.TokenUsage.CacheReadTokens
			
			detailText.WriteString(fmt.Sprintf("📥 In: [cyan]%s[white] | 📤 Out: [cyan]%s[white] | 🔢 Total: [magenta]%s[white]\n",
				formatLargeNumber(int64(endpointStats.TokenUsage.InputTokens)),
				formatLargeNumber(int64(endpointStats.TokenUsage.OutputTokens)),
				formatLargeNumber(int64(totalTokens))))
			
			// Show cache tokens only if they exist
			if totalCacheTokens > 0 {
				detailText.WriteString(fmt.Sprintf("🆕 Cache Create: [cyan]%s[white] | 📖 Cache Read: [cyan]%s[white]\n",
					formatLargeNumber(int64(endpointStats.TokenUsage.CacheCreationTokens)),
					formatLargeNumber(int64(endpointStats.TokenUsage.CacheReadTokens))))
			}
		}
	} else {
		detailText.WriteString("\n[yellow::b]📊 Performance[white::-]\n")
		detailText.WriteString("[gray]No requests processed yet[white]\n")
	}
	
	// Active Connections - Only show if there are connections
	activeConnections := 0
	for _, conn := range metrics.ActiveConnections {
		if conn.Endpoint == endpoint.Config.Name {
			activeConnections++
		}
	}
	
	if activeConnections > 0 {
		detailText.WriteString(fmt.Sprintf("\n[yellow::b]🔌 Connections[white::-]\nActive: [cyan]%d[white]\n", activeConnections))
	} else {
		detailText.WriteString("\n[yellow::b]🔌 Connections[white::-]\n[gray]No active connections[white]\n")
	}
	
	// Add scrolling hint
	detailText.WriteString("\n[gray]↑↓ Arrow keys to scroll[white]")
	
	// Only update if content changed
	newContent := detailText.String()
	if newContent != v.lastDetailHash {
		v.lastDetailHash = newContent
		v.detailBox.SetText(newContent)
	}
}

// showGroupDetails shows details for a selected group header
func (v *EndpointsView) showGroupDetails(groupName string) {
	groupManager := v.endpointManager.GetGroupManager()
	allGroups := groupManager.GetAllGroups()
	
	var selectedGroup *endpoint.GroupInfo
	for _, group := range allGroups {
		if group.Name == groupName {
			selectedGroup = group
			break
		}
	}
	
	if selectedGroup == nil {
		v.detailBox.SetText("[gray]Group information not available[white]")
		return
	}
	
	var detailText strings.Builder
	detailText.WriteString(fmt.Sprintf("[blue::b]📂 Group: %s[white::-]\n\n", selectedGroup.Name))
	
	// Group status
	if groupManager.IsGroupInCooldown(selectedGroup.Name) {
		remaining := groupManager.GetGroupCooldownRemaining(selectedGroup.Name)
		detailText.WriteString(fmt.Sprintf("[red::b]❄️ Status: Cooldown (%ds remaining)[white::-]\n", int(remaining.Seconds())))
	} else if selectedGroup.IsActive {
		detailText.WriteString("[green::b]🟢 Status: Active[white::-]\n")
	} else {
		detailText.WriteString("[gray::b]⚫ Status: Standby[white::-]\n")
	}
	
	detailText.WriteString(fmt.Sprintf("Priority: [cyan]%d[white]\n", selectedGroup.Priority))
	detailText.WriteString(fmt.Sprintf("Endpoints: [cyan]%d[white]\n\n", len(selectedGroup.Endpoints)))
	
	// List endpoints in this group
	detailText.WriteString("[yellow::b]📋 Endpoints in Group[white::-]\n")
	for i, ep := range selectedGroup.Endpoints {
		status := ep.GetStatus()
		healthIcon := "🔴"
		if status.Healthy {
			healthIcon = "🟢"
		}
		
		detailText.WriteString(fmt.Sprintf("%d. %s %s (P:%d, %dms)\n", 
			i+1, healthIcon, ep.Config.Name, ep.Config.Priority, status.ResponseTime.Milliseconds()))
	}
	
	v.detailBox.SetText(detailText.String())
}

// ConnectionsView represents the connections tab
type ConnectionsView struct {
	container           *tview.Flex
	statsBox            *tview.TextView
	monitoringMiddleware *middleware.MonitoringMiddleware
	endpointManager     *endpoint.Manager  // Add endpoint manager reference
	config              *config.Config
	lastDisplayHash     string // Track content changes to avoid unnecessary updates
	needsUpdate         bool   // Flag to indicate if data has changed since last display
}

func NewConnectionsView(monitoringMiddleware *middleware.MonitoringMiddleware, endpointManager *endpoint.Manager, cfg *config.Config) *ConnectionsView {
	view := &ConnectionsView{
		monitoringMiddleware: monitoringMiddleware,
		endpointManager:     endpointManager,
		config:              cfg,
	}
	view.setupUI()
	return view
}

func (v *ConnectionsView) setupUI() {
	v.statsBox = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	v.statsBox.SetBorder(true).SetTitle(" 🔌 Active Connections ").SetTitleAlign(tview.AlignLeft)
	
	v.container = tview.NewFlex().AddItem(v.statsBox, 0, 1, true)
}

func (v *ConnectionsView) GetPrimitive() tview.Primitive {
	return v.container
}

func (v *ConnectionsView) Update() {
	metrics := v.monitoringMiddleware.GetMetrics().GetMetrics()
	
	// Build display text
	var stats strings.Builder
	stats.WriteString(fmt.Sprintf("[blue::b]📊 Connection Statistics[white::-]\n"))
	stats.WriteString(fmt.Sprintf("Active: [cyan]%3d[white] | Historical: [cyan]%4d[white]\n\n", 
		len(metrics.ActiveConnections), len(metrics.ConnectionHistory)))
	
	stats.WriteString("[blue::b]🔗 Active Connections[white::-]\n")
	
	// Convert map to slice for stable sorting
	connections := make([]*monitor.ConnectionInfo, 0, len(metrics.ActiveConnections))
	for _, conn := range metrics.ActiveConnections {
		connections = append(connections, conn)
	}
	
	// Sort connections by start time (newest first) for stable ordering
	sort.Slice(connections, func(i, j int) bool {
		return connections[i].StartTime.After(connections[j].StartTime)
	})
	
	// Always show exactly 15 lines to maintain consistent height
	connCount := 0
	for _, conn := range connections {
		if connCount >= 15 {
			break
		}
		duration := time.Since(conn.StartTime)
		
		// Display endpoint name and find its group
		endpointDisplay := conn.Endpoint
		groupName := "Unknown"
		if endpointDisplay == "" || endpointDisplay == "unknown" {
			endpointDisplay = "pending"
		} else {
			// Find the group for this endpoint
			endpoint := v.endpointManager.GetEndpointByNameAny(endpointDisplay)
			if endpoint != nil {
				if endpoint.Config.Group != "" {
					groupName = endpoint.Config.Group
				} else {
					groupName = "Default"
				}
			}
		}
		
		retryDisplay := ""
		if conn.RetryCount >= 0 {
			maxAttempts := v.config.Retry.MaxAttempts
			retryDisplay = fmt.Sprintf(" (%d/%d retry)", conn.RetryCount, maxAttempts)
		}
		
		stats.WriteString(fmt.Sprintf("  [cyan]%-12s[white] %-6s %-18s -> [yellow]%s[white]/[magenta]%s[white]%s [gray](%8s)[white]\n",
			truncateString(conn.ClientIP, 12),
			conn.Method,
			truncateString(conn.Path, 18),
			truncateString(endpointDisplay, 8),
			truncateString(groupName, 12),
			retryDisplay,
			formatDurationShort(duration)))
		connCount++
	}
	
	// Fill remaining lines to maintain consistent height
	for connCount < 15 {
		if connCount == 0 {
			stats.WriteString("  [gray]No active connections[white]\n")
		} else {
			stats.WriteString("\n")
		}
		connCount++
	}
	
	// Only update if content has changed
	newContent := stats.String()
	if newContent != v.lastDisplayHash {
		v.lastDisplayHash = newContent
		v.statsBox.SetText(newContent)
	}
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
}

// LogsView represents the logs tab
type LogsView struct {
	container       *tview.Flex
	logText         *tview.TextView
	logs            []LogEntry
	mutex           sync.RWMutex
	maxLogs         int
	lastDisplayHash string // Track content changes to avoid unnecessary updates
	needsUpdate     bool   // Flag to indicate if logs have changed since last display
}

func NewLogsView() *LogsView {
	view := &LogsView{
		logs:    make([]LogEntry, 0),
		maxLogs: 500,
	}
	view.setupUI()
	return view
}

func (v *LogsView) setupUI() {
	v.logText = tview.NewTextView().SetDynamicColors(false).SetScrollable(true).SetWrap(true)
	v.logText.SetBorder(true).SetTitle(" System Logs ").SetTitleAlign(tview.AlignLeft)
	
	v.container = tview.NewFlex().AddItem(v.logText, 0, 1, true)
}

func (v *LogsView) GetPrimitive() tview.Primitive {
	return v.container
}

func (v *LogsView) Update() {
	v.refreshLogDisplay()
}

func (v *LogsView) ForceUpdate() {
	v.mutex.Lock()
	v.needsUpdate = true // Force update regardless of current state
	v.mutex.Unlock()
	v.refreshLogDisplay()
}

func (v *LogsView) AddLog(level, message, source string) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    source,
	}
	
	v.logs = append(v.logs, entry)
	if len(v.logs) > v.maxLogs {
		v.logs = v.logs[len(v.logs)-v.maxLogs:]
	}
	v.needsUpdate = true
}

func (v *LogsView) AddLogSilent(level, message, source string) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    source,
	}
	
	v.logs = append(v.logs, entry)
	if len(v.logs) > v.maxLogs {
		v.logs = v.logs[len(v.logs)-v.maxLogs:]
	}
	// Don't set needsUpdate=true to avoid triggering UI refresh
}

func (v *LogsView) refreshLogDisplay() {
	v.mutex.RLock()
	needsUpdate := v.needsUpdate
	v.mutex.RUnlock()
	
	// Only update if there are new logs
	if !needsUpdate {
		return
	}
	
	v.mutex.Lock()
	defer v.mutex.Unlock()
	
	v.needsUpdate = false
	
	// Build display text
	var displayText strings.Builder
	
	start := 0
	if len(v.logs) > 50 {
		start = len(v.logs) - 50
	}
	
	for i := start; i < len(v.logs); i++ {
		entry := v.logs[i]
		timeStr := entry.Timestamp.Format("15:04:05")
		
		// Simplified log display without emojis and complex formatting
		var levelStr string
		switch strings.ToUpper(entry.Level) {
		case "ERROR":
			levelStr = "[ERR]"
		case "WARN":
			levelStr = "[WRN]"
		case "INFO":
			levelStr = "[INF]"
		default:
			levelStr = "[LOG]"
		}
		
		displayText.WriteString(fmt.Sprintf("%s %s %s: %s\n",
			timeStr, levelStr, entry.Source, entry.Message))
	}
	
	// Only update if content has changed
	newContent := displayText.String()
	if newContent != v.lastDisplayHash {
		v.lastDisplayHash = newContent
		v.logText.SetText(newContent)
		// Scroll to end after setting new text
		v.logText.ScrollToEnd()
	}
}


// ConfigView represents the config tab
type ConfigView struct {
	container  *tview.Flex
	configText *tview.TextView
	cfg        *config.Config
}

func NewConfigView(cfg *config.Config) *ConfigView {
	view := &ConfigView{cfg: cfg}
	view.setupUI()
	return view
}

func (v *ConfigView) setupUI() {
	v.configText = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	v.configText.SetBorder(true).SetTitle(" ⚙️ Configuration ").SetTitleAlign(tview.AlignLeft)
	
	v.container = tview.NewFlex().AddItem(v.configText, 0, 1, true)
}

func (v *ConfigView) GetPrimitive() tview.Primitive {
	return v.container
}

func (v *ConfigView) Update() {
	var details strings.Builder
	
	details.WriteString("[blue::b]🌐 Server[white::-]\n")
	details.WriteString(fmt.Sprintf("Host: [cyan]%s[white] | Port: [cyan]%d[white]\n\n", v.cfg.Server.Host, v.cfg.Server.Port))
	
	details.WriteString("[blue::b]🎯 Strategy[white::-]\n")
	details.WriteString(fmt.Sprintf("Type: [yellow]%s[white] | Fast Test: [yellow]%t[white]\n\n", 
		strings.Title(v.cfg.Strategy.Type), v.cfg.Strategy.FastTestEnabled))
	
	details.WriteString("[blue::b]🔐 Authentication[white::-]\n")
	if v.cfg.Auth.Enabled {
		details.WriteString("Status: [green]Enabled[white]\n")
	} else {
		details.WriteString("Status: [red]Disabled[white]\n")
	}
	details.WriteString("\n")
	
	details.WriteString("[blue::b]🖥️ TUI Settings[white::-]\n")
	details.WriteString(fmt.Sprintf("Update Interval: [cyan]%v[white]\n", v.cfg.TUI.UpdateInterval))
	
	saveStatus := "[red]Disabled[white]"
	saveHint := "Changes are applied to memory only"
	if v.cfg.TUI.SavePriorityEdits {
		saveStatus = "[green]Enabled[white]"
		saveHint = "Priority edits are saved to config file"
	}
	details.WriteString(fmt.Sprintf("Save Priority Edits: %s\n", saveStatus))
	details.WriteString(fmt.Sprintf("[gray]%s[white]\n\n", saveHint))
	
	details.WriteString("[blue::b]🎯 Endpoints[white::-]\n")
	details.WriteString(fmt.Sprintf("Total: [cyan]%d[white]\n", len(v.cfg.Endpoints)))
	for i, ep := range v.cfg.Endpoints {
		if i >= 8 {
			details.WriteString("[gray]... and more[white]\n")
			break
		}
		details.WriteString(fmt.Sprintf("  • [cyan]%s[white] ([yellow]%s[white]) P:%d\n",
			ep.Name, truncateString(ep.URL, 25), ep.Priority))
	}
	
	v.configText.SetText(details.String())
}

// Helper functions
func formatDurationShort(d time.Duration) string {
	if d == 0 {
		return "0ms"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fμs", float64(d.Nanoseconds())/1000)
	}
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Nanoseconds())/1000000)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatUptimeShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), math.Mod(d.Seconds(), 60))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", hours, minutes)
	} else {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		return fmt.Sprintf("%dd%dh", days, hours)
	}
}

// formatLargeNumber formats large numbers with K/M/B suffixes
func formatLargeNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		if n%1000 == 0 {
			return fmt.Sprintf("%dK", n/1000)
		}
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	} else if n < 1000000000 {
		if n%1000000 == 0 {
			return fmt.Sprintf("%dM", n/1000000)
		}
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	} else {
		return fmt.Sprintf("%.1fB", float64(n)/1000000000)
	}
}

// smartTruncateURL truncates URL intelligently showing domain and key path parts
func smartTruncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	
	// Try to preserve protocol and domain
	if len(url) > maxLen {
		// Find the domain part
		protocolEnd := strings.Index(url, "://")
		if protocolEnd == -1 {
			return truncateString(url, maxLen)
		}
		
		domainStart := protocolEnd + 3
		pathStart := strings.Index(url[domainStart:], "/")
		if pathStart == -1 {
			return truncateString(url, maxLen)
		}
		
		domain := url[:domainStart+pathStart]
		path := url[domainStart+pathStart:]
		
		// If domain itself is too long, just truncate normally
		if len(domain) >= maxLen-3 {
			return truncateString(url, maxLen)
		}
		
		// Calculate remaining space for path
		remaining := maxLen - len(domain) - 3 // 3 for "..."
		if remaining <= 0 {
			return domain + "..."
		}
		
		// Show beginning of path
		if len(path) <= remaining {
			return url
		}
		
		return domain + truncateString(path, remaining)
	}
	
	return truncateString(url, maxLen)
}

// formatCompactMetrics formats metrics in a more compact way
func formatCompactMetrics(label string, value interface{}) string {
	switch v := value.(type) {
	case int64:
		return fmt.Sprintf("%-12s: [cyan]%8s[white]", label, formatLargeNumber(v))
	case int:
		return fmt.Sprintf("%-12s: [cyan]%8s[white]", label, formatLargeNumber(int64(v)))
	case float64:
		if v >= 100 {
			return fmt.Sprintf("%-12s: [cyan]%8.0f[white]", label, v)
		} else {
			return fmt.Sprintf("%-12s: [cyan]%8.1f[white]", label, v)
		}
	case string:
		return fmt.Sprintf("%-12s: [cyan]%8s[white]", label, v)
	default:
		return fmt.Sprintf("%-12s: [cyan]%8v[white]", label, v)
	}
}
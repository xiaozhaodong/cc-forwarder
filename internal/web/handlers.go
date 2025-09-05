package web

// This file now acts as the main handlers file after refactoring.
// All specific handler implementations have been moved to separate files:
// - utils.go: Utility functions
// - templates.go: HTML templates
// - basic_handlers.go: Basic API handlers
// - sse_handlers.go: Server-Sent Events handlers
// - broadcast_handlers.go: Event broadcasting handlers
// - metrics_handlers.go: Metrics and statistics handlers
// - chart_handlers.go: Chart data handlers
// - group_handlers.go: Group management handlers
// - suspended_handlers.go: Suspended requests handlers
// - usage_handlers.go: Usage tracking handlers

// All handler methods are still WebServer methods and maintain the same interface.
// No changes are needed to the routing or other components that use these handlers.
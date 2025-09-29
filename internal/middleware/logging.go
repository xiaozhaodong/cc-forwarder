package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/internal/tracking"
)

// LoggingMiddleware provides request/response logging
type LoggingMiddleware struct {
	logger            *slog.Logger
	monitoringMiddleware *MonitoringMiddleware
	usageTracker      *tracking.UsageTracker
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger *slog.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: logger,
	}
}

// SetMonitoringMiddleware sets the monitoring middleware reference
func (lm *LoggingMiddleware) SetMonitoringMiddleware(mm *MonitoringMiddleware) {
	lm.monitoringMiddleware = mm
}

// SetUsageTracker sets the usage tracker reference
func (lm *LoggingMiddleware) SetUsageTracker(ut *tracking.UsageTracker) {
	lm.usageTracker = ut
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int64
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.statusCode == 0 {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += int64(n)
	return n, err
}

// Wrap wraps an HTTP handler with logging
func (lm *LoggingMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		clientIP := getClientIP(r)
		userAgent := truncateString(r.UserAgent(), 50)
		
		// Record request start in metrics - we'll update the endpoint later
		var connID string
		if lm.monitoringMiddleware != nil {
			connID = lm.monitoringMiddleware.RecordRequest("unknown", clientIP, userAgent, r.Method, r.URL.Path)
		}
		
		// Store connection ID in request context for use by proxy handler
		r = r.WithContext(context.WithValue(r.Context(), "conn_id", connID))
		
		// Wrap response writer
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     0,
			bytes:          0,
		}

		// Store request info (request start logging handled by lifecycle manager)
		lm.logger.Debug(fmt.Sprintf("📝 [请求接收] [%s] %s %s", connID, r.Method, r.URL.Path), 
			"method", r.Method,
			"path", r.URL.Path,
			"client_ip", clientIP,
			"user_agent", userAgent,
			"content_length", r.ContentLength,
			"conn_id", connID,
		)

		// Process request
		next.ServeHTTP(rw, r)

		// Calculate duration
		duration := time.Since(start)

		// Get endpoint info from context (set by retry handler)
		selectedEndpoint := "unknown"
		if ep, ok := r.Context().Value("selected_endpoint").(string); ok {
			selectedEndpoint = ep
		}

		// 🔧 [HTTP状态码修复] 优先从上下文读取最终状态码，解决流式请求中WriteHeader时机问题
		finalStatusCode := rw.statusCode
		if ctxStatusCode, ok := r.Context().Value("final_status_code").(int); ok && ctxStatusCode != 0 {
			finalStatusCode = ctxStatusCode
		}

		// Record response in metrics
		if lm.monitoringMiddleware != nil && connID != "" {
			lm.monitoringMiddleware.RecordResponse(connID, finalStatusCode, duration, rw.bytes, selectedEndpoint)
		}

		// Log response details (completion logging handled by lifecycle manager)
		statusEmoji := getStatusEmoji(finalStatusCode)
		lm.logger.Debug(fmt.Sprintf("%s [请求详情] [%s] %s %s → %d (%s)", statusEmoji, connID, r.Method, r.URL.Path, finalStatusCode, formatDuration(duration)),
			"method", r.Method,
			"path", r.URL.Path,
			"endpoint", selectedEndpoint,
			"status_code", finalStatusCode,
			"bytes_written", formatBytes(rw.bytes),
			"duration", formatDuration(duration),
			"client_ip", clientIP,
			"conn_id", connID,
		)

		// Log slow requests as warnings
		if duration > 10*time.Second {
			lm.logger.Warn(fmt.Sprintf("🐌 Slow request detected [%s]", connID),
				"method", r.Method,
				"path", r.URL.Path,
				"endpoint", selectedEndpoint,
				"duration", formatDuration(duration),
				"status_code", finalStatusCode,
				"conn_id", connID,
			)
		}

		// Log errors
		if finalStatusCode >= 400 {
			level := slog.LevelWarn
			emoji := "⚠️"
			if finalStatusCode >= 500 {
				level = slog.LevelError
				emoji = "❌"
			}

			lm.logger.Log(r.Context(), level, fmt.Sprintf("%s Error details", emoji),
				"method", r.Method,
				"path", r.URL.Path,
				"endpoint", selectedEndpoint,
				"status_code", finalStatusCode,
				"duration", formatDuration(duration),
				"conn_id", connID,
			)
		}
	})
}

// Helper functions for better log formatting

func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	// Check for X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Use RemoteAddr as fallback
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getStatusEmoji(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "✅"
	case statusCode >= 300 && statusCode < 400:
		return "🔄"
	case statusCode >= 400 && statusCode < 500:
		return "⚠️"
	case statusCode >= 500:
		return "❌"
	default:
		return "❓"
	}
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fμs", float64(d.Nanoseconds())/1000)
	} else if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1000000)
	} else {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
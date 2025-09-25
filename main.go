package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/logging"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
	"cc-forwarder/internal/transport"
	"cc-forwarder/internal/tui"
	"cc-forwarder/internal/utils"
	"cc-forwarder/internal/web"
)

var (
	configPath      = flag.String("config", "config/example.yaml", "Path to configuration file")
	showVersion     = flag.Bool("version", false, "Show version information")
	enableTUI       = flag.Bool("tui", true, "Enable TUI interface (default: true)")
	disableTUI      = flag.Bool("no-tui", false, "Disable TUI interface")
	enableWeb       = flag.Bool("web", false, "Enable Web interface")
	webPort         = flag.Int("web-port", 8088, "Web interface port (default: 8088)")
	primaryEndpoint = flag.String("p", "", "Set primary endpoint with highest priority (endpoint name)")

	// Build-time variables (set via ldflags)
	version = "dev"
	commit  = "unknown"
	date    = "unknown"

	// Runtime variables
	startTime         = time.Now()
	currentLogHandler *SimpleHandler // Track current log handler for cleanup
)

func main() {
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("Claude Request Forwarder\n")
		fmt.Printf("Version: %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", date)
		os.Exit(0)
	}

	// Determine TUI mode
	tuiEnabled := *enableTUI && !*disableTUI

	// Setup initial logger (will be updated when config is loaded)
	logger := setupLogger(config.LoggingConfig{Level: "info", Format: "text"}, nil)
	slog.SetDefault(logger)

	// Create configuration watcher
	configWatcher, err := config.NewConfigWatcher(*configPath, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create configuration watcher: %v\n", err)
		os.Exit(1)
	}
	defer configWatcher.Close()

	// Get initial configuration
	cfg := configWatcher.GetConfig()

	// Apply command line primary endpoint override
	if *primaryEndpoint != "" {
		cfg.PrimaryEndpoint = *primaryEndpoint
		if err := cfg.ApplyPrimaryEndpoint(logger); err != nil {
			logger.Error(fmt.Sprintf("❌ 主端点配置失败: %v", err))
			os.Exit(1)
		}
	}

	// Apply Web configuration from command line
	if *enableWeb {
		cfg.Web.Enabled = true
	}
	if *webPort != 8088 { // 只有当用户显式指定了端口时才覆盖
		cfg.Web.Port = *webPort
	}

	// Apply TUI configuration from config file and command line
	if cfg.TUI.UpdateInterval == 0 {
		cfg.TUI.UpdateInterval = 1 * time.Second // Default
	}

	// Command line flags override config file
	if *disableTUI {
		tuiEnabled = false
	} else if cfg != nil {
		// Use config file setting
		tuiEnabled = cfg.TUI.Enabled
	}

	// Update logger with config settings (TUI will be added later)
	logger = setupLogger(cfg.Logging, nil)
	slog.SetDefault(logger)

	// 🔧 Initialize debug configuration
	utils.SetDebugConfig(cfg)
	if cfg.Logging.TokenDebug.Enabled {
		logger.Info("🔍 Token调试功能已启用", "save_path", cfg.Logging.TokenDebug.SavePath)
	}

	if tuiEnabled {
		logger.Info("🖥️ TUI模式已启用，启动图形化监控界面")
	} else {
		logger.Info("🚀 Claude Request Forwarder 启动中... (无TUI模式)",
			"version", version,
			"commit", commit,
			"build_date", date,
			"config_file", *configPath,
			"endpoints_count", len(cfg.Endpoints),
			"strategy", cfg.Strategy.Type)
	}

	// Display proxy configuration (only in non-TUI mode)
	if !tuiEnabled {
		if cfg.Proxy.Enabled {
			proxyInfo := transport.GetProxyInfo(cfg)
			logger.Info("🔗 " + proxyInfo)
		} else {
			logger.Info("🔗 代理未启用，将直接连接目标端点")
		}

		// Display security information during startup
		if cfg.Auth.Enabled {
			logger.Info("🔐 鉴权已启用，访问需要Bearer Token验证")
		} else {
			logger.Info("🔓 鉴权已禁用，所有请求将直接转发")
			if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && cfg.Server.Host != "::1" {
				logger.Warn("⚠️  注意：将在非本地地址启动但未启用鉴权，请确保网络环境安全")
			}
		}
	}

	// Create endpoint manager
	endpointManager := endpoint.NewManager(cfg)
	endpointManager.Start()
	defer endpointManager.Stop()

	// Initialize EventBus
	eventBus := events.NewEventBus(logger)
	err = eventBus.Start()
	if err != nil {
		logger.Error(fmt.Sprintf("❌ EventBus启动失败: %v", err))
		os.Exit(1)
	}
	defer func() {
		if err := eventBus.Stop(); err != nil {
			logger.Error(fmt.Sprintf("❌ EventBus关闭失败: %v", err))
		}
	}()

	// Initialize usage tracker
	trackingConfig := &tracking.Config{
		Enabled:         cfg.UsageTracking.Enabled,
		DatabasePath:    cfg.UsageTracking.DatabasePath,
		Database:        cfg.UsageTracking.Database, // 直接使用新配置
		BufferSize:      cfg.UsageTracking.BufferSize,
		BatchSize:       cfg.UsageTracking.BatchSize,
		FlushInterval:   cfg.UsageTracking.FlushInterval,
		MaxRetry:        cfg.UsageTracking.MaxRetry,
		RetentionDays:   cfg.UsageTracking.RetentionDays,
		CleanupInterval: cfg.UsageTracking.CleanupInterval,
		ModelPricing:    convertModelPricing(cfg.UsageTracking.ModelPricing),
		DefaultPricing:  convertModelPricingSingle(cfg.UsageTracking.DefaultPricing),
	}

	usageTracker, err := tracking.NewUsageTracker(trackingConfig, cfg.Timezone)
	if err != nil {
		logger.Error(fmt.Sprintf("❌ 使用跟踪器初始化失败: %v", err))
		os.Exit(1)
	}
	defer func() {
		if usageTracker != nil {
			if err := usageTracker.Close(); err != nil {
				logger.Error(fmt.Sprintf("❌ 使用跟踪器关闭失败: %v", err))
			}
		}
	}()

	// Create proxy handler
	proxyHandler := proxy.NewHandler(endpointManager, cfg)
	
	// Connect EventBus to proxy handler  
	proxyHandler.SetEventBus(eventBus)

	// Create middleware
	loggingMiddleware := middleware.NewLoggingMiddleware(logger)
	monitoringMiddleware := middleware.NewMonitoringMiddleware(endpointManager)
	authMiddleware := middleware.NewAuthMiddleware(cfg.Auth)

	// Connect EventBus to components
	endpointManager.SetEventBus(eventBus)
	monitoringMiddleware.SetEventBus(eventBus)
	// Set usage tracker for middleware components
	loggingMiddleware.SetUsageTracker(usageTracker)

	// Set usage tracker for proxy handler and retry handler
	if proxyHandler != nil {
		proxyHandler.SetUsageTracker(usageTracker)
	}
	retryHandler := proxyHandler.GetRetryHandler()
	if retryHandler != nil {
		retryHandler.SetUsageTracker(usageTracker)
	}

	// Connect logging and monitoring middlewares
	loggingMiddleware.SetMonitoringMiddleware(monitoringMiddleware)
	proxyHandler.SetMonitoringMiddleware(monitoringMiddleware)

	// Store tuiApp and webServer references for configuration reloads
	var tuiApp *tui.TUIApp
	var webServer *web.WebServer

	// Setup configuration reload callback to update components
	configWatcher.AddReloadCallback(func(newCfg *config.Config) {
		// Update logger (pass current tuiApp)
		newLogger := setupLogger(newCfg.Logging, tuiApp)
		slog.SetDefault(newLogger)

		// Update config watcher's logger too
		configWatcher.UpdateLogger(newLogger)

		// Update endpoint manager
		endpointManager.UpdateConfig(newCfg)

		// Update proxy handler
		proxyHandler.UpdateConfig(newCfg)

		// Update auth middleware
		authMiddleware.UpdateConfig(newCfg.Auth)

		// Update TUI if enabled
		if tuiApp != nil {
			tuiApp.UpdateConfig(newCfg)
		}

		// Update Web server if enabled
		if webServer != nil {
			webServer.UpdateConfig(newCfg)
		}

		// Update usage tracker pricing if enabled
		if usageTracker != nil && newCfg.UsageTracking.Enabled {
			usageTracker.UpdatePricing(convertModelPricing(newCfg.UsageTracking.ModelPricing))
		}

		if !tuiEnabled {
			newLogger.Info("🔄 所有组件已更新为新配置")
		}
	})

	if !tuiEnabled {
		logger.Info("🔄 配置文件自动重载已启用")
	}

	// Setup HTTP server
	mux := http.NewServeMux()

	// Register monitoring endpoints
	monitoringMiddleware.RegisterHealthEndpoint(mux)

	// Add usage tracker health check endpoint
	if usageTracker != nil {
		mux.HandleFunc("/health/usage-tracker", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			if err := usageTracker.HealthCheck(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(fmt.Sprintf("Usage Tracker unhealthy: %v", err)))
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Usage Tracker healthy"))
		})
	}

	// Register proxy handler for all other requests with middleware chain
	mux.Handle("/", loggingMiddleware.Wrap(authMiddleware.Wrap(proxyHandler)))

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 0, // No write timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if !tuiEnabled {
			logger.Info("🌐 HTTP 服务器启动中...",
				"address", server.Addr,
				"endpoints_count", len(cfg.Endpoints))
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Check if server started successfully
	select {
	case err := <-serverErr:
		logger.Error(fmt.Sprintf("❌ 服务器启动失败: %v", err))
		os.Exit(1)
	default:
		// Server started successfully
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)

		if !tuiEnabled {
			logger.Info("✅ 服务器启动成功！")
			logger.Info("📋 配置说明：请在 Claude Code 的 settings.json 中设置")
			logger.Info("🔧 ANTHROPIC_BASE_URL: " + baseURL)
			logger.Info("📡 服务器地址: " + baseURL)

			// Security warning for non-localhost addresses
			if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && cfg.Server.Host != "::1" {
				if !cfg.Auth.Enabled {
					logger.Warn("⚠️  安全警告：服务器绑定到非本地地址但未启用鉴权！")
					logger.Warn("🔒 强烈建议启用鉴权以保护您的端点访问")
					logger.Warn("📝 在配置文件中设置 auth.enabled: true 和 auth.token 来启用鉴权")
				} else {
					logger.Info("🔒 已启用鉴权保护，服务器可安全对外开放")
				}
			}
		}
	}

	// Start Web server if enabled
	if cfg.Web.Enabled {
		webServer = web.NewWebServer(cfg, endpointManager, monitoringMiddleware, usageTracker, logger, startTime, *configPath, eventBus)
		if err := webServer.Start(); err != nil {
			logger.Error(fmt.Sprintf("❌ Web服务器启动失败: %v", err))
		}
	}

	// Start TUI if enabled
	if tuiEnabled {
		tuiApp = tui.NewTUIApp(cfg, endpointManager, monitoringMiddleware, startTime, *configPath)

		// Update logger to send logs to TUI as well
		logger = setupLogger(cfg.Logging, tuiApp)
		slog.SetDefault(logger)

		// Update config watcher's logger to use TUI-enabled logger
		configWatcher.UpdateLogger(logger)

		// Run TUI in a goroutine
		tuiErr := make(chan error, 1)
		go func() {
			tuiErr <- tuiApp.Run()
		}()

		// Wait for TUI to exit or server error
		select {
		case err := <-serverErr:
			logger.Error(fmt.Sprintf("❌ 服务器运行时错误(在TUI模式): %v", err))
			if tuiApp != nil {
				tuiApp.Stop()
			}
			os.Exit(1)
		case err := <-tuiErr:
			logger.Info("📱 TUI界面已关闭")
			if err != nil {
				logger.Error(fmt.Sprintf("TUI运行错误: %v", err))
			}
		}
	} else {
		// Wait for interrupt signal in non-TUI mode
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

		// Block until we receive a signal or server error
		select {
		case err := <-serverErr:
			logger.Error(fmt.Sprintf("❌ 服务器运行时错误(在控制台模式): %v", err))
			os.Exit(1)
		case sig := <-interrupt:
			logger.Info(fmt.Sprintf("📡 收到终止信号，开始优雅关闭... - 信号: %v", sig))
		}
	}

	// Graceful shutdown
	if !tuiEnabled {
		logger.Info("🛑 正在关闭服务器...")
	}

	// Close log file handler before shutdown
	if currentLogHandler != nil {
		currentLogHandler.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Close Web server if running
	if webServer != nil {
		webServer.Stop(ctx)
	}

	if err := server.Shutdown(ctx); err != nil {
		logger.Error(fmt.Sprintf("❌ 服务器关闭失败: %v", err))
		os.Exit(1)
	}

	if !tuiEnabled {
		logger.Info("✅ 服务器已安全关闭")
	}
}

// setupLogger configures the structured logger
func setupLogger(cfg config.LoggingConfig, tuiApp *tui.TUIApp) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var fileRotator *logging.FileRotator
	// Setup file logging if enabled
	if cfg.FileEnabled {
		maxSize, err := logging.ParseSize(cfg.MaxFileSize)
		if err != nil {
			fmt.Printf("警告：无法解析日志文件大小配置 '%s'，使用默认值 100MB: %v\n", cfg.MaxFileSize, err)
			maxSize = 100 * 1024 * 1024 // 100MB
		}

		fileRotator, err = logging.NewFileRotator(cfg.FilePath, maxSize, cfg.MaxFiles, cfg.CompressRotated)
		if err != nil {
			fmt.Printf("警告：无法创建日志文件轮转器: %v\n", err)
			fileRotator = nil
		}
	}

	var handler slog.Handler
	// Create a custom handler that only outputs the message
	handler = &SimpleHandler{
		level:                    level,
		tuiApp:                   tuiApp,
		fileRotator:              fileRotator,
		disableFileResponseLimit: cfg.FileEnabled && cfg.DisableResponseLimit,
	}
	currentLogHandler = handler.(*SimpleHandler) // Store reference for cleanup

	// Debug: print file logging configuration
	if cfg.FileEnabled {
		fmt.Printf("🔧 文件日志已启用: 路径=%s, 禁用响应限制=%v\n", cfg.FilePath, cfg.DisableResponseLimit)
	}

	return slog.New(handler)
}

// SimpleHandler only outputs the log message without any metadata
type SimpleHandler struct {
	level                    slog.Level
	tuiApp                   *tui.TUIApp
	fileRotator              *logging.FileRotator
	disableFileResponseLimit bool // Whether to disable response limit for file output
}

func (h *SimpleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *SimpleHandler) Handle(_ context.Context, r slog.Record) error {
	message := r.Message

	// ✅ 添加结构化日志参数处理
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value))
		return true
	})

	// 如果有参数，将它们添加到消息中
	if len(attrs) > 0 {
		message = message + " " + strings.Join(attrs, " ")
	}

	// Format log message with enhanced timestamp and process info
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	pid := os.Getpid()
	gid := getGoroutineID()
	level := "INFO"
	switch r.Level {
	case slog.LevelDebug:
		level = "DEBUG"
	case slog.LevelWarn:
		level = "WARN"
	case slog.LevelError:
		level = "ERROR"
	}

	// For file output - use full message if response limit is disabled
	if h.fileRotator != nil {
		fileMessage := message
		// If disable file response limit is TRUE, don't truncate; if FALSE, truncate
		if !h.disableFileResponseLimit && len(message) > 500 {
			fileMessage = message[:500] + "... (文件日志截断)"
		}
		// When disableFileResponseLimit is true, fileMessage = message (no truncation)
		formattedMessage := fmt.Sprintf("[%s] [PID:%d] [GID:%d] [%s] %s\n", timestamp, pid, gid, level, fileMessage)
		h.fileRotator.Write([]byte(formattedMessage))
	}

	// For UI/console output - always limit message length
	displayMessage := message
	if len(displayMessage) > 500 {
		displayMessage = displayMessage[:500] + "... (显示截断)"
	}

	// Send to TUI if available
	if h.tuiApp != nil {
		h.tuiApp.AddLog(level, displayMessage, "system")
	} else {
		// Only output to console when TUI is not available - with enhanced format
		consoleMessage := fmt.Sprintf("[%s] [PID:%d] [GID:%d] [%s] %s", timestamp, pid, gid, level, displayMessage)
		fmt.Println(consoleMessage)
	}

	return nil
}

func (h *SimpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Return the same handler since we don't use attributes
	return h
}

func (h *SimpleHandler) WithGroup(name string) slog.Handler {
	// Return the same handler since we don't use groups
	return h
}

// Close gracefully closes the handler and syncs any buffered data
func (h *SimpleHandler) Close() error {
	if h.fileRotator != nil {
		h.fileRotator.Sync()
		return h.fileRotator.Close()
	}
	return nil
}

// getGoroutineID extracts the goroutine ID from runtime stack trace
func getGoroutineID() int {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	idField := strings.Fields(string(buf))[1]
	id, err := strconv.Atoi(idField)
	if err != nil {
		return 0
	}
	return id
}

// 添加类型转换函数
func convertModelPricing(configPricing map[string]config.ModelPricing) map[string]tracking.ModelPricing {
	if configPricing == nil {
		return nil
	}

	result := make(map[string]tracking.ModelPricing)
	for model, pricing := range configPricing {
		result[model] = tracking.ModelPricing{
			Input:         pricing.Input,
			Output:        pricing.Output,
			CacheCreation: pricing.CacheCreation,
			CacheRead:     pricing.CacheRead,
		}
	}
	return result
}

func convertModelPricingSingle(configPricing config.ModelPricing) tracking.ModelPricing {
	return tracking.ModelPricing{
		Input:         configPricing.Input,
		Output:        configPricing.Output,
		CacheCreation: configPricing.CacheCreation,
		CacheRead:     configPricing.CacheRead,
	}
}

func convertTrackingToConfigPricing(trackingPricing map[string]tracking.ModelPricing) map[string]config.ModelPricing {
	if trackingPricing == nil {
		return nil
	}

	result := make(map[string]config.ModelPricing)
	for model, pricing := range trackingPricing {
		result[model] = config.ModelPricing{
			Input:         pricing.Input,
			Output:        pricing.Output,
			CacheCreation: pricing.CacheCreation,
			CacheRead:     pricing.CacheRead,
		}
	}
	return result
}

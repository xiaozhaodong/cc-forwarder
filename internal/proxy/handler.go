package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/proxy/response"
	"cc-forwarder/internal/tracking"
)

// Context key for endpoint information
type contextKey string

const EndpointContextKey = contextKey("endpoint")

// Handler handles HTTP proxy requests
type Handler struct {
	endpointManager      *endpoint.Manager
	config               *config.Config
	retryHandler         *RetryHandler
	usageTracker         *tracking.UsageTracker
	monitoringMiddleware *middleware.MonitoringMiddleware
	responseProcessor    *response.Processor
	tokenAnalyzer        *response.TokenAnalyzer
	forwarder            *handlers.Forwarder
	regularHandler       *handlers.RegularHandler
	streamingHandler     *handlers.StreamingHandler
	eventBus             events.EventBus  // EventBus事件总线
	// 🔧 [Critical修复] 保存共享的SuspensionManager实例的引用
	// 确保在SetUsageTracker中重建Handler时保持共享状态
	sharedSuspensionManager handlers.SuspensionManager
	// 🚀 [端点自愈] 端点恢复信号管理器
	recoverySignalManager *EndpointRecoverySignalManager
}

// TokenParserProviderImpl 实现TokenParserProvider接口
type TokenParserProviderImpl struct{}

// NewTokenParser 创建新的TokenParser实例
func (p *TokenParserProviderImpl) NewTokenParser() response.TokenParser {
	return NewTokenParser()
}

// NewTokenParserWithUsageTracker 创建带有UsageTracker的TokenParser实例
func (p *TokenParserProviderImpl) NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) response.TokenParser {
	return NewTokenParserWithUsageTracker(requestID, usageTracker)
}

// 适配器类型定义

// TokenParserAdapter 适配proxy.TokenParser到handlers.TokenParser
type TokenParserAdapter struct {
	innerParser *TokenParser
}

func (ta *TokenParserAdapter) ParseSSELine(line string) *monitor.TokenUsage {
	return ta.innerParser.ParseSSELine(line)
}

func (ta *TokenParserAdapter) SetModelName(model string) {
	ta.innerParser.SetModelName(model)
}

// StreamProcessorAdapter 适配proxy.StreamProcessor到handlers.StreamProcessor
type StreamProcessorAdapter struct {
	innerProcessor *StreamProcessor
}

func (spa *StreamProcessorAdapter) ProcessStreamWithRetry(ctx context.Context, resp *http.Response) (*tracking.TokenUsage, string, error) {
	return spa.innerProcessor.ProcessStreamWithRetry(ctx, resp)
}

// ErrorRecoveryManagerAdapter 适配*ErrorRecoveryManager到handlers.ErrorRecoveryManager
type ErrorRecoveryManagerAdapter struct {
	innerManager *ErrorRecoveryManager
}

func (era *ErrorRecoveryManagerAdapter) ClassifyError(err error, connID, endpointName, groupName string, attemptCount int) handlers.ErrorContext {
	ctx := era.innerManager.ClassifyError(err, connID, endpointName, groupName, attemptCount)
	return handlers.ErrorContext{
		RequestID:      ctx.RequestID,
		EndpointName:   ctx.EndpointName,
		GroupName:      ctx.GroupName,
		AttemptCount:   ctx.AttemptCount,
		ErrorType:      handlers.ErrorType(ctx.ErrorType),
		OriginalError:  ctx.OriginalError,
		RetryableAfter: ctx.RetryableAfter,
		MaxRetries:     ctx.MaxRetries,
	}
}

func (era *ErrorRecoveryManagerAdapter) HandleFinalFailure(errorCtx handlers.ErrorContext) {
	innerCtx := ErrorContext{
		RequestID:      errorCtx.RequestID,
		EndpointName:   errorCtx.EndpointName,
		GroupName:      errorCtx.GroupName,
		AttemptCount:   errorCtx.AttemptCount,
		ErrorType:      ErrorType(errorCtx.ErrorType),
		OriginalError:  errorCtx.OriginalError,
		RetryableAfter: errorCtx.RetryableAfter,
		MaxRetries:     errorCtx.MaxRetries,
	}
	era.innerManager.HandleFinalFailure(&innerCtx)
}

func (era *ErrorRecoveryManagerAdapter) GetErrorTypeName(errorType handlers.ErrorType) string {
	return era.innerManager.getErrorTypeName(ErrorType(errorType))
}

// TokenAnalyzerAdapter 适配*response.TokenAnalyzer到handlers.TokenAnalyzer
type TokenAnalyzerAdapter struct {
	innerAnalyzer *response.TokenAnalyzer
}

func (taa *TokenAnalyzerAdapter) AnalyzeResponseForTokens(ctx context.Context, responseBody, endpointName string, r *http.Request) {
	taa.innerAnalyzer.AnalyzeResponseForTokens(ctx, responseBody, endpointName, r)
}

func (taa *TokenAnalyzerAdapter) AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string) (*tracking.TokenUsage, string) {
	// 使用新的方法签名获取Token信息
	tokenUsage, modelName := taa.innerAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, connID, endpointName)
	
	return tokenUsage, modelName
}

// RequestLifecycleManagerAdapter 适配handlers.RequestLifecycleManager到response.RequestLifecycleManager
type RequestLifecycleManagerAdapter struct {
	innerManager handlers.RequestLifecycleManager
}

func (rlma *RequestLifecycleManagerAdapter) GetDuration() time.Duration {
	// 这里需要根据具体实现来获取持续时间
	// 暂时返回0，可能需要在handlers.RequestLifecycleManager接口中添加GetDuration方法
	return time.Duration(0)
}

// RetryHandlerAdapter 适配*RetryHandler到handlers.RetryHandler
type RetryHandlerAdapter struct {
	innerHandler *RetryHandler
}

func (rha *RetryHandlerAdapter) ShouldSuspendRequest(ctx context.Context) bool {
	return rha.innerHandler.shouldSuspendRequest(ctx)
}

func (rha *RetryHandlerAdapter) WaitForGroupSwitch(ctx context.Context, connID string) bool {
	return rha.innerHandler.waitForGroupSwitch(ctx, connID)
}

func (rha *RetryHandlerAdapter) SetEndpointManager(manager any) {
	if em, ok := manager.(*endpoint.Manager); ok {
		rha.innerHandler.SetEndpointManager(em)
	}
}

func (rha *RetryHandlerAdapter) SetUsageTracker(tracker *tracking.UsageTracker) {
	rha.innerHandler.SetUsageTracker(tracker)
}

func (rha *RetryHandlerAdapter) ExecuteWithContext(ctx context.Context, operation func(*endpoint.Endpoint, string) (*http.Response, error), connID string) (*http.Response, error) {
	return rha.innerHandler.ExecuteWithContext(ctx, Operation(operation), connID)
}

// 工厂实现 - 使用适配器

type TokenParserFactoryImpl struct{}

func (f *TokenParserFactoryImpl) NewTokenParserWithUsageTracker(connID string, usageTracker *tracking.UsageTracker) handlers.TokenParser {
	innerParser := NewTokenParserWithUsageTracker(connID, usageTracker)
	return &TokenParserAdapter{innerParser: innerParser}
}

type StreamProcessorFactoryImpl struct{}

func (f *StreamProcessorFactoryImpl) NewStreamProcessor(tokenParser handlers.TokenParser, usageTracker *tracking.UsageTracker, 
	w http.ResponseWriter, flusher http.Flusher, requestID, endpoint string) handlers.StreamProcessor {
	// 获取内部的TokenParser实例
	var concreteTokenParser *TokenParser
	if adapter, ok := tokenParser.(*TokenParserAdapter); ok {
		concreteTokenParser = adapter.innerParser
	} else {
		// 如果不是适配器类型，创建新的
		concreteTokenParser = NewTokenParserWithUsageTracker(requestID, usageTracker)
	}
	innerProcessor := NewStreamProcessor(concreteTokenParser, usageTracker, w, flusher, requestID, endpoint)
	return &StreamProcessorAdapter{innerProcessor: innerProcessor}
}

type ErrorRecoveryFactoryImpl struct{}

func (f *ErrorRecoveryFactoryImpl) NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) handlers.ErrorRecoveryManager {
	innerManager := NewErrorRecoveryManager(usageTracker)
	return &ErrorRecoveryManagerAdapter{innerManager: innerManager}
}

type RetryHandlerFactoryImpl struct{}

func (f *RetryHandlerFactoryImpl) NewRetryHandler(configInterface any) handlers.RetryHandler {
	if cfg, ok := configInterface.(*config.Config); ok {
		innerHandler := NewRetryHandler(cfg)
		return &RetryHandlerAdapter{innerHandler: innerHandler}
	}
	return nil
}

type RetryManagerFactoryImpl struct {
	config          *config.Config
	errorRecovery   *ErrorRecoveryManager
	endpointManager *endpoint.Manager
}

func (f *RetryManagerFactoryImpl) NewRetryManager() handlers.RetryManager {
	return NewRetryManager(f.config, f.errorRecovery, f.endpointManager)
}

type SuspensionManagerFactoryImpl struct {
	config          *config.Config
	endpointManager *endpoint.Manager
	recoverySignalManager *EndpointRecoverySignalManager // 🚀 [端点自愈] 恢复信号管理器
}

func (f *SuspensionManagerFactoryImpl) NewSuspensionManager() handlers.SuspensionManager {
	// 🚀 [端点自愈] 使用带恢复信号的SuspensionManager构造函数
	return NewSuspensionManagerWithRecoverySignal(f.config, f.endpointManager, f.endpointManager.GetGroupManager(), f.recoverySignalManager)
}


// NewHandler creates a new proxy handler
func NewHandler(endpointManager *endpoint.Manager, cfg *config.Config) *Handler {
	retryHandler := NewRetryHandler(cfg)
	retryHandler.SetEndpointManager(endpointManager)
	
	// 创建forwarder
	forwarder := handlers.NewForwarder(cfg, endpointManager)
	
	// 🚀 [端点自愈] 创建端点恢复信号管理器
	recoverySignalManager := NewEndpointRecoverySignalManager()

	h := &Handler{
		endpointManager:       endpointManager,
		config:                cfg,
		retryHandler:          retryHandler,
		responseProcessor:     response.NewProcessor(),
		forwarder:             forwarder,
		recoverySignalManager: recoverySignalManager, // 🚀 [端点自愈] 保存恢复信号管理器引用
	}
	
	// 初始化 token analyzer
	provider := &TokenParserProviderImpl{}
	h.tokenAnalyzer = response.NewTokenAnalyzer(nil, nil, provider)
	
	// 创建工厂实例
	tokenParserFactory := &TokenParserFactoryImpl{}
	streamProcessorFactory := &StreamProcessorFactoryImpl{}
	errorRecoveryFactory := &ErrorRecoveryFactoryImpl{}
	retryManagerFactory := &RetryManagerFactoryImpl{
		config:          cfg,
		errorRecovery:   NewErrorRecoveryManager(nil), // 临时创建，后续会在工厂中重新创建
		endpointManager: endpointManager,
	}
	suspensionManagerFactory := &SuspensionManagerFactoryImpl{
		config:                cfg,
		endpointManager:       endpointManager,
		recoverySignalManager: recoverySignalManager, // 🚀 [端点自愈] 传递恢复信号管理器
	}

	// 🔧 [Critical修复] 创建单一共享的SuspensionManager实例
	// 确保常规请求和流式请求共享同一个挂起计数器，真正实现全局限制
	sharedSuspensionManager := suspensionManagerFactory.NewSuspensionManager()

	// 🔧 [Critical修复] 保存共享SuspensionManager的引用到Handler结构体
	// 确保在SetUsageTracker中能重用相同的实例
	h.sharedSuspensionManager = sharedSuspensionManager

	// 创建RetryHandler适配器
	retryHandlerAdapter := &RetryHandlerAdapter{innerHandler: retryHandler}

	// 创建TokenAnalyzer适配器
	tokenAnalyzerAdapter := &TokenAnalyzerAdapter{innerAnalyzer: h.tokenAnalyzer}

	// 创建regularHandler - 传入正确初始化的组件
	h.regularHandler = handlers.NewRegularHandler(
		cfg,
		endpointManager,
		forwarder,
		nil, // usageTracker will be set later
		h.responseProcessor, // 传入已创建的responseProcessor
		tokenAnalyzerAdapter, // 传入TokenAnalyzer适配器
		retryHandlerAdapter, // 传入RetryHandler适配器
		errorRecoveryFactory,
		retryManagerFactory,
		suspensionManagerFactory,
		// 🔧 [Critical修复] 传入共享的SuspensionManager实例
		sharedSuspensionManager,
	)

	// 创建streamingHandler
	h.streamingHandler = handlers.NewStreamingHandler(
		cfg,
		endpointManager,
		forwarder,
		nil, // usageTracker will be set later
		tokenParserFactory,
		streamProcessorFactory,
		errorRecoveryFactory,
		retryManagerFactory, // 传递retryManagerFactory
		suspensionManagerFactory,
		// 🔧 [Critical修复] 传入相同的共享SuspensionManager实例
		sharedSuspensionManager,
	)
	
	// 初始化 token analyzer，暂时不设置 usageTracker 和 monitoringMiddleware
	// 这些将在 SetUsageTracker 和 SetMonitoringMiddleware 方法中设置
	// provider已经在上面定义过了，这里删除重复定义
	
	return h
}

// SetMonitoringMiddleware 设置监控中间件用于重试跟踪
func (h *Handler) SetMonitoringMiddleware(mm *middleware.MonitoringMiddleware) {
	h.monitoringMiddleware = mm
	h.retryHandler.SetMonitoringMiddleware(mm)
	
	// 同时更新tokenAnalyzer的monitoringMiddleware
	if h.tokenAnalyzer != nil {
		provider := &TokenParserProviderImpl{}
		h.tokenAnalyzer = response.NewTokenAnalyzer(h.usageTracker, mm, provider)
	}
}

// SetUsageTracker sets the usage tracker for request tracking
func (h *Handler) SetUsageTracker(ut *tracking.UsageTracker) {
	h.usageTracker = ut
	
	// ⚠️ 重要：先更新tokenAnalyzer，再创建适配器
	provider := &TokenParserProviderImpl{}
	h.tokenAnalyzer = response.NewTokenAnalyzer(ut, h.retryHandler.monitoringMiddleware, provider)

	// 创建共用的工厂实例
	errorRecoveryFactory := &ErrorRecoveryFactoryImpl{}
	retryManagerFactory := &RetryManagerFactoryImpl{
		config:          h.config,
		errorRecovery:   NewErrorRecoveryManager(nil), // 临时创建，后续会在工厂中重新创建
		endpointManager: h.endpointManager,
	}
	suspensionManagerFactory := &SuspensionManagerFactoryImpl{
		config:                h.config,
		endpointManager:       h.endpointManager,
		recoverySignalManager: h.recoverySignalManager, // 🚀 [端点自愈] 修复：确保恢复信号能力不丢失
	}

	// 重新创建regularHandler以包含usageTracker
	if h.regularHandler != nil {
		// 创建适配器 - 使用更新后的tokenAnalyzer
		retryHandlerAdapter := &RetryHandlerAdapter{innerHandler: h.retryHandler}
		tokenAnalyzerAdapter := &TokenAnalyzerAdapter{innerAnalyzer: h.tokenAnalyzer}

		h.regularHandler = handlers.NewRegularHandler(
			h.config,
			h.endpointManager,
			h.forwarder,
			ut,
			h.responseProcessor, // responseProcessor
			tokenAnalyzerAdapter, // tokenAnalyzer适配器
			retryHandlerAdapter, // retryHandler适配器
			errorRecoveryFactory,
			retryManagerFactory,
			suspensionManagerFactory,
			// 🔧 [Critical修复] 使用保存的共享SuspensionManager实例
			h.sharedSuspensionManager,
		)
	}
	
	// 重新创建streamingHandler以包含usageTracker
	if h.streamingHandler != nil {
		tokenParserFactory := &TokenParserFactoryImpl{}
		streamProcessorFactory := &StreamProcessorFactoryImpl{}

		h.streamingHandler = handlers.NewStreamingHandler(
			h.config,
			h.endpointManager,
			h.forwarder,
			ut, // 设置usageTracker
			tokenParserFactory,
			streamProcessorFactory,
			errorRecoveryFactory,
			retryManagerFactory,
			suspensionManagerFactory,
			// 🔧 [Critical修复] 使用保存的共享SuspensionManager实例
			h.sharedSuspensionManager,
		)
	}
	
	// 注意：h.tokenAnalyzer 已经在方法开头更新
}

// GetRetryHandler returns the retry handler for accessing suspended request counts
func (h *Handler) GetRetryHandler() *RetryHandler {
	return h.retryHandler
}

// SetEventBus 设置EventBus事件总线
func (h *Handler) SetEventBus(eventBus events.EventBus) {
	h.eventBus = eventBus
}

// extractModelFromRequestBody 从请求体中提取模型名称
// 仅对 /v1/messages 相关路径进行解析，避免不必要的JSON解析开销
func (h *Handler) extractModelFromRequestBody(bodyBytes []byte, path string) string {
	// 仅对包含 messages 的路径尝试解析模型
	if !strings.Contains(path, "/v1/messages") {
		return ""
	}
	
	// 避免解析空请求体
	if len(bodyBytes) == 0 {
		return ""
	}
	
	var requestBody struct {
		Model string `json:"model"`
	}
	
	if err := json.Unmarshal(bodyBytes, &requestBody); err == nil && requestBody.Model != "" {
		return requestBody.Model
	}
	
	return ""
}

// ServeHTTP implements the http.Handler interface
// 统一请求分发逻辑 - 整合流式处理、错误恢复和生命周期管理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 🔢 [count_tokens拦截] 特殊处理count_tokens端点
	if r.URL.Path == "/v1/messages/count_tokens" && h.config.TokenCounting.Enabled {
		ctx := r.Context()
		connID, _ := r.Context().Value("conn_id").(string)

		// 读取请求体
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusInternalServerError)
				return
			}
			r.Body.Close()
		}

		// 使用CountTokensHandler处理
		countTokensHandler := handlers.NewCountTokensHandler(h.config, h.endpointManager, h.forwarder)
		countTokensHandler.Handle(ctx, w, r, bodyBytes, connID)
		return
	}

	// 创建请求上下文
	ctx := r.Context()
	
	// 获取连接ID
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// 创建统一的请求生命周期管理器
	lifecycleManager := NewRequestLifecycleManagerWithRecoverySignal(h.usageTracker, h.monitoringMiddleware, connID, h.eventBus, h.recoverySignalManager)
	
	// 克隆请求体用于重试
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			lifecycleManager.HandleError(err)
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
	}

	// 异步解析请求体中的模型名称（不阻塞主转发流程）
	go func(body []byte, path string) {
		if modelName := h.extractModelFromRequestBody(body, path); modelName != "" {
			lifecycleManager.SetModel(modelName)
		}
	}(append([]byte(nil), bodyBytes...), r.URL.Path) // 传递副本避免数据竞争

	// 检测是否为SSE流式请求
	isSSE := h.detectSSERequest(r, bodyBytes)
	
	// 开始请求跟踪（传递流式标记）
	clientIP := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")
	lifecycleManager.StartRequest(clientIP, userAgent, r.Method, r.URL.Path, isSSE)
	
	// 统一请求处理
	if isSSE {
		// 流式请求处理 - 使用StreamingHandler
		if h.streamingHandler != nil {
			h.streamingHandler.HandleStreamingRequest(ctx, w, r, bodyBytes, lifecycleManager)
			// h.regularHandler.HandleRegularRequestUnified(ctx, w, r, bodyBytes, lifecycleManager)
		} else {
			// 备用方案：如果streamingHandler不可用，使用regularHandler
			h.regularHandler.HandleRegularRequestUnified(ctx, w, r, bodyBytes, lifecycleManager)
		}
	} else {
		// 常规请求处理 - 使用RegularHandler
		h.regularHandler.HandleRegularRequestUnified(ctx, w, r, bodyBytes, lifecycleManager)
	}
}

// detectSSERequest 统一SSE请求检测逻辑
func (h *Handler) detectSSERequest(r *http.Request, bodyBytes []byte) bool {
	// 检查多种SSE请求模式:
	acceptHeader := r.Header.Get("Accept")
	cacheControlHeader := r.Header.Get("Cache-Control")
	streamHeader := r.Header.Get("stream")
	
	// 1. Accept头包含text/event-stream
	if strings.Contains(acceptHeader, "text/event-stream") {
		return true
	}
	
	// 2. Cache-Control头包含no-cache (常见于SSE)
	if strings.Contains(cacheControlHeader, "no-cache") {
		return true
	}
	
	// 3. stream头设置为true
	if streamHeader == "true" {
		return true
	}
	
	// 4. 请求体包含stream参数为true
	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, `"stream":true`) || strings.Contains(bodyStr, `"stream": true`) {
		return true
	}
	
	return false
}

// UpdateConfig updates the handler configuration
func (h *Handler) UpdateConfig(cfg *config.Config) {
	h.config = cfg
	
	// Update retry handler with new config
	h.retryHandler.UpdateConfig(cfg)
}

// noOpFlusher 是一个不执行实际flush操作的flusher实现
// 用于测试和不支持Flusher的环境
type noOpFlusher struct{}

func (f *noOpFlusher) Flush() {
	// 不执行任何操作，避免panic但保持流式处理逻辑
}


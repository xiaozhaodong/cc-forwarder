package proxy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
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
	endpointManager   *endpoint.Manager
	config            *config.Config
	retryHandler      *RetryHandler
	usageTracker      *tracking.UsageTracker
	responseProcessor *response.Processor
	tokenAnalyzer     *response.TokenAnalyzer
	forwarder         *handlers.Forwarder
	regularHandler    *handlers.RegularHandler
	streamingHandler  *handlers.StreamingHandler
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

func (rha *RetryHandlerAdapter) SetEndpointManager(manager interface{}) {
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

func (f *RetryHandlerFactoryImpl) NewRetryHandler(configInterface interface{}) handlers.RetryHandler {
	if cfg, ok := configInterface.(*config.Config); ok {
		innerHandler := NewRetryHandler(cfg)
		return &RetryHandlerAdapter{innerHandler: innerHandler}
	}
	return nil
}


// NewHandler creates a new proxy handler
func NewHandler(endpointManager *endpoint.Manager, cfg *config.Config) *Handler {
	retryHandler := NewRetryHandler(cfg)
	retryHandler.SetEndpointManager(endpointManager)
	
	// 创建forwarder
	forwarder := handlers.NewForwarder(cfg, endpointManager)
	
	h := &Handler{
		endpointManager:   endpointManager,
		config:            cfg,
		retryHandler:      retryHandler,
		responseProcessor: response.NewProcessor(),
		forwarder:         forwarder,
	}
	
	// 初始化 token analyzer
	provider := &TokenParserProviderImpl{}
	h.tokenAnalyzer = response.NewTokenAnalyzer(nil, nil, provider)
	
	// 创建工厂实例
	tokenParserFactory := &TokenParserFactoryImpl{}
	streamProcessorFactory := &StreamProcessorFactoryImpl{}
	errorRecoveryFactory := &ErrorRecoveryFactoryImpl{}
	retryHandlerFactory := &RetryHandlerFactoryImpl{}
	
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
		retryHandlerFactory,
	)
	
	// 初始化 token analyzer，暂时不设置 usageTracker 和 monitoringMiddleware
	// 这些将在 SetUsageTracker 和 SetMonitoringMiddleware 方法中设置
	// provider已经在上面定义过了，这里删除重复定义
	
	return h
}

// SetMonitoringMiddleware sets the monitoring middleware for retry tracking
func (h *Handler) SetMonitoringMiddleware(mm interface{
	RecordRetry(connID string, endpoint string)
}) {
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
	
	// 重新创建regularHandler以包含usageTracker
	if h.regularHandler != nil {
		// 创建适配器 - 使用更新后的tokenAnalyzer
		retryHandlerAdapter := &RetryHandlerAdapter{innerHandler: h.retryHandler}
		tokenAnalyzerAdapter := &TokenAnalyzerAdapter{innerAnalyzer: h.tokenAnalyzer}
		errorRecoveryFactory := &ErrorRecoveryFactoryImpl{}
		
		h.regularHandler = handlers.NewRegularHandler(
			h.config,
			h.endpointManager,
			h.forwarder,
			ut,
			h.responseProcessor, // responseProcessor 
			tokenAnalyzerAdapter, // tokenAnalyzer适配器
			retryHandlerAdapter, // retryHandler适配器
			errorRecoveryFactory,
		)
	}
	
	// 重新创建streamingHandler以包含usageTracker
	if h.streamingHandler != nil {
		tokenParserFactory := &TokenParserFactoryImpl{}
		streamProcessorFactory := &StreamProcessorFactoryImpl{}
		errorRecoveryFactory := &ErrorRecoveryFactoryImpl{}
		retryHandlerFactory := &RetryHandlerFactoryImpl{}
		
		h.streamingHandler = handlers.NewStreamingHandler(
			h.config,
			h.endpointManager,
			h.forwarder,
			ut, // 设置usageTracker
			tokenParserFactory,
			streamProcessorFactory,
			errorRecoveryFactory,
			retryHandlerFactory,
		)
	}
	
	// 注意：h.tokenAnalyzer 已经在方法开头更新
}

// GetRetryHandler returns the retry handler for accessing suspended request counts
func (h *Handler) GetRetryHandler() *RetryHandler {
	return h.retryHandler
}

// ServeHTTP implements the http.Handler interface
// 统一请求分发逻辑 - 整合流式处理、错误恢复和生命周期管理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 创建请求上下文
	ctx := r.Context()
	
	// 获取连接ID
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// 创建统一的请求生命周期管理器
	lifecycleManager := NewRequestLifecycleManager(h.usageTracker, connID)
	
	// 开始请求跟踪
	clientIP := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")
	lifecycleManager.StartRequest(clientIP, userAgent)
	
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

	// 检测是否为SSE流式请求
	isSSE := h.detectSSERequest(r, bodyBytes)
	
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


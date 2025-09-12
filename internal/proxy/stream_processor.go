package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"cc-forwarder/internal/tracking"
)

// 缓冲区大小常量
const (
	StreamBufferSize     = 8192  // 8KB主缓冲区
	LineBufferInitSize   = 1024  // 1KB行缓冲区初始大小
	BackgroundBufferSize = 4096  // 4KB后台解析缓冲区
)

// StreamProcessor 流式处理器核心结构体
type StreamProcessor struct {
	// 核心组件
	tokenParser    *TokenParser              // Token解析器，用于提取模型信息和使用统计
	usageTracker   *tracking.UsageTracker    // 使用跟踪器，记录请求生命周期
	responseWriter http.ResponseWriter       // HTTP响应写入器
	flusher        http.Flusher              // HTTP刷新器，用于立即发送数据到客户端
	
	// 错误处理和恢复
	errorRecovery  *ErrorRecoveryManager     // 错误恢复管理器
	lastAPIError   error                     // V2架构：最后一次API错误信息
	
	// 请求标识信息
	requestID      string                    // 请求唯一标识符
	endpoint       string                    // 端点名称
	
	// 流式处理状态
	startTime      time.Time                 // 处理开始时间
	bytesProcessed int64                     // 已处理字节数
	lineBuffer     []byte                    // SSE行缓冲区
	partialData    []byte                    // 部分数据缓冲区，用于错误恢复
	
	// 并发控制
	parseWg        sync.WaitGroup            // 等待组，确保后台解析完成
	parseMutex     sync.Mutex               // 解析互斥锁，保护共享状态
	
	// 错误处理
	parseErrors    []error                   // 解析过程中的错误集合
	maxParseErrors int                       // 最大允许解析错误数
	
	// 完成状态跟踪
	completionRecorded bool                  // 是否已经记录完成状态，防止重复记录
}

// NewStreamProcessor 创建新的流式处理器实例
func NewStreamProcessor(tokenParser *TokenParser, usageTracker *tracking.UsageTracker, 
	w http.ResponseWriter, flusher http.Flusher, requestID, endpoint string) *StreamProcessor {
	
	return &StreamProcessor{
		tokenParser:    tokenParser,
		usageTracker:   usageTracker,
		responseWriter: w,
		flusher:        flusher,
		errorRecovery:  NewErrorRecoveryManager(usageTracker),
		requestID:      requestID,
		endpoint:       endpoint,
		startTime:      time.Now(),
		lineBuffer:     make([]byte, 0, LineBufferInitSize),
		partialData:    make([]byte, 0, BackgroundBufferSize),
		maxParseErrors: 10, // 最多允许10个解析错误
	}
}

// ProcessStream 实现边接收边转发的8KB缓冲区流式处理
// 这是核心方法，实现真正的流式处理机制
func (sp *StreamProcessor) ProcessStream(ctx context.Context, resp *http.Response) (*tracking.TokenUsage, error) {
	defer resp.Body.Close()
	defer sp.waitForBackgroundParsing() // 确保所有后台解析完成
	
	// 初始化8KB缓冲区
	buffer := make([]byte, StreamBufferSize)
	reader := bufio.NewReader(resp.Body)
	
	// 记录流处理开始
	slog.Info(fmt.Sprintf("🌊 [流式处理] [%s] 开始流式处理，端点: %s", sp.requestID, sp.endpoint))
	
	// 主流式处理循环
	for {
		// 检查context取消 - 优先级最高
		select {
		case <-ctx.Done():
			// 客户端取消，进入优雅取消处理
			return sp.handleCancellationV2(ctx, ctx.Err())
		default:
			// 继续正常处理
		}
		
		// 1. 从响应中读取数据到8KB缓冲区
		n, err := reader.Read(buffer)
		
		if n > 0 {
			chunk := buffer[:n]
			
			// 保存部分数据用于错误恢复
			sp.savePartialData(chunk)
			
			// 2. 立即转发到客户端 - 这是关键！不等待完整响应
			if writeErr := sp.forwardToClient(chunk); writeErr != nil {
				// 使用错误恢复管理器处理转发错误
				errorCtx := sp.errorRecovery.ClassifyError(writeErr, sp.requestID, sp.endpoint, "", 0)
				sp.errorRecovery.HandleFinalFailure(errorCtx)
				slog.Error(fmt.Sprintf("❌ [流式错误] [%s] 转发到客户端失败: %v", sp.requestID, writeErr))
				return nil, fmt.Errorf("failed to forward to client: %w", writeErr)
			}
			
			// 3. 并行解析Token信息 - 不影响转发性能
			sp.parseTokensInBackground(chunk)
			
			// 4. 更新处理状态
			sp.bytesProcessed += int64(n)
		}
		
		// 处理读取结束和错误
		if err == io.EOF {
			// 等待所有后台解析完成
			sp.waitForBackgroundParsing()
			
			// 获取最终的 Token 使用信息
			finalTokenUsage := sp.getFinalTokenUsage()
			
			slog.Info(fmt.Sprintf("✅ [流式完成] [%s] 端点: %s, 流处理正常完成，已处理 %d 字节", 
				sp.requestID, sp.endpoint, sp.bytesProcessed))
			return finalTokenUsage, nil
		}
		
		if err != nil {
			// 网络中断或其他错误，尝试部分数据处理
			return sp.handlePartialStreamV2(err)
		}
	}
}

// forwardToClient 立即转发数据到客户端
func (sp *StreamProcessor) forwardToClient(data []byte) error {
	// 写入数据到响应
	if _, err := sp.responseWriter.Write(data); err != nil {
		return err
	}
	
	// 立即刷新，确保数据立即发送到客户端
	sp.flusher.Flush()
	
	return nil
}

// parseTokensInBackground 并发Token解析，不阻塞主流
// 这个方法在后台goroutine中解析SSE事件，提取模型信息和Token使用统计
func (sp *StreamProcessor) parseTokensInBackground(data []byte) {
	// 为每个数据块启动一个后台goroutine
	sp.parseWg.Add(1)
	
	go func() {
		defer sp.parseWg.Done()
		
		// 创建后台处理缓冲区
		parseBuffer := make([]byte, len(data))
		copy(parseBuffer, data)
		
		// 逐字节处理，构建SSE行
		sp.parseMutex.Lock()
		defer sp.parseMutex.Unlock()
		
		for _, b := range parseBuffer {
			// 构建行缓冲区
			sp.lineBuffer = append(sp.lineBuffer, b)
			
			// 检测换行符，处理完整的SSE行
			if b == '\n' {
				line := strings.TrimSpace(string(sp.lineBuffer))
				
				// ✅ 修复：处理所有行，包括空行（空行触发SSE事件解析）
				sp.processSSELine(line)
				
				// 重置行缓冲区，准备下一行
				sp.lineBuffer = sp.lineBuffer[:0]
			}
		}
	}()
}

// processSSELine 处理单个SSE行
// 修改版本：仅进行 Token 解析，不再直接记录到 usageTracker
func (sp *StreamProcessor) processSSELine(line string) {
	// ✅ 使用V2架构进行解析
	result := sp.tokenParser.ParseSSELineV2(line)
	
	if result != nil {
		// ✅ 检查是否有错误信息
		if result.ErrorInfo != nil {
			// V2架构：处理API错误信息
			slog.Error(fmt.Sprintf("❌ [API错误V2] [%s] 类型: %s, 消息: %s", 
				sp.requestID, result.ErrorInfo.Type, result.ErrorInfo.Message))
			
			// 将错误信息存储，供上层生命周期管理器处理
			sp.lastAPIError = fmt.Errorf("API错误 %s: %s", result.ErrorInfo.Type, result.ErrorInfo.Message)
			return
		}
		
		// ✅ 处理正常Token信息
		if result.TokenUsage != nil {
			// V2架构：直接使用ParseResult，无需类型转换
			trackingTokens := result.TokenUsage
			modelName := result.ModelName
			
			// 确保模型名称不为空
			if modelName == "" {
				modelName = "default"
			}
			
			// 仅记录解析日志，不再直接记录到 usageTracker
			if !sp.completionRecorded {
				sp.completionRecorded = true  // 标记已解析完成状态
				
				slog.Debug(fmt.Sprintf("🔄 [Token解析进度] [%s] V2架构实时解析 - 模型: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d", 
					sp.requestID, modelName, trackingTokens.InputTokens, trackingTokens.OutputTokens, trackingTokens.CacheCreationTokens, trackingTokens.CacheReadTokens))
			}
		}
	}
}

// ensureRequestCompletion 确保请求完成状态被记录（fallback机制）
// 🚫 DEPRECATED: 已被 getFinalTokenUsage() 替代，此方法已完全移除违规调用
// 此方法不再执行任何操作，仅保留方法签名以维持兼容性
func (sp *StreamProcessor) ensureRequestCompletion() {
	// ⚠️ 此方法已完全弃用，所有功能已迁移到 getFinalTokenUsage() 方法
	// 原因：违反单一责任原则，直接调用 usageTracker 而非通过生命周期管理器
	// 
	// 新的架构要求：
	// 1. StreamProcessor 只负责解析和返回Token信息
	// 2. Handler 调用生命周期管理器记录完成状态 
	// 3. 不再有任何组件直接调用 usageTracker
	
	slog.Debug(fmt.Sprintf("⚠️ [已弃用] [%s] ensureRequestCompletion已弃用，请使用getFinalTokenUsage", sp.requestID))
}

// handlePartialStream 处理部分数据流中断情况（修复版本）
// 🚫 DEPRECATED: 已被 handlePartialStreamV2() 替代，此方法已完全移除违规调用
// 当网络中断或其他错误发生时，不再进行错误分类，让上层统一处理
func (sp *StreamProcessor) handlePartialStream(err error) error {
	// ⚠️ 此方法已弃用，请使用 handlePartialStreamV2() 方法
	// 原因：违反生命周期管理器架构，直接调用 usageTracker 而非返回Token信息
	
	// 记录流处理中断但不做任何usageTracker调用
	slog.Warn(fmt.Sprintf("⚠️ [流式中断] [%s] 流处理中断: %v, 已处理 %d 字节. 错误将由上层统一处理.", 
		sp.requestID, err, sp.bytesProcessed))
	
	// 等待所有后台解析完成
	sp.waitForBackgroundParsing()
	
	// 尝试从部分数据中恢复有用信息
	if len(sp.partialData) > 0 {
		sp.errorRecovery.RecoverFromPartialData(sp.requestID, sp.partialData, time.Since(sp.startTime))
	}
	
	// 直接返回错误，让调用者(handler)通过生命周期管理器来分类和处理最终失败
	return err
}

// ProcessStreamWithRetry 支持网络中断恢复的流式处理（增强版本）
// 在网络不稳定环境下提供智能重试机制
// 返回值：(finalTokenUsage *tracking.TokenUsage, modelName string, err error)
// 修改为返回 Token 使用信息和模型名称而非直接记录到 usageTracker
func (sp *StreamProcessor) ProcessStreamWithRetry(ctx context.Context, resp *http.Response) (*tracking.TokenUsage, string, error) {
	const maxRetries = 3
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 分类当前尝试的错误上下文
		var lastErr error
		
		if attempt > 0 {
			// 使用错误恢复管理器计算重试延迟
			errorCtx := sp.errorRecovery.ClassifyError(lastErr, sp.requestID, sp.endpoint, "", attempt)
			
			// 检查是否应该重试
			if !sp.errorRecovery.ShouldRetry(errorCtx) {
				slog.Info(fmt.Sprintf("🛑 [重试停止] [%s] 错误恢复管理器建议停止重试", sp.requestID))
				sp.errorRecovery.HandleFinalFailure(errorCtx)
				return nil, "", lastErr
			}
			
			// 执行重试延迟
			if retryErr := sp.errorRecovery.ExecuteRetry(ctx, errorCtx); retryErr != nil {
				return nil, "", retryErr
			}
		}
		
		// 尝试流式处理
		finalTokenUsage, err := sp.ProcessStream(ctx, resp)
		
		if err == nil {
			// ✅ 检查是否在处理过程中遇到了API错误
			if sp.lastAPIError != nil {
				// 流式处理成功，但遇到了API错误（如SSE错误事件）
				return nil, "", sp.lastAPIError
			}
			
			// 处理成功，获取模型名称
			modelName := sp.tokenParser.GetModelName()
			if modelName == "" {
				modelName = "default"
			}
			
			if attempt > 0 {
				slog.Info(fmt.Sprintf("✅ [重试成功] [%s] 第 %d 次重试成功", sp.requestID, attempt))
			}
			return finalTokenUsage, modelName, nil
		}
		
		lastErr = err
		
		// 简化的重试判断逻辑，避免重复错误分类
		// 对于流式处理，我们主要关注网络相关的错误是否可重试
		shouldRetry := false
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// 简单判断是否为可重试的网络/超时错误
			if strings.Contains(errStr, "timeout") || 
			   strings.Contains(errStr, "connection") || 
			   strings.Contains(errStr, "network") ||
			   strings.Contains(errStr, "reset") ||
			   strings.Contains(errStr, "refused") {
				shouldRetry = true
			}
		}
		
		if shouldRetry && attempt < maxRetries {
			slog.Warn(fmt.Sprintf("🔄 [网络错误重试] [%s] 网络相关错误将重试: %v", sp.requestID, err))
			continue
		}
		
	// 不可重试错误或重试次数已满，直接返回让上层处理
		slog.Info(fmt.Sprintf("🛑 [重试停止] [%s] %d 次重试后停止，错误将由上层处理: %v", 
			sp.requestID, attempt, err))
		return nil, "", err
	}
	
	// 创建最终失败的错误上下文
	finalErrorCtx := &ErrorContext{
		RequestID:     sp.requestID,
		EndpointName:  sp.endpoint,
		AttemptCount:  maxRetries,
		ErrorType:     ErrorTypeUnknown,
		OriginalError: fmt.Errorf("stream processing failed after %d retries", maxRetries),
	}
	sp.errorRecovery.HandleFinalFailure(finalErrorCtx)
	
	return nil, "", fmt.Errorf("stream processing failed after %d retries", maxRetries)
}

// waitForBackgroundParsing 等待所有后台解析完成
func (sp *StreamProcessor) waitForBackgroundParsing() {
	// 等待所有后台goroutine完成
	sp.parseWg.Wait()
	
	// 处理剩余的行缓冲区数据
	sp.parseMutex.Lock()
	if len(sp.lineBuffer) > 0 {
		line := strings.TrimSpace(string(sp.lineBuffer))
		if len(line) > 0 {
			sp.processSSELine(line)
		}
		sp.lineBuffer = sp.lineBuffer[:0]
	}
	sp.parseMutex.Unlock()
}

// savePartialData 保存部分数据用于错误恢复
func (sp *StreamProcessor) savePartialData(chunk []byte) {
	// 限制部分数据缓冲区大小，防止内存过度使用
	const maxPartialDataSize = 64 * 1024 // 64KB

	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()

	// 如果添加新数据会超过限制，则移除旧数据
	if len(sp.partialData)+len(chunk) > maxPartialDataSize {
		// 保留最后的32KB数据，丢弃更早的数据
		keepSize := maxPartialDataSize/2 - len(chunk)
		if keepSize > 0 && keepSize < len(sp.partialData) {
			copy(sp.partialData, sp.partialData[len(sp.partialData)-keepSize:])
			sp.partialData = sp.partialData[:keepSize]
		} else {
			sp.partialData = sp.partialData[:0]
		}
	}

	// 添加新的数据块
	sp.partialData = append(sp.partialData, chunk...)
}

// GetProcessingStats 获取处理统计信息
func (sp *StreamProcessor) GetProcessingStats() map[string]interface{} {
	return map[string]interface{}{
		"request_id":       sp.requestID,
		"endpoint":         sp.endpoint, 
		"bytes_processed":  sp.bytesProcessed,
		"processing_time":  time.Since(sp.startTime),
		"parse_errors":     len(sp.parseErrors),
		"max_parse_errors": sp.maxParseErrors,
	}
}

// Reset 重置处理器状态，用于复用
func (sp *StreamProcessor) Reset() {
	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()
	
	sp.startTime = time.Now()
	sp.bytesProcessed = 0
	sp.lineBuffer = sp.lineBuffer[:0]
	sp.partialData = sp.partialData[:0] // 重置部分数据缓冲区
	sp.parseErrors = sp.parseErrors[:0]
	
	// 重置TokenParser状态
	if sp.tokenParser != nil {
		sp.tokenParser.Reset()
	}
	
	slog.Info(fmt.Sprintf("🔄 [处理器重置] [%s] 流处理器已重置", sp.requestID))
}

// handleCancellation 处理客户端取消请求 - Phase 2 优雅取消处理器
// 🚫 DEPRECATED: 已被 handleCancellationV2() 替代，不再直接调用 usageTracker
func (sp *StreamProcessor) handleCancellation(ctx context.Context, cancelErr error) error {
	// ⚠️ 此方法已弃用，请使用 handleCancellationV2() 方法
	// 原因：违反生命周期管理器架构，通过 collectAvailableInfo 间接调用 usageTracker
	
	slog.Info(fmt.Sprintf("🚫 [客户端取消] [%s] 检测到客户端取消: %v", sp.requestID, cancelErr))
	
	// 等待后台解析完成，但不超过超时时间
	if finished := sp.waitForParsingWithTimeout(2 * time.Second); finished {
		// 成功等待解析完成，调用新版本方法获取Token信息
		tokenUsage, err := sp.collectAvailableInfoV2(cancelErr, "cancelled_with_data")
		_ = tokenUsage // 忽略Token信息，保持原接口兼容
		return err
	} else {
		// 超时未完成，调用新版本方法获取Token信息
		tokenUsage, err := sp.collectAvailableInfoV2(cancelErr, "cancelled_timeout")
		_ = tokenUsage // 忽略Token信息，保持原接口兼容
		return err
	}
}

// waitForParsingWithTimeout 带超时的等待解析完成 - Phase 2 超时等待机制
func (sp *StreamProcessor) waitForParsingWithTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	
	go func() {
		sp.parseWg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		slog.Info(fmt.Sprintf("✅ [解析完成] [%s] 后台解析在取消前完成", sp.requestID))
		return true
	case <-time.After(timeout):
		slog.Warn(fmt.Sprintf("⏰ [解析超时] [%s] 后台解析在 %v 内未完成", sp.requestID, timeout))
		return false
	}
}

// collectAvailableInfo 智能信息收集 - Phase 2 分阶段保存逻辑
// 🚫 DEPRECATED: 已被 collectAvailableInfoV2() 替代，此方法已完全移除违规调用
func (sp *StreamProcessor) collectAvailableInfo(cancelErr error, status string) error {
	// ⚠️ 此方法已完全弃用，请使用 collectAvailableInfoV2() 方法
	// 原因：违反生命周期管理器架构，直接调用 usageTracker 而非返回Token信息
	// 
	// 新的架构要求：
	// 1. StreamProcessor 只负责收集Token信息
	// 2. Handler 调用生命周期管理器记录状态
	// 3. 不再有任何组件直接调用 usageTracker
	
	slog.Debug(fmt.Sprintf("⚠️ [已弃用] [%s] collectAvailableInfo已弃用，请使用collectAvailableInfoV2", sp.requestID))
	return cancelErr
}

// getFinalTokenUsage 获取最终的Token使用信息
// 这个方法替代了原有的ensureRequestCompletion中的直接记录逻辑
func (sp *StreamProcessor) getFinalTokenUsage() *tracking.TokenUsage {
	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()
	
	// 尝试从TokenParser获取最终使用统计
	finalUsage := sp.tokenParser.GetFinalUsage()
	
	if finalUsage != nil {
		// 有完整的token信息，记录详细日志
		modelName := sp.tokenParser.GetModelName()
		if modelName == "" {
			modelName = "default"
		}
		slog.Info(fmt.Sprintf("🪙 [Token最终统计] [%s] 流式处理完成 - 模型: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d", 
			sp.requestID, modelName, finalUsage.InputTokens, finalUsage.OutputTokens, finalUsage.CacheCreationTokens, finalUsage.CacheReadTokens))
		return finalUsage
	} else {
		// 没有token信息，返回空的token使用统计
		slog.Info(fmt.Sprintf("🎯 [无Token完成] [%s] 流式响应不包含token信息", sp.requestID))
		return &tracking.TokenUsage{
			InputTokens: 0, OutputTokens: 0, 
			CacheCreationTokens: 0, CacheReadTokens: 0,
		}
	}
}

// handlePartialStreamV2 处理部分数据流中断情况（返回Token信息版本）
// 当网络中断或其他错误发生时，收集已解析的Token信息并返回
func (sp *StreamProcessor) handlePartialStreamV2(err error) (*tracking.TokenUsage, error) {
	// 记录流处理中断
	slog.Warn(fmt.Sprintf("⚠️ [流式中断] [%s] 流处理中断: %v, 已处理 %d 字节. 错误将由上层统一处理.", 
		sp.requestID, err, sp.bytesProcessed))
	
	// 等待所有后台解析完成
	sp.waitForBackgroundParsing()
	
	// 尝试从部分数据中恢复有用信息
	if len(sp.partialData) > 0 {
		sp.errorRecovery.RecoverFromPartialData(sp.requestID, sp.partialData, time.Since(sp.startTime))
	}
	
	// 尝试获取已解析的Token信息
	var partialTokenUsage *tracking.TokenUsage
	finalUsage := sp.tokenParser.GetFinalUsage()
	modelName := "partial_stream"
	
	if finalUsage != nil {
		// 有部分Token信息，使用已解析的数据
		partialTokenUsage = finalUsage
		if sp.tokenParser.modelName != "" {
			modelName = sp.tokenParser.modelName + "_partial"
		}
		slog.Info(fmt.Sprintf("💾 [部分保存] [%s] 部分流式数据已解析Token，模型: %s, 输入: %d, 输出: %d", 
			sp.requestID, modelName, finalUsage.InputTokens, finalUsage.OutputTokens))
	} else {
		// 没有Token信息，返回空统计
		partialTokenUsage = &tracking.TokenUsage{
			InputTokens: 0, OutputTokens: 0, 
			CacheCreationTokens: 0, CacheReadTokens: 0,
		}
		slog.Info(fmt.Sprintf("💾 [部分保存] [%s] 部分流式数据无Token信息，模型: %s", 
			sp.requestID, modelName))
	}
	
	// 返回Token信息和错误，让上层处理
	return partialTokenUsage, err
}

// handleCancellationV2 处理客户端取消请求（返回Token信息版本）
func (sp *StreamProcessor) handleCancellationV2(ctx context.Context, cancelErr error) (*tracking.TokenUsage, error) {
	slog.Info(fmt.Sprintf("🚫 [客户端取消] [%s] 检测到客户端取消: %v", sp.requestID, cancelErr))
	
	// 等待后台解析完成，但不超过超时时间
	if finished := sp.waitForParsingWithTimeout(2 * time.Second); finished {
		// 成功等待解析完成，收集可用信息
		return sp.collectAvailableInfoV2(cancelErr, "cancelled_with_data")
	} else {
		// 超时未完成，收集部分信息
		return sp.collectAvailableInfoV2(cancelErr, "cancelled_timeout")
	}
}

// collectAvailableInfoV2 智能信息收集（返回Token信息版本）
func (sp *StreamProcessor) collectAvailableInfoV2(cancelErr error, status string) (*tracking.TokenUsage, error) {
	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()
	
	// 尝试获取已解析的信息
	modelName := sp.tokenParser.GetModelName()
	finalUsage := sp.tokenParser.GetFinalUsage()
	
	var tokenUsage *tracking.TokenUsage
	
	if finalUsage != nil && modelName != "" {
		// 有完整Token信息的取消
		tokenUsage = finalUsage
		slog.Info(fmt.Sprintf("💾 [完整取消] [%s] 保存完整Token信息 - 模型: %s, 输入: %d, 输出: %d", 
			sp.requestID, modelName, finalUsage.InputTokens, finalUsage.OutputTokens))
	} else if modelName != "" {
		// 有模型信息但无Token的取消
		tokenUsage = &tracking.TokenUsage{
			InputTokens: 0, OutputTokens: 0, 
			CacheCreationTokens: 0, CacheReadTokens: 0,
		}
		slog.Info(fmt.Sprintf("📝 [部分取消] [%s] 保存模型信息 - 模型: %s (已取消)", 
			sp.requestID, modelName))
	} else {
		// 无任何信息的取消
		tokenUsage = &tracking.TokenUsage{
			InputTokens: 0, OutputTokens: 0, 
			CacheCreationTokens: 0, CacheReadTokens: 0,
		}
		slog.Info(fmt.Sprintf("🚫 [纯取消] [%s] 客户端在连接建立后取消", sp.requestID))
	}
	
	return tokenUsage, cancelErr
}
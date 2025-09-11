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
func (sp *StreamProcessor) ProcessStream(ctx context.Context, resp *http.Response) error {
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
			return sp.handleCancellation(ctx, ctx.Err())
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
				return fmt.Errorf("failed to forward to client: %w", writeErr)
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
			
			// 检查是否已经通过SSE解析记录了完成状态，如果没有则使用fallback
			sp.ensureRequestCompletion()
			
			slog.Info(fmt.Sprintf("✅ [流式完成] [%s] 端点: %s, 流处理正常完成，已处理 %d 字节", 
				sp.requestID, sp.endpoint, sp.bytesProcessed))
			return nil
		}
		
		if err != nil {
			// 网络中断或其他错误，尝试部分数据处理
			return sp.handlePartialStream(err)
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
func (sp *StreamProcessor) processSSELine(line string) {
	// 使用现有的TokenParser进行解析
	tokenUsage := sp.tokenParser.ParseSSELine(line)
	
	if tokenUsage != nil {
		// 解析成功，转换为tracking.TokenUsage并记录到usage tracker
		trackingTokens := &tracking.TokenUsage{
			InputTokens:          int64(tokenUsage.InputTokens),
			OutputTokens:         int64(tokenUsage.OutputTokens),
			CacheCreationTokens:  int64(tokenUsage.CacheCreationTokens),
			CacheReadTokens:      int64(tokenUsage.CacheReadTokens),
		}
		
		// 获取模型名称
		modelName := sp.tokenParser.GetModelName()
		if modelName == "" {
			modelName = "default"
		}
		
		// 记录完成状态到usage tracker
		if sp.usageTracker != nil && sp.requestID != "" && !sp.completionRecorded {
			duration := time.Since(sp.startTime)
			sp.usageTracker.RecordRequestComplete(sp.requestID, modelName, trackingTokens, duration)
			sp.usageTracker.RecordRequestUpdate(sp.requestID, sp.endpoint, "", "completed", 0, 0)
			sp.completionRecorded = true  // 标记已记录完成状态
			
			slog.Info(fmt.Sprintf("🪙 [Token使用统计] [%s] 从流式解析中提取完整令牌使用情况 - 模型: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d", 
				sp.requestID, modelName, trackingTokens.InputTokens, trackingTokens.OutputTokens, trackingTokens.CacheCreationTokens, trackingTokens.CacheReadTokens))
		}
	}
}

// ensureRequestCompletion 确保请求完成状态被记录（fallback机制）
func (sp *StreamProcessor) ensureRequestCompletion() {
	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()
	
	if sp.usageTracker != nil && sp.requestID != "" && !sp.completionRecorded {
		// 如果还没有记录完成状态，使用fallback方式记录
		duration := time.Since(sp.startTime)
		
		// 尝试从TokenParser获取最终使用统计
		finalUsage := sp.tokenParser.GetFinalUsage()
		modelName := sp.tokenParser.GetModelName()
		
		if finalUsage != nil && modelName != "" {
			// 有完整的token和模型信息
			sp.usageTracker.RecordRequestComplete(sp.requestID, modelName, finalUsage, duration)
			slog.Info(fmt.Sprintf("🪙 [Token使用统计] [%s] 从TokenParser获取最终令牌使用情况 - 模型: %s, 输入: %d, 输出: %d", 
				sp.requestID, modelName, finalUsage.InputTokens, finalUsage.OutputTokens))
		} else {
			// 没有token信息，使用默认值记录完成状态
			emptyTokens := &tracking.TokenUsage{
				InputTokens: 0, OutputTokens: 0, 
				CacheCreationTokens: 0, CacheReadTokens: 0,
			}
			defaultModel := "default"
			if modelName != "" {
				defaultModel = modelName
			}
			
			sp.usageTracker.RecordRequestComplete(sp.requestID, defaultModel, emptyTokens, duration)
			slog.Info(fmt.Sprintf("🎯 [无Token完成] [%s] 流式响应不包含token信息，标记为完成，模型: %s", 
				sp.requestID, defaultModel))
		}
		
		sp.usageTracker.RecordRequestUpdate(sp.requestID, sp.endpoint, "", "completed", 0, 0)
		sp.completionRecorded = true
	}
}

// handlePartialStream 处理部分数据流中断情况（修复版本）
// 当网络中断或其他错误发生时，不再进行错误分类，让上层统一处理
func (sp *StreamProcessor) handlePartialStream(err error) error {
	// 仅记录流处理中断，不再进行错误分类
	slog.Warn(fmt.Sprintf("⚠️ [流式中断] [%s] 流处理中断: %v, 已处理 %d 字节. 错误将由上层统一处理.", 
		sp.requestID, err, sp.bytesProcessed))
	
	// 等待所有后台解析完成
	sp.waitForBackgroundParsing()
	
	// 尝试从部分数据中恢复有用信息
	if len(sp.partialData) > 0 {
		sp.errorRecovery.RecoverFromPartialData(sp.requestID, sp.partialData, time.Since(sp.startTime))
	}
	
	// 检查是否有有效的Token数据，并记录部分完成
	if sp.usageTracker != nil && sp.requestID != "" {
		duration := time.Since(sp.startTime)
		emptyTokens := &tracking.TokenUsage{
			InputTokens: 0, OutputTokens: 0, 
			CacheCreationTokens: 0, CacheReadTokens: 0,
		}
		modelName := "partial_stream"
		if sp.tokenParser.modelName != "" {
			modelName = sp.tokenParser.modelName + "_partial"
		}
		sp.usageTracker.RecordRequestComplete(sp.requestID, modelName, emptyTokens, duration)
		sp.usageTracker.RecordRequestUpdate(sp.requestID, sp.endpoint, "", "partial_complete", 0, 0)
		
		slog.Info(fmt.Sprintf("💾 [部分保存] [%s] 部分流式数据已保存，模型: %s", 
			sp.requestID, modelName))
	}
	
	// 直接返回错误，让调用者(handler)来分类和处理最终失败
	return err
}

// ProcessStreamWithRetry 支持网络中断恢复的流式处理（增强版本）
// 在网络不稳定环境下提供智能重试机制
func (sp *StreamProcessor) ProcessStreamWithRetry(ctx context.Context, resp *http.Response) error {
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
				return lastErr
			}
			
			// 执行重试延迟
			if retryErr := sp.errorRecovery.ExecuteRetry(ctx, errorCtx); retryErr != nil {
				return retryErr
			}
		}
		
		// 尝试流式处理
		err := sp.ProcessStream(ctx, resp)
		
		if err == nil {
			// 处理成功
			if attempt > 0 {
				slog.Info(fmt.Sprintf("✅ [重试成功] [%s] 第 %d 次重试成功", sp.requestID, attempt))
			}
			return nil
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
		return err
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
	
	return fmt.Errorf("stream processing failed after %d retries", maxRetries)
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
func (sp *StreamProcessor) handleCancellation(ctx context.Context, cancelErr error) error {
	slog.Info(fmt.Sprintf("🚫 [客户端取消] [%s] 检测到客户端取消: %v", sp.requestID, cancelErr))
	
	// 等待后台解析完成，但不超过超时时间
	if finished := sp.waitForParsingWithTimeout(2 * time.Second); finished {
		// 成功等待解析完成，收集可用信息
		return sp.collectAvailableInfo(cancelErr, "cancelled_with_data")
	} else {
		// 超时未完成，收集部分信息
		return sp.collectAvailableInfo(cancelErr, "cancelled_timeout")
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
func (sp *StreamProcessor) collectAvailableInfo(cancelErr error, status string) error {
	sp.parseMutex.Lock()
	defer sp.parseMutex.Unlock()
	
	// 记录取消时间
	duration := time.Since(sp.startTime)
	
	// 尝试获取已解析的信息
	modelName := sp.tokenParser.GetModelName()
	finalUsage := sp.tokenParser.GetFinalUsage()
	
	if sp.usageTracker != nil && sp.requestID != "" && !sp.completionRecorded {
		if finalUsage != nil && modelName != "" {
			// 有完整Token信息的取消
			sp.usageTracker.RecordRequestComplete(sp.requestID, modelName, finalUsage, duration)
			slog.Info(fmt.Sprintf("💾 [完整取消] [%s] 保存完整Token信息 - 模型: %s, 输入: %d, 输出: %d", 
				sp.requestID, modelName, finalUsage.InputTokens, finalUsage.OutputTokens))
		} else if modelName != "" {
			// 有模型信息但无Token的取消
			emptyTokens := &tracking.TokenUsage{
				InputTokens: 0, OutputTokens: 0, 
				CacheCreationTokens: 0, CacheReadTokens: 0,
			}
			sp.usageTracker.RecordRequestComplete(sp.requestID, modelName+"_cancelled", emptyTokens, duration)
			slog.Info(fmt.Sprintf("📝 [部分取消] [%s] 保存模型信息 - 模型: %s (已取消)", 
				sp.requestID, modelName))
		} else {
			// 无任何信息的取消
			emptyTokens := &tracking.TokenUsage{
				InputTokens: 0, OutputTokens: 0, 
				CacheCreationTokens: 0, CacheReadTokens: 0,
			}
			sp.usageTracker.RecordRequestComplete(sp.requestID, "cancelled", emptyTokens, duration)
			slog.Info(fmt.Sprintf("🚫 [纯取消] [%s] 客户端在连接建立后取消", sp.requestID))
		}
		
		// 更新请求状态为取消
		sp.usageTracker.RecordRequestUpdate(sp.requestID, sp.endpoint, "", status, 0, 0)
		sp.completionRecorded = true
	}
	
	return cancelErr
}
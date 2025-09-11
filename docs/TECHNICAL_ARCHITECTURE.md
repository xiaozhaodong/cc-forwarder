# Technical Architecture Documentation

## Detailed Component Architecture

### Stream Processor v2 (`internal/proxy/stream_processor.go`)

**528 lines**: Advanced streaming request processor with comprehensive capabilities

**Core Features**:
- **Cancellation Support**: Graceful handling of client disconnections
- **Error Recovery**: Automatic error detection and recovery mechanisms
- **Token Integration**: Real-time token parsing during streaming
- **Performance Optimization**: Efficient buffering and flushing mechanisms

**Implementation Details**:
```go
type StreamProcessor struct {
    tokenParser     *TokenParser
    usageTracker   *tracking.UsageTracker
    writer         http.ResponseWriter
    flusher        http.Flusher
    requestID      string
    endpointName   string
}
```

### Error Recovery System (`internal/proxy/error_recovery.go`)

**475 lines**: Intelligent error classification and recovery system

**Error Categories**:
- **Network Errors**: Connection timeouts, DNS failures, network unreachable
- **API Errors**: Rate limiting, authentication failures, service unavailable
- **Streaming Errors**: Connection drops, parsing failures, protocol errors
- **Unknown Errors**: Unclassified errors with fallback handling

**Recovery Strategies**:
```go
type ErrorRecoveryManager struct {
    usageTracker *tracking.UsageTracker
    strategies   map[ErrorType]RecoveryStrategy
}
```

### Request Lifecycle Management (`internal/proxy/lifecycle_manager.go`)

**279 lines**: Complete request state tracking and management

**State Tracking**:
- **Status Transitions**: Comprehensive tracking from initiation to completion
- **Duration Monitoring**: Accurate timing measurements for performance analysis
- **Endpoint Tracking**: Record of all attempted endpoints and results
- **Integration Hub**: Central coordination point for all request components

**Lifecycle States**:
```go
type RequestLifecycleManager struct {
    requestID     string
    startTime     time.Time
    currentStatus string
    endpoint      string
    group         string
    retryCount    int
}
```

## Request Status System Implementation

### Status Definitions and Transitions

#### **Core Status States**
- **`pending`**: Initial request state, not yet forwarded
- **`forwarding`**: Request is being forwarded to endpoint  
- **`retry`**: Request is being retried (same endpoint or different endpoint)
- **`processing`**: HTTP response received successfully, Token parsing in progress
- **`completed`**: Token parsing and cost calculation fully completed
- **`suspended`**: Request temporarily suspended waiting for group recovery
- **`error`**: Request failed at any stage
- **`timeout`**: Request timed out

#### **Status Update Triggers**
1. **`pending`** → Set at request start (RecordRequestStart)
2. **`forwarding`** → Set when first attempting an endpoint
3. **`retry`** → Set when retrying same endpoint or switching to new endpoint
4. **`suspended`** → Set when all groups fail, request waits for recovery
5. **`processing`** → Set when HTTP response returns successfully
6. **`completed`** → Set when token parsing completes

### User Experience Improvements

#### **Before Enhancement (User Confusion)**
```
req-abc123  22:15:33  ✅ 成功  -  0  0  0  0  $0.00
                        ↑
                "成功了为什么token是0？？？"
```

#### **After Enhancement (Clear Status)**
```
req-abc123  22:15:33  ⚙️ 解析中  -  0  0  0  0  $0.00
                        ↓ Auto-updates after token parsing
req-abc123  22:15:33  ✅ 完成    claude-sonnet-4  25  97  0  0  $0.45
                        ↑
              "Perfect! Now I understand the processing is complete!"
```

### Visual Design

#### **Status Indicators in Web Interface**
- **🔄 转发中** (`forwarding`): Blue gradient with pulsing animation
- **⚙️ 解析中** (`processing`): Orange gradient with pulsing animation
- **✅ 完成** (`completed`): Green gradient
- **❌ 失败** (`error`): Red gradient
- **⏰ 超时** (`timeout`): Orange-red gradient

#### **CSS Implementation**
```css
.status-badge.status-processing {
    background: linear-gradient(135deg, #fbbf24, #f59e0b);
    color: #92400e;
    animation: pulse 2s infinite;
}

.status-badge.status-completed {
    background: linear-gradient(135deg, #10b981, #059669);
    color: white;
}
```

### Technical Implementation

#### **Backend Status Updates**
- **File**: `internal/proxy/retry.go` (Line 181)
  - **Change**: `status := "processing"` (was `"success"`)
  - **Trigger**: HTTP response successful
  
- **File**: `internal/proxy/token_parser.go` (Line 209)  
  - **Addition**: `RecordRequestUpdate(requestID, "", "", "completed", 0, 0)`
  - **Trigger**: Token parsing completed

#### **Frontend Status Display**
- **File**: `internal/web/static/js/utils.js`
  - **Enhancement**: Added status mappings for `processing` and `completed`
  - **Backward Compatibility**: Maintains support for legacy `success` status

## Token Parser and Model Detection

**Architecture Fix**: Resolved critical token parsing duplication bug where both `message_start` and `message_delta` events were processing token usage

**Correct Event Separation**: 
- `message_start` now only extracts model information
- `message_delta` handles complete token usage statistics

**Model Name Extraction**: Automatically extracts Claude model information (e.g., `claude-3-haiku-20240307`) from `message_start` events

**Non-Claude Endpoint Compatibility**: Added fallback mechanism in `message_delta` for endpoints that don't provide token usage information

**Clear Logging Separation**: 
```
🎯 [模型提取] [req-xxxxxxxx] 从message_start事件中提取模型信息: claude-3-5-haiku
🪙 [Token使用统计] [req-xxxxxxxx] 从message_delta事件中提取完整令牌使用情况 - 模型: claude-3-5-haiku, 输入: 25, 输出: 97, 缓存创建: 0, 缓存读取: 0
```

## Non-Token Response Handling

**Enhanced Compatibility**: The system provides intelligent fallback mechanisms for responses that don't contain token usage information.

### Problem Solved
Previously, requests that returned successful HTTP responses (200 OK) but contained no token information would remain indefinitely in `processing` status.

### Common Non-Token Response Types
- **Health Check Requests**: `/v1/models` endpoint returning model lists
- **Third-party APIs**: Non-Claude compatible endpoints without usage tracking
- **Configuration Queries**: System configuration or status endpoints  
- **Error Responses**: Non-standard error formats without usage data

### Fallback Implementation 
**File**: `internal/proxy/handler.go` (analyzeResponseForTokens function)

```go
// Fallback: No token information found, mark request as completed with default model
if h.usageTracker != nil && connID != "" {
    emptyTokens := &tracking.TokenUsage{
        InputTokens: 0, OutputTokens: 0, 
        CacheCreationTokens: 0, CacheReadTokens: 0,
    }
    h.usageTracker.RecordRequestComplete(connID, "default", emptyTokens, 0)
    h.usageTracker.RecordRequestUpdate(connID, "", "", "completed", 0, 0)
}
```

### Enhanced Logging
- **Response Content**: Info-level logging shows complete response content for analysis
- **Non-Token Detection**: Clear identification of responses without token information

### Database Storage
Non-token requests are properly stored with:
- **Status**: `completed` (no longer stuck in `processing`)
- **Model Name**: `default` for identification and filtering
- **Token Counts**: All set to 0 (accurate representation)
- **Total Cost**: $0.00 (no AI processing cost incurred)

## Request Suspension and Recovery System

**Request Suspension Capability**: The system supports intelligent request suspension when all endpoint groups fail, preventing request loss during temporary outages.

### Configuration
```yaml
request_suspend:
  enabled: true                # Enable request suspension functionality
  timeout: "300s"             # Maximum suspension time (5 minutes default)
  max_suspended_requests: 100 # Maximum number of requests that can be suspended simultaneously
```

### Suspension Behavior
- **Automatic Suspension**: When all groups fail, requests are automatically suspended instead of being dropped
- **Group Recovery Detection**: System continuously monitors for group recovery and automatically resumes suspended requests
- **FIFO Processing**: Suspended requests are processed in first-in-first-out order when groups become available
- **Timeout Protection**: Requests suspended longer than the configured timeout are automatically failed to prevent indefinite hanging
- **Capacity Management**: System limits the number of suspended requests to prevent memory exhaustion

### Request Lifecycle with Suspension
1. **Normal Processing**: Request forwarded to available endpoint in active group
2. **Group Failure**: If current group fails, system attempts other available groups
3. **Total Failure**: If all groups fail, request is suspended with log: `⏸️ [请求挂起] 连接 req-xxxxxxxx 请求已挂起`
4. **Group Recovery**: When any group recovers, suspended requests resume processing
5. **Successful Recovery**: Resumed request processed normally: `🔄 [请求恢复] 连接 req-xxxxxxxx 请求已恢复处理`
6. **Timeout Handling**: Long-suspended requests fail gracefully: `⏰ [请求超时] 连接 req-xxxxxxxx 挂起超时`

### Manual Group Management Integration
The request suspension system works seamlessly with manual group management:
- **Manual Activation**: Administrators can manually activate groups via Web interface to resume suspended requests
- **Strategic Recovery**: Different groups can be activated based on current conditions and performance requirements
- **Load Distribution**: Suspended requests distribute across newly activated endpoints

## Usage Tracking System Implementation

### Complete Request Lifecycle Tracking

**Request Tracking Interface**: The Web interface includes a comprehensive request tracking page that replaces the simple logs view.

**Features**:
- **Multi-dimensional Filtering**: Filter by date range, status, model, endpoint, group
- **Real-time Updates**: Live request monitoring via SSE integration  
- **Detailed View**: Complete request lifecycle with timing, tokens, and cost information
- **Export Capabilities**: CSV/JSON export with flexible filtering options
- **Performance Analytics**: Statistical summaries and trends
- **Pagination Support**: Efficient browsing of large request datasets

### Database Schema and Timezone Handling

The system uses SQLite with WAL mode for high-performance usage tracking. **All timestamp fields use local timezone (CST +08:00)** for accurate time recording:

```sql
-- Correct timezone configuration in schema.sql
created_at DATETIME DEFAULT (datetime('now', 'localtime')),
updated_at DATETIME DEFAULT (datetime('now', 'localtime'))

-- Triggers also use local time
UPDATE table_name SET updated_at = datetime('now', 'localtime') WHERE id = NEW.id;
```

### Asynchronous Operation Design

**Complete Non-Blocking Architecture**:
- **Event Channel**: Buffered channel (default 1000 events) with non-blocking send
- **Batch Processing**: Groups events for efficient database writes (default 100 events/batch)
- **Independent Goroutines**: Separate processing threads prevent main request blocking
- **Graceful Degradation**: Event dropping on buffer overflow (with logging) instead of blocking

### Critical Bug Fixes (2025-09-09)

**Token Parser Architecture Overhaul**:
- **Problem**: Both `message_start` and `message_delta` events were processing token usage, causing double token counting and incorrect cost calculations
- **Solution**: Clear separation - `message_start` only extracts model information, `message_delta` handles complete token statistics
- **Impact**: Accurate cost calculations, no more duplicate token counting, proper fallback for non-Claude endpoints

**Database Status Logic Fix**:
- **Problem**: `completeRequest` SQL only updated `pending → completed`, leaving `processing` requests stuck
- **Solution**: Changed SQL to `status = CASE WHEN status != 'completed' THEN 'completed' ELSE status END`
- **Impact**: All request states now properly transition to completed

**Retry Status Tracking Enhancement**:
- **Problem**: Same-endpoint retries weren't updating status to `retry`, only cross-endpoint switches
- **Solution**: Added status update to `retry` for all retry attempts (internal/proxy/retry.go:242-245)
- **Impact**: Complete visibility into retry behavior, better debugging and monitoring

### Data Collection Points

**Request Lifecycle Tracking**:
```go
// 1. Request Start (middleware/logging.go:76)
usageTracker.RecordRequestStart(requestID, clientIP, userAgent)

// 2. Status Updates (proxy/retry.go:154,185,211)  
usageTracker.RecordRequestUpdate(requestID, endpoint, group, status, retryCount, httpStatus)

// 3. Token Completion (proxy/token_parser.go:206)
usageTracker.RecordRequestComplete(requestID, modelName, tokens, duration)
```

### Model Detection and Token Parsing

**Dual SSE Event Processing**:
- **message_start**: Extracts model information (e.g., `claude-3-5-haiku-20241022`)
- **message_delta**: Processes token usage from response streams
- **Integrated Logging**: Model info included in token usage logs
- **Safe Implementation**: Model extraction doesn't affect client responses

### Cost Calculation

**Real-time Pricing Integration**:
```yaml
model_pricing:
  "claude-sonnet-4-20250514":
    input: 3.00          # USD per 1M tokens
    output: 15.00
    cache_creation: 3.75 # 1.25x input for cache creation
    cache_read: 0.30     # 0.1x input for cache reads
```

### Performance Characteristics

**Verified Operation Metrics** (2025-09-05):
- **Zero Blocking**: All database operations asynchronous 
- **Accurate Timezone**: CST +08:00 timestamps in all fields
- **Model Detection**: 100% success rate for SSE streams with model info
- **Cost Tracking**: Precise calculation including cache token costs
- **Example Usage**: 5 requests, $0.044938 total cost, 1,148 input + 97 output tokens

### Data Export Capabilities

**Multi-format Export Support**:
```go
// CSV Export with full request lifecycle
tracker.ExportToCSV(ctx, startTime, endTime, modelName, endpointName, groupName)

// JSON Export for programmatic access  
tracker.ExportToJSON(ctx, startTime, endTime, modelName, endpointName, groupName)
```

## Troubleshooting Guide

### Common Issues Resolved
1. **Timezone Problems**: Use `datetime('now', 'localtime')` instead of `CURRENT_TIMESTAMP`
2. **Model Name Missing**: Ensure SSE streams contain `message_start` events
3. **High Costs**: Monitor cache token usage (cache_creation_tokens, cache_read_tokens)
4. **Performance Impact**: All tracking operations are fully asynchronous and non-blocking

### JavaScript Module Architecture

**Modern Frontend Design** (2025-09-05 Update):
The Web interface uses a modular JavaScript architecture for enhanced maintainability:

- **utils.js** (302 lines): Formatting functions, notifications, DOM utilities
- **sseManager.js** (430 lines): SSE connections, reconnection logic, event handling
- **requestsManager.js** (512 lines): Request tracking, filtering, pagination, export
- **groupsManager.js** (357 lines): Group operations, status management
- **endpointsManager.js** (428 lines): Endpoint monitoring, priority management
- **webInterface.js** (494 lines): Core class, tab management, initialization

**Benefits**:
- **Maintainability**: Each module has focused responsibilities (~200-500 lines each)
- **Team Collaboration**: Multiple developers can work on different modules simultaneously  
- **Code Reuse**: Utility functions shared across modules
- **Debugging**: Issues can be isolated to specific modules
- **Performance**: Modules loaded in optimized order with intelligent caching
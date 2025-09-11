# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Version Information

**Current Version**: v2.0 Architecture (2025-09-11)
**Major Update**: Complete proxy architecture redesign with enhanced streaming and error recovery

### Recent Updates

**2025-09-11**: Major architecture upgrade
- Stream Processor v2 with advanced streaming capabilities
- Intelligent error recovery and classification system
- Complete request lifecycle management
- 25+ comprehensive test files added
- Unified request processing architecture
- Enhanced logging with architecture identification

**2025-09-09**: Token parsing and status system enhancements  
- Fixed token parsing duplication bug
- Enhanced request status granularity  
- Improved user experience in Web interface

**2025-09-05**: Web handler refactoring and JavaScript modularization
- Modular web handler architecture (11 specialized files)
- Modern JavaScript module system
- Comprehensive request tracking interface

## Project Overview

Claude Request Forwarder is a high-performance Go application that transparently forwards Claude API requests to multiple endpoints with intelligent routing, health checking, and automatic retry/fallback capabilities. It includes both a Terminal User Interface (TUI) and Web Interface for real-time monitoring and management.

**Key Features v2.0**:
- **Dual Architecture**: Streaming v2 and Unified v2 request processing
- **Intelligent Error Recovery**: Smart error classification and recovery strategies  
- **Complete Lifecycle Tracking**: End-to-end request monitoring and analytics
- **Advanced Streaming**: Real-time SSE processing with cancellation support
- **Comprehensive Testing**: 25+ test files with extensive coverage

## Build and Development Commands

```bash
# Build the application
go build -o cc-forwarder

# Run with default configuration and TUI
./cc-forwarder -config config/config.yaml

# Run without TUI (console mode)
./cc-forwarder -config config/config.yaml --no-tui

# Run tests
go test ./...

# Test specific packages
go test ./internal/endpoint
go test ./internal/proxy
go test ./internal/middleware

# Check version
./cc-forwarder -version
```

## Architecture Overview

### Core Components

- **`main.go`**: Application entry point with TUI/console mode switching, graceful shutdown, and configuration management
- **`config/`**: Configuration management with hot-reloading via fsnotify
- **`internal/endpoint/`**: Endpoint management, health checking, fast testing, and group management
- **`internal/proxy/`**: HTTP request forwarding, streaming support, and retry logic with configurable group switching
- **`internal/middleware/`**: Authentication, logging, and monitoring middleware
- **`internal/tui/`**: Terminal User Interface using rivo/tview
- **`internal/web/`**: Web Interface with real-time monitoring, SSE support, and group management
- **`internal/transport/`**: HTTP/HTTPS/SOCKS5 proxy transport configuration

### Proxy Architecture v2 (2025-09-11 Update)

**Major Architecture Enhancement**: The proxy system has been completely redesigned with enhanced streaming capabilities, intelligent error recovery, and comprehensive lifecycle management.

#### **Stream Processor v2**
- **`internal/proxy/stream_processor.go`**: Advanced streaming request processor with cancellation support and error recovery
- **Token Integration**: Seamless integration with token parsing and usage tracking
- **Performance Optimization**: Efficient buffering and flushing mechanisms
- **Error Resilience**: Automatic error detection and recovery for streaming connections

#### **Error Recovery Management**
- **`internal/proxy/error_recovery.go`**: Intelligent error classification and recovery system
- **Smart Categorization**: Automatic classification of network, API, and streaming errors
- **Recovery Strategies**: Context-aware error handling with appropriate recovery actions
- **User Feedback**: Clear error messaging and status reporting

#### **Request Lifecycle Management** 
- **`internal/proxy/lifecycle_manager.go`**: Complete request state tracking and management
- **Status Transitions**: Comprehensive tracking from initiation to completion
- **Duration Monitoring**: Accurate timing measurements for performance analysis
- **Integration Hub**: Central coordination point for all request-related components

#### **Unified Request Handling**
- **Architecture Identification**: Clear distinction between streaming (v2) and regular (unified v2) requests
- **Enhanced Logging**: All requests now include architecture type in logs:
  ```
  🌊 [流式架构] [req-xxxxxxxx] 使用streaming v2架构
  🔄 [常规架构] [req-xxxxxxxx] 使用unified v2架构
  ```
- **Consistent Processing**: Unified error handling and status tracking across both request types

### Key Design Patterns

**Strategy Pattern**: Endpoint selection via "priority" or "fastest" strategies with optional pre-request fast testing

**Middleware Chain**: Request processing through authentication, logging, and monitoring layers

**Observer Pattern**: Configuration hot-reloading with callback-based component updates

**Circuit Breaker Pattern**: Health checking with automatic endpoint marking as healthy/unhealthy

**Factory Pattern**: Request processor creation based on request type (streaming vs regular)

**State Machine Pattern**: Request lifecycle management with clear state transitions

### Request Flow v2

#### **Enhanced Request Processing Pipeline**
1. **Request Reception**: Middleware chain processing (auth → logging → monitoring)
2. **Architecture Detection**: Automatic SSE stream detection and processor selection
3. **Lifecycle Initialization**: Request ID generation and lifecycle manager setup
4. **Endpoint Selection**: Strategy-based selection with health status validation
5. **Request Processing**: 
   - **Streaming Requests**: Stream Processor v2 with real-time token parsing
   - **Regular Requests**: Unified processor with response analysis
6. **Error Handling**: Intelligent error classification and recovery attempts
7. **Status Tracking**: Complete lifecycle monitoring with granular status updates
8. **Response Delivery**: Optimized delivery with proper connection management

#### **Status Flow Enhancement**
```
正常流程: pending → forwarding → processing → completed
流式流程: pending → forwarding → streaming → processing → completed
重试流程: pending → forwarding → retry → processing → completed
错误恢复: pending → forwarding → error_recovery → retry → completed
```

## Configuration

- **Primary config**: `config/config.yaml` (copy from `config/example.yaml`)
- **Hot-reloading**: Automatic configuration reload via fsnotify with 500ms debounce
- **Dynamic Token Resolution**: Tokens and API keys are resolved dynamically at runtime for group-based failover
- **Global timeout**: Default timeout for all non-streaming requests (5 minutes)
- **API Key support**: Endpoints can specify `api-key` field which is automatically passed as `X-Api-Key` header

### Interface Configuration

**Web Interface** (recommended for production):
```yaml
web:
  enabled: true              # Enable web interface
  host: "0.0.0.0"           # Web interface host (default: localhost)
  port: 8010                 # Web interface port (default: 8088)
```

**TUI Interface** (development/debugging):
```yaml
tui:
  enabled: false             # Disable in production/Docker environments
  update_interval: "1s"      # TUI refresh interval
  save_priority_edits: false # Save priority changes to config file
```

### Advanced Group Configuration

**Group Switching Control**:
```yaml
group:
  cooldown: "600s"                      # Group failure cooldown duration
  auto_switch_between_groups: true      # Enable automatic group switching (default: true)
  # false = Manual intervention required via Web interface
  # true = Automatic failover to backup groups
```

**Configuration Behavior**:
- **Auto Mode** (`auto_switch_between_groups: true`): System automatically switches to backup groups when primary group fails
- **Manual Mode** (`auto_switch_between_groups: false`): Requires manual intervention through Web interface when group fails
- **Backward Compatibility**: Missing parameter defaults to automatic mode

### Group Management

The system supports endpoint grouping with automatic failover and cooldown mechanisms:

**Group Configuration**:
- Each endpoint can belong to a group using the `group` field
- Groups have priorities defined by `group-priority` (lower number = higher priority)
- Only one group is active at a time based on priority and cooldown status
- Groups inherit settings from the first endpoint in their group

**Group Behavior**:
- **Active Group**: The highest priority group not in cooldown or manually paused
- **Cooldown**: When all endpoints in a group fail, the group enters cooldown mode
- **Manual Control**: Groups can be manually paused, resumed, or activated via Web interface with full lifecycle management
- **Configurable Switching**: Auto/manual group switching controlled by `group.auto_switch_between_groups`

### Manual Group Operations
**Web Interface Controls**: The Web interface provides comprehensive group management capabilities:
- **Activate Group**: `POST /api/v1/groups/{name}/activate` - Forces a group to become active immediately, bypassing cooldown
- **Pause Group**: `POST /api/v1/groups/{name}/pause` - Manually pauses a group, preventing it from being selected
- **Resume Group**: `POST /api/v1/groups/{name}/resume` - Resumes a paused group, making it available for selection

**Group States and Transitions**:
- **Active**: Currently processing requests (only one group can be active at a time)
- **Available**: Healthy and ready to be activated, but not currently active
- **Cooldown**: Temporarily disabled due to failures, waiting for cooldown period to expire
- **Paused**: Manually disabled by administrator, requires manual resumption
- **Unhealthy**: All endpoints in the group are down

**Manual Activation Scenarios**:
1. **Emergency Failover**: Quickly switch to backup groups during primary group issues
2. **Maintenance Mode**: Pause primary groups for maintenance, activate backup groups
3. **Performance Optimization**: Activate faster responding groups during high load periods
4. **Geographic Routing**: Manually activate region-specific groups based on user distribution

**Integration with Request Suspension**:
- Manual group activation immediately triggers processing of suspended requests
- Administrators can strategically activate specific groups to handle different types of suspended requests
- Real-time feedback shows how many suspended requests are processed upon group activation
- **Cooldown Duration**: Configurable via `group.cooldown` (default: 10 minutes)

**Group Inheritance Rules**:
- Endpoints inherit `group` and `group-priority` from previous endpoints if not specified
- Static inheritance: `timeout` and `headers` are inherited during configuration parsing
- Dynamic resolution: `token` and `api-key` are resolved at runtime from the first endpoint in the same group
- Groups can be mixed and matched with different priorities for failover scenarios

**Dynamic Token Resolution**:
- **Runtime Resolution**: Tokens and API keys are not inherited during config parsing but resolved dynamically at request time
- **Group-based Sharing**: All endpoints in a group share the token/api-key from the first endpoint that defines it
- **Override Support**: Individual endpoints can override group tokens by explicitly specifying their own `token` or `api-key`
- **Failover-friendly**: When groups switch during failover, the new active group's tokens are automatically used

**Example Group Configuration**:
```yaml
endpoints:
  # Primary group (highest priority) - defines group token
  - name: "primary"
    url: "https://api.openai.com"
    group: "main"
    group-priority: 1
    priority: 1
    token: "sk-main-group-token"        # 🔑 Shared by all main group endpoints
    
  # Backup for primary group - uses main group token
  - name: "primary_backup"
    url: "https://api.anthropic.com"
    priority: 2
    # 🔄 Inherits group: "main" and group-priority: 1
    # 🔑 Uses main group token dynamically at runtime
    
  # Secondary group (lower priority) - defines different group token
  - name: "secondary"
    url: "https://api.example.com"
    group: "backup"
    group-priority: 2
    priority: 1
    token: "sk-backup-group-token"      # 🔑 Shared by all backup group endpoints
    
  # Custom override within backup group
  - name: "secondary_special"
    url: "https://api.special.com"
    priority: 2
    token: "sk-special-override"        # 🔑 Overrides backup group token
    # 🔄 Still inherits group: "backup" and group-priority: 2
```

## Testing Approach (2025-09-11 Update)

**Comprehensive Test Architecture**: The codebase has been significantly enhanced with extensive test coverage following Go best practices.

### Test Organization

The testing structure follows Go conventions with tests co-located with source code:

#### **Unit Tests** (Internal Package Testing)
- **Location**: `*_test.go` files alongside source code in each package
- **Access**: Can test unexported functions and internal implementation details
- **Coverage**: Core business logic, edge cases, and error conditions
- **Examples**:
  ```bash
  internal/proxy/handler_test.go           # Request handler testing
  internal/proxy/stream_processor_test.go  # Streaming logic testing
  internal/proxy/error_recovery_test.go    # Error handling testing
  internal/endpoint/manager_test.go        # Endpoint management testing
  internal/tracking/tracker_test.go        # Usage tracking testing
  ```

#### **Integration Tests** (External Testing)
- **Location**: `tests/integration/` directory 
- **Access**: Test through public APIs and interfaces
- **Coverage**: End-to-end workflows and component integration
- **Examples**:
  ```bash
  tests/integration/request_suspend/       # Request suspension workflows
  tests/unit/monitor/                     # External monitoring interfaces
  ```

### Test Categories and Coverage

#### **Core Proxy Testing**
- **Stream Processing**: Comprehensive streaming request handling with cancellation
- **Error Recovery**: All error types (network, API, streaming) with recovery strategies
- **Lifecycle Management**: Complete request state transitions and timing
- **Token Parsing**: SSE event processing and model detection
- **Retry Logic**: Multi-endpoint retry scenarios with group switching

#### **Performance Testing** 
- **`internal/proxy/performance_test.go`**: Load testing and performance benchmarks
- **Concurrent Request Handling**: Multi-threaded request processing validation
- **Memory Efficiency**: Resource usage monitoring and leak detection
- **Response Time Analysis**: Latency measurements and optimization validation

#### **Integration Testing**
- **Request Suspension**: Complete suspension/recovery workflows
- **Group Management**: Multi-group failover and manual intervention scenarios  
- **Usage Tracking**: End-to-end request tracking and data export
- **Stream Token Integration**: Real-time token parsing during streaming

### Test Configuration

- **Test Configuration**: `test_config.yaml` for test-specific settings
- **Mock Endpoints**: Configurable test servers for controlled testing environments
- **Database Testing**: In-memory SQLite for isolated database tests
- **Concurrency Testing**: Multi-goroutine safety validation

### Testing Commands

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific module tests
go test ./internal/proxy/...
go test ./internal/endpoint/...
go test ./internal/tracking/...

# Run integration tests only
go test ./tests/...

# Run performance tests
go test -bench=. ./internal/proxy/

# Run tests with race detection
go test -race ./...

# Verbose test output
go test -v ./internal/proxy/
```

### Test Quality Metrics

**Coverage Statistics** (2025-09-11):
- **Total Test Files**: 25+ comprehensive test files
- **Code Coverage**: High coverage of critical paths and error conditions  
- **Test Scenarios**: 200+ individual test cases covering normal and edge cases
- **Performance Tests**: Dedicated benchmarking for critical components
- **Integration Workflows**: Complete end-to-end testing scenarios

**Test Categories**:
- **Unit Tests**: 20 files testing individual components and functions
- **Integration Tests**: 5 files testing system workflows and interactions
- **Performance Tests**: Dedicated benchmarking and load testing
- **Edge Case Testing**: Comprehensive error condition and boundary testing

## Request ID Tracking and Lifecycle Monitoring

**Request ID Generation**: The system generates unique short UUID-based request IDs in the format `req-xxxxxxxx` (8 hex characters) for every incoming request, replacing the previous timestamp-based format.

**Complete Lifecycle Tracking**: Each request can be traced through its entire lifecycle using the request ID:

```
🚀 Request started [req-4167c856]
🎯 [请求转发] [req-4167c856] 选择端点: instcopilot-sg (组: main, 总尝试 1)
✅ [请求成功] [req-4167c856] 端点: instcopilot-sg (组: main), 状态码: 200 (总尝试 1 个端点)
✅ Request completed [req-4167c856]
```

**Log Coverage**: Request IDs are included in all critical log events:
- **Request Start/End**: `🚀 Request started [req-xxxxxxxx]` and `✅ Request completed [req-xxxxxxxx]`
- **Endpoint Selection**: `🎯 [请求转发] [req-xxxxxxxx] 选择端点: endpoint-name`
- **Success/Failure**: `✅ [请求成功] [req-xxxxxxxx]` or `❌ [网络错误] [req-xxxxxxxx]`
- **Retry Logic**: `🔄 [需要重试] [req-xxxxxxxx]` and `⏳ [等待重试] [req-xxxxxxxx]`
- **Request Suspension**: `⏸️ [请求挂起] 连接 req-xxxxxxxx 请求已挂起`
- **Error Handling**: `⚠️ Request error [req-xxxxxxxx]` and `🐌 Slow request detected [req-xxxxxxxx]`

**Implementation Details**:
- **UUID Generation**: Uses `crypto/rand` with 4-byte random values converted to hex
- **Context Propagation**: Request ID flows through all middleware layers via `context.WithValue`
- **Memory Management**: Connection tracking integrated with monitoring system
- **Debugging**: Easy log filtering using `grep "req-xxxxxxxx" logfile` for complete request analysis

**Token Parser and Model Detection** (2025-09-09 Update):
- **Architecture Fix**: Resolved critical token parsing duplication bug where both `message_start` and `message_delta` events were processing token usage
- **Correct Event Separation**: `message_start` now only extracts model information, `message_delta` handles complete token usage statistics
- **Model Name Extraction**: Automatically extracts Claude model information (e.g., `claude-3-haiku-20240307`) from `message_start` events
- **Non-Claude Endpoint Compatibility**: Added fallback mechanism in `message_delta` for endpoints that don't provide token usage information
- **Clear Logging Separation**: 
  ```
  🎯 [模型提取] [req-xxxxxxxx] 从message_start事件中提取模型信息: claude-3-5-haiku
  🪙 [Token使用统计] [req-xxxxxxxx] 从message_delta事件中提取完整令牌使用情况 - 模型: claude-3-5-haiku, 输入: 25, 输出: 97, 缓存创建: 0, 缓存读取: 0
  ```
- **Database Status Logic Fix**: Corrected `completeRequest` SQL to properly update status from any non-completed state to completed
- **Clean Architecture**: Removed redundant `RecordRequestUpdate` calls after `RecordRequestComplete` for better code maintainability

**Benefits**:
- **Easy Debugging**: Quickly identify all logs related to a specific request
- **Performance Analysis**: Track request duration from start to completion
- **Issue Resolution**: Trace failed requests through retry attempts and fallback logic
- **Request Correlation**: Connect client-side issues with server-side processing

## Request Status System (2025-09-09 Update)

**Enhanced Status Granularity**: The system now provides fine-grained request status tracking to eliminate user confusion and improve transparency in the Web interface.

### Status Lifecycle

The request status system uses a clear progression that accurately reflects the processing stages:

```
正常流程: pending → forwarding → processing → completed
重试流程: pending → forwarding → retry → retry → processing → completed  
跨端点:  pending → forwarding → retry(端点2) → processing → completed
挂起流程: pending → forwarding → suspended → forwarding → processing → completed
```

### Status Definitions

#### **Core Status States**
- **`forwarding`**: Request is being forwarded to endpoint  
- **`retry`**: Request is being retried (same endpoint or different endpoint) ⭐ **Enhanced**
- **`processing`**: HTTP response received successfully, Token parsing in progress ⭐ **New**
- **`completed`**: Token parsing and cost calculation fully completed ⭐ **New** 
- **`suspended`**: Request temporarily suspended waiting for group recovery ⭐ **New**
- **`error`**: Request failed at any stage
- **`timeout`**: Request timed out

#### **Status Update Triggers**
1. **`pending`** → Set at request start (RecordRequestStart)
2. **`forwarding`** → Set when first attempting an endpoint
3. **`retry`** → Set when retrying same endpoint or switching to new endpoint ⭐ **Fixed**
4. **`suspended`** → Set when all groups fail, request waits for recovery
5. **`processing`** → Set when HTTP response returns successfully (internal/proxy/retry.go:198)
6. **`completed`** → Set when token parsing completes (internal/tracking/database.go:380)

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
- **⚙️ 解析中** (`processing`): Orange gradient with pulsing animation ⭐ **New**
- **✅ 完成** (`completed`): Green gradient ⭐ **New**
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

#### **Asynchronous Processing Benefits**
- **Non-blocking**: Status updates don't affect request forwarding performance
- **Real-time**: Web interface shows live status progression via SSE
- **Transparent**: Users understand exactly what stage their request is in

### Migration and Compatibility

- **Backward Compatible**: Existing `success` status still supported for historical data
- **Seamless Transition**: New requests automatically use enhanced status system
- **No Breaking Changes**: API endpoints remain unchanged

**Benefits**: Eliminates user confusion about "successful" requests with zero tokens, provides clear processing transparency, improves debugging capabilities, and enhances overall user experience in the Web interface.

### Non-Token Response Handling (2025-09-07 Update)

**Enhanced Compatibility**: The system now provides intelligent fallback mechanisms for responses that don't contain token usage information, ensuring complete request lifecycle tracking for all API calls.

#### **Problem Solved**
Previously, requests that returned successful HTTP responses (200 OK) but contained no token information would remain indefinitely in `processing` status, causing user confusion in the Web interface.

#### **Common Non-Token Response Types**
- **Health Check Requests**: `/v1/models` endpoint returning model lists
- **Third-party APIs**: Non-Claude compatible endpoints without usage tracking
- **Configuration Queries**: System configuration or status endpoints  
- **Error Responses**: Non-standard error formats without usage data

#### **Fallback Implementation** 
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

#### **Enhanced Logging**
- **Response Content**: Info-level logging shows complete response content for analysis
  ```
  📄 [响应内容] 端点: packycode, 状态码: 200, 长度: 156字节, 响应内容: {"data": [...]}
  ```
- **Non-Token Detection**: Clear identification of responses without token information
  ```  
  🎯 [无Token响应] 端点: packycode, 连接: req-abc123 - 响应不包含token信息，标记为完成
  ✅ [无Token完成] 连接: req-abc123 已标记为完成状态，模型: default
  ```

#### **Database Storage**
Non-token requests are properly stored with:
- **Status**: `completed` (no longer stuck in `processing`)
- **Model Name**: `default` for identification and filtering
- **Token Counts**: All set to 0 (accurate representation)
- **Total Cost**: $0.00 (no AI processing cost incurred)

#### **Web Interface Benefits**
- **Clear Status**: Shows "完成" instead of indefinite "解析中" 
- **Proper Filtering**: Non-token requests can be filtered by model "default"
- **Complete Tracking**: Full request lifecycle visibility for all API calls
- **Zero Confusion**: Users understand these requests completed successfully without token usage

**Technical Benefits**: Ensures robust request tracking for all response types, improves system reliability, provides complete audit trails, and maintains consistent user experience across different API endpoint types.

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

## Key Features to Understand

**TUI Interface**: Real-time monitoring with tabs for Overview, Endpoints, Connections, Logs, and Configuration

**Web Interface**: Modern web-based monitoring and management interface with the following features:
- **Real-time Dashboard**: Live monitoring with Server-Sent Events (SSE) for instant updates
- **Request Tracking**: Complete request lifecycle monitoring with detailed tracking page
- **Group Management**: Interactive group control with activate/pause/resume operations
- **Endpoint Monitoring**: Visual health status and performance metrics
- **Charts & Analytics**: Performance visualization with Chart.js integration
- **Data Export**: CSV/JSON export functionality for request data
- **Modular Architecture**: Modern JavaScript architecture with separated modules for maintainability
- **Responsive Design**: Mobile-friendly interface with modern CSS styling
- **API Integration**: RESTful API for all management operations

**Streaming Support**: Automatic SSE detection and real-time streaming with proper event handling

**Proxy Support**: HTTP/HTTPS/SOCKS5 proxy configuration for all outbound requests

**Security**: Bearer token authentication with automatic header stripping and token injection. API key support with X-Api-Key header injection.

**Health Monitoring**: Continuous endpoint health checking with `/v1/models` endpoint testing

**Advanced Group Management**: 
- **Configurable Behavior**: Auto/manual group switching via `group.auto_switch_between_groups`
- **Web-based Control**: Full group lifecycle management through web interface
- **Real-time Updates**: Live status updates via SSE for group state changes
- **Intelligent Failover**: Priority-based routing with cooldown periods
- **Manual Intervention**: Ability to override automatic behavior when needed

## Web API Reference

The application provides a comprehensive REST API for monitoring and management:

### Group Management API
```bash
# Get all groups status
GET /api/v1/groups

# Manually activate a group
POST /api/v1/groups/{name}/activate

# Pause a group (manual intervention)
POST /api/v1/groups/{name}/pause

# Resume a paused group
POST /api/v1/groups/{name}/resume
```

### Monitoring API
```bash
# Get system status
GET /api/v1/status

# Get endpoints status  
GET /api/v1/endpoints

# Get connection statistics
GET /api/v1/connections

# Real-time updates via Server-Sent Events
GET /api/v1/stream?client_id={id}&events=status,endpoint,group,connection,log,chart
```

### Usage Tracking API
```bash
# Get usage statistics with filtering
GET /api/v1/usage/stats?start_date=2025-01-01&end_date=2025-12-31&model=claude-3-5-haiku&endpoint=instcopilot-sg

# Get detailed request logs
GET /api/v1/usage/requests?limit=100&offset=0&model=claude-sonnet-4&status=success

# Export usage data
GET /api/v1/usage/export?format=csv&start_date=2025-09-01&end_date=2025-09-30
GET /api/v1/usage/export?format=json&model=claude-3-5-haiku

# Get database health and statistics
GET /api/v1/usage/health
```

### Authentication
All API requests require Bearer token authentication:
```bash
curl -H "Authorization: Bearer your-token-here" http://localhost:8010/api/v1/groups
```

## Development Architecture

### File Structure (2025-09-11 Update)
```
internal/
├── web/
│   ├── server.go          # Web server setup and routing
│   ├── handlers.go        # Main handler documentation (16 lines)
│   ├── basic_handlers.go  # Basic API handlers (233 lines)
│   ├── sse_handlers.go    # Server-Sent Events handlers (249 lines)
│   ├── broadcast_handlers.go # Event broadcasting (62 lines)
│   ├── metrics_handlers.go   # Performance metrics (79 lines)
│   ├── chart_handlers.go     # Data visualization charts (173 lines)
│   ├── group_handlers.go     # Group management (140 lines)
│   ├── suspended_handlers.go # Request suspension handling (86 lines)
│   ├── usage_handlers.go     # Usage tracking API (183 lines)
│   ├── utils.go             # Utility functions (50 lines)
│   ├── templates.go         # HTML templates (1272 lines)
│   ├── events.go            # Server-Sent Events implementation
│   ├── usage_api.go         # Usage tracking API endpoints
│   └── static/
│       ├── css/style.css    # Web interface styling
│       └── js/              # Modularized JavaScript architecture
│           ├── utils.js           # Utility functions and formatters
│           ├── sseManager.js      # SSE connection management
│           ├── requestsManager.js # Request tracking functionality
│           ├── groupsManager.js   # Group management operations
│           ├── endpointsManager.js # Endpoint management
│           ├── webInterface.js    # Core Web interface class
│           └── charts.js          # Chart.js integration
├── endpoint/
│   ├── manager.go         # Endpoint and group management
│   └── group_manager.go   # Advanced group operations
├── tracking/
│   ├── tracker.go         # Main usage tracker with async operations
│   ├── database.go        # Database operations and schema management
│   ├── queries.go         # Query methods and data retrieval
│   ├── error_handler.go   # Error handling and recovery
│   └── schema.sql         # Database schema with timezone fixes
└── proxy/
    ├── handler.go         # Main request handler with unified architecture
    ├── retry.go           # Configurable retry logic with group switching
    ├── token_parser.go    # SSE token parsing and model detection
    ├── stream_processor.go # Advanced streaming processor v2 ⭐ NEW
    ├── error_recovery.go  # Intelligent error classification and recovery ⭐ NEW 
    ├── lifecycle_manager.go # Complete request lifecycle management ⭐ NEW
    └── [15+ test files]   # Comprehensive test coverage
```

### New Architecture Components (2025-09-11)

#### **Stream Processor v2** (`stream_processor.go`)
- **528 lines**: Advanced streaming request processor
- **Features**: Cancellation support, error recovery, token integration
- **Performance**: Optimized buffering and real-time processing
- **Integration**: Seamless usage tracking and lifecycle management

#### **Error Recovery System** (`error_recovery.go`) 
- **475 lines**: Intelligent error classification and recovery
- **Categories**: Network errors, API errors, streaming errors, unknown errors
- **Recovery**: Context-aware recovery strategies and user feedback
- **Monitoring**: Comprehensive error tracking and reporting

#### **Lifecycle Management** (`lifecycle_manager.go`)
- **279 lines**: Complete request state tracking
- **Features**: Status transitions, duration monitoring, endpoint tracking  
- **Integration**: Central coordination hub for all request components
- **Analytics**: Detailed timing and performance metrics

#### **Enhanced Handler Architecture** 
- **handler.go**: Expanded from basic forwarding to unified architecture (695 lines added)
- **Dual Processing**: Automatic detection and routing for streaming vs regular requests
- **Architecture Logging**: Clear identification of processing type in logs
- **Integration**: Seamless coordination with all v2 components

### Code Architecture Refactoring (2025-09-05)

**Major Web Handler Refactoring**: The monolithic `handlers.go` file (2475 lines) has been successfully refactored into a modular architecture with 11 specialized files, each following the single responsibility principle:

1. **Modular Design**: Each handler file focuses on specific functionality (basic API, SSE, metrics, charts, groups, etc.)
2. **Maintainability**: Individual files are 16-250 lines, making them easier to understand and modify
3. **Team Collaboration**: Different developers can work on separate modules simultaneously without conflicts
4. **Code Quality**: Clear separation of concerns with utilities, templates, and specific handlers
5. **Scalability**: New features can be added to appropriate modules without affecting others

**Refactoring Benefits**:
- **Before**: Single 2475-line file with mixed responsibilities
- **After**: 11 focused files totaling ~4105 lines with clear module boundaries
- **Quality**: 100% functionality preservation with improved code organization
- **Performance**: Better compilation times and reduced cognitive load

### Important Implementation Notes

1. **HTML Templates**: Web interface HTML is now in dedicated `templates.go` file, requiring recompilation for changes
2. **Static Assets**: CSS and JS files are served from the filesystem and can be modified without recompilation
3. **SSE Integration**: Real-time updates use Server-Sent Events for efficient push notifications, handled by `sse_handlers.go`
4. **Group State Management**: Thread-safe group operations with proper locking mechanisms in `group_handlers.go`
5. **Configuration Hot-Reload**: File system monitoring with debounced updates (500ms delay)
6. **Usage Tracking**: Fully asynchronous database operations with proper timezone handling (CST +08:00) in `usage_handlers.go`
7. **Modular Architecture**: Both Go backend and JavaScript frontend use modular design for better maintainability and team collaboration

### JavaScript Module Architecture

**Modern Frontend Design** (2025-09-05 Update):
The Web interface now uses a modular JavaScript architecture for enhanced maintainability:

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

## Usage Tracking System

### Complete Request Lifecycle Tracking

**Request Tracking Interface** (2025-09-05 Update):
The Web interface now includes a comprehensive request tracking page that replaces the simple logs view:

**Features**:
- **Multi-dimensional Filtering**: Filter by date range, status, model, endpoint, group
- **Real-time Updates**: Live request monitoring via SSE integration  
- **Detailed View**: Complete request lifecycle with timing, tokens, and cost information
- **Export Capabilities**: CSV/JSON export with flexible filtering options
- **Performance Analytics**: Statistical summaries and trends
- **Pagination Support**: Efficient browsing of large request datasets

**Usage Tracking APIs**:
```bash
# Get detailed request logs with filtering and pagination
GET /api/v1/usage/requests?limit=100&offset=0&status=success&model=claude-sonnet-4

# Get usage statistics and summaries
GET /api/v1/usage/stats?period=7d&start_date=2025-09-01&end_date=2025-09-05

# Export request data in multiple formats
GET /api/v1/usage/export?format=csv&model=claude-3-5-haiku&start_date=2025-09-01
```

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

**Event Architecture Clarification**:
- **`RecordRequestStart`**: Initial request state (`pending`)
- **`RecordRequestUpdate`**: All intermediate state changes (`forwarding`, `retry`, `processing`, etc.)
- **`RecordRequestComplete`**: Final completion with token data and cost calculation (`completed`)

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

### Troubleshooting

**Common Issues Resolved**:
1. **Timezone Problems**: Use `datetime('now', 'localtime')` instead of `CURRENT_TIMESTAMP`
2. **Model Name Missing**: Ensure SSE streams contain `message_start` events
3. **High Costs**: Monitor cache token usage (cache_creation_tokens, cache_read_tokens)
4. **Performance Impact**: All tracking operations are fully asynchronous and non-blocking
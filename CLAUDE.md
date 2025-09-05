# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Request Forwarder is a high-performance Go application that transparently forwards Claude API requests to multiple endpoints with intelligent routing, health checking, and automatic retry/fallback capabilities. It includes both a Terminal User Interface (TUI) and Web Interface for real-time monitoring and management.

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

### Key Design Patterns

**Strategy Pattern**: Endpoint selection via "priority" or "fastest" strategies with optional pre-request fast testing

**Middleware Chain**: Request processing through authentication, logging, and monitoring layers

**Observer Pattern**: Configuration hot-reloading with callback-based component updates

**Circuit Breaker Pattern**: Health checking with automatic endpoint marking as healthy/unhealthy

### Request Flow

1. Request reception with middleware chain (auth → logging → monitoring)
2. Endpoint selection based on strategy and health status
3. Header transformation (strip client auth, inject endpoint tokens and API keys)
4. Request forwarding with timeout and retry handling
5. Response streaming (SSE) or buffered response handling
6. Error handling with automatic endpoint fallback

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

## Testing Approach

The codebase includes comprehensive unit tests:
- `*_test.go` files in each package
- Test configuration in `test_config.yaml`
- Health check testing with mock endpoints
- Fast tester functionality testing
- Proxy handler testing with various scenarios

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

**Token Parser and Model Detection**:
- **Dual Event Processing**: TokenParser simultaneously processes `message_start` and `message_delta` events from Claude API SSE streams
- **Model Name Extraction**: Automatically extracts Claude model information (e.g., `claude-3-haiku-20240307`) from `message_start` events
- **Integrated Logging**: Model information is seamlessly integrated into token usage logs:
  ```
  🪙 [Token Parser] [req-xxxxxxxx] 从SSE流中提取令牌使用情况 - 模型: claude-3-haiku-20240307, 输入: 25, 输出: 10, 缓存创建: 0, 缓存读取: 0
  ```
- **Safe Implementation**: Model extraction does not affect data forwarding or client responses, operates as a pure monitoring feature
- **Backward Compatibility**: Gracefully handles cases where model information is not available, falling back to standard token logging

**Benefits**:
- **Easy Debugging**: Quickly identify all logs related to a specific request
- **Performance Analysis**: Track request duration from start to completion
- **Issue Resolution**: Trace failed requests through retry attempts and fallback logic
- **Request Correlation**: Connect client-side issues with server-side processing

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
- **Group Management**: Interactive group control with activate/pause/resume operations
- **Endpoint Monitoring**: Visual health status and performance metrics
- **Charts & Analytics**: Performance visualization with Chart.js integration
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

### File Structure
```
internal/
├── web/
│   ├── server.go          # Web server setup and routing
│   ├── handlers.go        # HTTP handlers and HTML templates
│   ├── events.go          # Server-Sent Events implementation
│   ├── usage_api.go       # Usage tracking API endpoints
│   └── static/
│       ├── css/style.css  # Web interface styling
│       └── js/
│           ├── app.js     # Main JavaScript functionality
│           └── charts.js  # Chart.js integration
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
    ├── retry.go           # Configurable retry logic with group switching
    └── token_parser.go    # SSE token parsing and model detection
```

### Important Implementation Notes

1. **HTML Templates**: Web interface HTML is embedded in Go code (`handlers.go`), requiring recompilation for changes
2. **Static Assets**: CSS and JS files are served from the filesystem and can be modified without recompilation
3. **SSE Integration**: Real-time updates use Server-Sent Events for efficient push notifications
4. **Group State Management**: Thread-safe group operations with proper locking mechanisms
5. **Configuration Hot-Reload**: File system monitoring with debounced updates (500ms delay)
6. **Usage Tracking**: Fully asynchronous database operations with proper timezone handling (CST +08:00)

## Usage Tracking System

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
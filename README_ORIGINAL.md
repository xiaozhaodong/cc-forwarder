# Claude Code Request Forwarder

A high-performance Go application that transparently forwards Claude Code API requests to multiple endpoints with intelligent routing, health checking, and automatic retry/fallback capabilities.

[中文文档](README_CN.md) | English

## Features

- **Transparent Proxying**: Forward all HTTP requests transparently to backend endpoints
- **SSE Streaming Support**: Full support for Server-Sent Events streaming
- **Token Management**: Override or add Authorization Bearer tokens per endpoint  
- **Routing Strategies**: Priority-based or fastest-response routing
- **Health Checking**: Automatic endpoint health monitoring
- **Retry & Fallback**: Exponential backoff with automatic endpoint fallback
- **Group Management**: Intelligent endpoint grouping with automatic failover and cooldown periods
- **Monitoring**: Built-in health checks and Prometheus-style metrics
- **Structured Logging**: Configurable JSON or text logging with multiple levels
- **TUI Interface**: Built-in Terminal User Interface for real-time monitoring with interactive priority editing (enabled by default)
- **Dynamic Priority Override**: Runtime endpoint priority adjustment via `-p` parameter for testing and failover scenarios

## Quick Start

1. **Build the application**:
   ```bash
   go build -o endpoint_forwarder
   ```

2. **Copy and configure the example config**:
   ```bash
   cp config/example.yaml config/config.yaml
   # Edit config.yaml with your endpoints and tokens
   ```

3. **Run the forwarder**:
   ```bash
   # Default mode with TUI interface
   ./endpoint_forwarder -config config/config.yaml
   
   # Run without TUI (traditional console mode)
   ./endpoint_forwarder -config config/config.yaml --no-tui
   
   # Explicitly enable TUI (default behavior)
   ./endpoint_forwarder -config config/config.yaml --tui
   
   # Override endpoint priority at runtime (useful for testing or failover)
   ./endpoint_forwarder -config config/config.yaml -p "endpoint-name"
   ```

4. **Configure Claude Code**:
   Set in Claude Code's `settings.json`:
   ```json
   {
     "ANTHROPIC_BASE_URL": "http://localhost:8080"
   }
   ```

## Configuration

### Server Configuration
```yaml
server:
  host: "0.0.0.0"  # Server bind address
  port: 8080        # Server port
```

### Routing Strategy
```yaml
strategy:
  type: "priority"  # "priority" or "fastest"
```

- **priority**: Use endpoints in priority order (lower number = higher priority)
- **fastest**: Use endpoint with lowest response time

### Retry Configuration
```yaml
retry:
  max_attempts: 3      # Maximum retry attempts per endpoint
  base_delay: "1s"     # Initial delay between retries
  max_delay: "30s"     # Maximum delay cap
  multiplier: 2.0      # Exponential backoff multiplier
```

### Health Check Configuration
```yaml
health:
  check_interval: "30s"     # How often to check endpoint health
  timeout: "5s"             # Health check timeout
  health_path: "/v1/models" # Health check endpoint path
```

### Group Management Configuration
```yaml
group:
  cooldown: "600s"           # Group cooldown duration when all endpoints fail (default: 10 minutes)
```

The system supports intelligent endpoint grouping with automatic failover and cooldown mechanisms, plus dynamic key resolution:

**Group Configuration Features:**
- **Priority-based Groups**: Groups have priorities (lower number = higher priority)
- **Automatic Failover**: When all endpoints in a group fail, system switches to next priority group
- **Cooldown Periods**: Failed groups enter cooldown mode before being reconsidered
- **Inheritance**: Endpoints inherit group settings from previous endpoints
- **Single Active Group**: Only one group is active at a time for deterministic routing
- **Dynamic Key Resolution**: Keys are resolved dynamically at runtime for group-level key sharing

**Group Behavior:**
- **Active Group Selection**: Highest priority group not in cooldown becomes active
- **Cooldown Trigger**: When all endpoints in a group fail, the group enters cooldown
- **Automatic Recovery**: Groups automatically reactivate after cooldown period expires
- **Priority-based Routing**: Requests only go to endpoints in the active group

**Dynamic Key Resolution Mechanism:**
- **Runtime Resolution**: Keys are not inherited during config parsing but resolved dynamically at request time
- **Group-level Sharing**: All endpoints in a group share the token/api-key from the first endpoint that defines it
- **Override Support**: Individual endpoints can override group keys by explicitly specifying their own `token` or `api-key`
- **Failover-friendly**: When groups switch during failover, the new active group's keys are automatically used

**Group Configuration Example:**
```yaml
endpoints:
  # Primary group (highest priority) - defines group keys
  - name: "primary"
    url: "https://api.openai.com"
    group: "main"           # Group name
    group-priority: 1       # Group priority (1 = highest)
    priority: 1             # Priority within group
    token: "sk-main-group-token"      # 🔑 Main group key, shared by other endpoints in group
    api-key: "main-api-key"           # 🔑 Main group API key, shared by other endpoints in group
    
  # Backup endpoint in primary group - uses main group keys
  - name: "primary_backup"
    url: "https://api.anthropic.com"
    priority: 2
    # 🔄 Inherits group: "main" and group-priority: 1
    # 🔑 Dynamically uses main group keys: token and api-key resolved at runtime from primary endpoint
    
  # Secondary group (lower priority) - defines different group keys
  - name: "secondary"
    url: "https://api.example.com"
    group: "backup"         # Different group
    group-priority: 2       # Lower priority
    priority: 1
    token: "sk-backup-group-token"    # 🔑 Backup group key, shared by other endpoints in group
    api-key: "backup-api-key"         # 🔑 Backup group API key
    
  # Custom override within backup group
  - name: "secondary_special"
    url: "https://api.special.com"
    priority: 2
    token: "sk-custom-override"       # 🔑 Overrides backup group key, only this endpoint uses this
    # 🔄 Still belongs to backup group
    # 🔑 api-key still uses group default
    
  # Tertiary group (lowest priority)
  - name: "local"
    url: "http://localhost:11434"
    group: "local"
    group-priority: 3       # Lowest priority
    priority: 1
    # 🔓 No token needed for local service
```

**Group Inheritance Rules:**
- **Group Settings**: Endpoints inherit `group` and `group-priority` from previous endpoints if not specified
- **Static Inheritance**: `timeout` and `headers` are inherited during configuration parsing
- **Dynamic Resolution**: `token` and `api-key` are not inherited during config parsing but resolved at runtime
- **Group Priority**: Group-level key sharing works independently of configuration inheritance

**Key Configuration Best Practices:**
- First endpoint in each group should define the token and api-key for that group
- Other endpoints in the group don't need to repeat key configuration, they'll automatically share group keys
- If an endpoint needs a special key, explicitly specify token/api-key to override group defaults
- Local services typically don't require key configuration

**Use Cases:**
- **High Availability**: Primary/backup group setup for critical services
- **Cost Optimization**: Use different providers based on priority (e.g., GPT-4 → Claude → Local)
- **Geographic Routing**: Group endpoints by region with automatic failover
- **Load Balancing**: Distribute load across multiple groups with different priorities

### Global Timeout Configuration
```yaml
global_timeout: "300s"      # Default timeout for all non-streaming requests (5 minutes)
```

**Usage:**
- Sets the default timeout for all endpoints that don't specify their own `timeout`
- Only applies to non-streaming requests
- Can be overridden by individual endpoint `timeout` settings

### Authentication Configuration
```yaml
auth:
  enabled: false                    # Enable Bearer token authentication (default: false)
  token: "your-bearer-token"        # Bearer token for authentication (required when enabled)
```

### TUI Interface Configuration
```yaml
tui:
  enabled: true                     # Enable TUI interface (default: true)
  update_interval: "1s"             # TUI refresh interval (default: 1s)
```

**TUI Features:**
- **Real-time Monitoring**: Live request metrics, response times, and success rates
- **Multi-tab Interface**: Overview, Endpoints, Connections, Logs, and Configuration tabs
- **Interactive Navigation**: Tab/Shift+Tab to switch tabs, 1-5 for direct access
- **Color-coded Status**: Green=Healthy, Yellow=Warning, Red=Error
- **Live Connection Tracking**: Monitor active connections and traffic
- **Real-time Logs**: Real-time System logs

**TUI Controls:**
- `Tab/Shift+Tab`: Navigate between tabs
- `1-5`: Jump directly to tab (1=Overview, 2=Endpoints, etc.)
- `Ctrl+C`: Quit application
- `Arrow Keys`: Navigate within views

**Priority Editing (Endpoints Tab):**
- `Enter`: Enter priority edit mode for real-time priority adjustment
- `ESC`: Exit edit mode without saving changes
- `Ctrl+S`: Save priority changes to configuration
- `1-9`: Set priority for selected endpoint (in edit mode)
- Visual indicators show current edit state and unsaved changes

**Usage:**
- When `enabled: false` (default): No authentication is required, requests pass through directly
- When `enabled: true`: All requests must include `Authorization: Bearer <token>` header
- The token in the header must exactly match the configured token
- Returns HTTP 401 Unauthorized for missing, malformed, or invalid tokens
- Only applies to the main proxy endpoints (health check endpoints remain open)

**Health Check Behavior:**
- **Endpoint**: Tests the `/v1/models` endpoint (suitable for Claude API)
- **Success Criteria**: Accepts both 2xx (success) and 4xx (client error) status codes
  - 2xx responses indicate the endpoint is working correctly
  - 4xx responses (401, 403, etc.) indicate the endpoint is reachable but may need proper authentication
- **Failure Criteria**: 5xx server errors indicate endpoint problems
- **Strategy Logging**: For "fastest" strategy, logs endpoint latencies before each selection

### Endpoint Configuration
```yaml
endpoints:
  - name: "primary"
    url: "https://api.anthropic.com"
    priority: 1
    timeout: "30s"
    token: "sk-ant-your-token-here"  # Optional: Override/add auth token
    headers:                         # Optional: Additional headers
      X-Custom-Header: "value"
```

#### Parameter Inheritance & Dynamic Key Resolution
For convenience, the system supports two mechanisms:

**Static Inheritance (Configuration Stage):**
Subsequent endpoints can inherit the following parameters from the first endpoint:
- `timeout`: Request timeout duration (defaults to `global_timeout` if not specified)
- `headers`: HTTP headers (with smart merging)

**Dynamic Resolution (Runtime):**
Key-related parameters are resolved dynamically at runtime:
- `token`: Retrieved from the first endpoint in the same group that defines a token
- `api-key`: Retrieved from the first endpoint in the same group that defines an api-key

```yaml
endpoints:
  # Main group - defines group keys and inheritable parameters
  - name: "primary"
    url: "https://api.anthropic.com"
    group: "main"
    group-priority: 1
    priority: 1
    timeout: "45s"                    # ⏱️ Will be statically inherited
    token: "sk-main-group-token"      # 🔑 Dynamic resolution: shared within group
    api-key: "main-api-key"           # 🔑 Dynamic resolution: shared within group
    headers:                          # 📋 Will be statically inherited & merged
      Authorization-Fallback: "Bearer fallback"
      X-API-Version: "v1"
      User-Agent: "Claude-Forwarder/1.0"
    
  # Main group backup endpoint - inheritance + dynamic resolution
  - name: "secondary"
    url: "https://backup.anthropic.com" 
    priority: 2
    # 🔄 Group settings inherited: group="main", group-priority=1
    # ⏱️ Static inheritance: timeout=45s
    # 📋 Static inheritance: all headers
    # 🔑 Dynamic resolution: token and api-key resolved at runtime from primary
    headers:
      X-Custom-Header: "secondary"    # 🔄 Merged with inherited headers
    
  # Backup group - new group key definition
  - name: "backup"
    url: "https://api.backup.com"
    group: "backup"                   # New group
    group-priority: 2
    priority: 1
    timeout: "30s"                    # 🚫 Overrides static inheritance
    token: "sk-backup-group-token"    # 🔑 New group key definition
    # ✅ Still inherits headers from primary (static)
    
  # Backup group custom endpoint
  - name: "backup_custom"
    url: "https://api.custom.com"
    priority: 2
    token: "sk-custom-override"       # 🔑 Overrides group default key
    # 🔄 Group settings inherited: group="backup", group-priority=2
    # ⏱️ Static inheritance: timeout=45s (from primary)
    # 📋 Static inheritance: headers (from primary)
    # 🔑 Dynamic resolution: api-key still from backup endpoint
    
  # Minimal configuration endpoint
  - name: "minimal"
    url: "https://minimal.anthropic.com"
    priority: 3
    # ✅ Static inheritance from primary: timeout, headers
    # 🔑 Dynamic resolution: keys from backup group
```

**Header Merging Rules:**
- If no headers specified → inherit all headers from first endpoint
- If headers specified → merge with first endpoint's headers (your headers override)
- Headers with same key → your value takes precedence

**Key Resolution Rules:**
- Endpoint's own key takes priority: If endpoint defines token/api-key, use it directly
- Group sharing: If endpoint doesn't define it, get from first endpoint in same group that has the key
- No key: If no endpoint in group has the key, don't set it (suitable for local services)

### Proxy Configuration
```yaml
proxy:
  enabled: true
  type: "http"  # "http", "https", or "socks5"
  
  # Option 1: Complete proxy URL
  url: "http://proxy.example.com:8080"
  # url: "socks5://proxy.example.com:1080"
  
  # Option 2: Host and port (alternative to URL)
  host: "proxy.example.com"
  port: 8080
  
  # Optional authentication
  username: "proxy_user"
  password: "proxy_pass"
```

**Proxy Support:**
- **HTTP/HTTPS Proxy**: Standard HTTP proxy with optional authentication
- **SOCKS5 Proxy**: Full SOCKS5 support with optional authentication  
- **Flexible Configuration**: Use complete URL or separate host:port
- **Security**: Proxy credentials are handled securely
- **Performance**: Optimized transport layer for all proxy types

**Usage Notes:**
- All outbound requests (health checks, fast tests, and API calls) use the configured proxy
- Proxy settings apply globally to all endpoints
- For corporate environments, ensure proxy allows HTTPS CONNECT method
- SOCKS5 proxies provide better performance for high-throughput scenarios

## Monitoring Endpoints

The forwarder provides several monitoring endpoints:

- **GET /health**: Basic health check
- **GET /health/detailed**: Detailed health information for all endpoints  
- **GET /metrics**: Prometheus-style metrics

### Example Health Check Response
```json
{
  "status": "healthy",
  "healthy_endpoints": 2,
  "total_endpoints": 3
}
```

## Usage Examples

### Basic Request Forwarding
```bash
# Regular API request - will be forwarded to the best available endpoint
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-3-sonnet-20240229", "max_tokens": 100, "messages": [{"role": "user", "content": "Hello"}]}'
```

### SSE Streaming
```bash
# Streaming request - automatically detected and handled
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{"model": "claude-3-sonnet-20240229", "max_tokens": 100, "messages": [{"role": "user", "content": "Count to 10"}], "stream": true}'
```

### Health Monitoring
```bash
# Check overall health
curl http://localhost:8080/health

# Get detailed endpoint status
curl http://localhost:8080/health/detailed

# Get Prometheus metrics
curl http://localhost:8080/metrics
```

## How It Works

1. **Request Reception**: The forwarder receives HTTP requests on the configured port
2. **Group Selection**: Based on group priorities and cooldown status, selects the active group
3. **Endpoint Selection**: Within the active group, selects the best available endpoint based on the configured strategy (priority/fastest)
4. **Request Forwarding**: Transparently forwards the request with proper header handling:
   - **Host Header**: Automatically set to match the target endpoint's hostname
   - **Authorization**: Override/inject tokens as configured, remove client tokens
   - **Security**: Automatically strips sensitive client headers (`X-API-Key`, `Authorization`)
   - **Custom Headers**: Add endpoint-specific headers as configured
   - **Original Headers**: Preserve all other headers from the original request
5. **Response Handling**: 
   - Regular requests: Buffers and forwards the complete response
   - SSE requests: Streams response in real-time with proper event handling
6. **Error Handling**: On failure, automatically retries with exponential backoff, then falls back to the next available endpoint within the active group
7. **Group Management**: If all endpoints in the active group fail, the group enters cooldown and system switches to the next priority group
8. **Health Monitoring**: Continuously monitors endpoint health and adjusts routing accordingly

## Command Line Options

```bash
./endpoint_forwarder [OPTIONS]
```

Options:
- `-config path/to/config.yaml`: Path to configuration file (default: "config/example.yaml")
- `-version`: Show version information
- `-tui`: Enable TUI interface (default: true)
- `-no-tui`: Disable TUI interface (run in traditional console mode)
- `-p "endpoint-name"`: Override endpoint priority (set specified endpoint as primary with priority 1)

Examples:
```bash
# Default mode with TUI
./endpoint_forwarder -config my-config.yaml

# Run without TUI (traditional console logging)
./endpoint_forwarder -config my-config.yaml -no-tui

# Show version information
./endpoint_forwarder -version

# Override endpoint priority (useful for testing specific endpoints)
./endpoint_forwarder -config my-config.yaml -p "backup-endpoint"

# Combine options: run without TUI and override priority
./endpoint_forwarder -config my-config.yaml -no-tui -p "test-endpoint"
```

## Logging

The application uses structured logging with enhanced formatting for better human readability:

```yaml
logging:
  level: "info"    # debug, info, warn, error
  format: "text"   # text (human-readable) or json (machine-readable)
```

### Log Features

**Enhanced Readability:**
- 🎯 Emoji indicators for different log types and statuses
- 📊 Formatted response times (μs/ms/s) and data sizes (B/KB/MB)  
- 🚀 Request lifecycle tracking with endpoint information
- ⏱️  Precise timestamp formatting (HH:MM:SS.mmm)

**Request Logging:**
- Request start with selected endpoint name
- Response completion with status indicators
- Error tracking with appropriate severity levels
- Performance monitoring (slow request detection)

**Log Examples:**
```
15:04:05.123 level=INFO msg="🚀 Request started" method=POST path=/v1/messages client_ip=192.168.1.100 user_agent="Claude-Client/1.0" content_length=245
15:04:05.456 level=INFO msg="🎯 Selected endpoint" endpoint=primary url=https://api.anthropic.com priority=1 attempt=1 total_endpoints=3  
15:04:06.789 level=INFO msg="✅ Request completed" method=POST path=/v1/messages endpoint=primary status_code=200 bytes_written=1.2KB duration=633.2ms client_ip=192.168.1.100
```

**Security Features:**
- Automatically removes sensitive client headers (`X-API-Key`, `Authorization`) 
- Replaces with endpoint-configured tokens
- Prevents credential leakage between client and backend

## Production Considerations

- Configure appropriate timeouts for your use case
- Monitor the `/health` and `/metrics` endpoints
- Use a reverse proxy (nginx/Apache) for SSL termination
- Configure log rotation for production deployments
- Set up alerts based on endpoint health metrics
- Consider rate limiting at the reverse proxy level

## License

This project is provided as-is for educational and development purposes.
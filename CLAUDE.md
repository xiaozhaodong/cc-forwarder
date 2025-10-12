# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Version Information

**Current Version**: v3.5.0 (2025-10-12)
**Major Update**: 状态机架构重构 - 业务状态与错误状态分离

## Project Overview

Claude Request Forwarder is a high-performance Go application that transparently forwards Claude API requests to multiple endpoints with intelligent routing, health checking, and automatic retry/fallback capabilities.

**Key Features v3.5.0**:
- **状态机架构重构**: 双轨状态管理（业务状态+错误状态分离），前端可同时显示业务进度和错误原因
- **MySQL数据库支持**: 适配器模式实现SQLite/MySQL多数据库兼容，支持连接池管理
- **/v1/messages/count_tokens端点**: 完整支持Token计数端点，智能转发与降级估算
- **端点自愈机制**: 从5分钟冷却优化到0.7秒快速恢复，智能健康检查
- **流式Token修复**: FlushPendingEvent机制解决SSE终止空行缺失
- **Cloudflare错误码**: 520-525错误码智能处理与重试策略
- **前端UI升级**: 状态显示优化、HTTP状态码展示、交互体验增强
- **响应格式检测**: 修复JSON误判bug，精确格式识别
- **Token调试工具**: 可配置开关，支持debug数据采集
- **400错误码重试**: 归类为限流错误，享受与429相同重试策略
- **Modular Architecture**: Complete handler.go refactoring with single responsibility principle
- **Dual Processing**: Streaming v2 and Unified v2 request processing
- **Intelligent Error Recovery**: Smart error classification and recovery strategies
- **Complete Lifecycle Tracking**: End-to-end request monitoring and analytics
- **Advanced Streaming**: Real-time SSE processing with cancellation support
- **Comprehensive Testing**: 30+ test files with extensive coverage

## Quick Start

```bash
# Build the application
go build -o cc-forwarder

# Run with default configuration
./cc-forwarder -config config/config.yaml

# Run tests
go test ./...

# Check version
./cc-forwarder -version
```

## Core Architecture

### Main Components
- **`internal/proxy/`**: Modular request forwarding with v3.5 state machine architecture
  - `handler.go`: Core HTTP request coordinator (~430 lines)
  - **`handlers/`**: Specialized request processing modules
    - `count_tokens.go`: Count Tokens endpoint handler (+188 lines) ⭐ NEW
    - `streaming.go`: Streaming request handler (~500 lines) ⚡ ENHANCED
    - `regular.go`: Regular request handler (~480 lines) ⚡ ENHANCED
    - `forwarder.go`: HTTP request forwarder (~144 lines)
    - `interfaces.go`: Component interfaces (~115 lines)
  - **`response/`**: Response processing modules
    - `processor.go`: Response processing and decompression (~270 lines) ⚡ ENHANCED
    - `analyzer.go`: Token analysis and parsing (~745 lines) ⚡ ENHANCED
    - `utils.go`: Response utility functions (~21 lines)
    - `format_detection_test.go`: Format detection tests (+342 lines) ⭐ NEW
    - `processor_stream_test.go`: Stream processing tests (+306 lines) ⭐ NEW
    - `processor_unified_test.go`: Unified processing tests (+128 lines) ⭐ NEW
  - `stream_processor.go`: Advanced streaming processor v2 with FlushPendingEvent
  - `error_recovery.go`: Intelligent error handling with Cloudflare support
  - `lifecycle_manager.go`: State machine lifecycle tracking (~730 lines) ⚡ MAJOR REFACTOR
  - `endpoint_recovery_manager.go`: Endpoint self-healing (+119 lines) ⭐ NEW
  - `suspension_manager.go`: Request suspension management (refactored)
- **`internal/tracking/`**: Usage tracking with database abstraction ⚡ MAJOR REFACTOR
  - `database_adapter.go`: Database adapter interface (+144 lines) ⭐ NEW
  - `mysql_adapter.go`: MySQL implementation (+602 lines) ⭐ NEW
  - `sqlite_adapter.go`: SQLite implementation (+337 lines) ⭐ NEW
  - `tracker.go`: Event-driven usage tracker (~1000 lines) ⚡ ENHANCED
  - `database.go`: Database operations (~1200 lines) ⚡ ENHANCED
  - `queries.go`: Query interface with state machine support ⚡ ENHANCED
- **`internal/endpoint/`**: Endpoint management and health checking
- **`internal/web/`**: Web interface with real-time monitoring
- **`internal/utils/`**: Utility modules
  - `debug.go`: Token debugging tools (+237 lines) ⭐ NEW
- **`config/`**: Configuration management with hot-reloading

### Request Flow v2.1
```
1. Request Reception → Architecture Detection → Lifecycle Init
2. Handler Coordination → Specialized Processing (Streaming/Regular)
3. Response Analysis → Token Extraction → Client Delivery
4. Error Recovery → Retry Logic → Status Tracking
```

### Status Lifecycle
```
正常流程: pending → forwarding → processing → completed
流式流程: pending → forwarding → streaming → processing → completed
重试流程: pending → forwarding → retry → processing → completed
错误恢复: pending → forwarding → error_recovery → retry → completed
```

## Configuration Essentials

**Primary config**: `config/config.yaml` (copy from `config/example.yaml`)

**Key Settings**:
```yaml
# Web Interface (recommended for production)
web:
  enabled: true
  host: "0.0.0.0"
  port: 8010

# Group Management
group:
  cooldown: "600s"
  auto_switch_between_groups: true  # Auto failover

# Request Suspension
request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: 100
```

### Group Configuration Example
```yaml
endpoints:
  # Primary group (highest priority)
  - name: "primary"
    url: "https://api.openai.com"
    group: "main"
    group-priority: 1
    priority: 1
    token: "sk-main-group-token"        
    
  # Secondary group (lower priority)
  - name: "secondary"
    url: "https://api.example.com"
    group: "backup"
    group-priority: 2
    priority: 1
    token: "sk-backup-group-token"
```

## Development Commands

```bash
# Test specific modules
go test ./internal/proxy/...      # Proxy architecture tests
go test ./internal/endpoint/...   # Endpoint management tests
go test ./internal/tracking/...   # Usage tracking tests

# Integration tests
go test ./tests/...

# Performance tests
go test -bench=. ./internal/proxy/

# Run with race detection
go test -race ./...
```

## Testing Structure

**Unit Tests**: Co-located with source code (`*_test.go`)
- Access to internal functions and implementation details
- 20+ files covering core components

**Integration Tests**: `tests/integration/` directory
- End-to-end workflow testing
- 5 files covering system interactions

**Test Quality Metrics**:
- **Total Test Files**: 25+ comprehensive test files
- **Test Scenarios**: 200+ individual test cases
- **Coverage**: High coverage of critical paths and error conditions

## Key Design Patterns

- **Factory Pattern**: Request processor creation (streaming vs regular)
- **State Machine Pattern**: Request lifecycle management
- **Strategy Pattern**: Endpoint selection algorithms
- **Circuit Breaker Pattern**: Health checking and failover

## API Quick Reference

**Group Management**:
```bash
GET  /api/v1/groups                    # List all groups
POST /api/v1/groups/{name}/activate    # Activate group
POST /api/v1/groups/{name}/pause       # Pause group
```

**Monitoring**:
```bash
GET /api/v1/status                     # System status
GET /api/v1/endpoints                  # Endpoint status
GET /api/v1/stream                     # Real-time updates (SSE)
```

**Usage Tracking**:
```bash
GET /api/v1/usage/stats                # Usage statistics
GET /api/v1/usage/requests             # Request logs
GET /api/v1/usage/export               # Data export
```

## Architecture Logging

The system provides clear architecture identification in logs:
```
🌊 [流式架构] [req-xxxxxxxx] 使用streaming v2架构
🔄 [常规架构] [req-xxxxxxxx] 使用unified v2架构
```

## Request ID Tracking

**Request ID Generation**: The system generates unique short UUID-based request IDs in the format `req-xxxxxxxx` (8 hex characters) for every incoming request.

**Complete Lifecycle Tracking**: Each request can be traced through its entire lifecycle using the request ID:

```
🚀 Request started [req-4167c856]
🎯 [请求转发] [req-4167c856] 选择端点: instcopilot-sg (组: main, 总尝试 1)
✅ [请求成功] [req-4167c856] 端点: instcopilot-sg (组: main), 状态码: 200 (总尝试 1 个端点)
✅ Request completed [req-4167c856]
```

**Debugging**: Easy log filtering using `grep "req-xxxxxxxx" logfile` for complete request analysis.

## Troubleshooting

**Common Issues**:
1. **Configuration**: Ensure `config/config.yaml` exists
2. **Endpoint Health**: Check `/api/v1/endpoints` for status
3. **Group State**: Verify active groups in web interface
4. **Request Tracking**: Use request ID for log correlation
5. **Token Parsing**: Check for `message_start` events in SSE streams

## Documentation

For detailed technical information, see:
- **`docs/TECHNICAL_ARCHITECTURE.md`**: Complete component specifications, implementation details, and troubleshooting
- **Configuration Reference**: Full parameter documentation in example files
- **API Documentation**: Comprehensive endpoint reference in web interface

## Recent Updates

**2025-10-12**: Major v3.5.0 状态机架构重构版本 🚀
- **状态机重构**: 双轨状态管理，业务状态(pending/forwarding/processing/completed/failed/cancelled)与错误状态(retry/suspended)完全分离
- **MySQL支持**: 数据库适配器模式，支持SQLite和MySQL，连接池管理，时区支持
- **Count Tokens端点**: 实现/v1/messages/count_tokens完整支持，智能转发+本地降级估算
- **端点自愈**: 从5分钟冷却优化到0.7秒快速恢复，实时健康检查与自动恢复
- **新增测试**: 16个新测试文件，覆盖状态机、数据库适配器、响应格式检测、端点自愈等
- **数据库Schema**: 新增failure_reason、last_failure_reason、cancel_reason字段，支持错误溯源
- **前端优化**: 状态机兼容性，HTTP状态码显示，请求列表交互体验升级
- **代码质量**: 净增5,892行高质量代码，66个文件修改，架构更清晰

**2025-09-24**: Major v3.4.2 400错误码重试与统计优化
- 400错误码重试支持：将400错误码归类为限流错误，享受与429相同的重试策略
- 失败请求统计优化：StatsOverview替换挂起请求数为基于数据库的实际失败请求统计
- UI改进：图标⏸️ → ❌，样式warning → error，与前端徽章设计保持一致
- 错误分类优化：完善错误处理优先级，避免400错误被误归类为一般HTTP错误
- 前后端数据一致性：统一字段命名suspended_requests → failed_requests

**2025-09-24**: Major v3.4.1 Cloudflare错误码支持
- Cloudflare 5xx错误码支持：将Cloudflare专有的520-525错误码归类为服务器错误
- 智能重试策略：Cloudflare错误享受与502相同的重试策略和组故障转移
- 错误处理增强：在组故障情况下可触发请求挂起等待组切换
- 修改internal/proxy/error_recovery.go错误分类逻辑

**2025-09-23**: Major v3.4.0 流式Token修复与前端升级
- 流式Token丢失修复：实现FlushPendingEvent机制解决SSE终止空行缺失问题
- 事件缓冲区管理：在流结束/取消/中断时自动触发flush确保Token完整性
- empty_response精确判断：只在真正无使用量时标记，防止误判
- 前端架构迁移：完成React Layout架构升级，优化UI和交互体验
- 图表功能增强：新增端点成本分析，优化布局，完成概览页面融合
- 端点日志优化：修复流式请求尝试次数计数不准确问题
- 集成测试完善：新增streaming_missing_newline_test.go验证修复效果

**2025-09-20**: Major v3.3.2 架构优化版本
- 重试策略统一：移除复杂适配器架构，简化重试逻辑，提升性能
- 错误处理增强：消除重复错误分类日志，完善错误分类系统
- Token管理改进：失败请求Token保存机制，重复计费防护
- 流式请求修复：修复挂起状态存储bug，统一RetryCount语义
- 监控指标修复：修复错误类型断言失败导致的监控降级问题
- 开发规范：添加AGENTS.md项目开发规范文档

**2025-09-13**: Major v3.1.0 功能增强版本
- 异步模型解析：从请求体中零延迟提取模型名称，解决count_tokens端点显示"unknown"问题
- 智能模型对比：检测请求体与响应模型不一致并警告，以响应模型为准
- 优化Web界面：简化请求追踪列表，支持点击行查看详情
- 完整流式识别：多模式检测流式请求，精确标记处理方式
- 线程安全模型管理：RWMutex保护，支持并发访问
- 多项图表修复：端点健康状态、Token分布等图表显示问题

**2025-09-12**: Major v3.0.0 modular refactoring
- Complete handler.go modular architecture with single responsibility principle
- Dedicated modules: handlers/ (streaming, regular, forwarder) and response/ (processor, analyzer, utils)
- Enhanced maintainability with 1,568 lines split across 7 specialized modules
- Full functional compatibility with improved code organization
- Fixed streaming request endpoint logging issue (endpoint=unknown)
- All 25+ test files continue to pass with identical behavior

**2025-09-11**: Major v2.0 architecture upgrade
- Stream Processor v2 with advanced streaming capabilities
- Intelligent error recovery and classification system
- Complete request lifecycle management
- 25+ comprehensive test files added
- Unified request processing architecture

**2025-09-09**: Token parsing and status system enhancements
- Fixed critical token parsing duplication bug
- Enhanced request status granularity for better user experience
- Improved error handling and status tracking

**2025-09-05**: Web handler refactoring and JavaScript modularization
- Modular architecture with 11 specialized handler files
- Modern JavaScript module system for better maintainability
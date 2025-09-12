# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Version Information

**Current Version**: v3.0.0 Architecture (2025-09-12)  
**Major Update**: Handler.go modular refactoring with enhanced maintainability

## Project Overview

Claude Request Forwarder is a high-performance Go application that transparently forwards Claude API requests to multiple endpoints with intelligent routing, health checking, and automatic retry/fallback capabilities.

**Key Features v3.0**:
- **Modular Architecture**: Complete handler.go refactoring with single responsibility principle
- **Dual Processing**: Streaming v2 and Unified v2 request processing
- **Intelligent Error Recovery**: Smart error classification and recovery strategies  
- **Complete Lifecycle Tracking**: End-to-end request monitoring and analytics
- **Advanced Streaming**: Real-time SSE processing with cancellation support
- **Comprehensive Testing**: 25+ test files with extensive coverage

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
- **`internal/proxy/`**: Modular request forwarding with v2.1 architecture
  - `handler.go`: Core HTTP request coordinator (~430 lines)
  - **`handlers/`**: Specialized request processing modules ⭐ NEW
    - `streaming.go`: Streaming request handler (~310 lines) ⭐ NEW
    - `regular.go`: Regular request handler (~311 lines) ⭐ NEW
    - `forwarder.go`: HTTP request forwarder (~144 lines) ⭐ NEW
    - `interfaces.go`: Component interfaces (~115 lines) ⭐ NEW
  - **`response/`**: Response processing modules ⭐ NEW
    - `processor.go`: Response processing and decompression (~173 lines) ⭐ NEW
    - `analyzer.go`: Token analysis and parsing (~346 lines) ⭐ NEW
    - `utils.go`: Response utility functions (~21 lines) ⭐ NEW
  - `stream_processor.go`: Advanced streaming processor v2
  - `error_recovery.go`: Intelligent error handling
  - `lifecycle_manager.go`: Complete request lifecycle tracking
- **`internal/endpoint/`**: Endpoint management and health checking
- **`internal/web/`**: Web interface with real-time monitoring
- **`internal/tracking/`**: Usage tracking and analytics
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
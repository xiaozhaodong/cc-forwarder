# Claude API 智能转发器 (Claude API Smart Forwarder)

一个基于 Go 语言开发的高性能 Claude API 请求智能转发器，具有智能路由、健康检查、自动重试/故障转移、实时监控等功能。

## 🎯 项目说明

本项目基于 [xinhai-ai/endpoint_forwarde](https://github.com/xinhai-ai/endpoint_forwarder) 进行二次开发和功能增强。

### 原项目说明

- **原项目地址**: https://github.com/xinhai-ai/endpoint_forwarder
- **原项目许可**: "This project is provided as-is for educational and development purposes"
- **感谢原作者**: 感谢原项目作者提供的基础框架和核心功能

### 增强功能

在原项目基础上，本项目新增了以下重要功能：

- ✨ **Web管理界面**: 现代化的Web界面，支持实时监控和组管理
- 🎯 **请求ID追踪**: 完整的请求生命周期追踪系统
- 🤖 **Token解析器**: Claude API SSE流中的模型信息和Token使用量提取
- ⏸️ **请求挂起系统**: 智能请求挂起和恢复机制
- 🔄 **手动组切换**: 支持手动暂停/恢复/激活组操作
- 📊 **实时数据流**: Server-Sent Events (SSE) 实时更新
- 🗃️ **使用情况追踪设计**: SQLite数据库使用统计和成本分析设计方案
- 🚀 **SQLite读写分离优化**: 专为本地使用优化的高并发数据库架构 **[v3.1.1新增]**
- 🔄 **重试策略优化**: 统一简化的重试架构，提升性能和维护性 **[v3.3.0新增]**
- 💰 **Token管理增强**: 失败请求Token保存，重复计费防护机制 **[v3.2.1新增]**
- 🛡️ **错误处理改进**: 消除重复日志，完善错误分类系统 **[v3.3.2新增]**

## 🎉 v3.5.0 版本亮点

### 🚀 状态机架构重构
- **双轨状态管理**: 业务状态与错误状态完全分离
  - 业务状态: pending → forwarding → processing → completed/failed/cancelled
  - 错误状态: retry, suspended 独立管理
  - 前端可同时显示业务进度和错误原因
- **统一事件系统**: 新增 flexible_update, success, final_failure 语义化事件类型
- **数据库Schema增强**: 新增 failure_reason, last_failure_reason, cancel_reason 字段

### 🗄️ MySQL数据库支持
- **数据库适配器模式**: 支持SQLite和MySQL切换，连接池管理
- **完整实现**:
  - database_adapter.go (+144行): 适配器接口
  - mysql_adapter.go (+602行): MySQL实现
  - sqlite_adapter.go (+337行): SQLite实现
- **时区支持**: 统一时区处理，向后兼容

### 🔢 /v1/messages/count_tokens 端点
- **智能转发**: 优先转发到支持端点，失败降级到本地估算
- **新增处理器**: count_tokens.go (+188行)

### ⚡ 端点自愈机制
- **快速恢复**: 从5分钟优化到0.7秒自动恢复
- **智能检测**: 监控503/502错误触发恢复检查
- **新增模块**: endpoint_recovery_manager.go 等3个文件 (+580行)

## 🎉 v3.1.1 版本亮点

### 🎯 SQLite并发问题彻底解决
- **读写分离架构**: 8个读连接 + 1个写连接，完全适配SQLite单写者特性
- **写操作队列化**: 串行处理所有写操作，彻底消除`SQLITE_BUSY`错误
- **事务安全重构**: 修复`defer tx.Rollback()`嵌套问题
- **本地优化**: 专为本地SQLite使用场景设计，性能与稳定性双重提升

### 📊 技术优势
- **零配置升级**: 完全向后兼容，无需修改现有配置
- **高并发读取**: 支持多客户端同时查询，响应速度显著提升
- **锁竞争消除**: 写操作不再冲突，系统稳定性大幅改善
- **持续时间修复**: 确保请求耗时数据完整准确记录

## 🚀 核心功能

### 基础转发功能

- **透明代理**: 透明转发所有HTTP请求到后端端点
- **SSE流式支持**: 完整支持Server-Sent Events流式传输，智能识别流式请求
- **Token管理**: 每个端点可配置独立的Bearer Token
- **路由策略**: 支持优先级路由和最快响应路由
- **健康检查**: 自动端点健康监控
- **重试与故障转移**: 指数退避重试和自动端点故障转移

### 高级功能

- [ ] **组管理**: 智能端点分组，支持自动故障转移和冷却期
- [ ] **监控**: 内置健康检查和Prometheus风格的指标
- [ ] **结构化日志**: 可配置的JSON或文本日志，多级别支持
- [ ] **TUI界面**: 内置终端用户界面，支持实时监控和交互式优先级编辑
- [ ] **动态优先级覆盖**: 通过 `-p`参数进行运行时端点优先级调整

### 增强功能 (本项目新增)

#### 🌐 Web管理界面

- **实时仪表板**: 使用SSE进行实时更新的现代化Web界面
- **优化的请求追踪**: 简洁的列表展示，支持点击行查看详情
- **组管理**: 交互式组控制，支持激活/暂停/恢复操作
- **端点监控**: 可视化健康状态和性能指标
- **图表分析**: 使用Chart.js进行性能可视化
- **响应式设计**: 移动设备友好的现代CSS样式
- **API集成**: 完整的RESTful API支持所有管理操作

#### 🎯 请求ID追踪系统

- **短UUID格式**: `req-xxxxxxxx` 格式，便于跟踪和搜索
- **完整生命周期**: 从请求开始到完成/挂起的全程追踪
- **日志集成**: 所有关键日志都包含请求ID
- **调试友好**: 大幅提升问题排查和日志分析效率

#### 🤖 智能Token解析器

- **异步模型解析**: 从请求体中异步提取模型信息，零延迟转发
- **智能模型对比**: 检测请求体与响应中模型不一致情况并警告
- **模型检测**: 从Claude API SSE流中提取模型信息
- **Token统计**: 精确统计输入/输出/缓存Token使用量
- **实时监控**: 集成到日志系统中，方便成本分析
- **多事件解析**: 同时处理 `message_start`和 `message_delta`事件
- **线程安全**: 支持并发访问，保证数据一致性

#### ⏸️ 请求挂起与恢复系统

- **智能挂起**: 在端点不可用时自动挂起请求
- **自动恢复**: 端点恢复后自动处理挂起的请求

#### 📊 使用情况追踪系统 (Usage Tracking)

- **SQLite数据库**: 使用纯Go SQLite驱动，完美支持Windows/Linux/macOS
- **全方位统计**: Token使用量、请求成功率、端点性能、成本分析
- **实时监控**: Web界面实时显示使用统计和成本信息
- **数据导出**: 支持CSV/JSON格式导出，便于进一步分析
- **自动化处理**: 异步数据记录，不影响请求转发性能
- **成本计算**: 基于模型定价自动计算Token使用成本

**🪟 Windows兼容性保证**: v1.0.2版本彻底解决了Windows平台SQLite依赖问题，现在可以无障碍启用使用追踪功能。

**配置示例**:
```yaml
usage_tracking:
  enabled: true                          # 启用使用追踪
  database_path: "data/usage.db"         # 数据库文件路径
  buffer_size: 1000                      # 内存缓冲区大小
  batch_size: 100                        # 批量写入大小
  flush_interval: "5s"                   # 刷新间隔
  
  model_pricing:                         # 模型定价配置
    "claude-3-5-haiku-20241022":
      input: 1.00          # 每百万Token价格(USD)
      output: 5.00         # 输出Token价格
      cache_creation: 1.25 # 缓存创建Token价格
      cache_read: 0.10     # 缓存读取Token价格
    
    "claude-sonnet-4-20250514":
      input: 3.00
      output: 15.00
      cache_creation: 3.75
      cache_read: 0.30
```
- **超时保护**: 配置超时时间防止请求无限挂起
- **容量控制**: 限制最大挂起请求数量

#### 🔄 手动组管理

- **灵活控制**: 支持自动和手动两种组切换模式
- **Web界面操作**: 通过Web界面进行组的暂停/恢复/激活
- **实时状态**: SSE实时更新组状态变化
- **冷却管理**: 智能冷却期管理和状态显示

## 📋 快速开始

1. **构建应用程序**:

   ```bash
   go build -o cc-forwarder
   ```
2. **复制并配置示例配置**:

   ```bash
   cp config/example.yaml config/config.yaml
   # 编辑 config.yaml 配置你的端点和tokens
   ```
3. **运行转发器**:

   ```bash
   # 默认模式，带TUI界面
   ./cc-forwarder -config config/config.yaml

   # 不带TUI的传统控制台模式
   ./cc-forwarder -config config/config.yaml --no-tui

   # 显式启用TUI（默认行为）
   ./cc-forwarder -config config/config.yaml --tui

   # 运行时覆盖端点优先级（用于测试或故障转移）
   ./cc-forwarder -config config/config.yaml -p "endpoint-name"
   ```
4. **配置Claude Code**:
   在Claude Code的 `settings.json`中设置：

   ```json
   {
     "ANTHROPIC_BASE_URL": "http://localhost:8088"
   }
   ```
5. **访问Web界面**（推荐）:

   ```
   http://localhost:8010
   ```

## 🔧 配置说明

### Web界面配置（推荐用于生产环境）

```yaml
web:
  enabled: true              # 启用Web界面
  host: "0.0.0.0"           # Web界面主机（默认: localhost）
  port: 8010                 # Web界面端口（默认: 8088）
```

### TUI界面配置（开发/调试用）

```yaml
tui:
  enabled: false             # 生产/Docker环境中禁用
  update_interval: "1s"      # TUI刷新间隔
  save_priority_edits: false # 保存优先级变更到配置文件
```

### 组管理配置

```yaml
group:
  cooldown: "600s"                      # 组故障冷却时间（默认: 10分钟）
  auto_switch_between_groups: true      # 启用组间自动切换（默认: true）
  # false = 需要通过Web界面手动干预
  # true = 自动故障转移到备用组
```

### 请求挂起配置

```yaml
request_suspend:
  enabled: true                # 启用挂起功能
  timeout: "300s"             # 挂起超时时间（5分钟）
  max_suspended_requests: 100  # 最大挂起请求数
```

## 🌟 使用场景

1. **高可用性**: 主备组配置，确保关键服务不中断
2. **成本优化**: 根据优先级使用不同供应商（如 GPT-4 → Claude → 本地模型）
3. **地理路由**: 按区域对端点分组，自动故障转移
4. **负载均衡**: 跨多个组分配负载，不同优先级
5. **开发测试**: 通过Web界面轻松切换和测试不同端点

## 📊 监控端点

转发器提供多个监控端点：

- **GET /health**: 基本健康检查
- **GET /health/detailed**: 所有端点的详细健康信息
- **GET /metrics**: Prometheus风格的指标

### Web API参考

#### 组管理API

```bash
# 获取所有组状态
GET /api/v1/groups

# 手动激活一个组
POST /api/v1/groups/{name}/activate

# 暂停一个组（手动干预）
POST /api/v1/groups/{name}/pause

# 恢复一个暂停的组
POST /api/v1/groups/{name}/resume
```

#### 监控API

```bash
# 获取系统状态
GET /api/v1/status

# 获取端点状态
GET /api/v1/endpoints

# 获取连接统计
GET /api/v1/connections

# 通过Server-Sent Events进行实时更新
GET /api/v1/stream?client_id={id}&events=status,endpoint,group,connection,log,chart
```

## 🛠️ 开发与构建

```bash
# 构建应用程序
go build -o cc-forwarder

# 运行测试
go test ./...

# 测试特定包
go test ./internal/endpoint
go test ./internal/proxy
go test ./internal/middleware

# 检查版本
./cc-forwarder -version
```

## 📚 版本信息

**当前版本**: v3.0.0 (2025-09-12)

**主要特性**: 
- 🏗️ 模块化架构重构，提升代码可维护性
- 🚀 高性能智能转发，支持流式和常规请求处理
- 🎯 完整的请求生命周期追踪系统
- 📊 现代化Web管理界面与实时监控

**详细更新历史**: 请查看 [CHANGELOG.md](./CHANGELOG.md)

## 🤝 贡献

欢迎提交Issue和Pull Request！

## 📄 许可证

本项目基于原项目进行二次开发，遵循原项目的许可声明："This project is provided as-is for educational and development purposes."

## 🙏 致谢

- 感谢 [xinhai-ai/endpoint_forwarder](https://github.com/xinhai-ai/endpoint_forwarder) 项目提供的基础框架
- 感谢开源社区提供的各种优秀库和工具

---

**开发者**: xiaozhaodong
**项目地址**: https://github.com/xiaozhaodong/cc-forwarder
**原项目**: https://github.com/xinhai-ai/endpoint_forwarder

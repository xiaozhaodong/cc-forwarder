# CLAUDE.md

## Project Overview

Claude Request Forwarder是高性能Go应用，透明转发Claude API请求到多个端点，支持智能路由、健康检查和自动重试/故障转移。包含TUI和Web界面用于实时监控管理。

## 构建和开发命令

```bash
# 构建应用
go build -o cc-forwarder

# 运行(TUI模式)
./cc-forwarder -config config/config.yaml

# 运行(控制台模式)
./cc-forwarder -config config/config.yaml --no-tui

# 运行测试
go test ./...
```

## 架构概述

### 核心组件
- **`main.go`**: 应用入口，TUI/控制台模式切换
- **`config/`**: 配置管理，热重载
- **`internal/endpoint/`**: 端点管理，健康检查，组管理
- **`internal/proxy/`**: HTTP请求转发，流式支持，重试逻辑
- **`internal/middleware/`**: 认证、日志、监控中间件
- **`internal/web/`**: Web界面，SSE实时监控
- **`internal/transport/`**: 代理传输配置

### 请求流程
1. 中间件链处理(认证→日志→监控)
2. 基于策略和健康状态选择端点
3. 请求头转换(移除客户端认证，注入端点token)
4. 请求转发和重试处理
5. 流式响应或缓冲响应处理
6. 错误处理和自动故障转移

## 配置

**主配置**: `config/config.yaml` (复制自 `config/example.yaml`)
**热重载**: 通过fsnotify自动重载，500ms防抖
**动态Token解析**: 运行时动态解析token和API密钥

### 界面配置
```yaml
web:
  enabled: true
  host: "0.0.0.0"
  port: 8010

tui:
  enabled: false  # 生产环境禁用
  update_interval: "1s"

group:
  cooldown: "600s"
  auto_switch_between_groups: true  # 自动组切换
```

### 组管理

**组配置**:
- 端点通过`group`字段属于某个组
- 组优先级通过`group-priority`定义(数值越小优先级越高)
- 同时只有一个组处于活跃状态

**组状态**:
- **Active**: 当前处理请求
- **Available**: 健康可激活但未激活
- **Cooldown**: 故障后冷却中
- **Paused**: 手动暂停
- **Unhealthy**: 组内所有端点不可用

**手动操作API**:
- `POST /api/v1/groups/{name}/activate` - 激活组
- `POST /api/v1/groups/{name}/pause` - 暂停组
- `POST /api/v1/groups/{name}/resume` - 恢复组

**组继承规则**:
- 端点继承前一个端点的`group`和`group-priority`
- 静态继承: 配置解析时继承`timeout`和`headers`
- 动态解析: 运行时从组内第一个端点解析`token`和`api-key`

**配置示例**:
```yaml
endpoints:
  - name: "primary"
    url: "https://api.openai.com"
    group: "main"
    group-priority: 1
    token: "sk-main-token"  # 组内共享
    
  - name: "backup"
    url: "https://api.anthropic.com"
    priority: 2  # 继承group: "main"
```

## 测试

包含完整的单元测试:
- 各包中的`*_test.go`文件
- `test_config.yaml`测试配置
- 健康检查和代理处理器测试

## 请求ID跟踪和生命周期监控

**请求ID生成**: 系统为每个请求生成唯一的短UUID格式ID `req-xxxxxxxx` (8位十六进制)

**完整生命周期跟踪**: 通过请求ID追踪完整请求生命周期

**日志覆盖**: 所有关键事件包含请求ID:
- 请求开始/结束: `🚀 Request started [req-xxx]`
- 端点选择: `🎯 [请求转发] [req-xxx] 选择端点`
- 成功/失败: `✅ [请求成功] [req-xxx]`
- 重试逻辑: `🔄 [需要重试] [req-xxx]`
- 请求挂起: `⏸️ [请求挂起] 连接 req-xxx`

**调试**: 使用`grep "req-xxxxxxxx" logfile`过滤特定请求日志

**Token解析器和模型检测** (2025-09-09更新):
- **架构修复**: 解决token解析重复处理bug
- **事件分离**: `message_start`提取模型信息，`message_delta`处理token统计
- **模型提取**: 自动提取Claude模型信息
- **非Claude端点兼容**: 为无token信息端点添加fallback机制
- **数据库状态修复**: 修正SQL逻辑，正确更新状态到completed

**优势**: 易于调试、性能分析、问题解决、请求关联

## 请求状态系统 (2025-09-09更新)

**增强状态粒度**: 提供细粒度请求状态跟踪，消除用户混淆

### 状态生命周期
```
正常流程: pending → forwarding → processing → completed
重试流程: pending → forwarding → retry → processing → completed
挂起流程: pending → forwarding → suspended → forwarding → processing → completed
```

### 状态定义
- **`forwarding`**: 请求转发中
- **`retry`**: 请求重试中
- **`processing`**: HTTP响应成功，Token解析中
- **`completed`**: Token解析和成本计算完成
- **`suspended`**: 请求暂停等待组恢复
- **`error`**: 请求失败
- **`timeout`**: 请求超时

### 用户体验改进

**改进前**: `req-abc123 ✅ 成功 - 0 0 0 0 $0.00` (用户困惑：成功了为什么token是0？)
**改进后**: `req-abc123 ⚙️ 解析中 - 0 0 0 0 $0.00` → `req-abc123 ✅ 完成 claude-sonnet-4 25 97 0 0 $0.45`

### 状态指示器
- **🔄 转发中** (`forwarding`): 蓝色渐变脉动动画
- **⚙️ 解析中** (`processing`): 橙色渐变脉动动画  
- **✅ 完成** (`completed`): 绿色渐变
- **❌ 失败** (`error`): 红色渐变
- **⏰ 超时** (`timeout`): 橙红渐变

**优势**: 消除用户混淆，提供清晰处理透明度，改善调试能力，提升Web界面用户体验。

### 非Token响应处理 (2025-09-07更新)

**增强兼容性**: 为不包含token信息的响应提供智能fallback机制

#### **解决问题**
之前成功返回(200 OK)但无token信息的请求会无限停留在`processing`状态

#### **常见非Token响应类型**
- 健康检查请求: `/v1/models`端点
- 第三方API: 非Claude兼容端点
- 配置查询: 系统配置或状态端点
- 错误响应: 非标准错误格式

#### **Fallback实现**
无token信息时标记为completed，使用"default"模型，token数为0，成本$0.00

**技术优势**: 确保所有响应类型的健壮请求跟踪，提高系统可靠性，提供完整审计跟踪。

## 请求挂起和恢复系统

**请求挂起能力**: 当所有端点组失败时智能挂起请求，防止临时故障期间请求丢失

### 配置
```yaml
request_suspend:
  enabled: true
  timeout: "300s"             # 最大挂起时间
  max_suspended_requests: 100  # 最大挂起请求数
```

### 挂起行为
- **自动挂起**: 所有组失败时自动挂起而非丢弃请求
- **组恢复检测**: 持续监控组恢复并自动恢复挂起请求
- **FIFO处理**: 先进先出顺序处理挂起请求
- **超时保护**: 超时挂起请求自动失败
- **容量管理**: 限制挂起请求数量防止内存耗尽

### 挂起生命周期
1. 正常处理 → 2. 组失败 → 3. 全部失败(挂起) → 4. 组恢复 → 5. 恢复处理 → 6. 超时处理

## 关键特性

**TUI界面**: 实时监控，包含概述、端点、连接、日志和配置标签页

**Web界面**: 现代Web监控管理界面，包含:
- 实时仪表板(SSE即时更新)
- 请求跟踪(完整生命周期监控)
- 组管理(交互式激活/暂停/恢复操作)
- 端点监控(可视化健康状态)
- 图表分析(Chart.js集成)
- 数据导出(CSV/JSON)
- 模块化架构和响应式设计

**其他特性**:
- 流式支持: 自动SSE检测和实时流处理
- 代理支持: HTTP/HTTPS/SOCKS5配置
- 安全: Bearer token认证，API key支持
- 健康监控: 持续端点健康检查
- 高级组管理: 自动/手动切换，优先级路由

## SSE事件驱动架构 (2025-09-10更新)

**纯事件驱动Web界面**: 从轮询更新完全转换为纯Server-Sent Events(SSE)架构，实现最佳性能和实时响应性。

### 架构转换
**之前**: 混合轮询+SSE方式，30秒服务端状态更新循环，前端定时器运行时间更新
**之后**: 100%事件驱动架构零周期轮询，服务器发送启动时间戳一次，前端实时计算运行时间，智能SSE缓存即时UI更新

### 关键SSE增强
#### **前端实时运行时间计算(行业最佳实践)**
```javascript
// 服务器发送启动时间戳一次
{ "start_timestamp": 1757485684 }

// 前端每秒计算运行时间
setInterval(() => {
    const uptimeSeconds = Math.floor(Date.now() / 1000) - serverStartTimestamp;
    displayUptime(formatUptime(uptimeSeconds));
}, 1000);
```

**优势**: 零服务器负载，完美准确性，网络弹性，行业标准

#### **智能SSE数据缓存**
- **端点页面**: SSE缓存→API后备
- **组页面**: SSE缓存→API后备  
- **图表页面**: 纯SSE数据自动刷新
- **概述页面**: 组合数据总是重新加载
- **请求页面**: 传统缓存(大数据集)

#### **实时单端点更新**
健康检查完成→单个表格行更新(无需全页刷新)

### 技术实现细节
- **后端优化**: 移除所有周期广播循环，增强单端点更新
- **前端架构**: SSE事件处理，缓存管理，实时更新
- **响应时间显示优化**: 基于响应时间大小的智能精度

### 性能优化
**调试日志清理**: 移除高频调试日志(~50+日志/分钟)影响浏览器性能
**网络流量减少**: 端点健康检查、状态更新、组管理缓存数据消除重复API请求

### 浏览器性能影响
| 指标 | 之前 | 之后 | 改进 |
|------|------|------|------|
| 控制台日志/分钟 | ~150 | ~10 | 93%减少 |
| API调用/页面切换 | 2-3 | 0-1 | 67%减少 |
| 内存使用 | 更高 | 更低 | 控制台对象清理 |
| UI响应性 | 1-30秒延迟 | <1秒 | 实时更新 |

**技术结果**: 纯事件驱动架构，行业标准实时监控能力，优化性能，卓越用户体验。

## Web API参考

应用程序提供完整的REST API用于监控和管理:

### 组管理API
```bash
GET /api/v1/groups                    # 获取所有组状态
POST /api/v1/groups/{name}/activate   # 手动激活组
POST /api/v1/groups/{name}/pause      # 暂停组
POST /api/v1/groups/{name}/resume     # 恢复组
```

### 监控API
```bash
GET /api/v1/status                    # 系统状态
GET /api/v1/endpoints                 # 端点状态
GET /api/v1/connections               # 连接统计
# 通过SSE实时更新
GET /api/v1/stream?client_id={id}&events=status,endpoint,group
```

### 使用跟踪API
```bash
# 使用统计(带过滤)
GET /api/v1/usage/stats?start_date=2025-01-01&model=claude-3-5-haiku
# 详细请求日志
GET /api/v1/usage/requests?limit=100&model=claude-sonnet-4&status=success
# 导出使用数据
GET /api/v1/usage/export?format=csv&start_date=2025-09-01
GET /api/v1/usage/export?format=json&model=claude-3-5-haiku
# 数据库健康统计
GET /api/v1/usage/health
```

### 认证
所有API请求需要Bearer token认证:
```bash
curl -H "Authorization: Bearer your-token-here" http://localhost:8010/api/v1/groups
```

## 开发架构

### 文件结构
```
internal/
├── web/                        # Web服务器和路由
│   ├── basic_handlers.go       # 基础API处理器(233行)
│   ├── sse_handlers.go         # SSE事件处理器(249行)
│   ├── group_handlers.go       # 组管理(140行)
│   ├── usage_handlers.go       # 使用跟踪API(183行)
│   ├── templates.go            # HTML模板(1272行)
│   └── static/js/              # 模块化JavaScript架构
│       ├── utils.js            # 工具函数和格式化
│       ├── sseManager.js       # SSE连接管理
│       ├── requestsManager.js  # 请求跟踪功能
│       ├── webInterface.js     # 核心Web界面类
│       └── charts.js           # Chart.js集成
├── endpoint/
│   ├── manager.go              # 端点和组管理
│   └── group_manager.go        # 高级组操作
├── tracking/
│   ├── tracker.go              # 主使用跟踪器异步操作
│   ├── database.go             # 数据库操作和模式管理
│   ├── queries.go              # 查询方法和数据检索
│   └── schema.sql              # 数据库模式时区修复
└── proxy/
    ├── retry.go                # 可配置重试逻辑组切换
    └── token_parser.go         # SSE令牌解析模型检测
```

### 代码架构重构 (2025-09-05)

**主要Web处理器重构**: 单一`handlers.go`文件(2475行)成功重构为11个专门文件的模块化架构:

**重构优势**:
- **之前**: 单一2475行文件混合职责
- **之后**: 11个专注文件总计~4105行清晰模块边界
- **质量**: 100%功能保留改进代码组织
- **性能**: 更好编译时间和减少认知负担

### 重要实现说明

1. **HTML模板**: Web界面HTML现在在专门的`templates.go`文件中，修改需要重新编译
2. **静态资源**: CSS和JS文件从文件系统提供，可以在不重新编译的情况下修改
3. **纯SSE架构**: 实时更新使用100%事件驱动SSE零周期轮询，由`sse_handlers.go`处理
4. **前端运行时间计算**: 服务器发送启动时间戳一次，前端实时计算运行时间(1秒间隔)
5. **智能SSE缓存**: 智能分页缓存策略在保持实时响应性的同时消除冗余API调用
6. **组状态管理**: `group_handlers.go`中的线程安全组操作与适当的锁定机制
7. **配置热重载**: 文件系统监控与防抖更新(500ms延迟)
8. **使用跟踪**: `usage_handlers.go`中完全异步数据库操作与适当的时区处理(CST +08:00)
9. **模块化架构**: Go后端和JavaScript前端都使用模块化设计以获得更好的可维护性和团队协作
10. **性能优化**: 调试日志清理将浏览器控制台输出减少93%，显著提高性能

### JavaScript模块架构

**现代前端设计** (2025-09-10更新):
Web界面使用为SSE事件驱动更新优化的模块化JavaScript架构:

- **utils.js** (302行): 格式化函数，通知，DOM实用程序，响应时间格式化
- **sseManager.js** (900+行): SSE连接，实时运行时间计算，事件处理，连接监控
- **requestsManager.js** (512行): 请求跟踪，过滤，分页，导出
- **webInterface.js** (494行): 核心类，智能缓存，标签管理，初始化

**SSE优化优势**:
- **实时性能**: 通过优化SSE事件处理实现亚秒级UI更新
- **智能缓存**: 智能分页缓存策略减少67%API调用
- **连接弹性**: 自动重连逻辑与运行时间暂停/恢复
- **调试优化**: 控制台日志减少93%，获得更好的浏览器性能
- **内存效率**: 减少对象创建和清理周期

## 使用跟踪系统

### 完整请求生命周期跟踪

**请求跟踪界面** (2025-09-05更新):
Web界面现在包括一个全面的请求跟踪页面，替换了简单的日志视图:

**功能**:
- **多维过滤**: 按日期范围，状态，模型，端点，组过滤
- **实时更新**: 通过SSE集成进行实时请求监控  
- **详细视图**: 完整的请求生命周期，包含时间，令牌和成本信息
- **导出功能**: 带灵活过滤选项的CSV/JSON导出
- **性能分析**: 统计摘要和趋势
- **分页支持**: 高效浏览大型请求数据集

### 数据库模式和时区处理

系统使用带WAL模式的SQLite进行高性能使用跟踪。**所有时间戳字段使用本地时区(CST +08:00)**准确时间记录:

```sql
created_at DATETIME DEFAULT (datetime('now', 'localtime')),
updated_at DATETIME DEFAULT (datetime('now', 'localtime'))
```

### 异步操作设计

**完全非阻塞架构**:
- **事件通道**: 缓冲通道(默认1000事件)非阻塞发送
- **批处理**: 为高效数据库写入分组事件(默认100事件/批次)
- **独立协程**: 单独的处理线程防止主请求阻塞
- **优雅降级**: 缓冲区溢出时丢弃事件(带日志)而不是阻塞

### 关键错误修复 (2025-09-09)

**Token解析器架构改革**:
- **问题**: `message_start`和`message_delta`事件都在处理token使用，导致双倍token计算和错误成本计算
- **解决方案**: 清晰分离-`message_start`仅提取模型信息，`message_delta`处理完整token统计
- **影响**: 准确的成本计算，不再有重复token计算，为非Claude端点提供适当的fallback

### 数据收集点

**请求生命周期跟踪**:
```go
// 1. 请求开始 (middleware/logging.go:76)
usageTracker.RecordRequestStart(requestID, clientIP, userAgent)

// 2. 状态更新 (proxy/retry.go:154,185,211)
usageTracker.RecordRequestUpdate(requestID, endpoint, group, status, retryCount, httpStatus)

// 3. Token完成 (proxy/token_parser.go:206)
usageTracker.RecordRequestComplete(requestID, modelName, tokens, duration)
```

### 模型检测和Token解析

**双SSE事件处理**:
- **message_start**: 提取模型信息(例如，`claude-3-5-haiku-20241022`)
- **message_delta**: 处理响应流中的token使用
- **集成日志**: token使用日志中包含模型信息
- **安全实现**: 模型提取不影响客户端响应

### 成本计算

**实时定价集成**:
```yaml
model_pricing:
  "claude-sonnet-4-20250514":
    input: 3.00          # 每100万token的美元
    output: 15.00
    cache_creation: 3.75 # 输入的1.25倍用于缓存创建
    cache_read: 0.30     # 输入的0.1倍用于缓存读取
```

### 性能特征

**验证操作指标** (2025-09-05):
- **零阻塞**: 所有数据库操作异步
- **准确时区**: 所有字段中的CST +08:00时间戳
- **模型检测**: 带模型信息的SSE流100%成功率
- **成本跟踪**: 包括缓存token成本的精确计算
- **示例使用**: 5个请求，$0.044938总成本，1,148输入+97输出token

### 数据导出功能

**多格式导出支持**:
```go
// 带完整请求生命周期的CSV导出
tracker.ExportToCSV(ctx, startTime, endTime, modelName, endpointName, groupName)

// 用于程序访问的JSON导出
tracker.ExportToJSON(ctx, startTime, endTime, modelName, endpointName, groupName)
```

### 故障排除

**已解决的常见问题**:
1. **时区问题**: 使用`datetime('now', 'localtime')`而不是`CURRENT_TIMESTAMP`
2. **缺少模型名称**: 确保SSE流包含`message_start`事件
3. **高成本**: 监控缓存token使用(cache_creation_tokens, cache_read_tokens)
4. **性能影响**: 所有跟踪操作完全异步且非阻塞
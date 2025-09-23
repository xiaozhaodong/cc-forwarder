# Changelog

所有项目的重要更改都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
项目遵循 [语义化版本控制](https://semver.org/lang/zh-CN/)。

## [3.4.0] - 2025-09-23

### 🔧 关键修复 (Critical Fixes)
- **流式Token丢失修复**: 解决上游缺少SSE终止空行导致Token信息丢失的问题
  - 实现 `FlushPendingEvent()` 方法强制解析缓存事件
  - 在流结束、取消、中断时自动触发缓冲区刷新
  - 确保 `empty_response` 只在真正无使用量时出现
  - 新增集成测试验证修复效果 (`streaming_missing_newline_test.go`)
- **端点日志优化**: 修复流式请求端点失败日志中尝试次数计数不准确问题

### 🎨 前端架构升级 (Frontend Architecture)
- **React架构迁移**: 完成Web界面React Layout架构迁移
- **UI优化**: 完善前端状态处理和交互体验
- **图表功能增强**:
  - 新增端点Token使用成本分析功能
  - 优化图表页面布局和优先级
  - 完成概览页面与图表功能融合重构
- **交互改进**: 简化请求页面时间筛选交互

### 📝 技术细节 (Technical Details)
- 修改 `TokenParser.FlushPendingEvent()` 支持 message_delta/message_start/error 事件类型
- 在 `StreamProcessor.waitForBackgroundParsing()` 中调用flush确保完整解析
- 在 `collectAvailableInfoV2()` 错误恢复场景中调用flush保留Token信息

## [3.3.2] - 2025-09-20

### 🐛 错误修复 (Bug Fixes)
- **日志优化**: 消除重复错误分类日志，减少日志冗余
- **性能提升**: 优化错误处理流程，降低系统开销
- **监控改进**: 确保同一错误不会产生重复日志条目

## [3.3.1] - 2025-09-20

### 🔄 重试策略优化 (Retry Strategy Optimization)
- **架构重构**: 重试策略架构全面优化，提升可维护性
- **错误分类**: 完善错误分类系统，提升系统稳定性
- **统一处理**: 统一错误处理流程，减少重复日志问题

## [3.3.0] - 2025-09-20

### 🚀 重大架构更新 (Major Architecture Update)
- **重试策略统一**: 采用统一简化设计，移除过度复杂的适配器架构
- **架构简化**: 大幅简化代码结构，提升系统性能和维护性
- **响应速度**: 简化重试逻辑，提升请求处理速度

## [3.2.2] - 2025-09-20

### 🐛 关键修复 (Critical Fixes)
- **流式请求挂起**: 修复流式请求挂起状态存储问题
- **状态一致性**: 统一RetryCount语义，确保状态管理一致性
- **流式处理**: 优化流式请求的状态管理机制

## [3.2.1] - 2025-09-20

### 💰 Token管理增强 (Token Management Enhancement)
- **失败请求Token保存**: 即使请求失败也能保存Token使用信息
- **重复计费防护**: 防止异常情况下的重复计费问题
- **成本追踪**: 完善成本追踪系统，提升财务透明度

## [3.2.0] - 2025-09-20

### 🔄 重试管理器完善 (Retry Manager Enhancement)
- **413错误处理**: 专门处理Payload Too Large错误
- **HTTP 429优先级**: 修复限流错误分类，正确支持重试和挂起
- **监控指标修复**: 修复错误类型断言失败导致的监控降级问题

### 📖 文档增强 (Documentation Enhancement)
- **开发规范**: 添加AGENTS.md项目开发规范文档

## [3.1.2] - 2025-09-13

### 🎯 用户体验优化 (UX Optimization)
- **模型信息早期显示**: 解决用户在请求处理早期看到"unknown"模型的问题
- **搭便车更新机制**: 通过状态更新"搭便车"实现模型信息早期显示
- **渐进式优化**: 多次更新机会确保最终一致性，用户体验显著改善

### 🛡️ 架构改进 (Architecture Improvements)
- **零延迟转发**: 保持异步解析，主请求流程完全无阻塞
- **线程安全设计**: 使用互斥锁保护更新标记，支持并发环境
- **重复更新保护**: 模型信息仅在首次有效时写入数据库，避免重复操作
- **幂等SQL设计**: UPDATE操作天然幂等，确保数据一致性

### 📈 技术实现 (Technical Implementation)
- **lifecycle_manager.go**: 添加搭便车机制和`modelUpdatedInDB`保护标记
- **tracker.go**: 新增`RequestUpdateDataWithModel`结构和`RecordRequestUpdateWithModel`方法  
- **database.go**: 实现`buildUpdateWithModelQuery`方法，支持`update_with_model`事件类型
- **向后兼容**: 不影响现有功能，渐进式增强

### ✨ 预期效果 (Expected Benefits)
- **早期可见性**: 模型信息在请求处理早期阶段就能显示
- **准确性保证**: 最终仍以SSE解析的权威模型为准
- **性能保持**: 利用现有状态更新流程，最小化额外开销

## [3.1.1] - 2025-09-13

### 🎯 重大优化 (Major Optimization)
- **SQLite读写分离架构**: 专为本地使用设计的数据库并发处理架构
- **写操作队列化**: 实现单写连接 + 队列串行化，彻底消除SQLite锁竞争
- **8读+1写连接模式**: 最大化读性能，完全避免写冲突

### 🔧 关键修复 (Critical Fixes)  
- **SQLITE_BUSY错误**: 彻底解决"database is locked"并发问题
- **事务嵌套问题**: 修复"cannot start a transaction within a transaction"错误
- **持续时间存储**: 确保request duration_ms字段正确保存
- **安全事务处理**: 重构defer rollback逻辑，避免重复回滚

### 📊 架构改进 (Architecture Improvements)
- **读写分离设计**: readDB处理查询，writeDB处理修改，完全适配SQLite单写者特性  
- **队列化写入**: 所有写操作通过队列串行处理，保证数据一致性
- **连接池优化**: 针对SQLite调整连接数配置，避免过度并发
- **完整向后兼容**: 所有现有API保持不变，零配置升级

### 🎯 性能提升 (Performance Gains)
- **消除锁竞争**: 写操作不再竞争，稳定性大幅提升
- **高并发读取**: 8个读连接支持高并发查询操作
- **本地优化**: 专门为本地SQLite使用场景优化

### 📝 技术细节 (Technical Details)
- **文件修改**: tracker.go、database.go、queries.go大幅重构
- **新增结构**: WriteRequest队列，读写分离连接管理
- **事务安全**: committed标志位确保事务正确提交

## [3.1.0] - 2025-09-13

### 🚀 新增 (Added)
- **异步模型解析**: 从请求体中异步提取模型名称，解决`/v1/messages/count_tokens`端点模型显示"unknown"问题
- **智能模型对比**: 请求体与响应中模型不一致时输出警告，以响应中模型为准
- **流式请求标记**: 完整实现流式请求识别功能，支持多种检测模式
- **优化的请求追踪UI**: 去掉操作列，支持点击行查看详情，界面更简洁

### 🔧 修复 (Fixed)
- **端点健康状态图表**: 修复Web界面图表显示错误
- **Token分布图表**: 修复Web图表token分布不显示问题  
- **5xx服务器错误**: 添加专门的服务器错误分类处理
- **流式处理错误**: 优化错误分类与状态管理

### 📈 改进 (Improved)
- **线程安全**: 使用RWMutex保护模型字段，支持并发访问
- **性能优化**: 异步处理，主转发流程零延迟
- **日志增强**: 详细的模型来源标识和不一致警告
- **Token解析重构**: 职责分离，提升代码可维护性

### 📊 技术细节 (Technical Details)
- **异步架构**: goroutine并行处理，请求体副本避免数据竞争
- **智能对比逻辑**: 只在两边都有有效值时进行对比，避免误报
- **优先级策略**: 响应中解析的模型优先级高于请求体模型
- **UI交互改进**: 点击行查看详情，提升用户体验

## [3.0.0] - 2025-09-12

### 🏗️ 重构 (Refactored)
- **Handler.go模块化重构**: 按照单一职责原则完全重构，1194行代码拆分为7个专门模块
- **handlers/模块**: 创建专门的请求处理模块
  - `streaming.go` (~310行) - 流式请求处理器
  - `regular.go` (~311行) - 常规请求处理器  
  - `forwarder.go` (~144行) - HTTP请求转发器
  - `interfaces.go` (~115行) - 组件接口定义
- **response/模块**: 创建专门的响应处理模块
  - `processor.go` (~173行) - 响应处理和解压缩
  - `analyzer.go` (~346行) - Token分析和解析
  - `utils.go` (~21行) - 响应工具方法
- **handler.go精简**: 重构为核心协调器角色（~432行），专注于请求分发

### 🔒 兼容性保证 (Compatibility)
- **API完全兼容**: 所有对外接口保持完全不变
- **功能完全一致**: 所有业务逻辑和处理流程保持完全相同
- **性能无退化**: 重构不引入任何性能开销
- **测试100%通过**: 所有现有25+测试文件继续通过
- **日志格式一致**: 所有日志输出格式和内容完全保持不变

### 🔧 修复 (Fixed)
- **流式请求日志修复**: 修复流式请求日志中显示`endpoint=unknown`问题
- **上下文传递**: 在streaming.go中添加selected_endpoint上下文设置

### 📈 改进 (Improved)
- **可维护性增强**: 模块化设计，职责明确，便于理解和维护
- **可测试性改善**: 每个模块可独立测试，测试覆盖更全面
- **可扩展性提升**: 新功能可在对应模块中扩展，影响范围控制
- **代码可读性**: 逻辑分层清晰，代码结构更加合理

### 📚 文档更新 (Documentation)
- **CHANGELOG标准化**: 创建标准的CHANGELOG.md版本历史记录
- **架构文档更新**: 更新CLAUDE.md反映v3.0模块化架构
- **README精简**: 精简README.md，引用CHANGELOG.md详细历史

## [2.0.0] - 2025-09-11

### 🚀 新增 (Added)
- **Stream Processor v2**: 高级流式处理器，支持实时SSE处理和取消支持
- **智能错误恢复**: 智能错误分类和恢复策略系统
- **完整生命周期管理**: 端到端请求监控和分析
- **综合测试覆盖**: 新增25+综合测试文件
- **统一请求处理架构**: 流式v2和统一v2请求处理架构

## [1.0.3] - 2025-09-09

### 🐛 修复 (Fixed)
- **Token解析重复处理**: 修复Token解析重复处理导致成本计算错误的严重bug
- **状态系统增强**: 增强请求状态粒度，提供更好的用户体验

### 📈 改进 (Improved)
- **错误处理**: 改进错误处理和状态追踪
- **用户体验**: 优化状态反馈机制

## [1.0.2] - 2025-09-10

### 🔧 优化 (Optimized)
- **Web处理器重构**: 11个专门处理器文件的模块化架构
  - `dashboard_handlers.go` (312行) - 仪表板数据处理
  - `endpoint_handlers.go` (421行) - 端点状态和监控
  - `connection_handlers.go` (203行) - 连接管理
  - 其他专门处理器...
- **JavaScript模块化**: 现代JavaScript模块系统，提高可维护性
- **SQLite优化**: 数据库操作优化和性能提升
- **部署简化**: 单一二进制文件，零外部依赖
- **性能保证**: SQLite功能完全保持不变，性能优化

## [1.0.1] - 2025-09-08

### 🔧 修复 (Fixed)
- **Token解析兼容性**: 解决标准Claude API格式Token解析失败问题
- **格式兼容性**: 增强SSE事件格式兼容性，支持`event:`和`event: `两种格式
- **数据行解析**: 修复`data:`和`data: `格式兼容性问题  
- **Token提取增强**: 改进`parseMessageStart`函数，支持从`message_start`事件中提取token使用信息

### 📈 改进 (Improved)
- **调试日志**: 提供更详细的Token解析状态和错误信息
- **API兼容性**: 支持更多不同格式的Claude API端点
- **数据准确性**: Token使用统计更加准确和完整
- **nonocode端点兼容性**: 修复nonocode等使用标准Claude API格式的端点Token信息无法解析的问题

## [1.0.0] - 2025-09-05

### 🚀 新增 (Added)
- **Web管理界面**: 现代化Web界面，支持实时监控和组管理
- **请求ID追踪**: 完整的请求生命周期追踪系统，格式为`req-xxxxxxxx`
- **Token解析器**: Claude API SSE流中的模型信息和Token使用量提取
- **请求挂起系统**: 智能请求挂起和恢复机制
- **手动组切换**: 支持手动暂停/恢复/激活组操作
- **实时数据流**: Server-Sent Events (SSE) 实时更新
- **使用情况追踪**: SQLite数据库使用统计和成本分析

### 🏗️ 架构 (Architecture)
- **模块化设计**: 清晰的模块分离和组织
- **RESTful API**: 完整的REST API支持
- **实时监控**: 基于SSE的实时数据更新
- **数据持久化**: SQLite数据库集成

### 📊 监控功能 (Monitoring)
- **端点健康监控**: 实时端点状态追踪
- **请求统计**: 详细的请求和响应统计
- **成本分析**: Token使用成本计算和分析
- **性能指标**: 响应时间和成功率监控

## [0.x.x] - 基础版本

基于 [xinhai-ai/endpoint_forwarder](https://github.com/xinhai-ai/endpoint_forwarder) 的原始功能：

### 📦 基础功能 (Core Features)
- **透明代理**: 透明转发所有HTTP请求到后端端点
- **SSE流式支持**: 完整支持Server-Sent Events流式传输
- **Token管理**: 每个端点可配置独立的Bearer Token
- **路由策略**: 支持优先级路由和最快响应路由
- **健康检查**: 自动端点健康监控
- **重试与故障转移**: 指数退避重试和自动端点故障转移
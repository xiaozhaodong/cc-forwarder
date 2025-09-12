# Changelog

所有项目的重要更改都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
项目遵循 [语义化版本控制](https://semver.org/lang/zh-CN/)。

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
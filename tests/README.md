# 测试文档 - Claude Request Forwarder

## 📁 测试目录结构

```
tests/
├── unit/                          # 单元测试
│   ├── config/                   # 配置模块测试
│   │   └── request_suspend_test.go
│   ├── endpoint/                 # 端点管理测试
│   │   └── group_notification_test.go
│   ├── monitor/                  # 监控系统测试
│   │   └── suspend_test.go
│   └── proxy/                    # 代理层测试
│       └── suspend_test.go
├── integration/                  # 集成测试
│   └── request_suspend/          # 请求挂起功能集成测试
│       ├── comprehensive_integration_test.go    # 综合集成测试
│       ├── functional_verification_test.go     # 功能验证测试
│       ├── performance_test.go                 # 性能测试
│       └── backward_compatibility_test.go      # 向后兼容性测试
├── performance/                  # 性能测试 (未来扩展)
└── testdata/                     # 测试数据文件
```

## 🚀 快速开始

### 运行所有测试
```bash
./scripts/run_tests.sh
```

### 运行特定类型测试
```bash
# 只运行单元测试
./scripts/run_tests.sh --unit

# 只运行集成测试
./scripts/run_tests.sh --integration

# 运行性能测试
./scripts/run_tests.sh --performance

# 生成覆盖率报告
./scripts/run_tests.sh --coverage

# 详细输出
./scripts/run_tests.sh --verbose
```

## 📊 测试覆盖

### 单元测试 (Unit Tests)

#### 1. 配置层测试 (`tests/unit/config/`)
- **request_suspend_test.go**: 
  - 配置默认值设置
  - 配置验证逻辑
  - YAML解析功能
  - 边界条件测试

#### 2. 端点层测试 (`tests/unit/endpoint/`)
- **group_notification_test.go**:
  - 组切换通知机制
  - 订阅/取消订阅功能
  - 并发安全性
  - 内存泄漏防护

#### 3. 监控层测试 (`tests/unit/monitor/`)
- **suspend_test.go**:
  - 挂起请求统计记录
  - 连接状态管理
  - 历史数据生成
  - 图表数据API

#### 4. 代理层测试 (`tests/unit/proxy/`)
- **suspend_test.go**:
  - 挂起判断逻辑
  - 请求计数管理
  - 配置更新处理
  - 错误场景测试

### 集成测试 (Integration Tests)

#### 1. 综合集成测试 (`comprehensive_integration_test.go`)
- 端到端请求挂起流程
- 组切换通知机制验证
- Web API集成测试
- 错误处理和恢复

#### 2. 功能验证测试 (`functional_verification_test.go`)
- 手动模式挂起功能
- 挂起超时处理
- Token统计准确性
- SSE流式请求处理

#### 3. 性能测试 (`performance_test.go`)
- 并发请求处理
- 内存使用监控
- 长时间运行稳定性
- QPS性能验证

#### 4. 向后兼容性测试 (`backward_compatibility_test.go`)
- 功能禁用时的行为
- 配置默认值正确性
- API接口一致性
- 热重载功能

## 📈 性能基准

### 关键操作性能指标
- 配置验证: ~5.7 ns/op (0 allocs/op)
- 挂起判断: ~2.1 ns/op (0 allocs/op)
- 计数获取: ~15.4 ns/op (0 allocs/op)
- 监控记录: ~458 ns/op (0 allocs/op)

### 并发处理能力
- 支持 50+ 并发挂起请求
- QPS: >10 requests/second
- 成功率: 95%+ (正常条件下)

## 🛠️ 开发指南

### 添加新测试

1. **单元测试**:
   ```bash
   # 在对应的 tests/unit/{module}/ 目录下创建
   touch tests/unit/{module}/new_test.go
   ```

2. **集成测试**:
   ```bash
   # 在 tests/integration/ 下创建新的功能目录
   mkdir tests/integration/new_feature/
   touch tests/integration/new_feature/integration_test.go
   ```

### 测试命名规范
- 文件名: `{feature}_{type}_test.go`
- 测试函数: `Test{Module}_{Function}_{Scenario}`
- 基准测试: `Benchmark{Module}_{Operation}`

### 测试标签
使用构建标签来组织不同类型的测试：
```go
//go:build integration
// +build integration

//go:build performance  
// +build performance
```

## 📋 测试清单

### 功能测试检查项
- [ ] 配置解析正确性
- [ ] 默认值设置
- [ ] 验证逻辑完整性
- [ ] 挂起判断准确性
- [ ] 组切换通知及时性
- [ ] 监控数据准确性
- [ ] Web界面API响应
- [ ] 错误处理健壮性

### 性能测试检查项
- [ ] 内存使用稳定
- [ ] CPU占用合理
- [ ] 并发处理能力
- [ ] 响应时间要求
- [ ] 长时间运行稳定性

### 兼容性测试检查项
- [ ] 向后兼容性
- [ ] 配置热重载
- [ ] API接口一致性
- [ ] 功能禁用时行为

## 📊 测试报告

测试执行后会生成以下报告：
- `test_reports/test_summary.txt`: 测试总结
- `test_reports/coverage.html`: 覆盖率报告
- `test_reports/benchmark.log`: 性能基准测试
- `test_reports/{module}_test.log`: 各模块详细日志

## ⚠️ 注意事项

1. **测试隔离**: 每个测试用例应该相互独立，不依赖执行顺序
2. **资源清理**: 确保测试后正确清理临时文件和goroutine
3. **超时设置**: 所有测试都应设置合理的超时时间
4. **并发安全**: 测试本身要注意线程安全，特别是共享状态的测试
5. **错误覆盖**: 不仅测试成功路径，也要测试各种错误场景

## 🚨 故障排除

### 常见问题

1. **测试超时**:
   ```bash
   # 增加超时时间
   go test -timeout 60s ./tests/...
   ```

2. **并发测试失败**:
   ```bash
   # 使用race检测器
   go test -race ./tests/...
   ```

3. **内存泄漏**:
   ```bash
   # 运行内存profiling
   go test -memprofile=mem.prof ./tests/...
   ```

### 调试技巧
- 使用 `-v` 标志获取详细输出
- 使用 `-run` 运行特定测试
- 设置环境变量 `GOMAXPROCS=1` 简化调试

---

**最后更新**: 2025-09-04 10:38:56  
**维护者**: Claude Code Assistant
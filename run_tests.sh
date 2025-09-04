#!/bin/bash

# 请求挂起功能测试运行脚本
# 用于执行完整的测试套件并生成报告

set -e

echo "🧪 Claude Request Forwarder - 请求挂起功能测试套件"
echo "=============================================="
echo

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试结果跟踪
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 运行测试的函数
run_test() {
    local test_name="$1"
    local test_command="$2"
    local timeout="${3:-30s}"
    
    echo -e "${YELLOW}📋 运行测试: $test_name${NC}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if timeout "$timeout" bash -c "$test_command" > "test_${test_name// /_}.log" 2>&1; then
        echo -e "${GREEN}✅ $test_name 通过${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}❌ $test_name 失败${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo "   日志文件: test_${test_name// /_}.log"
    fi
    echo
}

# 创建测试报告目录
mkdir -p test_reports
cd test_reports

echo "🔧 编译验证..."
if ! go build -v ../. > build.log 2>&1; then
    echo -e "${RED}❌ 编译失败，请检查代码${NC}"
    echo "构建日志:"
    cat build.log
    exit 1
fi
echo -e "${GREEN}✅ 编译成功${NC}"
echo

# 基础单元测试
echo "📊 运行基础单元测试..."
run_test "配置层测试" "go test -v ../config/ -timeout 15s"
run_test "代理层测试" "go test -v ../internal/proxy/ -timeout 15s" 
run_test "监控层测试" "go test -v ../internal/monitor/ -timeout 15s"

# 集成测试
echo "🔗 运行集成测试..."
run_test "基本挂起功能" "go test -v ../internal/integration/ -run 'TestRequestSuspendBasic' -timeout 30s"
run_test "简化集成测试" "go test -v ../internal/integration/ -run 'TestRequestSuspendSimplifiedBackwardCompatibility' -timeout 20s"

# 向后兼容性测试  
echo "🔄 运行向后兼容性测试..."
run_test "功能禁用兼容性" "go test -v ../internal/integration/ -run 'TestBackwardCompatibilityDisabledFeature' -timeout 15s"

# 性能测试（可选，因为可能会很长时间）
if [[ "$1" == "--include-performance" ]]; then
    echo "⚡ 运行性能测试..."
    run_test "性能基准测试" "go test -v ../internal/integration/ -run 'TestRequestSuspendPerformance' -timeout 60s"
fi

# 基准测试
echo "🚀 运行基准测试..."
run_test "配置基准测试" "go test -bench=BenchmarkRequestSuspend -benchmem ../config/ -timeout 30s"
run_test "代理基准测试" "go test -bench=BenchmarkRetryHandler -benchmem ../internal/proxy/ -timeout 30s"
run_test "监控基准测试" "go test -bench=BenchmarkMetrics -benchmem ../internal/monitor/ -timeout 30s"

# 生成测试报告
echo "📄 生成测试报告..."

cat > test_summary.txt << EOF
请求挂起功能测试总结报告
==============================

测试时间: $(date)
测试环境: $(go version)
总测试数: $TOTAL_TESTS
通过测试: $PASSED_TESTS  
失败测试: $FAILED_TESTS
成功率: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%

详细结果:
EOF

# 列出所有日志文件
for log_file in test_*.log; do
    if [[ -f "$log_file" ]]; then
        test_name=$(basename "$log_file" .log | sed 's/test_//' | sed 's/_/ /g')
        echo "- $test_name: 详细日志见 $log_file" >> test_summary.txt
    fi
done

# 显示最终结果
echo
echo "🏁 测试完成！"
echo "============="
echo -e "总测试数: ${YELLOW}$TOTAL_TESTS${NC}"
echo -e "通过测试: ${GREEN}$PASSED_TESTS${NC}"
echo -e "失败测试: ${RED}$FAILED_TESTS${NC}"

if [[ $FAILED_TESTS -eq 0 ]]; then
    echo -e "${GREEN}🎉 所有测试通过！${NC}"
    exit 0
else
    echo -e "${RED}⚠️  有 $FAILED_TESTS 个测试失败${NC}"
    echo "请检查相应的日志文件获取详细信息"
    exit 1
fi
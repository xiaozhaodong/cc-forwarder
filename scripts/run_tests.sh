#!/bin/bash

# 测试执行脚本 - Claude Request Forwarder
# 创建时间: 2025-09-04 10:38:56

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 输出带颜色的日志
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# 显示帮助信息
show_help() {
    echo "测试执行脚本 - Claude Request Forwarder"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --unit                只运行单元测试"
    echo "  --integration        只运行集成测试"
    echo "  --performance        运行性能测试"
    echo "  --coverage           生成测试覆盖率报告"
    echo "  --verbose            显示详细输出"
    echo "  --help               显示此帮助信息"
    echo ""
}

# 默认参数
RUN_UNIT=true
RUN_INTEGRATION=true
RUN_PERFORMANCE=false
RUN_COVERAGE=false
VERBOSE=false

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --unit)
            RUN_UNIT=true
            RUN_INTEGRATION=false
            RUN_PERFORMANCE=false
            shift
            ;;
        --integration)
            RUN_UNIT=false
            RUN_INTEGRATION=true
            RUN_PERFORMANCE=false
            shift
            ;;
        --performance)
            RUN_PERFORMANCE=true
            shift
            ;;
        --coverage)
            RUN_COVERAGE=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            log_error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 创建测试报告目录
REPORT_DIR="$PROJECT_ROOT/test_reports"
mkdir -p "$REPORT_DIR"

log_info "开始执行测试套件..."
log_info "项目路径: $PROJECT_ROOT"

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 运行测试的函数
run_test_package() {
    local package=$1
    local name=$2
    
    log_info "运行 $name 测试..."
    
    if [[ $VERBOSE == true ]]; then
        if go test -v "$package" -timeout 30s; then
            log_success "$name 测试通过"
            ((PASSED_TESTS++))
        else
            log_error "$name 测试失败"
            ((FAILED_TESTS++))
        fi
    else
        if go test "$package" -timeout 30s > "$REPORT_DIR/${name}_test.log" 2>&1; then
            log_success "$name 测试通过"
            ((PASSED_TESTS++))
        else
            log_error "$name 测试失败"
            cat "$REPORT_DIR/${name}_test.log"
            ((FAILED_TESTS++))
        fi
    fi
    ((TOTAL_TESTS++))
}

# 1. 运行单元测试
if [[ $RUN_UNIT == true ]]; then
    log_info "=== 单元测试 ==="
    
    # 配置层测试
    run_test_package "./tests/unit/config" "配置层"
    
    # 监控层测试  
    run_test_package "./tests/unit/monitor" "监控层"
    
    # 代理层测试
    run_test_package "./tests/unit/proxy" "代理层"
    
    # 端点层测试
    run_test_package "./tests/unit/endpoint" "端点层"
    
    # 运行原有的单元测试（保持兼容性）
    if [[ -f "config/config_test.go" ]]; then
        run_test_package "./config" "配置原有"
    fi
    
    if [[ -f "internal/proxy/token_parser_test.go" ]]; then
        run_test_package "./internal/proxy" "代理原有"
    fi
    
    if [[ -f "internal/endpoint/manager_test.go" ]]; then
        run_test_package "./internal/endpoint" "端点原有"
    fi
fi

# 2. 运行集成测试
if [[ $RUN_INTEGRATION == true ]]; then
    log_info "=== 集成测试 ==="
    
    # 请求挂起集成测试
    run_test_package "./tests/integration/request_suspend" "请求挂起集成"
fi

# 3. 运行性能测试
if [[ $RUN_PERFORMANCE == true ]]; then
    log_info "=== 性能测试 ==="
    
    log_info "运行基准测试..."
    go test -bench=. -benchmem ./tests/unit/... > "$REPORT_DIR/benchmark.log" 2>&1
    
    if [[ $? -eq 0 ]]; then
        log_success "性能测试完成"
        if [[ $VERBOSE == true ]]; then
            cat "$REPORT_DIR/benchmark.log"
        fi
    else
        log_warning "性能测试失败"
        cat "$REPORT_DIR/benchmark.log"
    fi
fi

# 4. 生成测试覆盖率
if [[ $RUN_COVERAGE == true ]]; then
    log_info "=== 测试覆盖率 ==="
    
    log_info "生成覆盖率报告..."
    go test -coverprofile="$REPORT_DIR/coverage.out" ./... > "$REPORT_DIR/coverage.log" 2>&1
    
    if [[ $? -eq 0 ]]; then
        go tool cover -html="$REPORT_DIR/coverage.out" -o "$REPORT_DIR/coverage.html"
        go tool cover -func="$REPORT_DIR/coverage.out" > "$REPORT_DIR/coverage_summary.txt"
        
        log_success "覆盖率报告已生成: $REPORT_DIR/coverage.html"
        
        # 显示覆盖率摘要
        if [[ $VERBOSE == true ]]; then
            cat "$REPORT_DIR/coverage_summary.txt"
        fi
    else
        log_warning "覆盖率生成失败"
        cat "$REPORT_DIR/coverage.log"
    fi
fi

# 5. 生成测试总结报告
generate_summary() {
    local summary_file="$REPORT_DIR/test_summary.txt"
    
    cat > "$summary_file" << EOF
===========================================
    Claude Request Forwarder 测试报告
===========================================

执行时间: $(date '+%Y-%m-%d %H:%M:%S')
项目路径: $PROJECT_ROOT

测试结果统计:
- 总测试模块: $TOTAL_TESTS
- 通过测试: $PASSED_TESTS
- 失败测试: $FAILED_TESTS
- 成功率: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%

测试配置:
- 单元测试: $(if [[ $RUN_UNIT == true ]]; then echo "✅"; else echo "❌"; fi)
- 集成测试: $(if [[ $RUN_INTEGRATION == true ]]; then echo "✅"; else echo "❌"; fi)
- 性能测试: $(if [[ $RUN_PERFORMANCE == true ]]; then echo "✅"; else echo "❌"; fi)
- 覆盖率报告: $(if [[ $RUN_COVERAGE == true ]]; then echo "✅"; else echo "❌"; fi)

测试文件位置:
- 单元测试: tests/unit/
- 集成测试: tests/integration/
- 性能测试: tests/performance/
- 测试报告: test_reports/

===========================================
EOF

    log_success "测试总结报告已生成: $summary_file"
}

generate_summary

# 显示最终结果
echo ""
log_info "=== 测试完成 ==="

if [[ $FAILED_TESTS -eq 0 ]]; then
    log_success "所有测试通过! ($PASSED_TESTS/$TOTAL_TESTS)"
    exit 0
else
    log_error "有 $FAILED_TESTS 个测试失败! ($PASSED_TESTS/$TOTAL_TESTS)"
    exit 1
fi
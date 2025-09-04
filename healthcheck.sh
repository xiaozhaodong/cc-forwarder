#!/bin/bash

# 健康检查脚本
# 用于验证服务是否正常运行

set -e

# 配置
PROXY_URL="http://localhost:8087"
WEB_URL="http://localhost:8088"
HEALTH_ENDPOINT="/health"
TIMEOUT=10

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "[INFO] $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 检查代理服务健康状态
check_proxy_health() {
    log_info "检查代理服务健康状态..."
    
    if curl -f -s --max-time $TIMEOUT "${PROXY_URL}${HEALTH_ENDPOINT}" > /dev/null; then
        log_success "代理服务 ($PROXY_URL) 健康检查通过"
        return 0
    else
        log_error "代理服务 ($PROXY_URL) 健康检查失败"
        return 1
    fi
}

# 检查Web界面
check_web_interface() {
    log_info "检查Web界面..."
    
    if curl -f -s --max-time $TIMEOUT "$WEB_URL" > /dev/null; then
        log_success "Web界面 ($WEB_URL) 访问正常"
        return 0
    else
        log_warning "Web界面 ($WEB_URL) 可能未启用或无法访问"
        return 1
    fi
}

# 检查容器状态
check_container_status() {
    log_info "检查容器状态..."
    
    if docker-compose ps | grep -q "Up"; then
        log_success "容器运行状态正常"
        return 0
    else
        log_error "容器状态异常"
        docker-compose ps
        return 1
    fi
}

# 显示详细状态
show_detailed_status() {
    echo "========== 详细状态信息 =========="
    
    echo ""
    echo "容器状态:"
    docker-compose ps
    
    echo ""
    echo "服务端点:"
    echo "  代理服务: $PROXY_URL"
    echo "  Web界面:  $WEB_URL"
    echo "  健康检查: ${PROXY_URL}${HEALTH_ENDPOINT}"
    
    echo ""
    echo "最近日志 (最后10行):"
    docker-compose logs --tail=10 endpoint-forwarder
    
    echo "=================================="
}

# 主函数
main() {
    local overall_status=0
    
    echo "Claude Request Forwarder 健康检查"
    echo "================================="
    
    # 检查容器状态
    if ! check_container_status; then
        overall_status=1
    fi
    
    # 检查代理服务
    if ! check_proxy_health; then
        overall_status=1
    fi
    
    # 检查Web界面 (可选)
    check_web_interface || true
    
    echo ""
    if [ $overall_status -eq 0 ]; then
        log_success "所有核心服务健康检查通过"
    else
        log_error "部分服务健康检查失败"
    fi
    
    # 如果有参数 --detailed 则显示详细状态
    if [[ "${1:-}" == "--detailed" || "${1:-}" == "-d" ]]; then
        show_detailed_status
    fi
    
    exit $overall_status
}

# 执行主函数
main "$@"
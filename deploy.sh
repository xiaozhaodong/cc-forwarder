#!/bin/bash

# Claude Request Forwarder Docker 部署脚本
# 
# 使用方法:
#   ./deploy.sh build    - 构建Docker镜像
#   ./deploy.sh up       - 启动服务
#   ./deploy.sh down     - 停止服务
#   ./deploy.sh restart  - 重启服务
#   ./deploy.sh logs     - 查看日志
#   ./deploy.sh clean    - 清理镜像和容器

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目信息
PROJECT_NAME="endpoint-forwarder"
IMAGE_NAME="endpoint-forwarder:latest"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查Docker环境
check_docker() {
    log_info "检查Docker环境..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker未安装或未在PATH中找到"
        exit 1
    fi
    
    if ! docker info &> /dev/null; then
        log_error "Docker守护进程未运行或权限不足"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose未安装或未在PATH中找到"
        exit 1
    fi
    
    log_success "Docker环境检查通过"
}

# 初始化配置
init_config() {
    log_info "初始化配置文件..."
    
    # 创建必要的目录
    mkdir -p config logs
    
    # 检查配置文件
    if [ ! -f "config/config.yaml" ]; then
        if [ -f "config/example.yaml" ]; then
            cp config/example.yaml config/config.yaml
            log_warning "已从example.yaml创建config.yaml，请根据需要修改配置"
        else
            log_error "未找到配置文件模板 config/example.yaml"
            exit 1
        fi
    else
        log_info "配置文件 config/config.yaml 已存在"
    fi
}

# 构建镜像
build_image() {
    log_info "开始构建Docker镜像..."
    
    # 检查Dockerfile
    if [ ! -f "Dockerfile" ]; then
        log_error "未找到Dockerfile"
        exit 1
    fi
    
    # 构建镜像
    docker build -t $IMAGE_NAME . --no-cache
    
    log_success "Docker镜像构建完成: $IMAGE_NAME"
}

# 启动服务
start_services() {
    log_info "启动服务..."
    init_config
    
    # 使用docker-compose启动服务
    docker-compose up -d
    
    # 等待服务启动
    log_info "等待服务启动..."
    sleep 5
    
    # 检查服务状态
    if docker-compose ps | grep -q "Up"; then
        log_success "服务启动成功!"
        log_info "代理服务地址: http://localhost:8087"
        log_info "Web界面地址: http://localhost:8088"
        log_info "使用 './deploy.sh logs' 查看日志"
    else
        log_error "服务启动失败，请检查日志"
        docker-compose logs
    fi
}

# 停止服务
stop_services() {
    log_info "停止服务..."
    docker-compose down
    log_success "服务已停止"
}

# 重启服务
restart_services() {
    log_info "重启服务..."
    stop_services
    sleep 2
    start_services
}

# 查看日志
show_logs() {
    log_info "显示服务日志..."
    docker-compose logs -f --tail=50
}

# 清理资源
clean_resources() {
    log_warning "这将删除所有相关的Docker镜像和容器，是否继续? (y/N)"
    read -r response
    
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        log_info "清理Docker资源..."
        
        # 停止并删除容器
        docker-compose down --rmi all --volumes --remove-orphans
        
        # 删除相关镜像
        docker rmi $IMAGE_NAME 2>/dev/null || true
        
        # 清理构建缓存
        docker builder prune -f
        
        log_success "清理完成"
    else
        log_info "取消清理操作"
    fi
}

# 显示帮助信息
show_help() {
    echo "Claude Request Forwarder Docker 部署脚本"
    echo ""
    echo "使用方法: $0 [命令]"
    echo ""
    echo "可用命令:"
    echo "  build    - 构建Docker镜像"
    echo "  up       - 启动服务 (包含配置初始化)"
    echo "  down     - 停止服务"
    echo "  restart  - 重启服务"
    echo "  logs     - 查看实时日志"
    echo "  clean    - 清理Docker镜像和容器"
    echo "  help     - 显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 build && $0 up     # 构建并启动服务"
    echo "  $0 logs               # 查看服务日志"
    echo "  $0 restart            # 重启服务"
}

# 主函数
main() {
    case "${1:-}" in
        "build")
            check_docker
            build_image
            ;;
        "up")
            check_docker
            start_services
            ;;
        "down")
            check_docker
            stop_services
            ;;
        "restart")
            check_docker
            restart_services
            ;;
        "logs")
            check_docker
            show_logs
            ;;
        "clean")
            check_docker
            clean_resources
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        "")
            log_error "请指定一个命令"
            show_help
            exit 1
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
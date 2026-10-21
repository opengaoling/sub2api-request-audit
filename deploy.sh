#!/usr/bin/env bash
#=============================================================================
# 🚀 Sub2API Request Audit · 一键部署脚本 (内置 Caddy 自动申请与自动续期 SSL 证书)
# 项目地址: https://github.com/opengaoling/sub2api-request-audit
# 支持系统: Ubuntu / Debian / CentOS / AlmaLinux / Rocky / Fedora / Alpine
#=============================================================================

set -o pipefail

# ANSI 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# 全局默认参数与版本号
SCRIPT_VERSION="v1.0.0"
DEFAULT_INSTALL_DIR="/opt/sub2api"
DEFAULT_PORT="8080"
DOCKER_IMAGE="ghcr.io/opengaoling/sub2api-request-audit:latest"
POSTGRES_IMAGE="postgres:18-alpine"
REDIS_IMAGE="redis:8-alpine"
CADDY_IMAGE="caddy:2.8.4-alpine"

log_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
log_ok()   { echo -e "${GREEN}[ OK ]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_err()  { echo -e "${RED}[ERR ]${NC} $1"; }

print_banner() {
    clear 2>/dev/null || true
    echo -e "${CYAN}${BOLD}"
    echo "============================================================================"
    echo "       🚀 Sub2API Request Audit · 一键部署脚本 (${SCRIPT_VERSION})       "
    echo "         基于 Sub2API · AI API 网关 · 统一账号调度 · 请求审计治理         "
    echo "          Caddy 自动化 HTTPS: 自动申请 Let's Encrypt / ZeroSSL 证书并自动续期   "
    echo "============================================================================"
    echo -e "${NC}"
}

check_root() {
    if [ "$(id -u)" != "0" ]; then
        log_err "请使用 root 权限或 sudo 运行此部署脚本。"
        exit 1
    fi
}

# 生成随机十六进制密钥
generate_hex_secret() {
    local len="${1:-32}"
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex "$len" 2>/dev/null
    else
        tr -dc 'a-f0-9' < /dev/urandom 2>/dev/null | head -c "$((len * 2))"
    fi
}

# 生成随机密码
generate_random_password() {
    local len="${1:-16}"
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 24 2>/dev/null | tr -dc 'a-zA-Z0-9' | head -c "$len"
    else
        tr -dc 'a-zA-Z0-9' < /dev/urandom 2>/dev/null | head -c "$len"
    fi
}

# 获取公网 IP (用于无域名提示)
get_public_ip() {
    local ip=""
    ip="$(curl -s4 --max-time 3 https://api.ipify.org 2>/dev/null)" || \
    ip="$(curl -s4 --max-time 3 https://ifconfig.me 2>/dev/null)" || \
    ip="$(curl -s4 --max-time 3 https://icanhazip.com 2>/dev/null)" || \
    ip="127.0.0.1"
    echo "$ip" | tr -d ' \n\r'
}

# 检查并安装基础依赖
install_dependencies() {
    log_info "正在检查基础运行依赖 (curl, tar, openssl, awk, sed)..."
    local need_install=""
    for cmd in curl tar openssl awk sed; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            need_install="$need_install $cmd"
        fi
    done

    if [ -n "$need_install" ]; then
        log_info "正在安装缺失的依赖:$need_install"
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -qq && apt-get install -y -qq $need_install
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y -q $need_install
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q $need_install
        elif command -v apk >/dev/null 2>&1; then
            apk add --no-cache $need_install
        fi
    fi
}

# 检查并安装 Docker 与 Docker Compose
check_install_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        log_warn "未检测到 Docker 环境，正在使用官方脚本自动安装 Docker..."
        curl -fsSL https://get.docker.com | sh
        systemctl enable --now docker 2>/dev/null || service docker start 2>/dev/null || true
    fi

    # 检查 docker compose 命令兼容性
    local has_compose=false
    if docker compose version >/dev/null 2>&1; then
        has_compose=true
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        has_compose=true
        COMPOSE_CMD="docker-compose"
    fi

    if [ "$has_compose" != "true" ]; then
        log_info "正在配置 Docker Compose 插件..."
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -qq && apt-get install -y -qq docker-compose-v2 || apt-get install -y -qq docker-compose-plugin || true
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y -q docker-compose-plugin || true
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q docker-compose-plugin || true
        fi

        if docker compose version >/dev/null 2>&1; then
            COMPOSE_CMD="docker compose"
        elif command -v docker-compose >/dev/null 2>&1; then
            COMPOSE_CMD="docker-compose"
        else
            log_info "正在直接下载官方 Docker Compose 可执行插件..."
            local arch="$(uname -m)"
            case "$arch" in
                x86_64) arch="x86_64" ;;
                aarch64|arm64) arch="aarch64" ;;
                *) arch="x86_64" ;;
            esac
            mkdir -p /usr/local/lib/docker/cli-plugins /usr/libexec/docker/cli-plugins
            curl -SL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${arch}" -o /usr/local/lib/docker/cli-plugins/docker-compose
            chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
            cp /usr/local/lib/docker/cli-plugins/docker-compose /usr/local/bin/docker-compose 2>/dev/null || true
            COMPOSE_CMD="docker compose"
        fi
    fi

    log_ok "Docker 与 Docker Compose 环境已就绪 ($(docker --version 2>/dev/null | head -n1))"
}

# 交互式收集配置
gather_user_input() {
    echo -e "${YELLOW}------------------------------------------------------------${NC}"
    echo -e "${BOLD}请配置部署参数（直接回车可使用默认建议值）：${NC}"
    echo -e "${YELLOW}------------------------------------------------------------${NC}"

    # 1. 域名配置
    while true; do
        if [ -n "$DOMAIN" ]; then
            INPUT_DOMAIN="$DOMAIN"
        else
            if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
                INPUT_DOMAIN=""
            else
                read -r -p "👉 请输入要绑定的域名 (例如: api.example.com，回车跳过使用 HTTP 端口模式): " INPUT_DOMAIN
            fi
        fi
        INPUT_DOMAIN="$(echo "$INPUT_DOMAIN" | tr -d ' ')"

        if [ -z "$INPUT_DOMAIN" ] || [ "$INPUT_DOMAIN" = "none" ] || [ "$INPUT_DOMAIN" = "skip" ] || [ "$INPUT_DOMAIN" = "no" ]; then
            INPUT_DOMAIN=""
            ENABLE_SSL="false"
            break
        elif [[ "$INPUT_DOMAIN" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$ ]]; then
            ENABLE_SSL="true"
            break
        else
            log_err "域名格式不正确，请输入合法域名（如 api.example.com）或直接回车跳过："
            DOMAIN=""
        fi
    done

    # 2. 邮箱配置 (用于 Let's Encrypt / ZeroSSL 到期告警通知)
    if [ "$ENABLE_SSL" = "true" ]; then
        if [ -z "$EMAIL" ]; then
            if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
                INPUT_EMAIL=""
            else
                read -r -p "👉 请输入联系邮箱 (用于证书续期通知，建议填写，回车跳过): " INPUT_EMAIL
                INPUT_EMAIL="$(echo "$INPUT_EMAIL" | tr -d ' ')"
            fi
        else
            INPUT_EMAIL="$EMAIL"
        fi
    else
        INPUT_EMAIL=""
    fi

    # 3. 访问端口
    if [ -z "$PORT" ]; then
        if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
            INPUT_PORT="$DEFAULT_PORT"
        else
            read -r -p "👉 请输入服务访问端口 (默认: $DEFAULT_PORT, 回车确认): " INPUT_PORT
            INPUT_PORT="$(echo "$INPUT_PORT" | tr -d ' ')"
            [ -z "$INPUT_PORT" ] && INPUT_PORT="$DEFAULT_PORT"
        fi
    else
        INPUT_PORT="$PORT"
    fi

    # 4. 管理员初始账号 (ADMIN_EMAIL)
    local default_admin_email="admin@sub2api.local"
    if [ -z "$ADMIN_EMAIL" ]; then
        if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
            INPUT_ADMIN_EMAIL="$default_admin_email"
        else
            read -r -p "👉 请输入管理员邮箱 (默认: ${default_admin_email}, 回车确认): " INPUT_ADMIN_EMAIL
            INPUT_ADMIN_EMAIL="$(echo "$INPUT_ADMIN_EMAIL" | tr -d ' ')"
            [ -z "$INPUT_ADMIN_EMAIL" ] && INPUT_ADMIN_EMAIL="$default_admin_email"
        fi
    else
        INPUT_ADMIN_EMAIL="$ADMIN_EMAIL"
    fi

    # 5. 管理员初始密码 (ADMIN_PASSWORD)
    local auto_admin_pwd="$(generate_random_password 16)"
    if [ -z "$ADMIN_PASSWORD" ]; then
        if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
            INPUT_ADMIN_PASSWORD="$auto_admin_pwd"
        else
            read -r -p "👉 请输入管理员初始密码 (默认随机: ${auto_admin_pwd}, 回车确认): " INPUT_ADMIN_PASSWORD
            INPUT_ADMIN_PASSWORD="$(echo "$INPUT_ADMIN_PASSWORD" | tr -d ' ')"
            [ -z "$INPUT_ADMIN_PASSWORD" ] && INPUT_ADMIN_PASSWORD="$auto_admin_pwd"
        fi
    else
        INPUT_ADMIN_PASSWORD="$ADMIN_PASSWORD"
    fi

    # 6. 安装目录
    if [ -z "$INSTALL_DIR" ]; then
        if [ "$NON_INTERACTIVE" = "true" ] || [ "$NON_INTERACTIVE" = "1" ]; then
            INPUT_DIR="$DEFAULT_INSTALL_DIR"
        else
            read -r -p "👉 请输入安装目录路径 (默认: $DEFAULT_INSTALL_DIR, 回车确认): " INPUT_DIR
            INPUT_DIR="$(echo "$INPUT_DIR" | tr -d ' ')"
            [ -z "$INPUT_DIR" ] && INPUT_DIR="$DEFAULT_INSTALL_DIR"
        fi
    else
        INPUT_DIR="$INSTALL_DIR"
    fi

    echo ""
    log_info "配置确认清单:"
    if [ "$ENABLE_SSL" = "true" ]; then
        echo "  - 部署域名:     ${GREEN}https://${INPUT_DOMAIN}${NC}"
        echo "  - 证书通知邮箱: ${GREEN}${INPUT_EMAIL:-未设置 (Caddy 自动申请)}${NC}"
        echo "  - 证书管理方案: ${GREEN}Caddy 自动申请 Let's Encrypt / ZeroSSL 证书并全自动静默续期${NC}"
        echo "  - 端口映射:     ${GREEN}80 / 443 (HTTP 自动强制跳转 HTTPS)${NC}"
    else
        local server_ip="$(get_public_ip)"
        echo "  - 访问地址:     ${GREEN}http://${server_ip}:${INPUT_PORT}${NC}"
        echo "  - 运行模式:     ${YELLOW}HTTP 端口模式 (未配置域名 SSL)${NC}"
        echo "  - 监听端口:     ${GREEN}${INPUT_PORT}${NC}"
    fi
    echo "  - 管理员账号:   ${GREEN}${INPUT_ADMIN_EMAIL}${NC}"
    echo "  - 管理员密码:   ${GREEN}${INPUT_ADMIN_PASSWORD}${NC}"
    echo "  - 安装目录:     ${GREEN}${INPUT_DIR}${NC}"
    echo "  - 核心架构:     ${GREEN}Sub2API + PostgreSQL 18 + Redis 8 + Caddy 反向代理${NC}"
    echo ""
    if [ "$NON_INTERACTIVE" != "true" ] && [ "$NON_INTERACTIVE" != "1" ]; then
        read -r -p "确认以上配置并开始部署? [Y/n]: " confirm
        if [[ "$confirm" =~ ^[nN]$ ]]; then
            log_warn "用户取消操作，退出脚本。"
            exit 0
        fi
    fi
}

# 检查端口占用
check_ports() {
    log_info "正在检查端口占用情况..."
    local check_list=()
    if [ "$ENABLE_SSL" = "true" ]; then
        check_list=(80 443)
    else
        check_list=("$INPUT_PORT")
    fi

    local conflict=false
    for p in "${check_list[@]}"; do
        if ss -tuln 2>/dev/null | grep -q ":$p " || netstat -tuln 2>/dev/null | grep -q ":$p "; then
            if [ -d "$INPUT_DIR" ] && [ -f "$INPUT_DIR/docker-compose.yaml" ]; then
                log_warn "检测到端口 $p 已在使用，正在检查是否为本服务..."
            else
                log_err "端口 $p 已被其他程序占用，请先停止占用该端口的服务 (例如 Nginx/Apache) 后再试。"
                conflict=true
            fi
        fi
    done

    if [ "$conflict" = "true" ]; then
        read -r -p "是否尝试强行继续部署? [y/N]: " force_continue
        if ! [[ "$force_continue" =~ ^[yY]$ ]]; then
            exit 1
        fi
    fi
}

# 自动放行防火墙端口
open_firewall_ports() {
    log_info "正在配置系统防火墙放行必要通信端口..."
    local port_list=()
    if [ "$ENABLE_SSL" = "true" ]; then
        port_list=(80 443)
    else
        port_list=("$INPUT_PORT")
    fi

    for p in "${port_list[@]}"; do
        if command -v iptables >/dev/null 2>&1; then
            iptables -I INPUT 1 -p tcp --dport "$p" -j ACCEPT 2>/dev/null || true
            [ "$p" = "443" ] && iptables -I INPUT 1 -p udp --dport 443 -j ACCEPT 2>/dev/null || true
        fi
        if command -v ufw >/dev/null 2>&1; then
            ufw allow "$p"/tcp >/dev/null 2>&1 || true
            [ "$p" = "443" ] && ufw allow 443/udp >/dev/null 2>&1 || true
        fi
        if command -v firewall-cmd >/dev/null 2>&1; then
            firewall-cmd --permanent --add-port="$p"/tcp >/dev/null 2>&1 || true
            [ "$p" = "443" ] && firewall-cmd --permanent --add-port=443/udp >/dev/null 2>&1 || true
            firewall-cmd --reload >/dev/null 2>&1 || true
        fi
    done

    if command -v netfilter-persistent >/dev/null 2>&1; then
        netfilter-persistent save >/dev/null 2>&1 || true
    fi
}

# 写入部署配置文件
write_configs() {
    log_info "正在初始化部署目录: $INPUT_DIR"
    mkdir -p "$INPUT_DIR" "$INPUT_DIR/data" "$INPUT_DIR/postgres_data" "$INPUT_DIR/redis_data"
    chmod 700 "$INPUT_DIR/postgres_data" "$INPUT_DIR/redis_data" 2>/dev/null || true
    cd "$INPUT_DIR"

    # 生成固定随机密钥（如果未提供）
    local postgres_pwd="${POSTGRES_PASSWORD:-$(generate_hex_secret 16)}"
    local redis_pwd="${REDIS_PASSWORD:-$(generate_hex_secret 16)}"
    local jwt_secret="${JWT_SECRET:-$(generate_hex_secret 32)}"
    local totp_key="${TOTP_ENCRYPTION_KEY:-$(generate_hex_secret 32)}"

    # 1. 写入 Caddyfile
    log_info "正在生成 Caddy 配置文件..."
    if [ "$ENABLE_SSL" = "true" ]; then
        cat > "$INPUT_DIR/Caddyfile" <<EOF
#=============================================================================
# Caddy 自动化 HTTPS 配置文件
# 自动向 Let's Encrypt / ZeroSSL 申请证书并在后台定时自动续期
# 自动启用 HTTP->HTTPS 强制跳转、TLS 1.3 及 HTTP/3 (QUIC)
# 针对 AI 模型流式传输 (SSE) 避免响应缓冲
#=============================================================================
{
    admin off
}

${INPUT_DOMAIN} {
$( [ -n "$INPUT_EMAIL" ] && [ "$INPUT_EMAIL" != "0" ] && echo "    tls ${INPUT_EMAIL}" )

    # 静态资源优化
    @static {
        path /assets/*
        path /logo.png
        path /favicon.ico
    }
    header @static {
        Cache-Control "public, max-age=31536000, immutable"
        -Pragma
        -Expires
    }

    # 智能高效压缩（排除 text/event-stream 避免流式数据被缓冲）
    encode zstd gzip {
        match {
            header Content-Type text/css*
            header Content-Type text/csv*
            header Content-Type text/html*
            header Content-Type text/javascript*
            header Content-Type text/markdown*
            header Content-Type text/plain*
            header Content-Type text/xml*
            header Content-Type application/json*
            header Content-Type application/javascript*
            header Content-Type application/xml*
            header Content-Type application/rss+xml*
            header Content-Type image/svg+xml*
        }
    }

    # 请求大小限制 (大模型长上下文/多模态图像/文件上传，支持至 256MB)
    request_body {
        max_size 256MB
    }

    # 反向代理至 Sub2API 服务
    reverse_proxy sub2api:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
        header_up X-Forwarded-Host {host}
        header_up CF-Connecting-IP {http.request.header.CF-Connecting-IP}

        # 针对 AI 流式传输 (SSE) 与长连接的优化
        transport http {
            keepalive 120s
            keepalive_idle_conns 256
            read_buffer 16KB
            write_buffer 16KB
            compression off
        }
    }

    log {
        output stdout
        format console
        level INFO
    }
}
EOF
    else
        cat > "$INPUT_DIR/Caddyfile" <<EOF
#=============================================================================
# Caddy HTTP 端口反向代理配置文件
#=============================================================================
{
    admin off
}

:${INPUT_PORT} {
    @static {
        path /assets/*
        path /logo.png
        path /favicon.ico
    }
    header @static {
        Cache-Control "public, max-age=31536000, immutable"
        -Pragma
        -Expires
    }

    encode zstd gzip {
        match {
            header Content-Type text/css*
            header Content-Type text/csv*
            header Content-Type text/html*
            header Content-Type text/javascript*
            header Content-Type text/markdown*
            header Content-Type text/plain*
            header Content-Type text/xml*
            header Content-Type application/json*
            header Content-Type application/javascript*
            header Content-Type application/xml*
            header Content-Type application/rss+xml*
            header Content-Type image/svg+xml*
        }
    }

    request_body {
        max_size 256MB
    }

    reverse_proxy sub2api:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
        header_up X-Forwarded-Host {host}
        header_up CF-Connecting-IP {http.request.header.CF-Connecting-IP}

        transport http {
            keepalive 120s
            keepalive_idle_conns 256
            read_buffer 16KB
            write_buffer 16KB
            compression off
        }
    }

    log {
        output stdout
        format console
        level INFO
    }
}
EOF
    fi

    # 2. 写入 docker-compose.yaml
    log_info "正在生成 docker-compose.yaml 文件..."
    if [ "$ENABLE_SSL" = "true" ]; then
        cat > "$INPUT_DIR/docker-compose.yaml" <<EOF
services:
  sub2api:
    image: ${DOCKER_IMAGE}
    container_name: sub2api
    restart: always
    security_opt:
      - no-new-privileges:true
    ulimits:
      nofile:
        soft: 100000
        hard: 100000
    environment:
      - AUTO_SETUP=true
      - SERVER_HOST=0.0.0.0
      - SERVER_PORT=8080
      - SERVER_MODE=release
      - RUN_MODE=standard
      - DATABASE_HOST=postgres
      - DATABASE_PORT=5432
      - DATABASE_USER=sub2api
      - DATABASE_PASSWORD=${postgres_pwd}
      - DATABASE_DBNAME=sub2api
      - DATABASE_SSLMODE=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - REDIS_PASSWORD=${redis_pwd}
      - ADMIN_EMAIL=${INPUT_ADMIN_EMAIL}
      - ADMIN_PASSWORD=${INPUT_ADMIN_PASSWORD}
      - JWT_SECRET=${jwt_secret}
      - TOTP_ENCRYPTION_KEY=${totp_key}
      - TZ=Asia/Shanghai
      - SERVER_MAX_REQUEST_BODY_SIZE=268435456
      - GATEWAY_MAX_BODY_SIZE=268435456
      - SERVER_H2C_ENABLED=true
    volumes:
      - ./data:/app/data
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
    networks:
      - sub2api-net

  postgres:
    image: ${POSTGRES_IMAGE}
    container_name: sub2api-postgres
    restart: always
    environment:
      - PGDATA=/var/lib/postgresql/data
      - POSTGRES_USER=sub2api
      - POSTGRES_PASSWORD=${postgres_pwd}
      - POSTGRES_DB=sub2api
      - TZ=Asia/Shanghai
    volumes:
      - ./postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sub2api -d sub2api"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
    networks:
      - sub2api-net

  redis:
    image: ${REDIS_IMAGE}
    container_name: sub2api-redis
    restart: always
    command: redis-server --save 60 1 --appendonly yes --appendfsync everysec --requirepass ${redis_pwd}
    environment:
      - TZ=Asia/Shanghai
      - REDIS_PASSWORD=${redis_pwd}
      - REDISCLI_AUTH=${redis_pwd}
    volumes:
      - ./redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
    networks:
      - sub2api-net

  caddy:
    image: ${CADDY_IMAGE}
    container_name: sub2api-caddy
    restart: always
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - sub2api
    networks:
      - sub2api-net

volumes:
  caddy_data:
    name: sub2api_caddy_data
  caddy_config:
    name: sub2api_caddy_config

networks:
  sub2api-net:
    name: sub2api_internal_net
    driver: bridge
EOF
    else
        cat > "$INPUT_DIR/docker-compose.yaml" <<EOF
services:
  sub2api:
    image: ${DOCKER_IMAGE}
    container_name: sub2api
    restart: always
    security_opt:
      - no-new-privileges:true
    ulimits:
      nofile:
        soft: 100000
        hard: 100000
    environment:
      - AUTO_SETUP=true
      - SERVER_HOST=0.0.0.0
      - SERVER_PORT=8080
      - SERVER_MODE=release
      - RUN_MODE=standard
      - DATABASE_HOST=postgres
      - DATABASE_PORT=5432
      - DATABASE_USER=sub2api
      - DATABASE_PASSWORD=${postgres_pwd}
      - DATABASE_DBNAME=sub2api
      - DATABASE_SSLMODE=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - REDIS_PASSWORD=${redis_pwd}
      - ADMIN_EMAIL=${INPUT_ADMIN_EMAIL}
      - ADMIN_PASSWORD=${INPUT_ADMIN_PASSWORD}
      - JWT_SECRET=${jwt_secret}
      - TOTP_ENCRYPTION_KEY=${totp_key}
      - TZ=Asia/Shanghai
      - SERVER_MAX_REQUEST_BODY_SIZE=268435456
      - GATEWAY_MAX_BODY_SIZE=268435456
      - SERVER_H2C_ENABLED=true
    volumes:
      - ./data:/app/data
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
    networks:
      - sub2api-net

  postgres:
    image: ${POSTGRES_IMAGE}
    container_name: sub2api-postgres
    restart: always
    environment:
      - PGDATA=/var/lib/postgresql/data
      - POSTGRES_USER=sub2api
      - POSTGRES_PASSWORD=${postgres_pwd}
      - POSTGRES_DB=sub2api
      - TZ=Asia/Shanghai
    volumes:
      - ./postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sub2api -d sub2api"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
    networks:
      - sub2api-net

  redis:
    image: ${REDIS_IMAGE}
    container_name: sub2api-redis
    restart: always
    command: redis-server --save 60 1 --appendonly yes --appendfsync everysec --requirepass ${redis_pwd}
    environment:
      - TZ=Asia/Shanghai
      - REDIS_PASSWORD=${redis_pwd}
      - REDISCLI_AUTH=${redis_pwd}
    volumes:
      - ./redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
    networks:
      - sub2api-net

  caddy:
    image: ${CADDY_IMAGE}
    container_name: sub2api-caddy
    restart: always
    ports:
      - "${INPUT_PORT}:${INPUT_PORT}"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - sub2api
    networks:
      - sub2api-net

volumes:
  caddy_data:
    name: sub2api_caddy_data
  caddy_config:
    name: sub2api_caddy_config

networks:
  sub2api-net:
    name: sub2api_internal_net
    driver: bridge
EOF
    fi

    # 3. 写入环境配置备忘 .env
    cat > "$INPUT_DIR/.env" <<EOF
DOMAIN=${INPUT_DOMAIN}
EMAIL=${INPUT_EMAIL}
PORT=${INPUT_PORT}
ENABLE_SSL=${ENABLE_SSL}
INSTALL_DIR=${INPUT_DIR}
ADMIN_EMAIL=${INPUT_ADMIN_EMAIL}
ADMIN_PASSWORD=${INPUT_ADMIN_PASSWORD}
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=${postgres_pwd}
POSTGRES_DB=sub2api
REDIS_PASSWORD=${redis_pwd}
JWT_SECRET=${jwt_secret}
TOTP_ENCRYPTION_KEY=${totp_key}
DOCKER_IMAGE=${DOCKER_IMAGE}
EOF
    chmod 600 "$INPUT_DIR/.env"

    # 4. 创建快捷管理命令 sub2api.sh
    cat > "$INPUT_DIR/sub2api.sh" << 'EOF'
#!/usr/bin/env bash
# Sub2API Request Audit Management Tool
VERSION="v1.0.0"

SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ]; do
    DIR="$(cd -P "$(dirname "$SOURCE")" >/dev/null 2>&1 && pwd)"
    SOURCE="$(readlink "$SOURCE")"
    [[ $SOURCE != /* ]] && SOURCE="$DIR/$SOURCE"
done
BASE_DIR="$(cd -P "$(dirname "$SOURCE")" >/dev/null 2>&1 && pwd)"
[ ! -f "$BASE_DIR/docker-compose.yaml" ] && [ -d "/opt/sub2api" ] && BASE_DIR="/opt/sub2api"
cd "$BASE_DIR"

if docker compose version >/dev/null 2>&1; then
    CMD="docker compose"
else
    CMD="docker-compose"
fi

press_any_key() {
    echo ""
    read -r -p "按回车键继续..." dummy
}

clean_docker_cache() {
    echo "正在清理 Docker 悬空镜像及未使用的构建缓存..."
    docker image prune -f
    docker builder prune -f 2>/dev/null || true
    echo "Docker 镜像与构建缓存清理完毕！"
}

clean_container_logs() {
    echo "正在清理 Docker 容器超大运行日志文件..."
    local cleaned=0
    if [ -d "/var/lib/docker/containers" ]; then
        for log_file in $(find /var/lib/docker/containers/ -name "*-json.log" 2>/dev/null); do
            if [ -f "$log_file" ]; then
                truncate -s 0 "$log_file" 2>/dev/null || true
                cleaned=$((cleaned + 1))
            fi
        done
        echo "已成功截断清空 $cleaned 个 Docker 容器日志文件！"
    else
        echo "未发现 /var/lib/docker/containers 目录。"
    fi
}

clean_containers_keep_data() {
    echo "此操作将停止并移除所有容器及网络，但保留数据目录、数据库和 SSL 证书！"
    read -r -p "确认清理容器与网络环境? [y/N]: " confirm_c
    if [[ "$confirm_c" =~ ^[yY]$ ]]; then
        echo "正在停止容器并清理网络环境..."
        $CMD down --remove-orphans 2>/dev/null || true
        echo "容器已清理完毕，数据目录与 SSL 证书完整保留。"
        echo "如需重新启动服务，只需执行: sub2api start"
    fi
}

clean_uninstall_all() {
    echo "⚠️  高危警告：此操作将彻底删除 Sub2API 容器、网络、SSL 证书卷以及所有数据库与配置数据！"
    read -r -p "此操作不可逆，请确认是否继续？请输入 'yes' 确认: " confirm_clean
    if [ "$confirm_clean" = "yes" ] || [ "$confirm_clean" = "YES" ]; then
        echo "正在彻底卸载与清理..."
        $CMD down -v --remove-orphans 2>/dev/null || true
        docker volume rm -f sub2api_caddy_data sub2api_caddy_config 2>/dev/null || true
        docker network rm sub2api_internal_net 2>/dev/null || true
        rm -f /usr/local/bin/sub2api 2>/dev/null || true
        cd /
        rm -rf "$BASE_DIR"
        echo "Sub2API 服务及所有数据已彻底卸载并清理完毕！"
        exit 0
    else
        echo "已取消彻底卸载操作。"
    fi
}

cleanup_menu() {
    while true; do
        clear 2>/dev/null || true
        echo "============================================================================"
        echo "                    🧹 Sub2API · 系统清理与维护菜单                        "
        echo "============================================================================"
        echo "  1. 清理 Docker 悬空镜像与构建缓存 (释放磁盘空间，不影响正在运行的服务)"
        echo "  2. 截断/清理 Docker 容器超大运行日志 (释放日志磁盘占用)"
        echo "  3. 停止并清理容器与虚拟网络 (保留应用数据、数据库和 SSL 证书)"
        echo "  4. 彻底卸载服务并清理所有数据/证书 (危险操作，不可逆)"
        echo "  ----------------------------------------------------------------------------"
        echo "  0. 返回主菜单"
        echo ""
        read -r -p "👉 请输入清理选择 [0-4]: " clean_choice
        case "$clean_choice" in
            1) clean_docker_cache; press_any_key ;;
            2) clean_container_logs; press_any_key ;;
            3) clean_containers_keep_data; press_any_key ;;
            4) clean_uninstall_all; press_any_key; break ;;
            0|b|B|q|Q) break ;;
            *) echo "无效输入，请重新选择。"; sleep 1 ;;
        esac
    done
}

show_info() {
    source "$BASE_DIR/.env" 2>/dev/null || true
    echo "============================================================================"
    echo "                 Sub2API Request Audit · 部署配置信息                     "
    echo "============================================================================"
    if [ "$ENABLE_SSL" = "true" ] && [ -n "$DOMAIN" ]; then
        echo "访问地址:     https://${DOMAIN}"
        echo "证书管理:     Caddy 自动申请与全自动后台续期 (免配置 Crontab)"
    else
        echo "访问地址:     http://<服务器IP>:${PORT:-8080}"
        echo "证书管理:     未启用域名 HTTPS"
    fi
    echo "管理员账号:   ${ADMIN_EMAIL}"
    echo "管理员密码:   ${ADMIN_PASSWORD}"
    echo "安装目录:     ${BASE_DIR}"
    echo "服务端口:     ${PORT:-8080}"
    echo "数据库密码:   ${POSTGRES_PASSWORD}"
    echo "Redis 密码:   ${REDIS_PASSWORD}"
    echo "============================================================================"
}

show_menu() {
    while true; do
        clear 2>/dev/null || true
        echo "============================================================================"
        echo "       🚀 Sub2API Request Audit · 管理控制台 (${VERSION})       "
        echo "============================================================================"
        echo "  1. 查看服务运行状态 (Status)"
        echo "  2. 启动服务 (Start)"
        echo "  3. 停止服务 (Stop)"
        echo "  4. 重启所有容器服务 (Restart)"
        echo "  5. 查看部署配置与管理凭据 (Info)"
        echo "  6. 查看 Sub2API 主服务日志 (Sub2API Logs)"
        echo "  7. 查看 Caddy 与 SSL 证书日志 (Caddy & SSL Logs)"
        echo "  8. 查看 PostgreSQL 数据库日志 (Database Logs)"
        echo "  9. 查看 Redis 缓存日志 (Redis Logs)"
        echo " 10. 平滑更新服务镜像 (Update)"
        echo " 11. 系统清理与维护 (Docker 缓存清理 / 日志截断 / 彻底卸载)"
        echo "  ----------------------------------------------------------------------------"
        echo "  0. 退出管理控制台"
        echo ""
        read -r -p "👉 请输入选择 [0-11]: " choice
        case "$choice" in
            1) echo "=== 服务运行状态 ==="; $CMD ps; press_any_key ;;
            2) echo "正在启动服务..."; $CMD up -d; press_any_key ;;
            3) echo "正在停止服务..."; $CMD stop; press_any_key ;;
            4) echo "正在重启所有容器..."; $CMD restart; press_any_key ;;
            5) show_info; press_any_key ;;
            6) $CMD logs -f sub2api ;;
            7) echo "查看 Caddy 日志 (Ctrl+C 退出)..."; $CMD logs -f caddy ;;
            8) echo "查看 PostgreSQL 日志 (Ctrl+C 退出)..."; $CMD logs -f postgres ;;
            9) echo "查看 Redis 日志 (Ctrl+C 退出)..."; $CMD logs -f redis ;;
            10) echo "正在拉取最新镜像并更新..."; $CMD pull && $CMD up -d --remove-orphans; press_any_key ;;
            11) cleanup_menu ;;
            0|q|Q) echo "已退出。"; exit 0 ;;
            *) echo "无效输入，请重新选择。"; sleep 1 ;;
        esac
    done
}

case "$1" in
    status)
        echo "=== 服务运行状态 ==="
        $CMD ps
        ;;
    restart)
        echo "正在重启所有容器..."
        $CMD restart
        ;;
    stop)
        echo "正在停止服务..."
        $CMD stop
        ;;
    start)
        echo "正在启动服务..."
        $CMD up -d
        ;;
    logs)
        $CMD logs -f sub2api
        ;;
    logs-caddy|logs-ssl)
        echo "查看 Caddy 访问与 SSL 证书申请日志 (Ctrl+C 退出)..."
        $CMD logs -f caddy
        ;;
    logs-db|logs-postgres)
        echo "查看 PostgreSQL 日志 (Ctrl+C 退出)..."
        $CMD logs -f postgres
        ;;
    logs-redis)
        echo "查看 Redis 日志 (Ctrl+C 退出)..."
        $CMD logs -f redis
        ;;
    update)
        echo "正在拉取最新镜像并平滑更新..."
        $CMD pull
        $CMD up -d --remove-orphans
        echo "更新完毕！"
        ;;
    info)
        show_info
        ;;
    clean|cleanup)
        cleanup_menu
        ;;
    uninstall)
        clean_uninstall_all
        ;;
    version|-v|--version)
        echo "Sub2API 管理脚本版本: ${VERSION}"
        ;;
    menu|"")
        show_menu
        ;;
    *)
        echo "用法: $0 {menu|status|start|stop|restart|logs|logs-caddy|logs-db|logs-redis|update|info|clean|uninstall|version}"
        exit 1
        ;;
esac
EOF
    chmod +x "$INPUT_DIR/sub2api.sh"
    ln -sf "$INPUT_DIR/sub2api.sh" /usr/local/bin/sub2api 2>/dev/null || true
}

# 启动服务
start_services() {
    log_info "正在拉取容器镜像并启动服务..."
    cd "$INPUT_DIR"
    $COMPOSE_CMD pull || true
    $COMPOSE_CMD up -d --remove-orphans

    log_info "等待服务启动与初始化 (约 10 秒)..."
    sleep 10

    local running_cnt="$($COMPOSE_CMD ps --status running 2>/dev/null | grep -E "sub2api|caddy|postgres|redis" | wc -l)"
    if [ "$running_cnt" -ge 4 ]; then
        log_ok "Sub2API、Caddy、PostgreSQL 及 Redis 容器均已成功启动！"
    else
        log_warn "部分服务正在初始化，可使用 'sub2api status' 或 '$COMPOSE_CMD ps' 查看容器实时状态。"
    fi
}

# 显示最终部署成功信息
print_success() {
    local access_url=""
    if [ "$ENABLE_SSL" = "true" ] && [ -n "$INPUT_DOMAIN" ]; then
        access_url="https://${INPUT_DOMAIN}"
    else
        local public_ip="$(get_public_ip)"
        access_url="http://${public_ip}:${INPUT_PORT}"
    fi

    echo ""
    echo -e "${GREEN}${BOLD}============================================================================${NC}"
    echo -e "${GREEN}${BOLD}             🎉 恭喜！Sub2API Request Audit 已成功部署并启动！             ${NC}"
    echo -e "${GREEN}${BOLD}============================================================================${NC}"
    echo ""
    echo -e "  🌐 ${BOLD}平台访问地址${NC}:     ${CYAN}${access_url}${NC}"
    echo ""
    echo -e "  👤 ${BOLD}管理员账号${NC}:       ${YELLOW}${INPUT_ADMIN_EMAIL}${NC}"
    echo -e "  🔑 ${BOLD}管理员初始密码${NC}:   ${YELLOW}${INPUT_ADMIN_PASSWORD}${NC} ${RED}(首次登录后请妥善保存或修改！)${NC}"
    echo ""
    if [ "$ENABLE_SSL" = "true" ]; then
        echo -e "  🔒 ${BOLD}SSL 域名证书状态${NC}:"
        echo -e "     - ${GREEN}Caddy 自动化 HTTPS 已就绪${NC}：正在/已自动向 Let's Encrypt / ZeroSSL 申请证书。"
        echo -e "     - ${GREEN}全自动证书续期${NC}：后台服务会在证书到期前自动续签，${BOLD}无需任何人工干预或配置 Crontab${NC}。"
        echo -e "     - 证书及密钥已在 Docker 卷 ${CYAN}sub2api_caddy_data${NC} 中持久化保存。"
        echo ""
    fi
    echo -e "  🛠️  ${BOLD}常用管理命令（已全局注册 sub2api 命令）${NC}:"
    echo -e "     - 管理主控制台:       ${CYAN}sub2api${NC}"
    echo -e "     - 查看运行状态:       ${CYAN}sub2api status${NC}"
    echo -e "     - 查看主服务日志:     ${CYAN}sub2api logs${NC}"
    echo -e "     - 查看证书与访问日志: ${CYAN}sub2api logs-caddy${NC}"
    echo -e "     - 重启所有服务:       ${CYAN}sub2api restart${NC}"
    echo -e "     - 平滑更新版本:       ${CYAN}sub2api update${NC}"
    echo -e "     - 查看配置与凭据:     ${CYAN}sub2api info${NC}"
    echo -e "     - 系统维护与清理:     ${CYAN}sub2api clean${NC}"
    echo ""
    echo -e "${GREEN}============================================================================${NC}"
}

get_compose_cmd() {
    if docker compose version >/dev/null 2>&1; then
        echo "docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        echo "docker-compose"
    else
        echo "docker compose"
    fi
}

press_any_key() {
    echo ""
    read -r -p "按回车键继续..." dummy
}

service_status() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    if [ ! -d "$target_dir" ] || [ ! -f "$target_dir/docker-compose.yaml" ]; then
        log_warn "未在 $target_dir 检测到有效部署，请先执行安装。"
        return
    fi
    local cmd="$(get_compose_cmd)"
    echo "=== 服务运行状态 ($target_dir) ==="
    (cd "$target_dir" && $cmd ps)
}

service_start() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    log_info "正在启动所有服务..."
    (cd "$target_dir" && $cmd up -d)
    log_ok "服务已启动。"
}

service_stop() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    log_info "正在停止服务..."
    (cd "$target_dir" && $cmd stop)
    log_ok "服务已停止。"
}

service_restart() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    log_info "正在重启所有容器..."
    (cd "$target_dir" && $cmd restart)
    log_ok "所有服务已重启完成。"
}

service_logs() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    echo "正在查看 Sub2API 日志 (按 Ctrl+C 退出)..."
    (cd "$target_dir" && $cmd logs -f sub2api)
}

service_logs_caddy() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    echo "正在查看 Caddy 证书申请与 Web 访问日志 (按 Ctrl+C 退出)..."
    (cd "$target_dir" && $cmd logs -f caddy)
}

service_update() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    log_info "正在拉取最新镜像并平滑更新..."
    (cd "$target_dir" && $cmd pull && $cmd up -d --remove-orphans)
    log_ok "更新完成！"
}

service_info() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    if [ -f "$target_dir/.env" ]; then
        source "$target_dir/.env" 2>/dev/null || true
    fi
    echo ""
    echo -e "${CYAN}${BOLD}============================================================================${NC}"
    echo -e "${CYAN}${BOLD}                 Sub2API Request Audit · 部署配置信息                     ${NC}"
    echo -e "${CYAN}${BOLD}============================================================================${NC}"
    if [ "$ENABLE_SSL" = "true" ] && [ -n "$DOMAIN" ]; then
        echo -e "  - 访问地址:     ${GREEN}https://${DOMAIN}${NC}"
        echo -e "  - 证书管理:     ${GREEN}Caddy 自动申请与全自动后台续期 (免配置 Crontab)${NC}"
    else
        echo -e "  - 访问地址:     ${GREEN}http://<服务器IP>:${PORT:-8080}${NC}"
        echo -e "  - 运行模式:     ${YELLOW}HTTP 端口模式 (未配置域名 SSL)${NC}"
    fi
    echo -e "  - 管理员账号:   ${YELLOW}${ADMIN_EMAIL}${NC}"
    echo -e "  - 管理员密码:   ${YELLOW}${ADMIN_PASSWORD}${NC}"
    echo -e "  - 服务端口:     ${GREEN}${PORT:-8080}${NC}"
    echo -e "  - 安装目录:     ${GREEN}${INSTALL_DIR:-$target_dir}${NC}"
    echo -e "  - 数据库用户:   ${GREEN}${POSTGRES_USER:-sub2api}${NC}"
    echo -e "  - 数据库密码:   ${YELLOW}${POSTGRES_PASSWORD}${NC}"
    echo -e "  - Redis 密码:   ${YELLOW}${REDIS_PASSWORD}${NC}"
    echo -e "${CYAN}${BOLD}============================================================================${NC}"
    echo ""
}

clean_docker_cache() {
    log_info "正在清理 Docker 悬空镜像及未使用的构建缓存..."
    if command -v docker >/dev/null 2>&1; then
        docker image prune -f
        docker builder prune -f 2>/dev/null || true
        log_ok "Docker 悬空镜像与构建缓存清理完毕！"
    else
        log_warn "未检测到 Docker。"
    fi
}

clean_container_logs() {
    log_info "正在扫描并截断清理 Docker 容器日志文件..."
    local cleaned=0
    if [ -d "/var/lib/docker/containers" ]; then
        for log_file in $(find /var/lib/docker/containers/ -name "*-json.log" 2>/dev/null); do
            if [ -f "$log_file" ]; then
                truncate -s 0 "$log_file" 2>/dev/null || true
                cleaned=$((cleaned + 1))
            fi
        done
        log_ok "已成功截断清空 $cleaned 个 Docker 容器日志文件，释放磁盘空间！"
    else
        log_info "未发现 /var/lib/docker/containers 目录。"
    fi
}

clean_containers_keep_data() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    if [ -d "$target_dir" ] && [ -f "$target_dir/docker-compose.yaml" ]; then
        log_warn "此操作将停止并移除所有容器及虚拟网络，但会完整保留数据目录、数据库和 SSL 证书！"
        read -r -p "确认清理容器与网络环境? [y/N]: " confirm_clean_c
        if [[ "$confirm_clean_c" =~ ^[yY]$ ]]; then
            log_info "正在停止容器并清理网络环境..."
            (cd "$target_dir" && $cmd down --remove-orphans 2>/dev/null || true)
            log_ok "容器与虚拟网络已清理完毕，数据目录与 SSL 证书依然完整保留。"
            log_info "如需重新启动服务，只需执行: sub2api start 或 $0 start"
        else
            log_info "已取消清理容器操作。"
        fi
    else
        log_warn "未找到已部署的 compose 目录: $target_dir"
    fi
}

clean_uninstall_all() {
    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    local cmd="$(get_compose_cmd)"
    echo ""
    echo -e "${RED}${BOLD}============================================================================${NC}"
    echo -e "${RED}${BOLD}  ⚠️  高危警告：彻底卸载与完全清除操作                                        ${NC}"
    echo -e "${RED}${BOLD}============================================================================${NC}"
    echo -e "${RED}此操作将："
    echo -e "  1. 彻底停止并删除所有 Sub2API、PostgreSQL、Redis 与 Caddy 容器实例"
    echo -e "  2. 删除内部虚拟桥接网络 sub2api_internal_net"
    echo -e "  3. 删除 Caddy 证书持久化卷 (sub2api_caddy_data / sub2api_caddy_config)"
    echo -e "  4. 彻底删除安装目录 (${target_dir}) 及其所有数据库和配置数据！"
    echo -e "  5. 移除全局快捷管理命令 /usr/local/bin/sub2api"
    echo -e "${NC}"
    read -r -p "此操作不可逆，请确认是否彻底卸载并清理全部数据？请输入 'yes' 确认: " confirm_clean
    if [ "$confirm_clean" = "yes" ] || [ "$confirm_clean" = "YES" ]; then
        log_info "正在彻底卸载与清理..."
        if [ -d "$target_dir" ] && [ -f "$target_dir/docker-compose.yaml" ]; then
            (cd "$target_dir" && $cmd down -v --remove-orphans 2>/dev/null || true)
        fi
        if command -v docker >/dev/null 2>&1; then
            docker rm -f sub2api sub2api-caddy sub2api-postgres sub2api-redis 2>/dev/null || true
            docker volume rm -f sub2api_caddy_data sub2api_caddy_config 2>/dev/null || true
            docker network rm sub2api_internal_net 2>/dev/null || true
        fi
        if [ -d "$target_dir" ]; then
            rm -rf "$target_dir"
            log_ok "已清除安装目录: $target_dir"
        fi
        rm -f /usr/local/bin/sub2api 2>/dev/null || true
        log_ok "已移除全局 sub2api 命令。"
        echo -e "${GREEN}${BOLD}Sub2API 服务及所有数据已彻底卸载并清理完毕！${NC}"
    else
        log_info "已取消彻底卸载操作。"
    fi
}

cleanup_menu() {
    while true; do
        clear 2>/dev/null || true
        echo -e "${CYAN}${BOLD}"
        echo "============================================================================"
        echo "                    🧹 Sub2API · 系统清理与维护菜单                        "
        echo "============================================================================"
        echo -e "${NC}"
        echo -e "${GREEN}${BOLD}  1.${NC} 清理 Docker 悬空镜像与构建缓存 (释放磁盘空间，不影响正在运行的服务)"
        echo -e "${GREEN}${BOLD}  2.${NC} 截断/清理 Docker 容器超大运行日志 (释放日志磁盘占用)"
        echo -e "${YELLOW}${BOLD}  3.${NC} 停止并清理容器与虚拟网络 (保留应用数据、数据库和 SSL 证书)"
        echo -e "${RED}${BOLD}  4.${NC} 彻底卸载服务与清理所有数据/证书 (危险操作，不可逆)"
        echo -e "  ----------------------------------------------------------------------------"
        echo -e "${BLUE}${BOLD}  0.${NC} 返回主菜单"
        echo ""
        read -r -p "👉 请输入清理选项 [0-4]: " clean_choice
        case "$clean_choice" in
            1) clean_docker_cache; press_any_key ;;
            2) clean_container_logs; press_any_key ;;
            3) clean_containers_keep_data; press_any_key ;;
            4) clean_uninstall_all; press_any_key; break ;;
            0|b|B|q|Q) break ;;
            *) log_warn "无效输入，请重新选择。"; sleep 1 ;;
        esac
    done
}

do_install() {
    print_banner
    install_dependencies
    check_install_docker
    gather_user_input
    check_ports
    open_firewall_ports
    write_configs
    start_services
    print_success
}

show_main_menu() {
    while true; do
        print_banner
        echo -e "${GREEN}${BOLD}  1.${NC} 安装 / 重新部署 Sub2API (Install / Redeploy)"
        echo -e "${GREEN}${BOLD}  2.${NC} 启动所有服务 (Start Services)"
        echo -e "${GREEN}${BOLD}  3.${NC} 停止所有服务 (Stop Services)"
        echo -e "${GREEN}${BOLD}  4.${NC} 重启所有容器服务 (Restart Services)"
        echo -e "${GREEN}${BOLD}  5.${NC} 查看运行状态与配置 (Status & Info)"
        echo -e "${GREEN}${BOLD}  6.${NC} 查看 Sub2API 服务日志 (Sub2API Logs)"
        echo -e "${GREEN}${BOLD}  7.${NC} 查看 Caddy 与 SSL 证书日志 (Caddy & SSL Logs)"
        echo -e "${GREEN}${BOLD}  8.${NC} 平滑更新镜像版本 (Update Image)"
        echo -e "${YELLOW}${BOLD}  9.${NC} 系统清理与维护 (Docker 缓存清理 / 日志截断 / 卸载清理)"
        echo -e "  ----------------------------------------------------------------------------"
        echo -e "${RED}${BOLD}  0.${NC} 退出脚本"
        echo ""
        read -r -p "👉 请输入选择 [0-9]: " menu_choice
        case "$menu_choice" in
            1) do_install; break ;;
            2) service_start; press_any_key ;;
            3) service_stop; press_any_key ;;
            4) service_restart; press_any_key ;;
            5) service_info; service_status; press_any_key ;;
            6) service_logs ;;
            7) service_logs_caddy ;;
            8) service_update; press_any_key ;;
            9) cleanup_menu ;;
            0|q|Q) echo "退出脚本。"; exit 0 ;;
            *) log_warn "无效输入，请重新选择。"; sleep 1 ;;
        esac
    done
}

show_usage() {
    echo -e "${BOLD}Sub2API Request Audit · 一键部署与管理脚本${NC}"
    echo "项目地址: https://github.com/opengaoling/sub2api-request-audit"
    echo ""
    echo "用法: $0 [命令|选项]"
    echo ""
    echo "常用管理命令:"
    echo "  menu          显示交互式主菜单 (默认)"
    echo "  install       安装或重新部署 Sub2API 完整栈"
    echo "  clean         进入系统清理与维护菜单 (清理 Docker 缓存/日志/彻底卸载)"
    echo "  status        查看服务运行状态"
    echo "  start         启动所有容器服务"
    echo "  stop          停止服务"
    echo "  restart       重启所有服务"
    echo "  logs          查看 Sub2API 主服务实时日志"
    echo "  logs-caddy    查看 Caddy 与 SSL 证书申请日志"
    echo "  update        拉取最新镜像平滑更新"
    echo "  info          查看当前配置与访问凭据"
    echo "  uninstall     彻底卸载并清理全部数据与证书"
    echo ""
    echo "配置选项 (用于自动化/非交互部署):"
    echo "  -d, --domain <domain>           绑定域名 (启用 Caddy 自动申请 HTTPS 证书)"
    echo "  -e, --email <email>             证书通知邮箱 (可选)"
    echo "  -p, --port <port>               服务端口 (默认: 8080)"
    echo "  --admin-email <email>           管理员初始邮箱 (默认: admin@sub2api.local)"
    echo "  --admin-password <password>     管理员初始密码 (留空则随机生成)"
    echo "  --dir <path>                    安装目录 (默认: /opt/sub2api)"
    echo "  --image <image>                 Sub2API Docker 镜像"
    echo "  -y, --yes                       非交互自动确认模式"
    echo "  -h, --help                      查看帮助信息"
    echo ""
}

ACTION=""
CLI_SPECIFIED_DOMAIN=""
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -d|--domain) CLI_SPECIFIED_DOMAIN="$2"; DOMAIN="$2"; shift 2 ;;
            -e|--email)  EMAIL="$2"; shift 2 ;;
            -p|--port)   PORT="$2"; shift 2 ;;
            --admin-email) ADMIN_EMAIL="$2"; shift 2 ;;
            --admin-password) ADMIN_PASSWORD="$2"; shift 2 ;;
            --dir)       INSTALL_DIR="$2"; shift 2 ;;
            --image)     DOCKER_IMAGE="$2"; shift 2 ;;
            -y|--yes)    NON_INTERACTIVE="true"; shift ;;
            install|setup) ACTION="install"; shift ;;
            clean|cleanup|--clean) ACTION="clean"; shift ;;
            status) ACTION="status"; shift ;;
            start) ACTION="start"; shift ;;
            stop) ACTION="stop"; shift ;;
            restart) ACTION="restart"; shift ;;
            logs) ACTION="logs"; shift ;;
            logs-caddy|logs-ssl) ACTION="logs-caddy"; shift ;;
            update) ACTION="update"; shift ;;
            info) ACTION="info"; shift ;;
            uninstall) ACTION="uninstall"; shift ;;
            menu) ACTION="menu"; shift ;;
            -v|--version|version) echo "Sub2API 一键部署脚本版本: ${SCRIPT_VERSION}"; exit 0 ;;
            -h|--help) show_usage; exit 0 ;;
            *) shift ;;
        esac
    done
}

main() {
    check_root
    parse_args "$@"

    local target_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    if [ -f "$target_dir/.env" ]; then
        source "$target_dir/.env" 2>/dev/null || true
    fi

    if [ -n "$ACTION" ]; then
        case "$ACTION" in
            install) do_install ;;
            clean) cleanup_menu ;;
            status) service_status ;;
            start) service_start ;;
            stop) service_stop ;;
            restart) service_restart ;;
            logs) service_logs ;;
            logs-caddy) service_logs_caddy ;;
            update) service_update ;;
            info) service_info ;;
            uninstall) clean_uninstall_all ;;
            menu) show_main_menu ;;
            *) show_main_menu ;;
        esac
        return
    fi

    if [ -n "$CLI_SPECIFIED_DOMAIN" ] || [ "$NON_INTERACTIVE" = "true" ]; then
        do_install
    else
        show_main_menu
    fi
}

main "$@"

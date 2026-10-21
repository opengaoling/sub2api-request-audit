# Sub2API Request Audit

Sub2API Request Audit 是基于 Sub2API 维护的 AI API 网关项目，用于统一管理上游账号、API Key、请求转发、调度、计费、审计与后台运维。

本仓库专注于生产可用性、请求审计、上游错误处理和必要的兼容性修复。README 不包含赞助、合作推广、返利邀请或趋势统计链接。

## 重要提示

- 使用本项目接入上游 AI 服务前，请确认符合对应服务商的用户协议和所在地法律法规。
- 本项目只提供技术实现，使用者需要自行承担账号、计费、服务中断、数据和合规风险。
- 生产部署前请开启 HTTPS、配置强随机密钥，并限制管理后台访问范围。

## 主要功能

- 多上游账号管理，支持 OAuth、API Key 等账号类型。
- 面向用户的 API Key 创建、管理、限额与计费。
- OpenAI、Anthropic、Gemini 等协议兼容转发。
- 账号调度、粘性会话、并发控制和请求速率限制。
- 请求审计、错误记录、响应体截断展示和管理后台查询。
- 可配置的上游错误处理规则，用于临时排除异常账号。
- 管理后台用于用户、账号、模型、计费、监控和系统设置。
- 支付相关能力可按需启用，配置见 [docs/PAYMENT.md](docs/PAYMENT.md)。

## 技术栈

| 模块 | 技术 |
| --- | --- |
| 后端 | Go, Gin, Ent |
| 前端 | Vue 3, Vite, TailwindCSS |
| 数据库 | PostgreSQL |
| 缓存/队列 | Redis |
| 部署 | Docker Compose, GitHub Actions, GHCR |

## 一键脚本部署 (推荐)

提供全自动一键部署与运维管理脚本（内置 Caddy 自动申请与全自动续期 SSL 证书、自动配置 PostgreSQL 18 & Redis 8、自动生成强随机密钥与全功能快捷管理命令）：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh)
```

> [!TIP]
> 支持在 Ubuntu / Debian / CentOS / AlmaLinux / Rocky / Fedora / Alpine 等主流 Linux 发行版上运行，脚本会自动检测并就绪 Docker 与 Docker Compose 环境。

### 部署特性
- ⚡ **开箱即用**：全自动检测与就绪 Docker、Docker Compose 及依赖环境。
- 🔒 **全自动 HTTPS**：内置 Caddy，自动向 Let's Encrypt / ZeroSSL 申请证书并在后台定时自动续期，支持 TLS 1.3 与 HTTP/3 (QUIC)。
- 🚀 **流式传输 (SSE) 优化**：针对 AI 场景优化，防止模型流式输出产生缓冲延迟；内置 256MB 请求体容量，满足超长上下文与多模态文件上传。
- 🛡️ **安全开箱**：自动生成高强度的 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY`、PostgreSQL 与 Redis 密码及管理员初始凭据。
- 📦 **本地目录持久化**：应用数据、数据库、缓存与证书统一在安装目录保存，备份与整机迁移极简。
- 🛠️ **全局快捷命令 (`sub2api`)**：一键管理服务生命周期，支持状态查看、启停、重启、日志跟踪、平滑更新、系统清理与完全卸载。

### 常用管理命令
部署完成后，在服务器任何位置均可直接使用全局 `sub2api` 命令：

| 命令 | 说明 |
| --- | --- |
| `sub2api` | 打开交互式管理主控制台 |
| `sub2api status` | 查看所有容器服务运行状态 |
| `sub2api start` | 启动所有服务 |
| `sub2api stop` | 停止所有服务 |
| `sub2api restart` | 重启所有容器服务 |
| `sub2api logs` | 实时查看 Sub2API 主服务日志 |
| `sub2api logs-caddy` | 查看 Caddy 访问与 SSL 证书申请/续期日志 |
| `sub2api update` | 平滑拉取最新镜像并无缝更新 |
| `sub2api info` | 查看当前部署配置、访问地址与管理凭据 |
| `sub2api clean` | 进入系统维护菜单 (清理 Docker 缓存/截断日志/完全卸载) |

### 非交互式 / 自动化部署参数
支持通过参数直接静默部署，方便自动化脚本集成：

```bash
# 绑定域名并配置自动 HTTPS (推荐)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -d api.yourdomain.com -e admin@yourdomain.com -y

# 无域名纯 HTTP 端口部署 (默认 8080 端口)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -p 8080 -y
```

## 手动 Docker Compose 部署

推荐使用 `deploy/docker-compose.local.yml`，数据以本地目录保存，便于备份和迁移。

```bash
cd deploy
cp .env.example .env
```

编辑 `.env`，至少设置以下密钥和密码：

```bash
POSTGRES_PASSWORD=your_secure_password_here
JWT_SECRET=your_jwt_secret_here
TOTP_ENCRYPTION_KEY=your_totp_key_here
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=your_admin_password
SERVER_PORT=8080
```

生成随机密钥示例：

```bash
openssl rand -hex 32
```

启动服务：

```bash
docker compose -f docker-compose.local.yml up -d
```

查看状态和日志：

```bash
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs -f sub2api
```

升级镜像并重建容器：

```bash
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml up -d
```

## 访问与初始化

服务启动后访问：

```text
http://YOUR_SERVER_IP:8080
```

首次启动会进入初始化流程，按页面提示完成数据库、Redis 和管理员账号配置。若通过环境变量预置管理员账号，请妥善保存初始化密码。

## Nginx 反向代理

如果通过 Nginx 反向代理，并需要兼容 Codex CLI 等会传递下划线请求头的客户端，请在 Nginx `http` 块中加入：

```nginx
underscores_in_headers on;
```

否则 Nginx 默认会丢弃包含下划线的请求头，可能影响粘性会话或部分客户端兼容性。

## 配置要点

常用配置文件位于 `deploy/config.example.yaml`，Docker 部署常用环境变量位于 `deploy/.env.example`。

生产环境建议检查：

- `jwt.secret` 或 `JWT_SECRET` 使用强随机值。
- `totp_encryption_key` 或 `TOTP_ENCRYPTION_KEY` 使用强随机值。
- `server.trusted_proxies` 只配置可信反代地址。
- `cors.allowed_origins` 限制为真实前端域名。
- `security.url_allowlist` 根据实际上游域名收敛配置。
- 管理后台仅对可信网络或可信身份入口开放。
- PostgreSQL、Redis 数据目录纳入备份策略。

## 简单模式

简单模式适合个人或内部团队使用，会隐藏部分 SaaS 和计费相关能力。

```bash
RUN_MODE=simple
SIMPLE_MODE_CONFIRM=true
```

生产环境启用简单模式时仍需配置强密钥、HTTPS 和访问控制。

## 源码开发

后端开发：

```bash
cd backend
go run ./cmd/server
```

前端开发：

```bash
cd frontend
pnpm install
pnpm run dev
```

修改 Ent schema 后需要重新生成：

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

## 目录结构

```text
sub2api-request-audit/
├── backend/                 # Go 后端服务
│   ├── cmd/server/          # 程序入口
│   ├── internal/            # 核心业务模块
│   └── resources/           # 静态资源和模型价格数据
├── frontend/                # Vue 前端
│   └── src/                 # 页面、组件、API 和状态管理
├── deploy/                  # Docker Compose、示例配置和部署文件
├── deploy.sh                # 一键部署与运维管理脚本
├── docs/                    # 项目文档
└── .github/workflows/       # GitHub Actions 工作流
```

## 维护说明

本仓库会选择性审查并同步上游修复。默认只合并明确需要的 bugfix、安全修复和兼容性修复，不自动同步无关功能、推广内容或不必要的行为变化。

发布版本时以 GitHub Actions 构建结果和部署验证为准，Release 用中文记录本次变更内容。源码包由 GitHub Release 自动生成。

## License

本项目遵循 [GNU Lesser General Public License v3.0](LICENSE) 或后续版本。

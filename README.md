# Sub2API Request Audit

> 基于 Sub2API 维护的 AI API 网关项目，专注于生产可用性、请求审计、上游管理、多协议转发与后台运维。

## 🚀 一键部署 (推荐)

支持全自动安装 Docker 环境、自动配置 PostgreSQL 18 & Redis 8、通过共享 Caddy 自动申请与全自动续期 SSL 证书：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh)
```

支持命令行参数静默部署：

```bash
# 绑定域名并配置自动 HTTPS (推荐)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -d api.yourdomain.com -e admin@yourdomain.com -y

# 纯 HTTP 端口部署 (默认 8080 端口)
bash <(curl -fsSL https://raw.githubusercontent.com/opengaoling/sub2api-request-audit/main/deploy.sh) -p 8080 -y
```

## 🛠️ 常用管理命令

部署完成后，在服务器任意位置均可使用全局 `sub2api` 命令：

| 命令 | 说明 |
| --- | --- |
| `sub2api` | 打开交互式管理控制台 |
| `sub2api status` | 查看容器服务运行状态 |
| `sub2api restart` | 重启服务 |
| `sub2api update` | 平滑拉取最新镜像并更新 |
| `sub2api logs` | 实时查看 Sub2API 主服务日志 |
| `sub2api logs-caddy` | 查看 Caddy 访问与 SSL 证书日志 |
| `sub2api info` | 查看当前配置、访问地址与凭据 |
| `sub2api clean` | 系统维护 (清理缓存/截断日志/完全卸载) |

## 🐳 Docker Compose 手动部署

```bash
git clone https://github.com/opengaoling/sub2api-request-audit.git
cd sub2api-request-audit/deploy
cp .env.example .env  # 按需修改密码与密钥
docker compose -f docker-compose.local.yml up -d
```

服务启动后访问 `http://<服务器IP>:8080` 完成初始配置。

## 📄 License

本项目遵循 [GNU Lesser General Public License v3.0](LICENSE)。

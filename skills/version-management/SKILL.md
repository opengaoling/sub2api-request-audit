---
name: version-management
description: Guidelines and procedures for bumping version numbers in Sub2API upon every update or code change, ensuring deployments and GHCR images reflect new versions.
---

# Sub2API Version Management Skill

## Core Requirement
每次对项目进行任何功能更新、逻辑修复、配置调整或优化并推送到远程仓库前，**必须同步递增更新版本号**。
这样用户在界面（如导航栏/侧边栏的 `VersionBadge` 版本徽标、管理后台系统更新检查等）和容器镜像标签/运行时中，能够明确确认新版本是否已成功构建和部署生效。

## Version Source of Truth (版本号源文件)

1. **后端主版本文件**（核心版本来源）：
   - 路径：`backend/cmd/server/VERSION`
   - 作用：
     - 在编译时通过 `//go:embed VERSION` 嵌入二进制。
     - 在 `Dockerfile.ghcr` 构建镜像时，通过 `-ldflags="-X main.Version=..."` 注入版本。
     - 在 `/api/v1/admin/system/version` 和 `/api/v1/admin/system/check-updates` 接口中返回给前端。
     - 前端 `VersionBadge` 组件展示的版本号即直接取自该字段。

2. **前端包元数据**（保持同步）：
   - 路径：`frontend/package.json` 中的 `"version"` 字段。

## Version Numbering Rules (版本递增规范)

采用标准语义化版本号（Semantic Versioning, `MAJOR.MINOR.PATCH`）：
- **PATCH 版本（如 `0.2.4` -> `0.2.5`）**：常规 Bug 修复、UI 细节微调、小功能完善、算法修正、配置变更等。
- **MINOR 版本（如 `0.2.5` -> `0.3.0`）**：新增完整业务模块、重大功能引入、破坏性较小的大版本迭代。
- **MAJOR 版本（如 `0.3.0` -> `1.0.0`）**：架构重大重构、重大向下不兼容变更。

## Standard Workflow (标准发布流程)

每次代码修改完成后，严格遵循以下步骤：
1. **递增版本号**：
   - 更新 `backend/cmd/server/VERSION`（例如将 `0.2.4` 改为 `0.2.5`）。
   - 同步更新 `frontend/package.json` 中的 `"version"` 字段。
2. **Git Commit**：
   - 将版本文件与代码修改一同提交，提交信息中注明版本号（例如 `chore: bump version to 0.2.5`）。
3. **推送与远程构建**：
   - 执行 `git push origin main`。
4. **监控 GHCR 构建**：
   - 使用 GitHub CLI 监控构建工作流：
     ```bash
     gh run list -L 3
     gh run watch <run-id>
     ```
   - 必须确认 `GHCR Docker Image` 和 `CI` 均构建成功后，方可完成任务并向用户汇报。

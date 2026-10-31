---
name: version-management
description: Guidelines and procedures for bumping version numbers, creating GitHub Releases, and triggering manual GitHub Actions builds in Sub2API.
---

# Sub2API Version Management Skill

## Core Requirements (核心要求)
1. **同步更新版本号**：每次对项目进行任何功能更新、逻辑修复、配置调整或优化并推送到远程仓库前，**必须同步递增更新版本号**。
2. **每个版本创建 Release**：每个更新版本必须在 GitHub 上创建正式 Release（包含 Tag 和更新说明），以便用户与系统追踪版本更新日志。
3. **Actions 改为手动触发**：GitHub Actions 工作流（GHCR 镜像构建、CI、安全扫描等）已全部改为手动触发模式（`workflow_dispatch`），日常代码 push 不会自动触发构建。构建和发布均需手动触发并监视直至成功。

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

## Standard Workflow (标准发布与构建流程)

每次代码修改完成后，严格遵循以下全流程：

1. **递增版本号**：
   - 更新 `backend/cmd/server/VERSION`（例如将 `0.2.4` 改为 `0.2.5`）。
   - 同步更新 `frontend/package.json` 中的 `"version"` 字段。

2. **提交与推送代码**：
   - 提交 Git Commit，信息中注明版本号与主要变更：
     ```bash
     git add -A
     git commit -m "chore: bump version to 0.2.5"
     git push origin main
     ```

3. **创建 GitHub Release**：
   - 每个版本必须创建 GitHub Release，使用对应的版本号作为 Tag 和标题，并附带更新说明：
     ```bash
     gh release create <version> --title "Sub2API <version>" --notes "<更新说明>"
     # 例如：
     # gh release create 0.2.5 --title "Sub2API 0.2.5" --notes "## 功能改进与修复\n- ..."
     ```

4. **手动触发并监视 Actions 构建**：
   - 工作流已改为手动触发，使用 GitHub CLI 触发 GHCR Docker 镜像构建：
     ```bash
     gh workflow run ghcr-image.yml
     ```
   - 监视构建运行状态直至构建成功：
     ```bash
     gh run list -L 3 --workflow=ghcr-image.yml
     gh run watch <run-id>
     ```
   - 确认镜像成功推送到 GHCR 后，方可向用户汇报发布完成。

# 版本管理与发布规范

每次对项目进行任何代码更新、功能开发、Bug 修复或配置调整时：
1. **必须同步递增更新版本号**：
   - 更新 `backend/cmd/server/VERSION`（系统核心版本文件，编译及 Docker 构建时注入，并在系统更新与前端版本徽标中显示）。
   - 同步更新 `frontend/package.json` 中的 `"version"` 字段。
2. **规范提交与构建**：
   - 提交 commit 时附带版本说明（如 `chore: bump version to 0.2.5`）。
   - 推送至远程仓库后，必须监视 GitHub Actions 中的 `GHCR Docker Image` 工作流直至构建成功。

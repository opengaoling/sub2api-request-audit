# 版本管理与发布规范

每次对项目进行任何代码更新、功能开发、Bug 修复或配置调整时：
1. **必须同步递增更新版本号**：
   - 更新 `backend/cmd/server/VERSION`（核心版本文件）。
   - 同步更新 `frontend/package.json` 中的 `"version"` 字段。
2. **每个版本创建 Release**：
   - 每次发布新版本必须使用 `gh release create <version>` 创建 GitHub Release 并附带更新日志说明。
3. **Actions 手动触发与监视**：
   - GitHub Actions 工作流已全部改为手动触发（`workflow_dispatch`），代码 push 不会自动触发。
   - 推送代码后，需使用 `gh workflow run ghcr-image.yml` 手动触发镜像构建，并监视直至构建成功。

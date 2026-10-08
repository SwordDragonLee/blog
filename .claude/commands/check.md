---
description: 一键质量检查：后端 vet/build + 前端 tsc/build
allowed-tools: Bash, Read
---

# 质量检查

按顺序执行以下检查，任何一步失败就停下修复，不要跳过继续：

1. 后端静态检查与构建（在 `server/` 目录）：
   - `go vet ./...`
   - `go build ./...`
2. 管理平台（在 `web/admin/` 目录）：
   - `npm run build`（含 tsc --noEmit + vite build）
3. 前台博客（在 `web/blog/` 目录）：
   - `npm run build`（next build）

规则：
- 只检查本次改动涉及的端（用 `git status --porcelain` 判断），全部通过时输出简短通过报告；某端无改动则跳过并说明。
- 失败时给出最小修复建议（报错原文 + 修复方向），不要大规模重构。
- 不要运行 docker、不要启动 dev server。

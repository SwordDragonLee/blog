# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

AI 博客生成平台：后端调用 LLM 分析公开 Git 仓库 → 生成 4 篇中文文章（含 SVG 配图）→ 管理平台审核 → 发版到前台。Monorepo 三端：

- `server/` — Go 后端（Gin + GORM + zap + viper + RabbitMQ + Redis），模块名 `blog/server`
- `web/admin/` — 管理平台（Vite + React 18 + Antd 5 + MobX）
- `web/blog/` — 前台博客（Next.js App Router + Tailwind 4）
- `docker-compose.yml`（根目录）— 本地开发依赖：MySQL 8 + Redis 7 + RabbitMQ 3 + Qdrant

架构与实现细节全貌见 `docs/architecture.md`（按需阅读；本文只留规则与环境事实）。第三方端点拼接与厂商坑见 `docs/third-party-apis.md`。

## 常用命令

```bash
# 依赖服务（MySQL :3309、Redis :6380、RabbitMQ :5672，管理界面 :15672）
# dev 密码写死在 compose 里（blogdev，仅限本机调试；server/.env 的连接密码须与其一致）
docker compose up -d

# 一键启动三端（后端 air 热更新 + admin + blog），或单独启动
task              # = task server + task admin + task blog
task server       # cd server && air（配置 server/.air.toml，改 .go/.yaml 自动重启）
task admin        # Vite dev，http://localhost:5173
task blog         # Next.js dev，http://localhost:3000

# 后端检查与构建（验证靠 build/vet + 前端 tsc；测试仅 service 层 rag_chunk_test.go）
task vet          # go vet ./...
task build        # 编译 server.exe
cd server && go run ./cmd/server              # 不用 air 时直接跑
cd web/admin && npm run build                 # tsc --noEmit && vite build
cd web/blog  && npm run build                 # next build
```

## SDD 规范驱动开发

所有 AI 辅助开发行为以 `.claude/` 目录下的规范为准。详见 `.claude/README.md`。硬约束红线总表见 `.claude/specs/common/project-constraints.spec.md`。


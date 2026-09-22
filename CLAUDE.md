# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

AI 博客生成平台：后端调用 LLM 分析公开 Git 仓库 → 生成 4 篇中文文章（含 SVG 配图）→ 管理平台审核 → 发版到前台。Monorepo 三端：

- `server/` — Go 后端（Gin + GORM + zap + viper + RabbitMQ + Redis），模块名 `blog/server`
- `web/admin/` — 管理平台（Vite + React 18 + Antd 5 + MobX）
- `web/blog/` — 前台博客（Next.js App Router + Tailwind 4）
- `docker-compose.yml`（根目录）— 本地开发依赖：MySQL 8 + Redis 7 + RabbitMQ 3 + Qdrant

设计与 API 全貌见 `docs/superpowers/specs/2026-09-14-ai-blog-platform-design.md`。

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

# 后端检查与构建（当前无 *_test.go，验证靠 build/vet + 前端 tsc）
task vet          # go vet ./...
task build        # 编译 server.exe
cd server && go run ./cmd/server              # 不用 air 时直接跑
cd web/admin && npm run build                 # tsc --noEmit && vite build
cd web/blog  && npm run build                 # next build
```

端口非默认值：MySQL 3309、Redis 6380（本机 3306/6379 被其他服务占用），后端 :8080。默认管理员 admin/admin123（`config.yaml` auth 段）。

代理关系：admin 的 Vite dev server 把 `/api` 代理到 `localhost:8080`；blog 在服务端用 `API_BASE`（默认 `http://localhost:8080`）直连后端，不直连数据库。

## 架构

### 后端：单二进制 = API + Worker

`cmd/server/main.go` 装配一切：启动时 GORM AutoMigrate 建表并种子管理员，同时拉起 Gin HTTP 服务与 RabbitMQ consumer（独立 goroutine），SIGINT/SIGTERM 优雅停机。依赖注入为手工构造，经 `router.Deps` 传入。

分层：`router/`（路由装配）→ `api/`（HTTP handler，薄）→ `service/`（业务逻辑）→ `repo/`（GORM）+ `mq/`（RabbitMQ）+ `task/`（生成流水线）。响应封装在 `resp/`，通用中间件在 `middleware/`。

路由两组（`internal/router/router.go`）：`/api/v1/auth/login` 公开；`/api/v1/tasks|articles/*` 走 JWT；`/api/v1/portal/*` 前台只读公开（文章列表/详情、`/portal/figures/:file.svg`）+ 唯一的前台写操作 `POST /portal/ask`（RAG 问答，SSE 流式，按 IP 限流）。

### 生成任务流水线（异步，RabbitMQ 驱动）

管理端创建任务 → `gen_task` 落库（pending）+ 投递消息（direct exchange `blog.tasks`，队列 `blog.task.generate`，持久化）→ consumer 调 `task.Pipeline.Handle`：

克隆（`analyzer/`，shallow clone 到 `task.workdir`，结束清理）→ 静态分析采样代码 → LLM 第一轮分析（技术栈/亮点/选题）→ 逐篇 LLM 写作（输出 JSON：title/summary/tags/markdown/figures）→ `svggen/` 渲染配图 → 4 篇文章落库为 draft。

- 实时进度写 Redis hash `task:{id}:progress`，管理端每 2 秒轮询 `GET /tasks/:id`。
- 消费失败 nack 重投，最多 `task.max_retry`（3）次，超限进死信队列 `blog.task.dlq`，`main.go` 的 `OnDead` 回调把任务标记 failed。
- pipeline 幂等：文章按 slug upsert、配图按 article_id 重建，重跑安全。
- 任务取消：`TaskService.SetCancelFunc(consumer.CancelTask)`；consumer 以 `mq.ErrTaskCanceled` sentinel 区分「取消/删除」与真实失败（前者 ack 丢弃，不重试）。

### LLM 客户端（`internal/llm/`）

OpenAI 兼容 `/chat/completions` 为主；**base_url 含 `/anthropic` 时自动切换 Anthropic Messages 协议**（如智谱 Coding Plan，见 `client_anthropic.go`）。结构化输出靠 prompt 约定 JSON schema + 解析校验，失败自动重试；网络错误指数退避。

### RAG 技术问答（`internal/qdrant/` + `service/rag*.go` + `api/rag_handler.go`）

文章发布后异步切块向量化入 Qdrant（`qdrant` 容器，REST :6333；下线/退回 draft 即删向量）；前台聊天气泡 `web/blog/components/AiChat.tsx` 调 `POST /portal/ask`，SSE 事件序列 `delta* → citations → done`。embedding 走 SiliconFlow 免费的 BAAI/bge-m3（OpenAI 兼容 `/embeddings`，1024 维，`rag.embedding_*` 单独配置；bge-m3 不接受 dimensions 参数，`embedding_send_dimensions` 须为 false，智谱 embedding-3 才需要 true）。问答对话模型由 `rag.chat_*` 单独指定（glm-4-flash 免费档，走智谱标准 paas/v4 端点）——问答面向公众调用量大，不消耗主模型生成套餐额度；`chat_model` 留空则复用主模型。切块 ~800 字符、标题优先断开、尾部 overlap；point ID = `articleID*10000+chunkIndex`（确定性，重复发布幂等覆盖）。`rag.enabled=false` 或依赖缺失时问答返回 503，发布不受影响。设计细节见 `docs/superpowers/specs/2026-09-17-rag-tech-qa-design.md`。

### SVG 配图（`internal/svggen/`）

LLM 只输出结构化图表数据（spec JSON），Go 端确定性渲染为统一暗色科技风 SVG（cover/architecture/flow/compare/timeline 五种），零外部依赖。SVG 原文存 `svg_asset` 表，前台经 `/portal/figures/:file.svg` 引用；管理端可单篇重新生成配图。

### 文章状态机与缓存

`draft --publish--> published --offline--> draft`；编辑已发布文章会退回 draft，需重新发版。发版/下线/编辑时主动删 Redis 前台缓存（`portal:articles:list:*`、`portal:article:{slug}`），并有 TTL 兜底。

### 前端

- admin（`web/admin/src/`）：MobX stores（`stores/auth.ts` 登录态存 localStorage、`stores/task.ts` 轮询）、axios 实例（`api/http.ts`）自动带 JWT、401 跳登录；页面在 `pages/`，`types.ts` 是与后端对齐的共享类型。
- blog（`web/blog/app/`）：App Router，服务端 fetch 后端渲染 Markdown（react-markdown + remark-gfm + rehype-highlight），ISR revalidate 60s。

## 约定与注意事项

- 代码注释、提交信息、UI 文案均为中文；标识符用英文。
- **私密数据（密钥/密码）只存 `.env`，config.yaml 不含任何私密项**：本地 `server/.env`（`config.Load` 自动加载）、容器部署时 compose 同目录 `.env`（`env_file` 注入 server 容器），均已被 .gitignore 忽略；模板为 `server/.env.example` 与 `docker/env.production.example`。任意配置项可用同名环境变量覆盖（点号换下划线，如 `llm.api_key` → `LLM_API_KEY`，见 `config/config.go`）。

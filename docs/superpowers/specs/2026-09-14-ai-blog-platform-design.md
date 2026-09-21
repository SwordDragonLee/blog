# AI 博客生成平台 — 设计文档

日期：2026-09-14
状态：已批准

## 1. 背景与目标

构建一个「AI 博客生成平台」，由三部分组成：

1. **前台博客网站**（Next.js）：面向读者，展示已发布的文章，图文结合。
2. **管理平台**（React + Antd + MobX）：面向管理员，提交 Git 仓库地址、查看生成进度、编辑/预览/审核文章、发版。
3. **Go 后端**（Gin + GORM + zap + viper + RabbitMQ）：提供管理端与前台只读 API；耗时生成任务经 RabbitMQ 异步执行（克隆仓库 → 调用 LLM 分析与写作 → 渲染 SVG 配图 → 文章入库（draft））。

核心用户流程：

> 管理员在管理平台输入一个公开 Git 仓库地址 → 后端异步分析该仓库的技术栈与代码亮点 → 生成 4 篇图文结合的博客文章（草稿）→ 管理员审核、可编辑、可重新生成配图 → 点击「发版」→ 前台立即可见。

## 2. 技术选型

| 层 | 选型 | 说明 |
|---|---|---|
| 前台 | Next.js（App Router）+ Tailwind CSS | SSR/ISR 渲染，react-markdown 渲染文章（GFM + 代码高亮） |
| 管理平台 | Vite + React 18 + Antd 5 + MobX（mobx-react-lite）+ React Router | MobX 管理全局状态 |
| 后端框架 | Go + Gin | HTTP 服务，admin / portal 两组路由 |
| ORM | GORM + MySQL 8 | 5 张表 |
| 日志 | zap | 结构化日志，dev/prod 两种模式 |
| 配置 | viper | `config.yaml` + 环境变量覆盖（LLM base_url/api_key/model 等） |
| 缓存/进度 | Redis 7 | 任务实时进度、前台文章缓存、LLM 限流 |
| 任务队列 | RabbitMQ 3 | 耗时生成任务异步化：API 落库后投递消息，worker 消费执行；消息持久化 + 失败重试 + 死信队列 |
| LLM | OpenAI 兼容接口（可配置） | 通过配置切换 DeepSeek / 通义千问 / GLM / OpenAI 等 |
| 配图 | SVG 程序化渲染 | LLM 输出结构化图表数据，Go 端确定性渲染，零外部依赖 |
| 依赖编排 | Docker Compose | MySQL + Redis + RabbitMQ；Go 与前端本地运行 |

## 3. 总体架构

```
┌─────────────────┐     ┌──────────────────────┐
│  前台博客网站     │     │  管理平台              │
│  Next.js (SSR)  │     │  React+Antd+MobX      │
└────────┬────────┘     └──────────┬───────────┘
         │  /portal/* 只读接口      │  /api/v1/* 管理接口(JWT)
         └──────────┬──────────────┘
                    ▼
         ┌─────────────────────┐      ┌──────────────┐
         │  Go API (Gin)        │─────▶│  OpenAI 兼容  │
         │  · 落库 + 投递消息    │      │  LLM 服务     │
         └────────┬────────────┘      └──────▲───────┘
                  ▼                          │
         ┌─────────────────────┐             │
         │  RabbitMQ            │             │
         │  blog.task.generate  │             │
         └────────┬────────────┘             │
                  ▼                          │
         ┌─────────────────────┐            │
         │  Go Worker（同二进制  │────────────┘
         │  独立 goroutine 消费）│
         │  · git shallow clone │
         │  · SVG 图表渲染器     │
         └────┬──────────┬─────┘
              ▼          ▼
          ┌───────┐  ┌───────┐
          │ MySQL │  │ Redis │   (Docker Compose 拉起)
          └───────┘  └───────┘
```

RabbitMQ 拓扑：direct exchange `blog.tasks`，队列 `blog.task.generate`（持久化），死信队列 `blog.task.dlq`（重试超限后进入并标记任务失败）。Go 后端为单二进制：启动时同时拉起 Gin API 与 MQ consumer（独立 goroutine），部署简单且任务语义完整。

## 4. Monorepo 目录结构

```
blog/
├── server/                  # Go 后端
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── api/            # HTTP handler（admin/portal 两组路由，中间件）
│   │   ├── service/        # 业务逻辑（任务、文章、发布、LLM 编排）
│   │   ├── repo/           # GORM 数据访问
│   │   ├── llm/            # OpenAI 兼容客户端（chat + JSON 输出校验重试）
│   │   ├── analyzer/       # git shallow clone + 技术栈识别 + 代码采样
│   │   ├── svggen/         # SVG 图表渲染器（cover/architecture/flow/compare/timeline）
│   │   ├── mq/             # RabbitMQ 连接、发布者、消费者（拓扑声明、ack/nack、死信）
│   │   ├── task/           # 任务执行 pipeline（由 MQ consumer 调用，进度写 Redis）
│   │   ├── model/          # GORM 模型
│   │   └── config/         # viper 配置加载
│   ├── config.yaml
│   └── go.mod
├── web/
│   ├── blog/               # Next.js 前台
│   └── admin/              # 管理平台
├── deploy/
│   └── docker-compose.yml  # MySQL 8 + Redis 7 + RabbitMQ 3
├── docs/                   # 设计文档
└── README.md
```

## 5. 核心生成流水线（异步任务）

管理平台提交 git 地址 → API 创建 `gen_task`（pending）并投递消息到 RabbitMQ → worker 消费执行，进度写 Redis，管理平台每 2 秒轮询：

| 步骤 | 内容 | 进度 |
|---|---|---|
| ① 克隆 | `git clone --depth 1` 公开仓库到临时目录（结束后清理） | 5% |
| ② 仓库分析 | 识别特征文件（go.mod / package.json / requirements.txt / Cargo.toml / pom.xml…）→ 判定语言与技术栈；采样核心代码文件（跳过 vendor/node_modules/二进制，按文件重要性选取，总量截断） | 15% |
| ③ LLM 分析 | 第一轮：输入仓库摘要 + 代码采样，输出 JSON：技术栈清单、架构摘要、代码亮点、4 篇文章选题大纲 | 30% |
| ④ 逐篇写作 | 第二轮：每篇一次调用，输出 JSON：`{title, summary, tags, markdown, figures[]}`；Markdown 中用 `{{figure:id}}` 占位插图 | 30%→90% |
| ⑤ 渲染配图 | svggen 将 figures 渲染为风格统一的 SVG（含每篇封面图）入库 | 95% |
| ⑥ 完成 | 4 篇文章落库为 `draft`，`repo_analysis` 落库，任务结束（审核属于文章状态机，非任务阶段） | 100% |

### 文章规划（每次生成 4 篇，字数由系统设定）

1. 技术栈全景解读（约 2000 字）
2. 架构设计解析（约 2500 字，配架构图）
3. 优雅代码写法赏析（约 2500 字，含代码片段高亮）
4. 工程实践与最佳实践总结（约 1800 字，配流程图/时间线）

### LLM 交互设计

- 客户端：OpenAI 兼容 `/chat/completions`，viper 配置 `base_url / api_key / model / timeout`。
- 结构化输出：prompt 中约定 JSON schema，响应做 JSON 解析校验，失败自动重试（最多 3 次）。
- 网络错误/超时：指数退避重试。
- 每篇文章正文中文撰写，代码与技术名词保留英文。

### SVG 配图方案

- LLM 输出结构化图表数据（不输出图片本身），后端确定性渲染，保证 100% 成功率。
- 图类型：`cover`（封面卡片：技术栈标签排版）、`architecture`（分层架构图）、`flow`（流程图）、`compare`（对比卡片）、`timeline`（时间线）。
- 渲染：Go 端统一暗色科技风模板，自动计算节点坐标、连线、配色；SVG 原文存库，前台通过 `/portal/figures/:id.svg` 引用。
- 管理平台可对单篇文章一键重新生成配图（重跑 LLM 图表输出 + 渲染）。

## 6. 数据模型（MySQL，5 张表）

| 表 | 关键字段 |
|---|---|
| `admin_user` | id, username, password_hash(bcrypt), created_at |

后端启动时 GORM AutoMigrate 建表；若 `admin_user` 为空，则按 viper 配置（默认 `admin / admin123`，可改）自动创建管理员。

| 表（续） | 关键字段 |
| `gen_task` | id, git_url, status(pending/running/success/failed), progress(int 0-100), step, message, error, repo_analysis_id, created_at, updated_at |
| `repo_analysis` | id, git_url, default_branch, tech_stack(JSON), summary, highlights(JSON), created_at |
| `article` | id, repo_analysis_id, title, slug(唯一), summary, content_md(MEDIUMTEXT), word_count, tags(JSON), status(draft/published/offline), sort_order, published_at, created_at, updated_at |
| `svg_asset` | id, article_id, kind, title, spec(JSON), svg_content(MEDIUMTEXT), created_at |

slug 由 LLM 在写作输出中给出英文 slug，后端校验唯一性，冲突时追加短随机后缀；标题重复同理。

### 文章状态机

```
draft（待审核）--publish 发版--> published --offline 下线--> draft
published 可再编辑：保存后回到 draft，需重新发版
```

## 7. Redis 用途

| Key | 用途 |
|---|---|
| `task:{id}:progress` | 任务实时进度（step、percent、message、日志追加），管理平台 2s 轮询 |
| `portal:articles:list:{page}:{tag}` | 前台文章列表缓存 |
| `portal:article:{slug}` | 前台文章详情缓存 |
| `llm:ratelimit:{key}` | LLM 接口简易限流 |

失效策略：发版/下线/编辑已发布文章时主动删除相关缓存；缓存同时设 TTL 兜底。

## 8. API 设计（REST，前缀 /api/v1）

### 管理端（JWT 鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /auth/login | 登录，返回 JWT |
| POST | /tasks | 创建生成任务（git_url） |
| GET | /tasks | 任务列表 |
| GET | /tasks/:id | 任务详情（含 Redis 实时进度，轮询用） |
| POST | /tasks/:id/retry | 失败任务重跑 |
| GET | /articles | 文章列表（?status= 筛选） |
| GET | /articles/:id | 文章详情（含配图） |
| PUT | /articles/:id | 编辑文章（标题/摘要/正文/标签） |
| POST | /articles/:id/publish | 发版 |
| POST | /articles/:id/offline | 下线 |
| POST | /articles/:id/regenerate-figures | 重新生成该篇配图 |

### 前台（只读，无鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /portal/articles | 已发布文章分页列表（?page=&tag=） |
| GET | /portal/articles/:slug | 文章详情（Markdown + 配图 id 列表 + 标签） |
| GET | /portal/figures/:id.svg | SVG 配图（Content-Type: image/svg+xml） |

## 9. 前台博客网站（Next.js）

- 首页：文章卡片流（SVG 封面 + 标题 + 摘要 + 标签 + 发布时间），标签筛选，分页。
- 详情页：服务端拉取 Markdown → react-markdown（remark-gfm + rehype-highlight）渲染；SVG 插图内联展示；响应式排版。
- 数据获取：服务端调 Go API，ISR（revalidate 60s），不直连数据库。
- 样式：Tailwind CSS，简洁阅读向设计。

## 10. 管理平台（React + Antd + MobX）

- 页面：
  - 登录页
  - 工作台 Dashboard（任务概览 + 文章统计）
  - 任务：新建任务表单（git_url 校验）、任务列表、任务详情（进度条 + 步骤日志，2s 轮询）
  - 文章管理：列表（状态筛选：待审核/已发布/已下线）
  - 文章编辑：左侧 Markdown 编辑器 / 右侧实时预览；配图预览卡片；重新生成配图按钮；保存；**发版按钮（draft→published）**；下线按钮
- MobX stores：`AuthStore`（token、登录态持久化 localStorage）、`TaskStore`（轮询逻辑）、`ArticleStore`。
- 请求封装：axios 实例，JWT 自动附带，401 跳转登录。

## 11. 错误处理

- LLM 调用：超时 + 指数退避重试；JSON 校验失败重试（最多 3 次）；仍失败则任务置 `failed` 并记录 error，可一键重跑（重跑 = 重新投递 MQ 消息）。
- git clone 失败（仓库不存在 / 私有仓库 / 非公开）：明确错误信息写入任务。
- MQ 消费失败：nack 并重投，同一任务最多重试 3 次，超限进入死信队列 `blog.task.dlq` 并将任务标记 `failed`。
- 服务重启：未 ack 的消息由 RabbitMQ 自动重新投递；pipeline 幂等（文章按 slug upsert，SVG 按 article_id 重建），重跑安全。
- 发布时缓存失效失败：重试，失败不阻塞发版（TTL 兜底）。

## 12. 测试策略

- Go：service 层单元测试；svggen 渲染快照测试（合法 SVG、关键元素存在）；analyzer 的技术栈识别单测（固定样本目录）；LLM 与 git 通过接口 mock。
- 前端：ESLint + TypeScript 编译通过 + 构建通过；页面以手动验收为主。
- 端到端验收：用真实公开仓库（如一个小型 Go/JS 项目）跑通 提交→生成→审核→发版→前台可见 全流程。

## 13. 非目标（本期不做）

- 私有仓库支持（无凭证克隆）
- 多用户 / 角色权限
- 评论、点赞、搜索等前台互动功能
- AI 生图 API 配图
- 文章国际化（仅中文）
- 部署流水线 / HTTPS / 域名配置

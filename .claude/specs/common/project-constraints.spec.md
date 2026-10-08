# 项目核心约束

本项目是 AI 博客生成平台 Monorepo：`server/`（Go + Gin + GORM + RabbitMQ + Redis + Qdrant）、`web/admin/`（Vite + React 18 + Antd 5 + MobX）、`web/blog/`（Next.js App Router + Tailwind 4）。

本文件是全仓**红线总表**，在 `.claude/specs/` 中优先级最高。条目的展开说明与正反示例见 `specs/coding/go-backend.spec.md`、`specs/coding/web-frontend.spec.md`；更新红线先改本文件，再同步专项 spec。

## 绝对禁止

### 密钥与 Git 安全

- **禁止**在任何进入 git 的内容中出现明文密钥/密码/证书私钥——代码、`config.yaml`、文档、提交信息都算；私密数据只进 `.env`（已被 gitignore）
- **禁止**未经用户明确授权执行 `git commit` / `git push`
- **禁止**用 `--no-verify` 跳过 commit 校验
- **禁止**提交调试残留：Go 的 `fmt.Println`、前端的 `console.log` / `debugger`

### 分层与接口边界

- **禁止**后端分层越界：api 层写业务逻辑、handler 直接持有 `*gorm.DB` / `*redis.Client`、repo 反向引用 service
- **禁止**前端绕过统一封装发请求：admin 组件裸用 fetch / 自建 axios 实例（必须走 `api/http.ts`）；blog 组件硬编码后端地址（必须走 `lib/api`）
- **禁止**给前台 `/portal/*` 新增写接口（唯一例外：`POST /portal/ask`）

### 并发与可中断性

- **禁止**裸 `time.Sleep` 等待（必须 `select` + `time.After` / `ctx.Done()`，停机与任务取消要能即时打断）
- **禁止**裸 `go func(){}()` 悬挂 goroutine（必须有 ctx 或 done 退出通道）
- **禁止**丢弃原始错误：向上包装一律 `fmt.Errorf("...: %w", err)`

### 前端红线

- **禁止**新增 class 组件、滥用 `any`
- **禁止**在页面里手写与后端重复的 interface（对齐类型统一放 `types.ts`）

## 必须遵守

- 代码注释、提交信息、UI 文案一律**中文**；标识符用英文
- 阻塞操作（HTTP 请求、子进程、DB、MQ）**必须**携带 ctx，保证任务取消/停机可即时打断
- 涉及 MQ 重投 / 任务重跑的写库操作**必须**幂等（三种模式：唯一业务键 upsert、delete-then-write、确定性 ID 覆盖）
- 新增配置项三处同步：`config/` 结构体 + `config.yaml` 默认值 + `.env.example` 模板
- 提交前验证闭环：后端过 `task vet` + `task build`，前端过对应目录 `npm run build`，测试跑 `cd server && go test ./...`
- 非平凡改动（跨层、新增接口/表、状态机变更）先走 `/spec` → `/plan`，确认后再实施
- 存量代码不符合红线时按「顺手修复、单独提交」处理，不做无关重构

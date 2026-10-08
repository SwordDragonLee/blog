# 命名与目录结构规范

## 目录职责地图

### server/internal/（Go 后端）

| 目录 | 职责 |
| --- | --- |
| `router` | 路由装配、`Deps` 依赖注入入口 |
| `api` | HTTP handler（薄：参数绑定 → 调 service → resp 封装） |
| `service` | 业务逻辑 |
| `repo` | GORM 数据访问 |
| `mq` / `task` | RabbitMQ 生产消费 / 生成任务流水线 |
| `llm` / `svggen` / `qdrant` / `analyzer` / `mailer` | 外部能力适配（LLM 协议、SVG 渲染、向量库、静态分析、邮件） |
| `middleware` / `resp` / `httputil` / `logger` / `model` / `config` / `app` | 横切关注点与启动装配 |

新文件跟着分层走：handler 进 `api/`、业务进 `service/`、数据访问进 `repo/`、外部协议适配进对应能力包。

### web/admin/src/

`api/`（axios 实例与接口函数）· `pages/`（页面与页面级组件，平铺）· `components/`（跨页面复用组件）· `stores/`（MobX store）· `types.ts`（与后端对齐的共享类型）

### web/blog/

`app/`（App Router 路由与页面）· `components/`（客户端组件）· `lib/`（服务端取数封装等）

## 文件命名

- Go 文件 **snake_case**：`rag_chunk.go`、`client_anthropic.go`；包名小写单词
- React 组件 **PascalCase**，一组件一文件：`CreateTaskModal.tsx`、`AiChat.tsx`
- 非组件 TS 文件小驼峰：`http.ts`、`types.ts`
- Next.js 路由段小写：`app/article/[slug]/page.tsx`

## 标识符

| 环境 | 规则 |
| --- | --- |
| Go 导出 | PascalCase；错误 sentinel `ErrXxx`；常量 PascalCase（Go 惯例不用 SCREAMING_SNAKE） |
| Go 非导出 | camelCase |
| TS 变量/函数 | camelCase；事件回调 props 用 `onXxx`，内部 handler 用 `handleXxx` |
| TS 组件/类型 | PascalCase；模块级常量 SCREAMING_SNAKE |

标识符英文，注释中文（红线总表）。

## 禁止

- **禁止**在仓库根 / 包根平铺新文件绕过上述目录职责
- **禁止**跨目录放同职责文件（如 handler 里私有的数据访问函数放 repo/ 之外）

# AI 流程指南

对照代码讲清楚这个项目的 AI 部分怎么运转：文章怎么被 AI 写出来、配图怎么生成、前台的技术问答（RAG）怎么工作，以及所有模型调用如何收口在统一的客户端层。

## 0. AI 能力总览

| 能力 | 触发方 | 模型 | 输出形态 | 核心代码 |
|---|---|---|---|---|
| 仓库分析 | 管理平台生成任务（异步） | glm-4.6 | JSON（技术栈/亮点/选题） | `task/pipeline.go` |
| 文章写作 | 生成任务内逐篇调用 | glm-4.6 | JSON（title/summary/tags/markdown/figures） | `task/pipeline.go` |
| 配图设计 | 生成任务 / 管理端重生成 | glm-4.6 | JSON 图表 spec → Go 渲染 SVG | `llm/prompt` + `svggen/` |
| RAG 问答回答 | 前台访客（实时） | glm-4-flash（免费） | SSE 流式文本 | `service/rag.go` + `api/rag_handler.go` |
| 文本向量化 | 发布时异步 / 提问时同步 | BAAI/bge-m3（SiliconFlow 免费） | 1024 维向量 | `llm/embedding.go` |

**模型分工原则**：管理平台是自己人用、调用量小，用套餐内的高质量 glm-4.6；前台问答面向公众、调用量不可控，回答与向量化全部用免费模型，成本恒为零。

## 1. LLM 客户端层（`internal/llm/`）

所有模型调用收口在这一层，上层不直接发 HTTP：

- **双协议自动切换**（`client.go`）：`base_url` 含 `/anthropic` 时自动走 Anthropic Messages 协议（`client_anthropic.go`：system 提为顶层参数、`x-api-key` 鉴权、关闭思考模式）；否则走 OpenAI 兼容 `/chat/completions`。当前 glm-4.6 走前者（智谱 Coding Plan 套餐端点），glm-4-flash 与 bge-m3 走后者（标准计费 API，免费模型不扣余额）。
- **结构化输出 `ChatJSON`**：prompt 中约定 JSON schema → 解析 → 调用方传入的 `validate` 函数校验 → 校验不过把错误信息喂回模型重写，最多 3 轮。仓库分析和文章写作都靠这个保证输出可直接落库。
- **重试与退避**：网络层错误指数退避重试（`chatWithRetry`）。
- **流式 `StreamChat`**（`stream.go` / `stream_anthropic.go`）：SSE 增量解析，每段文本回调 `onDelta`，供问答的打字机输出；同样按端点自动分协议。
- **Embedder**（`embedding.go`）：OpenAI 兼容 `/embeddings` 批量向量化，指数退避重试；`dimensions` 参数按 `embedding_send_dimensions` 开关决定是否发送（bge-m3 固定 1024 维，传了会报 400；智谱 embedding-3 需要传）。

## 2. 文章生成流水线（离线异步）

```
管理端 POST /tasks
   │
   ▼
gen_task 落库(pending) ──► RabbitMQ（direct exchange blog.tasks
   │                        队列 blog.task.generate，消息持久化）
   ▼
consumer 调 task.Pipeline.Handle
   │
   ├─ 1. 克隆仓库（analyzer：shallow clone 到 task.workdir，结束清理）
   ├─ 2. 静态分析采样（按预算截断：max_files 30 / 单文件 16KB / 总预算 120KB）
   ├─ 3. LLM 第一轮 analyze_repo ──► 技术栈/亮点/选题（repo_analysis 落库）
   ├─ 4. 逐篇 LLM write_article ──► title/summary/tags/markdown + figures JSON
   │      （markdown 中 {{figure:xxx}} 占位符与 figures 数组一一对应）
   ├─ 5. svggen.Render：LLM 只出图表数据 spec（cover/architecture/flow/
   │      compare/timeline 五种），Go 端确定性渲染为暗色科技风 SVG 存 svg_asset
   └─ 6. 4 篇文章落库为 draft，等管理员审核发版
```

可靠性设计：

- **实时进度**：Redis hash `task:{id}:progress`，管理端每 2 秒轮询 `GET /tasks/:id`
- **失败重试**：消费失败 nack 重投，最多 `task.max_retry`（3）次；超限进死信队列 `blog.task.dlq`，`OnDead` 回调把任务标记 failed
- **主动取消**：`TaskService.SetCancelFunc(consumer.CancelTask)`；consumer 以 `mq.ErrTaskCanceled` sentinel 区分「取消/删除」与真实失败（前者 ack 丢弃不重试）
- **幂等重跑**：文章按 slug upsert、配图按 article_id 重建，任务重试不会产生重复数据

## 3. RAG 技术问答（在线实时）

### 3.1 索引链路（文章发布时异步触发）

```
publish 成功 ──► goroutine（context.WithoutCancel + 60s 超时，
                  失败仅记日志，不影响发布结果）
   │
   ├─ 1. RemoveArticle 清旧向量（幂等前提）
   ├─ 2. cleanMarkdownForIndex：去掉 {{figure:xxx}} 占位符、压缩空行
   ├─ 3. chunkMarkdown：~800 字符切块，标题处优先断开，尾部 overlap 100 字符
   ├─ 4. bge-m3 批量向量化（SiliconFlow，1024 维）
   └─ 5. EnsureCollection + UpsertChunks
         point ID = articleID*10000 + chunkIndex（确定性，重复发布直接覆盖）
```

文章**下线**或**编辑退回草稿**时同步调用 `RemoveArticle` 删除向量，保证对外不可见的内容不再被问答引用。

### 3.2 问答链路（前台访客提问）

`POST /api/v1/portal/ask`（公开接口，按 IP 每分钟 10 次限流），响应为 SSE，事件序列 `delta* → citations → done`（出错发 `error`）：

1. 问题向量化（bge-m3）→ Qdrant 检索 top5 → 相似度阈值 0.3 过滤噪声
2. **无命中**：直接下发固定话术，不调 LLM（省 token 且绝不编造）
3. **有命中**：组装消息 = system 规则（仅依据片段回答/句末标 [n]/不足则明说/中文/markdown）+ 最近 3 轮对话历史 + 带编号的参考片段
4. glm-4-flash `StreamChat` 流式生成，每段增量实时下发 `delta` 事件
5. 结束后下发 `citations`（title/slug/score/snippet）→ `done`

前端 `web/blog/components/AiChat.tsx`：右下角聊天气泡，fetch ReadableStream 手工解析 SSE 帧，react-markdown 渲染回答（`prose-chat` 紧凑样式），citations 渲染成可点击的「资料来源」卡片跳转 `/article/{slug}`。

## 4. 模型分工与费用

| 模型 | 端点 / 协议 | 用途 | 费用 |
|---|---|---|---|
| glm-4.6 | `open.bigmodel.cn/api/anthropic`（Anthropic Messages） | 仓库分析、文章写作、配图 spec、配图重生成 | Coding Plan 包月内，不按量扣费 |
| glm-4-flash | `open.bigmodel.cn/api/paas/v4`（OpenAI 兼容） | RAG 问答回答 | 官方免费档 |
| BAAI/bge-m3 | `api.siliconflow.cn/v1`（OpenAI 兼容） | 文章切块向量化、问题向量化 | SiliconFlow 免费档 |

注意事项：

- 智谱 Coding Plan 只覆盖 `/anthropic` chat 端点；同 key 调标准 API 的付费模型（如 embedding-3）会因账户余额报 429
- bge-m3 不接受 `dimensions` 参数，`rag.embedding_send_dimensions` 必须为 false；若换回智谱 embedding-3 则改为 true
- 换 embedding 模型 = 换向量空间：需删掉 Qdrant 集合（维度不同不兼容）、改 `embedding_dimensions`、重新发布文章重建索引

## 5. 配置速查（`server/config.yaml`）

| 段 | 作用 |
|---|---|
| `llm` | 主模型（生成任务）：base_url / api_key / model / 超时 / 速率 |
| `rag` | 问答总开关、embedding（base_url/key/model/dimensions）、问答专用 chat 模型（chat_*）、切块（chunk_size/overlap）、检索（top_k/score_threshold） |
| `qdrant` | 向量库地址与集合名 |

所有配置项均可用同名环境变量覆盖（点号换下划线，如 `llm.api_key` → `LLM_API_KEY`）。

## 6. 关键设计决策回顾

1. **结构化输出靠 prompt 约定 + 校验重试**，不依赖模型 function calling——任何 OpenAI 兼容模型都能接入
2. **配图由 Go 确定性渲染**，LLM 只出数据不出图——样式统一、渲染零失败（失败保留原图）
3. **无命中不调 LLM**——RAG 检索为空直接回固定话术，省钱且杜绝编造
4. **point ID 确定性生成**——重复发布/任务重试天然幂等，无需额外去重逻辑
5. **公众链路全免费模型**——问答与向量化零成本，套餐额度只花在内部生成上

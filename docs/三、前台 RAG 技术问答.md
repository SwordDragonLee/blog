# AI 功能三：前台 RAG 技术问答

> 本文讲清「访客在博客右下角聊天气泡里提一个问题 → 流式收到带引用的回答」的全链路。
> 入口代码：`server/internal/api/rag_handler.go` 的 `Ask` → `server/internal/service/rag.go` 的 `RagService.Ask`。
> 前置阅读：向量库里的数据从哪来见《ai-rag-indexing.md》。

## 一句话概述

前台公开接口 `POST /api/v1/portal/ask`（无需登录，按 IP 限流）：把问题向量化 → 在 Qdrant 里检索最相关的文章片段 → 拼进提示词让 glm-4-flash **SSE 流式**回答 → 最后下发引用的文章出处。回答被约束为"仅依据片段、不足则明说"，从机制上抑制编造。

## 全链路

```
web/blog/components/AiChat.tsx（聊天气泡）
  └─ POST /portal/ask  { question, history: 最近几轮对话 }
       ├─ RagHandler.Ask            api/rag_handler.go
       │    ├─ Enabled 校验：rag.enabled=false 或依赖缺失 → 503
       │    ├─ IP 限流：每分钟 10 次（进程内固定窗口），超限 429
       │    └─ SSE 响应头 → 调 RagService.Ask，增量文本实时下发
       └─ RagService.Ask            service/rag.go
            ├─ ① 问题向量化   embedder.Embed（与索引用同一个 bge-m3）
            ├─ ② 向量检索     store.Search top_k → 阈值过滤 → 按文章分组合并（上下文全保留，引用一篇一条）
            ├─ ③ 无命中短路    不调 LLM，直接下发固定话术（省 token 且不编造）
            ├─ ④ 组装消息      system 规则 + 检索片段 + 最近几轮 history + 问题
            └─ ⑤ 流式生成      chat 客户端 SSE 转发，每段增量回调 onDelta
```

## SSE 事件协议（rag_handler.go）

响应 `Content-Type: text/event-stream`（带 `X-Accel-Buffering: no` 防 Nginx 缓冲），事件严格按序：

```
event: delta      data: {"text":"..."}     ← N 次，增量正文
event: citations  data: [{title,slug,score,snippet}, ...]   ← 恰好一次
event: done       data: {}                 ← 正常结束标记
event: error      data: {"message":"..."}  ← 服务端出错（此后连接结束）
```

注意：进入 SSE 后不再走 `resp` 统一 JSON 封装，业务错误（检索失败、生成失败等）一律以 `error` 事件下发；只有进入 SSE **之前**的失败（未启用 503、参数 400、限流 429）走普通 JSON。前端据此分两段处理。

引用结构 `Citation`（随 citations 事件给前端，前端渲染成可点击的出处卡片，跳 `/article/{slug}`）：

| 字段 | 含义 |
|---|---|
| title / slug | 来源文章标题与地址 |
| score | 向量相似度得分 |
| snippet | 命中片段前 300 字 |

## 提示词设计（rag.go `askSystemPrompt`）

system 只有三条硬规则：**仅依据参考片段回答**、句末标注引用编号如 `[1]`、**片段不足时明说"暂未找到相关内容"不要编造**。user 消息 = 编号片段 + 最近几轮 history + 本次问题。

设计意图：问答面向公众，最怕一本正经地胡说。把"可以说不知道"写进规则、无命中时根本不调 LLM，双保险压低编造率。

## 模型与端点（与主模型隔离）

- 问答对话模型由 `rag.chat_*` **单独配置**：当前 glm-4-flash（智谱免费档），走标准 OpenAI 兼容端点 `https://open.bigmodel.cn/api/paas/v4`。
- `chat_model` 留空则复用主模型（glm-4.6）——不推荐，问答调用量大，会烧主套餐额度。
- 问答也走 SSE 流式（llm/stream.go 解析增量），用户看到逐字出现的效果。
- 向量化用的 embedder 与索引侧是**同一个**——问题向量与文章向量必须出自同一模型同一维度，否则检索无意义。

## 限流实现（rag_handler.go `rateLimiter`）

- 进程内 map + 固定窗口：每 IP 每分钟 `limit` 次（路由装配时传 0 取默认 10）。
- 窗口过期后计数重置；map 超 10000 个 IP 时整体清空防内存泄漏。
- 局限：多实例部署时不共享（每实例各算各的）；重启清零。单机部署下够用。

## 降级与失败行为

| 场景 | 行为 |
|---|---|
| `rag.enabled=false` 或 Qdrant/embedding/chat 任一依赖缺失 | 接口 503「问答功能未启用」，发布/前台其余功能完全不受影响 |
| 问题为空 / 参数非法 | 400 |
| 超 IP 限流 | 429「提问太频繁」 |
| 问题向量化 / 检索 / 生成失败 | SSE `error` 事件，前端提示稍后重试 |
| 检索命中但分数全低于阈值 | 不调 LLM，直接下发「暂未找到相关内容」话术 |

## 配置项（config.yaml `rag:` 段）

```yaml
rag:
  enabled: true
  top_k: 5                    # 检索返回片段数
  score_threshold: 0.3        # 低于该相似度的命中视为噪声丢弃
  chat_model: glm-4-flash     # 留空复用主模型
  chat_base_url: https://open.bigmodel.cn/api/paas/v4
  chat_api_key: ***
  chat_max_tokens: 2048       # 单次回答长度上限
  # embedding_* / qdrant / chunk_* 见《ai-rag-indexing.md》
```

## 常见排查

- **回答全是"暂未找到相关内容"**：先确认库里有没有已发布文章（向量只来自 published）；再看 `score_threshold` 是否设得过高。
- **引用点开 404**：文章在回答后被下线/删除，属正常竞态；前端对失效 slug 应有兜底页。
- **SSE 变成一次性出全文**：中间有反代缓冲，检查 Nginx `X-Accel-Buffering` / `proxy_buffering off`。
- **问答消耗了主套餐额度**：`chat_model` 没单配，回落到了 glm-4.6。

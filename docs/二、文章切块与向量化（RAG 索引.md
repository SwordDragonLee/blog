# AI 功能二：文章切块与向量化（RAG 索引）

> 本文讲清「一篇已发布文章如何变成 Qdrant 里的向量」。
> 入口代码：`server/internal/service/article.go` 的 `indexForRAG` → `server/internal/service/rag.go` 的 `IndexArticle`。

## 一句话概述

文章**发布成功后**，后台 goroutine 把正文按标题切块 → 调 embedding 模型把每块变成 1024 维向量 → 连同原文写入 Qdrant，供前台技术问答检索。整个过程与发布接口异步、失败不影响发布。

## 时机：不在生成流水线里，而在「发版」之后

生成任务只把文章落库为 draft，此时 Qdrant 里什么都没有。向量化唯一的入口是发版：

```
管理员点「发版」 ArticleService.Publish        service/article.go
  ├─ 落库 draft → published、写 published_at
  ├─ 失效前台缓存
  ├─ 异步邮件通知
  └─ indexForRAG → go func → RagService.IndexArticle   ← 在这里
```

`indexForRAG`（article.go）的三个细节：

- `context.WithoutCancel(ctx)`：与 HTTP 请求上下文解耦，响应返回后切块向量化继续跑；
- 限时 60s（embedding 要对每块发请求，不能同步等）；
- 传**值拷贝**避免与调用方后续字段赋值竞争；失败仅 log.Warn，不影响发布结果。

## 四步流程（rag.go `IndexArticle`）

```
① RemoveArticle   清掉该文旧向量（幂等前提；失败不阻断，见下方 point ID 说明）
② chunkMarkdown   切块（纯本地字符串处理）
③ Embed           每块文本 → 1024 维向量（唯一的外部 API 调用）
④ UpsertChunks    连向量带原文写入 Qdrant（wait=true，返回即落盘）
```

### ② 切块规则（service/rag_chunk.go `chunkMarkdown`）

- 目标块长 `rag.chunk_size`（缺省 800 字符，中文按字符计）。
- **标题优先断开**：遇到 markdown 标题且当前块已过半（≥ chunk_size/2）时在此分块，保证块的语义完整性。
- 单块超长时强制切分；相邻块携带尾部重叠 `rag.chunk_overlap`（缺省 100，未配置时取 chunk_size/8），避免答案恰好被切断在边界上。
- 切好的块过滤掉不足 30 字的碎片（纯标题、纯空行）；切不出任何块（如空文章）则整个索引跳过。

### ③ 向量化（llm/embedding.go `Embedder`）

- 模型：SiliconFlow 免费的 **BAAI/bge-m3**，1024 维，OpenAI 兼容 `/embeddings` 端点。
- `embedding_base_url` / `embedding_api_key` 未单独配置时回落到主 LLM 配置（本项目主模型走 anthropic 形态端点，而 embedding 只有 OpenAI 形态，所以**必须单配**）。
- **不发 dimensions 参数**：`embedding_send_dimensions: false`。bge-m3 输出维度固定 1024，不接受该参数；智谱 embedding-3 这类可选维度模型才需要 `true`。
- 批量：一次请求携带全部块（`input` 数组），按响应中的 `index` 回填；返回条数与输入不一致、某条缺向量都视为失败。
- 网络错误指数退避重试（与 Chat 客户端同款，最多 3 次）。

### ④ 写入 Qdrant（qdrant/qdrant.go）

- REST 轻封装（仅 4 个操作），刻意不引官方 gRPC SDK 的依赖链。
- `EnsureCollection`：幂等创建集合，cosine 距离、维度取 `rag.embedding_dimensions`(1024)；已存在直接复用。**换 embedding 模型必须同步改维度并重建集合**。
- **确定性 point ID**：`pointID = articleID*10000 + chunkIndex`。重复发布时同 ID 自然覆盖，无需"先查后写"；块数变少时残留的旧块由步骤①的先删后写兜住。
- payload 随向量存原文与出处：`article_id`、`chunk_index`、`content`（块原文）、`title`、`slug`——检索命中后直接用 payload 组装引用，不再回查 MySQL。

## 向量生命周期闭环

| 事件 | 向量操作 | 代码 |
|---|---|---|
| 发版 | 异步切块入库 | article.go `indexForRAG` |
| 编辑已发布文章（退回 draft） | 立即删除 | article.go `RemoveArticle` |
| 下线 | 立即删除 | 同上 |
| 删除任务 | 兜底清扫（事务提交后按文章 ID 逐个删） | task.go `Delete` + `RagCleaner` 接口，bootstrap_services.go 注入 |

不变量：**Qdrant 里的向量始终只对应当前处于 published 状态的文章**。发版是唯一入口，退回/下线是出口，删任务是兜底（覆盖「发版后几十秒内立刻退回 draft」竞态窗口内异步 upsert 晚落地的孤儿向量）。

已知毫秒级窗口：upsert 落地前一瞬间文章恰好退回 draft 会留孤儿向量，删任务时清扫；概率极低，不做逐条防护。

## 配置项（config.yaml `rag:` 段）

```yaml
rag:
  enabled: true                  # false 或依赖缺失时：跳过索引，问答返回 503，发布不受影响
  chunk_size: 800                # 切块目标字符数
  chunk_overlap: 100             # 相邻块重叠
  embedding_base_url: https://api.siliconflow.cn/v1
  embedding_api_key: ***
  embedding_model: BAAI/bge-m3
  embedding_dimensions: 1024     # 必须与模型实际输出一致，换模型需重建 Qdrant 集合
  embedding_send_dimensions: false
```

Qdrant 连接独立一段（不在 rag: 下）：

```yaml
qdrant:
  base_url: http://localhost:6333
  collection: blog_articles   # 集合名，缺省 blog_articles
```

## 模型分工（为什么三个模型各干各的）

| 用途 | 模型 | 提供方 | 原因 |
|---|---|---|---|
| 文章生成 | glm-4.6 | 智谱 Coding Plan（anthropic 形态端点） | 质量最高，量小 |
| 向量化 | bge-m3 | SiliconFlow | 免费、中文效果好、1024 维 |
| 问答对话 | glm-4-flash | 智谱标准 paas/v4 端点 | 免费档；问答面向公众调用量大，不消耗生成套餐额度 |

## 常见排查

- **发版后问答搜不到新文章**：向量化是异步的（通常几秒~几十秒），稍等；查服务日志有无「文章向量化入库失败」。
- **检索结果语义错乱/报维度错误**：最近是否换过 embedding 模型？维度必须与集合一致，需删集合重建并全部重新发版。
- **验证 Qdrant 数据**：`curl http://localhost:6333/collections/blog_articles` 看 points_count。

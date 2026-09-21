# RAG 技术问答 设计文档

日期:2026-09-17
状态:已确认(Qdrant 向量库 + 前台聊天气泡)

## 背景与目标

平台已能生成并发布技术文章。新增「全站技术问答」:访客在博客前台向 AI 提问,
系统基于**已发布文章的向量检索结果**流式回答,并给出引用出处(可点击跳转原文)。

价值:前台内容消费闭环 + AI 应用经验(RAG/流式/引用),与现有架构(发布钩子、
LLM 客户端、门户只读路由)自然衔接。

## 关键决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 向量存储 | Qdrant(独立 docker 服务) | 用户选择;简历关键词;官方 REST API 简单 |
| 客户端封装 | 手写 REST(net/http),不用官方 go-client | go-client 拉整条 gRPC 依赖链;接口面仅 4 方法,与 mq 包手写风格一致 |
| Embedding | 智谱 embedding-3(OpenAI 兼容 /embeddings) | 复用现有 LLM Key;配置独立 embedding_base_url 以兼容 Anthropic chat 端点 |
| 量级判断 | 数千 chunk 暴力检索足够 | Qdrant HNSW 天然覆盖;不需要 MySQL 双写向量 |
| 问答入口 | 前台聊天气泡(公开,portal 路由) | 展示价值;admin 不做 |
| 多轮 | 仅带最近 3 轮文本上下文 | 控制成本与复杂度 |

## 数据流

### 写入(文章生命周期钩子)
```
publish        → 异步 IndexArticle:markdown 清理(去图片占位符)
                 → 按标题+段落切块(~800 字符,overlap 100)
                 → 批量 embedding → Upsert 到 Qdrant
offline        → RemoveArticle(按 article_id 过滤删除)
已发布再编辑退回 draft → RemoveArticle(内容将变,旧向量作废)
重新生成配图   → 不触发(文本未变)
```

### 查询(前台问答)
```
GET /api/v1/portal/ask?q=...(SSE,公开)
问题 embedding → Qdrant top_k=5(score 低于阈值丢弃)
→ prompt:系统规则(仅依据片段、引用编号、不足则明说)+ 片段 + 最近3轮上下文
→ LLM 流式输出,SSE 逐 token 推送
→ 结束时推送 citations 事件:[{title, slug, score, snippet}]
```

## 后端组件

1. **deploy/docker-compose.yml**:qdrant 服务(6333 REST,数据卷持久化)
2. **config**:新增 `qdrant`(base_url/collection)与 `rag`(enabled/embedding_base_url/
   embedding_model/embedding_dimensions/chunk_size/chunk_overlap/top_k/score_threshold)
3. **internal/llm**:`Embed(ctx, texts []string) ([][]float32, error)`,走 OpenAI 兼容
   /embeddings,与 chat 的 Anthropic 分流解耦
4. **internal/qdrant**(新包):`EnsureCollection / UpsertChunks / Search / DeleteByArticle`,
   point payload 携带 {article_id, chunk_index, content, title, slug},命中免回库
5. **internal/service/rag.go**(新):IndexArticle / RemoveArticle / Ask(流式回调);
   钩子挂在 article service Publish/Offline/Update(退 draft)处,异步执行、失败仅记日志
6. **internal/api/rag_handler.go**(新):SSE handler(`text/event-stream`),
   事件:`delta`(token)、`citations`(JSON)、`done`;简单 IP 令牌桶限流防滥用

## 前台(web/blog)

`components/AiChat.tsx`(client 组件)+ 根布局挂载:
- 右下角浮动按钮 → 聊天面板(桌面 380px 卡片,移动端近全屏)
- fetch + ReadableStream 手解 SSE;react-markdown + rehype-highlight 渲染(与文章页一致)
- citations 渲染为引用卡片(标题 + 相关度),点击跳 `/article/{slug}`
- 请求体带最近 3 轮对话文本;会话历史仅存内存

## 配置示例(config.yaml 追加)

```yaml
qdrant:
  base_url: http://localhost:6333
  collection: blog_articles
rag:
  enabled: true
  embedding_base_url: ""     # 空 = 复用 llm.base_url;带 /anthropic 时必填
  embedding_model: embedding-3
  embedding_dimensions: 2048
  chunk_size: 800
  chunk_overlap: 100
  top_k: 5
  score_threshold: 0.3
```

## 验证

1. `cd deploy && docker compose up -d qdrant` → 健康检查
2. 发布一篇文章 → Qdrant collection 出现对应 points(payload 正确)
3. curl SSE 提问(文章内容相关问题)→ 流式回答内容贴合、citations 指向该文
4. 文章下线 → points 被删除;再次提问不再引用
5. 前台浮窗 E2E:提问 → 流式渲染 → 引用跳转
6. 边界:rag.enabled=false 时接口 404/提示关闭;Qdrant 不可用时提问降级报错不崩

## 明确不做

- 用户体系/鉴权(公开只读问答)
- 深度多轮对话与记忆
- 混合检索(BM25)/重排(reranker)——量级不需要,留作演进
- config.yaml 不提交(含真实 Key,项目约定)

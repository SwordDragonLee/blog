# 第三方服务调用速查

项目所有外部 HTTP 依赖的端点拼接与官方文档地址。base_url 等非密钥配置在 `server/config.yaml`，密钥在 `server/.env`（gitignored）。

## 端点拼接规则

代码统一为 `strings.TrimSuffix(base_url, "/") + 后缀`，协议由 base_url 自动判别（含 `/anthropic` 即走 Anthropic Messages 格式）：

| 用途 | 拼接后缀 | 代码位置 |
|---|---|---|
| 文章生成（Anthropic 协议） | `/v1/messages` | `internal/llm/client_anthropic.go` |
| 生成 / RAG 问答（OpenAI 兼容） | `/chat/completions` | `internal/llm/client.go` |
| 向量化 | `/embeddings` | `internal/llm/embedding.go` |
| 检索精排 | `/rerank` | `internal/llm/rerank.go` |
| 向量库操作 | `/collections/...`（upsert/delete/search/scroll） | `internal/qdrant/` |

## 当前在用的厂商端点

| 厂商 | base_url | 模型 | 用途 | 套餐 |
|---|---|---|---|---|
| 智谱 | `https://open.bigmodel.cn/api/anthropic` | glm-5.3 | 文章生成 | Coding Plan |
| 智谱 | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash | RAG 问答 | 免费档 |
| SiliconFlow | `https://api.siliconflow.cn/v1` | BAAI/bge-m3 | 向量化（1024 维） | 免费 |
| SiliconFlow | `https://api.siliconflow.cn/v1` | BAAI/bge-reranker-v2-m3 | 检索精排 | 免费 |
| Qdrant（自托管容器） | `http://localhost:6333` | — | 文章向量存储与检索 | — |

## 官方文档

| 服务 | 文档地址 | 说明 |
|---|---|---|
| 智谱开放平台 | <https://docs.bigmodel.cn> | 全部模型 API 参数与定价；Coding Plan 的 anthropic 兼容端点用法；控制台 <https://open.bigmodel.cn> 看模型广场与额度 |
| SiliconFlow | <https://docs.siliconflow.cn/cn/api-reference> | embeddings / rerank 端点参数；控制台 <https://cloud.siliconflow.cn> 看模型广场与 model ID |
| Anthropic（协议官方参考） | <https://docs.anthropic.com/en/api/messages> | 本项目不直连 Claude，仅作 Messages 协议格式参考 |
| Qdrant | <https://api.qdrant.tech> | REST API 交互式参考；概念文档 <https://qdrant.tech/documentation/concepts/>（points / search / filtering / collections） |

## 注意事项

- bge-m3 **不接受 `dimensions` 参数**，`rag.embedding_send_dimensions` 必须为 false；换 embedding 模型需同步改维度并重建 Qdrant 集合。
- 智谱 `/api/anthropic`（Coding Plan 套餐）与 `/api/paas/v4`（API 余额计费）**额度相互独立**：生成走前者、公众问答走后者，避免问答量消耗生成套餐。
- Qdrant 官方 Go SDK 是纯 gRPC 客户端（:6334），本项目 compose 只映射 REST :6333，故用轻量 REST 封装（见 `internal/qdrant/qdrant.go` 包注释）。

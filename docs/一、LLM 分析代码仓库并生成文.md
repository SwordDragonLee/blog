# AI 功能一：LLM 分析代码仓库并生成文章

> 本文面向后续维护者与 AI 助手，完整讲清「输入一个 Git 仓库地址 → 产出 4 篇带配图的中文草稿文章」的全链路。
> 入口代码：`server/internal/task/pipeline.go` 的 `Pipeline.Handle`。

## 一句话概述

管理端创建生成任务后，任务不直接执行，而是落库 + 投递 RabbitMQ 消息；消费者拉起流水线：**克隆仓库 → 静态分析采样代码 → LLM 第一轮分析选题 → 逐篇 LLM 写作 → Go 确定性渲染 SVG 配图 → 4 篇文章落库为 draft**，全程实时进度写 Redis 供管理端轮询。

## 全链路

```
管理端 POST /tasks
  ├─ TaskService.Create        service/task.go：校验 git_url → gen_task 落库(pending) → 投递 MQ 消息
  └─ MQ consumer               mq/consumer.go：QoS=1 逐条消费 → Pipeline.Handle
       ├─ ① 克隆仓库    5%   analyzer.Clone
       ├─ ② 仓库分析   15%   analyzer.Detect + analyzer.Sample + buildDigest
       ├─ ③ LLM 分析   30%   p.analyzeRepo → llm.ChatJSON（第一轮）
       ├─ ④ 逐篇写作   30~90% p.writeArticle → llm.ChatJSON（第二轮，每篇一次）
       ├─ ⑤ 渲染配图         svggen.Render（纯 Go，无外部调用）
       └─ ⑥ 完成     100%   saveArticle + saveFiguresAndCover（文章落库为 draft，任务结束；
                             审核走文章状态机 draft→published，不属于任务阶段）
```

### ① 克隆仓库（analyzer/git.go）

- `git clone --depth 1 --single-branch` 浅克隆到 `task.workdir`（默认 `tmp/`），结束 `defer Cleanup` 删目录。
- 强制 `-c http.version=HTTP/1.1`：大仓库在丢包网络下 HTTP/2 易中途断流（`error: RPC failed` / `fatal: early EOF`）。
- 超时由 `task.clone_timeout_seconds` 控制（当前 600s）。`cmd.WaitDelay = 10s`：Windows 下超时杀掉 git.exe 后子进程 git-remote-https 仍握着 stderr 管道，不加它 `Wait` 会永久阻塞、任务卡死在克隆步骤。
- `task.proxy` 非空时向 git 子进程注入 `HTTP_PROXY/HTTPS_PROXY`（如 `http://127.0.0.1:7890`），github 直连不可达的场景走本机代理；留空直连。
- **错误分类**（`classifyCloneErr`）：超时（区分克隆超时与任务主动取消）→ 仓库不存在/需认证（`errClauses` 关键词）→ 网络异常 → 兜底。错误文案用 `errLine` 从 git stderr 里挑**真正的报错行**（按 `error:`/`fatal:`/`RPC failed` 等关键词匹配，首行 `Cloning into 'xxx'...` 是进度提示不是错误）。

### ② 静态分析采样（analyzer/detect.go、sample.go）

- `Detect`：语言统计（按扩展名）、目录树、README 摘要、识别技术栈。
- `Sample`：按预算采样代码文件——上限 `task.max_files`(30) 个、单文件截断 `task.max_file_kb`(16KB)、总量 `task.total_budget_kb`(120KB)。
- `buildDigest` 把以上材料按 `llm.DigestSection` 分节拼成一段文本（即 `digest`），是喂给 LLM 的全部代码材料。**LLM 看到的仓库就是这个摘要**，所以预算参数直接决定分析质量与 token 消耗。

### ③ LLM 第一轮：分析选题（pipeline.go `analyzeRepo`、llm/prompt.go `BuildAnalysisPrompt`）

- system：资深架构师人设 + 严格 JSON 输出约束。
- user：仓库材料 + 固定 JSON schema。**4 篇选题是固定的**（技术栈全景 / 架构设计解析 / 优雅代码赏析 / 工程实践总结），模型只润色标题和大纲，保证任何仓库都产出结构一致的专栏。
- 输出 `AnalysisOut`：repo_name、tech_stack、summary、highlights、article_plan（title/slug/summary/tags/outline/target_words）。
- 落库 `repo_analysis` 表（含 git_url、head commit、技术栈快照）。

### ④ LLM 第二轮：逐篇写作（pipeline.go `writeArticle`、prompt.go `BuildArticlePrompt`）

- 输入：仓库名 + 第一轮分析结果原文（JSON）+ 本篇选题。
- 输出 JSON：`{title, slug, summary, tags, markdown, figures}`。
- 正文要求：达到 target_words ±20%、必须引用采样材料里**真实存在**的代码片段、在正文插入 `{{figure:fig-1}}` 占位符（独立成段）。
- `figures` 数组只输出**结构化图表数据**（spec JSON），按 kind 四种：`architecture`（分层+连线）/ `flow`（步骤）/ `compare`（左右对比）/ `timeline`（时间线）。
- **语义校验**（失败会把错误反馈给模型重写，见下节 ChatJSON）：标题/slug/markdown 非空；字数 ≥ `minArticleWords`(1000)；至少 1 张配图；figure id 不重复、kind 合法。

### ⑤ 渲染配图（svggen/）

LLM 不画图，只出数据；`svggen.Render` 把 spec JSON 确定性渲染为统一暗色科技风 SVG。渲染失败仅跳过该图（log.Warn），不阻塞整篇。另渲染封面卡片（`RenderCover`）存入 `svg_asset` 并回填 `article.cover_asset_id`。前台经 `/portal/figures/:file.svg` 引用。

### ⑥ 入库（pipeline.go `saveArticle` / `saveFiguresAndCover`）

- **按 slug upsert，重跑安全**：slug 已存在则覆盖内容并强制回到 draft、同步删旧配图；不存在则新建。slug 撞车时 `uniqueSlug` 加后缀重试 3 次。
- 幂等清理 `cleanupPreviousDrafts`：开始写作前删除本任务历史尝试遗留的草稿（已发布的不动），避免重试后新旧混杂。
- 完成后任务标记 success，文章等管理员审核发版。

## LLM 客户端机制（llm/client.go、client_anthropic.go）

两次 LLM 调用都走 `ChatJSON`，它的可靠性是三条循环叠加：

| 循环 | 次数 | 处理什么 |
|---|---|---|
| `ChatJSON` 校验重试 | `maxOutputAttempts`=3 | 模型输出不是合法 JSON 或没过语义校验 → 把错误信息拼进对话反馈给模型重写 |
| `chatWithRetry` | `maxCallRetries`=3 | 网络/HTTP 错误，指数退避（1s/2s/4s） |
| MQ 消费重投 | `task.max_retry`=3 | 整个 Handle 失败后消息重入队再跑一遍（见下） |

- **协议自动切换**：base_url 含 `/anthropic` 走 Anthropic Messages 格式（`x-api-key` 头、system 提顶层字段、关闭 thinking）；否则走 OpenAI 兼容格式（`Authorization: Bearer`、`response_format: json_object`）。两者只是 JSON 字段形状不同，请求都发给同一厂商（本项目为智谱：glm-4.6 走 Coding Plan 的 `/api/anthropic` 端点）。
- **限流**：`throttle` 基于 Redis 固定窗口（`llm.rate_per_minute`），令牌耗尽时等待重试；Redis 不可用直接放行不阻塞。
- **"调用 API"的本质**：`http.NewRequestWithContext` 拼 URL（base_url+`/chat/completions` 或 `/v1/messages`）→ 塞认证头 → JSON body → `c.http.Do` 发出。没有 SDK，手写的这几行与官方 SDK 内部等价。

## 失败、重试与日志留痕

- 流水线任何一步失败 → `p.fail`：把**真实错误**写入任务日志（`任务失败：%v`）、进度 hash 和 `gen_task.error`（截 1000 字符），然后返回 err 交给 MQ 层决策。
- consumer `retryOrFail`：未超 `max_retry` 则带 `x-retry-count` 头重投，并向任务日志追加「第 N 次执行失败：…」「将自动重试」两行（`consumer.Logf` → `task.AppendLog`）；超限进死信队列 `blog.task.dlq`，`OnDead` 回调（app/ondead.go）把任务标 failed，错误文案保留最后一次真实错误（`重试超限，最后一次错误：…`）。
- **取消**：任务取消/删除返回 `mq.ErrTaskCanceled` 哨兵，consumer ack 丢弃消息、不重试。服务停机的失败不 ack，RabbitMQ 自动重投。
- 每次进入 Handle 都会 `INCR task:{id}:attempts`，日志出现「══ 第 N 次尝试 ══」分隔线，手动重试时清零该计数。

## 进度与日志（task/progress.go）

| Redis key | 类型 | 内容 |
|---|---|---|
| `task:{id}:progress` | hash | step/percent/message，管理端 2s 轮询 `GET /tasks/:id` |
| `task:{id}:logs` | list | 带时间戳的执行日志，上限 200 条、TTL 7 天 |
| `task:{id}:attempts` | 计数 | 第几次尝试（手动重试归零） |

进行中任务以 Redis 覆盖 DB 展示实时进度；终态以 DB 为准。

## 配置项（config.yaml）

```yaml
task:
  workdir: tmp                    # 克隆临时目录
  clone_timeout_seconds: 600      # 克隆超时（大仓库需要长超时）
  max_files: 30                   # 采样文件数上限
  max_file_kb: 16                 # 单文件截断
  total_budget_kb: 120            # 采样总预算
  article_count: 4                # 每次生成文章数
  concurrency: 1                  # 消费并发
  max_retry: 3                    # MQ 失败重投次数
llm:
  base_url / api_key / model      # 主生成模型（glm-4.6，anthropic 格式端点）
  temperature / max_tokens / timeout_seconds / rate_per_minute
```

## 关键设计决策

1. **异步 MQ 驱动**而非同步执行：整个流程 10+ 分钟（克隆+多次 LLM 调用），必须异步；RabbitMQ 持久化消息 + 死信队列保证进程重启不丢任务。
2. **prompt 约定 JSON 而非 function calling**：OpenAI 兼容与 Anthropic 两套协议的 tool 语法不同，纯 prompt 约定 + 校验重试在两种协议下行为一致，还兼容任意"OpenAI 兼容"厂商。
3. **LLM 只出图表数据、Go 渲染 SVG**：模型输出的图不可复现、易畸形；结构化 spec + 确定性渲染保证同一数据永远画出同一张图，且可单图重生成。
4. **幂等优先**：slug upsert、配图按 article_id 重建、attempt 计数——消息重投、任务重试、进程崩溃恢复都安全。

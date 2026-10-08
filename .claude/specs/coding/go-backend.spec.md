# Go 后端编码规范（server/）

> 硬约束。存量代码不符合时按「顺手修复、单独提交」处理，不做无关重构。

## 分层边界

`router → api（薄 handler）→ service（业务）→ repo / mq / task`，依赖只允许自上而下：

- **禁止** api 层写业务逻辑、直接持有 `*gorm.DB` / `*redis.Client`——数据访问只在 repo / service
- **禁止** repo 反向引用 service；跨模块复用走依赖注入（参照 `app/` 的手工装配，经 `router.Deps` 传递）
- handler 只做：参数绑定 → 调 service → `resp/` 封装响应

```go
// ❌ 禁止：handler 里直接查库
func (h *ArticleHandler) List(c *gin.Context) {
    var arts []model.Article
    h.db.Find(&arts) // api 层持有 db，越界
}

// ✅ 正确：handler 薄，业务在 service
func (h *ArticleHandler) List(c *gin.Context) {
    arts, err := h.articleSvc.List(c.Request.Context())
    if err != nil {
        resp.Error(c, err)
        return
    }
    resp.OK(c, arts)
}
```

## context：一切阻塞操作可中断

- HTTP 请求（`http.NewRequestWithContext`）、子进程（`exec.CommandContext`）、DB（`db.WithContext(ctx)`）**必须**携带 ctx——管理端取消任务、服务停机依赖它即时打断
- 可中断的定时等待用 `select` + `time.After`，**禁止**裸 `time.Sleep`（停机信号无法打断）

```go
// ❌ 禁止：Sleep 期间停机/取消无法打断
time.Sleep(reconnectDelay)

// ✅ 正确：ctx 取消立即返回（见 mq/consumer.go workerLoop）
select {
case <-ctx.Done():
    return
case <-time.After(reconnectDelay):
}
```

- 需要区分「主动取消」与「真实失败」时用 sentinel 错误（参照 `mq.ErrTaskCanceled` + `errors.Is`）；**cancel() 要在判断 dctx.Err() 之前执行的位置问题上警惕**：先释放再判断会把一切失败误判为取消

## 错误与日志

- 错误信息用中文，向上包装一律 `fmt.Errorf("...: %w", err)` 保留错误链，**禁止**丢弃原错误
- 日志一律 `zap`（结构化字段），**禁止 `fmt.Println` 调试残留进入提交**（本地调试完即删）
- HTTP handler 内不直接打日志再返回错误——错误经 `resp/` 统一处理，日志在 service / 中间件层记

## 并发与幂等

- 后台 goroutine 必须有退出通道（ctx 或 done channel），**禁止**裸 `go func(){}()` 悬挂
- 涉及 MQ 重投 / 任务重跑的写库操作必须幂等，三种成熟模式（见 `task/pipeline.go`、`service/rag.go`）：
  - 唯一业务键 upsert（文章按 slug）
  - delete-then-write（配图按 article_id 重建）
  - 确定性 ID 覆盖（Qdrant point ID = articleID*10000+chunkIndex）

## 配置与密钥

- **密钥/密码只进 `.env`**（已被 gitignore）；`config.yaml` 不出现任何私密项
- 新增配置项：`config/` 结构体 + yaml 默认值 + `.env.example` 同步补模板；环境变量覆盖规则 = 点号换下划线（`llm.api_key` → `LLM_API_KEY`）

## 注释与命名

- 标识符英文，注释中文；导出符号必须有注释（godoc 风格，从 `// 函数名` 开头）
- 包注释写在唯一一个文件顶部，说明职责而非实现

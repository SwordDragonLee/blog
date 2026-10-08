# Git 规范

## 分支模型

单分支主干开发：日常开发直接在 `main` 上进行，push 到 `origin/main` 即触发 GitHub Actions 按路径过滤构建三端镜像并部署（`.github/workflows/deploy.yml`；`workflow_dispatch` 为强制全量入口）。**main 任何时刻都必须处于可构建、可发布状态**——提交前完成验证闭环（见 `project-constraints.spec.md`）。

大改动需要长期实验时才开分支，做完尽快合回 `main`，不留长命分支。不使用 MR / squash 流程。

## Commit Message

格式：`<type>: <中文具体描述>`。描述必须具体到「做了什么」，重要决策可括号补一句「为什么」：

```bash
# ✅ 正确（摘自本仓历史）
feat: 接入 CI/CD（GitHub Actions 构建推 TCR + SSH 部署，三端镜像参数化）
fix: 新建生成任务成功后关闭弹窗
fix: Qdrant 集合尚未创建时优雅降级（总览空列表、检索无命中、删除幂等）
docs: 补强取消链路与请求细节的关键注释

# ❌ 错误（描述不清晰）
fix: 修改bug
update
feat: 完成功能
```

## type 类型

| type       | 用途                           |
| ---------- | ------------------------------ |
| `feat`     | 新增功能                       |
| `fix`      | 修复 bug                       |
| `docs`     | 仅文档修改                     |
| `style`    | 格式调整（不影响逻辑）         |
| `refactor` | 重构（不新增功能，不修复 bug） |
| `perf`     | 性能优化                       |
| `chore`    | 构建流程、依赖变更             |
| `revert`   | 回滚                           |

## 提交粒度

- 一次提交只做一件事；代码与其注释/文档同步更新，不留「下一条提交再补」
- 强相关的小变更可并入一条，用「A + B」连接（如 `feat: Swagger/OpenAPI 接口文档集成 + datatypes.JSON 收编为 model.JSON`）
- 存量代码违反红线的顺手修复**单独提交**，不混入功能提交

## Git 红线

Git 安全类红线（未授权不 commit/push、禁 `--no-verify`、禁调试残留入提交）统一见 `project-constraints.spec.md`，此处不重复维护。

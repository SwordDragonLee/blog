# 命令列表

## 主流程（commands/coding/）— 默认用这套

| 命令 | 用途 |
|------|------|
| `/spec` | 生成功能规范 |
| `/plan` | 技术方案 |
| `/tasks` | 拆任务 |
| `/code` | 按任务写代码 |
| `/verify` | 对照 specs 校验 |

```
/spec → /plan → /tasks → /code → /verify
```

## 项目工具（commands/ 顶层）

| 命令 | 用途 |
|------|------|
| `/check` | 一键质量检查（vet + build + 前端 tsc/build） |
| `/commit` | 按项目规范生成中文提交信息并提交 |

## superpowers（commands/superpowers/）— 可选增强

| 命令 | 用途 |
|------|------|
| `/brainstorm` | 需求澄清 |
| `/sdd-review` / `/sdd-verify` / `/sdd-debug` | SDD 审查/校验/调试变体 |

与 `coding/` 重叠时 **优先 `coding/`**；仅在用户点名 superpowers 或需要 brainstorm 时用后者。

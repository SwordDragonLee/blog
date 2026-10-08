# AI 全局行为准则

## 语言

- 所有输出使用中文，代码和代码注释除外
- 专有名词保留英文（Go、Gin、GORM、RabbitMQ、React、Next.js、Qdrant 等）

## 行为约束

- **禁止**未经用户明确授权执行 `git commit` / `git push`
- 硬约束（必须/禁止）以 `.claude/specs/` 为准，不自行发明规范
- 项目地图与命令见根目录 `CLAUDE.md`；架构细节按需读 `docs/architecture.md`，第三方端点拼接与厂商坑读 `docs/third-party-apis.md`
- 生成代码前先 Read 现有文件与相邻模块，了解上下文，不盲目覆盖
- 修改存量代码时只改必要的部分，不做无关重构
- 不确定时主动提问，不假设需求

## 规范冲突处理

优先级（高 → 低）：

1. `specs/common/project-constraints.spec.md`（绝对禁止项）
2. `specs/coding/`、`specs/common/` 专项/通用 spec
3. `.claude/skills/`、`.claude/memory/`（流程与未升格笔记；与 spec 冲突时以 spec 为准）

同层冲突：更具体的规则优先；同等具体时以后制定的为准。
**specs 与源码事实冲突时**：以源码为准，并应修正 spec（禁止默默跟错文档）。

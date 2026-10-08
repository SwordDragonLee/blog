# 文档目录规范

`docs/` 是**事实文档层**（与 `.claude/specs/` 的规范层相对）：写代码推不出的东西——设计动机、部署环境、第三方端点的坑。文档与源码冲突时以源码为准，并更新文档。

## 设计文档（SDD `/spec` 产物）

- 路径：`docs/superpowers/specs/{YYYY-MM-DD}-{slug}-design.md`（slug = 英文 kebab-case 简述）
- 格式对齐既有样例（无 YAML frontmatter）：`# {标题} 设计文档` + `日期:` + `状态:` 行，正文含 背景与目标 / 关键决策（表格，多方案取舍在此留痕）/ 数据流 / 组件与接口 / 边界情况 / 验证与回滚（按需取舍）
- 已有样例：`2026-09-14-ai-blog-platform-design.md`、`2026-09-17-rag-tech-qa-design.md`
- 后续链路产物同目录平铺：`-plan.md`（/plan）、`-tasks.md`（/tasks），不建子目录
- 设计文档实现完成后**保留**，作为决策历史

## 长期事实文档（随源码维护）

- `docs/architecture.md` —— 架构与实现全貌；改源码顺手更新对应小节
- `docs/third-party-apis.md` —— 第三方端点拼接规则与厂商坑；新接外部服务时同步登记
- `docs/deployment-guide.md` 等运维手册 —— 部署 / CI 拓扑变更时更新

## 约束的归属

必须/禁止类约束进 `.claude/specs/`，不写进 `docs/`；`docs/` 只陈述事实。

## 杂项

- 新增专题文档：英文 kebab-case 命名，放 `docs/` 根或主题子目录，不建深层嵌套
- 一次性草稿、会话笔记不进 `docs/`（放 `.claude/memory/` 或会话内），避免堆积失修文档

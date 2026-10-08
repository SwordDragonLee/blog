# /plan - 生成技术方案

## 使用方式

```
/plan {YYYY-MM-DD}-{slug}
```

示例：`/plan 2026-09-17-rag-tech-qa`

## 执行步骤

1. 在 `docs/superpowers/specs/` 下查找对应的 `-design.md` 文档（匹配 `{YYYY-MM-DD}-{slug}-design.md`）
2. 读取设计文档，分析涉及的现有模块和文件
3. 生成技术方案，包含：
   - 文件变更清单（新增 / 修改，后端按 `router → api → service → repo/model/config` 分层列出）
   - 接口与数据契约（路由注册点、GORM 模型与 AutoMigrate 影响）
   - 前端方案：admin 页面 / 组件与 store 变更；blog 页面（SSR / ISR、generateMetadata）与客户端组件
   - 复用清单（`llm/`、`svggen/`、`qdrant/`、`mq/` 现有封装）
   - 配置变更（`config/` 结构体 + `config.yaml` 默认值 + `.env.example` 三处同步）
   - 缓存与一致性影响（Redis 前台缓存失效、Qdrant 向量、MQ 消息与幂等）

## 输出

将方案文档写入设计文档**同目录**的 `{YYYY-MM-DD}-{slug}-plan.md`（平铺，不建子目录）。

## 约束

- 必须引用相关 `.claude/specs/` 文件作为依据
- 优先复用现有组件与 service 封装
- 标注每个文件的修改类型（新增 / 修改）

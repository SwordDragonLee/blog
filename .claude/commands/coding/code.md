# /code - 按任务生成代码

## 使用方式

```
/code {YYYY-MM-DD}-{slug} [任务编号]
/code {YYYY-MM-DD}-{slug}
```

示例：
```
/code 2026-09-17-rag-tech-qa 3
/code 2026-09-17-rag-tech-qa
```

## 执行步骤

1. 在 `docs/superpowers/specs/` 下查找对应文档（匹配 `{YYYY-MM-DD}-{slug}-*.md`）
2. 优先读取 `-tasks.md`，如果不存在则直接读取 `-plan.md` 作为执行依据，同时读取 `-design.md` 了解需求上下文
3. 如果指定了任务编号，只执行该任务（单任务模式）
4. 如果未指定任务编号，按 Wave 分组并行执行（多 Agent 模式）：
   - 读取 tasks.md 中的 Wave 分组
   - 同一 Wave 内，对每个任务启动独立 Agent（`subagent_type: "general-purpose"`）
   - 同一 Wave 的所有 Agent 在一次响应中并行发起
   - 等待当前 Wave 全部 Agent 完成后，再执行下一个 Wave
5. 每个 Agent 完成后：
   - 收集返回的修改文件列表
   - 确认同一 Wave 内无文件冲突（两个 Agent 不应修改同一文件）
   - 如有冲突，记录并报告
6. 全部 Wave 完成后，输出总结：修改了哪些文件、是否有冲突需人工处理
7. 对于每个任务，Agent 按规范和模板生成代码，生成后进行自检

## 约束

- 每个文件生成前先用 Read 工具检查是否已存在
- 生成后必须符合 `specs/common/` 和 `specs/coding/` 中所有规范
- 不得修改任务范围之外的文件

## Agent Prompt 模板

每个 Agent 的 prompt 必须自包含，不依赖会话上下文，包含以下部分：

1. **任务描述**：从 tasks.md 摘录（标题、涉及文件、验收标准）
2. **前置 Wave 产出摘要**（如有）：文件路径、导出的函数/组件名、关键 API 签名
3. **必须遵守的 spec 规则**：关键条款摘要（非全文，避免 prompt 过长）
4. **约束**：不得修改任务范围之外的文件

## 波次间上下文传递

- 每个 Wave 完成后，主 Agent 收集所有子 Agent 的返回结果
- 提取摘要：文件路径、导出的函数/组件名、关键 API 签名（如函数参数、组件 props）
- 启动下一个 Wave 时，将摘要注入到对应 Agent 的 prompt 中
- 子 Agent 不需要知道 Wave 概念，只需知道「这些文件已存在，可以直接 import」

## 冲突预防

- 同一 Wave 内的任务禁止修改相同文件
- 如发现任务间存在文件冲突，将其中一个移到下一 Wave

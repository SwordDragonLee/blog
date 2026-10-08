# /superpowers:brainstorm - 需求探索

基于 Superpowers brainstorming 技能，探索需求意图、对齐技术方案，产出结论后可衔接 `/spec` 生成规范文档。

## 使用方式

```
/superpowers:brainstorm {需求描述或问题}
```

## 执行步骤

1. 通过 Skill 工具加载 `superpowers:brainstorming`，按其流程执行需求探索
2. 探索过程中涉及的代码现状（存量模块、组件、接口），先 Read 再讨论
3. 探索结束后，输出结论摘要：
   - 功能范围（做什么、不做什么）
   - 涉及的存量模块和文件
   - 技术方案倾向
   - 需要确认的遗留问题（如有）

## 衔接核心流程

探索结论可直接作为 `/spec` 的输入，跳过 `/spec` 的讨论模式，选择"直接生成文档"。

## SDD 约束

探索过程中产出的任何代码片段、文件路径建议、技术方案，必须遵守 `.claude/specs/` 下的规范。

# /superpowers:sdd-debug - 排查 bug

基于 Superpowers systematic-debugging 技能，系统化排查和修复 bug。

## 使用方式

```
/superpowers:sdd-debug {问题描述或错误信息}
```

## 执行步骤

1. 通过 Skill 工具加载 `superpowers:systematic-debugging`，按其流程执行调试
2. 定位问题根因后，提出修复方案
3. 实施修复前，先 Read 相关文件了解上下文，确认修复不会破坏存量逻辑
4. **修复代码前，必须先用 Glob + Read 工具读取 `.claude/specs/` 下所有 `.spec.md` 文件**，确保遵守全部规范（新增 spec 后无需改此命令，自动覆盖）：
   - `.claude/specs/common/*.spec.md`
   - `.claude/specs/coding/*.spec.md`
5. 修复后的代码必须同时符合 Superpowers 调试流程和上述 spec 规范，两者冲突时以 spec 为准

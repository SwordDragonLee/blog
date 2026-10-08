# /superpowers:sdd-verify - 完成前自检

基于 Superpowers verification-before-completion 技能，在声称完成工作之前进行验证。

## 使用方式

```
/superpowers:sdd-verify
/superpowers:sdd-verify {文件或目录路径}
```

## 执行步骤

1. 通过 Skill 工具加载 `superpowers:verification-before-completion`，按其流程执行验证
2. 验证维度分为两层：
   - **Superpowers 层**：功能是否按需求实现、是否有明显遗漏（由技能本身覆盖）
   - **SDD 层**：编码规范是否合规（由本命令补充）

## SDD 验证清单

验证时额外检查：

- [ ] 后端改动是否通过 `task vet` + `task build`
- [ ] 前端改动是否通过对应目录 `npm run build`（含 tsc）
- [ ] 阻塞操作是否携带 ctx；可中断等待是否用 select 而非 time.Sleep
- [ ] 是否有 `fmt.Println` 调试残留
- [ ] 密钥是否只在 `.env`；提交内容无明文私密项
- [ ] 前端请求走统一封装，组件内无硬编码后端地址
- [ ] 是否修改了任务范围之外的文件
- [ ] 完整规范见 `.claude/specs/` 目录

## 原则

先运行验证命令或检查，确认输出通过后，再声称工作完成。证据先于断言。

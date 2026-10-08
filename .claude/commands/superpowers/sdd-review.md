# /superpowers:sdd-review - 代码审查

基于 Superpowers requesting-code-review 技能，对代码进行审查。

## 使用方式

```
/superpowers:sdd-review {文件或目录路径}
/superpowers:sdd-review              # 审查当前分支所有变更
```

## 执行步骤

1. 通过 Skill 工具加载 `superpowers:requesting-code-review`，按其流程执行审查
2. 审查维度分为两层：
   - **Superpowers 层**：逻辑正确性、边界情况、错误处理（由技能本身覆盖）
   - **SDD 层**：编码规范合规性（由本命令补充）

## SDD 审查清单

审查前，必须先用 Glob + Read 工具读取 `.claude/specs/` 下所有 `.spec.md` 文件，逐条对照检查（新增 spec 后无需改此命令，自动覆盖）：

- `.claude/specs/common/*.spec.md`
- `.claude/specs/coding/*.spec.md`

审查时额外检查以下高频项是否合规：

- [ ] Go 新增代码是否遵守分层：api 层无业务逻辑、repo 不反向引用 service
- [ ] 阻塞操作（HTTP 请求 / exec 子进程 / DB）第一个参数是否传 ctx
- [ ] 是否有 `fmt.Println` 调试残留（日志一律 zap）
- [ ] 错误信息是否中文、以 `%w` 包装上抛、经 `resp/` 统一封装
- [ ] 密钥/密码是否只出现在 `.env`（config.yaml 与代码中不得有明文）
- [ ] 前端请求是否走统一封装（admin 走 `api/http.ts`，blog 服务端走 `lib/api`），无裸写后端地址
- [ ] 文件命名和目录结构是否符合 `naming-structure.spec.md`

## 输出

按严重程度分级输出：
- **阻塞**（必须修复）：违反 `project-constraints.spec.md` 中的绝对禁止项
- **建议**（推荐修复）：不符合最佳实践但不影响功能

---
description: 按项目规范生成中文提交信息并提交
allowed-tools: Bash(git *), Read
---

# 规范提交

1. 运行 `git status --porcelain` 和 `git diff --stat` 了解本次改动内容。
2. 判断改动类型，提交信息格式为：`<类型>: <一句话中文描述>`

   类型取值：feat / fix / refactor / docs / chore / test / style

   示例：`feat: 前台文章列表支持标签筛选`

3. 检查待提交内容：
   - `server/.env`、`*.local.*`、`settings.local.json` 等私密/个人文件绝不能进暂存区，发现则中止并警告。
   - `config.yaml` 若有 diff，逐行确认不含密钥/密码后再提交。
4. 分批提交：不相关的改动拆成多次提交，每次只做一件事。
5. 用 HEREDOC 方式写提交信息，只提交，不推送（push 由用户决定）。

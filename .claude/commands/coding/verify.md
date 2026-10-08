# /verify - 规范合规检查

## 使用方式

```
/verify server/internal/service/
/verify web/admin/src/pages/CreateTaskModal.tsx
/verify server/internal/mq/ --spec project-constraints
```

## 执行步骤

1. 解析输入，确定检查目标（文件或目录）和可选的 spec 过滤参数
2. 读取 `specs/common/` 和 `specs/coding/` 下的所有 spec 文件（如指定了 `--spec` 则只读取对应文件）
3. 对目标文件逐项检查每条规则，判断合规性
4. 输出检查报告

## 输出格式

```
## 合规检查报告

目标: server/internal/service/
检查时间: 2026-10-02

### 合规项 (5)
- [PASS] 阻塞操作携带 ctx → go-backend.spec.md
- [PASS] 错误中文且 %w 包装 → go-backend.spec.md
- ...

### 不合规项 (2)
- [FAIL] 裸 time.Sleep 等待
  → spec: go-backend.spec.md — "可中断的定时等待用 select + time.After，禁止裸 time.Sleep"
  → 文件: server/internal/service/rag.go:63
  → 修改建议: 改为 select { case <-ctx.Done(): return; case <-time.After(d): }
- ...

### 检查覆盖
- project-constraints.spec.md ✓
- go-backend.spec.md ✓
- web-frontend.spec.md (跳过: 与目标无关)
- naming-structure.spec.md ✓
```

## 约束

- 只读操作，不修改任何文件
- 每条不合规项必须引用对应的 spec 文件和具体规则
- 给出具体的修改建议，而非泛泛描述

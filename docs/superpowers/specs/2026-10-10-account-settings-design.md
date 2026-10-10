# 管理端账户设置页 设计文档

日期: 2026-10-10
状态: 已确认，待实施

## 背景与目标

当前管理端是单管理员系统（`admin_user` 表，启动种子写入），「邮箱设置」以弹窗形式挂在 Header 右上角，没有修改用户名和密码的入口。本功能新增一个「账户设置」页面，把邮箱绑定挪入页面，并支持修改用户名与密码。

价值：管理员可以自助完成全部账号资料维护，不再需要改数据库或依赖弹窗入口。

与现有架构的衔接点：

- 复用 `AuthService`（登录/bcrypt/JWT 签发）与 `GET/PUT /auth/profile`（邮箱读写，不改契约）
- 复用 admin 前端 `router/config.tsx` 纯配置路由（带 `title` 自动进侧边菜单）与 `api/http.ts` 请求封装
- 复用 `authStore`（MobX + localStorage 持久化），新增会话更新 action

## 关键决策表

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 页面定位 | 单管理员「个人账户设置」页，非多用户 CRUD | 系统仅一个种子管理员；多用户需角色/权限体系，远超需求 |
| 页面入口 | 侧边栏新增「账户设置」菜单项；Header 移除邮箱设置弹窗 | 复用 router 配置派生菜单机制；弹窗能力被页面完整替代 |
| 改用户名后的会话 | 后端重签 JWT 返回 `{token, user}`，前端无感更新 | JWT Claims 含 `username`，强制重登体验差；无状态重签成本为零 |
| 改密码后的会话 | 前端清除登录态，跳转登录页重新登录 | 标准安全实践；后端 JWT 无状态无法主动失效 |
| 旧 token 残留风险 | 接受：改密后旧 token 在有效期内仍可用 | 单管理员内部系统，引入 token 版本号/黑名单（Redis 状态）复杂度不值 |
| 接口形态 | 新增两个独立小接口，而非扩充 `/auth/profile` | 改密码需验证旧密码，语义与资料更新完全不同；profile 保持邮箱单一职责 |
| 密码规则 | 6~64 位，不做复杂度要求 | 内部单管理员系统，bcrypt 哈希已兜底；复杂度规则徒增烦扰 |

## 不做什么

- 不做多管理员、角色与权限体系
- 不做旧 token 强制失效（无 token 版本号机制，见决策表）
- 不做密码找回（邮件重置链接）
- 不做密码复杂度校验（仅长度）
- `web/blog` 前台零涉及

## 数据流 / 接口契约

三个接口均挂在现有 `admin` JWT 中间件组（`router.go`），handler 薄、逻辑全在 `AuthService`。

### 1. PUT /api/v1/auth/username（新增）

修改用户名，成功后重签 JWT。

- 请求体：`{ "username": "newname" }`
- 校验：trim 后非空、长度 1~64（与 `admin_user.username` 列 `size:64` 一致）；与当前用户名相同则直接幂等返回新 token
- 唯一性：先查 `username` 是否已被占用（`uniqueIndex`），占用返回 409
- 响应：`resp.Envelope{data: {token: string, user: AdminUser}}`（结构与 `POST /auth/login` 的 data 一致，前端复用 `LoginResp` 类型）
- 错误：400 参数不合法（`ErrInvalid`）、409 用户名已占用（`ErrConflict`）

### 2. PUT /api/v1/auth/password（新增）

修改密码，需验证旧密码。

- 请求体：`{ "old_password": "...", "new_password": "..." }`
- 校验：旧密码 bcrypt 比对；新密码 trim 后长度 6~64
- 响应：`resp.Envelope{data: null}`（200 即成功）
- 错误：400 旧密码错误 / 新密码不合法（统一 `ErrInvalid`）

> **注意**：旧密码错误**不能**用 401——`resp.FailWith` 会把 `ErrUnauthorized` 映射为 HTTP 401 + code 401，而前端 `http.ts` 响应拦截器对 `code === 401` 会全局清 token 跳登录页，用户将看不到「旧密码错误」提示且被意外登出。故统一用 400 `ErrInvalid` 承载。

### 3. GET / PUT /api/v1/auth/profile（不变）

继续负责邮箱读写，前端从 Header 弹窗挪到页面调用，契约零改动。唯一前端行为增强：保存成功后同步 `authStore.user.email`（现状弹窗保存后未同步，属顺手修复）。

## 数据模型

**无新表、无新字段。** `admin_user` 现有列（`username` uniqueIndex、`password_hash`、`email`）完全满足，AutoMigrate 无影响。

## 配置项

**无新增配置。** 不涉及 `config.yaml` / `.env.example` 三处同步。

## 前端设计（web/admin）

### 新页面 `pages/AccountSettings.tsx`

两个独立 Card 竖排（用户反馈调整：用户名与邮箱合并为一个表单）：

```
Card「用户信息」  用户名 Input + 邮箱 Input + [保存]
                  → diff 后只对变更字段发请求：
                    用户名变了 → PUT /auth/username（重签 JWT，setSession 无感换发）
                    邮箱变了   → PUT /auth/profile（updateUser 同步）
                    均未变     → 提示「资料未变更」，不发请求
Card「修改密码」  旧密码 / 新密码 / 确认新密码 + [保存]
                  → PUT /auth/password → logout → 跳登录页
```

- 「用户信息」保存成功：message 提示，Header 用户名立即刷新，**不跳转**；两个字段都变更时先改用户名后改邮箱，串行提交
- 「修改密码」前端校验：两次输入一致、新密码 6~64 位；保存成功后 message 提示 → `authStore.logout()` → `navigate('/login', { replace: true })`
- 类型：响应对齐类型放 `types/auth.ts`（改用户名响应复用 `LoginResp`；请求体为简单对象按现有惯例内联）；请求走 `api/http.ts` 的 `get/put`

### 路由与 Header 改造

- `router/config.tsx` 的 `APP_ROUTES` 追加：`{ path: '/account', title: '账户设置', icon: <UserOutlined />, element: <AccountSettings /> }`（自动进侧边菜单，`matchMenuKey` 无需改动）
- `AdminLayout.tsx`：删除邮箱弹窗全部相关代码（`mailOpen`/`email`/`saving` state、`saveEmail`、`Modal`、`MailOutlined`），保留用户名展示与退出按钮

### authStore 变更（stores/auth.ts）

新增 action `setSession(token: string, user: AdminUser)`：写内存 + localStorage（`login()` 内部改为复用它），供改用户名、改邮箱场景调用。

## 边界情况与失败路径

| 场景 | 行为 |
| --- | --- |
| 新用户名已被占用 | 后端 409，页面 message.error 展示后端文案，表单保留 |
| 新用户名与当前相同 | 幂等：MySQL `RowsAffected=0`（默认返回变更行数），service 回读确认同名后照常重签返回，前端无感 |
| 保存未变更的邮箱 | 同上，`UpdateProfile` 回读确认存在即成功（该修复同时覆盖存量弹窗路径的误报 404） |
| 旧密码错误 | 后端 400（见接口契约注意项），message.error「旧密码错误」，表单保留 |
| 两次新密码不一致 | 前端表单校验拦截，不发请求 |
| 改用户名成功但 localStorage 写入异常 | `setSession` 内 try/catch，不影响页面流程 |
| 改密码成功后用户在其他标签页仍持有旧 token | 接受残留（见决策表），旧 token 有效期内仍可操作 |
| token 过期时进入账户设置页 | 走现有 `RequireAuth` + http 封装的 401 处理，无特殊逻辑 |

## 验证方式与回滚思路

### 验证

1. 后端：`task vet && task build`；前端：`cd web/admin && npm run build`
2. dev 联调三条主流程：
   - 改用户名 → Header 立即显示新名、无需重登；用新用户名重新登录成功
   - 改密码（旧密码错误被拒）→ 正确旧密码改成功 → 跳登录页 → 旧密码失效、新密码登录成功
   - 改邮箱 → 页面回显正确，刷新后仍正确（localStorage 已同步）
3. 回归：登录/退出、侧边菜单高亮、其余页面不受影响

### 回滚

无表结构变更、无配置变更、无 MQ 消息，`git revert` 即可完整回滚。

## 备注

- 本 spec 与后续实现均不主动 commit（遵循项目约定，需用户明确授权）
- handler 带 swag 注释对齐现有格式；`server/docs/docs.go` 为生成物，实施时若项目使用 swag 再重新生成，不手改

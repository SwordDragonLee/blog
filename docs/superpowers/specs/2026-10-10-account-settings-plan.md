# 管理端账户设置页 技术方案

日期: 2026-10-10
依据: [2026-10-10-account-settings-design.md](./2026-10-10-account-settings-design.md)

## 规范依据

- `.claude/specs/coding/go-backend.spec.md` — 分层边界（handler 薄、业务在 service）、错误 `fmt.Errorf("...: %w")` 包装、导出符号 godoc 注释、ctx 贯穿
- `.claude/specs/coding/web-frontend.spec.md` — 请求必须走 `api/http.ts`、对齐类型放 `types/`、全局状态走 MobX store、页面放 `pages/`
- `.claude/specs/common/project-constraints.spec.md` — 验证闭环（`task vet` + `task build` + `npm run build`）、中文注释

## 文件变更清单

### 后端（server/，按 router → api → service 分层）

| # | 文件 | 类型 | 变更内容 |
| --- | --- | --- | --- |
| 1 | `internal/router/router.go` | 修改 | admin JWT 组注册 `admin.PUT("/auth/username", ...)`、`admin.PUT("/auth/password", ...)`；顶部注释同步两条新路由 |
| 2 | `internal/api/account_handler.go` | 新增 | `AuthHandler` 的两个薄 handler：`UpdateUsername`、`ChangePassword`（参数绑定 → 调 service → `resp` 封装，带 swag 注释对齐 `profile_handler.go` 格式） |
| 3 | `internal/service/auth.go` | 修改 | 新增 `UpdateUsername`、`ChangePassword` 业务方法（详见下文） |

repo / model / config：**零改动**（无新表、无新字段、无新配置）。

### 前端（web/admin/）

| # | 文件 | 类型 | 变更内容 |
| --- | --- | --- | --- |
| 4 | `src/pages/AccountSettings.tsx` | 新增 | 账户设置页：三个独立 Card（用户名 / 邮箱 / 修改密码），antd Form + message，风格对齐 `AskUnanswered.tsx` |
| 5 | `src/router/config.tsx` | 修改 | `APP_ROUTES` 追加 `{ path: '/account', title: '账户设置', icon: <UserOutlined />, element: <AccountSettings /> }`（带 title 自动进侧边菜单，`matchMenuKey` 前缀匹配天然支持） |
| 6 | `src/components/AdminLayout.tsx` | 修改 | 删除邮箱弹窗全部代码：`mailOpen`/`email`/`saving` state、`saveEmail`、`Modal`、`MailOutlined` 及其 import；保留用户名展示与退出按钮 |
| 7 | `src/stores/auth.ts` | 修改 | 新增 action `setSession(token, user)`（token + user 同步写内存与 localStorage）与 `updateUser(user)`（仅更新 user；`login()` 改为复用 `setSession`） |

`src/types/auth.ts`：**零改动**——改用户名响应复用 `LoginResp`；`{username}`、`{old_password, new_password}` 为简单请求体，遵循现有惯例内联（同 `login`）。

## 接口与数据契约

路由注册点均在 `router.go` 的 `admin` 组（JWT 保护），与 `/auth/profile` 同组：

```
admin.PUT("/auth/username", authHandler.UpdateUsername)   // 改用户名，重签 JWT
admin.PUT("/auth/password", authHandler.ChangePassword)   // 改密码，验旧密码
```

### service 层签名（auth.go）

```go
// UpdateUsername 修改用户名并重签 JWT；用户名占用返回 ErrConflict。
func (s *AuthService) UpdateUsername(ctx context.Context, userID uint, username string) (string, *model.AdminUser, error)

// ChangePassword 校验旧密码后更新密码哈希；旧密码错误/新密码不合法返回 ErrInvalid。
func (s *AuthService) ChangePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error
```

实现要点：

- **UpdateUsername**：`strings.TrimSpace` → 非空 + `utf8.RuneCountInString` ≤ 64（`username` 列 `size:64` 按字符计）→ `Where("username = ? AND id != ?", username, userID)` 查重（命中 → `ErrConflict`）→ `Update("username", ...)` → `s.sign(user)` 重签 → 返回 `(token, user, nil)`。查重与更新之间的竞态由 `uniqueIndex` 兜底：更新返回 duplicate 错误时包装为 `ErrConflict`
- **ChangePassword**：`First` 取用户 → `bcrypt.CompareHashAndPassword` 验旧密码（失败 → `fmt.Errorf("%w: 旧密码错误", ErrInvalid)`，**不用 `ErrUnauthorized`**——`resp.FailWith` 映射 401 会触发前端 `http.ts` 拦截器全局登出，用户看不到错误提示，见 design「接口契约注意项」）→ 新密码 trim 后 6~64 → `bcrypt.GenerateFromPassword`（cost 取默认）→ `Update("password_hash", ...)`
- 错误链全部 `fmt.Errorf("...: %w", err)`；DB 操作 `WithContext(ctx)`

### 请求 / 响应

| 接口 | 请求体 | 成功响应 data | 失败 |
| --- | --- | --- | --- |
| `PUT /auth/username` | `{"username": "..."}` | `{token, user}`（复用 login data 结构） | 400 参数不合法 / 409 用户名已占用 |
| `PUT /auth/password` | `{"old_password": "...", "new_password": "..."}` | `null` | 400 旧密码错误或新密码不合法 |

swag 注释对齐 `profile_handler.go` 现有格式；`server/docs/docs.go` 为生成物不手改，Taskfile 无 swag 任务，本次不重生成（注释先行，接口文档后续统一生成）。

## 前端方案

### AccountSettings.tsx 页面结构

```
Typography.Title level={4}  账户设置

Card「用户名」     Form: [Input(回显 authStore.username)]
                   [保存] → PUT /auth/username → setSession(token, user)
                            → message.success('用户名已更新')   // Header 立即刷新，不跳转

Card「邮箱」       Form: [Input(回显 authStore.user?.email)]
                   [保存] → PUT /auth/profile → updateUser(user)
                            → message.success('邮箱已保存')

Card「修改密码」   Form: [旧密码 Input.Password]
                         [新密码 Input.Password]  rules: required + len 6~64
                         [确认新密码]              rules: validator 与新密码一致
                   [保存] → PUT /auth/password → message.success('密码已修改，请重新登录')
                            → authStore.logout() → navigate('/login', { replace: true })
```

- 回显直接读 `authStore`（登录时已持久化 localStorage），不额外发 GET
- 三个 Card 各自独立 Form 与提交态（`saving` useState），互不影响
- 错误统一 `message.error((e as Error).message)`（http.ts 拦截器已解出后端 message）
- 图标：路由用 `UserOutlined`

### authStore 变更

```ts
/** 全量替换会话：token + user 同步写内存与 localStorage */
setSession(token: string, user: AdminUser): void
/** 仅更新用户资料（token 不变），localStorage 同步 */
updateUser(user: AdminUser): void
```

`login()` 内部改为复用 `setSession`，消除重复的 localStorage 写入逻辑。

## 复用清单

| 复用项 | 位置 | 用途 |
| --- | --- | --- |
| `get/put` 类型化请求 | `web/admin/src/api/http.ts` | JWT 自动附带、401 跳登录、错误 message 解包 |
| `authStore` | `web/admin/src/stores/auth.ts` | 会话持久化与 Header 用户名展示（扩展而非新建 store） |
| 路由配置派生菜单 | `web/admin/src/router/config.tsx` + `index.ts` | 加一条配置即得菜单项 + 高亮 |
| `AuthService.sign` / bcrypt | `server/internal/service/auth.go` | JWT 重签、密码哈希与比对 |
| 哨兵错误 + `resp.FailWith` | `service/auth.go` + `internal/resp/resp.go` | 错误 → HTTP 状态码映射 |
| `httputil.BindJSON` | `server/internal/httputil/` | 请求体绑定与 400 响应（同 profile_handler） |
| antd Card/Form/Input/Button/message | — | 页面 UI，无新依赖 |

## 配置变更

**无。** 不涉及 `config/` 结构体、`config.yaml`、`.env.example` 三处同步。

## 缓存与一致性影响

**无。** 不触碰 Redis 前台缓存、Qdrant 向量、RabbitMQ 消息；`admin_user` 仅管理端读写，前台 `/portal/*` 零涉及，无幂等诉求（单行 UPDATE 天然幂等）。

## 实施顺序与验证

1. 后端：`service/auth.go` → `api/account_handler.go` → `router/router.go` → `task vet && task build`
2. 前端：`stores/auth.ts` → `pages/AccountSettings.tsx` → `router/config.tsx` → `AdminLayout.tsx` → `cd web/admin && npm run build`
3. dev 联调手测：
   - 改用户名 → Header 立即变新名、无需重登；退出后用新用户名登录成功
   - 改密码：旧密码错 → 400 提示且**不**被登出；正确改完 → 跳登录页 → 旧密码失效
   - 改邮箱 → 回显正确，刷新页面仍正确
   - 回归：登录/退出、侧边菜单「账户设置」高亮、其余页面正常
4. 回滚：无 schema/配置变更，`git revert` 即可

实施完成后走 `/verify`；本次为跨层新增接口，按约定调用 code-reviewer 评审。

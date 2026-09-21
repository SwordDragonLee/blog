# 发布邮件通知 + 文章微信分享 设计

日期：2026-09-16。两个独立小功能，均已完成方案讨论并确认。

## 功能 1：文章发布成功后邮件通知发布者

### 需求

文章在管理端发版成功（draft → published 的真实迁移，重复发版不会触发）后，给**点击发版按钮的管理员**绑定的邮箱异步发送一封通知邮件。发信成败不影响发布结果。

### 架构

```
Publish handler（从 JWT 取 user id）
  → ArticleService.Publish(ctx, id, publisherID)   // 发布 + 失效缓存（现有逻辑不变）
  → go mailer.NotifyArticlePublished(...)           // 异步，WithoutCancel + 10s 超时
      → 查 admin_user.email → wneessen/go-mail 发 HTML 邮件 → 只记日志
```

### 组件

- **配置**（`config.go` + `config.yaml` 新增 `email` 段）：
  - `enabled`（默认 false，未启用时邮件发送为 no-op）
  - `host` / `port`（默认 465，SSL）/ `username` / `password`（SMTP 授权码，不是登录密码）
  - `from`（缺省 = username）/ `site_url`（前台博客地址，拼文章链接用）
  - 真实授权码只在本地 config.yaml 填写，**不提交**
- **模型**：`admin_user` 加 `email` 字段（size 128、默认空串），AutoMigrate 自动补列
- **`internal/mailer/`**（新包，唯一职责：发邮件）：
  - `Mailer{cfg config.Email, log *zap.Logger}`，`NotifyArticlePublished(publisherID uint, title, slug string, publishedAt time.Time)`：内部开 goroutine，查邮箱、组信、发送；disabled / 未绑邮箱 / 失败均只记日志
  - 邮件库 `github.com/wneessen/go-mail`（活跃维护、纯标准库依赖、原生支持 465 SSL，适配 QQ/163 SMTP）
  - 邮件内容：中文主题「文章已发布：《标题》」+ HTML 正文（标题、发布时间、前台链接 `site_url + /article/{slug}`、说明文字）
- **API**：
  - `GET /api/v1/auth/profile` → `{username, email}`（admin JWT）
  - `PUT /api/v1/auth/profile` → 更新邮箱（trim + 基本格式校验）
  - `POST /articles/:id/publish`：handler 用 `middleware.CtxUserID` 取当前用户传入 service
- **admin UI**：`AdminLayout` 头部用户名旁新增「邮箱设置」→ Modal（输入邮箱、保存，antd + 现有 http 封装）；`types.ts` 的 `AdminUser` 加 `email`

### 错误处理

- 未绑定邮箱：跳过发送，log.Info
- SMTP 失败/超时（10s）：log.Error，不影响发布响应
- `email.enabled=false`：no-op，log.Debug

### 验证

1. `task vet && task build`
2. 配置真实 QQ 邮箱 SMTP 授权码后发版一次，收件箱收到邮件、链接可打开
3. 未绑邮箱 / enabled=false 发版不报错、日志有跳过记录
4. profile GET/PUT：改邮箱后重新发版，邮件发到新邮箱

## 功能 2：文章分享到微信

### 需求与约束

前台读者点击分享按钮把文章分享到微信。网页直调微信分享（JS-SDK）需要认证公众号 + JS 安全域名，个人博客不可行；采用业界标准替代——**文章链接二维码**：微信扫码 → 在微信内打开 → 用微信自带菜单转发/分享朋友圈。

### 组件

- **`web/blog/components/ShareButton.tsx`**（client component，仿 LikeButton 模式）：
  - 按钮文案「分享」，点击弹出面板
  - 面板内容：`qrcode.react` 生成的二维码（纯前端 SVG，零后端改动、无网络请求）+ 文章链接 + 「复制链接」按钮 + 提示文案「微信扫一扫，在微信中打开后可转发或分享到朋友圈」
  - URL 取 `window.location.href`，自动适配任何部署域名
  - 点面板外区域关闭
- **`app/article/[slug]/page.tsx`**：文末与 `LikeButton` 同行放置 `<ShareButton />`（服务端页面引用 client 组件，模式与点赞一致）

### 验证

1. `cd web/blog && npm run build`
2. dev 起前台打开文章页 → 点分享 → 二维码与当前 URL 一致；手机微信扫码可打开文章
3. 复制链接按钮写剪贴板成功

## 备注

- 两个功能互不依赖，可并行实施；spec 与实现均不 commit（config.yaml 含真实 key，遵循项目约定）

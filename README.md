# AI 博客生成平台

根据公开 Git 仓库自动生成图文结合的技术博客：Go 后端调用大模型分析仓库 → 生成 4 篇中文文章（含 SVG 配图）→ 管理平台审核 → 发版到前台。

## 项目结构

```
blog/
├── server/    # Go 后端（Gin + GORM + zap + viper + RabbitMQ + Redis）
├── web/
│   ├── blog/  # 前台（Next.js）
│   └── admin/ # 管理平台（React + Antd + MobX）
├── deploy/    # docker-compose（MySQL + Redis + RabbitMQ）
└── docs/      # 设计文档
```

## 快速开始

### 1. 启动依赖服务

```bash
cd deploy
docker compose up -d
```

- MySQL: localhost:3309（root / <见 deploy/.env>，库 blog；3306/3307 已被本机其他服务占用）
- Redis: localhost:6380（本机 6379 已被占用）
- RabbitMQ: localhost:5672（管理界面 http://localhost:15672，blog / <见 deploy/.env>）

### 2. 配置并启动后端

编辑 `server/config.yaml`，填写 `llm.api_key`，或启动前设置环境变量 `LLM_API_KEY`。

```bash
cd server
go run ./cmd/server
```

默认管理员：admin / admin123（见 config.yaml auth 段）。

### 3. 启动管理平台

```bash
cd web/admin
npm install
npm run dev   # http://localhost:5173
```

### 4. 启动前台

```bash
cd web/blog
npm install
npm run dev   # http://localhost:3000
```

## 使用流程

1. 管理平台 → 新建任务 → 粘贴公开 Git 仓库地址（如 https://github.com/user/repo）
2. 实时查看生成进度（克隆 → 分析 → 写作 → 配图）
3. 文章列表审核，可编辑 Markdown、重新生成配图
4. 点击「发版」→ 前台立即可见
 

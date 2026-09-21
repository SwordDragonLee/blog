# 生产部署指南：国内 ECS + 域名 + HTTPS

> 目标：把本地跑通的三端（Go server / admin 管理端 / Next.js 前台）+ 四个中间件
> 部署到一台国内云服务器（2C4G），用 Docker Compose 编排，Nginx 统一入口 + HTTPS。
> 文内 `<服务器IP>`、`<域名>` 为占位符，部署时替换。
> 关联文档：《ai-article-generation.md》《ai-rag-indexing.md》《ai-rag-ask.md》（部署排障会引用）。

## 一、目标架构

```
访客 ── https://<域名>（443）
         │
    nginx 容器（统一入口：HTTPS 终结 / 静态托管 / 反向代理）
         ├─ server_name <域名>            → blog 容器 :3000（Next.js SSR/ISR）
         ├─ server_name admin.<域名>      → admin 容器 :80（自带 nginx 托管静态 + /api 反代）
         │     └─ /api/*                  → server 容器 :8080
         └─ HTTP 80 强制跳转 443
         │
Docker 内网（compose 网络，端口一律不映射到公网）：
  server:8080 │ mysql:3306 │ redis:6379 │ rabbitmq:5672 │ qdrant:6333
```

要点：

- **八个容器一个 `compose.prod.yml`**（三端 + nginx 入口 + 四中间件），`restart: unless-stopped`，数据全落 named volume
- **admin 部署为 `admin.<域名>` 子域**：admin 前端用的是 `BrowserRouter`（无 basename）+ 相对路径 axios（`/api/v1`），子域方案**代码零改动**；备选方案是给 `BrowserRouter` 加 `basename="/admin"` 挂在主域路径下
- blog 是 SSR，不能静态托管，必须常驻进程；它在容器网络内用 `API_BASE=http://server:8080` 直连后端取数
- 前台 RAG 问答是 SSE（`POST /api/v1/portal/ask`），nginx 必须关缓冲（见 §6 nginx 配置）

## 二、阶段 0：云资源（备案是长周期项，先启动）

| # | 事项 | 选型/操作 | 周期 |
|---|---|---|---|
| 1 | 购买 ECS | 2核4G（常驻内存约 1.5G：MySQL ~400M、RabbitMQ ~200M、Qdrant ~300M、Next ~300M、其余 ~150M），Ubuntu 22.04/24.04，带宽 3~5M 固定或按量 | 即时 |
| 2 | 系统盘外加 2G swap | 服务器初始化时创建（见 §5 步骤 2） | — |
| 3 | 购买域名 + 实名认证 | 与 ECS 同厂商（备案方便），`.com`/`.cn` 均可 | 实名数小时~1 天 |
| 4 | **提交 ICP 备案** | 控制台备案系统，用 ECS 申请「备案服务码」；1~3 周，**第一天就提** | 1~3 周 |
| 5 | 安全组/防火墙 | 只放行 22 / 80 / 443 | 即时 |

内存红线：**2C4G 禁止在服务器上构建镜像**（Next.js 构建峰值 >2G 会 OOM）——镜像一律本机构建、传输上云。

## 三、阶段 1：产物化（文件清单）

以下文件为**待创建项**，由实现阶段产出：

| 文件 | 作用 |
|---|---|
| `docker/Dockerfile.server` | 多阶段：`golang:1.25-alpine` 构建（`CGO_ENABLED=0`）→ `alpine` 运行层（tzdata + ca-certificates）；**config.yaml 不打镜像**（`server/.dockerignore` 排除），运行时挂载 |
| `docker/Dockerfile.blog` | Next.js standalone 产物镜像（`next.config.ts` 已加 `output: 'standalone'`）；启动 `PORT=3000 HOSTNAME=0.0.0.0 node server.js`。构建上下文仍是 `./web/blog` |
| `docker/Dockerfile.admin` | 多阶段：`node:22-alpine` `npm ci && npm run build` → `nginx:1.27-alpine` 托管 `dist/`（SPA 回退 + `/api` 反代到 `server:8080`，conf 内嵌在 Dockerfile） |
| `docker/compose.prod.yml` | 八服务编排（与本地 `deploy/docker-compose.yml` 的差异见 §4 对照表） |
| `docker/nginx/blog.conf` | `<域名>` server 块：反代 `blog:3000`；`/api` 反代 `server:8080`（SSE location 关缓冲）；443 段注释预留 |
| `docker/nginx/admin.conf` | `admin.<域名>` server 块：整站转给 `admin:80`；443 段注释预留 |
| `docker/nginx/default.conf` | 兜底块：IP 直连返回 444（覆盖镜像自带欢迎页） |
| `docker/env.production.example` | `.env` 模板：中间件密码 + server 密钥 + 主机相关项，复制为 `.env` 后 `chmod 600` |

> **配置只有一份**：`server/config.yaml`（不含任何密钥，可入库）。生产与本地的差异不用第二份 config 表达——
> 拓扑固定项（服务名/端口/release 模式）由 `compose.prod.yml` 的 `environment` 注入，
> 密钥与主机相关项由 `.env` 覆盖（viper 环境变量优先级最高，键名 = 配置项点号换下划线大写）。

配套的三个 `.dockerignore`（`server/`、`web/blog/`、`web/admin/`）已就位：上下文瘦身，且保证 `config.yaml` 永不进入镜像构建上下文。

### 生产环境相对本地的差异项（全部经 compose environment 或 .env 表达，无第二份 config）

| 配置项 | 本地值 | 生产值 |
|---|---|---|
| `server.mode` | debug | **release** |
| `mysql.host`/`port` | localhost / 3309 | mysql / 3306（compose 网络） |
| `redis.addr` | localhost:6380 | redis:6379，**设密码** |
| `rabbitmq.url` | amqp://guest@localhost:5672 | amqp://blog:`<强密码>`@rabbitmq:5672（compose 里 `RABBITMQ_DEFAULT_USER/PASS` 与之配套，**禁用默认 guest 跨容器访问**） |
| `qdrant.base_url` | http://localhost:6333 | http://qdrant:6333 |
| `task.proxy` | http://127.0.0.1:7890 | **留空**（ECS 直连 github 时好时坏，见 §8 排障） |
| `auth.admin_password` | admin123 | **强密码**（首启种子管理员用它） |
| `email.site_url` | http://localhost:3000 | https://<域名>（通知邮件里的文章链接） |
| `rag.embedding_*` / key | 本地同款 | 同款可复用（SiliconFlow bge-m3 免费档 + 智谱 glm） |

### 安全红线（不可违反）

1. **私密数据只存 `.env`**：密钥/密码不写进任何 config.yaml、不提交 git、不进镜像、不进聊天窗口/截图。本地 `server/.env`（config.Load 自动加载）、部署时 `/opt/blog/.env`（compose `env_file` 注入 server 容器，同时供中间件变量替换），均已 gitignore；模板 `server/.env.example` 与 `docker/env.production.example`
2. `server/config.yaml` 与 `config.production.example.yaml` 均已去密文化、可入库；但 **git 历史里的旧版 config.yaml 仍含真实 key**——介意可做历史清理（BFG/filter-repo）并到服务商作废旧 key 重新签发
3. MySQL / Redis / RabbitMQ / Qdrant 端口**一律不写 `ports:` 映射**，只被同网络容器访问
4. RabbitMQ 管理界面（15672）不在公网暴露；需要看时走 SSH 隧道：`ssh -L 15672:localhost:15672 root@<IP>`（compose 里给 rabbitmq 加 `ports: "127.0.0.1:15672:15672"` 绑回环）

## 四、`compose.prod.yml` 与本地 compose 的差异对照

| 差异点 | 本地 deploy/docker-compose.yml | 生产 compose.prod.yml |
|---|---|---|
| 端口映射 | MySQL 3309 / Redis 6380 映射宿主机（开发方便） | **全部不映射**（仅 nginx 的 80/443 映射） |
| 服务数量 | 4 个中间件 | 8 个：四中间件 + server + blog + admin + nginx |
| 数据持久化 | 匿名卷/无 | named volumes：mysql-data、redis-data、rabbitmq-data、qdrant-data（**向量必须持久化**，否则重启后 RAG 索引全丢） |
| 健康检查 | 无 | mysql/redis/qdrant 加 healthcheck，server 依赖 `condition: service_healthy` |
| 重启策略 | 无 | `restart: unless-stopped` |
| 日志 | — | json-file 限额（max-size 10m × 3 文件），防日志吃满磁盘 |

## 五、阶段 2/3：本机演练 + 服务器上线

### 阶段 2：本机 Docker 演练（上云前必须全绿）

1. 本机构建三端镜像（命令见下）
2. 准备演练配置：`cp docker/env.production.example docker/.env` 改密码，并在此 `.env` 里加一行 `SERVER_CONFIG_PATH=../server/config.yaml`（直接挂仓库配置，不复制文件）
3. **先停掉本地 dev 栈**：`cd deploy && docker compose down`（数据卷保留，回来时 `up -d` 即可）——两套栈的容器名都是 `blog-*`、nginx 都要占 80 端口，同时跑必冲突
4. 用生产 compose 启动（演练差异：nginx 80 映射本机；用 `http://blog.localhost` / `http://admin.localhost` 访问——现代浏览器自动把 `*.localhost` 解析到 127.0.0.1，无需改 hosts；先把 nginx conf 里的 `<域名>` 临时替换成 `blog.localhost` / `admin.localhost`）
5. 演练完切回 dev 栈：`cd docker && docker compose -f compose.prod.yml down`，再 `cd ../deploy && docker compose up -d`
4. 跑 §7 冒烟清单（把域名换回 localhost）

```bash
# 本机构建（仓库根目录；三个 Dockerfile 都在 docker/，构建上下文分别是 server/、web/blog/、web/admin/）
docker build -f docker/Dockerfile.server -t blog-server:latest ./server
docker build -f docker/Dockerfile.blog    -t blog-blog:latest   ./web/blog
docker build -f docker/Dockerfile.admin   -t blog-admin:latest  ./web/admin

# 导出 / 传输 / 导入（2C4G 不在服务器构建）
docker save blog-server:latest blog-admin:latest blog-blog:latest | gzip > blog-images.tar.gz
scp blog-images.tar.gz root@<服务器IP>:/opt/blog/

# 服务器
cd /opt/blog && docker load < blog-images.tar.gz
```

### 阶段 3：服务器初始化与上线

```bash
ssh root@<服务器IP>

# 1) 装 Docker（国内源）
curl -fsSL https://get.docker.com | bash -s docker --mirror Aliyun
systemctl enable --now docker

# 2) 2G swap（防内存尖峰）
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

# 3) 目录与文件
mkdir -p /opt/blog/nginx/certs
#  本机分别 scp：compose.prod.yml、.env（真实密钥）、nginx/*.conf、config.yaml（非私密配置）
scp docker/compose.prod.yml        root@<IP>:/opt/blog/
scp docker/env.production.example  root@<IP>:/opt/blog/   # 服务器上复制为 .env，填齐密钥/强密码后 chmod 600
scp docker/nginx/*.conf            root@<IP>:/opt/blog/nginx/
scp server/config.yaml             root@<IP>:/opt/blog/config.yaml   # 仓库原样文件（无密钥，服务地址由 compose environment 注入）

# 4) 启动
cd /opt/blog && docker compose -f compose.prod.yml up -d
docker compose -f compose.prod.yml ps        # 八个容器应全部 Up/healthy
docker logs blog-server --tail 50            # 看启动日志：AutoMigrate + 种子管理员 + HTTP 已启动
```

说明：MySQL 首次启动由 server 的 GORM AutoMigrate 自动建表并种子管理员（密码 = 生产配置的 `auth.admin_password`），无需手工导入库。

## 六、阶段 4：域名解析与 HTTPS（备案通过后）

1. **DNS**：控制台加两条 A 记录 → `@` 与 `admin` 都指向 `<服务器IP>`（备案只针对主域名，子域免备案）
2. **证书**：阿里云/腾讯云控制台申请**免费 DV 证书**（同时填 `域名` 与 `admin.域名`，或申请一张通配符 `*.域名`），签发后下载 **nginx 版**（`xxx.pem` + `xxx.key`）
3. **上线证书**：

```bash
scp xxx.pem xxx.key root@<IP>:/opt/blog/nginx/certs/
# nginx conf 里放开 443 server 块（阶段 1 预留的注释段），挂载 certs/ 只读
ssh root@<IP> 'cd /opt/blog && docker compose -f compose.prod.yml restart nginx'
```

4. 80 端口 server 块保留 `return 301 https://$host$request_uri;` 强跳
5. 免费证书 3 个月有效期：到期前在控制台重新签发替换 `certs/` 并 restart nginx；嫌烦可换 certbot standalone + cron（续期前后 `docker compose stop/start nginx`，停机数秒）

## 七、冒烟验收清单（演练与正式各跑一遍）

| # | 检查项 | 期望 |
|---|---|---|
| 1 | `https://<域名>` | 前台文章列表渲染（SSR），封面 SVG 正常 |
| 2 | 打开一篇文章 | Markdown/代码高亮/插图（`/portal/figures/*.svg`）正常 |
| 3 | `https://admin.<域名>` | 登录页；用生产密码登录 |
| 4 | admin 建任务 | 克隆→分析→写作→完成，实时进度推进（列表页进度不再是只有 1%/100%） |
| 5 | **克隆环节** | github 直连成功；失败 → 见 §8 排障第 1 条 |
| 6 | 审核发版一篇 | 前台出现该文章；RAG 索引日志出现「文章已向量化入库」 |
| 7 | 前台聊天气泡提问 | SSE **逐字**输出 + 引用卡片可点（一次性出全文 = 反代缓冲，见 §8 第 3 条） |
| 8 | 通知邮件 | 收件人收到的文章链接是 `https://<域名>/article/...` |
| 9 | 安全 | `nmap -p 3306,6379,5672,6333,8080 <IP>` 全部 closed/filtered；RabbitMQ 15672 公网不可达 |
| 10 | 持久化 | `docker compose restart` 后文章/向量/任务记录均在 |

## 八、排障速查

| 症状 | 原因与处理 |
|---|---|
| 任务失败：克隆 github 超时 | 国内 ECS 对 github 时好时坏。先重试；仍不通：给服务器配代理（`task.proxy` 填服务器上的代理地址），或增大 `task.clone_timeout_seconds`（如 900），或改用 gitee 镜像仓库地址建任务 |
| 前台 502 / 打不开 | `docker logs blog-blog`；多为 blog 容器内 `API_BASE` 解析不到 `server` 服务名——确认两容器同 compose 网络 |
| 问答一次性出全文（无逐字流式） | nginx 缓冲了 SSE：对应 location 加 `proxy_buffering off;`（代码里已带 `X-Accel-Buffering: no` 响应头，仍需 nginx 配合） |
| 问答返回 503 | 生产 config.yaml 的 `rag.enabled=false` 或 embedding/qdrant/chat 配置缺失；按《ai-rag-ask.md》依赖清单核对 |
| 服务器卡死/进程被杀 | 内存耗尽：`free -h` 看 swap；确认没有在服务器上做任何 build |
| 邮件没发出 | `docker logs blog-server` 搜 SMTP；`email.*` 配置与授权码核对（失败只记日志不阻断发布，属设计行为） |
| 磁盘涨满 | `docker system df`；compose 已限容器日志，通常是大镜像堆积：`docker image prune -f` |

## 九、上线后运维

- **备份（crontab，每日 3 点）**：

```bash
0 3 * * * docker exec blog-mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" blog' | gzip > /opt/blog/backups/blog-$(date +\%F).sql.gz && find /opt/blog/backups -mtime +7 -delete
```

  Qdrant 向量可随时由发布动作重建（发布→索引幂等），不强制备份；要备份则打包 `qdrant-data` 卷。
- **更新发版**：日常走第十节 CI/CD（git push 即发版）；手工备用路径：本机重新 build → save/scp/load → `docker compose -f compose.prod.yml up -d`（只重建变更的服务）
- **监控（可选加分项）**：server 已暴露 `GET /metrics`（Prometheus 格式，含 MQ 指标）——接一套 Prometheus + Grafana 即可看到任务吞吐与 HTTP 状态；公网暴露需加白名单或反代鉴权
- **回滚**：保留上一版镜像 tag，出问题改 compose 里 tag 后 `up -d` 即回滚

## 十、CI/CD 发版（GitHub Actions + 腾讯云 TCR 个人版）

第五节的 save/scp/load 适合首次部署；之后的日常发版走 CI/CD：`git push main` 即自动构建受影响的镜像 → 推腾讯云 TCR 个人版（免费，大陆服务器拉取快且稳）→ SSH 远程执行 `pull + up -d`。服务器只做拉取，2C4G 不参与构建；首次部署仍按第五节手工初始化。

工作流见 `.github/workflows/deploy.yml`：按改动路径过滤（改 server/ 只建 server 镜像，全程约 5~10 分钟）；只改 nginx conf 或 compose 时不建镜像，仅远程重启生效；构建失败不会发版。

### 一次性配置（先于首次 CI 发版）

1. **开通 TCR 个人版**（免费）：控制台「容器镜像服务 → 个人版」→ 设置登录密码 → 建命名空间 → 在命名空间下新建 3 个**私有**仓库：`blog-server`、`blog-blog`、`blog-admin`。记下 registry 地址（形如 `registry.cn-guangzhou.tencentcloudcr.com`，选与 ECS 同地域）。
2. **部署专用 SSH 密钥**（本机生成，不复用日常私钥）：

```bash
ssh-keygen -t ed25519 -f blog_deploy_key -N "" -C "blog-deploy"
# 公钥追加到服务器 /root/.ssh/authorized_keys；私钥全文填 GitHub Secret 的 SSH_KEY
```

3. **GitHub 仓库** Settings → Secrets and variables → Actions：
   - Variables（非私密）：`REGISTRY=<registry 地址>`、`NAMESPACE=<命名空间>`
   - Secrets：`TCR_USERNAME=<腾讯云账号 ID>`、`TCR_PASSWORD=<开通 TCR 时设置的密码>`、`SSH_HOST=<服务器 IP>`、`SSH_USER=root`、`SSH_KEY=<私钥全文>`
4. **服务器**（一次性）：

```bash
# 腾讯内网镜像加速：拉 mysql/redis 等官方镜像免出国（写入后重启 docker 生效）
cat > /etc/docker/daemon.json <<'EOF'
{ "registry-mirrors": ["https://mirror.ccs.tencentyun.com"] }
EOF
systemctl restart docker

docker login <REGISTRY>   # 用户名 = 腾讯云账号 ID，密码 = 开通 TCR 时设置的密码

# /opt/blog/.env 追加三行，切换到 TCR 拉取（说明见 env.production.example 末尾）：
#   SERVER_IMAGE=<REGISTRY>/<命名空间>/blog-server:latest
#   BLOG_IMAGE=<REGISTRY>/<命名空间>/blog-blog:latest
#   ADMIN_IMAGE=<REGISTRY>/<命名空间>/blog-admin:latest
docker compose -f compose.prod.yml pull server blog admin   # 验证三个镜像能拉取
```

5. 首次部署：GitHub 仓库 Actions 页 → deploy → Run workflow → **勾选 force_all**（跳过路径过滤，三个镜像全部构建推送）→ 确认构建/部署两个 job 全绿。之后的日常发版不需要勾。

### 日常发版与回滚

- **发版**：`git push main`。构建在 GitHub 的机器上做，不占本地也不占服务器。
- **回滚**：每个镜像都额外打了 git 短 SHA tag；出问题把 .env 里对应行的 `:latest` 换成旧 SHA 再 `up -d`，修复后改回 `:latest` 追平。
- **nginx conf 变更**：deploy 步骤每次都会 `restart nginx`，conf 提交推送即生效，无需登服务器。

## 十一、节奏与分工

| 谁 | 事项 |
|---|---|
| 用户 | 阶段 0 全部（买 ECS/域名、提交备案）、scp 传文件与 config.yaml、在服务器粘贴命令 |
| AI 助手 | 阶段 1 全部文件的编写与验证、阶段 2 排障、阶段 3-4 逐条命令与排障 |

> 文档对应 2026-09-20 确认的部署方案（国内 ECS 2C4G + 域名备案 + HTTPS）。
> 阶段 1 的「待创建」文件落地后，本文件即为唯一部署操作手册。

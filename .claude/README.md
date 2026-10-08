# .claude/ — SDD 规范驱动开发目录

所有 AI 辅助开发行为以本目录下的规范为准。AI 助手（Claude Code / WorkBuddy 等）在本仓库工作时必须遵守以下约定。

## 目录与权威

```
.claude/
├── README.md                  # 本文件：目录规范 + SDD 工作流约定（提交到 git）
├── settings.json              # 团队共享配置：权限、hooks、env（提交到 git）
├── settings.local.json        # 个人配置，优先级更高（已 gitignore，禁止提交）
├── rules/                     # 行为准则、冲突优先级（总纲）
│   └── global-behavior.rule.md
├── specs/                     # 必须/禁止（最高硬约束）
│   ├── common/                #   通用：docs / git / 命名结构 / 项目红线
│   └── coding/                #   专项：go-backend / web-frontend
├── commands/                  # SDD 命令（工作流入口）
│   ├── coding/                #   主流程五件套：/spec /plan /tasks /code /verify
│   ├── superpowers/           #   可选增强：/brainstorm /sdd-review /sdd-verify /sdd-debug
│   ├── check.md  commit.md    #   项目工具命令
│   └── README.md              #   命令总表
├── agents/                    # 自定义子代理（code-reviewer）
└── memory/                    # 未升格笔记（可暂不存在；最低权威，服从 skill/spec）
```

## 规范优先级（高 → 低）

1. `specs/common/project-constraints.spec.md`（绝对禁止项）
2. `specs/coding/`、`specs/common/` 专项/通用 spec
3. `.claude/skills/`、`.claude/memory/`（流程惯例与未升格笔记）
4. 本 README 的 SDD 工作流约定 + 根目录 `CLAUDE.md` 的项目规则

同层冲突更具体者优先；**specs 与源码事实冲突时以源码为准，并修正 spec**，禁止默默跟错文档。

配套文件（在仓库根目录与 `docs/`，属事实文档而非规范，与源码冲突时以源码为准）：

| 文件                        | 作用                                       | 是否入 git        |
| --------------------------- | ------------------------------------------ | ----------------- |
| `CLAUDE.md`                 | 项目地图与规则，AI 进场必读                | ✅                |
| `CLAUDE.local.md`           | 个人补充说明                               | ❌（已 gitignore） |
| `docs/architecture.md`      | 架构与实现细节全貌（按需查阅）             | ✅                |
| `docs/third-party-apis.md`  | 第三方端点拼接规则与厂商坑                 | ✅                |

## SDD 流程

```
/brainstorm（可选）→ /spec → /plan → /tasks → /code → /verify（→ /sdd-review）
```

命令总表见 `commands/README.md`。`/verify` 与 `/sdd-review` 前必须先读 `.claude/specs/` 下全部 `*.spec.md` 再逐条对照——新增 spec 自动纳入检查，无需改命令。

## SDD 工作流约定

1. **进场先读**：AI 开始工作前必须读根目录 `CLAUDE.md`；涉及架构细节再按需读 `docs/architecture.md`，第三方端点问题读 `docs/third-party-apis.md`。
2. **需求先行**：非平凡改动（跨层、新增接口/表、状态机变更）先走 `/spec` → `/plan` 产出方案，经确认后实施；规范文档落在 `.claude/specs/`（编码约束）或 `docs/`（设计文档）。
3. **小步提交**：一次提交只做一件事；提交信息、代码注释、UI 文案用中文，标识符用英文。
4. **验证闭环**：改动后端必须过 `task vet` + `task build`；改动前端必须过对应目录的 `npm run build`（tsc 校验）。测试以 `cd server && go test ./...`（service 层为主）为准。
5. **私密数据红线**：密钥/密码只进 `.env`（已被 gitignore），`config.yaml` 与任何提交内容不得出现明文私密项。环境变量覆盖规则见 `server/config/config.go`。
6. **分层不越界**：后端严格 `router → api（薄）→ service → repo/mq/task`，api 层不写业务逻辑；前台只读接口走 `/portal/*`，写操作只有 `ask`。完整条目见 `specs/coding/go-backend.spec.md`。
7. **评审**：较大改动完成后用 `/sdd-review` 或 code-reviewer 子代理按 specs 检查。

## 维护规则

- 规范变更只改本目录 + 根目录 `CLAUDE.md`，不散落在对话里。
- 新增编码约束 → `specs/coding/` 加 spec 文件（命令的检查清单会自动覆盖，不用改命令）。
- `settings.local.json` 里的权限属个人偏好，禁止拷贝进 `settings.json`。

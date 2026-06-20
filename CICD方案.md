# 本地 CI/CD 流水线完整方案

> 最后更新：2026-06-20（安全加固完成：服务器密码 AES-GCM 加密存储+自动迁移、SSH known_hosts TOFU 校验、CSRF 防护、下载 token 机制、并发锁、原子写入、路径穿越校验、SSH 连接回收；前端拆分为 index.html/app.css/app.js；按钮布局分组重构；补全 download-token 等 31 个 API 路由文档）  
> 设计目标：纯自建、零外部依赖、可视化操作、通用可扩展、AI Agent 友好

---

## 一、总体架构

```
┌──────────────────────────────────────────────────────────────┐
│                      Windows / Linux / macOS                   │
│                                                              │
│  用户访问方式:                                                  │
│    ├─ 浏览器打开 http://localhost:8080  （Web UI）               │
│    ├─ 终端执行 ci <command>            （CLI）                   │
│    └─ MCP 协议被 AtomCode 自动调用      （AI Agent）              │
│                                                              │
│  ┌─ ci.exe（Go 单文件二进制）────────────────────────────────┐  │
│  │                                                          │  │
│  │  ┌─ cmd/ ─────────────────────────────────────────────┐  │  │
│  │  │  main.go        ← Cobra 根命令，注册所有子命令        │  │  │
│  │  │  commands.go    ← CLI 子命令定义（含远程管理/日志/服务器命令）│  │  │
│  │  │  remote_commands.go ← 远程管理/日志/服务器 CLI 命令实现    │  │  │
│  │  │  serve.go       ← ci serve（内嵌 Web 服务器）        │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  │                                                          │  │
│  │  ┌─ internal/ ─────────────────────────────────────────┐  │  │
│  │  │  runner/       ← 调用 PowerShell 后端引擎             │  │  │
│  │  │  config/       ← 读取 projects.json + auth.json      │  │  │
│  │  │  output/       ← 统一输出格式（文本/JSON）             │  │  │
│  │  │  serve/        ← Web 服务器 + 13 个 API 路由          │  │  │
│  │  │    ├─ handler.go    ← HTTP handler + 中间件           │  │  │
│  │  │    └─ web/index.html ← 单页 Web UI 控制台             │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  │                                                          │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │  PowerShell 后端引擎（ci-runner / cd-deploy）        │  │  │
│  │  │  ci.exe 通过 os/exec 调用，复用已有逻辑               │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌─ 数据存储 ────────────────────────────────────────────┐   │
│  │  projects.json    ← 项目配置（Web UI 管理）              │   │
│  │  auth.json        ← 认证信息（SHA256 + 随机盐）          │   │
│  │  reports/{proj}/  ← 测试报告（最近 20 条自动留存）        │   │
│  └──────────────────────────────────────────────────────────┘  │
│                                                              │
│      ┌── SSH/SFTP ───────────────────────────────┐           │
│      │  每项目独立服务器/路径                        │           │
│      └─────────────────────────────────────────────┘           │
│                        │                                      │
└────────────────────────┼──────────────────────────────────────┘
                         │ SSH (22)
                         ▼
           ┌─────────────────────┐
           │  部署目标 A          │
           │  (服务器 / VM)       │
           └─────────────────────┘
```

### 核心原则

| 原则 | 说明 |
|------|------|
| **零外部依赖** | 仅用 Go 标准库 + Windows 自带 PowerShell，不装任何第三方 CI 工具 |
| **单二进制分发** | `ci.exe` 一个文件复制即用，无需安装运行环境 |
| **全可视化** | 用户不碰任何配置文件，增删项目、改密码、看报告都在浏览器操作 |
| **AI Agent 友好** | 内置 MCP 服务器 + OpenAI Function Calling Schema，大模型可直接调用 |
| **全链路自动化** | 支持自动流水线（通过后自动执行下一步，失败自动终止） |

---

## 二、目录结构

```
D:\Idea\project\ci-cd\
├── ci.exe                        ← Go 编译产物（单文件二进制）
├── cmd\ci\main.go                ← Cobra 入口
├── go.mod / go.sum               ← Go 模块定义
│
├── internal\                     ← Go 后端逻辑
│   ├── cmd\
│   │   ├── commands.go           ← 15 个 CLI 子命令定义
│   │   └── serve.go              ← ci serve 命令（启动 Web 服务器）
│   ├── config\
│   │   ├── config.go             ← projects.json 读写
│   │   └── auth.go               ← auth.json 读写 + SHA256 加密
│   ├── output\
│   │   └── formatter.go          ← 输出格式化（文本/JSON）
│   ├── runner\
│   │   └── runner.go             ← 调用 PowerShell 引擎 + 报告持久化
│   └── serve\
│       ├── handler.go            ← HTTP handler + 中间件 + 路由注册
│       ├── remote.go             ← SSH/SFTP 远程管理 handler（终端+文件）
│       ├── log.go                ← 审计日志持久化 + 统一报告 API
│       └── web\index.html        ← Web UI 前端（单页 HTML）
│
├── ci-runner.ps1                 ← CI 执行引擎（check/build/test）
├── cd-deploy.ps1                 ← SSH/SFTP 部署引擎
├── ci-push.ps1                   ← 多远程仓库推送
├── ci-mcp-server.ps1             ← MCP 协议服务器（AI Agent 集成）
├── build-and-run-web.bat         ← 一键编译 + 启动脚本
│
├── projects.json                 ← 项目配置（Web UI 自动管理）
├── auth.json                     ← 认证信息（自动生成）
├── servers.json                  ← 独立服务器配置（通过 Web UI / CLI 管理）
├── reports\                      ← 测试报告存储目录
│   └── {project}\
│       └── test-{timestamp}.json
├── logs\                         ← 审计日志（每日自动轮转）
│   └── audit-YYYY-MM-DD.jsonl    ← 所有操作日志，每条一行 JSON
│
├── rules\                        ← 代码检查规则
│   ├── eslint-vue.mjs            ← Vue 项目 ESLint 规则
│   └── checkstyle.xml            ← Maven 项目 Checkstyle 规则
│
└── hooks\                        ← Git hooks 模板
    ├── pre-commit
    ├── pre-push
    └── install-hooks.bat
```

---

## 三、CLI 工具设计（Go + Cobra）

### 3.1 技术选型

| 方案 | 说明 |
|------|------|
| **语言** | Go 1.22+ |
| **CLI 框架** | Cobra（Kubernetes、Hugo、Docker 等使用） |
| **产物** | 单文件 `ci.exe`（Windows） |
| **后端引擎** | PowerShell 脚本（Go 通过 `os/exec` 调用） |

### 3.2 完整命令列表（27 个）

| 命令 | 功能 | 参数（可选 vs 必填） |
|------|------|-------------------|
| `check [project]` | 代码检查（tsc/eslint/checkstyle） | 可选：不传则检查全部 |
| `test [project]` | 单元测试（Jest/Vitest/Maven），返回结构化报告 | 可选：不传则全部 |
| `build [project]` | 完整构建（npm run build / mvn package） | 可选 |
| `push [project]` | 推送到所有 Git 远程仓库 | 可选 |
| `deploy [project]` | SSH/SFTP 部署到远程服务器 | 可选 |
| `hooks [project]` | 安装 Git hooks | 可选 |
| `list` | 列出所有项目及状态 | 无 |
| `status [project]` | 查看项目构建产物和 Git 状态 | 可选 |
| `describe` | 输出工具 Schema（LLM/AI Agent 发现用） | `--format openai/mcp/text` |
| `passwd [user] [pass]` | 修改或重置 Web UI 登录密码 | 可选：不传则重置为默认 |
| `report <project>` | 查看/删除测试报告 | `--json`, `--list`, `--delete <id>` |
| `report all` | 列出所有项目的测试报告 | `--keyword`, `--json` |
| `serve` | 启动 Web UI 服务器 | `--port 8080`, `--no-open` |
| `doctor` | 诊断 CI/CD 环境状态 | `--json` |
| `project list` | 列出所有项目的详细信息 | `--json` |
| `local ls` | 列出本地目录（盘符/导航），用于选择项目路径 | `--path`（默认列盘符） |
| `rules list` | 列出可用代码检查规则文件 | 无 |
| `rules view <file>` | 查看规则文件内容 | `<file>`（必填） |
| `remote ls <server>` | 列出远程服务器目录 | `--path`, `--source` |
| `remote download <server>` | 从远程服务器下载文件 | `--path`（必填）, `--source` |
| `remote upload <server>` | 上传文件到远程服务器 | `--file`（必填）, `--path`（必填）, `--source` |
| `remote delete <server>` | 删除远程服务器上的文件或目录 | `--path`（必填）, `--source` |
| `remote mkdir <server>` | 在远程服务器上创建目录 | `--path`（必填）, `--source` |
| `server list` | 列出所有独立服务器 | 无 |
| `server add` | 添加独立服务器 | `--name`, `--host`, `--user`（必填）, `--port`, `--auth-type`, `--key-path`, `--password` |
| `server delete <name>` | 删除独立服务器 | 无 |
| `log query` | 查询审计日志 | `--date`, `--level`, `--keyword`, `--limit`, `--json` |
| `log dates` | 列出有审计日志的日期 | 无 |
| `log delete` | 删除指定日期的审计日志 | `--date`（必填） |

> 注：`remote term`（交互式 SSH 终端）仅 Web UI 提供，基于 WebSocket，不适合命令行模式，故未纳入 CLI。

### 3.3 执行结果格式

#### 人类可读模式

```bash
$ ci check pair-front
[pair-front] ✅ 通过 (3.2s)
```

#### JSON 模式

```bash
$ ci check pair-front --json
```

```json
[
  {
    "project": "pair-front",
    "action": "check",
    "status": "pass",
    "duration": "3.4s"
  }
]
```

### 3.4 shell 自动补全

```bash
ci completion powershell > $PROFILE
ci completion bash > /etc/bash_completion.d/ci
```

---

## 四、Web UI 控制台

### 4.1 技术方案

| 组件 | 技术 |
|------|------|
| 后端 | Go `net/http` + 内嵌静态文件 |
| 前端 | 单页 HTML + CSS + JavaScript（无框架，零依赖） |
| 认证 | HTTP Basic Auth + SHA256 密码加密 |
| 端口 | 默认 8080，通过 `--port` 修改 |

### 4.2 启动方式

```bash
ci.exe serve
# 浏览器自动打开 http://localhost:8080
# 默认用户名: admin  密码: 123456
```

### 4.3 Web UI 功能矩阵

| 功能 | 位置 | 说明 |
|------|------|------|
| 项目列表 | 主表格 | 显示名称、类型、版本、Git 分支、状态、进度 |
| **检查** | 工具栏 + 项目行 | 单项目或批量 |
| **构建** | 工具栏 + 项目行 | 单项目或批量 |
| **测试** | 工具栏 + 项目行 | 单项目或批量，测试后自动弹出报告 |
| **推送** | 工具栏 + 项目行 | 单项目或批量 |
| **部署** | 工具栏 + 项目行 | 单项目或批量 |
| **▶ 流水线** | 项目行 | 对单项目执行 check→build→test→push→deploy 全链路 |
| **⏯ 运行流水线** | 工具栏 | 对所有项目执行全链路 |
| **🌐 自动流水线** | 工具栏开关 | ON 时单步通过后自动继续下一步 |
| **📊 报告** | 项目行 | 查看最新测试报告弹窗 |
| **➕ 添加项目** | 工具栏 | 编辑弹窗：名称、路径（支持📁浏览选择）、部署配置、Git 远程、规则 |
| **🔑 修改密码** | 头部按钮 | 输入旧密码 + 新密码 + 确认 |
| **🏥 环境诊断** | 工具栏 | 检查工具链完整性 |
| **📋 运行日志** | 底部面板 | 实时操作日志 |
| **🖥️ 远程管理** | 头部 Tab | SSH 终端（xterm.js）+ 文件管理（左右分栏） |
| **📋 审计日志** | 头部按钮 | 查看/搜索/删除持久化审计日志 |
| **📊 所有报告** | 头部按钮 | 查看所有项目测试报告列表 |

### 4.4 API 路由（共 31 个）

| 路由 | 方法 | 功能 |
|------|------|------|
| `/api/check` | GET | 执行代码检查 |
| `/api/build` | GET | 执行构建 |
| `/api/test` | GET | 执行测试 + 持久化报告 |
| `/api/push` | GET | 推送 Git |
| `/api/status` | GET | 查看状态 |
| `/api/projects` | GET | 获取项目列表 |
| `/api/project` | POST | 保存项目配置 |
| `/api/deploy/test` | GET | 测试 SSH 连接 |
| `/api/auth/status` | GET | 查询认证状态 |
| `/api/auth/change-password` | POST | 修改密码 |
| `/api/report/latest` | GET | 获取最新测试报告 |
| `/api/report/list` | GET | 获取历史报告列表 |
| `/api/report/delete` | POST | 删除指定报告 |
| `/api/report/all` | GET | 获取所有项目测试报告（合并） |
| `/api/rules/` | GET | 查看规则文件内容（路径后接文件名） |
| `/api/local/ls` | GET | 列出本地目录（盘符/导航），用于项目路径浏览选择 |
| `/api/remote/projects` | GET | 获取可远程管理的服务器列表（含项目+独立服务器） |
| `/api/remote/servers` | GET/POST | 独立服务器 CRUD |
| `/api/remote/server` | POST | 删除独立服务器 |
| `/api/remote/term` | WebSocket | SSH 远程终端 |
| `/api/remote/ls` | GET | 远程文件列表 |
| `/api/remote/download-token` | GET | 生成一次性下载 token（绕过 Basic Auth 供浏览器原生下载） |
| `/api/remote/download` | GET | 下载远程文件（支持 token 免认证） |
| `/api/remote/upload` | POST | 上传文件到远程服务器 |
| `/api/remote/delete` | POST | 删除远程文件/目录 |
| `/api/remote/mkdir` | POST | 创建远程目录 |
| `/api/remote/disconnect` | POST | 断开并清理缓存的 SSH 连接 |
| `/api/log/append` | POST | 写入审计日志（前端 log() 调用） |
| `/api/log/query` | GET | 查询审计日志（按日期/级别/关键字） |
| `/api/log/dates` | GET | 列出有审计日志的日期 |
| `/api/log/delete` | POST | 删除指定日期的审计日志 |

---

## 五、认证系统

### 5.1 密码存储

```json
// auth.json
{
  "username": "admin",
  "salt": "zVVbK+shszb9rE4V3Whjng==",
  "hash": "6e5dbd2f8be8df9bc74553bc7f8ac6..."
}
```

- 密码使用 **SHA256 + 16 字节随机盐** 加密，不存明文
- 首次启动自动创建 `auth.json`
- 默认账号：`admin` / `123456`

### 5.2 修改密码路径

| 方式 | 命令/操作 |
|------|-----------|
| Web UI | 头部 🔑 按钮 → 输入旧密码 + 新密码 |
| CLI | `ci passwd admin myNewPass` |
| CLI 重置 | `ci passwd`（重置为 admin/123456） |

---

## 六、测试报告系统

### 6.1 报告生成（ci-runner.ps1 Invoke-Test）

| 项目类型 | 测试框架检测 | 报告来源 | 覆盖率 |
|---------|-------------|---------|--------|
| React | 检测 Jest / Vitest | JSON 输出解析 | `coverage/coverage-summary.json` |
| Vue | 检测 Vitest / Jest | JSON 输出解析 | `coverage/coverage-summary.json` |
| Maven | Surefire XML | `target/surefire-reports/TEST-*.xml` | `target/site/jacoco/jacoco.xml` |
| MavenMulti | 遍历各子模块 Surefire | 同上 | 同上 |

### 6.2 报告数据结构

```json
{
  "project": "pair-front",
  "action": "test",
  "status": "pass",
  "duration": "12.3s",
  "report": {
    "total": 42,
    "passed": 40,
    "failed": 2,
    "skipped": 0,
    "coverage": "85.2%",
    "failures": [
      { "suite": "App.test.tsx", "test": "renders welcome", "message": "期望的元素未找到" }
    ]
  }
}
```

### 6.3 报告持久化

| 路径 | 规则 |
|------|------|
| `reports/{项目名}/test-{时间戳}.json` | 每次测试自动保存 |
| 保留策略 | 最近 20 条，自动清理旧的 |

### 6.4 报告管理

| 操作 | CLI | Web UI |
|------|-----|--------|
| 查看最新 | `ci report <project>` | 📊 按钮 |
| 查看 JSON | `ci report <project> --json` | — |
| 列出历史 | `ci report <project> --list` | 报告弹窗底部 |
| 删除 | `ci report <project> --delete <id>` | 历史列表中❌按钮 |
| 测试后自动展示 | — | 点击「测试」后自动弹出 |

---

## 七、CI 执行引擎（ci-runner.ps1）

### 7.1 类型自动识别

| 检测特征 | 判定类型 |
|---------|---------|
| `package.json` + 依赖 `react` | React |
| `package.json` + 依赖 `vue` / `vue-router` | Vue |
| `package.json` + 依赖 `@angular/core` | Angular |
| `package.json` + 依赖 `next` | Next |
| `pom.xml` + `<modules>` | Maven 多模块 |
| `pom.xml` | Maven 单模块 |
| `build.gradle` | Gradle |
| `Cargo.toml` | Rust |
| `go.mod` | Go |

### 7.2 各阶段命令

| 类型 | check | build | test |
|------|-------|-------|------|
| React | `npx tsc --noEmit` + `npx eslint src/` | `npm run build` | Jest/Vitest 自动检测 |
| Vue | `npx vue-tsc --noEmit` + `npx eslint -c rules/` | `npm run build` | Vitest/Jest 自动检测 |
| Maven | `mvn compile -Xlint:all` + `mvn checkstyle:check` | `mvn clean package -DskipTests` | `mvn test -Dmaven.test.failure.ignore=true` |
| MavenMulti | 父目录执行 `mvn compile` | `mvn clean install -DskipTests` | 遍历子模块 Surefire |

---

## 八、部署引擎（cd-deploy.ps1）

### 8.1 部署阶段

| 阶段 | 协议 | 说明 |
|------|------|------|
| 上传 | SFTP / SCP | 将构建产物传输到目标服务器 |
| 控制 | SSH | 远程执行启动/停止/状态查询 |

### 8.2 各项目类型部署方式

| 类型 | 产物 | 上传目标 | 启动命令 |
|------|------|---------|---------|
| React/Vue | `dist/` | `$remote_dir/dist/` | `nginx -s reload` |
| Maven | `target/*.jar` | `$remote_dir/` | `java -jar *.jar` |
| MavenMulti | 各子模块 jar | `$remote_dir/services/` | `docker-compose up -d` |

### 8.3 部署配置

```json
{
  "host": "192.168.1.10",
  "port": 22,
  "user": "deploy",
  "remote_dir": "/opt/pair-front",
  "auth_type": "key"
}
```

---

## 九、全链路自动流水线

### 9.1 流水线顺序

```
check → build → test → push → deploy
```

### 9.2 自动模式

| 模式 | 行为 |
|------|------|
| 🌐 自动:OFF | 点一步执行一步，和传统 CI 一样 |
| 🌐 自动:ON | 某步通过后自动继续下一步，失败自动终止 |

### 9.3 触发方式

| 触发方式 | 说明 |
|----------|------|
| Web UI ▶ 流水线按钮 | 对单个项目执行全链路 |
| Web UI ⏯ 运行流水线 | 对所有项目执行全链路 |
| 自动模式 + 单步按钮 | 开启自动后，点击任意单步按钮自动串联后续步骤 |

### 9.4 实现原理

纯前端编排，不涉及后端改动：

```javascript
// 核心逻辑
async function runAction(action, project) {
  const data = await api(`/api/${action}?project=...`);
  if (data.status === 'pass' && autoPipeline) {
    const next = getNextStep(action);  // check → build → test → ...
    if (next) await runAction(next, project);
  }
}
```

---

## 十、认证与安全

### 10.1 认证方式

| 层级 | 方式 |
|------|------|
| Web UI | HTTP Basic Auth |
| 登录密码存储 | SHA256 + 16 字节随机盐，不存明文 |
| CLI 命令 | 本地执行，无需认证（依赖文件系统权限） |
| MCP 调用 | 由 Host 代理认证 |

### 10.2 密码策略

| 项目 | 规则 |
|------|------|
| 默认密码 | `admin` / `123456` |
| 最小长度 | 6 位 |
| 修改方式 | Web UI 或 `ci passwd` |
| 默认密码警告 | 启动时控制台输出 + 日志警告 |

### 10.3 服务器/部署密码加密存储

`projects.json` 和 `servers.json` 中的 SSH 密码（`deploy.password` / `server.password`）均以 **AES-256-GCM** 加密存储，磁盘上不存留明文。

| 项目 | 实现 |
|------|------|
| 加密算法 | AES-256-GCM（带 nonce + 认证标签） |
| 密钥 | 32 字节随机，存 `.secretkey` 文件（hex 编码，0600 权限） |
| 密文格式 | `enc:base64(nonce+ciphertext)`，`enc:` 前缀区分明文/密文 |
| 加密时机 | Web 保存项目/服务器时、CLI `server add` 时自动加密 |
| 自动迁移 | 加载配置时检测明文密码自动加密回写（兼容历史数据） |
| 解密时机 | SSH 连接时（`sshutil.BuildSSHConfig`）自动解密 |
| API 脱敏 | 所有 API 响应中 password 字段返回空值，不暴露密文 |

### 10.4 SSH 主机密钥校验

采用 **TOFU（Trust On First Use）** 策略，防止中间人攻击：

| 项目 | 实现 |
|------|------|
| Go 端 | `internal/sshutil` 实现 TOFU 回调，首次连接自动接受并写入 `.known_hosts`，后续严格校验，密钥不匹配则拒绝 |
| PowerShell 端 | `cd-deploy.ps1` 使用 `StrictHostKeyChecking=accept-new` + `UserKnownHostsFile` |
| known_hosts 文件 | `.known_hosts`，0600 权限 |

### 10.5 CSRF 防护

Web API 对所有状态变更方法（POST/PUT/DELETE）要求自定义请求头 `X-Requested-With: XMLHttpRequest`，浏览器跨站表单无法携带自定义头，从而阻止 CSRF 攻击。前端所有 POST 请求均已带上此头。

### 10.6 文件下载认证

浏览器原生下载（`<a download>` / iframe）无法可靠携带 Basic Auth 的 `Authorization` 头，故采用一次性 token 机制：

1. 前端 `fetch('/api/remote/download-token')`（带 Basic Auth）获取 32 字节随机 token
2. 用 `<a href="/api/remote/download?...&download_token=xxx" download>` 触发浏览器原生下载
3. token 一次性消费、60 秒过期，`basicAuth` 中间件放行带有效 token 的请求

### 10.7 并发安全

| 项目 | 实现 |
|------|------|
| 认证状态 | `activeAuth` 全局变量用 `sync.RWMutex` 保护读写 |
| SSH 连接缓存 | `getSSHClient` 用 double-check 锁模式，持锁期间不做网络 IO |
| known_hosts 写入 | 追加写入时串行化，避免并发写损坏 |

### 10.8 其他安全措施

| 项目 | 实现 |
|------|------|
| 配置原子写入 | `projects.json`/`servers.json`/`auth.json` 等用临时文件 + rename 原子替换 |
| 敏感文件权限 | 含密码的文件以 `0600` 权限写入 |
| 路径穿越校验 | `handleViewRuleFile` 用 `IsPathSafe` 二次校验，禁止访问 rules 目录外文件 |
| 输入校验 | `projectSaveHandler` 强类型反序列化 + 路径存在性/认证类型/端口/步骤 ID 校验 |
| 命令注入防护 | `ci-runner.ps1` 自定义命令用 `& $cmd @args` 直接调用，禁用 `Invoke-Expression` |
| SSH 连接回收 | 后台 reaper 每 5 分钟清理空闲超 30 分钟的连接，进程退出时统一关闭 |

---

## 十一、AI Agent 集成（MCP + Function Calling）

### 11.1 工具 Schema（ci describe）

输出涵盖 **13 个工具**，每个工具包含完整参数描述：

| 工具名 | 参数 |
|--------|------|
| `ci_check` | `project`（可选） |
| `ci_test` | `project`（可选），返回结构化报告 |
| `ci_build` | `project`（可选） |
| `ci_push` | `project`（可选） |
| `ci_deploy` | `project`（可选） |
| `ci_hooks` | `project`（可选） |
| `ci_list` | 无参数 |
| `ci_status` | `project`（可选） |
| `ci_passwd` | `username`（可选），`password`（可选） |
| `ci_report` | `project`（必填），`delete`（可选） |
| `ci_serve` | `port`（可选） |
| `ci_doctor` | 无参数 |
| `ci_project_list` | 无参数 |

### 11.2 MCP 服务器（ci-mcp-server.ps1）

```json
// .atomcode/mcp.json 配置
{
  "mcpServers": {
    "ci-cd": {
      "command": "powershell.exe",
      "args": ["-ExecutionPolicy", "Bypass", "-File", "D:\\Idea\\project\\ci-cd\\ci-mcp-server.ps1"]
    }
  }
}
```

MCP 服务器特性：
- `tools/list` → 返回 13 个工具的完整 Schema
- `tools/call` → 按工具类型独立处理参数（passwd/report/serve/doctor 各有独立逻辑）
- 输出格式 → JSON-RPC 2.0 标准
- 无需配置，Host 自动发现

### 11.3 OpenAI Function Calling

```json
// ci describe --format openai 输出示例
{
  "tools": [
    {
      "name": "ci_test",
      "description": "对指定项目执行单元测试...",
      "parameters": {
        "type": "object",
        "properties": {
          "project": { "type": "string", "description": "项目名称（可选）" }
        }
      }
    }
  ]
}
```

---

## 十二、环境诊断（ci doctor）

```bash
$ ci doctor

🏥 CI/CD 环境诊断
──────────────────────────────────────────────────
  ✅ Go                   已安装
  ✅ Git                  已安装
  ✅ Node.js              已安装
  ✅ Maven                已安装
  ✅ ci-runner.ps1        存在
  ✅ auth.json            存在
  ✅ 项目配置                 3 个项目, 2 启用, 1 已配置部署
──────────────────────────────────────────────────
✅ 环境正常
```

检查项目：
- 工具链：Go / Git / Node.js / Java / Maven
- 配置文件：`ci-runner.ps1` / `auth.json`
- 项目配置：数量、启用数、部署配置完整数

---

## 十三、技术依赖清单

| 依赖 | 用途 | 来源 |
|------|------|------|
| Go 1.22+ | 编译 ci.exe | 仅编译时需要 |
| PowerShell 5.1+ | 运行后端脚本 | Windows 自带 |
| `ssh.exe` | SSH 远程执行 | Win10 1809+ 自带 |
| `sftp.exe` | SFTP 文件传输 | Win10 1809+ 自带 |
| Git | 版本控制 + hooks | 已安装 |
| Node.js + npm | 前端构建 | 项目需要 |
| JDK + Maven | 后端构建 | 项目需要 |

**零外部第三方 CI 工具依赖**（不安装 Jenkins / GitLab Runner 等）。

---

## 十四、数据文件说明

| 文件 | 位置 | 管理方式 | 格式 |
|------|------|---------|------|
| 项目配置 | `projects.json` | Web UI 增删改 / CLI `deploy` | JSON（部署密码 AES 加密） |
| 认证信息 | `auth.json` | 自动创建 + Web UI/CLI 改密 | JSON（SHA256 哈希） |
| 独立服务器 | `servers.json` | Web UI（远程管理 Tab）/ CLI `server add/list/delete` | JSON（密码 AES 加密） |
| 加密主密钥 | `.secretkey` | 自动生成（0600 权限） | hex 编码 32 字节 |
| SSH 主机密钥 | `.known_hosts` | TOFU 自动写入（0600 权限） | OpenSSH known_hosts 格式 |
| 测试报告 | `reports/{project}/test-*.json` | 自动生成 + Web UI 可删 / CLI `report` | JSON |
| 审计日志 | `logs/audit-YYYY-MM-DD.jsonl` | 自动写入（前端 log()）+ Web UI/CLI 查删 | JSONL |
| 代码规则 | `rules/` | 手动编辑 + CLI `rules list/view` | xml/mjs |
| Git hooks | `hooks/` | 手动编辑 | batch |
| Web 前端 | `web/` | 内嵌（index.html + app.css + app.js） | HTML/CSS/JS |

---

## 十五、启动与使用

```bash
# 1. 编译
go build -o ci.exe ci-cd/cmd/ci

# 2. 启动 Web UI
ci.exe serve
# 浏览器打开 http://localhost:8080
# 默认账号: admin / 123456

# 3. 添加项目（Web UI 中点击「+ 添加项目」）

# 4. 开始使用
#   - 点击「检查」「构建」「测试」「推送」「部署」
#   - 打开 🌐 自动:ON 体验全链路自动化
#   - 点击 ▶ 流水线 执行完整流程

# 5. 查看报告
ci report pair-front          # CLI 查看
# 或 Web UI 中点击 📊 报告

# 6. 修改密码
ci passwd                     # 重置为默认
ci passwd admin newPass       # 修改密码

# 7. 环境诊断
ci doctor
```

# 本地 CI/CD 流水线完整方案

> 最后更新：2026-06-19  
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
│  │  │  commands.go    ← 15 个子命令定义                    │  │  │
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
│       ├── handler.go            ← HTTP handler + 13 个 API 路由
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
├── reports\                      ← 测试报告存储目录
│   └── {project}\
│       └── test-{timestamp}.json
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

### 3.2 完整命令列表（15 个）

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
| `serve` | 启动 Web UI 服务器 | `--port 8080`, `--no-open` |
| `doctor` | 诊断 CI/CD 环境状态 | `--json` |
| `project list` | 列出所有项目的详细信息 | `--json` |

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
| **➕ 添加项目** | 工具栏 | 编辑弹窗：名称、路径、部署配置、Git 远程、规则 |
| **🔑 修改密码** | 头部按钮 | 输入旧密码 + 新密码 + 确认 |
| **🏥 环境诊断** | 工具栏 | 检查工具链完整性 |
| **📋 运行日志** | 底部面板 | 实时操作日志 |

### 4.4 API 路由（共 13 个）

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
| 密码存储 | SHA256 + 16 字节随机盐 |
| CLI 命令 | 本地执行，无需认证（依赖文件系统权限） |
| MCP 调用 | 由 Host 代理认证 |

### 10.2 密码策略

| 项目 | 规则 |
|------|------|
| 默认密码 | `admin` / `123456` |
| 最小长度 | 6 位 |
| 修改方式 | Web UI 或 `ci passwd` |
| 默认密码警告 | 启动时控制台输出 + 日志警告 |

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
| 项目配置 | `projects.json` | Web UI 增删改 | JSON |
| 认证信息 | `auth.json` | 自动创建 + Web UI/CLI 改密 | JSON |
| 测试报告 | `reports/{project}/test-*.json` | 自动生成 + Web UI 可删 | JSON |
| 代码规则 | `rules/` | 手动编辑 | xml/mjs |
| Git hooks | `hooks/` | 手动编辑 | batch |

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

# CI/CD 命令行工具

> **定位**: 面向开发者的本地 CI/CD 辅助工具，非 Jenkins/GitLab CI 替代品。
> **理念**: 单二进制、零外部服务依赖、手动触发、AI Agent 友好。
> **适用场景**: 开发者在个人电脑上对项目执行代码检查 → 构建 → 测试 → 推送 → 部署。

本地 CI/CD 工具链，覆盖完整流程：代码检查 → 测试 → 构建 → 推送 → 部署。
所有核心逻辑由 **Go 原生执行**（不依赖 PowerShell），跨平台可编译运行。

---

## 快速启动 Web UI

```bash
ci.exe serve
```

浏览器自动打开 http://localhost:8080，在界面中管理所有项目。

---

## 常用命令

### 项目管理

| 命令 | 说明 |
|------|------|
| `ci list` | 列出所有项目 |
| `ci status [project]` | 查看项目构建状态 |

### CI 流程

| 命令 | 说明 |
|------|------|
| `ci check [project]` | 代码检查（类型检查 + ESLint/Checkstyle） |
| `ci build [project]` | 完整构建 |
| `ci test [project]` | 运行单元测试 |

### 推送与部署

| 命令 | 说明 |
|------|------|
| `ci push [project]` | 推送到所有 Git 远程仓库 |
| `ci deploy [project]` | 部署到远程服务器 |

> `[project]` 可选，不指定则操作所有项目。

### 其他

| 命令 | 说明 |
|------|------|
| `ci hooks [project]` | 安装 Git hooks |
| `ci doctor` | 诊断环境状态 |
| `ci describe` | 输出工具 Schema（供 AI Agent 发现） |
| `ci --help` | 查看全部命令帮助 |
| `ci --json` | 以 JSON 格式输出（适合脚本调用） |

### 全局参数

| 参数 | 说明 |
|------|------|
| `--json` | JSON 格式输出 |
| `-h, --help` | 显示帮助信息 |

示例：

```bash
ci check pair-front --json          # JSON 格式输出检查结果
ci build                            # 构建所有项目
ci deploy pair-front                # 部署到生产环境
```

---

## 首次使用

1. 启动 Web UI：`ci.exe serve`
2. 在浏览器中添加项目
3. 点击「检查」「构建」「部署」运行流水线

### 首次部署前：配置 SSH 密钥

部署功能需要 SSH 密钥认证，只需配置一次：

1. **本机生成密钥**：终端运行 `ssh-keygen -t ed25519`（一路回车）
2. **公钥上传服务器**：把 `~/.ssh/id_ed25519.pub` 的内容追加到服务器的 `~/.ssh/authorized_keys`
3. **Web UI 填写私钥路径**：部署配置 → 密钥路径填 `C:\Users\你的用户名\.ssh\id_ed25519`

> 详细说明见 CICD方案.md 8.3.1 节。

## 环境要求

- Go 1.22+（仅编译时需要，分发只需 ci.exe 单文件）
- 前端项目需安装 Node.js
- 后端项目需安装 JDK 17+ 和 Maven

> 已迁移至 Go 原生引擎，不再依赖 PowerShell。

---

## 代码检查规则配置

规则文件存放在 `rules/` 目录下，由 CI/CD 独立管控，**不入侵项目源码**。

### 规则文件说明

| 文件 | 适用项目 | 检测内容 |
|------|---------|---------|
| `rules/eslint-vue.mjs` | Vue 前端项目 | JavaScript/TypeScript 代码规范 |
| `rules/checkstyle.xml` | Maven 后端项目 | Java 代码风格、命名规范、Javadoc |

### 自动匹配规则

项目类型自动匹配对应规则文件：

| 项目类型 | 类型检查 | 代码规范规则 |
|---------|---------|------------|
| React | `npx tsc --noEmit` | 项目自带的 `eslint.config.js` |
| Vue | `npx vue-tsc --noEmit` | `rules/eslint-vue.mjs` |
| Maven | `mvn compile -q` | `rules/checkstyle.xml` |

> React 项目使用自身的 ESLint 配置，Vue 和后端项目由 CI/CD 统一提供规则。

### 自定义规则

在 `rules/` 目录下修改或新增规则文件：

```bash
# 修改 Vue 规则
edit rules/eslint-vue.mjs

# 修改 Java 规则
edit rules/checkstyle.xml

# 新增自定义规则（放到 rules/custom/ 目录）
mkdir rules/custom
copy my-rules.xml rules/custom/
```

修改后立即生效，无需重新编译。

### 项目级规则管控（Web UI）

在 Web UI 中编辑项目时，可以：

1. **查看适用规则** — 系统根据项目类型自动显示对应的规则文件
2. **单独启用/禁用** — 勾选需要执行的检查项，取消勾选的规则在执行检查时自动跳过

> 规则开关状态保存在 `projects.json` 的 `rules` 字段（如 `"rules":[{"id":"eslint","enabled":false}]`），按项目独立配置。
> 未配置时所有规则默认启用，保持向后兼容。
> 规则文件本身修改后对所有项目生效；如需对单个项目关闭某条规则，用 Web UI 的勾选框控制。

各项目类型可控制的规则 ID：

| 项目类型 | 可控规则 ID | 说明 |
|---------|-----------|------|
| React | `tsc` / `eslint` | TypeScript 类型检查 / ESLint |
| Vue | `tsc` / `eslint` | vue-tsc 类型检查 / ESLint（rules/eslint-vue.mjs） |
| Maven | `compile` / `checkstyle` | Maven 编译检查 / Checkstyle（rules/checkstyle.xml） |
| MavenMulti | `compile` | 多模块编译检查 |

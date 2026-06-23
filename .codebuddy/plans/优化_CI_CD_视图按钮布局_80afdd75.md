---
name: 优化 CI/CD 视图按钮布局
overview: 在保留现有设计系统（暖石中性色 + 翡翠绿、Fraunces/IBM Plex 字体）的前提下，通过溢出菜单/下拉菜单收纳次要操作，降低 CI/CD 视图三处按钮密度：Header 右侧（4→2）、顶部工具栏（7→4）、项目表格每行操作列（~12→7）。
design:
  architecture:
    framework: html
  styleKeywords:
    - Editorial Engineering
    - Warm stone neutrals
    - Emerald accent
    - Minimal density
    - Consistent design tokens
  fontSystem:
    fontFamily: IBM Plex Sans
    heading:
      size: 19px
      weight: 600
    subheading:
      size: 13px
      weight: 600
    body:
      size: 13px
      weight: 400
  colorSystem:
    primary:
      - "#047857"
      - "#059669"
    background:
      - "#faf9f7"
      - "#ffffff"
      - "#f4f2ee"
    text:
      - "#1c1917"
      - "#57534e"
      - "#a8a29e"
    functional:
      - "#be123c"
      - "#b45309"
      - "#6d28d9"
todos:
  - id: dropdown-component
    content: 在 app.css 新增 .dropdown/.dropdown-menu/.dropdown-item 样式，在 app-core.js 新增 toggleDropdown/closeAllDropdowns/全局点击关闭逻辑
    status: completed
  - id: header-toolbar-refactor
    content: 重构 index.html Header 右侧为 2 按钮 + 更多下拉，重构工具栏为 4 按钮 + 设置下拉，适配 app-pipeline.js 开关状态同步
    status: completed
    dependencies:
      - dropdown-component
  - id: table-actions-refactor
    content: 重构 app-projects.js renderActionButtons 为 7 按钮 + ⋯ 溢出菜单，缩减 index.html 表格操作列宽至 360px
    status: completed
    dependencies:
      - dropdown-component
  - id: state-sync-verify
    content: 验证所有开关状态与下拉菜单文案同步，确保明暗主题下菜单样式一致
    status: completed
    dependencies:
      - header-toolbar-refactor
      - table-actions-refactor
---

## 用户需求

优化 CI/CD 控制台前端页面的按钮密度问题。当前页面按钮过多，视觉拥挤，需要通过下拉菜单/溢出菜单收纳次要操作，降低按钮密度，同时保留现有设计风格（配色、字体、圆角不变）。

## 产品概述

针对 CI/CD 视图的三个区域进行按钮精简优化：

- Header 右侧：4 个按钮减少为 2 个（保留主题切换 + 更多下拉）
- 顶部工具栏：7 个按钮减少为 4 个（保留运行流水线/添加项目/刷新 + 设置下拉收纳 4 个开关）
- 项目表格操作列：每行约 12 个按钮减少为 7 个（保留 5 步骤按钮 + 流水线主按钮 + ⋯ 溢出菜单）

## 核心功能

- 新增轻量下拉菜单组件（纯 CSS + vanilla JS，复用于 Header/工具栏/表格行）
- Header 次要操作（审计日志/所有报告/修改密码）收入「更多」下拉
- 工具栏 4 个开关（自动/并发/通知/监听）收入「设置」下拉，菜单项展示当前 ON/OFF 状态
- 表格行次要操作（编辑/报告/产物/监听/取消/删除）收入 ⋯ 溢出菜单
- 开关状态与下拉菜单文案实时同步
- 表格操作列宽度从 520px 缩减至约 360px

## 技术栈

- 纯 vanilla HTML/CSS/JS（无框架），与现有项目一致
- CSS 变量系统保留：--accent 翡翠绿、--bg- *暖石色、--r-* 圆角、--font-display/sans/mono
- 明暗主题兼容（body.dark）
- 字体保留：Fraunces（标题）+ IBM Plex Sans（正文）+ IBM Plex Mono（代码）

## 实现方案

### 核心策略

新增一套轻量下拉菜单组件（`.dropdown` / `.dropdown-menu` / `.dropdown-item`），通过 CSS 定位 + JS 全局点击关闭实现，复用于三个优化点。所有现有 onclick 函数保持不变，仅改变其承载的 UI 元素位置。

### 下拉菜单组件设计

- **CSS**：`.dropdown`（relative 容器）+ `.dropdown-menu`（absolute 定位，默认 display:none，`.open` 时显示）+ `.dropdown-item`（菜单项，支持 hover 状态、状态标识、危险样式）
- **JS**：`toggleDropdown(triggerEl)` 函数切换菜单开关；`document.addEventListener('click', ...)` 全局监听点击外部关闭；防止事件冒泡
- **主题兼容**：菜单背景用 `var(--bg-panel)`，边框用 `var(--border)`，hover 用 `var(--bg-hover)`，与现有组件一致

### Header 优化（4 → 2）

- 保留 `[🌓 主题]` 按钮可见（高频操作）
- 新增 `[⋯ 更多]` 下拉按钮，菜单项：📋 审计日志 / 📊 所有报告 / 🔑 修改密码
- 各菜单项 onclick 保持调用 `showAuditLog()` / `showAllReports()` / `showPasswordDialog()`

### 工具栏优化（7 → 4）

- 保留 `[⏯ 运行流水线]` `[+ 添加项目]` `[🔄 刷新]` 可见
- 新增 `[⚙ 设置]` 下拉按钮，菜单项展示 4 个开关的当前状态：
- 🌐 自动流水线 — ON/OFF（点击调用 `toggleAutoPipeline()`）
- ⚡ 并发执行 — ON/OFF（点击调用 `toggleConcurrent()`）
- 🔔 桌面通知 — ON/OFF（点击调用 `toggleNotification()`）
- 👀 全局监听 — ON/OFF（点击调用 `toggleWatch()`，注：此函数当前未定义，属于遗留问题，保持原样不修复）
- ON 状态菜单项添加 `.active` 样式（success 色），OFF 状态为默认色
- 开关状态同步：在 `toggleAutoPipeline` / `toggleConcurrent` / `toggleNotification` 末尾调用 `updateSettingsMenu()` 刷新菜单文案

### 表格操作列优化（~12 → 7）

- 保留 5 个步骤按钮（检查/构建/测试/推送/部署）+ `[▶ 流水线]` 主按钮
- 新增 `[⋯]` 溢出按钮，下拉菜单项：
- ✏️ 编辑 → `editProject(name)`
- 📊 报告 → `showReport(name)`
- 📁 产物 → `openBuildDir(name)`
- 👀 监听: ON/OFF → `toggleWatchProject(name)`（动态展示当前状态）
- ⏸ 取消 → `cancelPipeline(name)`
- 🗑 删除 → `deleteProject(name)`（danger 样式）
- 操作列宽从 `width:520px` 缩减至 `width:360px`
- 每行下拉菜单需携带项目名参数，通过 `data-project` 属性传递

### 状态同步机制

- **工具栏设置菜单**：新增 `updateSettingsMenu()` 函数，在 `toggleAutoPipeline` / `toggleConcurrent` / `toggleNotification` 函数末尾调用，更新菜单项文案和样式
- **表格行 ⋯ 菜单**：监听状态随 `renderProjects()` 自然刷新（每次重渲染时 `renderActionButtons` 重新生成菜单 HTML）
- **原有开关按钮元素**：`autoToggle` / `concurrentToggle` / `notifToggle` 的 DOM 操作改为更新下拉菜单项，原 `getElementById` 引用需适配

## 实现注意事项

- **事件冒泡控制**：下拉触发按钮的 click 事件需 `stopPropagation()`，否则全局关闭监听会立即关闭刚打开的菜单
- **z-index 层级**：表格行内的下拉菜单需 `z-index: 50`，确保不被相邻行遮挡；Header 下拉 `z-index: 200`
- **表格行 hover**：现有 `.project-table tbody tr:hover` 设有背景色，下拉菜单打开时需保持可见
- **性能**：表格每行新增一个下拉组件，DOM 节点数增加可控（每行 +1 按钮 +1 菜单容器），无性能瓶颈
- **兼容性**：所有 onclick 函数签名不变，仅改变调用入口的 UI 元素；`toggleWatch()` 遗留问题保持原样

## 目录结构

```
internal/serve/web/
├── index.html              # [MODIFY] Header 右侧按钮重组（L32-38）；工具栏按钮重组（L63-76）；表格 thead 操作列宽改为 360px（L79）
├── app.css                 # [MODIFY] 新增 .dropdown / .dropdown-menu / .dropdown-item / .dropdown-trigger 样式；新增 .dropdown-item.danger / .dropdown-item.active 样式
├── app-core.js             # [MODIFY] 新增 toggleDropdown() / closeAllDropdowns() / updateSettingsMenu() 函数；新增全局点击关闭监听；toggleNotification() 末尾调用 updateSettingsMenu()
├── app-pipeline.js         # [MODIFY] toggleAutoPipeline() / toggleConcurrent() 末尾调用 updateSettingsMenu()；移除对 autoToggle/concurrentToggle DOM 元素的直接操作改为更新菜单
└── app-projects.js         # [MODIFY] renderActionButtons() 重构：保留步骤按钮+流水线按钮，其余生成 ⋯ 下拉菜单
```

## 关键代码结构

```css
/* 下拉菜单核心样式 */
.dropdown { position: relative; display: inline-block; }
.dropdown-menu {
  display: none;
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  min-width: 180px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  box-shadow: var(--shadow-xl);
  padding: 6px;
  z-index: 200;
}
.dropdown-menu.open { display: block; }
.dropdown-item {
  padding: 8px 14px;
  border-radius: var(--r-sm);
  cursor: pointer;
  font-size: 12.5px;
  color: var(--text-secondary);
  transition: all 0.12s ease;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.dropdown-item:hover { background: var(--bg-hover); color: var(--text-primary); }
.dropdown-item.active { color: var(--success); }
.dropdown-item.active .toggle-state { color: var(--success); }
.dropdown-item.danger { color: var(--danger); }
.dropdown-item.danger:hover { background: var(--danger-subtle); }
```

```javascript
// 下拉菜单控制
function toggleDropdown(triggerEl) {
  const menu = triggerEl.nextElementSibling;
  const isOpen = menu.classList.contains('open');
  closeAllDropdowns();
  if (!isOpen) menu.classList.add('open');
}

function closeAllDropdowns() {
  document.querySelectorAll('.dropdown-menu.open').forEach(m => m.classList.remove('open'));
}

// 全局点击关闭
document.addEventListener('click', (e) => {
  if (!e.target.closest('.dropdown')) closeAllDropdowns();
});
```

## 设计方案

保留现有「Editorial Engineering」设计风格（暖石中性色 + 翡翠绿点缀），仅通过布局重组降低按钮密度。新增的下拉菜单组件严格复用现有 CSS 变量和视觉语言：圆角 var(--r-md)、阴影 var(--shadow-xl)、背景 var(--bg-panel)、边框 var(--border)、hover 态 var(--bg-hover)。菜单项排版采用 flex 布局，左侧图标+文字、右侧状态标识，保持与现有按钮一致的视觉密度。

### Header 区域

- 左侧不变（Logo + Tab 导航）
- 右侧从 4 按钮缩减为 2 元素：状态文本 pill + [🌓 主题] + [⋯ 更多]下拉
- 「更多」下拉按钮复用 `.header-btn` 样式，保持视觉一致性

### 工具栏区域

- 左组：[⏯ 运行流水线] 主按钮 + [⚙ 设置] 下拉按钮（替代 4 个开关按钮）
- 右组：[+ 添加项目] + [🔄 刷新]
- 设置下拉菜单项展示开关状态：ON 项使用 success 色标注，OFF 项使用 tertiary 色标注

### 项目表格操作列

- 5 个步骤按钮保持现有样式（btn-primary/success/warning/danger）
- [▶ 流水线] 主按钮保持 btn-primary
- [⋯] 溢出按钮使用 `.action-btn` 样式，点击展开下拉菜单
- 危险操作（删除）使用 `.dropdown-item.danger` 红色标注
- 操作列宽从 520px 缩减至 360px，表格更紧凑

## Agent Extensions

### Skill

- **frontend-design**
- Purpose: 指导下拉菜单组件的视觉设计，确保菜单样式与现有设计系统一致，避免泛化 AI 美学
- Expected outcome: 下拉菜单组件在视觉上与现有按钮、卡片、弹窗风格无缝融合

### SubAgent

- **code-explorer**
- Purpose: 在实现阶段快速定位所有引用 autoToggle/concurrentToggle/notifToggle DOM 元素的代码路径，确保状态同步无遗漏
- Expected outcome: 确认所有开关状态更新点，避免重构后状态不同步
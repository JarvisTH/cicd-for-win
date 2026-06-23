// === 前端核心工具函数和常量 ===

// 全局状态
let projects = [];
let remoteCount = 0;
let autoPipeline = false;
let concurrentPipeline = false;
let runningCount = 0;

const stepIcons = {pass:'✅',fail:'❌',running:'⏳',pending:'⚪'};
const defaultStepOrder = ['check','build','test','push','deploy'];
const stepLabels = {check:'检查',build:'构建',test:'测试',push:'推送',deploy:'部署'};
const stepDescs = {
  check: '代码检查（类型检查 + Lint）',
  build: '编译构建（npm build / mvn package）',
  test: '单元测试（Jest / Vitest / Maven）',
  push: '推送代码到 Git 远程仓库',
  deploy: '部署到远程服务器（SFTP + 启动）'
};
const stepDefaults = {
  check: {
    desc: '代码检查（类型检查 + Lint）',
    items: [
      { type: 'React', command: 'npx tsc --noEmit', args: '' },
      { type: 'React', command: 'npx eslint', args: 'src/' },
      { type: 'Vue', command: 'npx vue-tsc --noEmit', args: '' },
      { type: 'Vue', command: 'npx eslint', args: '-c rules/eslint-vue.mjs src/' },
      { type: 'Maven', command: 'mvn compile', args: '-Xlint:all' },
      { type: 'Maven', command: 'mvn checkstyle:check', args: '-Dcheckstyle.config=rules/checkstyle.xml' },
      { type: 'MavenMulti', command: 'mvn compile', args: '-Xlint:all' },
    ],
    note: '按项目类型自动选择，多条命令顺序执行'
  },
  build: {
    desc: '编译构建',
    items: [
      { type: 'React/Vue', command: 'npm run build', args: '' },
      { type: 'Maven', command: 'mvn clean package', args: '-DskipTests' },
      { type: 'MavenMulti', command: 'mvn clean install', args: '-DskipTests' },
    ],
    note: ''
  },
  test: {
    desc: '单元测试',
    items: [
      { type: 'React/Vue (Vitest)', command: 'npx vitest run', args: '--reporter=json' },
      { type: 'React/Vue (Jest)', command: 'npx jest', args: '--json --coverage' },
      { type: 'Maven/MavenMulti', command: 'mvn test', args: '-Dmaven.test.failure.ignore=true' },
    ],
    note: '自动检测测试框架并解析报告（含覆盖率）'
  },
  push: {
    desc: 'Git 推送',
    items: [
      { type: '通用', command: 'git push', args: '--all' },
    ],
    note: '推送到所有已启用的远程仓库'
  },
  deploy: {
    desc: '部署',
    items: [
      { type: 'React/Vue', command: 'SFTP 上传 dist/', args: '→ $remote_dir/' },
      { type: 'Maven', command: 'SFTP 上传 target/*.jar', args: '→ $remote_dir/' },
      { type: 'MavenMulti', command: 'SFTP 上传各子模块 jar', args: '→ $remote_dir/services/' },
    ],
    note: '上传后远程执行启动/重启命令'
  },
};

const ruleTypeColors = {
  React: 'tag-react', Vue: 'tag-vue', Maven: 'tag-maven',
  MavenMulti: 'tag-maven', Node: 'tag-node', Unknown: 'tag-unknown',
};

const btnStyles = {
  check: 'btn-primary', build: 'btn-success', test: 'btn-warning',
  push: 'btn-warning', deploy: 'btn-danger'
};
const btnLabels = { check: '检查', build: '构建', test: '测试', push: '推送', deploy: '部署' };

// 步骤状态管理
if (!window._stepStatus) window._stepStatus = {};
if (!window._stepErrors) window._stepErrors = {};

function stepKey(p, s) { return (p.name || p) + ':' + s; }
function getStep(p, s) { return window._stepStatus[stepKey(p, s)] || 'pending'; }
function setStep(p, s, v) { window._stepStatus[stepKey(p, s)] = v; }

// API 请求
async function api(path) {
  try {
    const r = await fetch(path);
    if (!r.ok) throw new Error(r.statusText);
    return await r.json();
  } catch(e) { log(`❌ API 错误: ${e.message}`, 'error'); return null; }
}

async function apiPost(path, body) {
  try {
    const r = await fetch(path, {method:'POST', headers:{'Content-Type':'application/json','X-Requested-With':'XMLHttpRequest'}, body:JSON.stringify(body)});
    return await r.json();
  } catch(e) { log(`❌ API 错误: ${e.message}`, 'error'); return null; }
}

// 日志
function log(msg, type) {
  const el = document.getElementById('logContent');
  const cls = type === 'error' ? 'error' : type === 'warn' ? 'warn' : type === 'info' ? 'info' : '';
  if (el.innerHTML === '等待操作...') el.innerHTML = '';
  el.innerHTML += `<div class="${cls}">[${new Date().toLocaleTimeString()}] ${msg}</div>`;
  el.scrollTop = el.scrollHeight;
  fetch('/api/log/append', {
    method: 'POST',
    headers: {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'},
    body: JSON.stringify({message: msg, level: type || 'info'})
  }).catch(() => {});
}

function clearLog() { document.getElementById('logContent').innerHTML = '等待操作...'; }

// 主题
function toggleTheme() {
  document.body.classList.toggle('dark');
  localStorage.setItem('theme', document.body.classList.contains('dark') ? 'dark' : 'light');
}
if (localStorage.getItem('theme') === 'dark') document.body.classList.add('dark');

// HTML/JS 转义
function escJs(s) { return String(s).replace(/\\/g, '\\\\').replace(/'/g, "\\'"); }
function escHtml(s) { return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

function formatFileSize(bytes) {
  if (!bytes || bytes === 0) return '-';
  const units = ['B','KB','MB','GB'];
  let i = 0; let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(1) + ' ' + units[i];
}

function getFileIcon(name) {
  const ext = name.split('.').pop().toLowerCase();
  const icons = {js:'📜',ts:'📘',tsx:'⚛️',jsx:'⚛️',json:'📋',md:'📝',txt:'📄',html:'🌐',css:'🎨',xml:'📰',yml:'⚙️',yaml:'⚙️',sh:'💻',bat:'🪟',ps1:'🪟',jar:'📦',png:'🖼️',jpg:'🖼️',svg:'🎨',pdf:'📕',zip:'📦',gz:'📦'};
  return icons[ext] || '📄';
}

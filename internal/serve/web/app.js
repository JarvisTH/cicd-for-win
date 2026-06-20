let projects = [];
let remoteCount = 0;
let autoPipeline = false;
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
const stepDefaultCmds = {
  check: '自动检测: npx tsc --noEmit / npx eslint / mvn compile',
  build: '自动检测: npm run build / mvn clean package -DskipTests',
  test: '自动检测: npx jest --json / npx vitest run / mvn test',
  push: 'git push（推送到所有已配置的远程仓库）',
  deploy: 'SFTP 上传构建产物到远程服务器并重启服务'
};

// 获取项目的流水线步骤（按配置顺序，仅启用的）
function getProjectSteps(p) {
  if (!p || !p.pipeline || !p.pipeline.steps || p.pipeline.steps.length === 0) {
    return defaultStepOrder;
  }
  return p.pipeline.steps.filter(s => s.enabled).map(s => s.id);
}

// 获取项目的全部步骤（含禁用的，用于 stepper 显示）
function getProjectAllSteps(p) {
  if (!p || !p.pipeline || !p.pipeline.steps || p.pipeline.steps.length === 0) {
    return defaultStepOrder;
  }
  return p.pipeline.steps.map(s => s.id);
}

function getNextStep(project, current) {
  const steps = getProjectSteps(project);
  const idx = steps.indexOf(current);
  if (idx < 0 || idx >= steps.length - 1) return null;
  return steps[idx + 1];
}
function toggleAutoPipeline() {
  autoPipeline = !autoPipeline;
  const btn = document.getElementById('autoToggle');
  btn.textContent = autoPipeline ? '🌐 自动:ON' : '🌐 自动:OFF';
  if (autoPipeline) btn.classList.add('auto-on');
  else btn.classList.remove('auto-on');
  log(`🌐 自动流水线: ${autoPipeline ? '已开启' : '已关闭'}`, 'info');
}
if (!window._stepStatus) window._stepStatus = {};
const ruleDefs = {
  React:   [{id:'tsc', label:'TypeScript 类型检查', cmd:'npx tsc --noEmit', file:'内置（使用项目 tsconfig.json）', def:true, desc:'检查 TypeScript 类型错误，使用项目自身的 tsconfig 配置'}],
  Vue:     [{id:'tsc', label:'vue-tsc 类型检查', cmd:'npx vue-tsc --noEmit', file:'内置（使用项目 tsconfig.json）', def:true, desc:'Vue 项目的 TypeScript 类型检查，支持 .vue 文件'},
            {id:'eslint', label:'ESLint 代码规范', cmd:'npx eslint -c rules/eslint-vue.mjs src/', file:'rules/eslint-vue.mjs', def:true, desc:'ESLint 代码规范检查，使用 CI/CD 独立管控的规则配置'}],
  Maven:   [{id:'compile', label:'Maven 编译检查', cmd:'mvn compile -Xlint:all', file:'内置（使用项目 pom.xml）', def:true, desc:'Maven 编译检查，含 -Xlint:all 编译器警告'},
            {id:'checkstyle', label:'Checkstyle 代码风格', cmd:'mvn checkstyle:check -Dcheckstyle.config=rules/checkstyle.xml', file:'rules/checkstyle.xml', def:true, desc:'Checkstyle 代码风格检查，含命名规范、导入顺序等'}],
  MavenMulti:[{id:'compile', label:'多模块编译检查', cmd:'mvn compile -Xlint:all', file:'内置（使用项目 pom.xml）', def:true, desc:'多模块 Maven 项目编译检查'}],
  Node:    [{id:'eslint', label:'ESLint 代码规范', cmd:'npx eslint src/', file:'内置（使用项目 .eslintrc）', def:true, desc:'ESLint 检查，使用项目自身的 ESLint 配置'}],
  Unknown: [],
};

// 项目类型与标签颜色
const ruleTypeColors = {
  React: 'tag-react', Vue: 'tag-vue', Maven: 'tag-maven',
  MavenMulti: 'tag-maven', Node: 'tag-node', Unknown: 'tag-unknown',
};

function stepKey(p,s){return (p.name||p)+':'+s}
function getStep(p,s){return window._stepStatus[stepKey(p,s)]||'pending'}
function setStep(p,s,v){window._stepStatus[stepKey(p,s)]=v}

function renderStepper(p) {
  const steps = getProjectAllSteps(p);
  const stepMap = {};
  if (p && p.pipeline && p.pipeline.steps) {
    p.pipeline.steps.forEach(s => { stepMap[s.id] = s.enabled; });
  }
  return steps.map(s => {
    const st = getStep(p, s);
    const disabled = stepMap[s] === false;
    if (disabled) {
      return `<span class="step-item pending" style="opacity:0.35;text-decoration:line-through" title="已禁用">⊘ ${stepLabels[s]}</span>`;
    }
    return `<span class="step-item ${st}">${stepIcons[st]} ${stepLabels[s]}</span>`;
  }).join('<span class="step-arrow">→</span>');
}

// 生成流水线概览文本（如 "检查 → 构建 → 部署"）
function renderPipelineSummary(p) {
  const steps = getProjectSteps(p);
  if (steps.length === 0) return '<span style="color:var(--text-quaternary);font-size:11px">无启用步骤</span>';
  const allSteps = getProjectAllSteps(p);
  const isCustom = p && p.pipeline && p.pipeline.steps && p.pipeline.steps.length > 0;
  const summary = steps.map(s => stepLabels[s]).join(' → ');
  if (isCustom) {
    return `<span style="color:var(--accent);font-size:11px;font-weight:600" title="自定义流水线">⚙ ${summary}</span>`;
  }
  return `<span style="color:var(--text-tertiary);font-size:11px">${summary}</span>`;
}

function renderRules(path) {
  const el = document.getElementById('rulesList');
  let type = 'Unknown';
  const matched = projects.find(p => p.path === path);
  if (matched && matched.type) type = matched.type;

  const rules = ruleDefs[type] || [];

  // 顶部显示项目类型
  let html = `<div style="margin-bottom:10px;font-size:12px;color:var(--text-tertiary)">
    检测到项目类型: <span class="tag ${ruleTypeColors[type] || 'tag-unknown'}">${type}</span>
    ${rules.length > 0 ? `共 ${rules.length} 条规则` : '<span style="color:var(--warning)">无适用规则</span>'}
  </div>`;

  if (rules.length === 0) {
    html += `<div class="rules-info" style="border-left-color:var(--warning);background:var(--warning-subtle)">
      <strong style="color:var(--warning)">该类型项目暂无内置检查规则</strong><br>
      可在流水线配置中为 <code>check</code> 步骤设置自定义命令。<br>
      例如: <code>npx eslint src/</code> 或 <code>go vet ./...</code>
    </div>`;
    el.innerHTML = html;
    return;
  }

  rules.forEach(r => {
    const checked = r.def ? 'checked' : '';
    const hasRuleFile = r.file && r.file.startsWith('rules/');
    html += `<div class="rule-item">
      <input type="checkbox" class="rule-cb" data-id="${r.id}" ${checked}>
      <div class="rule-content">
        <div class="rule-label">${r.label}</div>
        <div class="rule-cmd">$ ${r.cmd}</div>
        <div class="rule-file">
          📄 规则文件: <code>${r.file}</code>
          ${hasRuleFile ? `<button class="rule-view-btn" onclick="viewRuleFile('${r.file}')">查看内容</button>` : ''}
        </div>
        <div style="font-size:11px;color:var(--text-tertiary);margin-top:3px;line-height:1.5">${r.desc || ''}</div>
      </div>
    </div>`;
  });
  el.innerHTML = html;
}

// 查看规则文件内容
async function viewRuleFile(filePath) {
  const modal = document.getElementById('reportModal');
  let content = '加载中...';
  try {
    const resp = await fetch(`/api/rules?file=${encodeURIComponent(filePath)}`);
    if (resp.ok) {
      content = await resp.text();
    } else {
      content = `无法读取规则文件 ${filePath}\n\n该文件位于 CI/CD 安装目录下。\n路径: ${filePath}`;
    }
  } catch(e) {
    content = `无法读取规则文件: ${e.message}\n\n该文件位于 CI/CD 安装目录下。\n路径: ${filePath}`;
  }
  const ext = filePath.split('.').pop().toLowerCase();
  const langLabel = ext === 'mjs' || ext === 'js' ? 'JavaScript' : ext === 'xml' ? 'XML' : 'Text';
  modal.innerHTML = `<div class="modal-content" style="width:720px;max-width:90vw">
    <h2>📄 规则文件预览</h2>
    <div style="margin-bottom:10px;display:flex;align-items:center;gap:8px">
      <span style="font-size:12px;color:var(--text-tertiary)">文件:</span>
      <code style="font-family:var(--font-mono);font-size:12px;color:var(--accent);background:var(--accent-subtle);padding:2px 8px;border-radius:var(--r-xs)">${filePath}</code>
      <span class="tag tag-unknown" style="margin-left:auto">${langLabel}</span>
    </div>
    <div style="background:var(--bg-input);border:1px solid var(--border-subtle);border-radius:var(--r-md);padding:16px;font-family:var(--font-mono);font-size:12px;line-height:1.7;overflow:auto;max-height:55vh;white-space:pre-wrap;color:var(--text-secondary)">${escapeHtml(content)}</div>
    <div style="margin-top:10px;font-size:11px;color:var(--text-tertiary);line-height:1.6">
      💡 此文件位于 CI/CD 安装目录的 <code style="background:var(--bg-elevated);padding:1px 5px;border-radius:var(--r-xs);font-family:var(--font-mono);font-size:10px;color:var(--accent)">rules/</code> 下。修改此文件可自定义检查规则，无需改动项目源码。
    </div>
    <div class="modal-actions">
      <button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="padding:8px 20px">关闭</button>
    </div>
  </div>`;
  modal.classList.add('active');
}

// ===== 流水线配置 UI =====
let pipelineEditingSteps = []; // 编辑中的步骤数据

function renderPipelineConfig(project) {
  // 初始化编辑数据：从项目配置或默认值
  if (project && project.pipeline && project.pipeline.steps && project.pipeline.steps.length > 0) {
    pipelineEditingSteps = JSON.parse(JSON.stringify(project.pipeline.steps));
    // 确保每个步骤都有 command/args 字段
    pipelineEditingSteps.forEach(s => {
      if (s.command === undefined) s.command = '';
      if (s.args === undefined) s.args = '';
    });
  } else {
    pipelineEditingSteps = defaultStepOrder.map(id => ({ id, enabled: true, command: '', args: '' }));
  }
  renderPipelineSteps();
}

function renderPipelineSteps() {
  const container = document.getElementById('pipelineConfig');
  container.innerHTML = '';
  // 概览栏：显示当前启用的步骤链
  const enabledSteps = pipelineEditingSteps.filter(s => s.enabled).map(s => stepLabels[s.id] || s.id);
  const overview = document.createElement('div');
  overview.style.cssText = 'padding:8px 12px;background:var(--accent-subtle);border-radius:var(--r-sm);font-size:12px;color:var(--accent);font-weight:600;margin-bottom:6px';
  overview.innerHTML = `📋 当前流水线: ${enabledSteps.length > 0 ? enabledSteps.join(' → ') : '<span style="color:var(--danger)">无启用步骤</span>'}`;
  container.appendChild(overview);
  pipelineEditingSteps.forEach((step, index) => {
    const div = document.createElement('div');
    div.className = 'pipeline-step' + (step.enabled ? '' : ' disabled');
    div.draggable = true;
    div.dataset.index = index;
    const hasCustomCmd = step.command && step.command.trim();
    div.innerHTML = `
      <div class="step-header">
        <span class="drag-handle">⠿</span>
        <span class="step-label">${stepLabels[step.id] || step.id}</span>
        ${hasCustomCmd ? '<span style="font-size:9px;color:var(--warning);font-weight:700;padding:1px 5px;background:var(--warning-subtle);border-radius:9999px">自定义</span>' : ''}
        <button class="expand-btn" onclick="toggleStepExpand(${index})">${hasCustomCmd ? '编辑' : '高级'} ▾</button>
        <div class="step-toggle" onclick="togglePipelineStep(${index})" title="${step.enabled ? '点击禁用' : '点击启用'}"></div>
      </div>
      <div class="step-body">
        <div style="font-size:10px;color:var(--text-quaternary);line-height:1.5">默认: ${stepDefaultCmds[step.id] || ''}</div>
        <div class="cmd-row">
          <label>命令</label>
          <input type="text" placeholder="留空使用默认命令（如: npm run build）" value="${step.command || ''}" oninput="updateStepCommand(${index}, this.value)">
        </div>
        <div class="cmd-row">
          <label>参数</label>
          <input type="text" placeholder="额外参数（如: --mode production）" value="${step.args || ''}" oninput="updateStepArgs(${index}, this.value)">
        </div>
      </div>
    `;
    // 拖拽事件
    div.addEventListener('dragstart', (e) => {
      div.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', index);
    });
    div.addEventListener('dragend', () => {
      div.classList.remove('dragging');
      document.querySelectorAll('.pipeline-step').forEach(el => el.classList.remove('drag-over'));
    });
    div.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      div.classList.add('drag-over');
    });
    div.addEventListener('dragleave', () => {
      div.classList.remove('drag-over');
    });
    div.addEventListener('drop', (e) => {
      e.preventDefault();
      const fromIndex = parseInt(e.dataTransfer.getData('text/plain'));
      const toIndex = parseInt(div.dataset.index);
      if (fromIndex === toIndex) return;
      // 移动数组元素
      const [moved] = pipelineEditingSteps.splice(fromIndex, 1);
      pipelineEditingSteps.splice(toIndex, 0, moved);
      renderPipelineSteps();
    });
    container.appendChild(div);
  });
}

function togglePipelineStep(index) {
  if (pipelineEditingSteps[index]) {
    pipelineEditingSteps[index].enabled = !pipelineEditingSteps[index].enabled;
    renderPipelineSteps();
  }
}

function toggleStepExpand(index) {
  const container = document.getElementById('pipelineConfig');
  const stepEl = container.children[index + 1]; // +1 跳过 overview
  if (stepEl) stepEl.classList.toggle('expanded');
}

function updateStepCommand(index, value) {
  if (pipelineEditingSteps[index]) {
    pipelineEditingSteps[index].command = value;
  }
}

function updateStepArgs(index, value) {
  if (pipelineEditingSteps[index]) {
    pipelineEditingSteps[index].args = value;
  }
}

function resetPipeline() {
  pipelineEditingSteps = defaultStepOrder.map(id => ({ id, enabled: true, command: '', args: '' }));
  renderPipelineSteps();
}

function getPipelineConfigFromUI() {
  return { steps: JSON.parse(JSON.stringify(pipelineEditingSteps)) };
}

function log(msg, type) {
  const el = document.getElementById('logContent');
  const cls = type === 'error' ? 'error' : type === 'warn' ? 'warn' : type === 'info' ? 'info' : '';
  if (el.innerHTML === '等待操作...') el.innerHTML = '';
  el.innerHTML += `<div class="${cls}">[${new Date().toLocaleTimeString()}] ${msg}</div>`;
  el.scrollTop = el.scrollHeight;
  // 持久化到磁盘（异步，不阻塞）
  fetch('/api/log/append', {
    method: 'POST',
    headers: {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'},
    body: JSON.stringify({message: msg, level: type || 'info'})
  }).catch(() => {});
}

function toggleSection(name) {
  const body = document.getElementById(name + 'Section');
  const arrow = document.getElementById(name + 'Arrow');
  if (!body || !arrow) return;
  body.classList.toggle('collapsed');
  arrow.classList.toggle('open');
}
function clearLog() { document.getElementById('logContent').innerHTML = '等待操作...'; }

function toggleTheme() {
  document.body.classList.toggle('dark');
  localStorage.setItem('theme', document.body.classList.contains('dark') ? 'dark' : 'light');
}
if (localStorage.getItem('theme') === 'dark') document.body.classList.add('dark');

async function api(path) {
  try {
    const r = await fetch(path);
    if (!r.ok) throw new Error(r.statusText);
    return await r.json();
  } catch(e) { log(`❌ API 错误: ${e.message}`, 'error'); return null; }
}

async function apiPost(path, body) {
  try {
    const r = await fetch(path, {method:'POST',headers:{'Content-Type':'application/json','X-Requested-With':'XMLHttpRequest'},body:JSON.stringify(body)});
    return await r.json();
  } catch(e) { log(`❌ API 错误: ${e.message}`, 'error'); return null; }
}

// ===== 修改密码 =====
function showPasswordDialog() {
  document.getElementById('inputOldPass').value = '';
  document.getElementById('inputNewPass').value = '';
  document.getElementById('inputConfirmPass').value = '';
  document.getElementById('passwordMsg').textContent = '';
  document.getElementById('passwordModal').classList.add('active');
}
function closePasswordDialog() {
  document.getElementById('passwordModal').classList.remove('active');
}
async function changePassword() {
  const oldPass = document.getElementById('inputOldPass').value;
  const newPass = document.getElementById('inputNewPass').value;
  const confirmPass = document.getElementById('inputConfirmPass').value;
  const msgEl = document.getElementById('passwordMsg');

  if (!oldPass || !newPass) { msgEl.innerHTML = '<span style="color:var(--danger)">请填写完整</span>'; return; }
  if (newPass.length < 6) { msgEl.innerHTML = '<span style="color:var(--danger)">新密码不能少于 6 位</span>'; return; }
  if (newPass !== confirmPass) { msgEl.innerHTML = '<span style="color:var(--danger)">两次密码不一致</span>'; return; }

  const data = await apiPost('/api/auth/change-password', {old_password: oldPass, new_password: newPass});
  if (data && data.status === 'ok') {
    msgEl.innerHTML = '<span style="color:var(--success)">✅ 密码修改成功</span>';
    setTimeout(closePasswordDialog, 1500);
    log('🔑 密码已修改', 'info');
  } else {
    msgEl.innerHTML = `<span style="color:var(--danger)">❌ ${data?.error || '修改失败'}</span>`;
  }
}

// ===== 环境诊断 =====
async function runDoctor() {
  log('🔍 开始环境诊断...', 'info');
  let incompleteCount = 0;
  projects.forEach(p => { if (!p.deploy || !p.deploy.host) { incompleteCount++; } });
  if (incompleteCount > 0) log(`⚠ ${incompleteCount} 个项目缺少部署配置`, 'warn');
  const html = [`<h3>🏥 环境诊断</h3>
    <div class="doctor-item"><span class="icon">${projects.length > 0 ? '✅' : '⚠️'}</span><span class="label">项目配置</span><span class="value">${projects.length} 个项目</span></div>
    <div class="doctor-item"><span class="icon">${incompleteCount === 0 ? '✅' : '⚠️'}</span><span class="label">部署配置</span><span class="value">${projects.length - incompleteCount} 个完整, ${incompleteCount} 个未配置</span></div>
    <div class="doctor-item"><span class="icon">${navigator.onLine ? '✅' : '❌'}</span><span class="label">网络连接</span><span class="value">${navigator.onLine ? '在线' : '离线'}</span></div>
  `];
  if (incompleteCount > 0) {
    html.push(`<div style="margin-top:10px;padding:12px;background:var(--bg-elevated);border:1px solid var(--border-subtle);border-radius:var(--r-md);font-size:13px">
      <strong style="color:var(--warning)">💡 建议:</strong>
      <ul style="margin:8px 0 0 16px;color:var(--text-tertiary)">
        <li>编辑缺少部署配置的项目，填写服务器信息</li>
        <li>或执行 <code style="background:var(--bg-panel);padding:1px 6px;border-radius:var(--r-xs);font-family:var(--font-mono);font-size:11px">ci init deploy &lt;项目名&gt;</code></li>
      </ul>
    </div>`);
  }
  document.getElementById('doctorCard').innerHTML = html.join('');
  document.getElementById('doctorCard').style.display = 'block';
  setTimeout(() => { document.getElementById('doctorCard').style.display = 'none'; }, 15000);
  log('✅ 环境诊断完成', 'info');
}

// ===== 检查规则说明弹窗 =====
function showRulesHelp() {
  const modal = document.getElementById('reportModal');
  const typeRules = Object.entries(ruleDefs).filter(([t, rs]) => rs.length > 0);
  let rulesHtml = '';
  typeRules.forEach(([type, rules]) => {
    rulesHtml += `<div style="margin-bottom:16px">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
        <span class="tag ${ruleTypeColors[type] || 'tag-unknown'}">${type}</span>
        <span style="font-size:12px;color:var(--text-tertiary)">${rules.length} 条规则</span>
      </div>`;
    rules.forEach(r => {
      rulesHtml += `<div class="rule-item" style="margin-bottom:4px">
        <div class="rule-content">
          <div class="rule-label">${r.label}</div>
          <div class="rule-cmd">$ ${r.cmd}</div>
          <div style="font-size:11px;color:var(--text-tertiary);margin-top:2px">${r.desc}</div>
        </div>
      </div>`;
    });
    rulesHtml += `</div>`;
  });

  modal.innerHTML = `<div class="modal-content" style="width:680px;max-width:90vw">
    <h2>🔍 代码检查规则说明</h2>
    <div class="rules-info">
      <strong>什么是代码检查？</strong><br>
      代码检查（<code>check</code>）是流水线的第一个步骤，在构建前自动检测代码质量问题和类型错误。<br><br>
      <strong>规则如何工作？</strong><br>
      1. 系统自动检测项目类型（React / Vue / Maven / Node 等）<br>
      2. 根据项目类型匹配对应的检查规则<br>
      3. 每条规则可单独启用/禁用（在项目编辑 → 代码检查规则）<br>
      4. 规则文件位于 CI/CD 目录的 <code>rules/</code> 下，可自由修改<br><br>
      <strong>规则与项目源码的关系？</strong><br>
      规则文件由 CI/CD 独立管控，<strong>不入侵项目源码</strong>。修改规则不会影响项目本身的配置文件。
    </div>
    <h3 style="font-size:13px;color:var(--text-secondary);margin:16px 0 10px;font-weight:700">各项目类型的默认规则</h3>
    <div style="max-height:400px;overflow-y:auto">
      ${rulesHtml}
    </div>
    <div class="modal-actions">
      <button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="padding:8px 20px">知道了</button>
    </div>
  </div>`;
  modal.classList.add('active');
}

// ===== 项目管理 =====
async function refreshProjects() {
  const data = await api('/api/projects');
  projects = data?.projects || [];
  const welcome = document.getElementById('welcomeCard');
  welcome.style.display = projects.length === 0 ? 'block' : 'none';
  remoteCount = 0;
  projects.forEach(p => { if (p.remotes) remoteCount += p.remotes.filter(r => r.enabled !== false).length; });
  renderProjects();
}

function renderProjects() {
  const tbody = document.getElementById('projectBody');
  let pass = 0, fail = 0, deployed = 0;
  tbody.innerHTML = '';
  projects.forEach(p => {
    const s = p.status || 'pending';
    if (s === 'pass') pass++; else if (s === 'fail') fail++;
    if (p.deploy?.host) deployed++;
    const tt = (p.type||'unknown').toLowerCase();
    const configWarn = (!p.deploy || !p.deploy.host) ? '<span style="color:var(--warning);font-size:11px">⚠ 未配置部署</span>' : '<span style="color:var(--success);font-size:11px">✅ 已配置</span>';

    document.getElementById('totalCount').textContent = projects.length;
    document.getElementById('passCount').textContent = pass;
    document.getElementById('failCount').textContent = fail;
    document.getElementById('deployCount').textContent = deployed;
    document.getElementById('remoteCount').textContent = remoteCount;

    tbody.innerHTML += `<tr>
      <td><strong>${p.name}</strong></td>
      <td><span class="tag tag-${tt}">${p.type||'未知'}</span></td>
      <td style="font-size:12px;font-family:var(--font-mono);color:var(--text-secondary)">${p.version || '-'}</td>
      <td style="font-size:11px;color:var(--text-tertiary)">${p.git_branch || '-'}${p.git_commit ? '<br><code style="font-size:10px;background:var(--bg-elevated);padding:1px 5px;border-radius:var(--r-xs);font-family:var(--font-mono);color:var(--accent-hover)">'+p.git_commit.substring(0,7)+'</code>' : ''}</td>
      <td><span class="status status-${s}"><span class="status-dot"></span>${s === 'pass' ? '通过' : s === 'fail' ? '失败' : '等待'}</span></td>
      <td>
        ${configWarn}
        ${p.remotes && p.remotes.length > 0 ? '<span style="color:var(--purple);font-size:11px">📤 ' + p.remotes.filter(r=>r.enabled!==false).map(r=>r.name).join(', ') + '</span>' : '<span style="color:var(--text-quaternary);font-size:11px">无远程仓库</span>'}
      </td>
      <td>${renderPipelineSummary(p)}</td>
      <td><div class="stepper">${renderStepper(p)}</div></td>
      <td>
        ${renderActionButtons(p)}
      </td>
    </tr>`;
  });
}

// 生成项目操作按钮（根据流水线配置显示启用的步骤）
function renderActionButtons(p) {
  const allSteps = getProjectAllSteps(p);
  const stepMap = {};
  if (p.pipeline && p.pipeline.steps) {
    p.pipeline.steps.forEach(s => { stepMap[s.id] = s.enabled; });
  }
  const btnStyles = {
    check: 'btn-primary', build: 'btn-success', test: 'btn-warning',
    push: 'btn-warning', deploy: 'btn-danger'
  };
  const btnLabels = { check: '检查', build: '构建', test: '测试', push: '推送', deploy: '部署' };

  let html = '';
  // 按配置顺序显示启用的步骤按钮
  allSteps.forEach(s => {
    const enabled = stepMap[s] !== false; // 默认启用
    if (enabled) {
      if (s === 'deploy') {
        html += `<button class="action-btn ${btnStyles[s]}" onclick="runDeploy('${p.name}')">${btnLabels[s]}</button>`;
      } else {
        html += `<button class="action-btn ${btnStyles[s]}" onclick="runAction('${s}','${p.name}')">${btnLabels[s]}</button>`;
      }
    }
  });
  html += `<button class="action-btn btn-outline" onclick="editProject('${p.name}')">编辑</button>`;
  html += `<button class="action-btn btn-outline" onclick="showReport('${p.name}')" style="font-size:10px">📊 报告</button>`;
  html += `<button class="action-btn btn-primary" onclick="runSinglePipeline('${p.name}')" style="font-size:10px">▶ 流水线</button>`;
  html += `<button class="action-btn btn-danger" onclick="deleteProject('${p.name}')" style="font-size:10px" title="删除项目">🗑</button>`;
  return html;
}

// ===== 操作执行 =====
async function runAction(action, project) {
  const p = projects.find(x => x.name === project);
  const start = Date.now();

  setStep(p, action, 'running');
  renderProjects();

  if (action === 'deploy') {
    if (!p || !p.deploy || !p.deploy.host) {
      log(`❌ [${project}] 未配置部署信息，请先编辑项目`, 'error');
      log(`💡 点击 [编辑] 填写主机地址和远程路径`, 'info');
      setStep(p, action, 'pending');
      return;
    }
  }

  log(`[${project}] ${action}...`);
  document.getElementById('statusText').textContent = `${action} ${project}...`;
  const data = await api(`/api/${action}?project=${encodeURIComponent(project)}`);
  const elapsed = ((Date.now() - start) / 1000).toFixed(1);

  if (data && data.status === 'pass') {
    setStep(p, action, 'pass');
    log(`✅ [${project}] ${action} 通过 (${elapsed}s)`, 'info');

    if (action === 'test') {
      await new Promise(r => setTimeout(r, 100));
      showReport(project, data);
    }

    if (autoPipeline) {
      const next = getNextStep(p, action);
      if (next) {
        log(`🔄 自动流水线: ${stepLabels[action]} ✅ → 继续 ${stepLabels[next]}...`, 'info');
        await new Promise(r => setTimeout(r, 200));
        await runAction(next, project);
      } else {
        log(`🎉 [${project}] 流水线全部完成！`, 'info');
      }
    }
  } else if (data) {
    setStep(p, action, 'fail');
    log(`❌ [${project}] ${action} 失败 (${elapsed}s)`, 'error');
    if (autoPipeline) {
      log(`⏹ 自动流水线: ${stepLabels[action]} 失败，流水线终止`, 'warn');
    }
    if (action === 'check') {
      log(`📋 错误详情: 请查看上方输出`, 'error');
      log(`💡 修复建议:`, 'warn');
      log(`   1. 根据错误信息修改对应文件`, 'warn');
      log(`   2. 修改后重新执行: ci check ${project}`, 'warn');
    }
    if (action === 'deploy') {
      log(`📋 诊断信息:`, 'warn');
      log(`   Step 1: ping → 检查服务器是否在线`, 'warn');
      log(`   Step 2: DNS → 检查主机名解析`, 'warn');
      log(`   Step 3: SSH → 检查端口和认证`, 'warn');
      log(`💡 可能原因:`, 'warn');
      log(`   1. 目标服务器未开机`, 'warn');
      log(`   2. SSH 服务未启动`, 'warn');
      log(`   3. 认证方式不匹配（推荐使用 SSH 密钥）`, 'warn');
    }
  }
  document.getElementById('statusText').textContent = '就绪';
  refreshProjects();
}

function runDeploy(project) {
  const p = projects.find(x => x.name === project);
  if (!p) return;
  if (!p.deploy || !p.deploy.host) {
    log(`❌ [${project}] 未配置部署信息`, 'error');
    log(`💡 请点击 [编辑] 填写部署配置`, 'info');
    return;
  }
  runAction('deploy', project);
}

async function batchRun(action) {
  log(`🔄 批量 ${action}...`, 'info');
  for (const p of projects) await runAction(action, p.name);
  log(`✅ 批量 ${action} 完成`, 'info');
}

async function runPipelineAll() {
  log('⏯ 开始全链路流水线...', 'info');
  for (const p of projects) {
    const steps = getProjectSteps(p);
    if (steps.length === 0) {
      log(`⏭ [${p.name}] 无启用的流水线步骤，跳过`, 'warn');
      continue;
    }
    log(`🔷 [${p.name}] 开始流水线 (${steps.join(' → ')})`, 'info');
    await runAction(steps[0], p.name);
  }
  log('✅ 全链路流水线执行完毕', 'info');
}

async function runSinglePipeline(project) {
  const p = projects.find(x => x.name === project);
  if (!p) return;
  const steps = getProjectSteps(p);
  if (steps.length === 0) {
    log(`⏭ [${project}] 无启用的流水线步骤`, 'warn');
    return;
  }
  log(`▶️ 开始流水线: ${project} (${steps.join(' → ')})`, 'info');
  await runAction(steps[0], project);
  log(`✅ 流水线完成: ${project}`, 'info');
}

// ===== 项目编辑弹窗 =====
function showAddDialog() {
  document.getElementById('modalTitle').textContent = '添加项目';
  ['inputName','inputPath','inputHost','inputPort','inputUser','inputRemote','inputKeyPath'].forEach(id => document.getElementById(id).value = '');
  document.getElementById('inputPort').value = '22';
  document.getElementById('inputTarget').value = 'production';
  document.getElementById('inputAuthType').value = 'key';
  document.getElementById('remoteList').innerHTML = '';
  document.getElementById('testResult').textContent = '';
  toggleKeyPath();
  renderPipelineConfig(null);
  document.getElementById('projectModal').classList.add('active');
}

function toggleKeyPath() {
  const v = document.getElementById('inputAuthType').value;
  document.getElementById('keyPathGroup').style.display = v === 'key' ? 'block' : 'none';
}

function editProject(name) {
  const p = projects.find(x => x.name === name); if (!p) return;
  document.getElementById('modalTitle').textContent = `编辑: ${name}`;
  document.getElementById('inputName').value = p.name;
  document.getElementById('inputPath').value = p.path || '';
  document.getElementById('inputTarget').value = p.deployTarget || 'production';
  document.getElementById('inputHost').value = p.deploy?.host || '';
  document.getElementById('inputPort').value = p.deploy?.port || '22';
  document.getElementById('inputUser').value = p.deploy?.user || '';
  document.getElementById('inputAuthType').value = p.deploy?.auth_type || 'key';
  document.getElementById('inputKeyPath').value = p.deploy?.identity_file || '';
  document.getElementById('inputRemote').value = p.deploy?.remote_dir || '';
  document.getElementById('remoteList').innerHTML = '';
  document.getElementById('testResult').textContent = '';
  toggleKeyPath();
  if (p.remotes) p.remotes.forEach(r => addRemoteRow(r.name, r.url, r.enabled));
  renderRules(p.path);
  renderPipelineConfig(p);
  ['deploy','git','rules'].forEach(s => {
    const body = document.getElementById(s+'Section');
    const arrow = document.getElementById(s+'Arrow');
    if (body) body.classList.remove('collapsed');
    if (arrow) arrow.classList.add('open');
  });
  document.getElementById('projectModal').classList.add('active');
}

function testConnection() {
  const host = document.getElementById('inputHost').value;
  const port = document.getElementById('inputPort').value;
  const user = document.getElementById('inputUser').value;
  const auth = document.getElementById('inputAuthType').value;
  const key = document.getElementById('inputKeyPath').value;
  const el = document.getElementById('testResult');
  if (!host || !user) { el.textContent = '❌ 请填写主机和用户名'; return; }
  el.textContent = '⏳ 测试中...';
  const params = new URLSearchParams({ host, port, user, auth_type: auth, identity_file: key });
  fetch(`/api/deploy/test?${params}`).then(r => r.json()).then(data => { el.textContent = data.status === 'ok' ? '✅ 连接成功' : '❌ 连接失败'; }).catch(() => { el.textContent = '❌ 连接失败'; });
}

function closeModal() { document.getElementById('projectModal').classList.remove('active'); }

// ========================================================================
// 本地目录浏览器（选择项目路径）
// ========================================================================
let localBrowserCurrentPath = '';

function openPathBrowser() {
  const cur = document.getElementById('inputPath').value.trim();
  document.getElementById('pathBrowserModal').classList.add('active');
  // 若已有路径，从该路径开始浏览；否则从盘符列表开始
  if (cur) {
    gotoLocalPath(cur);
  } else {
    loadLocalDir('');
  }
}

async function loadLocalDir(dirPath) {
  const tbody = document.getElementById('localFileBody');
  tbody.innerHTML = '<tr><td colspan="2" class="empty-state">加载中...</td></tr>';
  try {
    const params = new URLSearchParams();
    if (dirPath) params.set('path', dirPath);
    const data = await fetch('/api/local/ls?' + params.toString()).then(r => r.json());
    if (data.error) {
      tbody.innerHTML = `<tr><td colspan="2" class="empty-state">❌ ${data.error}</td></tr>`;
      return;
    }
    localBrowserCurrentPath = data.path || '';
    document.getElementById('localPathInput').value = localBrowserCurrentPath;
    document.getElementById('localCurrentPath').textContent = localBrowserCurrentPath || '(请选择盘符)';
    renderLocalBreadcrumb(localBrowserCurrentPath);
    renderLocalTable(data);
  } catch(e) {
    tbody.innerHTML = `<tr><td colspan="2" class="empty-state">❌ 读取失败: ${e.message}</td></tr>`;
  }
}

function renderLocalBreadcrumb(dirPath) {
  const el = document.getElementById('localBreadcrumb');
  if (!dirPath) { el.innerHTML = '<span>💻 计算机</span>'; return; }
  let html = '';
  const isWin = /^[A-Za-z]:[\\/]/.test(dirPath);
  if (isWin) {
    const drive = dirPath.substring(0, 3);
    html += `<a onclick="loadLocalDir('${escJs(drive)}')">💽 ${drive}</a>`;
    const rest = dirPath.substring(3).split(/[\\/]/).filter(Boolean);
    let cur = drive;
    rest.forEach((part, i) => {
      cur = cur.replace(/[\\/]$/, '') + '\\' + part;
      const isLast = i === rest.length - 1;
      html += '<span class="sep">\\</span>';
      html += isLast ? `<span>${escHtml(part)}</span>` : `<a onclick="loadLocalDir('${escJs(cur)}')">${escHtml(part)}</a>`;
    });
  } else {
    html = '<a onclick="loadLocalDir(\'/\')">/</a>';
    let cur = '';
    dirPath.split('/').filter(Boolean).forEach((part, i, arr) => {
      cur += '/' + part;
      const isLast = i === arr.length - 1;
      html += '<span class="sep">/</span>';
      html += isLast ? `<span>${escHtml(part)}</span>` : `<a onclick="loadLocalDir('${escJs(cur)}')">${escHtml(part)}</a>`;
    });
  }
  el.innerHTML = html;
}

function renderLocalTable(data) {
  const tbody = document.getElementById('localFileBody');
  const rows = [];
  // 盘符列表视图
  if (data.drives && data.drives.length > 0) {
    data.drives.forEach(d => {
      rows.push(`<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(d)}')">
        <td><span class="file-name">💽 ${escHtml(d)}</span></td>
        <td class="file-size">-</td>
      </tr>`);
    });
    tbody.innerHTML = rows.join('') || '<tr><td colspan="2" class="empty-state">未找到可用盘符</td></tr>';
    return;
  }
  // 返回上级（parent 为空且当前在盘符根时，返回盘符列表）
  if (data.parent === '' && data.path) {
    rows.push(`<tr style="cursor:pointer" onclick="loadLocalDir('')">
      <td><span class="file-name">📁 ..</span></td>
      <td class="file-size">-</td>
    </tr>`);
  } else if (data.parent && data.parent !== data.path) {
    rows.push(`<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(data.parent)}')">
      <td><span class="file-name">📁 ..</span></td>
      <td class="file-size">-</td>
    </tr>`);
  }
  const files = data.files || [];
  if (files.length === 0 && rows.length === 0) {
    tbody.innerHTML = '<tr><td colspan="2" class="empty-state">空目录</td></tr>';
    return;
  }
  files.forEach(f => {
    if (f.is_dir) {
      const child = joinLocalPath(data.path, f.name);
      rows.push(`<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(child)}')">
        <td><span class="file-name">📁 ${escHtml(f.name)}</span></td>
        <td class="file-size">-</td>
      </tr>`);
    } else {
      rows.push(`<tr>
        <td><span class="file-name" style="opacity:0.6">${getFileIcon(f.name)} ${escHtml(f.name)}</span></td>
        <td class="file-size">${formatFileSize(f.size)}</td>
      </tr>`);
    }
  });
  tbody.innerHTML = rows.join('');
}

function joinLocalPath(base, name) {
  if (!base) return name;
  if (base.endsWith('\\') || base.endsWith('/')) return base + name;
  const isWin = /^[A-Za-z]:[\\/]/.test(base);
  return base + (isWin ? '\\' : '/') + name;
}

function gotoLocalPath(path) {
  const p = (path !== undefined ? path : document.getElementById('localPathInput').value).trim();
  if (!p) { loadLocalDir(''); return; }
  loadLocalDir(p);
}

function chooseLocalPath() {
  if (!localBrowserCurrentPath) { log('❌ 请先进入一个目录', 'warn'); return; }
  document.getElementById('inputPath').value = localBrowserCurrentPath;
  document.getElementById('pathBrowserModal').classList.remove('active');
}

function escJs(s) { return String(s).replace(/\\/g, '\\\\').replace(/'/g, "\\'"); }
function escHtml(s) { return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }


// 删除项目
async function deleteProject(name) {
  if (!confirm(`确定删除项目「${name}」？\n此操作仅删除 CI/CD 配置，不会删除项目代码。`)) return;
  const idx = projects.findIndex(x => x.name === name);
  if (idx < 0) { log(`❌ 项目不存在: ${name}`, 'error'); return; }
  projects.splice(idx, 1);
  try {
    await apiPost('/api/project', {projects});
    log(`🗑️ 已删除项目: ${name}`, 'warn');
    renderProjects();
  } catch(e) {
    log(`❌ 删除失败: ${e.message}`, 'error');
    refreshProjects(); // 回滚
  }
}

function addRemoteRow(name, url, enabled) {
  const rl = document.getElementById('remoteList');
  const div = document.createElement('div'); div.className = 'remote-row';
  div.innerHTML = `<input class="remote-name" placeholder="名称 (github)" value="${name||''}"><input class="remote-url" placeholder="URL" value="${url||''}" style="flex:2"><label style="font-size:12px;color:var(--text-tertiary);white-space:nowrap"><input type="checkbox" class="remote-enabled" ${enabled !== false ? 'checked' : ''}> 启用</label><button class="btn-danger" style="padding:5px 10px;font-size:12px" onclick="this.parentElement.remove()">✕</button>`;
  rl.appendChild(div);
}

async function saveProject() {
  const remotes = [];
  document.querySelectorAll('#remoteList .remote-row').forEach(row => {
    const name = row.querySelector('.remote-name').value;
    const url = row.querySelector('.remote-url').value;
    if (name && url) remotes.push({ name, url, enabled: row.querySelector('.remote-enabled').checked });
  });
  const p = {
    name: document.getElementById('inputName').value,
    path: document.getElementById('inputPath').value,
    enabled: true,
    deployTarget: document.getElementById('inputTarget').value,
    deploy: {
      host: document.getElementById('inputHost').value,
      port: parseInt(document.getElementById('inputPort').value) || 22,
      user: document.getElementById('inputUser').value,
      auth_type: document.getElementById('inputAuthType').value,
      identity_file: document.getElementById('inputKeyPath').value,
      remote_dir: document.getElementById('inputRemote').value,
    },
    remotes: remotes
  };
  const rules = [];
  document.querySelectorAll('.rule-cb').forEach(cb => { rules.push({ id: cb.dataset.id, enabled: cb.checked }); });
  p.rules = rules;
  p.pipeline = getPipelineConfigFromUI();
  if (!p.name || !p.path) { log('❌ 请填写项目名称和路径', 'error'); return; }
  const idx = projects.findIndex(x => x.name === p.name);
  if (idx >= 0) projects[idx] = p;
  else projects.push(p);
  try {
    await apiPost('/api/project', {projects});
    log(`✅ 项目已保存: ${p.name}`, 'info');
  } catch(e) { log(`❌ 保存失败: ${e.message}`, 'error'); }
  closeModal();
  renderProjects();
}

// ===== 测试报告弹窗 =====
async function showReport(project, data) {
  const p = projects.find(x => x.name === project);
  if (!p) return;

  if (data && data.report) {
    if (data.report.total !== undefined) {
      data = { duration: data.duration, report: data.report };
    } else if (data.report.report) {
      data = { duration: data.report.duration, report: data.report.report };
    }
  }

  if (!data || !data.report) {
    data = await api(`/api/report/latest?project=${encodeURIComponent(project)}`);
    if (data && data.report && data.report.report) {
      data = { duration: data.report.duration, report: data.report.report };
    }
  }

  const rep = data?.report;
  if (!rep) { log(`📭 [${project}] 无测试报告`, 'warn'); return; }

  const total = rep.total || 0;
  const passed = rep.passed || 0;
  const failed = rep.failed || 0;
  const skipped = rep.skipped || 0;
  const coverage = rep.coverage || '-';
  const failures = rep.failures || [];

  // 同时获取历史列表
  const listData = await api(`/api/report/list?project=${encodeURIComponent(project)}`);
  const reports = listData?.reports || [];

  let html = `<div class="modal-content" style="width:680px">
    <h2>🧪 测试报告: ${project}</h2>
    <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:10px;margin-bottom:12px">
      <div class="stat-card"><div class="num" style="color:var(--accent-hover)">${total}</div><div class="label">总数</div></div>
      <div class="stat-card"><div class="num" style="color:var(--success)">${passed}</div><div class="label">通过</div></div>
      <div class="stat-card"><div class="num" style="color:var(--danger)">${failed}</div><div class="label">失败</div></div>
      <div class="stat-card"><div class="num" style="color:var(--warning)">${skipped}</div><div class="label">跳过</div></div>
    </div>
    <div style="margin-bottom:12px;padding:12px;background:var(--bg-elevated);border:1px solid var(--border-subtle);border-radius:var(--r-md)">
      <span style="color:var(--text-tertiary);font-size:12px;text-transform:uppercase;letter-spacing:0.04em;font-weight:600">覆盖率</span>
      <span style="color:var(--purple);font-size:20px;font-weight:600;margin-left:8px">${coverage}</span>
      <span style="color:var(--text-tertiary);font-size:12px;text-transform:uppercase;letter-spacing:0.04em;font-weight:600;margin-left:24px">耗时</span>
      <span style="color:var(--text-primary);font-size:16px;font-weight:500;margin-left:8px;font-family:var(--font-mono)">${data.duration || '-'}</span>
    </div>`;

  if (failures.length > 0) {
    html += `<div style="margin-bottom:12px">
      <h3 style="color:var(--danger);font-size:13px;margin-bottom:8px;font-weight:600">❌ 失败详情 (${failures.length} 个)</h3>`;
    failures.forEach(f => {
      html += `<div style="background:var(--bg-elevated);border:1px solid var(--border-subtle);border-left:2px solid var(--danger);border-radius:var(--r-sm);padding:12px;margin-bottom:6px">
        <div style="color:var(--danger);font-size:13px;font-weight:600">[${f.suite}] ${f.test}</div>
        <div style="color:var(--text-tertiary);font-size:12px;margin-top:6px;font-family:var(--font-mono);white-space:pre-wrap">${f.message}</div>
      </div>`;
    });
    html += `</div>`;
  }

  // 历史报告列表
  if (reports.length > 0) {
    html += `<div style="margin-top:14px;border-top:1px solid var(--border-subtle);padding-top:12px">
      <h3 style="color:var(--text-tertiary);font-size:12px;margin-bottom:8px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em">📋 历史报告 (${reports.length})</h3>
      <div style="max-height:200px;overflow-y:auto">`;
    reports.forEach(r => {
      const st = r.status === 'pass' ? '✅' : '❌';
      const info = r.total ? `${r.passed}/${r.total} 通过` : '-';
      html += `<div style="display:flex;align-items:center;gap:8px;padding:7px 10px;border-bottom:1px solid var(--border-subtle);font-size:12px">
        <span>${st}</span>
        <span style="color:var(--text-tertiary);flex:1;font-family:var(--font-mono)">${r.timestamp || r.id}</span>
        <span style="color:var(--text-secondary);width:90px">${info}</span>
        <button class="action-btn btn-outline" style="font-size:10px" onclick="showReportById('${project}','${r.id}')">查看</button>
        <button class="action-btn btn-danger" style="font-size:10px" onclick="deleteReport('${project}','${r.id}')">删除</button>
      </div>`;
    });
    html += `</div></div>`;
  }

  html += `<div style="margin-top:14px;text-align:right">
    <button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="padding:8px 20px">关闭</button>
  </div></div>`;

  const modal = document.getElementById('reportModal');
  modal.innerHTML = html;
  modal.classList.add('active');
}

async function showReportById(project, id) {
  const data = await api(`/api/report/latest?project=${encodeURIComponent(project)}`);
  const listData = await api(`/api/report/list?project=${encodeURIComponent(project)}`);
  const reports = listData?.reports || [];
  const reportInfo = reports.find(r => r.id === id);
  if (!reportInfo) { log(`📭 未找到该报告`, 'warn'); return; }
  // 从磁盘读取指定报告
  const r = await fetch(`/api/report/latest?project=${encodeURIComponent(project)}&id=${id}`);
  // 回退：直接显示所有报告，让用户看到列表
  showReport(project);
}

async function deleteReport(project, id) {
  if (!confirm(`确定删除 ${id} 的测试报告？`)) return;
  const data = await apiPost('/api/report/delete', {project, id});
  if (data && data.status === 'ok') {
    log(`🗑️ 已删除报告: ${id}`, 'info');
    showReport(project);
  } else {
    log(`❌ 删除失败: ${data?.error || '未知错误'}`, 'error');
  }
}

// ========================================================================
// 远程管理 - 视图切换
// ========================================================================
let currentView = 'cicd';
function switchView(view) {
  currentView = view;
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  if (view === 'cicd') {
    document.querySelectorAll('.tab-btn')[0].classList.add('active');
    document.getElementById('mainContainer').style.display = 'block';
    document.getElementById('remoteView').style.display = 'none';
  } else {
    document.querySelectorAll('.tab-btn')[1].classList.add('active');
    document.getElementById('remoteView').style.display = 'block';
    document.getElementById('mainContainer').style.display = 'none';
    loadRemoteProjects();
  }
}
// 默认显示 cicd 视图
document.getElementById('remoteView').style.display = 'none';

// 窗口大小变化时自动适配当前激活标签页的终端
window.addEventListener('resize', () => {
  const s = getSession();
  if (s && s.fitAddon && s.term && s.connected) {
    try { s.fitAddon.fit(); } catch(e) {}
  }
});

// ========================================================================
// 远程管理 - 多标签页架构（支持同时连接多台服务器）
// ========================================================================
// 每个标签页维护独立的会话：终端实例、WebSocket、文件列表、路径等
const remoteSessions = {}; // key: tabId, value: session object
let activeTabId = null;    // 当前激活的标签页
let sessionTimeout = 10 * 60 * 1000; // 默认 10 分钟

function createSession(tabId, name, source) {
  return {
    tabId,
    name,
    source,
    term: null,
    fitAddon: null,
    ws: null,
    connected: false,
    connecting: false,
    currentPath: '/',
    remoteFiles: [],
    selectedFile: '',
    lastActivity: Date.now(),
    sessionTimer: null,
    termContainer: null,  // 每个会话独立的终端 DOM 容器（隐藏的）
  };
}

function getSession() {
  return activeTabId ? remoteSessions[activeTabId] : null;
}

// ===== 会话超时管理 =====
function updateActivity() {
  const s = getSession();
  if (s) s.lastActivity = Date.now();
}

function startSessionTimer(tabId) {
  const s = remoteSessions[tabId];
  if (!s) return;
  if (s.sessionTimer) clearInterval(s.sessionTimer);
  s.lastActivity = Date.now();
  s.sessionTimer = setInterval(() => {
    if (!s.connected) return;
    const elapsed = Date.now() - s.lastActivity;
    if (elapsed > sessionTimeout) {
      log(`⏰ [${s.name}] 会话超时（${Math.round(sessionTimeout/60000)} 分钟无操作），自动断开`, 'warn');
      forceDisconnectTab(tabId);
    }
  }, 10000);
}

function stopSessionTimer(tabId) {
  const s = remoteSessions[tabId];
  if (s && s.sessionTimer) {
    clearInterval(s.sessionTimer);
    s.sessionTimer = null;
  }
}

// ===== 兼容旧代码的全局访问器 =====
function getRemoteProjectName() { const s = getSession(); return s ? s.name : ''; }
function getRemoteSource() { const s = getSession(); return s ? s.source : ''; }
function getRemoteConnected() { const s = getSession(); return s ? s.connected : false; }

// ===== 标签页管理 =====
function renderRemoteTabs() {
  const container = document.getElementById('remoteTabs');
  const tabIds = Object.keys(remoteSessions);
  if (tabIds.length === 0) {
    container.innerHTML = '<div class="remote-tab-placeholder">选择服务器并点击「连接」可同时管理多台服务器</div>';
    return;
  }
  container.innerHTML = '';
  tabIds.forEach(tabId => {
    const s = remoteSessions[tabId];
    const tab = document.createElement('div');
    tab.className = 'remote-tab' + (tabId === activeTabId ? ' active' : '');
    const statusClass = s.connected ? 'connected' : (s.connecting ? 'connecting' : 'disconnected');
    tab.innerHTML = `
      <span class="tab-status-dot ${statusClass}"></span>
      <span onclick="switchToTab('${tabId}')">${s.name}</span>
      <span class="tab-close" onclick="event.stopPropagation();closeTab('${tabId}')" title="关闭标签页">✕</span>
    `;
    tab.onclick = () => switchToTab(tabId);
    container.appendChild(tab);
  });
}

function switchToTab(tabId) {
  const s = remoteSessions[tabId];
  if (!s) return;
  // 切换前不需要保存终端状态（终端 DOM 已经在会话容器中）
  activeTabId = tabId;
  renderRemoteTabs();
  // 恢复文件列表显示
  renderFileBreadcrumb(s.currentPath);
  renderFileTable(s.remoteFiles);
  // 恢复终端：将此会话的终端容器显示出来
  showTerminalForTab(tabId);
  // 恢复下载按钮状态
  document.getElementById('downloadBtn').disabled = !s.selectedFile;
}

function closeTab(tabId) {
  const s = remoteSessions[tabId];
  if (!s) return;
  // 如果已连接，先断开
  if (s.ws) {
    try { s.ws.close(); } catch(e) {}
  }
  stopSessionTimer(tabId);
  // 通知后端清除连接
  if (s.name) {
    fetch(`/api/remote/disconnect?project=${encodeURIComponent(s.name)}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} }).catch(() => {});
  }
  // 销毁终端
  if (s.term) { try { s.term.dispose(); } catch(e) {} }
  delete remoteSessions[tabId];
  // 如果关闭的是当前激活的标签页，切换到其他标签
  if (activeTabId === tabId) {
    const remaining = Object.keys(remoteSessions);
    if (remaining.length > 0) {
      switchToTab(remaining[0]);
    } else {
      activeTabId = null;
      // 恢复占位内容
      document.getElementById('terminal-container').innerHTML = '<div style="padding:40px 20px;color:var(--text-tertiary);font-size:14px;text-align:center;line-height:2"><div style="font-size:32px;margin-bottom:12px;opacity:0.4">🖥️</div>选择服务器后点击「连接」<br><span style="font-size:12px;color:var(--text-quaternary)">支持同时连接多台服务器，通过标签页切换</span></div>';
      document.getElementById('fileBreadcrumb').innerHTML = '<span style="color:var(--text-tertiary)">选择服务器 → 连接</span>';
      document.getElementById('fileBody').innerHTML = '<tr><td colspan="2" class="empty-state">未连接</td></tr>';
      document.getElementById('downloadBtn').disabled = true;
    }
  }
  renderRemoteTabs();
  log(`📋 已关闭标签页: ${s.name}`, 'info');
}

function forceDisconnectTab(tabId) {
  const s = remoteSessions[tabId];
  if (!s) return;
  if (s.ws) {
    try { s.ws.close(); } catch(e) {}
    s.ws = null;
  }
  s.connected = false;
  s.connecting = false;
  stopSessionTimer(tabId);
  if (s.name) {
    fetch(`/api/remote/disconnect?project=${encodeURIComponent(s.name)}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} }).catch(() => {});
  }
  if (s.term) {
    s.term.write('\r\n=== 连接已断开 ===\r\n');
  }
  // 更新文件列表
  if (tabId === activeTabId) {
    document.getElementById('fileBody').innerHTML = '<tr><td colspan="2" class="empty-state">已断开连接</td></tr>';
    document.getElementById('fileBreadcrumb').innerHTML = '<span style="color:var(--text-tertiary)">已断开</span>';
    document.getElementById('downloadBtn').disabled = true;
  }
  renderRemoteTabs();
}

// 显示指定标签页的终端
function showTerminalForTab(tabId) {
  const s = remoteSessions[tabId];
  if (!s || !s.term) return;
  const mainContainer = document.getElementById('terminal-container');
  // 清空主容器，将此会话的终端元素移入
  // xterm 的 DOM 在 term.element 上，我们需要把它重新 attach
  mainContainer.innerHTML = '';
  mainContainer.style.display = 'block';
  mainContainer.style.minHeight = '420px';
  // 重新将终端元素附加到主容器
  if (s.term.element) {
    mainContainer.appendChild(s.term.element);
  }
  s.term.focus();
  setTimeout(() => { try { s.fitAddon.fit(); } catch(e){} }, 50);
  setTimeout(() => { try { s.fitAddon.fit(); } catch(e){} }, 200);
}

async function loadRemoteProjects() {
  try {
    const data = await api('/api/remote/projects');
    const sel = document.getElementById('remoteProjectSelect');
    sel.innerHTML = '<option value="">-- 选择服务器 --</option>';
    if (data && data.servers) {
      data.servers.forEach(s => {
        const opt = document.createElement('option');
        opt.value = s.ref;
        opt.dataset.source = s.source;
        const deploy = s.deploy || {};
        opt.textContent = `${s.name} (${deploy.user || '?'}@${deploy.host || '?'})`;
        sel.appendChild(opt);
      });
    }
    // 更新删除按钮状态
    updateDeleteBtnState();
  } catch(e) { log(`❌ 加载远程项目失败: ${e.message}`, 'error'); }
}

function onRemoteProjectChange() {
  updateDeleteBtnState();
}

function updateDeleteBtnState() {
  const sel = document.getElementById('remoteProjectSelect');
  const opt = sel.options[sel.selectedIndex];
  const btn = document.getElementById('deleteServerBtn');
  if (opt && opt.dataset && opt.dataset.source === 'standalone') {
    btn.style.display = 'inline-flex';
  } else {
    btn.style.display = 'none';
  }
}

// ===== 独立服务器 CRUD =====
function showAddServerDialog() {
  document.getElementById('svrName').value = '';
  document.getElementById('svrHost').value = '';
  document.getElementById('svrPort').value = '22';
  document.getElementById('svrUser').value = '';
  document.getElementById('svrAuthType').value = 'key';
  document.getElementById('svrKeyPath').value = '';
  document.getElementById('svrPassword').value = '';
  document.getElementById('svrNote').value = '';
  document.getElementById('addServerMsg').textContent = '';
  toggleSvrKeyPath();
  document.getElementById('addServerModal').classList.add('active');
}

function closeAddServerDialog() {
  document.getElementById('addServerModal').classList.remove('active');
}

function toggleSvrKeyPath() {
  const v = document.getElementById('svrAuthType').value;
  document.getElementById('svrKeyPathGroup').style.display = v === 'key' ? 'block' : 'none';
  document.getElementById('svrPassGroup').style.display = v === 'password' ? 'block' : 'none';
}

async function doAddServer() {
  const msgEl = document.getElementById('addServerMsg');
  const svr = {
    name: document.getElementById('svrName').value.trim(),
    host: document.getElementById('svrHost').value.trim(),
    port: parseInt(document.getElementById('svrPort').value) || 22,
    user: document.getElementById('svrUser').value.trim(),
    auth_type: document.getElementById('svrAuthType').value,
    identity_file: document.getElementById('svrKeyPath').value.trim(),
    password: document.getElementById('svrPassword').value,
    note: document.getElementById('svrNote').value.trim(),
  };
  if (!svr.name || !svr.host || !svr.user) {
    msgEl.innerHTML = '<span style="color:var(--danger)">名称、主机、用户名不能为空</span>';
    return;
  }
  try {
    const resp = await fetch('/api/remote/servers', {
      method: 'POST',
      headers: {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'},
      body: JSON.stringify(svr),
    });
    const data = await resp.json();
    if (data.status === 'ok') {
      msgEl.innerHTML = '<span style="color:var(--success)">✅ 服务器已添加</span>';
      log(`🖥️ 已添加服务器: ${svr.name} (${svr.user}@${svr.host})`, 'info');
      setTimeout(() => { closeAddServerDialog(); loadRemoteProjects(); }, 800);
    } else {
      msgEl.innerHTML = `<span style="color:var(--danger)">❌ ${data.error || '添加失败'}</span>`;
    }
  } catch(e) {
    msgEl.innerHTML = `<span style="color:var(--danger)">❌ ${e.message}</span>`;
  }
}

async function deleteSelectedServer() {
  const sel = document.getElementById('remoteProjectSelect');
  const name = sel.value;
  if (!name) return;
  if (!confirm(`确定删除服务器「${name}」？`)) return;
  try {
    const params = new URLSearchParams({ name });
    const resp = await fetch(`/api/remote/server?${params}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} });
    const data = await resp.json();
    if (data.status === 'ok') {
      log(`🗑️ 已删除服务器: ${name}`, 'info');
      loadRemoteProjects();
    } else {
      log(`❌ 删除失败: ${data.error}`, 'error');
    }
  } catch(e) {
    log(`❌ 删除失败: ${e.message}`, 'error');
  }
}

function setRemoteStatus(text, cls) {
  // 多标签模式下状态显示在标签上，这里保留兼容
}

function connectRemote() {
  const sel = document.getElementById('remoteProjectSelect');
  const opt = sel.options[sel.selectedIndex];
  const name = sel.value;
  if (!name) { log('❌ 请先选择服务器', 'error'); return; }

  const source = opt && opt.dataset ? (opt.dataset.source || 'project') : 'project';
  const tabId = name + '|' + source;

  // 如果该服务器已经有标签页，切换过去
  if (remoteSessions[tabId]) {
    log(`ℹ️ 服务器 ${name} 已有标签页，切换过去`, 'info');
    switchToTab(tabId);
    // 如果未连接，重新连接
    if (!remoteSessions[tabId].connected && !remoteSessions[tabId].connecting) {
      doConnect(tabId);
    }
    return;
  }

  // 创建新会话
  remoteSessions[tabId] = createSession(tabId, name, source);
  activeTabId = tabId;
  renderRemoteTabs();
  doConnect(tabId);
}

async function doConnect(tabId) {
  const s = remoteSessions[tabId];
  if (!s) return;
  s.connecting = true;
  renderRemoteTabs();
  log(`🔌 正在连接 ${s.name}...`, 'info');

  // 初始化终端（如果还没有）
  if (!s.term) {
    const termContainer = document.getElementById('terminal-container');
    // 如果当前显示的是其他标签的终端，先切换
    if (activeTabId !== tabId) {
      activeTabId = tabId;
    }
    termContainer.innerHTML = '';
    termContainer.style.display = 'block';
    termContainer.style.minHeight = '420px';

    try {
      if (typeof Terminal === 'undefined') {
        throw new Error('xterm.js 未加载，请检查网络或刷新页面');
      }
      const isDark = document.body.classList.contains('dark');
      const theme = isDark ? {
        background: '#0f1118', foreground: '#e2e8f0', cursor: '#0ea5e9',
        selectionBackground: 'rgba(14,165,233,0.3)',
      } : {
        background: '#0f172a', foreground: '#e2e8f0', cursor: '#38bdf8',
        selectionBackground: 'rgba(56,189,248,0.3)',
      };
      // 通用 ANSI 16 色
      const ansiColors = {
        black: '#1e293b', red: '#f43f5e', green: '#10b981', yellow: '#f59e0b',
        blue: '#3b82f6', magenta: '#8b5cf6', cyan: '#06b6d4', white: '#e2e8f0',
        brightBlack: '#64748b', brightRed: '#fb7185', brightGreen: '#34d399',
        brightYellow: '#fbbf24', brightBlue: '#60a5fa', brightMagenta: '#a78bfa',
        brightCyan: '#22d3ee', brightWhite: '#f8fafc',
      };
      s.term = new Terminal({
        cursorBlink: true, cursorStyle: 'block',
        fontSize: 14,
        fontFamily: "'JetBrains Mono','Cascadia Code','Fira Code',ui-monospace,monospace",
        theme: { ...theme, ...ansiColors },
        allowTransparency: false,
        scrollback: 5000, convertEol: false, rows: 30, cols: 120,
      });
      s.fitAddon = new FitAddon.FitAddon();
      s.term.loadAddon(s.fitAddon);
      s.term.open(termContainer);
      s.term.focus();
      setTimeout(() => { try { s.fitAddon.fit(); } catch(e){} }, 50);
      setTimeout(() => { try { s.fitAddon.fit(); } catch(e){} }, 200);
    } catch(e) {
      console.error('终端初始化失败:', e);
      log(`⚠️ 终端初始化失败: ${e.message}`, 'warn');
      s.term = null;
      termContainer.innerHTML = `<div style="padding:8px;background:var(--bg-elevated);border-bottom:1px solid var(--border-subtle);font-size:11px;color:var(--text-tertiary)">⚠️ xterm.js 不可用，使用简易终端</div><textarea id="fallbackTerminal_${tabId}" style="width:100%;height:calc(100% - 32px);background:var(--bg-input);color:var(--success);border:none;padding:8px;font-family:var(--font-mono);font-size:13px;resize:none;outline:none;line-height:1.5" spellcheck="false" autofocus placeholder="连接成功后在此输入命令..."></textarea>`;
      const ta = document.getElementById(`fallbackTerminal_${tabId}`);
      if (ta) ta.addEventListener('keydown', (event) => fallbackHandleKeyForSession(s, event));
    }
  } else {
    // 已有终端，切换到该标签
    activeTabId = tabId;
    showTerminalForTab(tabId);
    s.term.reset();
    s.term.clear();
  }

  // 建立 WebSocket 连接
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/api/remote/term?project=${encodeURIComponent(s.name)}&source=${encodeURIComponent(s.source)}`;
  s.ws = new WebSocket(url);

  s.ws.onopen = () => {
    s.connected = true;
    s.connecting = false;
    renderRemoteTabs();
    if (s.term) { s.term.focus(); setTimeout(() => { try { s.fitAddon.fit(); } catch(e){} }, 100); }
    writeToTerminalForSession(s, '\r\n=== SSH 终端已连接 ===\r\n');
    startSessionTimer(tabId);
    // 加载文件列表
    if (tabId === activeTabId) {
      setTimeout(() => refreshFileList(), 800);
    }
  };

  s.ws.onmessage = (evt) => {
    s.lastActivity = Date.now();
    if (evt.data instanceof Blob) {
      evt.data.arrayBuffer().then(buf => writeToTerminalBytesForSession(s, new Uint8Array(buf)));
    } else {
      try {
        const msg = JSON.parse(evt.data);
        if (msg.type === 'connected') {
          writeToTerminalForSession(s, msg.message + '\r\n');
        } else if (msg.error) {
          writeToTerminalForSession(s, '错误: ' + msg.error + '\r\n');
        }
      } catch(e) {
        writeToTerminalForSession(s, evt.data);
      }
    }
  };

  s.ws.onerror = () => {
    s.connecting = false;
    renderRemoteTabs();
    writeToTerminalForSession(s, 'WebSocket 连接错误\r\n');
    log(`❌ [${s.name}] WebSocket 连接失败`, 'error');
  };

  s.ws.onclose = () => {
    if (s.connected) {
      writeToTerminalForSession(s, '=== SSH 连接已断开 ===\r\n');
      log(`🔌 [${s.name}] SSH 连接已断开`, 'warn');
    }
    s.connected = false;
    s.connecting = false;
    s.ws = null;
    stopSessionTimer(tabId);
    renderRemoteTabs();
  };

  // 终端输入 → WebSocket
  if (s.term) {
    s.term.onData(data => {
      s.lastActivity = Date.now();
      if (s.ws && s.ws.readyState === WebSocket.OPEN) {
        s.ws.send(data);
      }
    });
    s.term.onResize(size => {
      s.lastActivity = Date.now();
      if (s.ws && s.ws.readyState === WebSocket.OPEN) {
        s.ws.send(`resize${size.cols}x${size.rows}`);
      }
    });
  }
}

// ===== 按会话的终端写入 =====
function writeToTerminalForSession(s, text) {
  if (s && s.term && s.term.write) {
    s.term.write(text);
  }
}
function writeToTerminalBytesForSession(s, data) {
  if (s && s.term && s.term.write) { s.term.write(data); return; }
  writeToTerminalForSession(s, new TextDecoder('utf-8').decode(data));
}

// 旧接口兼容：写入当前激活会话的终端
function writeToTerminal(text) {
  const s = getSession();
  if (s) writeToTerminalForSession(s, text);
}
function writeToTerminalBytes(data) {
  const s = getSession();
  if (s) writeToTerminalBytesForSession(s, data);
}

// fallback textarea 键盘处理（按会话）
function fallbackHandleKeyForSession(s, event) {
  if (event.ctrlKey && event.key === 'c') { event.preventDefault(); fallbackSendForSession(s, '\x03'); return; }
  if (event.ctrlKey && event.key === 'd') { event.preventDefault(); fallbackSendForSession(s, '\x04'); return; }
  if (event.key === 'Enter') { event.preventDefault(); fallbackSendForSession(s, '\r'); }
  else if (event.key === 'Backspace') { event.preventDefault(); fallbackSendForSession(s, '\x7f'); }
  else if (event.key === 'Tab') { event.preventDefault(); fallbackSendForSession(s, '\t'); }
  else if (event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey) { event.preventDefault(); fallbackSendForSession(s, event.key); }
}
function fallbackSendForSession(s, char) {
  if (s && s.ws && s.ws.readyState === WebSocket.OPEN) {
    s.ws.send(char);
    s.lastActivity = Date.now();
  }
}

// ===== 统一终端写入（兼容 xterm 和 textarea 降级） =====
// 剥离所有 ANSI 转义序列：CSI (\x1b[...) + OSC (\x1b]...) + 私有序列 (\x1b[?...)
function stripAnsi(str) {
  return str
    .replace(/\x1b\[[\d;?]*[a-zA-Z]/g, '')   // CSI: \x1b[<params><letter>
    .replace(/\x1b\][^\x1b]*(\x1b\\|\x07)/g, '') // OSC: \x1b]<string>(\x1b\ | \a)
    .replace(/\x1b\][^\x07]*\x07/g, '')       // OSC 变体: \x1b]<string>\a
    .replace(/\x1b\[[\d;]*[HfABCDGJKlLPS]/g, '') // 光标移动/清屏等
    .replace(/\x1b[PX^_].*?(\x1b\\|\x07)/g, '')  // 其他 APC/SOS/PM
    .replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, ''); // 控制字符
}
function writeToTerminal(text) {
  if (term && term.write) {
    term.write(text);
  } else {
    const ta = document.getElementById('fallbackTerminal');
    if (ta) {
      ta.value += stripAnsi(text);
      ta.scrollTop = ta.scrollHeight;
    }
  }
}
function writeToTerminalBytes(data) {
  if (term && term.write) { term.write(data); return; }
  writeToTerminal(new TextDecoder('utf-8').decode(data));
}

// fallback textarea 键盘输入发送到 WebSocket
function fallbackSendInput(char) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(char);
    updateActivity();
  }
}
// fallback textarea 特殊按键处理
function fallbackHandleKey(event) {
  // Ctrl+C 发送中断信号
  if (event.ctrlKey && event.key === 'c') {
    event.preventDefault();
    fallbackSendInput('\x03');
    return;
  }
  // Ctrl+D 发送 EOF
  if (event.ctrlKey && event.key === 'd') {
    event.preventDefault();
    fallbackSendInput('\x04');
    return;
  }
  if (event.key === 'Enter') {
    event.preventDefault();
    fallbackSendInput('\r');
  } else if (event.key === 'Backspace') {
    event.preventDefault();
    fallbackSendInput('\x7f');
  } else if (event.key === 'Tab') {
    event.preventDefault();
    fallbackSendInput('\t');
  } else if (event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey) {
    event.preventDefault();
    fallbackSendInput(event.key);
  }
}

// 断开当前标签页
function disconnectCurrentTab() {
  if (!activeTabId) { log('❌ 没有活动的标签页', 'warn'); return; }
  const s = remoteSessions[activeTabId];
  if (!s) return;
  forceDisconnectTab(activeTabId);
  log(`🔌 [${s.name}] 已断开连接`, 'info');
}

// 断开所有标签页
function disconnectAllTabs() {
  const tabIds = Object.keys(remoteSessions);
  if (tabIds.length === 0) { log('ℹ️ 没有连接的服务器', 'info'); return; }
  if (!confirm(`确定断开所有 ${tabIds.length} 个服务器连接？`)) return;
  tabIds.forEach(tabId => forceDisconnectTab(tabId));
  log(`🔌 已断开全部 ${tabIds.length} 个服务器连接`, 'warn');
}

// 旧接口兼容
function disconnectRemote() {
  disconnectCurrentTab();
}

// ========================================================================
// 远程管理 - 文件浏览
// ========================================================================
// 兼容访问器：从当前会话读取状态
function currentRemotePath() { const s = getSession(); return s ? s.currentPath : '/'; }
function remoteFiles() { const s = getSession(); return s ? s.remoteFiles : []; }
function selectedFileName() { const s = getSession(); return s ? s.selectedFile : ''; }
function remoteProjectName() { return getRemoteProjectName(); }
function remoteSource() { return getRemoteSource(); }
function remoteConnected() { return getRemoteConnected(); }

async function refreshFileList() {
  updateActivity();
  const s = getSession();
  if (!s) {
    document.getElementById('fileBody').innerHTML = '<tr><td colspan="2" class="empty-state">请先选择服务器并连接</td></tr>';
    return;
  }
  await loadFileList(s.currentPath);
}

async function loadFileList(dirPath) {
  const s = getSession();
  if (!s) return;
  if (!s.connected && !dirPath.startsWith('/__local')) {
    log('❌ 连接已断开，请先点击「连接」', 'warn');
    return;
  }
  s.lastActivity = Date.now();
  try {
    const params = new URLSearchParams({ project: s.name, path: dirPath, source: s.source });
    const data = await fetch(`/api/remote/ls?${params}`).then(r => r.json());
    if (data.error) {
      log(`❌ ${data.error}`, 'error');
      return;
    }
    s.currentPath = data.path;
    s.remoteFiles = data.files || [];
    renderFileBreadcrumb(data.path);
    renderFileTable(data.files);
  } catch(e) {
    log(`❌ 读取文件列表失败: ${e.message}`, 'error');
  }
}

function renderFileBreadcrumb(dirPath) {
  const el = document.getElementById('fileBreadcrumb');
  if (!dirPath || dirPath === '/') {
    el.innerHTML = '<a onclick="loadFileList(\'/\')">/</a>';
    return;
  }
  const parts = dirPath.split('/').filter(Boolean);
  let html = '<a onclick="loadFileList(\'/\')">/</a>';
  let cur = '';
  parts.forEach((part, i) => {
    cur += '/' + part;
    const isLast = i === parts.length - 1;
    html += '<span class="sep">/</span>';
    if (isLast) {
      html += `<span>${part}</span>`;
    } else {
      html += `<a onclick="loadFileList('${cur}')">${part}</a>`;
    }
  });
  el.innerHTML = html;
}

function renderFileTable(files) {
  const tbody = document.getElementById('fileBody');
  const s = getSession();
  const selFile = s ? s.selectedFile : '';
  if (!files || files.length === 0) {
    tbody.innerHTML = '<tr><td colspan="2" class="empty-state">空目录</td></tr>';
    return;
  }
  const curPath = s ? s.currentPath : '/';
  tbody.innerHTML = files.map(f => {
    const icon = f.is_dir ? '📁' : getFileIcon(f.name);
    const sizeStr = f.is_dir ? '-' : formatFileSize(f.size);
    const isSelected = selFile === f.name;
    // 目录：整行可点击，双击进入；文件：单击选中
    if (f.is_dir) {
      const childPath = curPath === '/' ? '/' + f.name : curPath + '/' + f.name;
      const safePath = childPath.replace(/'/g, "\\'");
      return `<tr style="cursor:pointer" ondblclick="loadFileList('${safePath}')" onclick="loadFileList('${safePath}')">
        <td><span class="file-name">${icon} ${f.name}</span></td>
        <td class="file-size">${sizeStr}</td>
      </tr>`;
    }
    const safeName = f.name.replace(/'/g, "\\'");
    return `<tr class="${isSelected ? 'selected-row' : ''}" style="cursor:default" onclick="selectFile('${safeName}')">
      <td><span class="file-name" style="${isSelected ? 'color:var(--accent);font-weight:600' : ''}">${icon} ${f.name}</span></td>
      <td class="file-size">${sizeStr}</td>
    </tr>`;
  }).join('');
  document.getElementById('downloadBtn').disabled = !selFile;
}

function selectFile(name) {
  const s = getSession();
  if (!s) return;
  s.selectedFile = s.selectedFile === name ? '' : name;
  renderFileTable(s.remoteFiles);
}

function downloadSelected() {
  const s = getSession();
  if (!s || !s.selectedFile) { log('❌ 请先点击选中一个文件', 'warn'); return; }
  downloadFile(s.selectedFile);
  s.selectedFile = '';
  renderFileTable(s.remoteFiles);
}

function getFileIcon(name) {
  const ext = name.split('.').pop().toLowerCase();
  const icons = {
    txt: '📄', md: '📝', json: '📋', xml: '📋', yml: '📋', yaml: '📋',
    js: '📜', ts: '📜', jsx: '📜', tsx: '📜', vue: '📜',
    py: '🐍', go: '🔷', rs: '🦀', java: '☕', kt: '📘',
    html: '🌐', css: '🎨', scss: '🎨', less: '🎨',
    jpg: '🖼️', jpeg: '🖼️', png: '🖼️', gif: '🖼️', svg: '🖼️', ico: '🖼️',
    zip: '📦', gz: '📦', tar: '📦', rar: '📦', '7z': '📦',
    sh: '⚡', bat: '⚡', ps1: '⚡',
    log: '📋', conf: '⚙️', cfg: '⚙️', ini: '⚙️',
    pdf: '📕', doc: '📕', docx: '📕', xls: '📊', xlsx: '📊',
  };
  return icons[ext] || '📄';
}

function formatFileSize(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function goToParentDir() {
  const s = getSession();
  if (!s || !s.currentPath || s.currentPath === '/') return;
  const parent = s.currentPath.substring(0, s.currentPath.lastIndexOf('/'));
  loadFileList(parent || '/');
}

// ========================================================================
// 远程管理 - 文件上传/下载/删除/创建目录
// ========================================================================

function uploadFile() {
  const s = getSession();
  if (!s) { log('❌ 请先选择服务器', 'error'); return; }
  document.getElementById('fileUploadInput').click();
}

async function doUpload() {
  const s = getSession();
  if (!s) return;
  s.lastActivity = Date.now();
  const input = document.getElementById('fileUploadInput');
  if (!input.files || input.files.length === 0) return;
  const file = input.files[0];
  const progress = document.getElementById('uploadProgress');
  const status = document.getElementById('uploadStatus');
  const bar = document.getElementById('uploadBarFill');
  progress.classList.add('active');

  status.textContent = `📤 上传 ${file.name} (${formatFileSize(file.size)})...`;
  bar.style.width = '0%';

  const formData = new FormData();
  formData.append('file', file);

  try {
    const params = new URLSearchParams({ project: s.name, path: s.currentPath, source: s.source });
    const resp = await fetch(`/api/remote/upload?${params}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'}, body: formData });
    const data = await resp.json();
    bar.style.width = '100%';
    if (data.status === 'ok') {
      status.textContent = `✅ 上传完成: ${data.filename} (${formatFileSize(data.size)})`;
      log(`📤 [${s.name}] 上传 ${data.filename} 完成`, 'info');
      setTimeout(() => progress.classList.remove('active'), 2000);
      loadFileList(s.currentPath);
    } else {
      status.textContent = `❌ 上传失败: ${data.error}`;
      log(`❌ 上传失败: ${data.error}`, 'error');
    }
  } catch(e) {
    status.textContent = `❌ 上传失败: ${e.message}`;
    log(`❌ 上传失败: ${e.message}`, 'error');
  }
  input.value = '';
}

async function downloadFile(name) {
  const s = getSession();
  if (!s) return;
  s.lastActivity = Date.now();
  const filePath = s.currentPath === '/' ? '/' + name : s.currentPath + '/' + name;
  const params = new URLSearchParams({ project: s.name, path: filePath, source: s.source });
  log(`📥 [${s.name}] 下载: ${filePath}`, 'info');

  try {
    // 1. 先获取一次性下载 token（此请求带 Basic Auth 认证）
    const tokenResp = await fetch('/api/remote/download-token', { credentials: 'same-origin' });
    if (!tokenResp.ok) {
      log(`❌ 获取下载凭证失败 (${tokenResp.status})`, 'error');
      return;
    }
    const tokenData = await tokenResp.json();
    const token = tokenData.token;

    // 2. 用带 token 的 URL 通过 <a download> 触发浏览器原生下载
    //    token 让请求绕过 Basic Auth，浏览器原生处理 Content-Disposition
    const downloadUrl = `/api/remote/download?${params}&download_token=${encodeURIComponent(token)}`;
    const a = document.createElement('a');
    a.href = downloadUrl;
    a.download = name;
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    log(`✅ 下载已触发: ${name}`, 'info');
  } catch(e) {
    log(`❌ 下载异常: ${e.message}`, 'error');
  }
}

async function deleteRemoteFile(name) {
  const s = getSession();
  if (!s) return;
  s.lastActivity = Date.now();
  if (!confirm(`确定删除 ${name}？`)) return;
  const filePath = s.currentPath === '/' ? '/' + name : s.currentPath + '/' + name;
  const params = new URLSearchParams({ project: s.name, path: filePath, source: s.source });
  try {
    const resp = await fetch(`/api/remote/delete?${params}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} });
    const data = await resp.json();
    if (data.status === 'ok') {
      log(`🗑️ [${s.name}] 已删除: ${filePath}`, 'info');
      loadFileList(s.currentPath);
    } else {
      log(`❌ 删除失败: ${data.error}`, 'error');
    }
  } catch(e) {
    log(`❌ 删除失败: ${e.message}`, 'error');
  }
}

function showMkdirDialog() {
  const s = getSession();
  if (!s) { log('❌ 请先选择服务器', 'error'); return; }
  document.getElementById('inputMkdir').value = '';
  document.getElementById('mkdirMsg').textContent = '';
  document.getElementById('mkdirModal').classList.add('active');
}

function closeMkdirDialog() {
  document.getElementById('mkdirModal').classList.remove('active');
}

async function doMkdir() {
  const s = getSession();
  if (!s) return;
  s.lastActivity = Date.now();
  const name = document.getElementById('inputMkdir').value.trim();
  const msgEl = document.getElementById('mkdirMsg');
  if (!name) { msgEl.innerHTML = '<span style="color:var(--danger)">请输入目录名称</span>'; return; }
  const newPath = s.currentPath === '/' ? '/' + name : s.currentPath + '/' + name;
  const params = new URLSearchParams({ project: s.name, path: newPath, source: s.source });
  try {
    const resp = await fetch(`/api/remote/mkdir?${params}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} });
    const data = await resp.json();
    if (data.status === 'ok') {
      msgEl.innerHTML = '<span style="color:var(--success)">✅ 目录已创建</span>';
      log(`📁 [${s.name}] 创建目录: ${newPath}`, 'info');
      setTimeout(() => { closeMkdirDialog(); loadFileList(s.currentPath); }, 800);
    } else {
      msgEl.innerHTML = `<span style="color:var(--danger)">❌ ${data.error}</span>`;
    }
  } catch(e) {
    msgEl.innerHTML = `<span style="color:var(--danger)">❌ ${e.message}</span>`;
  }
}

// ========================================================================
// 审计日志查看器
// ========================================================================
async function showAuditLog() {
  document.getElementById('auditLogModal').classList.add('active');
  // 加载可选日期
  try {
    const datesData = await fetch('/api/log/dates').then(r => r.json());
    const sel = document.getElementById('logDateSelect');
    sel.innerHTML = '<option value="">今天</option>';
    if (datesData && datesData.dates) {
      datesData.dates.forEach(d => {
        const opt = document.createElement('option');
        opt.value = d;
        opt.textContent = d;
        sel.appendChild(opt);
      });
    }
  } catch(e) {}
  loadAuditLog();
}

async function loadAuditLog() {
  const date = document.getElementById('logDateSelect').value;
  const level = document.getElementById('logLevelSelect').value;
  const keyword = document.getElementById('logKeywordInput').value.trim();
  const params = new URLSearchParams({ date, level, keyword, limit: '500' });
  try {
    const data = await fetch(`/api/log/query?${params}`).then(r => r.json());
    const tbody = document.getElementById('auditLogBody');
    const logs = data.logs || [];
    document.getElementById('logCountLabel').textContent = `共 ${logs.length} 条`;
    if (logs.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" class="empty-state">无日志</td></tr>';
      return;
    }
    tbody.innerHTML = logs.map((l, i) => {
      const levelClass = 'log-level ' + (l.level || 'info');
      const levelIcon = l.level === 'error' ? '❌' : l.level === 'warn' ? '⚠️' : 'ℹ️';
      return `<tr onclick="showLogDetail(${i})" style="cursor:pointer" title="点击查看详情">
        <td class="log-time">${l.time || '-'}</td>
        <td class="${levelClass}">${levelIcon}</td>
        <td class="log-msg">${escapeHtml(l.message || '')}</td>
      </tr>`;
    }).join('');
    // 保存日志数据供详情查看
    window._auditLogs = logs;
  } catch(e) {
    document.getElementById('auditLogBody').innerHTML = '<tr><td colspan="3" class="empty-state" style="color:var(--danger)">加载失败</td></tr>';
  }
}

// 查看日志详情
function showLogDetail(index) {
  const logs = window._auditLogs || [];
  const l = logs[index];
  if (!l) return;
  document.getElementById('logDetailTime').textContent = l.time || '-';
  const levelIcon = l.level === 'error' ? '❌' : l.level === 'warn' ? '⚠️' : 'ℹ️';
  document.getElementById('logDetailLevel').textContent = levelIcon + ' ' + (l.level || 'info');
  document.getElementById('logDetailMessage').textContent = l.message || '';
  document.getElementById('logDetailModal').classList.add('active');
}

// 删除当天审计日志
async function deleteAuditLog() {
  const date = document.getElementById('logDateSelect').value || new Date().toISOString().slice(0, 10);
  if (!confirm(`确定删除 ${date} 的全部审计日志？此操作不可恢复。`)) return;
  try {
    const resp = await fetch('/api/log/delete', {
      method: 'POST',
      headers: {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'},
      body: JSON.stringify({date}),
    });
    const data = await resp.json();
    if (data.status === 'ok') {
      log(`🗑️ 已删除 ${date} 的审计日志`, 'warn');
      loadAuditLog();
    } else {
      log(`❌ 删除审计日志失败: ${data.error}`, 'error');
    }
  } catch(e) {
    log(`❌ 删除审计日志失败: ${e.message}`, 'error');
  }
}

// ========================================================================
// 统一报告查看器
// ========================================================================
async function showAllReports() {
  document.getElementById('allReportsModal').classList.add('active');
  document.getElementById('reportKeywordInput').value = '';
  loadAllReports();
}

async function loadAllReports() {
  const keyword = document.getElementById('reportKeywordInput').value.trim().toLowerCase();
  try {
    const data = await fetch('/api/report/all').then(r => r.json());
    let reports = data.reports || [];
    if (keyword) {
      reports = reports.filter(r => r.project && r.project.toLowerCase().includes(keyword));
    }
    const tbody = document.getElementById('allReportsBody');
    document.getElementById('reportCountLabel').textContent = `共 ${reports.length} 条`;
    if (reports.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="empty-state">无报告</td></tr>';
      return;
    }
    tbody.innerHTML = reports.map(r => {
      const st = r.status === 'pass' ? '<span style="color:var(--success)">✅ 通过</span>' : r.status === 'fail' ? '<span style="color:var(--danger)">❌ 失败</span>' : '<span style="color:var(--text-tertiary)">-</span>';
      const cov = r.coverage || '-';
      return `<tr>
        <td><strong>${escapeHtml(r.project)}</strong></td>
        <td>${st}</td>
        <td>${r.total || 0}</td>
        <td style="color:var(--success)">${r.passed || 0}</td>
        <td style="color:var(--danger)">${r.failed || 0}</td>
        <td style="color:var(--purple)">${cov}</td>
        <td style="color:var(--text-tertiary);font-size:11px;font-family:var(--font-mono)">${r.timestamp || '-'}</td>
      </tr>`;
    }).join('');
  } catch(e) {
    document.getElementById('allReportsBody').innerHTML = '<tr><td colspan="7" class="empty-state" style="color:var(--danger)">加载失败</td></tr>';
  }
}

// ========================================================================
// 工具函数
// ========================================================================
function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

refreshProjects();

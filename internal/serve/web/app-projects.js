// === 项目管理 ===

const ruleDefs = {
  React:   [{id:'tsc', label:'TypeScript', cmd:'npx tsc --noEmit', file:'内置', def:true, desc:'检查 TypeScript 类型错误'}],
  Vue:     [{id:'tsc', label:'vue-tsc', cmd:'npx vue-tsc --noEmit', file:'内置', def:true, desc:'Vue 项目的 TypeScript 类型检查'},
            {id:'eslint', label:'ESLint 规范', cmd:'npx eslint -c rules/eslint-vue.mjs src/', file:'rules/eslint-vue.mjs', def:true, desc:'ESLint 检查'}],
  Maven:   [{id:'compile', label:'编译检查', cmd:'mvn compile -Xlint:all', file:'内置', def:true, desc:'Maven 编译检查'},
            {id:'checkstyle', label:'Checkstyle', cmd:'mvn checkstyle:check -Dcheckstyle.config=rules/checkstyle.xml', file:'rules/checkstyle.xml', def:true, desc:'Checkstyle 代码风格'}],
  MavenMulti:[{id:'compile', label:'多模块编译', cmd:'mvn compile -Xlint:all', file:'内置', def:true, desc:'多模块 Maven 编译'}],
  Node:    [{id:'eslint', label:'ESLint', cmd:'npx eslint src/', file:'内置', def:true, desc:'ESLint 检查'}],
  Unknown: [],
};

// ===== 刷新与渲染 =====
async function refreshProjects() {
  const data = await api('/api/projects');
  projects = data?.projects || [];
  try {
    const stData = await api('/api/steps/status');
    if (stData && stData.statuses) {
      if (!window._stepStatus) window._stepStatus = {};
      for (const [proj, steps] of Object.entries(stData.statuses)) {
        for (const [stepId, info] of Object.entries(steps)) {
          if (info.status === 'pass' || info.status === 'fail') {
            window._stepStatus[proj + ':' + stepId] = info.status;
            if (info.status === 'fail') {
              if (!window._stepErrors) window._stepErrors = {};
              window._stepErrors[proj + ':' + stepId] = { error_log: info.error_log || '', error: '' };
            }
          }
        }
      }
    }
  } catch(e) {}
  document.getElementById('welcomeCard').style.display = projects.length === 0 ? 'block' : 'none';
  remoteCount = 0;
  projects.forEach(p => { if (p.remotes) remoteCount += p.remotes.filter(r => r.enabled !== false).length; });
  renderProjects();
}

function getProjectStatus(p) {
  const allSteps = getProjectAllSteps(p);
  const stepStatuses = allSteps.map(s => getStep(p, s));
  if (stepStatuses.some(st => st === 'fail')) return 'fail';
  if (stepStatuses.some(st => st === 'running')) return 'running';
  if (stepStatuses.every(st => st === 'pass')) return 'pass';
  return 'pending';
}

function renderProjects() {
  const tbody = document.getElementById('projectBody');
  let pass = 0, fail = 0, deployed = 0;
  tbody.innerHTML = '';
  projects.forEach(p => {
    const s = getProjectStatus(p);
    if (s === 'pass') pass++; else if (s === 'fail') fail++;
    if (getStep(p, 'deploy') === 'pass') deployed++;
    const tt = (p.type||'unknown').toLowerCase();
    const configWarn = (!p.deploy || !p.deploy.host) ? '<span style="color:var(--warning);font-size:11px">⚠ 未配置部署</span>' : '<span style="color:var(--success);font-size:11px">✅ 已配置</span>';
    document.getElementById('totalCount').textContent = projects.length;
    document.getElementById('passCount').textContent = pass;
    document.getElementById('failCount').textContent = fail;
    document.getElementById('deployCount').textContent = deployed;
    document.getElementById('remoteCount').textContent = remoteCount;
    tbody.innerHTML += `<tr><td><strong>${p.name}</strong></td><td><span class="tag tag-${tt}">${p.type||'未知'}</span></td><td style="font-size:12px;font-family:var(--font-mono);color:var(--text-secondary)">${p.version || '-'}</td><td style="font-size:11px;color:var(--text-tertiary)">${p.git_branch || '-'}${p.git_commit ? '<br><code style="font-size:10px;background:var(--bg-elevated);padding:1px 5px;border-radius:var(--r-xs);font-family:var(--font-mono);color:var(--accent-hover)">'+p.git_commit.substring(0,7)+'</code>' : ''}</td><td><span class="status status-${s}"><span class="status-dot"></span>${s === 'pass' ? '通过' : s === 'fail' ? '失败' : s === 'running' ? '运行中' : '等待'}</span></td><td>${configWarn}${p.remotes && p.remotes.length > 0 ? '<span style="color:var(--purple);font-size:11px">📤 ' + p.remotes.filter(r=>r.enabled!==false).map(r=>r.name).join(', ') + '</span>' : '<span style="color:var(--text-quaternary);font-size:11px">无远程仓库</span>'}</td><td>${renderPipelineSummary(p)}</td><td><div class="stepper">${renderStepper(p)}</div></td><td>${renderActionButtons(p)}</td></tr>`;
  });
}

// ===== 操作按钮 =====
function renderActionButtons(p) {
  const allSteps = getProjectAllSteps(p);
  const stepMap = {};
  if (p.pipeline && p.pipeline.steps) p.pipeline.steps.forEach(s => stepMap[s.id] = s.enabled);
  let html = '';
  allSteps.forEach(s => {
    if (stepMap[s] !== false) {
      html += s === 'deploy'
        ? `<button class="action-btn btn-danger" onclick="runDeploy('${p.name}')">${btnLabels[s]}</button>`
        : `<button class="action-btn ${btnStyles[s]}" onclick="runAction('${s}','${p.name}')">${btnLabels[s]}</button>`;
    }
  });
  html += `<button class="action-btn btn-outline" onclick="editProject('${p.name}')">编辑</button>`;
  html += `<button class="action-btn btn-outline" onclick="showReport('${p.name}')" style="font-size:10px">📊 报告</button>`;
  html += `<button class="action-btn btn-outline" onclick="openBuildDir('${p.name}')" style="font-size:10px" title="打开构建产物目录">📁 产物</button>`;
  html += `<button class="action-btn btn-primary" onclick="runSinglePipeline('${p.name}')" style="font-size:10px">▶ 流水线</button>`;
  html += `<button class="action-btn btn-warning" onclick="cancelPipeline('${p.name}')" style="font-size:10px" title="当前步骤完成后暂停流水线">⏸</button>`;
  html += `<button class="action-btn btn-danger" onclick="deleteProject('${p.name}')" style="font-size:10px">🗑</button>`;
  return html;
}

// ===== 统计弹窗 =====
function showStatDetail(type) {
  const modal = document.getElementById('reportModal');
  let title = '', items = [];
  if (type === 'total') { title = '📋 所有项目'; items = projects.map(p => ({name:p.name,type:p.type||'未知',status:getProjectStatus(p),detail:`${p.version || '-'} · ${p.git_branch || '-'}`})); }
  else if (type === 'pass') { title = '✅ 已通过'; items = projects.filter(p => getProjectStatus(p) === 'pass').map(p => ({name:p.name,type:p.type||'未知',status:'pass',detail:p.version||'-'})); }
  else if (type === 'fail') { title = '❌ 失败'; items = projects.filter(p => getProjectStatus(p) === 'fail').map(p => { const f=defaultStepOrder.filter(s=>getStep(p,s)==='fail'); return {name:p.name,type:p.type||'未知',status:'fail',detail:'失败: '+f.map(s=>stepLabels[s]).join(', '),clickable:true,projectName:p.name}; }); }
  else if (type === 'deploy') { title = '🚀 部署成功'; items = projects.filter(p=>getStep(p,'deploy')==='pass').map(p=>({name:p.name,type:p.type||'未知',status:'pass',detail:p.deploy?`${p.deploy.user}@${p.deploy.host}`:'已部署'})); }
  else if (type === 'remote') { title = '📤 远程仓库'; projects.forEach(p=>{if(p.remotes)p.remotes.filter(r=>r.enabled!==false).forEach(r=>items.push({name:p.name,type:r.name,status:'pass',detail:r.url}));}); }
  if (!items.length) { modal.innerHTML = `<div class="modal-content" style="width:500px"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px"><h2 style="margin:0">${title}</h2><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="font-size:12px;padding:4px 12px">✕ 关闭</button></div><div style="padding:30px;text-align:center;color:var(--text-tertiary)">暂无数据</div></div>`; modal.classList.add('active'); return; }
  // 渲染详情表格
  const rows = items.map(it => {
    const icon = it.status === 'pass' ? '✅' : it.status === 'fail' ? '❌' : '⚪';
    const cursor = it.clickable ? 'cursor:pointer' : '';
    const click = it.clickable ? `onclick="showStepError('${it.projectName}','${defaultStepOrder.find(s => getStep(projects.find(p=>p.name===it.projectName), s) === 'fail')}')"` : '';
    return `<tr style="${cursor}" ${click}><td><strong>${it.name}</strong></td><td><span class="tag tag-${(it.type||'').toLowerCase()}">${it.type}</span></td><td style="text-align:center">${icon}</td><td style="font-size:11px;color:var(--text-tertiary)">${it.detail}</td></tr>`;
  }).join('');
  modal.innerHTML = `<div class="modal-content" style="width:750px;max-width:90vw">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
      <h2 style="margin:0">${title} <span style="font-size:13px;color:var(--text-tertiary);font-weight:400">(${items.length})</span></h2>
      <button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="font-size:12px;padding:4px 12px">✕ 关闭</button>
    </div>
    <table style="width:100%;font-size:13px;border-collapse:collapse">
      <thead><tr style="border-bottom:2px solid var(--border)">
        <th style="text-align:left;padding:6px 8px;font-size:10px;text-transform:uppercase;color:var(--text-quaternary)">项目</th>
        <th style="text-align:left;padding:6px 8px;font-size:10px;text-transform:uppercase;color:var(--text-quaternary);width:90px">类型</th>
        <th style="text-align:center;padding:6px 8px;font-size:10px;text-transform:uppercase;color:var(--text-quaternary);width:40px">状态</th>
        <th style="text-align:left;padding:6px 8px;font-size:10px;text-transform:uppercase;color:var(--text-quaternary)">详情</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>
  </div>`;
  modal.classList.add('active');
}

// ===== 项目编辑弹窗 =====
function showAddDialog() {
  document.getElementById('modalTitle').textContent = '添加项目';
  currentEditProjectType = 'Unknown';
  ['inputName','inputPath','inputHost','inputPort','inputUser','inputRemote','inputKeyPath'].forEach(id => document.getElementById(id).value = '');
  const branchSel = document.getElementById('inputGitBranch');
  if (branchSel) branchSel.innerHTML = '<option value="">默认（当前分支）</option>';
  document.getElementById('inputPort').value = '22';
  document.getElementById('inputTarget').value = 'production';
  document.getElementById('inputAuthType').value = 'key';
  document.getElementById('remoteList').innerHTML = '';
  document.getElementById('testResult').textContent = '';
  toggleKeyPath();
  renderRules(null); renderPipelineConfig(null);
  document.getElementById('projectModal').classList.add('active');
}

async function detectProject(path) { if (!path) return null; try { return await fetch(`/api/project/detect?path=${encodeURIComponent(path)}`).then(r=>r.json()); } catch(e) { return null; } }

async function onPathChanged() {
  const path = document.getElementById('inputPath').value.trim();
  if (!path) { renderRules(null); return; }
  const data = await detectProject(path);
  if (!data || data.error) { renderRules(null); currentEditProjectType = 'Unknown'; return; }
  currentEditProjectType = data.type || 'Unknown';
  const editingName = document.getElementById('inputName').value;
  const editingProject = projects.find(p => p.name === editingName);
  renderRules(editingProject || { path, rules: [] }, data.type);
  renderPipelineSteps();
  const rs = document.getElementById('rulesSection'); const ra = document.getElementById('rulesArrow');
  if (rs) rs.classList.remove('collapsed'); if (ra) ra.classList.add('open');
  if (data.branches && data.branches.length > 0) fillBranchSelect(data.branches, data.currentBranch);
  if (data.isGit && data.remotes && data.remotes.length > 0) autoFillRemotes(data.remotes);
}

function fillBranchSelect(branches, currentBranch) {
  const sel = document.getElementById('inputGitBranch');
  if (!sel) return;
  const prevValue = sel.value;
  sel.innerHTML = '<option value="">默认（当前分支）</option>';
  branches.forEach(b => { const opt = document.createElement('option'); opt.value = b; opt.textContent = b + (b === currentBranch ? ' （当前）' : ''); sel.appendChild(opt); });
  if (prevValue) sel.value = prevValue;
}

async function detectBranchesFromPath() {
  const path = document.getElementById('inputPath').value.trim();
  if (!path) { log('❌ 请先填写项目路径', 'warn'); return; }
  const data = await detectProject(path);
  if (!data || data.error) { log('❌ 检测失败', 'error'); return; }
  if (data.branches) fillBranchSelect(data.branches, data.currentBranch);
}

function autoFillRemotes(detected) {
  const rl = document.getElementById('remoteList');
  const existing = Array.from(rl.querySelectorAll('.remote-row')).some(r => r.querySelector('.remote-url').value.trim());
  if (existing) { log(`🔍 检测到 ${detected.length} 个 Git 远程，点击「从 Git 检测」导入`, 'info'); return; }
  rl.innerHTML = '';
  detected.forEach(r => addRemoteRow(r.name, r.url, true));
}

async function detectRemotesFromPath() {
  const path = document.getElementById('inputPath').value.trim();
  if (!path) { log('❌ 请先填写项目路径', 'warn'); return; }
  const data = await detectProject(path);
  if (!data || data.error || !data.isGit) { log('❌ 检测失败', 'error'); return; }
  document.getElementById('remoteList').innerHTML = '';
  data.remotes.forEach(r => addRemoteRow(r.name, r.url, true));
}

function toggleKeyPath() {
  document.getElementById('keyPathGroup').style.display = document.getElementById('inputAuthType').value === 'key' ? 'block' : 'none';
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
  const branchSel = document.getElementById('inputGitBranch');
  if (branchSel && p.gitBranch) branchSel.value = p.gitBranch;
  document.getElementById('remoteList').innerHTML = '';
  document.getElementById('testResult').textContent = '';
  toggleKeyPath();
  if (p.remotes) p.remotes.forEach(r => addRemoteRow(r.name, r.url, r.enabled));
  renderPipelineConfig(p);
  ['deploy','git','rules'].forEach(s => { const body=document.getElementById(s+'Section'); const arrow=document.getElementById(s+'Arrow'); if(body)body.classList.remove('collapsed'); if(arrow)arrow.classList.add('open'); });
  document.getElementById('projectModal').classList.add('active');
  onPathChanged();
}

function testConnection() {
  const host=document.getElementById('inputHost').value, port=document.getElementById('inputPort').value, user=document.getElementById('inputUser').value, auth=document.getElementById('inputAuthType').value, key=document.getElementById('inputKeyPath').value;
  const el=document.getElementById('testResult');
  if (!host||!user) { el.textContent='❌ 请填写主机和用户名'; return; }
  el.textContent='⏳ 测试中...';
  fetch(`/api/deploy/test?${new URLSearchParams({host,port,user,auth_type:auth,identity_file:key})}`).then(r=>r.json()).then(d=>{el.textContent=d.status==='ok'?'✅ 连接成功':'❌ 连接失败';}).catch(()=>{el.textContent='❌ 连接失败';});
}

function closeModal() { document.getElementById('projectModal').classList.remove('active'); }

// ===== 规则渲染 =====
function renderRules(project, typeOverride) {
  const el = document.getElementById('rulesList');
  let type = typeOverride || 'Unknown';
  const savedRules = {};
  let path = '';
  if (project && typeof project === 'object') { path = project.path||''; if (project.rules) project.rules.forEach(r=>savedRules[r.id]=r.enabled); }
  else if (typeof project === 'string') path = project;
  if (!typeOverride && path) { const m=projects.find(p=>p.path===path); if(m&&m.type)type=m.type; }
  const rules = ruleDefs[type]||[];
  let html = `<div style="margin-bottom:10px;font-size:12px;color:var(--text-tertiary)">检测到类型: <span class="tag ${ruleTypeColors[type]||'tag-unknown'}">${type}</span>${rules.length?'共 '+rules.length+' 条规则':''}</div>`;
  rules.forEach(r => {
    let checked = r.def; if (savedRules.hasOwnProperty(r.id)) checked = savedRules[r.id];
    html += `<div class="rule-item"><input type="checkbox" class="rule-cb" data-id="${r.id}" ${checked?'checked':''}><div class="rule-content"><div class="rule-label">${r.label}</div><div class="rule-cmd">$ ${r.cmd}</div></div></div>`;
  });
  el.innerHTML = html;
}

async function viewRuleFile(filePath) {
  const modal = document.getElementById('reportModal');
  let content = '加载中...';
  try { const resp = await fetch(`/api/rules?file=${encodeURIComponent(filePath)}`); content = resp.ok ? await resp.text() : '读取失败'; } catch(e) { content = `读取失败: ${e.message}`; }
  modal.innerHTML = `<div class="modal-content" style="width:720px"><h2>📄 规则文件</h2><div style="background:var(--bg-input);border:1px solid var(--border-subtle);border-radius:var(--r-md);padding:16px;font-family:var(--font-mono);font-size:12px;line-height:1.7;overflow:auto;max-height:55vh;white-space:pre-wrap">${escHtml(content)}</div><div class="modal-actions"><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')">关闭</button></div></div>`;
  modal.classList.add('active');
}

// ===== 本地目录浏览器 =====
let localBrowserCurrentPath = '';

function openPathBrowser() {
  const cur = document.getElementById('inputPath').value.trim();
  document.getElementById('pathBrowserModal').classList.add('active');
  if (cur) gotoLocalPath(cur); else loadLocalDir('');
}

async function loadLocalDir(dirPath) {
  const tbody = document.getElementById('localFileBody');
  tbody.innerHTML = '<tr><td colspan="2">加载中...</td></tr>';
  try {
    const data = await fetch(`/api/local/ls?${new URLSearchParams(dirPath?{path:dirPath}:{})}`).then(r=>r.json());
    if (data.error) { tbody.innerHTML=`<tr><td colspan="2">${data.error}</td></tr>`; return; }
    localBrowserCurrentPath = data.path||'';
    renderLocalTable(data);
  } catch(e) { tbody.innerHTML=`<tr><td colspan="2">${e.message}</td></tr>`; }
}

function renderLocalTable(data) {
  const tbody = document.getElementById('localFileBody');
  if (data.drives) { tbody.innerHTML = data.drives.map(d=>`<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(d)}')"><td>💽 ${d}</td><td>-</td></tr>`).join(''); return; }
  let rows = '';
  if (data.parent) rows += `<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(data.parent)}')"><td>📁 ..</td><td>-</td></tr>`;
  (data.files||[]).forEach(f => {
    if (f.is_dir) rows += `<tr style="cursor:pointer" onclick="loadLocalDir('${escJs(data.path+'/'+f.name)}')"><td>📁 ${f.name}</td><td>-</td></tr>`;
    else rows += `<tr><td><span style="opacity:0.6">${getFileIcon(f.name)} ${f.name}</span></td><td>${formatFileSize(f.size)}</td></tr>`;
  });
  tbody.innerHTML = rows;
}

function gotoLocalPath(p) { loadLocalDir(p || document.getElementById('localPathInput').value.trim()); }
function chooseLocalPath() { if (!localBrowserCurrentPath) return; document.getElementById('inputPath').value = localBrowserCurrentPath; document.getElementById('pathBrowserModal').classList.remove('active'); onPathChanged(); }

// ===== 远程仓库编辑 =====
function addRemoteRow(name, url, enabled) {
  const rl = document.getElementById('remoteList');
  const div = document.createElement('div'); div.className = 'remote-row';
  div.innerHTML = `<input class="remote-name" placeholder="名称" value="${name||''}"><input class="remote-url" placeholder="URL" value="${url||''}" style="flex:2"><label style="font-size:12px"><input type="checkbox" class="remote-enabled" ${enabled!==false?'checked':''}> 启用</label><button class="btn-danger" style="padding:5px 10px;font-size:12px" onclick="this.parentElement.remove()">✕</button>`;
  rl.appendChild(div);
}

// ===== 保存/删除项目 =====
async function saveProject() {
  const remotes = [];
  document.querySelectorAll('#remoteList .remote-row').forEach(row => { const n=row.querySelector('.remote-name').value, u=row.querySelector('.remote-url').value; if(n&&u) remotes.push({name:n,url:u,enabled:row.querySelector('.remote-enabled').checked}); });
  const p = {
    name: document.getElementById('inputName').value, path: document.getElementById('inputPath').value, enabled: true,
    deployTarget: document.getElementById('inputTarget').value,
    deploy: { host: document.getElementById('inputHost').value, port: parseInt(document.getElementById('inputPort').value)||22, user: document.getElementById('inputUser').value, auth_type: document.getElementById('inputAuthType').value, identity_file: document.getElementById('inputKeyPath').value, remote_dir: document.getElementById('inputRemote').value },
    remotes: remotes
  };
  const rules = []; document.querySelectorAll('.rule-cb').forEach(cb => rules.push({id:cb.dataset.id,enabled:cb.checked}));
  p.rules = rules; p.gitBranch = document.getElementById('inputGitBranch')?.value||'';
  p.pipeline = getPipelineConfigFromUI();
  if (!p.name||!p.path) { log('❌ 请填写项目名称和路径', 'error'); return; }
  const idx = projects.findIndex(x => x.name === p.name);
  if (idx >= 0) projects[idx] = p; else projects.push(p);
  try { await apiPost('/api/project', {projects}); log(`✅ 项目已保存: ${p.name}`, 'info'); } catch(e) { log(`❌ 保存失败`, 'error'); }
  closeModal(); refreshProjects();
}

async function deleteProject(name) {
  if (!confirm(`确定删除「${name}」？不会删除项目代码。`)) return;
  const idx = projects.findIndex(x => x.name === name);
  if (idx < 0) return; projects.splice(idx, 1);
  try { await apiPost('/api/project', {projects}); log(`🗑️ 已删除: ${name}`, 'warn'); refreshProjects(); } catch(e) { refreshProjects(); }
}

// ===== 打开构建产物目录 =====
function openBuildDir(name) {
  const p = projects.find(x => x.name === name);
  if (!p || !p.path) { log(`❌ [${name}] 找不到项目路径`, 'error'); return; }
  // 根据项目类型确定产物目录，不存在时回退到项目根目录
  const type = (p.type || '').toLowerCase();
  let subDir = '';
  if (['react', 'vue', 'angular', 'next', 'node'].includes(type)) {
    subDir = 'dist';
  } else if (['maven', 'mavenmulti'].includes(type)) {
    subDir = 'target';
  } else {
    subDir = 'dist'; // 默认尝试 dist
  }
  // 使用系统路径分隔符
  const basePath = p.path;
  const targetPath = basePath + '\\' + subDir;
  // 检查产物目录是否存在，不存在则打开项目根目录
  const openPath = basePath + '\\' + subDir;
  fetch(`/api/local/open-dir?path=${encodeURIComponent(openPath)}`)
    .then(r => r.json())
    .then(data => {
      if (data.status === 'ok') log(`📁 已打开: ${openPath}`, 'info');
      else {
        // 产物目录不存在，尝试打开项目根目录
        log(`⚠️ 产物目录不存在，打开项目根目录`, 'warn');
        fetch(`/api/local/open-dir?path=${encodeURIComponent(basePath)}`)
          .then(r2 => r2.json())
          .then(d2 => {
            if (d2.status === 'ok') log(`📁 已打开: ${basePath}`, 'info');
            else log(`❌ 打开失败: ${d2.error}`, 'error');
          });
      }
    })
    .catch(() => log(`❌ 打开目录失败`, 'error'));
}

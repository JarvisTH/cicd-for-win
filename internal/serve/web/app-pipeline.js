// === 流水线执行与管理 ===

function getProjectSteps(p) {
  if (!p || !p.pipeline || !p.pipeline.steps || p.pipeline.steps.length === 0) return defaultStepOrder;
  return p.pipeline.steps.filter(s => s.enabled).map(s => s.id);
}

function getProjectAllSteps(p) {
  if (!p || !p.pipeline || !p.pipeline.steps || p.pipeline.steps.length === 0) return defaultStepOrder;
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
  document.getElementById('autoToggle').textContent = autoPipeline ? '🌐 自动:ON' : '🌐 自动:OFF';
  if (autoPipeline) document.getElementById('autoToggle').classList.add('auto-on');
  else document.getElementById('autoToggle').classList.remove('auto-on');
  log(`🌐 自动流水线: ${autoPipeline ? '已开启' : '已关闭'}`, 'info');
}

function toggleConcurrent() {
  concurrentPipeline = !concurrentPipeline;
  document.getElementById('concurrentToggle').textContent = concurrentPipeline ? '⚡ 并发:ON' : '⚡ 并发:OFF';
  if (concurrentPipeline) document.getElementById('concurrentToggle').classList.add('auto-on');
  else document.getElementById('concurrentToggle').classList.remove('auto-on');
  log(`⚡ 并发执行: ${concurrentPipeline ? '已开启（多项目同时执行）' : '已关闭（项目逐个执行）'}`, 'info');
}

function renderStepper(p) {
  const steps = getProjectAllSteps(p);
  const stepMap = {};
  if (p && p.pipeline && p.pipeline.steps) p.pipeline.steps.forEach(s => stepMap[s.id] = s.enabled);
  return steps.map(s => {
    const st = getStep(p, s);
    const disabled = stepMap[s] === false;
    if (disabled) return `<span class="step-item pending" style="opacity:0.35;text-decoration:line-through" title="已禁用">⊘ ${stepLabels[s]}</span>`;
    if (st === 'fail') {
      const projName = (p.name || p).replace(/'/g, "\\'");
      return `<span class="step-item fail clickable" onclick="showStepError('${projName}','${s}')" title="点击查看错误详情">${stepIcons[st]} ${stepLabels[s]}</span>`;
    }
    return `<span class="step-item ${st}">${stepIcons[st]} ${stepLabels[s]}</span>`;
  }).join('<span class="step-arrow">→</span>');
}

function showStepError(project, step) {
  const key = project + ':' + step;
  const err = window._stepErrors[key];
  const modal = document.getElementById('reportModal');
  if (!err || (!err.error_log && !err.error)) {
    modal.innerHTML = `<div class="modal-content" style="width:600px;max-width:90vw"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px"><h2 style="margin:0">❌ ${stepLabels[step] || step} - 错误详情</h2><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')" style="font-size:12px;padding:4px 12px">✕ 关闭</button></div><div style="padding:20px;text-align:center;color:var(--text-tertiary)">未找到错误详情。可能该步骤的错误信息已丢失，请重新执行后重试。</div></div>`;
    modal.classList.add('active'); return;
  }
  const errLog = err.error_log || err.error || '';
  const errLines = errLog.split('\n').filter(l => l.trim());
  const isShortErr = !err.error_log && err.error;
  let bodyHtml = isShortErr ? `<div style="padding:12px 16px;background:var(--danger-subtle);border-radius:var(--r-sm);color:var(--danger);font-size:13px;line-height:1.6">${err.error}</div>`
    : `<pre class="error-log-pre">${errLines.map(l => l.replace(/</g,'&lt;').replace(/>/g,'&gt;')).join('\n')}</pre>`;
  modal.innerHTML = `<div class="modal-content" style="width:800px;max-width:90vw">${escHtml('')}</div>`;
}

// 生成流水线概览文本
function renderPipelineSummary(p) {
  const steps = getProjectSteps(p);
  if (steps.length === 0) return '<span style="color:var(--text-quaternary);font-size:11px">无启用步骤</span>';
  const isCustom = p && p.pipeline && p.pipeline.steps && p.pipeline.steps.length > 0;
  const summary = steps.map(s => stepLabels[s]).join(' → ');
  return isCustom ? `<span style="color:var(--accent);font-size:11px;font-weight:600" title="自定义流水线">⚙ ${summary}</span>` : `<span style="color:var(--text-tertiary);font-size:11px">${summary}</span>`;
}

// ===== 流水线配置 UI =====
let pipelineEditingSteps = [];
let currentEditProjectType = 'Unknown';

function getDefaultCmdForStep(stepId, projectType) {
  const def = stepDefaults[stepId];
  if (!def || !def.items) return null;
  for (const it of def.items) { if (it.type === projectType) return { command: it.command, args: it.args, matchedType: it.type }; }
  for (const it of def.items) {
    const types = it.type.split('/').map(t => t.split(' ')[0].trim());
    if (types.includes(projectType)) return { command: it.command, args: it.args, matchedType: it.type };
  }
  for (const it of def.items) { if (it.type === '通用') return { command: it.command, args: it.args, matchedType: it.type }; }
  return null;
}

function renderPipelineConfig(project) {
  if (project && project.pipeline && project.pipeline.steps && project.pipeline.steps.length > 0) {
    pipelineEditingSteps = JSON.parse(JSON.stringify(project.pipeline.steps));
    pipelineEditingSteps.forEach(s => { if (s.command === undefined) s.command = ''; if (s.args === undefined) s.args = ''; });
  } else {
    pipelineEditingSteps = defaultStepOrder.map(id => ({ id, enabled: true, command: '', args: '' }));
  }
  renderPipelineSteps();
}

function renderPipelineSteps() {
  const container = document.getElementById('pipelineConfig');
  container.innerHTML = '';
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
        <button class="step-toggle-btn ${step.enabled ? 'enabled' : 'disabled'}" onclick="togglePipelineStep(${index})">${step.enabled ? '✅ 启用' : '⏸ 禁用'}</button>
      </div>
      <div class="step-body">
        <div class="step-defaults">
          <div class="step-defaults-title">📋 默认命令${currentEditProjectType !== 'Unknown' ? ` · 当前类型: <span style="color:var(--accent)">${currentEditProjectType}</span>` : ''}</div>
          <table class="defaults-table">
            <thead><tr><th style="width:90px">类型</th><th>默认命令</th><th>参数</th></tr></thead>
            <tbody>${(stepDefaults[step.id] && stepDefaults[step.id].items || []).map(it => {
              const types = it.type.split('/').map(t => t.split(' ')[0].trim());
              const isActive = currentEditProjectType !== 'Unknown' && (it.type === currentEditProjectType || types.includes(currentEditProjectType) || it.type === '通用');
              return `<tr class="${isActive ? 'active-row' : ''}"><td>${it.type}</td><td><code>${it.command}</code></td><td><code>${it.args || '—'}</code></td></tr>`;
            }).join('')}</tbody>
          </table>
          ${stepDefaults[step.id] && stepDefaults[step.id].note ? `<div class="step-defaults-note">💡 ${stepDefaults[step.id].note}</div>` : ''}
        </div>
        <div class="cmd-row">
          <label>命令</label>
          <input type="text" placeholder="${(() => { const def = getDefaultCmdForStep(step.id, currentEditProjectType); return def ? `留空使用: ${def.command}` : '留空使用默认命令'; })()}" value="${step.command || ''}" oninput="updateStepCommand(${index}, this.value)">
        </div>
        <div class="cmd-row">
          <label>参数</label>
          <input type="text" placeholder="${(() => { const def = getDefaultCmdForStep(step.id, currentEditProjectType); return def ? (def.args ? `留空使用: ${def.args}` : '无默认参数') : '留空使用默认参数'; })()}" value="${step.args || ''}" oninput="updateStepArgs(${index}, this.value)">
        </div>
      </div>
    `;
    div.addEventListener('dragstart', (e) => { div.classList.add('dragging'); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', index); });
    div.addEventListener('dragend', () => { div.classList.remove('dragging'); document.querySelectorAll('.pipeline-step').forEach(el => el.classList.remove('drag-over')); });
    div.addEventListener('dragover', (e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; div.classList.add('drag-over'); });
    div.addEventListener('dragleave', () => { div.classList.remove('drag-over'); });
    div.addEventListener('drop', (e) => {
      e.preventDefault();
      const fromIndex = parseInt(e.dataTransfer.getData('text/plain'));
      const toIndex = parseInt(div.dataset.index);
      if (fromIndex === toIndex) return;
      const [moved] = pipelineEditingSteps.splice(fromIndex, 1);
      pipelineEditingSteps.splice(toIndex, 0, moved);
      renderPipelineSteps();
    });
    container.appendChild(div);
  });
}
function togglePipelineStep(index) { if (pipelineEditingSteps[index]) { pipelineEditingSteps[index].enabled = !pipelineEditingSteps[index].enabled; renderPipelineSteps(); } }
function toggleStepExpand(index) { const el = document.getElementById('pipelineConfig').children[index + 1]; if (el) el.classList.toggle('expanded'); }
function updateStepCommand(index, value) { if (pipelineEditingSteps[index]) pipelineEditingSteps[index].command = value; }
function updateStepArgs(index, value) { if (pipelineEditingSteps[index]) pipelineEditingSteps[index].args = value; }
function resetPipeline() { pipelineEditingSteps = defaultStepOrder.map(id => ({ id, enabled: true, command: '', args: '' })); renderPipelineSteps(); }
function getPipelineConfigFromUI() { return { steps: JSON.parse(JSON.stringify(pipelineEditingSteps)) }; }

// ===== 操作执行 =====
async function runAction(action, project) {
  const p = projects.find(x => x.name === project);
  const start = Date.now();
  setStep(p, action, 'running'); renderProjects();
  if (action === 'deploy' && (!p || !p.deploy || !p.deploy.host)) {
    log(`❌ [${project}] 未配置部署信息`, 'error'); setStep(p, action, 'pending'); renderProjects(); return;
  }
  runningCount++;
  const stepInfo = stepDefaults[action];
  if (stepInfo && stepInfo.items) log(`⚙️ [${project}] ${stepLabels[action]}: ${stepInfo.items.map(i => `${i.command} ${i.args}`.trim()).join(' | ')}`, 'info');
  document.getElementById('statusText').textContent = runningCount > 1 ? `正在执行 ${runningCount} 个操作...` : `${action} ${project}...`;
  const data = await api(`/api/${action}?project=${encodeURIComponent(project)}`);
  const elapsed = ((Date.now() - start) / 1000).toFixed(1);
  if (data && data.status === 'pass') {
    setStep(p, action, 'pass');
    log(`✅ [${project}] ${action} 通过 (${elapsed}s)`, 'info');
    if (action === 'test') { await new Promise(r => setTimeout(r, 100)); showReport(project, data); }
    if (autoPipeline) {
      const next = getNextStep(p, action);
      if (next) { log(`🔄 自动流水线: ${stepLabels[action]} ✅ → 继续 ${stepLabels[next]}...`, 'info'); await new Promise(r => setTimeout(r, 200)); await runAction(next, project); }
      else log(`🎉 [${project}] 流水线全部完成！`, 'info');
    }
  } else if (data) {
    setStep(p, action, 'fail');
    const errDetail = data.error_log || data.detail || data.error || '';
    window._stepErrors[stepKey(p, action)] = { error_log: errDetail, error: data.error || '', duration: elapsed + 's' };
    log(`❌ [${project}] ${action} 失败 (${elapsed}s)`, 'error');
    if (errDetail) { log(`📋 错误详情（点击该步骤可查看完整日志）:`, 'error'); errDetail.split('\n').filter(l => l.trim()).slice(-50).forEach(line => log(`   ${line}`, 'error')); }
  } else {
    setStep(p, action, 'fail');
    window._stepErrors[stepKey(p, action)] = { error_log: '', error: 'API 请求失败或响应格式异常', duration: elapsed + 's' };
    log(`❌ [${project}] ${action} 失败 (${elapsed}s)：API 响应异常`, 'error');
  }
  runningCount--;
  document.getElementById('statusText').textContent = runningCount > 0 ? `正在执行 ${runningCount} 个操作...` : '就绪';
  renderProjects();
}

function runDeploy(project) {
  const p = projects.find(x => x.name === project);
  if (!p || !p.deploy || !p.deploy.host) { log(`❌ [${project}] 未配置部署信息`, 'error'); return; }
  runAction('deploy', project);
}

async function batchRun(action) {
  log(`🔄 批量 ${action}...`, 'info');
  if (concurrentPipeline) await Promise.all(projects.map(p => runAction(action, p.name)));
  else { for (const p of projects) await runAction(action, p.name); }
  log(`✅ 批量 ${action} 完成`, 'info');
}

async function runPipelineAll() {
  log('⏯ 开始全链路流水线...', 'info');
  if (concurrentPipeline) {
    await Promise.all(projects.map(p => { const steps = getProjectSteps(p); return steps.length ? (log(`🔷 [${p.name}] 开始流水线 (${steps.join(' → ')})`, 'info'), runAction(steps[0], p.name)) : Promise.resolve(); }));
  } else {
    for (const p of projects) { const steps = getProjectSteps(p); if (!steps.length) { log(`⏭ [${p.name}] 无启用的流水线步骤`, 'warn'); continue; } log(`🔷 [${p.name}] 开始流水线 (${steps.join(' → ')})`, 'info'); await runAction(steps[0], p.name); }
  }
  log('✅ 全链路流水线执行完毕', 'info');
}

async function runSinglePipeline(project) {
  const p = projects.find(x => x.name === project);
  if (!p) return; const steps = getProjectSteps(p);
  if (!steps.length) { log(`⏭ [${project}] 无启用的流水线步骤`, 'warn'); return; }
  log(`▶️ 开始流水线: ${project} (${steps.join(' → ')})`, 'info');
  steps.forEach(s => setStep(p, s, 'pending'));
  try { await fetch(`/api/steps/status/clear?project=${encodeURIComponent(project)}`, { method: 'POST', headers: {'X-Requested-With': 'XMLHttpRequest'} }); } catch(e) {}
  renderProjects();
  for (let i = 0; i < steps.length; i++) {
    await runAction(steps[i], project);
    if (getStep(p, steps[i]) === 'fail') { log(`⏹ 流水线在 ${stepLabels[steps[i]]} 步骤终止`, 'warn'); break; }
  }
  if (steps.every(s => getStep(p, s) === 'pass')) log(`🎉 流水线全部通过: ${project}`, 'info');
  else log(`⚠️ 流水线未全部通过: ${project}`, 'warn');
}

// === 报告与日志 ===

// ===== 修改密码 =====
function showPasswordDialog() {
  ['inputOldPass','inputNewPass','inputConfirmPass'].forEach(id => document.getElementById(id).value = '');
  document.getElementById('passwordMsg').textContent = '';
  document.getElementById('passwordModal').classList.add('active');
}
function closePasswordDialog() { document.getElementById('passwordModal').classList.remove('active'); }

async function changePassword() {
  const oldPass = document.getElementById('inputOldPass').value;
  const newPass = document.getElementById('inputNewPass').value;
  const confirmPass = document.getElementById('inputConfirmPass').value;
  const msgEl = document.getElementById('passwordMsg');
  if (!oldPass||!newPass) { msgEl.innerHTML='<span style="color:var(--danger)">请填写完整</span>'; return; }
  if (newPass.length<6) { msgEl.innerHTML='<span style="color:var(--danger)">密码不能少于 6 位</span>'; return; }
  if (newPass!==confirmPass) { msgEl.innerHTML='<span style="color:var(--danger)">两次密码不一致</span>'; return; }
  const data = await apiPost('/api/auth/change-password', {old_password:oldPass,new_password:newPass});
  if (data&&data.status==='ok') { msgEl.innerHTML='<span style="color:var(--success)">✅ 修改成功</span>'; setTimeout(closePasswordDialog,1500); log('🔑 密码已修改','info'); }
  else msgEl.innerHTML=`<span style="color:var(--danger)">❌ ${data?.error||'修改失败'}</span>`;
}

// ===== 环境诊断 =====
async function runDoctor() {
  log('🔍 开始环境诊断...','info');
  let incompleteCount = projects.filter(p=>!p.deploy||!p.deploy.host).length;
  const html = [`<h3>🏥 环境诊断</h3><div class="doctor-item"><span class="icon">${projects.length?'✅':'⚠️'}</span><span class="label">项目</span><span class="value">${projects.length} 个</span></div><div class="doctor-item"><span class="icon">${incompleteCount===0?'✅':'⚠️'}</span><span class="label">部署配置</span><span class="value">${projects.length-incompleteCount} 完整</span></div>`];
  document.getElementById('doctorCard').innerHTML = html.join('');
  document.getElementById('doctorCard').style.display = 'block';
  setTimeout(()=>document.getElementById('doctorCard').style.display='none',15000);
}

// ===== 测试报告 =====
async function showReport(project, data) {
  const p = projects.find(x=>x.name===project);
  if (!p) return;
  if (data&&data.report) { if(data.report.total!==undefined) data={duration:data.duration,report:data.report}; else if(data.report.report) data={duration:data.report.duration,report:data.report.report}; }
  if (!data||!data.report) { data = await api(`/api/report/latest?project=${encodeURIComponent(project)}`); if(data&&data.report&&data.report.report) data={duration:data.report.duration,report:data.report.report}; }
  const rep = data?.report;
  if (!rep) { log(`📭 [${project}] 无报告`,'warn'); return; }
  const {total=0,passed=0,failed=0,skipped=0,coverage='-',failures=[]} = rep;
  const listData = await api(`/api/report/list?project=${encodeURIComponent(project)}`);
  const reports = listData?.reports||[];
  let html = `<div class="modal-content" style="width:680px"><h2>🧪 测试报告: ${project}</h2><div style="display:grid;grid-template-columns:repeat(4,1fr);gap:10px">`;
  html += `<div class="stat-card"><div class="num" style="color:var(--accent-hover)">${total}</div><div class="label">总数</div></div>`;
  html += `<div class="stat-card"><div class="num" style="color:var(--success)">${passed}</div><div class="label">通过</div></div>`;
  html += `<div class="stat-card"><div class="num" style="color:var(--danger)">${failed}</div><div class="label">失败</div></div>`;
  html += `<div class="stat-card"><div class="num" style="color:var(--warning)">${skipped}</div><div class="label">跳过</div></div></div>`;
  html += `<div style="padding:12px;background:var(--bg-elevated);border:1px solid var(--border-subtle);border-radius:var(--r-md);margin-bottom:12px"><span style="font-weight:600">覆盖率</span><span style="margin-left:8px;font-size:20px;color:var(--purple)">${coverage}</span></div>`;
  failures.forEach(f => { html += `<div style="background:var(--bg-elevated);border-left:2px solid var(--danger);padding:12px;margin-bottom:6px"><div style="color:var(--danger);font-weight:600">[${f.suite}] ${f.test}</div><div style="font-size:12px;margin-top:6px;font-family:var(--font-mono)">${f.message}</div></div>`; });
  if (reports.length) html += `<div style="margin-top:14px;border-top:1px solid var(--border-subtle);padding-top:12px"><h3 style="font-size:12px">历史 (${reports.length})</h3></div>`;
  html += `<div class="modal-actions"><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')">关闭</button></div></div>`;
  document.getElementById('reportModal').innerHTML = html;
  document.getElementById('reportModal').classList.add('active');
}

async function showReportById(project, id) {
  const data = await api(`/api/report/latest?project=${encodeURIComponent(project)}&id=${encodeURIComponent(id)}`);
  if (data&&data.report) showReport(project,data); else showReport(project);
}

async function deleteReport(project, id) {
  if (!confirm(`确定删除 ${id}？`)) return;
  const data = await apiPost('/api/report/delete',{project,id});
  if (data&&data.status==='ok') { log(`🗑️ 已删除报告`,'info'); showReport(project); }
}

// ===== 规则帮助 =====
function showRulesHelp() {
  const modal = document.getElementById('reportModal');
  let html = `<div class="modal-content" style="width:680px"><h2>🔍 规则说明</h2>`;
  html += `<div class="modal-actions"><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')">关闭</button></div></div>`;
  modal.innerHTML = html; modal.classList.add('active');
}

// ===== 审计日志 =====
async function showAuditLog() {
  document.getElementById('auditLogModal').classList.add('active');
  try { const d=await fetch('/api/log/dates').then(r=>r.json()); const sel=document.getElementById('logDateSelect'); sel.innerHTML='<option value="">今天</option>'; if(d&&d.dates)d.dates.forEach(dd=>{const o=document.createElement('option');o.value=dd;o.textContent=dd;sel.appendChild(o);}); } catch(e) {}
  loadAuditLog();
}

async function loadAuditLog() {
  const params = new URLSearchParams({date:document.getElementById('logDateSelect').value,level:document.getElementById('logLevelSelect').value,keyword:document.getElementById('logKeywordInput').value.trim(),limit:'500'});
  try { const data = await fetch(`/api/log/query?${params}`).then(r=>r.json()); const tbody=document.getElementById('auditLogBody'); const logs=data.logs||[]; tbody.innerHTML=logs.length?logs.map((l,i)=>`<tr onclick="showLogDetail(${i})" style="cursor:pointer"><td>${l.time||'-'}</td><td>${l.level||''}</td><td>${escHtml(l.message||'')}</td></tr>`).join(''):'<tr><td colspan="3">无日志</td></tr>'; window._auditLogs=logs; } catch(e) {}
}

function showLogDetail(index) { const l=(window._auditLogs||[])[index]; if(!l)return; document.getElementById('logDetailTime').textContent=l.time||'-'; document.getElementById('logDetailMessage').textContent=l.message||''; document.getElementById('logDetailModal').classList.add('active'); }

async function deleteAuditLog() {
  const date = document.getElementById('logDateSelect').value || new Date().toISOString().slice(0,10);
  if (!confirm(`删除 ${date} 的审计日志？`)) return;
  try { const r=await fetch('/api/log/delete',{method:'POST',headers:{'Content-Type':'application/json','X-Requested-With':'XMLHttpRequest'},body:JSON.stringify({date})}); const d=await r.json(); if(d.status==='ok')loadAuditLog(); } catch(e) {}
}

// ===== 所有报告 =====
async function showAllReports() { document.getElementById('allReportsModal').classList.add('active'); loadAllReports(); }

async function loadAllReports() {
  try { const data=await fetch('/api/report/all').then(r=>r.json()); const reports=data.reports||[]; const tbody=document.getElementById('allReportsBody'); tbody.innerHTML=reports.map(r=>`<tr><td>${r.project}</td><td>${r.status==='pass'?'✅':'❌'}</td><td>${r.total||0}</td><td>${r.passed||0}</td><td>${r.failed||0}</td><td>${r.coverage||'-'}</td><td>${r.timestamp||'-'}</td></tr>`).join(''); } catch(e) {}
}

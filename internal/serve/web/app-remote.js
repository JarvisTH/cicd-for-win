// === 远程管理 ===

let currentView = 'cicd';
const remoteSessions = {};
let activeTabId = null;
let sessionTimeout = 10 * 60 * 1000;

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
document.getElementById('remoteView').style.display = 'none';

window.addEventListener('resize', () => { const s=getSession(); if(s&&s.fitAddon&&s.term&&s.connected) try{s.fitAddon.fit();}catch(e){} });

// ===== 会话管理 =====
function createSession(tabId, name, source) { return {tabId,name,source,term:null,fitAddon:null,ws:null,connected:false,connecting:false,currentPath:'/',remoteFiles:[],selectedFile:'',lastActivity:Date.now(),sessionTimer:null,termContainer:null}; }
function getSession() { return activeTabId ? remoteSessions[activeTabId] : null; }
function updateActivity() { const s=getSession(); if(s)s.lastActivity=Date.now(); }

function startSessionTimer(tabId) {
  const s=remoteSessions[tabId]; if(!s)return;
  if(s.sessionTimer)clearInterval(s.sessionTimer);
  s.lastActivity=Date.now();
  s.sessionTimer=setInterval(()=>{if(!s.connected)return;if(Date.now()-s.lastActivity>sessionTimeout){log(`⏰ [${s.name}] 会话超时`,'warn');forceDisconnectTab(tabId);}},10000);
}
function stopSessionTimer(tabId) { const s=remoteSessions[tabId]; if(s&&s.sessionTimer){clearInterval(s.sessionTimer);s.sessionTimer=null;} }

function getRemoteProjectName() { const s=getSession(); return s?s.name:''; }
function getRemoteSource() { const s=getSession(); return s?s.source:''; }
function getRemoteConnected() { const s=getSession(); return s?s.connected:false; }

// ===== 标签页 =====
function renderRemoteTabs() {
  const container=document.getElementById('remoteTabs');
  const tabIds=Object.keys(remoteSessions);
  if(!tabIds.length){container.innerHTML='<div class="remote-tab-placeholder">选择服务器并点击「连接」</div>';return;}
  container.innerHTML='';
  tabIds.forEach(tabId=>{
    const s=remoteSessions[tabId];
    const tab=document.createElement('div'); tab.className='remote-tab'+(tabId===activeTabId?' active':'');
    tab.innerHTML=`<span class="tab-status-dot ${s.connected?'connected':s.connecting?'connecting':'disconnected'}"></span><span onclick="switchToTab('${tabId}')">${s.name}</span><span class="tab-close" onclick="event.stopPropagation();closeTab('${tabId}')">✕</span>`;
    tab.onclick=()=>switchToTab(tabId); container.appendChild(tab);
  });
}

function switchToTab(tabId) {
  const s=remoteSessions[tabId]; if(!s)return;
  activeTabId=tabId; renderRemoteTabs();
  renderFileBreadcrumb(s.currentPath); renderFileTable(s.remoteFiles);
  showTerminalForTab(tabId);
  document.getElementById('downloadBtn').disabled=!s.selectedFile;
}

function closeTab(tabId) {
  const s=remoteSessions[tabId]; if(!s)return;
  if(s.ws) try{s.ws.close();}catch(e){}
  stopSessionTimer(tabId);
  if(s.name) fetch(`/api/remote/disconnect?project=${encodeURIComponent(s.name)}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'}}).catch(()=>{});
  if(s.term) try{s.term.dispose();}catch(e){}
  delete remoteSessions[tabId];
  if(activeTabId===tabId){const r=Object.keys(remoteSessions);if(r.length)switchToTab(r[0]);else{activeTabId=null;document.getElementById('terminal-container').innerHTML='<div style="padding:40px;text-align:center;color:var(--text-tertiary)">选择服务器后点击「连接」</div>';document.getElementById('fileBreadcrumb').innerHTML='<span>选择服务器 → 连接</span>';document.getElementById('fileBody').innerHTML='<tr><td colspan="2">未连接</td></tr>';}}
  renderRemoteTabs();
}

function forceDisconnectTab(tabId) {
  const s=remoteSessions[tabId]; if(!s)return;
  if(s.ws){try{s.ws.close();}catch(e){}s.ws=null;}
  s.connected=false;s.connecting=false;stopSessionTimer(tabId);
  if(s.name) fetch(`/api/remote/disconnect?project=${encodeURIComponent(s.name)}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'}}).catch(()=>{});
  if(s.term)s.term.write('\r\n=== 断开 ===\r\n');
  if(tabId===activeTabId){document.getElementById('fileBody').innerHTML='<tr><td colspan="2">已断开</td></tr>';document.getElementById('downloadBtn').disabled=true;}
  renderRemoteTabs(); updateBroadcastBar();
}

function showTerminalForTab(tabId) {
  const s=remoteSessions[tabId]; if(!s||!s.term)return;
  const mc=document.getElementById('terminal-container'); mc.innerHTML=''; mc.style.display='block'; mc.style.minHeight='420px';
  if(s.term.element)mc.appendChild(s.term.element);
  s.term.focus();
  setTimeout(()=>{try{s.fitAddon.fit();}catch(e){}},50);
}

// ===== 服务器管理 =====
async function loadRemoteProjects() {
  try {
    const data=await api('/api/remote/projects');
    const sel=document.getElementById('remoteProjectSelect');
    sel.innerHTML='<option value="">-- 选择服务器 --</option>';
    if(data&&data.servers)data.servers.forEach(s=>{const o=document.createElement('option');o.value=s.ref;o.dataset.source=s.source;o.textContent=`${s.name} (${s.deploy?.user||'?'}@${s.deploy?.host||'?'})`;sel.appendChild(o);});
    updateDeleteBtnState();
  }catch(e){log(`❌ 加载失败`,'error');}
}
function onRemoteProjectChange(){updateDeleteBtnState();}
function updateDeleteBtnState(){const sel=document.getElementById('remoteProjectSelect');const btn=document.getElementById('deleteServerBtn');const opt=sel.options[sel.selectedIndex];btn.style.display=opt&&opt.dataset?.source==='standalone'?'inline-flex':'none';}

function showAddServerDialog() {
  ['svrName','svrHost','svrUser','svrKeyPath','svrPassword','svrNote'].forEach(id=>document.getElementById(id).value='');
  document.getElementById('svrPort').value='22';
  document.getElementById('svrAuthType').value='key';
  document.getElementById('addServerMsg').textContent='';
  toggleSvrKeyPath();
  document.getElementById('addServerModal').classList.add('active');
}
function closeAddServerDialog(){document.getElementById('addServerModal').classList.remove('active');}
function toggleSvrKeyPath(){document.getElementById('svrKeyPathGroup').style.display=document.getElementById('svrAuthType').value==='key'?'block':'none';}

async function doAddServer() {
  const svr={name:document.getElementById('svrName').value.trim(),host:document.getElementById('svrHost').value.trim(),port:parseInt(document.getElementById('svrPort').value)||22,user:document.getElementById('svrUser').value.trim(),auth_type:document.getElementById('svrAuthType').value,identity_file:document.getElementById('svrKeyPath').value.trim(),password:document.getElementById('svrPassword').value,note:document.getElementById('svrNote').value.trim()};
  if(!svr.name||!svr.host||!svr.user){document.getElementById('addServerMsg').innerHTML='<span style="color:var(--danger)">名称/主机/用户名必填</span>';return;}
  try{const r=await fetch('/api/remote/servers',{method:'POST',headers:{'Content-Type':'application/json','X-Requested-With':'XMLHttpRequest'},body:JSON.stringify(svr)});const d=await r.json();if(d.status==='ok'){log(`🖥️ 已添加: ${svr.name}`,'info');setTimeout(()=>{closeAddServerDialog();loadRemoteProjects();},800);}}catch(e){}
}

async function deleteSelectedServer() {
  const name=document.getElementById('remoteProjectSelect').value; if(!name||!confirm(`删除「${name}」？`))return;
  try{const r=await fetch(`/api/remote/server?${new URLSearchParams({name})}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'}});const d=await r.json();if(d.status==='ok'){log(`🗑️ 已删除`,'info');loadRemoteProjects();}}catch(e){}
}

// ===== 连接 =====
function connectRemote() {
  const sel=document.getElementById('remoteProjectSelect'); const opt=sel.options[sel.selectedIndex]; const name=sel.value;
  if(!name){log('❌ 请选择服务器','error');return;}
  const source=opt?.dataset?.source||'project'; const tabId=name+'|'+source;
  if(remoteSessions[tabId]){switchToTab(tabId);if(!remoteSessions[tabId].connected&&!remoteSessions[tabId].connecting)doConnect(tabId);return;}
  remoteSessions[tabId]=createSession(tabId,name,source); activeTabId=tabId; renderRemoteTabs(); doConnect(tabId);
}

async function doConnect(tabId) {
  const s=remoteSessions[tabId]; if(!s)return;
  s.connecting=true; renderRemoteTabs(); log(`🔌 连接 ${s.name}...`,'info');
  const proto=window.location.protocol==='https:'?'wss:':'ws:';
  const url=`${proto}//${window.location.host}/api/remote/term?project=${encodeURIComponent(s.name)}&source=${encodeURIComponent(s.source)}`;
  s.ws=new WebSocket(url);
  s.ws.onopen=()=>{s.connected=true;s.connecting=false;renderRemoteTabs();updateBroadcastBar();startSessionTimer(tabId);setTimeout(()=>refreshFileList(),800);};
  s.ws.onmessage=(evt)=>{s.lastActivity=Date.now();if(evt.data instanceof Blob){evt.data.arrayBuffer().then(buf=>{if(s.term)s.term.write(new Uint8Array(buf));});}else{if(s.term)s.term.write(evt.data);}};
  s.ws.onclose=()=>{s.connected=false;s.connecting=false;s.ws=null;stopSessionTimer(tabId);renderRemoteTabs();updateBroadcastBar();if(s.term)s.term.write('\r\n=== 断开 ===\r\n');};
  s.ws.onerror=()=>{s.connecting=false;renderRemoteTabs();log(`❌ WebSocket 连接失败`,'error');};
}

function disconnectCurrentTab(){if(!activeTabId)return;forceDisconnectTab(activeTabId);}
function disconnectAllTabs(){Object.keys(remoteSessions).forEach(tabId=>forceDisconnectTab(tabId));log(`🔌 已断开全部`,'warn');}

// ===== 文件管理 =====
async function refreshFileList(){updateActivity();const s=getSession();if(!s){document.getElementById('fileBody').innerHTML='<tr><td colspan="2">请先连接</td></tr>';return;}await loadFileList(s.currentPath);}

async function loadFileList(dirPath) {
  const s=getSession(); if(!s||!s.connected)return;
  s.lastActivity=Date.now();
  try{const data=await fetch(`/api/remote/ls?${new URLSearchParams({project:s.name,path:dirPath,source:s.source})}`).then(r=>r.json());if(data.error){log(`❌ ${data.error}`,'error');return;}s.currentPath=data.path;s.remoteFiles=data.files||[];renderFileBreadcrumb(data.path);renderFileTable(data.files);}catch(e){log(`❌ 文件列表失败`,'error');}
}

function renderFileBreadcrumb(dirPath) {
  const el=document.getElementById('fileBreadcrumb');
  if(!dirPath||dirPath==='/'){el.innerHTML='<a onclick="loadFileList(\'/\')">/</a>';return;}
  const parts=dirPath.split('/').filter(Boolean);
  let html='<a onclick="loadFileList(\'/\')">/</a>'; let cur='';
  parts.forEach((part,i)=>{cur+='/'+part;html+='<span class="sep">/</span>';if(i===parts.length-1)html+=`<span>${part}</span>`;else html+=`<a onclick="loadFileList('${cur}')">${part}</a>`;});
  el.innerHTML=html;
}

function renderFileTable(files) {
  const tbody=document.getElementById('fileBody'); const s=getSession();
  if(!files||!files.length){tbody.innerHTML='<tr><td colspan="2">空目录</td></tr>';return;}
  tbody.innerHTML=files.map(f=>{
    if(f.is_dir)return `<tr style="cursor:pointer" onclick="loadFileList('${escJs(s?.['currentPath']||'/'+'/'+f.name)}')"><td>📁 ${f.name}</td><td>-</td></tr>`;
    return `<tr onclick="selectFile('${escJs(f.name)}')" class="${s?.selectedFile===f.name?'selected-row':''}"><td>${getFileIcon(f.name)} ${f.name}</td><td>${formatFileSize(f.size)}</td></tr>`;
  }).join('');
  document.getElementById('downloadBtn').disabled=!s?.selectedFile;
}

function selectFile(name){const s=getSession();if(!s)return;s.selectedFile=s.selectedFile===name?'':name;renderFileTable(s.remoteFiles);}
function downloadSelected(){const s=getSession();if(!s||!s.selectedFile)return;downloadFile(s.selectedFile);s.selectedFile='';renderFileTable(s.remoteFiles);}

function goToParentDir(){const s=getSession();if(!s||!s.currentPath||s.currentPath==='/')return;loadFileList(s.currentPath.substring(0,s.currentPath.lastIndexOf('/'))||'/');}

function uploadFile(){document.getElementById('fileUploadInput').click();}

async function doUpload() {
  const s=getSession(); if(!s)return;
  const input=document.getElementById('fileUploadInput'); if(!input.files||!input.files[0])return;
  const fd=new FormData(); fd.append('file',input.files[0]);
  try{const r=await fetch(`/api/remote/upload?${new URLSearchParams({project:s.name,path:s.currentPath,source:s.source})}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'},body:fd});const d=await r.json();if(d.status==='ok'){log(`📤 上传完成`,'info');loadFileList(s.currentPath);}}catch(e){}
  input.value='';
}

async function downloadFile(name) {
  const s=getSession(); if(!s)return;
  const filePath=s.currentPath==='/'?'/'+name:s.currentPath+'/'+name;
  try{const tr=await fetch('/api/remote/download-token',{credentials:'same-origin'});const td=await tr.json();const a=document.createElement('a');a.href=`/api/remote/download?${new URLSearchParams({project:s.name,path:filePath,source:s.source})}&download_token=${encodeURIComponent(td.token)}`;a.download=name;a.style.display='none';document.body.appendChild(a);a.click();document.body.removeChild(a);}catch(e){log(`❌ 下载失败`,'error');}
}

async function deleteRemoteFile(name) {
  const s=getSession(); if(!s||!confirm(`删除 ${name}？`))return;
  const filePath=s.currentPath==='/'?'/'+name:s.currentPath+'/'+name;
  try{const r=await fetch(`/api/remote/delete?${new URLSearchParams({project:s.name,path:filePath,source:s.source})}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'}});const d=await r.json();if(d.status==='ok')loadFileList(s.currentPath);}catch(e){}
}

function showMkdirDialog(){document.getElementById('inputMkdir').value='';document.getElementById('mkdirMsg').textContent='';document.getElementById('mkdirModal').classList.add('active');}
function closeMkdirDialog(){document.getElementById('mkdirModal').classList.remove('active');}

async function doMkdir() {
  const s=getSession(); if(!s)return;
  const name=document.getElementById('inputMkdir').value.trim(); if(!name)return;
  const newPath=s.currentPath==='/'?'/'+name:s.currentPath+'/'+name;
  try{const r=await fetch(`/api/remote/mkdir?${new URLSearchParams({project:s.name,path:newPath,source:s.source})}`,{method:'POST',headers:{'X-Requested-With':'XMLHttpRequest'}});const d=await r.json();if(d.status==='ok'){closeMkdirDialog();loadFileList(s.currentPath);}}catch(e){}
}

// ===== 广播 =====
function updateBroadcastBar() {
  const connected=Object.values(remoteSessions).filter(s=>s.connected);
  const bar=document.getElementById('broadcastBar'); if(!bar)return;
  if(connected.length>=2){bar.style.display='flex';document.getElementById('broadcastCount').textContent=connected.length+' 台';}else bar.style.display='none';
}

function sendBroadcast() {
  const input=document.getElementById('broadcastInput'); const cmd=input?.value.trim(); if(!cmd)return;
  Object.values(remoteSessions).filter(s=>s.connected&&s.ws).forEach(s=>{try{s.ws.send(cmd+'\n');}catch(e){}});
  log(`📡 [广播] 已发送: ${cmd}`,'info'); input.value='';
}

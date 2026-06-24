// === 入口：初始化与 UI 切换 ===

function toggleSection(name) {
  const body = document.getElementById(name + 'Section');
  const arrow = document.getElementById(name + 'Arrow');
  if (!body || !arrow) return;
  body.classList.toggle('collapsed');
  arrow.classList.toggle('open');
}

function showRulesHelp() {
  const modal = document.getElementById('reportModal');
  modal.innerHTML = `<div class="modal-content" style="width:680px"><h2>🔍 代码检查规则说明</h2><div class="modal-actions"><button class="btn-outline" onclick="document.getElementById('reportModal').classList.remove('active')">关闭</button></div></div>`;
  modal.classList.add('active');
}

// 兼容旧引用
if (!window._stepStatus) window._stepStatus = {};
if (!window._stepErrors) window._stepErrors = {};

// ===== 状态轮询：每 1 秒同步服务端步骤状态（支持托盘/外部触发的流水线） =====

async function pollStepStatus() {
  try {
    const stData = await api('/api/steps/status');
    if (!stData || !stData.statuses) return;

    // 完整同步：以服务端为准，重建本地状态
    const newStatus = {};
    const newErrors = {};
    for (const [proj, steps] of Object.entries(stData.statuses)) {
      for (const [stepId, info] of Object.entries(steps)) {
        const key = proj + ':' + stepId;
        if (info.status === 'pass' || info.status === 'fail' || info.status === 'running') {
          newStatus[key] = info.status;
          if (info.status === 'fail') {
            newErrors[key] = { error_log: info.error_log || '', error: '' };
          }
        }
      }
    }
    // 检测是否有变化（避免无谓的日志和渲染）
    let changed = false;
    const oldKeys = Object.keys(window._stepStatus || {}).sort();
    const newKeys = Object.keys(newStatus).sort();
    if (oldKeys.length !== newKeys.length || oldKeys.some((k, i) => k !== newKeys[i] || window._stepStatus[k] !== newStatus[k])) {
      changed = true;
    }
    window._stepStatus = newStatus;
    window._stepErrors = newErrors;

    // 更新统计卡片
    const allSteps = Object.values(newStatus);
    document.getElementById('passCount').textContent = allSteps.filter(v => v === 'pass').length;
    document.getElementById('failCount').textContent = allSteps.filter(v => v === 'fail').length;

    if (changed) renderProjects();
  } catch(e) { /* 忽略 */ }
}

// 页面加载后刷新
document.addEventListener('DOMContentLoaded', () => {
  refreshProjects();
  // 初始化右上角「更多」菜单中设置开关项的显示状态
  if (typeof updateSettingsMenu === 'function') updateSettingsMenu();
  // 广播输入框键盘事件
  const input = document.getElementById('broadcastInput');
  if (input) {
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); sendBroadcast(); } });
    input.addEventListener('input', () => { document.getElementById('broadcastSendBtn').disabled = !input.value.trim(); });
  }

  // 启动轮询（1 秒间隔，托盘操作后状态近乎实时更新）
  setInterval(pollStepStatus, 1000);
});

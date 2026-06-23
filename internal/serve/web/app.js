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
});

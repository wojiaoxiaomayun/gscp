
let servers = [];
let openEditAlias = null;

//  Navigation 
function switchPage(name, btn) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
  document.getElementById('page-' + name).classList.add('active');
  btn.classList.add('active');
  if (window.innerWidth <= 700) closeSidebar();
  if (name === 'projects') loadWorkspaces();
  if (name === 'settings') loadSettings();
}

function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
}
function closeSidebar() {
  document.getElementById('sidebar').classList.remove('open');
}
document.addEventListener('click', function(e) {
  const sb = document.getElementById('sidebar');
  const toggle = document.querySelector('.sidebar-toggle');
  if (window.innerWidth <= 700 && sb.classList.contains('open')) {
    if (!sb.contains(e.target) && !toggle.contains(e.target)) closeSidebar();
  }
});

//  Load servers 
async function loadServers() {
  try {
    const res = await fetch('/api/servers');
    servers = await res.json() || [];
    renderList();
  } catch(e) {
    showToast('加载失败: ' + e.message, true);
  }
}

function renderList() {
  const container = document.getElementById('server-list');
  const count = servers ? servers.length : 0;
  document.getElementById('nav-count').textContent = count;

  if (!count) {
    container.innerHTML = '<div class="empty-state"><div class="empty-icon"></div><p>暂无服务器  在上方添加第一台</p></div>';
    return;
  }

  container.innerHTML = servers.map((s, i) => `
    <div style="animation-delay:${i*35}ms">
      <div class="server-item">
        <div><span class="alias-badge">${esc(s.alias)}</span></div>
        <div class="host-text">${esc(s.host)}</div>
        <div class="user-text">${esc(s.username)} ${s.key_path ? '<span title="密钥认证" style="color:var(--accent);font-size:0.75rem;">🔑</span>' : ''} ${s.has_password ? '<span title="已保存密码" style="color:var(--text-3);font-size:0.75rem;">●</span>' : ''}</div>
        <div class="item-actions">
          <button class="btn btn-edit btn-sm" onclick="toggleEdit('${esc(s.alias)}')">编辑</button>
          <button class="btn btn-danger-ghost btn-sm" onclick="deleteServer('${esc(s.alias)}')">删除</button>
        </div>
      </div>
      <div class="edit-panel" id="panel-${esc(s.alias)}">
        <div class="edit-panel-inner">
          <div class="edit-panel-content">
            <div class="form-grid">
              <div class="field">
                <label>HOST[:PORT]</label>
                <input id="e-host-${esc(s.alias)}" value="${esc(s.host)}" />
              </div>
              <div class="field">
                <label>USERNAME</label>
                <input id="e-user-${esc(s.alias)}" value="${esc(s.username)}" />
              </div>
              <div class="field">
                <label>PASSWORD（留空则不修改）</label>
                <input id="e-pass-${esc(s.alias)}" type="password" placeholder="${s.has_password ? '已保存密码' : '未配置'}" />
                <label style="margin-top:0.45rem;text-transform:none"><input id="e-clear-pass-${esc(s.alias)}" type="checkbox" ${s.has_password ? '' : 'disabled'} /> 清除已保存密码</label>
              </div>
              <div class="field">
                <label>SSH 私钥路径（清空即移除密钥）</label>
                <input id="e-keypath-${esc(s.alias)}" value="${esc(s.key_path || '')}" placeholder="~/.ssh/id_rsa" />
              </div>
              <div class="field" style="grid-column:1/-1">
                <label>私钥密码（留空则不修改）</label>
                <input id="e-keypass-${esc(s.alias)}" type="password" placeholder="${s.has_key_pass ? '已保存私钥密码' : '未配置'}" />
                <label style="margin-top:0.45rem;text-transform:none"><input id="e-clear-keypass-${esc(s.alias)}" type="checkbox" ${s.has_key_pass ? '' : 'disabled'} /> 清除已保存私钥密码</label>
              </div>
            </div>
            <div class="form-footer">
              <button class="btn btn-primary" onclick="saveEdit('${esc(s.alias)}')">保存</button>
              <button class="btn btn-ghost" onclick="toggleEdit('${esc(s.alias)}')">取消</button>
            </div>
          </div>
        </div>
      </div>
    </div>`).join('');
}

//  Edit 
function toggleEdit(alias) {
  if (openEditAlias && openEditAlias !== alias) {
    const prev = document.getElementById('panel-' + openEditAlias);
    if (prev) prev.classList.remove('open');
  }
  const panel = document.getElementById('panel-' + alias);
  if (!panel) return;
  const isOpen = panel.classList.toggle('open');
  openEditAlias = isOpen ? alias : null;
  if (isOpen) setTimeout(() => { const el = document.getElementById('e-host-' + alias); if (el) el.focus(); }, 50);
}

//  Add 
async function addServer() {
  const alias = document.getElementById('f-alias').value.trim();
  const host  = document.getElementById('f-host').value.trim();
  const user  = document.getElementById('f-user').value.trim();
  const pass  = document.getElementById('f-pass').value;
  const keyPath = document.getElementById('f-keypath').value.trim();
  const keyPass = document.getElementById('f-keypass').value;
  const errEl = document.getElementById('add-err');
  if (!alias || !host || !user) { errEl.textContent = '别名、主机和用户名必填'; return; }
  if (!pass && !keyPath) { errEl.textContent = '密码和私钥路径至少填一个'; return; }
  if (keyPass && !keyPath) { errEl.textContent = '填写私钥密码前请先填写私钥路径'; return; }
  errEl.textContent = '';
  try {
    const payload = {alias, host, username: user};
    if (pass) payload.password = pass;
    if (keyPath) { payload.key_path = keyPath; if (keyPass) payload.key_pass = keyPass; }
    const res = await fetch('/api/servers', {
      method: 'POST', headers: {'Content-Type':'application/json'},
      body: JSON.stringify(payload)
    });
    if (!res.ok) { const d = await res.json(); errEl.textContent = d.error || '添加失败'; return; }
    ['f-alias','f-host','f-user','f-pass','f-keypath','f-keypass'].forEach(id => document.getElementById(id).value = '');
    showToast('服务器已添加');
    loadServers();
  } catch(e) { errEl.textContent = e.message; }
}

//  Save edit 
async function saveEdit(alias) {
  const s = servers.find(x => x.alias === alias);
  if (!s) { showToast('服务器配置不存在', true); return; }
  const host = document.getElementById('e-host-' + alias).value.trim();
  const user = document.getElementById('e-user-' + alias).value.trim();
  const pass = document.getElementById('e-pass-' + alias).value;
  const keyPath = document.getElementById('e-keypath-' + alias).value.trim();
  const keyPass = document.getElementById('e-keypass-' + alias).value;
  const clearPass = document.getElementById('e-clear-pass-' + alias).checked;
  const clearKeyPass = document.getElementById('e-clear-keypass-' + alias).checked;
  if (!host || !user) { showToast('主机和用户名不能为空', true); return; }
  if (pass && clearPass) { showToast('不能同时设置和清除密码', true); return; }
  if (keyPass && clearKeyPass) { showToast('不能同时设置和清除私钥密码', true); return; }
  if (keyPass && !keyPath) { showToast('填写私钥密码前请先填写私钥路径', true); return; }
  const hasPassword = pass ? true : (clearPass ? false : s.has_password);
  if (!hasPassword && !keyPath) { showToast('密码和私钥路径至少保留一个', true); return; }
  try {
    const payload = {host, username: user, key_path: keyPath};
    if (pass) payload.password = pass;
    else if (clearPass) payload.password = '';
    if (keyPass) payload.key_pass = keyPass;
    else if (clearKeyPass) payload.key_pass = '';
    const res = await fetch('/api/servers/' + encodeURIComponent(alias), {
      method: 'PUT', headers: {'Content-Type':'application/json'},
      body: JSON.stringify(payload)
    });
    if (!res.ok) { const d = await res.json(); showToast(d.error || '保存失败', true); return; }
    const panel = document.getElementById('panel-' + alias);
    if (panel) panel.classList.remove('open');
    openEditAlias = null;
    showToast('已保存');
    loadServers();
  } catch(e) { showToast(e.message, true); }
}

//  Delete 
async function deleteServer(alias) {
  if (!confirm('确认删除服务器 "' + alias + '"？')) return;
  try {
    const res = await fetch('/api/servers/' + encodeURIComponent(alias), {method: 'DELETE'});
    if (!res.ok && res.status !== 204) { const d = await res.json(); showToast(d.error || '删除失败', true); return; }
    showToast('已删除');
    loadServers();
  } catch(e) { showToast(e.message, true); }
}

//  Import from URL 
async function importFromURL() {
  const url = document.getElementById('import-url').value.trim();
  const statusEl = document.getElementById('import-status');
  const btn = document.getElementById('import-btn');
  if (!url) { statusEl.className = 'import-status err'; statusEl.textContent = '请输入 URL'; return; }
  statusEl.className = 'import-status loading'; statusEl.textContent = '正在获取';
  btn.disabled = true;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error('HTTP ' + res.status);
    const data = await res.json();
    if (!Array.isArray(data)) throw new Error('JSON 必须是数组格式');
    let added = 0, skipped = 0, errors = [];
    for (const item of data) {
      if (!item.alias || !item.host || !item.username || (!item.password && !item.key_path) || (item.key_pass && !item.key_path)) {
        errors.push(item.alias || '?'); continue;
      }
      const payload = {alias: item.alias, host: item.host, username: item.username};
      if (item.password) payload.password = item.password;
      if (item.key_path) payload.key_path = item.key_path;
      if (item.key_pass) payload.key_pass = item.key_pass;
      const r = await fetch('/api/servers', {
        method: 'POST', headers: {'Content-Type':'application/json'},
        body: JSON.stringify(payload)
      });
      if (r.ok || r.status === 201) { added++; }
      else { const d = await r.json(); if (d.error && d.error.includes('exist')) skipped++; else errors.push(item.alias); }
    }
    let msg = `导入完成：新增 ${added} 台`;
    if (skipped) msg += `，跳过 ${skipped} 台（已存在）`;
    if (errors.length) msg += `，失败 ${errors.length} 台`;
    statusEl.className = 'import-status ok'; statusEl.textContent = msg;
    showToast(msg);
    loadServers();
  } catch(e) {
    statusEl.className = 'import-status err'; statusEl.textContent = '导入失败：' + e.message;
    showToast('导入失败：' + e.message, true);
  } finally { btn.disabled = false; }
}

//  Utils 
function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

let toastTimer;
function showToast(msg, isError) {
  const el = document.getElementById('toast');
  document.getElementById('toast-msg').textContent = msg;
  el.className = 'show' + (isError ? ' error' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = ''; }, 3000);
}

// Enter on add form
['f-alias','f-host','f-user','f-pass'].forEach(id => {
  document.getElementById(id).addEventListener('keydown', e => { if (e.key === 'Enter') addServer(); });
});
document.getElementById('import-url').addEventListener('keydown', e => { if (e.key === 'Enter') importFromURL(); });

loadServers();
loadWorkspaces();

// ═══════════════════════════════════════════════════════
//  Projects page
// ═══════════════════════════════════════════════════════
let workspaces = [];
let activeWsPath = null;
let wsFilter = '';
let genvData = null;   // { path, groups, targets, raw }
let rawMode = false;
let openTargetKey = null;

// ── Load workspaces ──────────────────────────────────
async function loadWorkspaces() {
  try {
    const res = await fetch('/api/workspaces');
    workspaces = await res.json() || [];
    renderWorkspaces();
    // Update nav badge
    document.getElementById('nav-ws-count').textContent = workspaces.length;
  } catch(e) {
    showToast('加载工作区失败: ' + e.message, true);
  }
}

function renderWorkspaces() {
  const el = document.getElementById('ws-list');
  if (!workspaces.length) {
    el.innerHTML = `<div class="empty-state">
      <div class="empty-icon">📂</div>
      <p>暂无工作区记录<br>运行 <code style="color:var(--accent)">gscp init</code> 或 <code style="color:var(--accent)">gscp run</code> 后自动记录</p>
    </div>`;
    return;
  }
  const q = wsFilter.trim().toLowerCase();
  const visible = [];
  workspaces.forEach((ws, i) => {
    const p = ws.path || ws;
    const branch = ws.git_branch || '';
    if (!q || p.toLowerCase().includes(q) || branch.toLowerCase().includes(q)) visible.push({ ws, i });
  });

  if (!visible.length) {
    el.innerHTML = `<div class="empty-state">
      <div class="empty-icon">🔍</div>
      <p>没有匹配 "${esc(wsFilter.trim())}" 的工作区</p>
    </div>`;
    return;
  }

  el.innerHTML = visible.map(({ ws, i }, vi) => {
    const p = ws.path || ws;
    const parts = p.replace(/\\/g, '/').split('/');
    const name = parts[parts.length - 1] || p;
    const parent = parts.slice(0, -1).join('/') || '/';
    const isActive = p === activeWsPath;
    const gitBranch = ws.git_branch || '';
    return `<div class="ws-item${isActive ? ' active' : ''}" data-ws-idx="${i}" style="animation-delay:${vi*30}ms">
      <svg class="ws-icon" width="15" height="15" viewBox="0 0 15 15" fill="none"><path d="M1.5 3A1.5 1.5 0 013 1.5h3.379a1.5 1.5 0 011.06.44l.622.621A1.5 1.5 0 009.12 3H12A1.5 1.5 0 0113.5 4.5v7A1.5 1.5 0 0112 13H3a1.5 1.5 0 01-1.5-1.5V3z" stroke="currentColor" stroke-width="1.3"/></svg>
      <div class="ws-text">
        <div class="ws-name">${esc(name)}</div>
        <div class="ws-sub">${esc(parent)}${gitBranch ? ` <span style="color:var(--accent);margin-left:0.4rem">⎇ ${esc(gitBranch)}</span>` : ''}</div>
      </div>
      <button class="ws-remove" data-ws-remove-idx="${i}" title="从列表移除">
        <svg width="13" height="13" viewBox="0 0 13 13" fill="none"><path d="M2 2l9 9M11 2l-9 9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
      </button>
    </div>`;
  }).join('');

  // Attach click handlers via JS (avoids HTML attribute escaping issues with paths)
  el.querySelectorAll('.ws-item').forEach(item => {
    const idx = parseInt(item.dataset.wsIdx, 10);
    const ws = workspaces[idx];
    const path = ws.path || ws;
    item.addEventListener('click', () => selectWorkspace(path));
  });
  el.querySelectorAll('.ws-remove').forEach(btn => {
    const idx = parseInt(btn.dataset.wsRemoveIdx, 10);
    const ws = workspaces[idx];
    const path = ws.path || ws;
    btn.addEventListener('click', e => { e.stopPropagation(); removeWorkspace(path); });
  });
}

function filterWorkspaces(val) {
  wsFilter = val;
  renderWorkspaces();
}

async function selectWorkspace(path) {
  activeWsPath = path;
  renderWorkspaces();
  openTargetKey = null;
  rawMode = false;

  const editor = document.getElementById('genv-editor');
  const placeholder = document.getElementById('genv-placeholder');
  editor.style.display = 'none';
  placeholder.style.display = 'flex';

  try {
    const res = await fetch('/api/genv/read', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({path})
    });
    if (res.status === 404) {
      placeholder.style.display = 'flex';
      placeholder.innerHTML = `<svg width="40" height="40" viewBox="0 0 40 40" fill="none" opacity="0.18"><path d="M8 8.5A2.5 2.5 0 0110.5 6h10.086a2.5 2.5 0 011.768.732l7.914 7.914A2.5 2.5 0 0131 16.414V33.5A2.5 2.5 0 0128.5 36h-18A2.5 2.5 0 018 33.5v-25z" stroke="currentColor" stroke-width="2"/><path d="M21 6v9a1 1 0 001 1h9" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <p>该目录下没有 .genv 文件<br><span style="color:var(--text-3)">运行 <code style="color:var(--accent)">gscp init</code> 创建</span></p>`;
      return;
    }
    if (!res.ok) { const d = await res.json(); showToast(d.error || '读取失败', true); return; }
    genvData = await res.json();
    // Normalize nulls that Go may serialize for empty maps
    if (!genvData.groups)  genvData.groups  = {};
    if (!genvData.targets) genvData.targets = {};
    // Mark targets that use upload_pairs so the UI shows the correct mode
    Object.values(genvData.targets).forEach(t => {
      if (Array.isArray(t.upload_pairs) && t.upload_pairs.length > 0) {
        t._upload_mode = 'pairs';
      }
    });
    renderGenvEditor();
  } catch(e) {
    showToast('读取 .genv 失败: ' + e.message, true);
  }
}

async function removeWorkspace(path) {
  if (!confirm(`从列表中移除工作区？\n${path}\n\n（不会删除文件）`)) return;
  try {
    const res = await fetch('/api/workspaces', {
      method: 'DELETE',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({path})
    });
    if (res.ok || res.status === 204) {
      if (activeWsPath === path) {
        activeWsPath = null;
        genvData = null;
        document.getElementById('genv-editor').style.display = 'none';
        document.getElementById('genv-placeholder').style.display = 'flex';
        document.getElementById('genv-placeholder').innerHTML = `<svg width="40" height="40" viewBox="0 0 40 40" fill="none" opacity="0.18"><path d="M8 8.5A2.5 2.5 0 0110.5 6h10.086a2.5 2.5 0 011.768.732l7.914 7.914A2.5 2.5 0 0131 16.414V33.5A2.5 2.5 0 0128.5 36h-18A2.5 2.5 0 018 33.5v-25z" stroke="currentColor" stroke-width="2"/><path d="M21 6v9a1 1 0 001 1h9" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg><p>选择左侧工作区<br>查看并编辑 .genv</p>`;
      }
      showToast('已移除');
      loadWorkspaces();
    }
  } catch(e) { showToast(e.message, true); }
}

// ── Upload mode section renderer ────────────────────
// Returns HTML string for the upload mode tabs + fields inside a target card.
// isPairs = true when upload_pairs is present and non-empty.
function renderUploadModeSection(key, t) {
  const k = esc(key);
  const hasPairs = Array.isArray(t.upload_pairs) && t.upload_pairs.length > 0;
  // Determine active mode: if upload_pairs exists (even empty array stored), use pairs mode
  const isPairs = hasPairs || (t._upload_mode === 'pairs');

  const localPath = t.local_path || t.localPath || '';
  const localPathDisplay = Array.isArray(localPath) ? localPath.join('\n') : localPath;
  const toPath = t.to_path || t.toPath || '';

  // Build pairs rows HTML
  const pairs = (t.upload_pairs || []);
  const pairsRows = pairs.map((p, i) =>
    `<div class="pair-row" id="pair-row-${k}-${i}">
      <input id="tv-pair-from-${k}-${i}" value="${esc(p.from || '')}" oninput="markDirty()" placeholder="./frontend/dist" />
      <input id="tv-pair-to-${k}-${i}" value="${esc(p.to || '')}" oninput="markDirty()" placeholder="/var/www/frontend" />
      <button class="pair-remove" onclick="removePairRow('${k}','${esc(key)}',${i})" title="删除">
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M1.5 1.5l9 9M10.5 1.5l-9 9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
      </button>
    </div>`
  ).join('');

  return `
    <div style="margin-bottom:0.1rem">
      <div style="font-family:var(--mono);font-size:0.7rem;color:var(--text-3);letter-spacing:0.06em;margin-bottom:0.45rem">上传模式</div>
      <div class="upload-mode-tabs">
        <button class="upload-mode-tab${!isPairs ? ' active' : ''}" onclick="switchUploadMode('${esc(key)}','single')">单目标</button>
        <button class="upload-mode-tab${isPairs ? ' active' : ''}" onclick="switchUploadMode('${esc(key)}','pairs')">多路径映射</button>
      </div>
    </div>

    <!-- Single mode -->
    <div id="tv-mode-single-${k}" style="display:${isPairs ? 'none' : 'block'}">
      <div class="target-form-grid">
        <div class="field">
          <label>LOCAL_PATH（支持多个路径，每行一个）</label>
          <textarea id="tv-local-${k}" oninput="markDirty()" placeholder="./dist&#10;./index.html" style="min-height:72px">${esc(localPathDisplay)}</textarea>
        </div>
        <div class="field">
          <label>TO_PATH</label>
          <input id="tv-to-${k}" value="${esc(toPath)}" oninput="markDirty()" placeholder="/var/www/app" />
        </div>
      </div>
    </div>

    <!-- Pairs mode -->
    <div id="tv-mode-pairs-${k}" style="display:${isPairs ? 'block' : 'none'}">
      <div class="pairs-header">
        <span>本地路径 (from)</span>
        <span>远程路径 (to)</span>
        <span></span>
      </div>
      <div class="pairs-editor" id="tv-pairs-${k}">
        ${pairsRows || '<div style="font-family:var(--mono);font-size:0.78rem;color:var(--text-3);padding:0.2rem 0">暂无映射，点击下方添加</div>'}
      </div>
      <button class="add-pair-btn" onclick="addPairRow('${esc(key)}')">
        <svg width="11" height="11" viewBox="0 0 11 11" fill="none"><path d="M5.5 1v9M1 5.5h9" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>
        添加路径映射
      </button>
    </div>`;
}

function switchUploadMode(key, mode) {
  if (!genvData || !genvData.targets[key]) return;
  // Flush current form state before switching
  _collectTargetFields(key);
  const t = genvData.targets[key];
  if (mode === 'pairs') {
    t._upload_mode = 'pairs';
    if (!Array.isArray(t.upload_pairs)) t.upload_pairs = [];
    // Clear single-mode fields to avoid confusion in buildRaw
    t.local_path = undefined;
    t.to_path = undefined;
  } else {
    t._upload_mode = 'single';
    t.upload_pairs = undefined;
    if (!t.local_path) t.local_path = './dist';
    if (!t.to_path) t.to_path = '/var/www/app';
  }
  markDirty();
  renderVisual();
}

function addPairRow(key) {
  if (!genvData || !genvData.targets[key]) return;
  _collectTargetFields(key);
  const t = genvData.targets[key];
  if (!Array.isArray(t.upload_pairs)) t.upload_pairs = [];
  t.upload_pairs.push({ from: '', to: '' });
  markDirty();
  renderVisual();
  // Focus the new from-input
  const k = esc(key);
  const idx = t.upload_pairs.length - 1;
  setTimeout(() => {
    const el = document.getElementById(`tv-pair-from-${k}-${idx}`);
    if (el) el.focus();
  }, 50);
}

function removePairRow(k, key, idx) {
  if (!genvData || !genvData.targets[key]) return;
  _collectTargetFields(key);
  const t = genvData.targets[key];
  if (Array.isArray(t.upload_pairs)) t.upload_pairs.splice(idx, 1);
  markDirty();
  renderVisual();
}

// Collect fields for a single target key from the DOM (without full re-render)
function _collectTargetFields(key) {
  if (!genvData || !genvData.targets[key]) return;
  const k = esc(key);
  const t = genvData.targets[key];

  const aliasEl   = document.getElementById('tv-alias-' + k);
  const defaultEl = document.getElementById('tv-default-' + k);
  const cmdsEl    = document.getElementById('tv-cmds-' + k);
  if (aliasEl)   t.active_alias = aliasEl.value;
  if (defaultEl) t.is_default   = defaultEl.value === 'true';
  if (cmdsEl)    t.commands     = cmdsEl.value.split('\n').map(s => s.trim()).filter(Boolean);

  const isPairs = t._upload_mode === 'pairs' || (Array.isArray(t.upload_pairs) && t.upload_pairs !== undefined);
  if (isPairs) {
    // Collect pairs from DOM
    const pairs = [];
    let i = 0;
    while (true) {
      const fromEl = document.getElementById(`tv-pair-from-${k}-${i}`);
      const toEl   = document.getElementById(`tv-pair-to-${k}-${i}`);
      if (!fromEl) break;
      pairs.push({ from: fromEl.value.trim(), to: toEl ? toEl.value.trim() : '' });
      i++;
    }
    t.upload_pairs = pairs;
    t.local_path = undefined;
    t.to_path = undefined;
  } else {
    const localEl = document.getElementById('tv-local-' + k);
    const toEl    = document.getElementById('tv-to-' + k);
    if (localEl) {
      const paths = localEl.value.split('\n').map(s => s.trim()).filter(Boolean);
      t.local_path = paths.length === 1 ? paths[0] : paths;
    }
    if (toEl) t.to_path = toEl.value.trim();
    t.upload_pairs = undefined;
  }
}

// ── Render genv editor ───────────────────────────────
function renderGenvEditor() {
  if (!genvData) return;
  const placeholder = document.getElementById('genv-placeholder');
  const editor = document.getElementById('genv-editor');
  placeholder.style.display = 'none';
  editor.style.display = 'flex';

  document.getElementById('genv-path-label').textContent = genvData.path;

  // Sync raw textarea
  document.getElementById('genv-raw-textarea').value = genvData.raw;
  document.getElementById('raw-hint').textContent = '';
  document.getElementById('raw-hint').className = 'raw-hint';

  // Show correct mode
  applyRawMode();
  renderVisual();
}

function renderVisual() {
  if (!genvData) return;
  const container = document.getElementById('genv-visual');
  const { groups, targets } = genvData;
  const servers = window._servers || [];

  // Groups section — editable
  const groupEntries = Object.entries(groups || {});
  const groupRows = groupEntries.map(([name, members], gi) => {
    const memberChips = members.map((m, mi) =>
      `<span class="member-chip">${esc(m)}<button class="member-chip-remove" onclick="removeMember(${gi},'${esc(name)}',${mi})" title="移除">
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none"><path d="M1.5 1.5l7 7M8.5 1.5l-7 7" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      </button></span>`
    ).join('');
    return `<div class="group-row">
      <div class="group-name-col">${esc(name)}</div>
      <div class="group-members-col">
        ${memberChips}
        <div class="add-member-row">
          <select id="new-member-${gi}">
            <option value="">— 选择环境 —</option>
            ${Object.keys(targets||{}).filter(k=>!members.includes(k)).map(k=>`<option value="${esc(k)}">${esc(k)}</option>`).join('')}
          </select>
          <button class="btn btn-ghost btn-sm" style="padding:0.22rem 0.5rem" onclick="addMember(${gi},'${esc(name)}')">
            <svg width="11" height="11" viewBox="0 0 11 11" fill="none"><path d="M5.5 1v9M1 5.5h9" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>
          </button>
        </div>
      </div>
      <div class="group-row-actions">
        <button class="btn btn-danger-ghost btn-sm" style="padding:0.22rem 0.5rem" onclick="deleteGroup('${esc(name)}')" title="删除组">
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M1.5 1.5l9 9M10.5 1.5l-9 9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
        </button>
      </div>
    </div>`;
  }).join('');

  const groupsHtml = `
    <div class="section-label" style="margin-bottom:0.6rem">环境组</div>
    <div class="groups-editor" style="margin-bottom:1.5rem">
      ${groupRows || '<div style="padding:0.75rem 1rem;font-family:var(--mono);font-size:0.8rem;color:var(--text-3)">暂无分组</div>'}
      <div class="add-group-row">
        <input id="new-group-name" placeholder="新组名称" autocomplete="off"
          onkeydown="if(event.key==='Enter')addGroup()" />
        <button class="btn btn-ghost btn-sm" onclick="addGroup()">
          <svg width="11" height="11" viewBox="0 0 11 11" fill="none"><path d="M5.5 1v9M1 5.5h9" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>
          添加组
        </button>
      </div>
    </div>`;

  // Targets
  const targetKeys = Object.keys(targets || {}).sort();
  const targetCards = targetKeys.map(key => {
    const t = targets[key];
    const isOpen = key === openTargetKey;
    const isDefault = t.is_default || t.isDefault;
    const alias = t.active_alias || t.activeAlias || '';
    const commands = (t.commands || []).join('\n');

    const serverOptions = servers.map(s =>
      `<option value="${esc(s.alias)}" ${s.alias === alias ? 'selected' : ''}>${esc(s.alias)} (${esc(s.host)})</option>`
    ).join('');

    return `<div class="target-card${isDefault ? ' is-default' : ''}${isOpen ? ' open' : ''}" id="tc-${esc(key)}">
      <div class="target-header" onclick="toggleTarget('${esc(key)}')">
        <span class="target-key">${esc(key)}</span>
        ${isDefault ? '<span class="default-badge">default</span>' : ''}
        <span class="target-alias-tag">${alias ? '→ ' + esc(alias) : ''}</span>
        <svg class="target-chevron" width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M5 3l4 4-4 4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div class="target-body">
        <div class="target-body-inner">
          <div class="target-body-content">
            <div class="target-form-grid">
              <div class="field">
                <label>ACTIVE_ALIAS</label>
                <select id="tv-alias-${esc(key)}" onchange="markDirty()">
                  <option value="">— 手动输入 —</option>
                  ${serverOptions}
                </select>
              </div>
              <div class="field">
                <label>IS_DEFAULT</label>
                <select id="tv-default-${esc(key)}" onchange="markDirty()">
                  <option value="false" ${!isDefault ? 'selected' : ''}>false</option>
                  <option value="true" ${isDefault ? 'selected' : ''}>true</option>
                </select>
              </div>
            </div>

            <!-- Upload mode tabs -->
            ${renderUploadModeSection(key, t)}

            <div class="target-form-grid" style="margin-top:0.65rem">
              <div class="field" style="grid-column:1/-1">
                <label>COMMANDS（每行一条）</label>
                <textarea id="tv-cmds-${esc(key)}" oninput="markDirty()" placeholder="cd /var/www/app&#10;sudo systemctl restart app">${esc(commands)}</textarea>
              </div>
            </div>
            <div class="target-footer">
              <button class="btn btn-danger-ghost btn-sm" onclick="deleteTarget('${esc(key)}')">删除此环境</button>
            </div>
          </div>
        </div>
      </div>
    </div>`;
  }).join('');

  container.innerHTML = `
    ${groupsHtml}
    <div class="section-label" style="margin-bottom:0.75rem">环境列表</div>
    <div class="targets-list">${targetCards || '<div class="empty-state" style="padding:2rem 0"><p>暂无环境配置</p></div>'}</div>
    <div class="add-target-row">
      <input id="new-target-key" placeholder="新环境名称" autocomplete="off" onkeydown="if(event.key==='Enter')addTarget()" />
      <button class="btn btn-ghost btn-sm" onclick="addTarget()">
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M6 1v10M1 6h10" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>
        添加环境
      </button>
    </div>`;
}

function toggleTarget(key) {
  openTargetKey = openTargetKey === key ? null : key;
  // Flush edits from previously open card before re-rendering
  renderVisual();
}

function addTarget() {
  const input = document.getElementById('new-target-key');
  const key = input.value.trim();
  if (!key) { showToast('请输入环境名称', true); return; }
  if (genvData.targets[key]) { showToast(`环境 "${key}" 已存在`, true); return; }
  genvData.targets[key] = {
    active_alias: '',
    is_default: false,
    _upload_mode: 'single',
    local_path: './dist',
    to_path: '/var/www/app',
    commands: ['cd /var/www/app', 'sudo systemctl restart app']
  };
  openTargetKey = key;
  input.value = '';
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

function deleteTarget(key) {
  if (!confirm(`确认删除环境 "${key}"？`)) return;
  delete genvData.targets[key];
  if (openTargetKey === key) openTargetKey = null;
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

// ── Dirty / save ─────────────────────────────────────
let isDirty = false;
function markDirty() {
  isDirty = true;
  document.getElementById('genv-save-btn').style.outline = '2px solid var(--accent)';
}
function clearDirty() {
  isDirty = false;
  document.getElementById('genv-save-btn').style.outline = '';
}

async function saveGenv() {
  if (!genvData) return;
  let raw;
  if (rawMode) {
    raw = document.getElementById('genv-raw-textarea').value;
    // Validate
    try { JSON.parse(raw); } catch(e) {
      showToast('JSON 格式错误，无法保存', true);
      return;
    }
  } else {
    // Collect from visual form
    collectFromVisual();
    raw = buildRaw();
  }

  try {
    const res = await fetch('/api/genv/write', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({path: genvData.path, raw})
    });
    if (!res.ok) { const d = await res.json(); showToast(d.error || '保存失败', true); return; }
    genvData.raw = raw;
    showToast('已保存');
    clearDirty();
    // Re-parse to keep in sync
    await selectWorkspace(genvData.path);
  } catch(e) { showToast(e.message, true); }
}

function collectFromVisual() {
  if (!genvData) return;
  const keys = Object.keys(genvData.targets);
  keys.forEach(key => _collectTargetFields(key));
}

function buildRaw() {
  const out = {};
  // Always include groups key if it exists (even if empty)
  if (genvData.groups && Object.keys(genvData.groups).length) {
    out.groups = genvData.groups;
  }
  Object.entries(genvData.targets).forEach(([key, t]) => {
    const isPairs = Array.isArray(t.upload_pairs) && t.upload_pairs !== undefined && t._upload_mode !== 'single';
    if (isPairs) {
      out[key] = {
        active_alias: t.active_alias || t.activeAlias || '',
        is_default: !!(t.is_default || t.isDefault),
        upload_pairs: (t.upload_pairs || []).filter(p => p.from || p.to),
        commands: t.commands || []
      };
    } else {
      const localPath = t.local_path || t.localPath || '';
      out[key] = {
        active_alias: t.active_alias || t.activeAlias || '',
        is_default: !!(t.is_default || t.isDefault),
        local_path: Array.isArray(localPath) ? localPath : localPath,
        to_path: t.to_path || t.toPath || '',
        commands: t.commands || []
      };
    }
  });
  return JSON.stringify(out, null, 2);
}

function syncRawFromTargets() {
  // Collect visual form state before syncing (only when not in raw mode)
  if (!rawMode) collectFromVisual();
  genvData.raw = buildRaw();
  document.getElementById('genv-raw-textarea').value = genvData.raw;
}

// ── Raw mode toggle ──────────────────────────────────
function toggleRawMode() {
  if (!rawMode) {
    // Switch to raw: collect current visual state first
    collectFromVisual();
    document.getElementById('genv-raw-textarea').value = buildRaw();
  } else {
    // Switch to visual: try to parse raw
    const raw = document.getElementById('genv-raw-textarea').value;
    try {
      JSON.parse(raw);
      genvData.raw = raw;
    } catch(e) {
      showToast('JSON 格式错误，无法切换到可视化模式', true);
      return;
    }
  }
  rawMode = !rawMode;
  applyRawMode();
  if (!rawMode) renderVisual();
}

function applyRawMode() {
  const visual = document.getElementById('genv-visual');
  const raw = document.getElementById('genv-raw');
  const btn = document.getElementById('genv-toggle-raw');
  if (rawMode) {
    visual.style.display = 'none';
    raw.style.display = 'flex';
    btn.textContent = '可视化编辑';
    btn.classList.add('btn-primary');
    btn.classList.remove('btn-ghost');
  } else {
    visual.style.display = 'block';
    raw.style.display = 'none';
    btn.textContent = '原始 JSON';
    btn.classList.remove('btn-primary');
    btn.classList.add('btn-ghost');
  }
}

// Live JSON validation in raw mode
document.getElementById('genv-raw-textarea').addEventListener('input', function() {
  const hint = document.getElementById('raw-hint');
  try {
    JSON.parse(this.value);
    hint.textContent = '✓ 格式正确';
    hint.className = 'raw-hint ok';
    markDirty();
  } catch(e) {
    hint.textContent = '✗ ' + e.message;
    hint.className = 'raw-hint err';
  }
});

// ── Groups editing ───────────────────────────────────
function addGroup() {
  const input = document.getElementById('new-group-name');
  const name = input.value.trim();
  if (!name) { showToast('请输入组名称', true); return; }
  if (!genvData.groups) genvData.groups = {};
  if (genvData.groups[name] !== undefined) { showToast(`组 "${name}" 已存在`, true); return; }
  genvData.groups[name] = [];
  input.value = '';
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

function deleteGroup(name) {
  if (!confirm(`确认删除组 "${name}"？`)) return;
  if (genvData.groups) delete genvData.groups[name];
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

function addMember(groupIdx, groupName) {
  const select = document.getElementById('new-member-' + groupIdx);
  const member = select ? select.value : '';
  if (!member) { showToast('请选择一个环境', true); return; }
  if (!genvData.groups) genvData.groups = {};
  if (!genvData.groups[groupName]) genvData.groups[groupName] = [];
  if (genvData.groups[groupName].includes(member)) {
    showToast(`"${member}" 已在组中`, true); return;
  }
  genvData.groups[groupName].push(member);
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

function removeMember(groupIdx, groupName, memberIdx) {
  if (!genvData.groups || !genvData.groups[groupName]) return;
  genvData.groups[groupName].splice(memberIdx, 1);
  syncRawFromTargets();
  renderVisual();
  markDirty();
}

// Cache servers for alias dropdowns
window._servers = [];
(async () => {
  try {
    const res = await fetch('/api/servers');
    window._servers = await res.json() || [];
  } catch(_) {}
})();

// ═══════════════════════════════════════════════════════
//  Scan local .genv files  (SSE streaming)
// ═══════════════════════════════════════════════════════
let _scanES = null;  // active EventSource

function cancelScan() {
  if (_scanES) { _scanES.close(); _scanES = null; }
  document.getElementById('scan-overlay').classList.remove('show');
}

async function scanLocalGenv() {
  // Must have scan roots configured
  if (!settingsData.scan_roots || !settingsData.scan_roots.length) {
    // Switch to settings page and highlight the warning
    const settingsBtn = document.querySelector('.nav-item[onclick*="settings"]');
    if (settingsBtn) switchPage('settings', settingsBtn);
    // Briefly pulse the warning banner to draw attention
    setTimeout(() => {
      const w = document.getElementById('scan-roots-warning');
      if (w) {
        w.style.transition = 'box-shadow 0.15s';
        w.style.boxShadow = '0 0 0 3px oklch(62% 0.22 25 / 0.5)';
        setTimeout(() => { w.style.boxShadow = ''; }, 1200);
        w.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    }, 120);
    return;
  }

  // Reset UI
  const overlay   = document.getElementById('scan-overlay');
  const dirEl     = document.getElementById('scan-current-dir');
  const listEl    = document.getElementById('scan-found-list');
  const statsEl   = document.getElementById('scan-stats');
  const cancelBtn = document.getElementById('scan-cancel-btn');
  const spinner   = overlay.querySelector('.scan-spinner');

  dirEl.textContent   = '准备中…';
  listEl.innerHTML    = '';
  statsEl.textContent = '';
  cancelBtn.style.display = '';
  spinner.style.display   = '';
  overlay.classList.add('show');

  const foundPaths = [];

  // Close any previous stream
  if (_scanES) { _scanES.close(); _scanES = null; }

  _scanES = new EventSource('/api/scan');

  _scanES.addEventListener('scanning', e => {
    const { dir } = JSON.parse(e.data);
    dirEl.textContent = dir;
  });

  _scanES.addEventListener('found', e => {
    const { path } = JSON.parse(e.data);
    foundPaths.push(path);
    const item = document.createElement('div');
    item.className = 'scan-found-item';
    item.textContent = path;
    listEl.appendChild(item);
    listEl.scrollTop = listEl.scrollHeight;
    statsEl.textContent = `已发现 ${foundPaths.length} 个`;
  });

  _scanES.addEventListener('done', async e => {
    _scanES.close(); _scanES = null;
    const { count } = JSON.parse(e.data);

    dirEl.textContent = '扫描完成';
    spinner.style.display = 'none';
    cancelBtn.textContent = '关闭';

    if (!count) {
      statsEl.textContent = '未找到任何 .genv 文件';
      return;
    }

    // Add all found paths to workspaces
    const existing = new Set(await fetch('/api/workspaces').then(r => r.json()).catch(() => []));
    let added = 0;
    for (const p of foundPaths) {
      if (!existing.has(p)) {
        await fetch('/api/workspaces/add', {
          method: 'POST',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify({path: p})
        });
        added++;
      }
    }
    statsEl.textContent = `共 ${count} 个，新增 ${added} 个工作区`;
    showToast(`扫描完成：发现 ${count} 个，新增 ${added} 个`);
    loadWorkspaces();
  });

  _scanES.addEventListener('error', e => {
    _scanES.close(); _scanES = null;
    // If readyState is CLOSED it means the stream ended (after 'done'), ignore
    dirEl.textContent = '连接中断';
    statsEl.textContent = '扫描出错，请重试';
    spinner.style.display = 'none';
    cancelBtn.textContent = '关闭';
  });
}

// ═══════════════════════════════════════════════════════
//  Settings page
// ═══════════════════════════════════════════════════════
let settingsData = { skip_dirs: [], scan_roots: [] };

async function loadSettings() {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) return;
    settingsData = await res.json();
    if (!settingsData.skip_dirs) settingsData.skip_dirs = [];
    if (!settingsData.scan_roots) settingsData.scan_roots = [];
    renderSettings();
  } catch(e) {
    showToast('加载配置失败: ' + e.message, true);
  }
}

function renderSettings() {
  // Skip dirs
  const tagsEl = document.getElementById('skip-tags-container');
  if (tagsEl) {
    tagsEl.innerHTML = settingsData.skip_dirs.map((d, i) =>
      `<span class="skip-tag">${esc(d)}<button class="skip-tag-remove" onclick="removeSkipDir(${i})" title="移除">
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none"><path d="M1.5 1.5l7 7M8.5 1.5l-7 7" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      </button></span>`
    ).join('');
  }
  // Scan roots
  const rootsEl = document.getElementById('scan-roots-container');
  const warningEl = document.getElementById('scan-roots-warning');
  const hasRoots = settingsData.scan_roots && settingsData.scan_roots.length > 0;
  if (warningEl) warningEl.style.display = hasRoots ? 'none' : 'flex';
  if (rootsEl) {
    if (!hasRoots) {
      rootsEl.innerHTML = '';
    } else {
      rootsEl.innerHTML = settingsData.scan_roots.map((r, i) =>
        `<div class="root-item"><span>${esc(r)}</span>
          <button class="btn btn-danger-ghost btn-sm" style="padding:0.2rem 0.45rem" onclick="removeScanRoot(${i})" title="移除">
            <svg width="11" height="11" viewBox="0 0 11 11" fill="none"><path d="M1.5 1.5l8 8M9.5 1.5l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
          </button>
        </div>`
      ).join('');
    }
  }
}

function addSkipDir() {
  const input = document.getElementById('new-skip-dir');
  const val = input.value.trim();
  if (!val) return;
  if (settingsData.skip_dirs.includes(val)) { showToast(`"${val}" 已在列表中`, true); return; }
  settingsData.skip_dirs.push(val);
  input.value = '';
  renderSettings();
}

function removeSkipDir(idx) {
  settingsData.skip_dirs.splice(idx, 1);
  renderSettings();
}

function resetSkipDirs() {
  if (!confirm('恢复为默认跳过目录列表？')) return;
  // Empty means backend uses defaults
  settingsData.skip_dirs = [];
  renderSettings();
  showToast('已重置为默认列表（保存后生效）');
}

function addScanRoot() {
  const input = document.getElementById('new-scan-root');
  const val = input.value.trim();
  if (!val) return;
  if (settingsData.scan_roots.includes(val)) { showToast(`路径已存在`, true); return; }
  settingsData.scan_roots.push(val);
  input.value = '';
  renderSettings();
}

function removeScanRoot(idx) {
  settingsData.scan_roots.splice(idx, 1);
  renderSettings();
}

async function saveSettings() {
  try {
    const res = await fetch('/api/settings', {
      method: 'PUT',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify(settingsData)
    });
    if (!res.ok) { const d = await res.json(); showToast(d.error || '保存失败', true); return; }
    showToast('配置已保存');
  } catch(e) { showToast(e.message, true); }
}

loadSettings();


import { BrowseTransport } from '/scram/browse-transport.js';
import { loadSettings, saveSettings } from '/scram/settings.js';

// Deliberately non-fatal: if the Scramjet vendor bundles failed to
// initialize, the node picker (loadHosts() below) must still work — it does
// not touch Controller at all — so the failure is caught here and only
// surfaces when the user actually tries to connect, in connect() below.
let Controller;
try {
  Controller = window.$scramjetController.Controller;
  if (!Controller) throw new Error('window.$scramjetController.Controller is undefined');
} catch (err) {
  console.error('Scramjet controller bundle failed to initialize:', err);
}

// This shell runs on the browse origin, which has its own credential — the
// panel's cookies are host-only and never reach here. See
// internal/panel/browse_origin.go for why the two must stay separate.
function readCsrfCookie() {
  const match = document.cookie.match(/(?:^|; )kejilion_browse_csrf=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : '';
}

// ---------------- Connect flow ----------------
const overlay = document.getElementById('connect-overlay');
const hostListEl = document.getElementById('host-list');
const statusEl = document.getElementById('status');
const browserEl = document.getElementById('browser');
const egressLabel = document.getElementById('egress-label');

let hostId = null;
let controller = null;
let transport = null;

// ---------------- Exit IP info ----------------
// Looks up the browsing session's actual egress IP/geolocation by requesting
// ipinfo.io *through the chosen node's Agent* (the same /api/v1/browse/fetch
// path proxied pages use), so it reports the egress node's exit, not this
// browser's own IP. Fetched once per session and memoized.
const IPINFO_URL = 'https://ipinfo.io/?token=99f2b5b7a72cd2';
let ipInfoPromise = null;

async function fetchThroughEgress(url) {
  const res = await fetch('/api/v1/browse/fetch', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', Origin: location.origin, 'X-CSRF-Token': readCsrfCookie() },
    body: JSON.stringify({ hostId, url, method: 'GET' }),
  });
  if (!res.ok) throw new Error('出口请求失败: HTTP ' + res.status);
  const meta = await res.json();
  if (meta.statusCode < 200 || meta.statusCode >= 300) {
    throw new Error('上游返回状态码 ' + meta.statusCode);
  }
  return JSON.parse(atob(meta.body));
}

function fetchIpInfo() {
  if (!ipInfoPromise) {
    ipInfoPromise = fetchThroughEgress(IPINFO_URL).catch((err) => {
      ipInfoPromise = null;
      throw err;
    });
  }
  return ipInfoPromise;
}

// The Scramjet rewriting engine is driven entirely by a Service Worker, and
// browsers only expose navigator.serviceWorker in a secure context (HTTPS,
// or a loopback host). Over plain HTTP the API is simply absent, which
// previously surfaced as a misleading "this browser does not support Service
// Worker" — the browser supports it fine, the page just is not allowed to
// use it here. Distinguish the two so the message is actionable. The server
// enforces the same rule (see browseSecureContext in internal/panel/browse.go);
// this check exists so the UI can explain it before any request is made.
const HTTPS_REQUIRED_MESSAGE =
  '浏览功能需要通过 HTTPS 访问。该功能依赖 Service Worker，浏览器只在 HTTPS（或 localhost）下允许注册。请为面板配置 HTTPS 后再使用。';

function isSecureContextAvailable() {
  return typeof window !== 'undefined' && window.isSecureContext === true;
}

async function registerServiceWorker() {
  if (!isSecureContextAvailable()) {
    throw new Error(HTTPS_REQUIRED_MESSAGE);
  }
  if (!('serviceWorker' in navigator)) {
    throw new Error('此浏览器不支持 Service Worker');
  }
  const registration = await navigator.serviceWorker.register('/scramjet-sw.js', { scope: '/' });
  await navigator.serviceWorker.ready;
  return registration;
}

async function loadHosts() {
  // Fail closed before listing anything: without a secure context no node can
  // work, so offering a picker would only lead to a dead end after the click.
  if (!isSecureContextAvailable()) {
    hostListEl.innerHTML =
      '<div id="host-list-empty" style="color:#c5221f">' + escapeHtml(HTTPS_REQUIRED_MESSAGE) + '</div>';
    return;
  }
  hostListEl.innerHTML = '<div id="host-list-empty">正在加载可用节点…</div>';
  try {
    // Not /api/v1/cluster/hosts: that lives on the panel origin, which this
    // shell has no session for. This is the browse origin's own projection of
    // the same list — see handleBrowseHosts in internal/panel/browse_origin.go.
    const res = await fetch('/api/v1/browse/hosts', { credentials: 'same-origin' });
    if (!res.ok) throw new Error('获取节点列表失败: HTTP ' + res.status);
    // The endpoint returns a paged envelope ({items, total}), not a bare array.
    const payload = await res.json();
    const items = Array.isArray(payload) ? payload : (payload && payload.items) || [];
    const hosts = items.filter((h) => h.browseAvailable);
    if (hosts.length === 0) {
      // Two distinct reasons a paired host can be absent, and the operator
      // needs to be told which one applies: a light node has no Agent at all
      // (nothing to grant), while a Panel node needs cluster.browse.fetch
      // granted at pairing time.
      hostListEl.innerHTML =
        '<div id="host-list-empty">没有可用的浏览出口节点。轻量节点没有 Agent，不能作为出口；' +
        '完整节点需要在配对时勾选浏览权限，请到“集群”里重新配对并授予。</div>';
      return;
    }
    hostListEl.innerHTML = '';
    for (const host of hosts) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'host-option';
      btn.innerHTML =
        '<span><span class="host-option-name">' + escapeHtml(host.name || host.id) + '</span>' +
        (host.isLocal ? ' <span class="host-option-meta">(本机)</span>' : '') + '</span>' +
        '<span class="host-option-meta">' + (host.browseWSAvailable ? 'HTTP + WS' : '仅 HTTP') + '</span>';
      btn.addEventListener('click', () => connect(host));
      hostListEl.appendChild(btn);
    }
  } catch (err) {
    hostListEl.innerHTML = '';
    statusEl.textContent = '加载节点列表失败: ' + err.message;
    statusEl.className = 'error';
  }
}

async function connect(host) {
  if (!Controller) {
    statusEl.textContent = 'Scramjet 渲染引擎未能加载，无法开始浏览（节点列表本身工作正常）。请刷新页面重试，若持续出现请查看浏览器控制台。';
    statusEl.className = 'error';
    return;
  }
  statusEl.textContent = '正在启动浏览会话…';
  statusEl.className = '';
  for (const btn of hostListEl.querySelectorAll('.host-option')) btn.disabled = true;

  try {
    const startRes = await fetch('/api/v1/browse/sessions', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Origin: location.origin, 'X-CSRF-Token': readCsrfCookie() },
      body: JSON.stringify({ hostId: host.id }),
    });
    if (!startRes.ok) throw new Error('会话启动失败: HTTP ' + startRes.status);
    const started = await startRes.json();
    hostId = started.hostId;

    const registration = await registerServiceWorker();
    transport = new BrowseTransport({ hostId, adblockEnabled: loadSettings(localStorage).adblockEnabled });
    fetchIpInfo().catch(() => {});
    controller = new Controller({ serviceworker: registration.active, transport });
    await controller.wait();
    overlay.style.display = 'none';
    browserEl.style.display = 'flex';
    egressLabel.textContent = host.name || host.id;
    resetTabsForNewSession();
    createTab(null);
  } catch (err) {
    statusEl.textContent = '连接失败: ' + err.message;
    statusEl.className = 'error';
    for (const btn of hostListEl.querySelectorAll('.host-option')) btn.disabled = false;
  }
}

loadHosts();

document.getElementById('new-session-btn').addEventListener('click', () => {
  browserEl.style.display = 'none';
  overlay.style.display = 'flex';
  statusEl.textContent = '';
  statusEl.className = '';
  ipInfoPromise = null;
  loadHosts();
});

// ---------------- Tabs ----------------
const tabsEl = document.getElementById('tabs');
const pagesEl = document.getElementById('pages');
const addressBar = document.getElementById('address-bar');
const btnBack = document.getElementById('btn-back');
const btnFwd = document.getElementById('btn-fwd');
const btnReload = document.getElementById('btn-reload');
const GLOBE_SVG = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>';
const SPINNER_SVG = '<svg class="spinner-svg" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><circle cx="12" cy="12" r="9" opacity="0.25"></circle><path d="M21 12a9 9 0 0 0-9-9"></path></svg>';
const SEARCH_SVG = '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>';

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

const SHORTCUTS_STORAGE_KEY = 'kpanelBrowserShortcuts';
const DEFAULT_SHORTCUTS = [
  { name: 'Google', url: 'https://www.google.com' },
  { name: 'YouTube', url: 'https://www.youtube.com' },
  { name: 'GitHub', url: 'https://github.com' },
  { name: 'Wikipedia', url: 'https://www.wikipedia.org' },
];

function loadShortcuts() {
  try {
    const raw = localStorage.getItem(SHORTCUTS_STORAGE_KEY);
    if (raw === null) return DEFAULT_SHORTCUTS.slice();
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : DEFAULT_SHORTCUTS.slice();
  } catch (_) {
    return DEFAULT_SHORTCUTS.slice();
  }
}

function saveShortcuts(list) {
  localStorage.setItem(SHORTCUTS_STORAGE_KEY, JSON.stringify(list));
}

let tabs = [];
let activeId = null;
let tabSeq = 0;

function resetTabsForNewSession() {
  tabs.forEach((t) => {
    if (t.frame) t.frame.element.remove();
    if (t.blankEl) t.blankEl.remove();
    if (t.chipEl) t.chipEl.remove();
  });
  tabs = [];
  activeId = null;
}

function hostnameOf(url) {
  try { return new URL(url).hostname; } catch (e) { return url; }
}

function normalizeInput(raw) {
  let v = (raw || '').trim();
  if (!v) return null;
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(v)) v = 'https://' + v;
  try { return new URL(v).toString(); } catch (e) { return null; }
}

function looksLikeHost(raw) {
  const v = raw.trim();
  if (!v || /\s/.test(v)) return false;
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(v)) return true;
  const host = v.split(/[/?#]/)[0].split(':')[0];
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return true;
  if (/^localhost$/i.test(host)) return true;
  return /^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$/.test(host);
}

function normalizeOrSearch(raw) {
  const v = (raw || '').trim();
  if (!v) return null;
  if (looksLikeHost(v)) {
    const direct = normalizeInput(v);
    if (direct) return direct;
  }
  return 'https://www.google.com/search?q=' + encodeURIComponent(v);
}

function createTab(url) {
  const tab = { id: ++tabSeq, url: null, title: '新标签页', frame: null, blankEl: null, loading: false };
  tabs.push(tab);
  renderChip(tab);
  if (url) {
    mountFrame(tab, url);
  } else {
    mountBlank(tab);
  }
  activateTab(tab.id);
  return tab;
}

function renderChip(tab) {
  const chip = document.createElement('div');
  chip.className = 'tab';
  chip.draggable = true;
  chip.innerHTML =
    '<span class="tab-favicon"></span>' +
    '<span class="tab-title"></span>' +
    '<span class="tab-close" title="关闭标签页">&times;</span>';
  chip.addEventListener('mousedown', (e) => {
    if (e.target.classList.contains('tab-close')) return;
    activateTab(tab.id);
  });
  chip.querySelector('.tab-close').addEventListener('click', (e) => {
    e.stopPropagation();
    closeTab(tab.id);
  });
  chip.addEventListener('dragstart', (e) => {
    draggedTab = tab;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', String(tab.id));
    setTimeout(() => chip.classList.add('dragging'), 0);
  });
  chip.addEventListener('dragend', () => {
    chip.classList.remove('dragging');
    draggedTab = null;
    syncTabOrderFromDom();
  });
  tabsEl.appendChild(chip);
  tab.chipEl = chip;
  tab.faviconEl = chip.querySelector('.tab-favicon');
  tab.titleEl = chip.querySelector('.tab-title');
  updateChip(tab);
}

let draggedTab = null;

function dragAfterElement(container, x) {
  const chips = [...container.querySelectorAll('.tab:not(.dragging)')];
  return chips.reduce(
    (closest, child) => {
      const box = child.getBoundingClientRect();
      const offset = x - box.left - box.width / 2;
      if (offset < 0 && offset > closest.offset) return { offset, element: child };
      return closest;
    },
    { offset: Number.NEGATIVE_INFINITY, element: null }
  ).element;
}

function syncTabOrderFromDom() {
  const ordered = [...tabsEl.children]
    .map((chip) => tabs.find((t) => t.chipEl === chip))
    .filter(Boolean);
  if (ordered.length === tabs.length) tabs = ordered;
}

tabsEl.addEventListener('dragover', (e) => {
  if (!draggedTab || !draggedTab.chipEl) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  const afterEl = dragAfterElement(tabsEl, e.clientX);
  if (afterEl == null) tabsEl.appendChild(draggedTab.chipEl);
  else tabsEl.insertBefore(draggedTab.chipEl, afterEl);
});

tabsEl.addEventListener('drop', (e) => e.preventDefault());

function updateChip(tab) {
  if (!tab.chipEl) return;
  tab.titleEl.textContent = tab.title || '新标签页';
  tab.chipEl.title = tab.url || '新标签页';
  tab.faviconEl.innerHTML = '';
  tab.faviconEl.className = 'tab-favicon';
  if (tab.loading) {
    tab.faviconEl.innerHTML = SPINNER_SVG;
  } else if (tab.faviconUrl) {
    const img = document.createElement('img');
    img.src = tab.faviconUrl;
    img.width = 14;
    img.height = 14;
    img.style.borderRadius = '3px';
    img.addEventListener('error', () => { tab.faviconEl.innerHTML = tab.url ? GLOBE_SVG : ''; });
    tab.faviconEl.appendChild(img);
  } else if (tab.url) {
    tab.faviconEl.innerHTML = GLOBE_SVG;
  }
}

function mountFrame(tab, url) {
  const el = document.createElement('iframe');
  el.className = 'page-frame';
  el.title = 'proxied site';
  pagesEl.appendChild(el);
  const frame = controller.createFrame(el);
  tab.frame = frame;
  tab.url = url;
  tab.title = hostnameOf(url);
  tab.loading = true;
  tab.faviconUrl = null;
  el.addEventListener('load', () => onFrameLoad(tab));
  frame.go(url);
  updateChip(tab);
  if (tab.id === activeId) syncToolbar(tab);
}

const PIN_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"></path><circle cx="12" cy="10" r="3"></circle></svg>';
const COPY_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>';
const CHECK_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>';

function renderIpCard(cardEl) {
  const ipEl = cardEl.querySelector('.ntp-ipcard-ip');
  const locEl = cardEl.querySelector('.ntp-ipcard-loc');
  const orgEl = cardEl.querySelector('.ntp-ipcard-org');

  function paintData(data) {
    cardEl.className = 'ntp-ipcard';
    ipEl.innerHTML =
      '<span>' + escapeHtml(data.ip || '—') + '</span>' +
      '<button type="button" class="ntp-ipcard-copy" title="复制">' + COPY_SVG + '</button>';
    locEl.textContent = [data.city, data.region, data.country].filter(Boolean).join(', ');
    orgEl.textContent = data.org || '';

    const copyBtn = ipEl.querySelector('.ntp-ipcard-copy');
    copyBtn.addEventListener('click', () => {
      navigator.clipboard.writeText(data.ip || '').then(() => {
        copyBtn.innerHTML = CHECK_SVG;
        setTimeout(() => { copyBtn.innerHTML = COPY_SVG; }, 1200);
      });
    });
  }

  function paintError(message) {
    cardEl.className = 'ntp-ipcard error';
    ipEl.textContent = message;
    locEl.textContent = '';
    orgEl.textContent = '';
  }

  fetchIpInfo().then(paintData, (err) => paintError(err.message));
}

function paintClocks() {
  const now = new Date();
  const time = now.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false });
  const datePart = now.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' });
  const weekdayPart = now.toLocaleDateString('zh-CN', { weekday: 'long' });
  const date = datePart + ' ' + weekdayPart;
  document.querySelectorAll('.ntp-clock-time').forEach((el) => { el.textContent = time; });
  document.querySelectorAll('.ntp-clock-date').forEach((el) => { el.textContent = date; });
}
paintClocks();
setInterval(paintClocks, 1000);

const modalBackdrop = document.getElementById('modal-backdrop');
const shortcutNameInput = document.getElementById('shortcut-name-input');
const shortcutUrlInput = document.getElementById('shortcut-url-input');
let onShortcutAdd = null;

function openAddShortcutModal(onAdd) {
  onShortcutAdd = onAdd;
  shortcutNameInput.value = '';
  shortcutUrlInput.value = '';
  modalBackdrop.classList.add('open');
  shortcutNameInput.focus();
}

function closeAddShortcutModal() {
  modalBackdrop.classList.remove('open');
  onShortcutAdd = null;
}

document.getElementById('shortcut-modal-cancel').addEventListener('click', closeAddShortcutModal);
modalBackdrop.addEventListener('click', (e) => {
  if (e.target === modalBackdrop) closeAddShortcutModal();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && modalBackdrop.classList.contains('open')) closeAddShortcutModal();
});
document.getElementById('shortcut-modal-save').addEventListener('click', () => {
  const name = shortcutNameInput.value.trim();
  const normalized = normalizeInput(shortcutUrlInput.value);
  if (!name) { shortcutNameInput.focus(); return; }
  if (!normalized) { shortcutUrlInput.focus(); return; }
  const cb = onShortcutAdd;
  closeAddShortcutModal();
  if (cb) cb({ name, url: normalized });
});

const TILE_COLORS = ['#4285F4', '#EA4335', '#34A853', '#673AB7', '#FF7043', '#00ACC1', '#5C6BC0', '#F4511E'];
function colorForName(name) {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  return TILE_COLORS[hash % TILE_COLORS.length];
}

function mountBlank(tab) {
  const el = document.createElement('div');
  el.className = 'blank-page';
  el.innerHTML =
    '<div class="ntp">' +
      '<div class="ntp-clock-time"></div>' +
      '<div class="ntp-clock-date"></div>' +
      '<form class="ntp-search">' +
        '<span class="ntp-search-icon">' + SEARCH_SVG + '</span>' +
        '<input class="ntp-search-input" placeholder="搜索或输入网址" autocomplete="off" spellcheck="false">' +
        '<span class="ntp-search-divider"></span>' +
        '<span class="ntp-search-engine">' + GLOBE_SVG + '</span>' +
      '</form>' +
      '<div class="ntp-shortcuts"></div>' +
      '<label class="ntp-adblock"><input type="checkbox" class="adblock-toggle">屏蔽广告域名</label>' +
    '</div>' +
    '<div class="ntp-ipcard loading">' +
      '<div class="ntp-ipcard-head">' + PIN_SVG + '<span>IP 位置</span></div>' +
      '<div class="ntp-ipcard-ip">获取中…</div>' +
      '<div class="ntp-ipcard-loc"></div>' +
      '<div class="ntp-ipcard-org"></div>' +
    '</div>';
  pagesEl.appendChild(el);
  tab.blankEl = el;

  paintClocks();
  renderIpCard(el.querySelector('.ntp-ipcard'));

  const searchForm = el.querySelector('.ntp-search');
  const searchInput = el.querySelector('.ntp-search-input');
  searchForm.addEventListener('submit', (e) => {
    e.preventDefault();
    navigateActive(searchInput.value);
  });

  const shortcutsEl = el.querySelector('.ntp-shortcuts');
  function renderShortcuts() {
    shortcutsEl.innerHTML = '';
    loadShortcuts().forEach((sc, i) => {
      const label = sc.name || sc.url;
      const tile = document.createElement('button');
      tile.type = 'button';
      tile.className = 'ntp-tile';
      tile.innerHTML =
        '<span class="ntp-tile-icon" style="background:' + colorForName(label) + '">' + escapeHtml((label[0] || '?').toUpperCase()) + '</span>' +
        '<span class="ntp-tile-label">' + escapeHtml(label) + '</span>' +
        '<span class="ntp-tile-remove" title="移除">&times;</span>';
      tile.addEventListener('click', () => navigateActive(sc.url));
      tile.querySelector('.ntp-tile-remove').addEventListener('click', (e) => {
        e.stopPropagation();
        const updated = loadShortcuts();
        updated.splice(i, 1);
        saveShortcuts(updated);
        renderShortcuts();
      });
      shortcutsEl.appendChild(tile);
    });

    const addTile = document.createElement('button');
    addTile.type = 'button';
    addTile.className = 'ntp-tile ntp-tile-add';
    addTile.innerHTML = '<span class="ntp-tile-icon">+</span><span class="ntp-tile-label">添加快捷方式</span>';
    addTile.addEventListener('click', () => {
      openAddShortcutModal((sc) => {
        const updated = loadShortcuts();
        updated.push(sc);
        saveShortcuts(updated);
        renderShortcuts();
      });
    });
    shortcutsEl.appendChild(addTile);
  }
  renderShortcuts();

  const adblockToggle = el.querySelector('.adblock-toggle');
  adblockToggle.checked = loadSettings(localStorage).adblockEnabled;
  adblockToggle.addEventListener('change', () => {
    saveSettings(localStorage, { adblockEnabled: adblockToggle.checked });
    if (transport) transport.adblockEnabled = adblockToggle.checked;
  });
}

function onFrameLoad(tab) {
  tab.loading = false;
  try {
    const decoded = decodeFrameUrl(tab.frame);
    if (decoded) tab.url = decoded;
    const docTitle = tab.frame.element.contentDocument && tab.frame.element.contentDocument.title;
    if (docTitle) tab.title = docTitle;
    if (!tab.faviconUrl) tab.faviconUrl = findFaviconUrl(tab);
  } catch (e) { /* best effort */ }
  updateChip(tab);
  if (tab.id === activeId) { syncAddressBar(tab); syncToolbar(tab); }
}

function decodeFrameUrl(frame) {
  try {
    const href = frame.element.contentWindow.location.href;
    const path = new URL(href).pathname;
    if (path.startsWith(frame.prefix)) {
      return decodeURIComponent(path.slice(frame.prefix.length));
    }
  } catch (e) { /* cross-origin or torn-down frame */ }
  return null;
}

function findFaviconUrl(tab) {
  try {
    const doc = tab.frame.element.contentDocument;
    const link = doc.querySelector('link[rel~="icon" i]');
    if (link && link.href) return link.href;
  } catch (e) { /* best effort */ }
  return null;
}

function activateTab(id) {
  activeId = id;
  tabs.forEach((t) => {
    const isActive = t.id === id;
    if (t.chipEl) t.chipEl.classList.toggle('active', isActive);
    if (t.frame) t.frame.element.classList.toggle('active', isActive);
    if (t.blankEl) t.blankEl.classList.toggle('active', isActive);
  });
  const tab = tabs.find((t) => t.id === id);
  if (!tab) return;
  syncAddressBar(tab);
  syncToolbar(tab);
  if (!tab.frame) addressBar.focus();
  if (tab.chipEl) tab.chipEl.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

function syncAddressBar(tab) {
  addressBar.value = tab.url || '';
}

function syncToolbar(tab) {
  const hasFrame = !!tab.frame;
  btnBack.disabled = !hasFrame;
  btnFwd.disabled = !hasFrame;
  btnReload.disabled = !hasFrame;
}

function closeTab(id) {
  const idx = tabs.findIndex((t) => t.id === id);
  if (idx === -1) return;
  const tab = tabs[idx];
  if (tab.frame) tab.frame.element.remove();
  if (tab.blankEl) tab.blankEl.remove();
  if (tab.chipEl) tab.chipEl.remove();
  tabs.splice(idx, 1);

  if (tabs.length === 0) {
    createTab(null);
    return;
  }
  if (activeId === id) {
    const next = tabs[idx] || tabs[idx - 1];
    activateTab(next.id);
  }
}

function activeTab() {
  return tabs.find((t) => t.id === activeId) || null;
}

function navigateActive(rawUrl) {
  const tab = activeTab();
  if (!tab) return;
  const url = normalizeOrSearch(rawUrl);
  if (!url) return;
  if (tab.frame) {
    tab.loading = true;
    tab.faviconUrl = null;
    tab.url = url;
    tab.title = hostnameOf(url);
    updateChip(tab);
    tab.frame.go(url);
    syncAddressBar(tab);
  } else {
    if (tab.blankEl) { tab.blankEl.remove(); tab.blankEl = null; }
    mountFrame(tab, url);
    if (tab.frame) tab.frame.element.classList.add('active');
    syncAddressBar(tab);
  }
}

document.getElementById('new-tab-btn').addEventListener('click', () => createTab(null));

addressBar.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' || e.keyCode === 13) {
    e.preventDefault();
    navigateActive(addressBar.value);
    addressBar.blur();
  }
});
document.getElementById('btn-go').addEventListener('click', () => navigateActive(addressBar.value));

let addressBarGainingFocus = false;
addressBar.addEventListener('mousedown', () => {
  addressBarGainingFocus = document.activeElement !== addressBar;
});
addressBar.addEventListener('focus', () => addressBar.select());
addressBar.addEventListener('mouseup', (e) => {
  if (addressBarGainingFocus) {
    e.preventDefault();
    addressBarGainingFocus = false;
  }
});

btnBack.addEventListener('click', () => {
  const tab = activeTab();
  if (tab && tab.frame) tab.frame.back();
});
btnFwd.addEventListener('click', () => {
  const tab = activeTab();
  if (tab && tab.frame) tab.frame.forward();
});
btnReload.addEventListener('click', () => {
  const tab = activeTab();
  if (tab && tab.frame) tab.frame.reload();
});

document.addEventListener('keydown', (e) => {
  if (browserEl.style.display === 'none') return;
  const mod = e.ctrlKey || e.metaKey;
  if (!mod) return;
  if (e.key === 't') { e.preventDefault(); createTab(null); }
  else if (e.key === 'w') { e.preventDefault(); if (activeId !== null) closeTab(activeId); }
  else if (e.key === 'l') { e.preventDefault(); addressBar.focus(); addressBar.select(); }
});

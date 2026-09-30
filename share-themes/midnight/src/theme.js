// 午夜终端 / Midnight console — an htop-style heat table with tmux-style panes.
// Protocol 2, no dependencies. Every snapshot string is inserted as a text node, never as HTML.
const $ = id => document.getElementById(id)
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false && kid !== '') node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
const remember = (key, fallback) => { try { return localStorage.getItem(key) || fallback } catch { return fallback } }
const store = (key, value) => { try { localStorage.setItem(key, value) } catch { /* sandboxed: keep in memory */ } }

const NS = 'http://www.w3.org/2000/svg'
// Distribution glyph from the host's bare path data, tinted toward the terminal ink.
function glyph(host) {
  const node = h('span', 'os'); node.style.setProperty('--os', host.system.accent || 'var(--dim)'); node.title = host.system.label || host.os
  node.setAttribute('aria-hidden', 'true')
  if (host.system.path) { const icon = document.createElementNS(NS, 'svg'); icon.setAttribute('viewBox', '0 0 24 24'); const path = document.createElementNS(NS, 'path'); path.setAttribute('d', host.system.path); icon.append(path); node.append(icon) }
  else if (host.system.image) { const img = h('img'); img.src = host.system.image; img.alt = ''; node.append(img) }
  else node.append('>')
  return node
}
function where(host, labels) {
  const flag = host.location.flag ? h('img', 'flag') : null
  if (flag) { flag.src = host.location.flag; flag.alt = ''; flag.decoding = 'async'; flag.setAttribute('aria-hidden', 'true') }
  return h('span', 'sub', flag || (host.location.countryCode ? `[${host.location.countryCode}] ` : ''), h('span', null, [host.location.text, host.os].filter(Boolean).join(' · ') || labels.unknownPlace))
}

const GLYPH = { online: '●', degraded: '▲', offline: '✕', pending: '○' }
const ORDER = ['online', 'degraded', 'offline', 'pending']
let snap = null, query = '', filter = 'all', sortKey = '', sortDir = 1, view = remember('midnight-view', 'list')
const open = new Set()
const level = ratio => ratio === null || ratio === undefined ? 'na' : ratio >= 0.9 ? 'hot' : ratio >= 0.75 ? 'warm' : 'ok'
const known = value => value && value !== '—'
const ascii = (ratio, cells = 12) => ratio === null || ratio === undefined ? '·'.repeat(cells) : '█'.repeat(Math.round(ratio * cells)) + '░'.repeat(cells - Math.round(ratio * cells))
const sortValue = (host, key) => ({ name: host.name.toLowerCase(), cpu: host.cpu.ratio ?? -1, memory: host.memory.ratio ?? -1, disk: host.disk.ratio ?? -1,
  traffic: host.traffic.ratio ?? -1, net: host.network.down.bytesPerSecond ?? -1, uptime: host.uptimeSeconds ?? -1 }[key])

function heat(reading, name) {
  const td = h('td', `heat ${level(reading.ratio)}`)
  td.dataset.label = name
  td.style.setProperty('--r', (reading.ratio ?? 0).toFixed(3))
  td.append(h('span', null, reading.text, level(reading.ratio) === 'hot' ? '!' : ''))
  return td
}

function details(host, labels) {
  const list = h('dl', 'kv')
  const add = (term, value) => { if (known(value)) list.append(h('div', null, h('dt', null, term), h('dd', null, value))) }
  add(labels.system, [host.os, host.architecture].filter(Boolean).join(' / '))
  if (host.cores) add(labels.cores, host.cores)
  add(labels.memory, host.memory.totalText !== '—' ? `${host.memory.usedText} / ${host.memory.totalText}` : '')
  add(labels.disk, host.disk.totalText !== '—' ? `${host.disk.usedText} / ${host.disk.totalText}` : '')
  add(host.traffic.monthly ? labels.monthly : labels.cumulative, `↓ ${host.traffic.received}  ↑ ${host.traffic.sent}`)
  add(labels.expiry, host.expiresOn); add(labels.price, host.price); add(labels.remaining, host.remaining)
  const node = h('div', 'details', list)
  if (host.traffic.hint) node.append(h('p', 'comment', `# ${host.traffic.hint}`))
  return node
}

function table(hosts, labels) {
  const columns = [['name', labels.server], ['cpu', labels.cpu], ['memory', labels.memory], ['disk', labels.disk], ['traffic', labels.traffic], ['net', labels.network], ['uptime', labels.uptime]]
  const head = h('tr', null, h('th', 'st', h('span', 'sr', labels.view)))
  for (const [key, label] of columns) {
    const th = h('th', `c-${key}`); th.scope = 'col'
    th.setAttribute('aria-sort', sortKey === key ? (sortDir > 0 ? 'ascending' : 'descending') : 'none')
    const button = h('button', 'sort', label, sortKey === key ? (sortDir > 0 ? ' ▲' : ' ▼') : '')
    button.type = 'button'
    button.addEventListener('click', () => {
      // First click sorts one way, second reverses, third restores the panel's own order.
      const first = key === 'name' ? 1 : -1
      if (sortKey !== key) { sortKey = key; sortDir = first } else if (sortDir === first) sortDir = -first; else sortKey = ''
      render(); document.querySelector(`th.c-${key} button`)?.focus()
    })
    th.append(button); head.append(th)
  }
  head.append(h('th', 'more', h('span', 'sr', labels.details)))
  const body = h('tbody')
  for (const host of hosts) {
    const t = host.traffic, expanded = open.has(host.id)
    const toggle = h('button', 'expand', expanded ? '−' : '+'); toggle.type = 'button'
    toggle.setAttribute('aria-expanded', String(expanded)); toggle.setAttribute('aria-label', `${labels.details}: ${host.name}`)
    toggle.addEventListener('click', () => { expanded ? open.delete(host.id) : open.add(host.id); render(); document.querySelector(`[data-host="${CSS.escape(host.id)}"] .expand`)?.focus() })
    const tr = h('tr', `host s-${host.state}${expanded ? ' is-open' : ''}`,
      h('td', 'st', h('span', null, GLYPH[host.state] || '○'), h('span', 'sr', host.stateLabel)),
      h('td', 'name', h('strong', null, glyph(host), h('span', null, host.name)), where(host, labels)),
      heat(host.cpu, labels.cpu), heat(host.memory, labels.memory), heat(host.disk, labels.disk),
      heat({ text: t.monthly && t.percent ? t.percent : `↓${t.received}`, ratio: t.monthly && t.percent ? t.ratio : null }, t.monthly ? labels.monthly : labels.traffic),
      h('td', 'net', h('span', null, '↓ ', host.network.down.text), h('span', null, '↑ ', host.network.up.text)),
      h('td', 'up', known(host.uptime) ? host.uptime : '—'), h('td', 'more', toggle))
    tr.dataset.host = host.id
    tr.querySelector('.net').dataset.label = labels.network; tr.querySelector('.up').dataset.label = labels.uptime
    body.append(tr)
    if (expanded) { const cell = h('td', null, details(host, labels)); cell.colSpan = 10; body.append(h('tr', 'detail-row', cell)) }
  }
  return h('table', 'grid', h('thead', null, head), body)
}

function pane(host, labels) {
  const line = (label, reading) => h('div', `line ${level(reading.ratio)}`, h('span', 'k', label.padEnd(4, ' ')), h('span', 'bar', `[${ascii(reading.ratio)}]`), h('b', null, reading.text, level(reading.ratio) === 'hot' ? '!' : ''))
  const t = host.traffic
  return h('section', `pane s-${host.state}`,
    h('header', null, h('span', 'st', GLYPH[host.state] || '○', ' ', host.stateLabel), h('h2', null, glyph(host), h('span', null, host.name))),
    h('p', 'pane-where', where(host, labels)),
    line(labels.cpu, host.cpu), line(labels.memory, host.memory), line(labels.disk, host.disk),
    t.monthly && t.percent ? line(labels.traffic, { text: t.percent, ratio: t.ratio }) : null,
    h('p', 'io', `↓ ${host.network.down.text}  ↑ ${host.network.up.text}  ⏱ ${known(host.uptime) ? host.uptime : '—'}`),
    details(host, labels))
}

function keyButton(text, pressed, onClick, id) {
  const button = h('button', null, text); button.type = 'button'; button.setAttribute('aria-pressed', String(pressed))
  button.addEventListener('click', () => { onClick(); document.getElementById(id)?.focus() })
  button.id = id
  return button
}

function render() {
  const { data, labels, locale, mode } = snap
  const root = document.documentElement
  root.lang = locale; root.dataset.mode = mode === 'light' ? 'light' : 'dark'
  document.title = data.title
  $('title').textContent = data.title
  $('desc').textContent = data.description; $('desc').hidden = !data.description
  $('q').placeholder = labels.search; $('findLabel').textContent = labels.search

  const counts = Object.fromEntries(ORDER.map(state => [state, data.hosts.filter(host => host.state === state).length]))
  const blocks = $('blocks')
  blocks.setAttribute('aria-label', `${labels.online} ${data.counts.online} / ${data.counts.total}`)
  blocks.replaceChildren(...data.hosts.map(host => { const cell = h('span', `b s-${host.state}`, GLYPH[host.state]); cell.title = `${host.name} · ${host.stateLabel}`; return cell }))
  $('tally').replaceChildren(...ORDER.filter(state => counts[state]).map(state => h('span', `s-${state}`, `${GLYPH[state]} ${counts[state]} ${data.states[state]}`)),
    h('span', 'dim', `${labels.total} ${data.counts.total}`))
  $('worth').textContent = data.value.groups.length ? `${labels.remaining}  ${data.value.groups.map(group => group.text).join('  ')}  (${data.value.included}/${data.counts.total} ${labels.covered})` : ''
  $('worth').hidden = !data.value.groups.length

  $('filters').replaceChildren(...[['all', labels.all], ['online', labels.online], ['degraded', labels.attention], ['offline', labels.offline]]
    .map(([key, text], index) => keyButton(`${index + 1} ${text}`, filter === key, () => { filter = key; render() }, `f-${key}`)))
  $('filters').setAttribute('aria-label', labels.filter)
  $('views').replaceChildren(keyButton(labels.list, view === 'list', () => { view = 'list'; store('midnight-view', view); render() }, 'v-list'),
    keyButton(labels.card, view === 'card', () => { view = 'card'; store('midnight-view', view); render() }, 'v-card'))
  $('views').setAttribute('aria-label', labels.view)

  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => (filter === 'all' || host.state === filter) && `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
  if (sortKey) hosts.sort((a, b) => { const x = sortValue(a, sortKey), y = sortValue(b, sortKey); return (x < y ? -1 : x > y ? 1 : 0) * sortDir })
  $('list').className = `out is-${view}`
  $('list').replaceChildren(!hosts.length ? h('p', 'empty', `> ${labels.empty}`) : view === 'list' ? table(hosts, labels) : h('div', 'panes', hosts.map(host => pane(host, labels))))
  const date = new Date(data.generatedAt)
  $('foot').textContent = `# ${labels.updated} ${Number.isFinite(date.getTime()) ? date.toLocaleString(locale) : '—'} · KPanel`
}

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  snap = msg; render()
})
$('q').addEventListener('input', event => { query = event.target.value; if (snap) render() })
// Keyboard: "/" focuses search, 1–4 switch the state filter (ignored while typing).
addEventListener('keydown', event => {
  if (!snap || event.target instanceof HTMLInputElement || event.ctrlKey || event.metaKey || event.altKey) return
  if (event.key === '/') { event.preventDefault(); $('q').focus() }
  const key = ['all', 'online', 'degraded', 'offline'][Number(event.key) - 1]
  if (key) { filter = key; render() }
})
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

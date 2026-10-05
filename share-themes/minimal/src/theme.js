// 简约看板 / Clear board — a bright status page. Protocol 2, no dependencies.
// Every snapshot string is inserted as a text node, never as HTML.
const $ = id => document.getElementById(id)
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false && kid !== '') node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
const svg = (tag, attrs) => { const node = document.createElementNS('http://www.w3.org/2000/svg', tag); for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v); return node }
const remember = (key, fallback) => { try { return localStorage.getItem(key) || fallback } catch { return fallback } }
const store = (key, value) => { try { localStorage.setItem(key, value) } catch { /* sandboxed: keep in memory */ } }

// System mark: the host sends bare path data, so the badge is ours to draw and colour.
function systemBadge(host, size) {
  const badge = h('span', `badge badge-${size}`)
  badge.style.setProperty('--os', host.system.accent || 'var(--idle)')
  badge.title = host.system.label || host.os
  if (host.system.path) { const icon = svg('svg', { viewBox: '0 0 24 24', 'aria-hidden': 'true' }); icon.append(svg('path', { d: host.system.path })); badge.append(icon) }
  else if (host.system.image) { const img = h('img'); img.src = host.system.image; img.alt = ''; badge.append(img) }
  else badge.append(h('b', null, (host.system.label || host.os || '?').slice(0, 1).toUpperCase()))
  if (host.location.flag) { const flag = h('img', 'flag'); flag.src = host.location.flag; flag.alt = ''; flag.decoding = 'async'; badge.append(flag) }
  badge.setAttribute('aria-hidden', 'true')
  return badge
}

const STATES = ['online', 'degraded', 'offline', 'pending']
let snap = null, query = '', filter = 'all', view = remember('clear-view', 'card')
const level = ratio => ratio === null || ratio === undefined ? 'na' : ratio >= 0.9 ? 'hot' : ratio >= 0.75 ? 'warm' : 'ok'
const known = value => value && value !== '—'

function meter(label, reading, extra) {
  const bar = h('span', 'bar'); const fill = h('span', 'fill')
  fill.style.inlineSize = `${Math.round((reading.ratio ?? 0) * 100)}%`
  bar.append(fill)
  const node = h('div', `meter ${level(reading.ratio)}`, h('span', 'meter-name', label), bar, h('span', 'meter-value', reading.text, level(reading.ratio) === 'hot' ? ' !' : ''))
  if (extra) node.append(h('span', 'meter-extra', extra))
  return node
}

function trafficMeter(host, labels) {
  const t = host.traffic
  const title = t.monthly ? labels.monthly : labels.cumulative
  const reading = { text: t.percent || '', ratio: t.monthly && t.percent ? t.ratio : null }
  const node = h('div', `traffic ${t.tone || 'normal'}`)
  const top = h('div', 'traffic-top', h('span', null, title), h('span', 'traffic-io', `↓ ${t.received}  ↑ ${t.sent}`))
  node.append(top)
  if (reading.text) {
    const bar = h('span', 'bar'); const fill = h('span', 'fill'); fill.style.inlineSize = `${Math.round(reading.ratio * 100)}%`; bar.append(fill)
    node.append(h('div', 'traffic-bar', bar, h('b', null, reading.text)))
  }
  if (t.hint) node.append(h('p', 'hint', t.hint))
  return node
}

function chips(host, labels) {
  const list = h('ul', 'chips')
  const add = (label, value) => { if (known(value)) list.append(h('li', null, h('span', null, label), ' ', h('b', null, value))) }
  add(labels.uptime, host.uptime)
  if (host.collected) add(labels.network, `↓ ${host.network.down.text} ↑ ${host.network.up.text}`)
  add(labels.expiry, host.expiresOn); add(labels.price, host.price); add(labels.remaining, host.remaining)
  return list
}

function identity(host, labels) {
  const place = [host.location.text, host.os].filter(Boolean).join(' · ')
  return h('div', 'who',
    h('h2', null, host.name),
    h('p', null, host.location.countryCode && !host.location.flag ? h('span', 'cc', host.location.countryCode) : null, place || labels.unknownPlace))
}

function card(host, labels) {
  return h('article', `card s-${host.state}`,
    h('header', null, systemBadge(host, 'lg'), identity(host, labels), h('span', `pill s-${host.state}`, host.stateLabel)),
    h('div', 'meters', meter(labels.cpu, host.cpu, host.cores ? `${host.cores} ${labels.cores}` : ''), meter(labels.memory, host.memory, host.memory.totalText !== '—' ? `${host.memory.usedText} / ${host.memory.totalText}` : ''),
      meter(labels.disk, host.disk, host.disk.totalText !== '—' ? `${host.disk.usedText} / ${host.disk.totalText}` : '')),
    trafficMeter(host, labels), chips(host, labels))
}

function row(host, labels) {
  const t = host.traffic
  return h('article', `row s-${host.state}`,
    h('span', `pill s-${host.state}`, host.stateLabel), h('div', 'row-id', systemBadge(host, 'sm'), identity(host, labels)),
    meter(labels.cpu, host.cpu), meter(labels.memory, host.memory), meter(labels.disk, host.disk),
    meter(t.monthly ? labels.monthly : labels.traffic, { text: t.percent || `↓ ${t.received}`, ratio: t.percent ? t.ratio : null }),
    h('div', 'row-meta', h('span', null, known(host.uptime) ? host.uptime : ''), host.collected ? h('span', null, `↓ ${host.network.down.text}`) : null))
}

function ring(counts) {
  const node = $('ring'); node.replaceChildren(svg('circle', { cx: 60, cy: 60, r: 50, class: 'track' }))
  const total = Math.max(1, counts.online + counts.degraded + counts.offline + counts.pending), length = 2 * Math.PI * 50
  let offset = 0
  for (const state of STATES) {
    const part = counts[state] / total * length
    if (part > 0) node.append(svg('circle', { cx: 60, cy: 60, r: 50, class: `seg s-${state}`, 'stroke-dasharray': `${Math.max(0, part - 2)} ${length}`, 'stroke-dashoffset': -offset, transform: 'rotate(-90 60 60)' }))
    offset += part
  }
}

// The host only relays these three requests; how they look and where they sit is this theme's own business.
const ask = (action, extra) => parent.postMessage({ source: 'kpanel-share-theme', type: 'action', action, ...extra }, '*')
function controls(labels, mode) {
  const next = mode === 'light' ? 'dark' : 'light', nextLabel = next === 'light' ? labels.lightMode : labels.darkMode
  const make = (icon, text, run) => { const node = h('button', null, h('span', null, icon), text); node.type = 'button'; node.firstChild.setAttribute('aria-hidden', 'true'); node.addEventListener('click', run); return node }
  $('ctl').replaceChildren(make('↻', labels.refresh, () => ask('refresh')), make(next === 'light' ? '☀' : '☾', nextLabel, () => ask('set-mode', { mode: next })), make('↩', labels.useDefault, () => ask('use-default')))
}

function render() {
  const { data, labels, locale, mode } = snap
  const root = document.documentElement
  root.lang = locale; root.dataset.mode = mode === 'dark' ? 'dark' : 'light'
  document.title = data.title
  controls(labels, root.dataset.mode)
  $('kicker').textContent = labels.fleet
  const date = new Date(data.generatedAt)
  const stamp = Number.isFinite(date.getTime()) ? date.toLocaleString(locale) : '—'
  $('stamp').textContent = `${labels.updated} ${stamp}`
  $('title').textContent = data.title
  $('desc').textContent = data.description; $('desc').hidden = !data.description
  $('searchLabel').textContent = labels.search; $('q').placeholder = labels.search

  const counts = Object.fromEntries(STATES.map(state => [state, data.hosts.filter(host => host.state === state).length]))
  ring(counts)
  $('ratio').textContent = `${data.counts.online}/${data.counts.total}`
  $('healthLabel').textContent = labels.online
  $('legend').replaceChildren(...STATES.filter(state => counts[state]).map(state => h('li', `s-${state}`, h('i'), data.states[state], h('b', null, counts[state]))))

  const worth = $('worth')
  worth.hidden = !data.value.groups.length
  if (data.value.groups.length) {
    worth.replaceChildren(h('span', 'worth-label', labels.remaining), ...data.value.groups.map(group => h('strong', null, group.text)),
      h('p', null, `${data.value.included}/${data.counts.total} ${labels.covered}${data.value.excluded ? ` · ${data.value.excluded} ${labels.excluded}` : ''}`), h('p', null, labels.valueHint))
  }

  const segments = [['all', labels.all, data.counts.total], ['online', labels.online, counts.online], ['degraded', labels.attention, counts.degraded], ['offline', labels.offline, counts.offline]]
  $('filters').replaceChildren(...segments.map(([key, text, count]) => toggle(text, filter === key, () => { filter = key; render() }, count)))
  $('filters').setAttribute('aria-label', labels.filter)
  $('views').replaceChildren(toggle(labels.card, view === 'card', () => setView('card')), toggle(labels.list, view === 'list', () => setView('list')))
  $('views').setAttribute('aria-label', labels.view)

  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => (filter === 'all' || host.state === filter) && `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
  const list = $('hosts'); list.className = `hosts is-${view}`
  list.replaceChildren(...(hosts.length ? hosts.map(host => view === 'card' ? card(host, labels) : row(host, labels)) : [h('p', 'empty', labels.empty)]))
  $('foot').textContent = `KPanel · ${labels.updated} ${stamp}`
}

function toggle(text, pressed, onClick, count) {
  const button = h('button', null, text, count !== undefined ? h('span', 'count', count) : null)
  button.type = 'button'; button.setAttribute('aria-pressed', String(pressed))
  button.addEventListener('click', event => { const index = [...event.currentTarget.parentNode.children].indexOf(event.currentTarget), group = event.currentTarget.parentNode.id; onClick(); document.getElementById(group)?.children[index]?.focus() })
  return button
}
function setView(next) { view = next; store('clear-view', next); render() }

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  snap = msg; render()
})
$('q').addEventListener('input', event => { query = event.target.value; if (snap) render() })
// chrome: 'self' — this theme draws the whole page, including refresh, light/dark and the way back to the default style.
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2, chrome: 'self' }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

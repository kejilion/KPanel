// 星图 / Orbit map — a dark world map with a card or list roster below it.
// Protocol 2, no dependencies. Every snapshot string is inserted as a text node, never as HTML.
const $ = id => document.getElementById(id)
const NS = 'http://www.w3.org/2000/svg'
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false && kid !== '') node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
const s = (tag, attrs, ...kids) => { const node = document.createElementNS(NS, tag); for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v); node.append(...kids); return node }
const remember = (key, fallback) => { try { return localStorage.getItem(key) || fallback } catch { return fallback } }
const store = (key, value) => { try { localStorage.setItem(key, value) } catch { /* sandboxed: keep in memory */ } }

function flagImage(location, cls = 'flag') {
  if (!location.flag) return null
  const img = h('img', cls); img.src = location.flag; img.alt = ''; img.decoding = 'async'; img.setAttribute('aria-hidden', 'true')
  return img
}
// The host sends bare path data; the glyph is drawn here in the distribution colour, lifted for dark glass.
function systemGlyph(host) {
  const node = h('span', 'os'); node.style.setProperty('--os', host.system.accent || 'var(--soft)'); node.title = host.system.label || host.os
  node.setAttribute('aria-hidden', 'true')
  if (host.system.path) node.append(s('svg', { viewBox: '0 0 24 24' }, s('path', { d: host.system.path })))
  else if (host.system.image) { const img = h('img'); img.src = host.system.image; img.alt = ''; node.append(img) }
  else node.append(h('b', null, (host.system.label || host.os || '?').slice(0, 1).toUpperCase()))
  return node
}

const RANK = { offline: 3, degraded: 2, pending: 1, online: 0 }
const GLYPH = { online: '●', degraded: '▲', offline: '✕', pending: '○' }
let snap = null, query = '', filter = 'all', region = '', view = remember('orbit-view', 'card')
const level = ratio => ratio === null || ratio === undefined ? 'na' : ratio >= 0.9 ? 'hot' : ratio >= 0.75 ? 'warm' : 'ok'
const known = value => value && value !== '—'

// Land is static: draw it once. Equirectangular: x = longitude + 180, y = 90 - latitude.
;(() => {
  const land = $('land'), dots = self.ORBIT_LAND || [], group = s('g', { class: 'dots' })
  for (let i = 0; i < dots.length; i += 2) if (dots[i] > -60) group.append(s('circle', { cx: (dots[i + 1] + 180).toFixed(1), cy: (90 - dots[i]).toFixed(1), r: 0.62 }))
  const grid = s('g', { class: 'grid' })
  for (let lat = -30; lat <= 60; lat += 30) grid.append(s('line', { x1: 0, x2: 360, y1: 90 - lat, y2: 90 - lat }))
  land.append(grid, group)
})()

function regions(hosts) {
  const map = new Map()
  for (const host of hosts) {
    const { countryCode: code, latitude, longitude } = host.location
    if (!code || latitude === null || longitude === null) continue
    const entry = map.get(code) || { code, latitude, longitude, name: host.location.country || code, hosts: [], worst: 'online' }
    entry.hosts.push(host)
    if (RANK[host.state] > RANK[entry.worst]) entry.worst = host.state
    map.set(code, entry)
  }
  return [...map.values()]
}

// Merge regions whose markers would overlap at the current map size, then place each
// label on whichever side is free (or hide it; the count and accessible name remain).
function clusters(entries, width, height) {
  const out = []
  const at = entry => [(entry.longitude + 180) / 360 * width, (90 - entry.latitude - 8) / 142 * height]
  for (const entry of [...entries].sort((a, b) => b.hosts.length - a.hosts.length)) {
    const [x, y] = at(entry)
    const near = out.find(group => Math.hypot(group.x - x, group.y - y) < 28)
    if (near) { near.entries.push(entry); if (RANK[entry.worst] > RANK[near.worst]) near.worst = entry.worst }
    else out.push({ x, y, worst: entry.worst, entries: [entry] })
  }
  for (const group of out) {
    const crowded = side => out.some(other => other !== group && Math.abs(other.y - group.y) < 24 && (side > 0 ? other.x - group.x : group.x - other.x) > 0 && Math.abs(other.x - group.x) < 90)
    group.side = !crowded(1) ? 'right' : !crowded(-1) ? 'left' : 'none'
  }
  return out
}

function drawMarkers(data, labels) {
  const markers = $('markers'), map = $('map'), all = regions(data.hosts)
  markers.setAttribute('aria-label', labels.map)
  markers.replaceChildren(...clusters(all, map.clientWidth || 1, map.clientHeight || 1).map(group => {
    const codes = group.entries.map(entry => entry.code), key = codes.join(',')
    const hosts = group.entries.flatMap(entry => entry.hosts)
    const active = region === key
    const button = h('button', `marker s-${group.worst} tag-${group.side}${active ? ' is-active' : ''}${region && !active ? ' is-dim' : ''}`,
      h('span', 'coin', group.entries.slice(0, 2).map(entry => entry.hosts[0].location.flag ? flagImage(entry.hosts[0].location) : h('span', 'noflag', entry.code)),
        h('span', 'count', hosts.length)), h('span', 'tag', codes.join(' · ')))
    button.type = 'button'
    button.style.left = `${group.x}px`; button.style.top = `${group.y}px`
    button.dataset.key = key
    button.setAttribute('aria-pressed', String(active))
    button.setAttribute('aria-label', `${group.entries.map(entry => entry.name).join(', ')}: ${hosts.length} · ${data.states[group.worst]}`)
    button.title = hosts.map(host => `${host.name} · ${host.stateLabel}`).join('\n')
    button.addEventListener('click', () => { region = active ? '' : key; render(); $('markers').querySelector(`[data-key="${CSS.escape(key)}"]`)?.focus() })
    return button
  }))
  return all.length
}

function ring(label, reading, metric) {
  const ratio = reading.ratio, length = 2 * Math.PI * 26
  const graphic = s('svg', { viewBox: '0 0 64 64', 'aria-hidden': 'true' },
    s('circle', { cx: 32, cy: 32, r: 26, class: 'track' }),
    s('circle', { cx: 32, cy: 32, r: 26, class: 'arc', transform: 'rotate(-90 32 32)', 'stroke-dasharray': `${(ratio ?? 0) * length} ${length}` }))
  return h('div', `ring m-${metric} ${level(ratio)}`, h('div', 'ring-art', graphic, h('b', null, reading.text)), h('span', null, label, level(ratio) === 'hot' ? ' ▲' : ''))
}

function trafficLine(host, labels) {
  const t = host.traffic, node = h('div', `traffic ${t.tone || 'normal'}`)
  node.append(h('div', 'traffic-head', h('span', null, t.monthly ? labels.monthly : labels.cumulative), h('b', null, t.percent)))
  if (t.monthly && t.percent) { const bar = h('span', 'bar'); const fill = h('span'); fill.style.inlineSize = `${Math.round(t.ratio * 100)}%`; bar.append(fill); node.append(bar) }
  node.append(h('div', 'io', h('span', null, `↓ ${t.received}`), h('span', null, `↑ ${t.sent}`)))
  if (t.hint) node.append(h('p', 'hint', t.hint))
  return node
}

function status(host) { return h('span', `status s-${host.state}`, h('i', null, GLYPH[host.state] || '○'), host.stateLabel) }
function place(host, labels) { return h('p', 'place', flagImage(host.location) || (host.location.countryCode ? h('span', 'cc', host.location.countryCode) : null), h('span', null, [host.location.text, host.os].filter(Boolean).join(' · ') || labels.unknownPlace)) }

function card(host, labels) {
  const facts = h('dl', 'facts')
  const add = (term, value) => { if (known(value)) facts.append(h('div', null, h('dt', null, term), h('dd', null, value))) }
  add(labels.uptime, host.uptime)
  if (host.collected) add(labels.network, `↓ ${host.network.down.text}  ↑ ${host.network.up.text}`)
  add(labels.expiry, host.expiresOn); add(labels.price, host.price); add(labels.remaining, host.remaining)
  return h('article', `card s-${host.state}`,
    h('header', null, systemGlyph(host), h('div', 'who', h('h2', null, host.name), place(host, labels)), status(host)),
    h('div', 'rings', ring(labels.cpu, host.cpu, 'cpu'), ring(labels.memory, host.memory, 'mem'), ring(labels.disk, host.disk, 'disk')),
    trafficLine(host, labels), facts)
}

function mini(label, reading, metric) {
  const bar = h('span', 'bar'); const fill = h('span'); fill.style.inlineSize = `${Math.round((reading.ratio ?? 0) * 100)}%`; bar.append(fill)
  return h('div', `mini m-${metric} ${level(reading.ratio)}`, h('span', 'mini-label', label), bar, h('b', null, reading.text))
}

function row(host, labels) {
  const t = host.traffic
  return h('article', `row s-${host.state}`,
    status(host), h('div', 'row-id', systemGlyph(host), h('div', 'who', h('h2', null, host.name), place(host, labels))),
    mini(labels.cpu, host.cpu, 'cpu'), mini(labels.memory, host.memory, 'mem'), mini(labels.disk, host.disk, 'disk'),
    mini(t.monthly ? labels.monthly : labels.traffic, { text: t.percent || `↓ ${t.received}`, ratio: t.percent ? t.ratio : null }, 'net'),
    h('span', 'row-up', known(host.uptime) ? host.uptime : '—'))
}

function button(text, pressed, onClick, extra) {
  const node = h('button', null, text, extra)
  node.type = 'button'; node.setAttribute('aria-pressed', String(pressed))
  node.addEventListener('click', event => { const parent = event.currentTarget.parentNode, index = [...parent.children].indexOf(event.currentTarget); onClick(); parent.children[index]?.focus() })
  return node
}

function render() {
  const { data, labels, locale, mode } = snap
  const root = document.documentElement
  root.lang = locale; root.dataset.mode = mode === 'light' ? 'light' : 'dark'
  document.title = data.title
  $('title').textContent = data.title
  $('desc').textContent = data.description; $('desc').hidden = !data.description
  const date = new Date(data.generatedAt), stamp = Number.isFinite(date.getTime()) ? date.toLocaleString(locale) : '—'
  $('stamp').textContent = `${labels.updated} ${stamp}`
  $('searchLabel').textContent = labels.search; $('q').placeholder = labels.search

    const placed = drawMarkers(data, labels)
  const unplaced = data.hosts.filter(host => host.location.latitude === null || host.location.longitude === null).length
  const stat = (term, value, cls = '') => h('div', cls, h('dt', null, term), h('dd', null, value))
  $('stats').replaceChildren(
    stat(labels.online, h('span', null, h('b', null, data.counts.online), ` / ${data.counts.total}`), 's-online'),
    stat(labels.attention, data.counts.attention, data.counts.attention ? 's-degraded' : ''),
    stat(labels.offline, data.counts.offline, data.counts.offline ? 's-offline' : ''),
    stat(labels.location, `${placed} ${labels.regions}${unplaced ? ` · ${labels.unknownPlace} ${unplaced}` : ''}`),
    ...(data.value.groups.length ? [stat(labels.remaining, data.value.groups.map(group => group.text).join(' · '), 'wide')] : []))

  const chips = [['all', labels.all], ['online', labels.online], ['degraded', labels.attention], ['offline', labels.offline]]
  $('filters').replaceChildren(...chips.map(([key, text]) => button(text, filter === key, () => { filter = key; render() })),
    ...(region ? [button(`${region.replaceAll(',', ' · ')} ✕`, true, () => { region = ''; render() })] : []))
  $('filters').setAttribute('aria-label', labels.filter)
  $('views').replaceChildren(button(labels.card, view === 'card', () => { view = 'card'; store('orbit-view', view); render() }), button(labels.list, view === 'list', () => { view = 'list'; store('orbit-view', view); render() }))
  $('views').setAttribute('aria-label', labels.view)

  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => (filter === 'all' || host.state === filter) && (!region || region.split(',').includes(host.location.countryCode)) &&
    `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
  const list = $('hosts'); list.className = `hosts is-${view}`
  list.replaceChildren(...(hosts.length ? hosts.map(host => view === 'card' ? card(host, labels) : row(host, labels)) : [h('p', 'empty', labels.empty)]))
  $('foot').textContent = `KPanel · ${labels.valueHint}`
  $('foot').hidden = !data.value.groups.length
}

let lastWidth = 0
new ResizeObserver(() => { const width = $('map').clientWidth; if (snap && width !== lastWidth) { lastWidth = width; drawMarkers(snap.data, snap.labels) } }).observe($('map'))

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  snap = msg; render()
})
$('q').addEventListener('input', event => { query = event.target.value; if (snap) render() })
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

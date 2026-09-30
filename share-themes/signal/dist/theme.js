// Signal — a status wall meant to be read at a glance. Protocol 2, no dependencies.
// Snapshot strings are only inserted as text nodes.
const $ = id => document.getElementById(id)
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false) node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
const ICON = { online: '●', degraded: '▲', offline: '✕', pending: '○' }
let snap = null, query = ''
const open = new Set()

function ring(name, reading) {
  const ratio = reading.ratio
  const node = h('div', `ring ${ratio === null ? 'na' : ratio >= 0.9 ? 'hot' : ratio >= 0.75 ? 'warm' : ''}`, h('b', null, reading.text), h('small', null, name))
  node.style.setProperty('--fill', `${Math.round((ratio ?? 0) * 360)}deg`)
  return node
}

function tile(host, labels) {
  const card = h('section', `tile ${host.state}`)
  const toggle = h('button', 'toggle'); toggle.type = 'button'
  toggle.setAttribute('aria-expanded', String(open.has(host.id)))
  toggle.append(h('span', 'badge', h('i', null, ICON[host.state] || '○'), ' ', host.stateLabel),
    h('strong', 'name', host.name), h('span', 'where', [host.location.text, host.os].filter(Boolean).join(' · ') || '—'))
  toggle.addEventListener('click', () => { open.has(host.id) ? open.delete(host.id) : open.add(host.id); render() })
  const rings = h('div', 'rings', ring(labels.cpu, host.cpu), ring(labels.memory, host.memory), ring(labels.disk, host.disk))
  card.append(toggle, rings)
  if (open.has(host.id)) {
    const more = h('dl', 'more')
    const add = (term, value) => { if (value && value !== '—') more.append(h('dt', null, term), h('dd', null, value)) }
    add(labels.uptime, host.uptime); add(labels.download, host.network.down.text); add(labels.upload, host.network.up.text)
    add(host.traffic.monthly ? labels.monthly : labels.cumulative, `${host.traffic.percent ? `${host.traffic.percent} · ` : ''}↓ ${host.traffic.received} ↑ ${host.traffic.sent}`)
    add(labels.expiry, host.expiresOn); add(labels.price, host.price); add(labels.remaining, host.remaining)
    card.append(more)
    if (host.traffic.hint) card.append(h('p', 'hint', host.traffic.hint))
  }
  return card
}

function render() {
  const { data, labels, locale, mode } = snap
  document.documentElement.lang = locale
  document.documentElement.dataset.mode = mode === 'light' ? 'light' : 'dark'
  document.title = data.title
  $('title').textContent = data.title
  $('desc').textContent = data.description; $('desc').hidden = !data.description
  $('q').placeholder = labels.search; $('qLabel').textContent = labels.search
  $('counts').replaceChildren(
    h('div', 'count', h('strong', null, data.counts.total), h('span', null, labels.total)),
    h('div', 'count good', h('strong', null, data.counts.online), h('span', null, `${ICON.online} ${labels.online}`)),
    h('div', `count ${data.counts.attention ? 'warn' : ''}`, h('strong', null, data.counts.attention), h('span', null, `${ICON.degraded} ${labels.attention}`)),
    h('div', `count ${data.counts.offline ? 'bad' : ''}`, h('strong', null, data.counts.offline), h('span', null, `${ICON.offline} ${labels.offline}`)))
  const strip = $('strip')
  strip.setAttribute('aria-label', `${labels.online} ${data.counts.online} / ${data.counts.total}`)
  strip.replaceChildren(...data.hosts.map(host => { const cell = h('span', `cell ${host.state}`, ICON[host.state] || '○'); cell.title = `${host.name} · ${host.stateLabel}`; return cell }))
  $('worth').textContent = data.value.groups.length ? `${labels.remaining}: ${data.value.groups.map(group => group.text).join('  ')}` : ''
  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
  $('tiles').replaceChildren(...(hosts.length ? hosts.map(host => tile(host, labels)) : [h('p', 'empty', labels.empty)]))
  const date = new Date(data.generatedAt)
  $('foot').textContent = `${labels.updated} ${Number.isFinite(date.getTime()) ? date.toLocaleString(locale) : '—'} · KPanel`
}

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  snap = msg; render()
})
$('q').addEventListener('input', event => { query = event.target.value; if (snap) render() })
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

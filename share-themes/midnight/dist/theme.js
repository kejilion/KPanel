// Midnight — a terminal-style status console. Protocol 2, no dependencies.
// Every string from the snapshot is inserted as a text node, never as HTML.
const $ = id => document.getElementById(id)
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false) node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
const COLUMNS = [['name', 'server'], ['cpu', 'cpu'], ['memory', 'memory'], ['disk', 'disk'], ['net', 'download'], ['traffic', 'monthly']]
let snap = null, query = '', filter = 'all', sortKey = '', sortDir = 1

const bar = (ratio, cells = 10) => {
  if (ratio === null || ratio === undefined) return '·'.repeat(cells)
  const filled = Math.round(Math.min(1, Math.max(0, ratio)) * cells)
  return '█'.repeat(filled) + '░'.repeat(cells - filled)
}
const level = ratio => ratio === null || ratio === undefined ? 'na' : ratio >= 0.9 ? 'hot' : ratio >= 0.75 ? 'warm' : 'ok'
const glyph = state => ({ online: '●', degraded: '▲', offline: '✕', pending: '○' }[state] || '○')
const sortValue = (host, key) => ({
  name: host.name.toLowerCase(), cpu: host.cpu.ratio ?? -1, memory: host.memory.ratio ?? -1, disk: host.disk.ratio ?? -1,
  net: (host.network.down.bytesPerSecond ?? -1) + (host.network.up.bytesPerSecond ?? 0), traffic: host.traffic.ratio ?? -1,
}[key])

function gauge(labels, name, reading) {
  const cell = h('td', `cell gauge ${level(reading.ratio)}`)
  cell.dataset.label = labels[name]
  cell.append(h('span', 'reading', h('span', 'bar', bar(reading.ratio)), ' ', h('b', null, reading.text)))
  return cell
}

function row(host, labels) {
  const tr = h('tr', `host ${host.state}`)
  const id = h('td', 'cell ident')
  id.append(h('span', `state ${host.state}`, h('span', null, glyph(host.state)), ' ', host.stateLabel), h('strong', 'name', host.name),
    h('span', 'sub', [host.location.text, host.os, host.architecture].filter(Boolean).join(' · ')))
  const net = h('td', 'cell net')
  net.dataset.label = `${labels.download} / ${labels.upload}`
  net.append(h('span', null, '↓ ', host.network.down.text), h('span', null, '↑ ', host.network.up.text))
  const traffic = h('td', `cell traffic ${host.traffic.tone || 'normal'}`)
  traffic.dataset.label = host.traffic.monthly ? labels.monthly : labels.cumulative
  const usage = h('span', 'reading')
  if (host.traffic.monthly && host.traffic.percent) usage.append(h('span', 'bar', bar(host.traffic.ratio)), ' ', h('b', null, host.traffic.percent))
  traffic.append(usage, h('span', 'sub', `${labels.received} ${host.traffic.received} · ${labels.sent} ${host.traffic.sent}`))
  if (host.traffic.hint) traffic.append(h('span', 'hint', host.traffic.hint))
  const facts = h('td', 'cell facts')
  for (const [name, value] of [['uptime', host.uptime], ['expiry', host.expiresOn], ['price', host.price], ['remaining', host.remaining]]) {
    if (value && value !== '—') facts.append(h('span', null, h('i', null, labels[name]), ' ', value))
  }
  tr.append(id, gauge(labels, 'cpu', host.cpu), gauge(labels, 'memory', host.memory), gauge(labels, 'disk', host.disk), net, traffic, facts)
  return tr
}

function render() {
  const { data, labels, locale, mode } = snap
  document.documentElement.lang = locale
  document.documentElement.dataset.mode = mode === 'light' ? 'light' : 'dark'
  document.title = data.title
  $('title').textContent = data.title
  $('desc').textContent = data.description
  $('desc').hidden = !data.description
  $('q').placeholder = labels.search
  $('findLabel').textContent = labels.search

  const summary = $('summary')
  const items = [[labels.total, data.counts.total, ''], [labels.online, data.counts.online, 'ok'], [labels.attention, data.counts.attention, data.counts.attention ? 'warm' : ''], [labels.offline, data.counts.offline, data.counts.offline ? 'hot' : '']]
  if (data.value.groups.length) items.push([labels.remaining, data.value.groups.map(group => group.text).join('  '), 'money'])
  summary.replaceChildren(...items.map(([term, value, tone]) => h('div', `stat ${tone}`, h('dt', null, term), h('dd', null, value))))
  if (data.value.groups.length) summary.append(h('p', 'value-note', `${labels.valueHint} ${data.value.included} / ${data.counts.total} ${labels.covered} · ${data.value.excluded} ${labels.excluded}`))

  const filters = $('filters')
  filters.replaceChildren(...[['all', labels.all], ['online', labels.online], ['degraded', labels.attention], ['offline', labels.offline]].map(([key, text]) => {
    const button = h('button', 'chip', text)
    button.type = 'button'; button.setAttribute('aria-pressed', String(filter === key))
    button.addEventListener('click', () => { filter = key; render() })
    return button
  }))

  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => (filter === 'all' || host.state === filter) &&
    `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
    .sort((a, b) => { if (!sortKey) return 0; const x = sortValue(a, sortKey), y = sortValue(b, sortKey); return (x < y ? -1 : x > y ? 1 : 0) * sortDir })

  const head = h('tr')
  for (const [key, label] of COLUMNS) {
    const th = h('th'); th.scope = 'col'
    th.setAttribute('aria-sort', sortKey === key ? (sortDir > 0 ? 'ascending' : 'descending') : 'none')
    const button = h('button', 'sort', labels[label], sortKey === key ? (sortDir > 0 ? ' ▲' : ' ▼') : '')
    button.type = 'button'
    button.addEventListener('click', () => {
      // First click sorts one way, second reverses, third restores the panel's own order.
      const first = key === 'name' ? 1 : -1
      if (sortKey !== key) { sortKey = key; sortDir = first } else if (sortDir === first) sortDir = -first; else sortKey = ''
      render()
    })
    th.append(button); head.append(th)
  }
  head.append(h('th', null, labels.uptime))
  const table = h('table', 'fleet', h('thead', null, head), h('tbody', null, hosts.map(host => row(host, labels))))
  $('list').replaceChildren(hosts.length ? table : h('p', 'empty', `> ${labels.empty}`))

  const date = new Date(data.generatedAt)
  $('foot').textContent = `# ${labels.updated} ${Number.isFinite(date.getTime()) ? date.toLocaleString(locale) : '—'} · KPanel`
}

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  snap = msg; render()
})
$('q').addEventListener('input', event => { query = event.target.value; if (snap) render(); event.target.focus() })
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
// Optional: report the natural content height so the public page keeps one scrollbar.
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

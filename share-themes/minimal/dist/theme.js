// Minimal — an editorial page: big numerals, ruled entries, hairline gauges.
// Protocol 2, no dependencies; snapshot strings are only ever inserted as text.
const $ = id => document.getElementById(id)
const h = (tag, cls, ...kids) => {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false) node.append(kid.nodeType ? kid : document.createTextNode(String(kid)))
  return node
}
let snap = null, query = '', tab = 'all'

function hairline(label, reading) {
  const ratio = reading.ratio
  const meter = h('span', 'hairline'); meter.setAttribute('role', 'img')
  meter.setAttribute('aria-label', `${label} ${reading.text}`)
  const fill = h('span', ratio === null ? 'fill na' : ratio >= 0.9 ? 'fill hot' : ratio >= 0.75 ? 'fill warm' : 'fill')
  fill.style.width = `${Math.round((ratio ?? 0) * 100)}%`
  meter.append(fill)
  return h('div', 'gauge', h('span', 'gauge-name', label), meter, h('span', 'gauge-value', reading.text))
}

function entry(host, labels, index) {
  const li = h('li', `entry ${host.state}`)
  const title = h('div', 'entry-head')
  title.append(h('span', 'no', String(index + 1).padStart(2, '0')),
    h('div', 'who', h('h2', null, host.name), h('p', null, [host.location.text, host.os].filter(Boolean).join(' — '))),
    h('span', `mark ${host.state}`, host.stateLabel))
  const gauges = h('div', 'gauges', hairline(labels.cpu, host.cpu), hairline(labels.memory, host.memory), hairline(labels.disk, host.disk))
  const notes = h('dl', 'notes')
  const add = (term, value) => { if (value && value !== '—') notes.append(h('div', null, h('dt', null, term), h('dd', null, value))) }
  add(labels.uptime, host.uptime)
  add(labels.download, host.network.down.text)
  add(labels.upload, host.network.up.text)
  add(host.traffic.monthly ? labels.monthly : labels.cumulative, `${host.traffic.percent ? `${host.traffic.percent} · ` : ''}↓ ${host.traffic.received}  ↑ ${host.traffic.sent}`)
  add(labels.expiry, host.expiresOn); add(labels.price, host.price); add(labels.remaining, host.remaining)
  li.append(title, gauges, notes)
  if (host.traffic.hint) li.append(h('p', 'aside', host.traffic.hint))
  return li
}

function render() {
  const { data, labels, locale, mode } = snap
  document.documentElement.lang = locale
  document.documentElement.dataset.mode = mode === 'light' ? 'light' : 'dark'
  document.title = data.title
  $('kicker').textContent = labels.fleet
  $('title').textContent = data.title
  $('desc').textContent = data.description; $('desc').hidden = !data.description
  $('searchLabel').textContent = labels.search; $('q').placeholder = labels.search

  const figures = [[data.counts.total, labels.total, ''], [data.counts.online, labels.online, 'good'], [data.counts.attention, labels.attention, data.counts.attention ? 'alert' : '']]
  const fig = $('figures')
  fig.replaceChildren(...figures.map(([value, label, tone]) => h('div', `figure ${tone}`, h('strong', null, value), h('span', null, label))))
  if (data.value.groups.length) {
    fig.append(h('div', 'figure worth', h('strong', null, data.value.groups[0].text), h('span', null, `${labels.remaining}${data.value.groups.length > 1 ? ` +${data.value.groups.length - 1}` : ''}`),
      h('small', null, `${data.value.groups.map(group => `${group.currency} ${group.text}`).join(' · ')} — ${data.value.included} / ${data.counts.total} ${labels.covered}`, data.value.excluded ? ` · ${data.value.excluded} ${labels.excluded}` : '')))
  }

  $('tabs').replaceChildren(...[['all', labels.all], ['online', labels.online], ['degraded', labels.attention], ['offline', labels.offline]].map(([key, text]) => {
    const button = h('button', 'tab', text); button.type = 'button'; button.setAttribute('aria-pressed', String(tab === key))
    button.addEventListener('click', () => { tab = key; render() })
    return button
  }))

  const needle = query.trim().toLowerCase()
  const hosts = data.hosts.filter(host => (tab === 'all' || host.state === tab) && `${host.name} ${host.location.text} ${host.os}`.toLowerCase().includes(needle))
  $('entries').replaceChildren(...(hosts.length ? hosts.map((host, index) => entry(host, labels, index)) : [h('li', 'none', labels.empty)]))
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

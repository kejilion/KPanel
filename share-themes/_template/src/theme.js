// KPanel share theme, protocol 2 (runtime "kpanel-share-theme@2").
//
// The host page owns nothing but the frame: you decide the markup, styling, layout, fonts, canvas/SVG and motion.
// The frame is sandboxed ("allow-scripts", opaque origin): no cookies, storage, network, popups or parent DOM.
//
// 1. Tell the host you speak protocol 2:
//      parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
// 2. Receive snapshots (sent once, then on every refresh, language or light/dark change):
//      { source: 'kpanel-share', type: 'snapshot', schema: 2,
//        locale: 'zh-CN' | 'zh-TW' | 'en-US', mode: 'light' | 'dark',
//        labels: { fleet, total, online, attention, offline, cpu, memory, disk, uptime, filter, view, ... },   // localized vocabulary
//        data: {
//          title, description, generatedAt,
//          counts: { total, online, attention, offline },
//          states: { online, degraded, offline, pending },                                         // localized state names
//          value: { included, excluded, groups: [{ currency, text, amount }] },                    // empty groups => hide it
//          hosts: [{
//            id, name, state: 'online' | 'degraded' | 'offline' | 'pending', stateLabel,
//            os, architecture, cores, collected,                                                   // collected=false => metrics unknown
//            location: { text, country, countryCode, city, region, isp, latitude, longitude, flag },  // lat/lon: country centre or null
//                                                                                    // flag: circular flag as a data: URL for <img src>, or ''
//            system: { key, label, accent, path, image },  // distribution mark: `path` is 24×24 SVG path data for your own <svg>,
//                                                          // `image` a data: URL when only a bitmap exists, `accent` the brand colour; '' when unknown
//            cpu:    { text, ratio },                                                              // ratio is 0..1, null when unknown
//            memory: { text, ratio, usedBytes, totalBytes, usedText, totalText },
//            disk:   { text, ratio, usedBytes, totalBytes, usedText, totalText },
//            load: { one, five, fifteen } | null,
//            uptime: 'text', uptimeSeconds,
//            network: { down: { bytesPerSecond, text }, up: { bytesPerSecond, text } },
//            traffic: { monthly, percent, ratio, tone: 'normal' | 'warning' | 'danger', received, sent, quotaGiB,
//                       hint, available, partial, estimated },                                     // show `hint`: it explains accuracy
//            price, expiresOn, remaining                                                           // '' => not configured, hide
//          }]
//        } }
//    Verify `event.source === parent` and source/type/schema before trusting a message.
// 3. Own the whole page (recommended): declare it in `ready` — { ..., protocol: 2, chrome: 'self' } — and the host hides its header,
//    frame and footer and gives you the full viewport (you scroll inside it). You must then draw these three controls yourself;
//    only these actions are accepted, everything else is ignored:
//      parent.postMessage({ source: 'kpanel-share-theme', type: 'action', action: 'refresh' }, '*')
//      parent.postMessage({ source: 'kpanel-share-theme', type: 'action', action: 'set-mode', mode: 'light' | 'dark' }, '*')
//      parent.postMessage({ source: 'kpanel-share-theme', type: 'action', action: 'use-default' }, '*')
//    Localized button text is in labels.refresh / lightMode / darkMode / useDefault.
//    Without chrome: 'self' the host keeps its frame; then optionally report your natural height (integer px, 320..32768):
//      parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height }, '*')
//
// Rules: insert user data with textContent / text nodes, never innerHTML; unknown numbers are "—" or null, never 0;
// colour must not be the only carrier of status; keep text >= 12px (body >= 14px); ship every asset inside the package.
const app = document.getElementById('app')

function render({ data, labels, locale, mode }) {
  document.documentElement.lang = locale
  document.documentElement.dataset.mode = mode
  document.title = data.title
  const title = document.createElement('h1')
  title.textContent = data.title
  const summary = document.createElement('p')
  summary.textContent = `${labels.online} ${data.counts.online} / ${data.counts.total}`
  const list = document.createElement('ul')
  for (const host of data.hosts) {
    const item = document.createElement('li')
    item.textContent = `${host.name} — ${host.stateLabel} — CPU ${host.cpu.text}`
    list.append(item)
  }
  const ask = (action, extra) => parent.postMessage({ source: 'kpanel-share-theme', type: 'action', action, ...extra }, '*')
  const button = (text, run) => { const node = document.createElement('button'); node.type = 'button'; node.textContent = text; node.addEventListener('click', run); return node }
  const next = mode === 'light' ? 'dark' : 'light'
  const bar = document.createElement('div')
  bar.append(button(labels.refresh, () => ask('refresh')), button(next === 'light' ? labels.lightMode : labels.darkMode, () => ask('set-mode', { mode: next })), button(labels.useDefault, () => ask('use-default')))
  app.replaceChildren(bar, title, summary, list)
}

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  render(msg)
})
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2, chrome: 'self' }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

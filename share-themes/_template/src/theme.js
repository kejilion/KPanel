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
//        labels: { fleet, total, online, attention, offline, cpu, memory, disk, uptime, ... },   // localized vocabulary
//        data: {
//          title, description, generatedAt,
//          counts: { total, online, attention, offline },
//          states: { online, degraded, offline, pending },                                         // localized state names
//          value: { included, excluded, groups: [{ currency, text, amount }] },                    // empty groups => hide it
//          hosts: [{
//            id, name, state: 'online' | 'degraded' | 'offline' | 'pending', stateLabel,
//            os, architecture, cores, collected,                                                   // collected=false => metrics unknown
//            location: { text, country, countryCode, city, region, isp },
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
// 3. Optionally report your natural height (integer px, 320..32768) to avoid a nested scrollbar:
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
  app.replaceChildren(title, summary, list)
}

addEventListener('message', event => {
  const msg = event.data
  if (event.source !== parent || msg?.source !== 'kpanel-share' || msg.type !== 'snapshot' || msg.schema !== 2 || !Array.isArray(msg.data?.hosts)) return
  render(msg)
})
parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
new ResizeObserver(() => parent.postMessage({ source: 'kpanel-share-theme', type: 'resize', height: Math.min(32768, Math.max(320, Math.ceil(document.documentElement.getBoundingClientRect().height))) }, '*')).observe(document.body)

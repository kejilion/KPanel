// Mock of the 广告专栏 offers API. Uses archived artwork when available,
// otherwise fictional SVG placeholders; the real Panel only serves cached
// WebP/PNG/JPEG files pinned by the app.kejilion.sh manifest
// (internal/offers, internal/panel/offers.go). A manual refresh switches the
// view to the "stale" state so the refresh-failure notice can be inspected.
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'

const PATH = '/api/v1/offers'
const MEDIA = `${PATH}/media/`

const vendors = [
  { id: 'aurora', vendor: '示例云 A', title: '香港 CN2 线路云服务器', sub: '示意素材 · 非真实厂商', from: '#0b3b3a', to: '#14b8a6', featured: true },
  { id: 'boreal', vendor: '示例云 B', title: '美国大带宽 VPS 年付', sub: '示意素材 · 非真实厂商', from: '#17154a', to: '#6366f1', featured: true },
  { id: 'cinder', vendor: '示例云 C', title: '日本软银线路 按月付', sub: '示意素材 · 非真实厂商', from: '#2a1306', to: '#ea580c', featured: true },
  { id: 'delta', vendor: '示例云 D', title: '入门 VPS 跑脚本', sub: '示意素材', from: '#0a1f4d', to: '#3b82f6' },
  { id: 'ember', vendor: '示例云 E', title: '4 GB 内存云主机', sub: '示意素材', from: '#1e0b3d', to: '#8b5cf6' },
  { id: 'fjord', vendor: '示例云 F', title: '原生 IP 大带宽', sub: '示意素材', from: '#062a33', to: '#06b6d4' },
  { id: 'glade', vendor: '示例云 G', title: '韩国双 ISP 云服务器', sub: '示意素材', from: '#3b0a24', to: '#db2777' },
  { id: 'harbor', vendor: '示例域名 H', title: '.com 域名首年特价', sub: '示意素材', from: '#062b45', to: '#0ea5e9' },
]

function banner(item, width, height) {
  const scale = width / (height * 2 === width ? 960 : 1600)
  const pad = Math.round((height * 2 === width ? 64 : 96) * scale)
  const eyebrow = Math.round((height * 2 === width ? 36 : 28) * scale)
  const title = Math.round((height * 2 === width ? 64 : 60) * scale)
  const sub = Math.round((height * 2 === width ? 38 : 30) * scale)
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${item.from}"/><stop offset="1" stop-color="${item.to}"/></linearGradient></defs>
<rect width="${width}" height="${height}" fill="url(#g)"/>
<circle cx="${width * 0.8}" cy="${height * 0.5}" r="${height * 0.42}" fill="none" stroke="#fff" stroke-opacity=".22" stroke-width="${Math.max(2, 3 * scale)}"/>
<circle cx="${width * 0.8}" cy="${height * 0.5}" r="${height * 0.26}" fill="#fff" fill-opacity=".14"/>
<g font-family="PingFang SC, Microsoft YaHei, sans-serif" fill="#fff">
<text x="${pad}" y="${height * 0.36}" font-size="${eyebrow}" font-weight="600" opacity=".88">${item.vendor}</text>
<text x="${pad}" y="${height * 0.36 + title * 1.3}" font-size="${title}" font-weight="700">${item.title}</text>
<text x="${pad}" y="${height * 0.36 + title * 1.3 + sub * 1.5}" font-size="${sub}" opacity=".85">${item.sub}</text>
</g></svg>`
}

const media = new Map()
function store(svg) {
  const data = Buffer.from(svg)
  const digest = createHash('sha256').update(data).digest('hex')
  media.set(digest, data)
  return `${MEDIA}${digest}`
}

let items = vendors.map((item) => ({
  id: item.id,
  vendor: item.vendor,
  alt: `${item.title}（示意素材）`,
  featured: Boolean(item.featured),
  url: `https://${item.id}.example.com/aff?ref=kpanel-preview`,
  host: `${item.id}.example.com`,
  card: store(banner(item, 960, 480)),
  ...(item.featured ? { wide: store(banner(item, 1600, 400)) } : {}),
}))

// Repository artwork is only used by this local UI mock; production keeps its remote source.
const artworkRoot = new URL('../docs/assets/', import.meta.url)
const artworkManifest = new URL('offers/v1.json', artworkRoot)
if (existsSync(artworkManifest)) {
  const manifest = JSON.parse(readFileSync(artworkManifest, 'utf8'))
  items = manifest.items.map(({ images, ...item }) => {
    const slots = Object.fromEntries(Object.entries(images).map(([slot, image]) => {
      if (!/^offers\/[a-z0-9-]+\.webp$/.test(image.path)) throw new Error('Invalid local artwork path')
      const data = readFileSync(new URL(image.path, artworkRoot))
      const digest = createHash('sha256').update(data).digest('hex')
      if (digest !== image.sha256) throw new Error(`Local artwork checksum mismatch: ${image.path}`)
      media.set(digest, data)
      return [slot, `${MEDIA}${digest}`]
    }))
    return { ...item, host: new URL(item.url).hostname, ...slots }
  })
}

let state = 'live'

export async function mockOffers(request, response, url, send) {
  if (url.pathname !== PATH && !url.pathname.startsWith(MEDIA)) return false
  if (request.method !== 'GET') {
    send(response, 405, { code: 'method_not_allowed', title: 'Method not allowed' })
    return true
  }
  if (url.pathname === PATH) {
    if (url.search === '?refresh=1') state = 'stale'
    send(response, 200, {
      schemaVersion: 1,
      state,
      source: 'https://app.kejilion.sh/offers/v1.json',
      updatedAt: '2026-10-10T00:00:00+08:00',
      fetchedAt: '2026-10-10T09:00:00+08:00',
      items,
    })
    return true
  }
  const data = media.get(url.pathname.slice(MEDIA.length))
  if (!data) {
    send(response, 404, { code: 'offers_image_not_found', title: 'Offers image not found' })
    return true
  }
  const type = data.subarray(8, 12).toString() === 'WEBP' ? 'image/webp' : 'image/svg+xml'
  response.writeHead(200, { 'content-type': type, 'cache-control': 'no-store' })
  response.end(data)
  return true
}

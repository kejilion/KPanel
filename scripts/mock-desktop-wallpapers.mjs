// Mock of the uploaded desktop wallpaper API for local previews. It keeps
// uploads in memory and follows the panel contract (internal/panel/desktop_wallpapers.go)
// closely enough to exercise the picker and the upload dialog; it does not
// re-validate images the way the panel does.
import { createHash, randomBytes } from 'node:crypto'

const PATH = '/api/v1/desktop/wallpapers'
const MAX_COUNT = 12
const MAX_BYTES = 48 << 20
const wallpapers = []

function sniff(buffer) {
  if (buffer.subarray(0, 4).toString() === 'RIFF' && buffer.subarray(8, 12).toString() === 'WEBP') return 'webp'
  if (buffer[0] === 0xff && buffer[1] === 0xd8 && buffer[2] === 0xff) return 'jpeg'
  return ''
}

function view(item) {
  const { image: _image, thumb: _thumb, ...wallpaper } = item
  return { ...wallpaper, imageURL: `${PATH}/${item.id}/image`, thumbURL: `${PATH}/${item.id}/thumb` }
}

function usage() {
  return {
    count: wallpapers.length,
    bytes: wallpapers.reduce((total, item) => total + item.imageBytes + item.thumbBytes, 0),
    maxCount: MAX_COUNT,
    maxBytes: MAX_BYTES,
  }
}

async function readBody(request) {
  const chunks = []
  for await (const chunk of request) chunks.push(chunk)
  return Buffer.concat(chunks)
}

export async function mockDesktopWallpapers(request, response, url, send) {
  if (url.pathname !== PATH && !url.pathname.startsWith(`${PATH}/`)) return false
  const parts = url.pathname.slice(PATH.length).split('/').filter(Boolean)
  if (!parts.length && request.method === 'GET') {
    send(response, 200, { wallpapers: [...wallpapers].reverse().map(view), usage: usage() })
    return true
  }
  if (!parts.length && request.method === 'POST') {
    const body = await readBody(request)
    let form
    try {
      form = await new Request('http://mock.local', { method: 'POST', headers: { 'content-type': request.headers['content-type'] ?? '' }, body }).formData()
    } catch {
      send(response, 400, { title: 'Desktop wallpaper upload is invalid', code: 'desktop_wallpaper_upload_invalid' })
      return true
    }
    const image = Buffer.from(await form.get('image')?.arrayBuffer?.() ?? new ArrayBuffer(0))
    const thumb = Buffer.from(await form.get('thumb')?.arrayBuffer?.() ?? new ArrayBuffer(0))
    let metadata
    try {
      metadata = JSON.parse(String(form.get('metadata') ?? ''))
    } catch {
      metadata = undefined
    }
    const format = sniff(image)
    if (!metadata?.name || !format || !sniff(thumb)) {
      send(response, 422, { title: 'Use a still JPEG or WebP picture', code: 'desktop_wallpaper_image_invalid' })
      return true
    }
    const current = usage()
    if (current.count >= MAX_COUNT || current.bytes + image.length + thumb.length > MAX_BYTES) {
      send(response, 409, { title: 'Wallpaper storage is full', code: 'desktop_wallpaper_quota_exceeded' })
      return true
    }
    const item = {
      id: randomBytes(16).toString('hex'),
      name: String(metadata.name).trim().slice(0, 40),
      format,
      width: 3840,
      height: 2160,
      imageBytes: image.length,
      thumbBytes: thumb.length,
      focusX: Number(metadata.focusX) || 500,
      focusY: Number(metadata.focusY) || 500,
      luminance: 45,
      ...(metadata.theme ? { theme: metadata.theme } : {}),
      createdAt: new Date().toISOString().replace(/\.\d+Z$/, 'Z'),
      imageDigest: createHash('sha256').update(image).digest('hex'),
      image,
      thumb,
    }
    wallpapers.push(item)
    send(response, 201, view(item))
    return true
  }
  const item = wallpapers.find((candidate) => candidate.id === parts[0])
  if (parts.length === 2 && ['image', 'thumb'].includes(parts[1]) && request.method === 'GET') {
    if (!item) {
      send(response, 404, { title: 'Desktop wallpaper not found', code: 'desktop_wallpaper_not_found' })
      return true
    }
    const data = item[parts[1]]
    response.writeHead(200, {
      'Content-Type': `image/${sniff(data)}`,
      'Content-Length': data.length,
      'Cache-Control': 'private, max-age=31536000, immutable',
      'X-Content-Type-Options': 'nosniff',
    })
    response.end(data)
    return true
  }
  if (parts.length === 1 && request.method === 'DELETE') {
    if (!item) {
      send(response, 404, { title: 'Desktop wallpaper not found', code: 'desktop_wallpaper_not_found' })
      return true
    }
    wallpapers.splice(wallpapers.indexOf(item), 1)
    response.writeHead(204)
    response.end()
    return true
  }
  send(response, 405, { title: 'Method not allowed', code: 'method_not_allowed' })
  return true
}

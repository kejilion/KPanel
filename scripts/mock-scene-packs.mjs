// Mock of the desktop 3D scene pack API for local previews. It follows the
// contract the panel server implements: the catalog and pack files come from the
// KPanel repository folder scene-packs/ (GitHub raw, or the gh.kejilion.pro
// mirror); installing downloads every catalog file and verifies its size and
// SHA-256; installed files are served with a sandbox CSP for the desktop iframe.
// Here the "repository" is the local checkout, so previews work offline.
import { createHash, randomBytes } from 'node:crypto'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const packsRoot = join(repositoryRoot, 'scene-packs')
const installRoot = await mkdtemp(join(tmpdir(), 'kpanel-mock-scene-packs-'))
const PACK_ID = /^[a-z0-9][a-z0-9-]{0,39}$/
const SOURCES = ['auto', 'github', 'mirror']
const GITHUB_RAW = 'https://raw.githubusercontent.com/kejilion/KPanel/main/scene-packs/'
const MIRROR = 'https://gh.kejilion.pro/'
const CONTENT_TYPES = {
  html: 'text/html; charset=utf-8',
  js: 'text/javascript; charset=utf-8',
  mjs: 'text/javascript; charset=utf-8',
  css: 'text/css; charset=utf-8',
  json: 'application/json',
  webp: 'image/webp',
  png: 'image/png',
  jpg: 'image/jpeg',
  avif: 'image/avif',
  glb: 'model/gltf-binary',
  gltf: 'model/gltf+json',
  bin: 'application/octet-stream',
  ktx2: 'image/ktx2',
  hdr: 'application/octet-stream',
  wasm: 'application/wasm',
  woff2: 'font/woff2',
  webm: 'video/webm',
  mp4: 'video/mp4',
}
// The pack page runs sandboxed with an opaque origin; this header keeps the same
// sandbox if the file is ever opened directly. Sources are narrowed to this pack below.
const PACK_CSP = [
  'sandbox allow-scripts',
  "default-src 'none'",
  "script-src 'self' 'wasm-unsafe-eval'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "media-src 'self' blob:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "worker-src 'self' blob:",
  "frame-src 'none'",
  "form-action 'none'",
  "base-uri 'none'",
].join('; ')

let source = 'auto'
const installed = new Map()

async function catalog() {
  try {
    const parsed = JSON.parse(await readFile(join(packsRoot, 'catalog.json'), 'utf8'))
    return Array.isArray(parsed.packs) ? parsed.packs.filter((pack) => PACK_ID.test(pack.id)) : []
  } catch {
    return []
  }
}

function describe(pack) {
  return {
    resourceVersion: `sha256:${createHash('sha256').update(JSON.stringify([pack, installed.get(pack.id) ?? null])).digest('hex')}`,
    id: pack.id,
    version: pack.version,
    name: pack.name,
    description: pack.description,
    author: pack.author,
    license: pack.license,
    tags: pack.tags ?? [],
    theme: pack.theme,
    cameras: pack.cameras ?? [],
    sizeBytes: pack.sizeBytes,
    installed: installed.has(pack.id),
    installedVersion: installed.get(pack.id)?.version ?? null,
    fileBase: installed.has(pack.id) ? `/api/v1/desktop/scene-packs/${pack.id}/files/${installed.get(pack.id).token}/` : null,
  }
}

function downloadBase(pack, useMirror) {
  const direct = `${GITHUB_RAW}${pack.path}/`
  return useMirror ? `${MIRROR}${direct}` : direct
}

function sendFile(response, body, extension, headers = {}) {
  response.writeHead(200, {
    'Content-Type': CONTENT_TYPES[extension] ?? 'application/octet-stream',
    'Content-Length': body.length,
    'Cache-Control': 'private, no-cache',
    'X-Content-Type-Options': 'nosniff',
    ...headers,
  })
  response.end(body)
}

export async function mockScenePacks(request, response, url, send, readJSON) {
  if (!url.pathname.startsWith('/api/v1/desktop/scene-packs')) return false
  const rest = url.pathname.slice('/api/v1/desktop/scene-packs'.length).split('/').filter(Boolean)
  const packs = await catalog()

  if (!rest.length && request.method === 'GET') {
    send(response, 200, { source, sources: SOURCES, packs: packs.map(describe) })
    return true
  }
  if (rest.length === 1 && rest[0] === 'source' && request.method === 'PUT') {
    const input = await readJSON(request)
    if (!SOURCES.includes(input?.source)) {
      send(response, 400, { title: '不支持的下载线路', code: 'scene_pack_source_invalid' })
      return true
    }
    source = input.source
    send(response, 200, { source, sources: SOURCES })
    return true
  }

  const pack = packs.find((candidate) => candidate.id === rest[0])
  if (!pack) {
    send(response, 404, { title: '场景不存在', code: 'scene_pack_not_found' })
    return true
  }
  if (rest.length === 2 && ['thumb', 'poster'].includes(rest[1]) && request.method === 'GET') {
    // Thumbnails come from the repository before install; the poster comes from the installed copy.
    const directory = rest[1] === 'thumb' ? join(packsRoot, pack.path) : join(installRoot, installed.get(pack.id)?.token ?? 'missing')
    try {
      sendFile(response, await readFile(join(directory, `${rest[1]}.webp`)), 'webp')
    } catch {
      send(response, 404, { title: '场景图片不可用', code: 'scene_pack_image_missing' })
    }
    return true
  }
  if (rest.length === 2 && rest[1] === 'install' && request.method === 'POST') {
    const input = await readJSON(request)
    if (input?.expectedResourceVersion !== describe(pack).resourceVersion) {
      send(response, 409, { title: '场景已变化，请刷新重试', code: 'scene_pack_changed' })
      return true
    }
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1400))
    const useMirror = source === 'mirror'
    const token = randomBytes(16).toString('hex')
    const target = join(installRoot, token)
    try {
      for (const file of pack.files) {
        if (file.path.includes('..') || file.path.startsWith('/')) throw new Error('unsafe path')
        const body = await readFile(join(packsRoot, pack.path, file.path))
        const digest = createHash('sha256').update(body).digest('hex')
        if (body.length !== file.size || digest !== file.sha256) throw new Error(`checksum mismatch: ${file.path}`)
        await mkdir(dirname(join(target, file.path)), { recursive: true })
        await writeFile(join(target, file.path), body)
      }
    } catch (error) {
      await rm(target, { recursive: true, force: true })
      send(response, 502, { title: '场景下载校验失败，未安装任何文件', code: 'scene_pack_verification_failed', detail: String(error.message) })
      return true
    }
    const previous = installed.get(pack.id)
    installed.set(pack.id, { version: pack.version, token, from: downloadBase(pack, useMirror) })
    if (previous) await rm(join(installRoot, previous.token), { recursive: true, force: true })
    send(response, 200, { ...describe(pack), downloadedFrom: installed.get(pack.id).from })
    return true
  }
  if (rest.length === 1 && request.method === 'DELETE') {
    const input = await readJSON(request)
    if (input?.expectedResourceVersion !== describe(pack).resourceVersion) {
      send(response, 409, { title: '场景已变化，请刷新重试', code: 'scene_pack_changed' })
      return true
    }
    const previous = installed.get(pack.id)
    installed.delete(pack.id)
    if (previous) await rm(join(installRoot, previous.token), { recursive: true, force: true })
    response.writeHead(204)
    response.end()
    return true
  }
  if (rest[1] === 'files' && request.method === 'GET' && installed.has(pack.id) && rest[2] === installed.get(pack.id).token) {
    const path = rest.slice(3).join('/')
    const file = pack.files.find((candidate) => candidate.path === path)
    if (!file) {
      response.writeHead(404)
      response.end()
      return true
    }
    const body = await readFile(join(installRoot, installed.get(pack.id).token, file.path))
    const host = request.headers.host ?? ''
    if (!/^(127\.0\.0\.1|localhost|\[::1\])(?::[0-9]{1,5})?$/.test(host)) {
      send(response, 400, { title: '预览只支持回环地址', code: 'scene_pack_host_invalid' })
      return true
    }
    const base = `http://${host}/api/v1/desktop/scene-packs/${pack.id}/files/${rest[2]}/`
    sendFile(response, body, file.path.split('.').pop().toLowerCase(), {
      'Content-Security-Policy': `${PACK_CSP.replaceAll("'self'", base)}; frame-ancestors 'self'; object-src 'none'`,
      'Access-Control-Allow-Origin': '*',
      'Cross-Origin-Resource-Policy': 'cross-origin',
      'Referrer-Policy': 'no-referrer',
      'X-Frame-Options': 'SAMEORIGIN',
      'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=(), usb=(), fullscreen=(), autoplay=()',
    })
    return true
  }
  send(response, 404, { title: '场景文件不可用', code: 'scene_pack_file_missing' })
  return true
}

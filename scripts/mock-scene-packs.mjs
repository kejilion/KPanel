// Mock of the desktop 3D scene pack API for local previews. It follows the
// contract the panel server implements: the catalog and pack files come from the
// KPanel repository folder scene-packs/ (GitHub raw, or the gh.kejilion.pro
// mirror); installing downloads every catalog file and verifies its size and
// SHA-256; installed files are served with a sandbox CSP for the desktop iframe.
// Here the "repository" is the local checkout, so previews work offline.
//
// Serving installed files (part of the same contract):
// - They live under a file base that names the installed version and a digest of
//   its file list (…/files/<version>-<digest>/), so their URLs never change content:
//   they are sent with an ETag (their SHA-256) and cached for a year as immutable.
//   A repeat visit loads a scene without a single request for its files; an update
//   changes the file base, so nothing stale is ever used.
// - Text and binary data that compress well (scripts, JSON, glTF and .bin buffers,
//   WebAssembly) are sent with Brotli or gzip when the browser accepts it.
import { createHash } from 'node:crypto'
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { brotliCompressSync, constants as zlib, gzipSync } from 'node:zlib'

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const packsRoot = join(repositoryRoot, 'scene-packs')
// Kept across restarts of the preview server, as the panel keeps installed packs.
const installRoot = join(tmpdir(), 'kpanel-mock-scene-packs')
const installedIndex = join(installRoot, 'installed.json')
await mkdir(installRoot, { recursive: true })
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
// sandbox if the file is ever opened directly, and blocks any network egress.
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
/** id -> { version, tag, from, files: [{ path, size, sha256 }] } */
const installed = new Map(Object.entries(await readFile(installedIndex, 'utf8').then(JSON.parse).catch(() => ({}))))
const saveInstalled = () => writeFile(installedIndex, JSON.stringify(Object.fromEntries(installed)))

/** The installed copy's tag: its version and a digest of its file list, so it changes with any file. */
function versionTag(pack) {
  const digest = createHash('sha256').update(JSON.stringify(pack.files.map((file) => [file.path, file.sha256]))).digest('hex')
  return `${pack.version}-${digest.slice(0, 12)}`
}

const COMPRESSIBLE = new Set(['html', 'js', 'mjs', 'css', 'json', 'gltf', 'bin', 'glb', 'wasm', 'txt', 'md'])
const compressed = new Map()
/** The body to send and its Content-Encoding, compressed once and kept, if that saves a tenth or more. */
function encodeFor(request, key, body, extension) {
  const accepts = String(request.headers['accept-encoding'] ?? '')
  if (!COMPRESSIBLE.has(extension) || body.length < 1024) return [body, undefined]
  const encoding = /\bbr\b/.test(accepts) ? 'br' : /\bgzip\b/.test(accepts) ? 'gzip' : undefined
  if (!encoding) return [body, undefined]
  const cacheKey = `${key}|${encoding}`
  if (!compressed.has(cacheKey)) {
    const packed = encoding === 'br'
      ? brotliCompressSync(body, { params: { [zlib.BROTLI_PARAM_QUALITY]: 9, [zlib.BROTLI_PARAM_SIZE_HINT]: body.length } })
      : gzipSync(body, { level: 9 })
    compressed.set(cacheKey, packed.length < body.length * 0.9 ? packed : null)
  }
  const packed = compressed.get(cacheKey)
  return packed ? [packed, encoding] : [body, undefined]
}

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
    fileBase: installed.has(pack.id) ? `/api/v1/desktop/scene-packs/${pack.id}/files/${installed.get(pack.id).tag}/` : null,
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
    const directory = rest[1] === 'thumb' ? join(packsRoot, pack.path) : join(installRoot, pack.id)
    try {
      sendFile(response, await readFile(join(directory, `${rest[1]}.webp`)), 'webp')
    } catch {
      send(response, 404, { title: '场景图片不可用', code: 'scene_pack_image_missing' })
    }
    return true
  }
  if (rest.length === 2 && rest[1] === 'install' && request.method === 'POST') {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1400))
    const useMirror = source === 'mirror'
    const target = join(installRoot, pack.id)
    await rm(target, { recursive: true, force: true })
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
    installed.set(pack.id, { version: pack.version, tag: versionTag(pack), from: downloadBase(pack, useMirror), files: pack.files })
    await saveInstalled()
    send(response, 200, { ...describe(pack), downloadedFrom: installed.get(pack.id).from })
    return true
  }
  if (rest.length === 1 && request.method === 'DELETE') {
    installed.delete(pack.id)
    await saveInstalled()
    await rm(join(installRoot, pack.id), { recursive: true, force: true })
    response.writeHead(204)
    response.end()
    return true
  }
  if (rest[1] === 'files' && request.method === 'GET' && installed.has(pack.id)) {
    // Files of the installed version only: a file base from before an update or a reinstall is gone.
    const copy = installed.get(pack.id)
    const path = rest.slice(3).join('/')
    const file = rest[2] === copy.tag ? copy.files.find((candidate) => candidate.path === path) : undefined
    if (!file) {
      response.writeHead(404, { 'Cache-Control': 'no-store' })
      response.end()
      return true
    }
    const headers = {
      'Content-Security-Policy': PACK_CSP,
      'Access-Control-Allow-Origin': '*',
      'Cross-Origin-Resource-Policy': 'cross-origin',
      'Referrer-Policy': 'no-referrer',
      'Cache-Control': 'private, max-age=31536000, immutable',
      ETag: `"${file.sha256}"`,
      Vary: 'Accept-Encoding',
    }
    if (request.headers['if-none-match'] === headers.ETag) {
      response.writeHead(304, headers)
      response.end()
      return true
    }
    const extension = file.path.split('.').pop().toLowerCase()
    const [body, encoding] = encodeFor(request, `${pack.id}/${copy.tag}/${file.path}`, await readFile(join(installRoot, pack.id, file.path)), extension)
    sendFile(response, body, extension, encoding ? { ...headers, 'Content-Encoding': encoding } : headers)
    return true
  }
  send(response, 404, { title: '场景文件不可用', code: 'scene_pack_file_missing' })
  return true
}

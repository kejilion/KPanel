// Mock of the file API surface the media gallery uses, for local previews only.
// It serves an in-memory /home/gallery tree seeded from KPANEL_MOCK_GALLERY_DIR
// (files become loose photos, subfolders become albums; an optional `.thumbs/`
// folder next to a file provides its thumbnail) or, without that variable, from
// the bundled wallpapers. Uploads, mkdir, rename and trash stay in memory; no
// host file is written. Range requests are honoured so videos can seek.
import { createHash } from 'node:crypto'
import { readdir, readFile, stat } from 'node:fs/promises'
import { dirname, extname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = '/home/gallery'
const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const types = {
  '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.png': 'image/png', '.gif': 'image/gif',
  '.webp': 'image/webp', '.avif': 'image/avif', '.heic': 'image/heic',
  '.mp4': 'video/mp4', '.m4v': 'video/mp4', '.mov': 'video/quicktime', '.webm': 'video/webm',
}
const nodes = new Map()

function version(path, size, modifiedAt) {
  return `sha256:${createHash('sha256').update(`${path}\0${size}\0${modifiedAt}`).digest('hex')}`
}

function parentOf(path) {
  const index = path.lastIndexOf('/')
  return index <= 0 ? '/' : path.slice(0, index)
}

function makeEntry(path, kind, size, modifiedAt) {
  const name = path.slice(path.lastIndexOf('/') + 1)
  return {
    name, path, kind,
    ...(kind === 'file' ? { mime: types[extname(name).toLowerCase()] || 'application/octet-stream' } : {}),
    sizeBytes: size, mode: kind === 'directory' ? 'drwxr-xr-x' : '-rw-r--r--', owner: 'root', group: 'root',
    modifiedAt, resourceVersion: version(path, size, modifiedAt), editable: false, previewable: kind === 'file',
  }
}

function addDirectory(path, modifiedAt = new Date().toISOString()) {
  if (!nodes.has(path)) nodes.set(path, { entry: makeEntry(path, 'directory', 4096, modifiedAt) })
}

async function seedFrom(directory, virtual, depth) {
  addDirectory(virtual)
  let names
  try {
    names = await readdir(directory, { withFileTypes: true })
  } catch {
    return
  }
  for (const item of names) {
    if (item.name.startsWith('.')) continue
    const source = join(directory, item.name)
    const target = `${virtual}/${item.name}`
    if (item.isDirectory()) {
      if (depth < 2) await seedFrom(source, target, depth + 1)
      continue
    }
    const info = await stat(source)
    const thumb = join(directory, '.thumbs', `${item.name}.jpg`)
    const thumbExists = await stat(thumb).then(() => true, () => false)
    nodes.set(target, {
      entry: makeEntry(target, 'file', info.size, info.mtime.toISOString()),
      source,
      thumb: thumbExists ? thumb : undefined,
    })
  }
}

async function seed() {
  addDirectory('/home', '2026-08-01T00:00:00Z')
  const sample = process.env.KPANEL_MOCK_GALLERY_DIR
  if (sample) {
    await seedFrom(resolve(sample), ROOT, 0)
    return
  }
  addDirectory(ROOT)
  addDirectory(`${ROOT}/KPanel 壁纸`)
  const wallpapers = join(repoRoot, 'web', 'public', 'wallpapers')
  const names = (await readdir(wallpapers)).filter((name) => name.endsWith('.webp')).sort()
  for (const [index, name] of names.entries()) {
    const source = join(wallpapers, name)
    const info = await stat(source)
    const modifiedAt = new Date(Date.UTC(2026, 8 - (index % 3), 26 - index * 3, 9)).toISOString()
    const target = index < 2 ? `${ROOT}/${name}` : `${ROOT}/KPanel 壁纸/${name}`
    nodes.set(target, { entry: makeEntry(target, 'file', info.size, modifiedAt), source })
  }
}

await seed()

/** Directory entries the generic mock should show at `/`. */
export const mockGalleryRootEntries = [nodes.get('/home').entry]

function inScope(path) {
  return typeof path === 'string' && (path === '/home' || path === ROOT || path.startsWith(`${ROOT}/`))
}

function children(path) {
  const prefix = path === '/' ? '/' : `${path}/`
  return [...nodes.values()]
    .filter((node) => node.entry.path.startsWith(prefix) && parentOf(node.entry.path) === path)
    .map((node) => node.entry)
    .sort((left, right) => (left.kind === right.kind ? left.name.localeCompare(right.name) : left.kind === 'directory' ? -1 : 1))
}

async function readBody(request, limit = 1024 << 20) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > limit) throw new Error('too large')
    chunks.push(chunk)
  }
  return Buffer.concat(chunks)
}

async function nodeContent(node, thumbnail) {
  if (node.data) return node.data
  return readFile(thumbnail && node.thumb ? node.thumb : node.source)
}

async function serveContent(request, response, url, node, send) {
  const thumbnail = url.searchParams.get('mode') === 'thumbnail'
  if (thumbnail && !/^image\/(jpeg|png|gif)$/.test(node.entry.mime)) {
    send(response, 422, { title: '无法生成文件缩略图', status: 422, code: 'file_thumbnail_unavailable' })
    return
  }
  const body = await nodeContent(node, thumbnail)
  const contentType = thumbnail && node.thumb ? 'image/jpeg' : node.entry.mime
  const disposition = url.searchParams.get('disposition') === 'attachment'
    ? `attachment; filename*=UTF-8''${encodeURIComponent(node.entry.name)}`
    : 'inline'
  const headers = {
    'Content-Type': contentType,
    'Content-Disposition': disposition,
    'Accept-Ranges': 'bytes',
    'Cache-Control': thumbnail ? 'private, max-age=86400' : 'no-store',
    ETag: `"${thumbnail ? 'thumbnail-' : ''}${node.entry.resourceVersion}"`,
  }
  const range = /^bytes=(\d*)-(\d*)$/.exec(request.headers.range || '')
  if (range && !thumbnail) {
    const start = range[1] ? Number(range[1]) : Math.max(0, body.length - Number(range[2]))
    const end = range[1] && range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1
    if (start >= body.length || start > end) {
      response.writeHead(416, { 'Content-Range': `bytes */${body.length}` })
      response.end()
      return
    }
    response.writeHead(206, { ...headers, 'Content-Range': `bytes ${start}-${end}/${body.length}`, 'Content-Length': end - start + 1 })
    response.end(request.method === 'HEAD' ? undefined : body.subarray(start, end + 1))
    return
  }
  response.writeHead(200, { ...headers, 'Content-Length': body.length })
  response.end(request.method === 'HEAD' ? undefined : body)
}

function moveTree(source, destination) {
  for (const [path, node] of [...nodes.entries()]) {
    if (path !== source && !path.startsWith(`${source}/`)) continue
    nodes.delete(path)
    const next = `${destination}${path.slice(source.length)}`
    nodes.set(next, { ...node, entry: makeEntry(next, node.entry.kind, node.entry.sizeBytes, node.entry.modifiedAt) })
  }
}

function removeTree(source) {
  for (const path of [...nodes.keys()]) {
    if (path === source || path.startsWith(`${source}/`)) nodes.delete(path)
  }
}

function validName(name) {
  return typeof name === 'string' && name.trim() && !name.includes('/') && name !== '.' && name !== '..'
}

export async function mockGallery(request, response, url, send, readJSON) {
  const path = url.searchParams.get('path')
  if (request.method === 'GET' && url.pathname === '/api/v1/files' && inScope(path)) {
    if (!nodes.has(path)) {
      send(response, 404, { title: '文件不存在', status: 404, code: 'not_found' })
      return true
    }
    const search = (url.searchParams.get('search') || '').toLowerCase()
    const all = children(path).filter((entry) => !search || entry.name.toLowerCase().includes(search))
    const offset = Number(url.searchParams.get('offset') || 0)
    const limit = Number(url.searchParams.get('limit') || 100)
    const entries = all.slice(offset, offset + limit)
    send(response, 200, {
      path, entries, offset, total: all.length, totalKnown: true,
      ...(offset + limit < all.length ? { nextOffset: offset + limit } : {}),
      truncated: false, scanTruncated: false, archiveManagementAvailable: false, readAt: new Date().toISOString(),
    })
    return true
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/entry' && inScope(path)) {
    const node = nodes.get(path)
    send(response, node ? 200 : 404, node ? node.entry : { title: '文件不存在', status: 404, code: 'not_found' })
    return true
  }
  if ((request.method === 'GET' || request.method === 'HEAD') && url.pathname === '/api/v1/files/content' && inScope(path)) {
    const node = nodes.get(path)
    if (!node || node.entry.kind !== 'file') {
      send(response, 404, { title: '文件不存在', status: 404, code: 'not_found' })
      return true
    }
    await serveContent(request, response, url, node, send)
    return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/upload' && inScope(path)) {
    const name = url.searchParams.get('name')
    const target = `${path}/${name}`
    if (!nodes.has(path) || nodes.get(path).entry.kind !== 'directory' || !validName(name)) {
      send(response, 404, { title: '目标目录不存在', status: 404, code: 'not_found' })
      return true
    }
    if (nodes.has(target) && url.searchParams.get('overwrite') !== 'true') {
      send(response, 409, { title: '目标文件已存在', status: 409, code: 'file_exists' })
      return true
    }
    let data
    try {
      data = await readBody(request)
    } catch {
      send(response, 413, { title: '文件超过上传上限', status: 413, code: 'file_too_large' })
      return true
    }
    const node = { entry: makeEntry(target, 'file', data.length, new Date().toISOString()), data }
    nodes.set(target, node)
    send(response, 200, node.entry)
    return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/download-tickets') {
    const input = await readJSON(request)
    if (!inScope(input.path)) {
      send(response, 404, { title: '模拟预览只支持图库目录内的下载', status: 404, code: 'mock_unsupported' })
      return true
    }
    send(response, 200, {
      downloadUrl: `/api/v1/files/content?path=${encodeURIComponent(input.path)}&disposition=attachment`,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    })
    return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/actions') {
    // The generic mock has no file actions, so this handler owns the endpoint and refuses other paths.
    const input = await readJSON(request)
    const touchesGallery = inScope(input.target) || (input.sources || []).some(inScope)
    if (!touchesGallery) {
      send(response, 422, { title: '模拟预览只支持图库目录内的文件操作', status: 422, code: 'mock_unsupported' })
      return true
    }
    const result = { action: input.action, succeeded: [], failed: [] }
    if (input.action === 'mkdir') {
      const created = `${input.target}/${input.name}`
      if (!nodes.has(input.target) || !validName(input.name)) result.failed.push({ path: created, detail: '目标目录不存在或名称无效' })
      else if (nodes.has(created)) result.failed.push({ path: created, detail: '目标已存在，请修改名称后重试' })
      else {
        addDirectory(created)
        result.succeeded.push({ path: created })
      }
    } else if (input.action === 'rename') {
      const [source] = input.sources || []
      if (!nodes.has(source)) result.failed.push({ path: source, detail: '文件状态已变化，请刷新后重试' })
      else if (nodes.has(input.target)) result.failed.push({ path: source, detail: '目标已存在，请修改名称后重试' })
      else {
        moveTree(source, input.target)
        result.succeeded.push({ path: source, destination: input.target })
      }
    } else if (input.action === 'trash') {
      for (const source of input.sources || []) {
        const node = nodes.get(source)
        if (!node || input.expectedResourceVersions?.[source] !== node.entry.resourceVersion) {
          result.failed.push({ path: source, detail: '文件状态已变化，请刷新后重试' })
          continue
        }
        removeTree(source)
        result.succeeded.push({ path: source })
      }
    } else {
      send(response, 422, { title: '模拟预览不支持该文件操作', status: 422, code: 'mock_unsupported' })
      return true
    }
    send(response, 200, result)
    return true
  }
  return false
}

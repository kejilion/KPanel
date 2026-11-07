// Synthetic, silent preview fixtures. This server never contacts a music platform.
const root = '/home/docker/navidrome/music'
const titles = ['网易云示例', 'QQ 示例', '无匹配示例', '来源失败示例', ...Array.from({ length: 88 }, (_, i) => '列表滚动示例 ' + String(i + 1).padStart(2, '0'))]
const wav = Buffer.alloc(44 + 16000 * 180)
wav.write('RIFF'); wav.writeUInt32LE(wav.length - 8, 4); wav.write('WAVEfmt ', 8)
wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22)
wav.writeUInt32LE(8000, 24); wav.writeUInt32LE(16000, 28); wav.writeUInt16LE(2, 32)
wav.writeUInt16LE(16, 34); wav.write('data', 36); wav.writeUInt32LE(16000 * 180, 40)
const entries = titles.map(title => ({
  name: `示例歌手 - ${title}.wav`, path: `${root}/示例歌手 - ${title}.wav`, kind: 'file', sizeBytes: wav.length,
  mode: '-rw-r--r--', owner: 'demo', group: 'demo', modifiedAt: '2026-11-06T00:00:00Z',
  resourceVersion: 'music-preview-v1', editable: false, previewable: true,
}))
// Valid silent MPEG Layer III frames for bitrate/lossy styling demonstrations.
const mp3 = Buffer.alloc(1044 * 96)
for (let at = 0; at < mp3.length; at += 1044) mp3.set([255, 251, 224, 0], at)
const mp3Entry = { ...entries[0], name: '码率展示示例.mp3', path: root + '/码率展示示例.mp3', sizeBytes: mp3.length }
entries.push(mp3Entry)
const sidecars = new Map()
function sidecarPath(path) { return /\.(lrc|jpg|jpeg|png|webp)$/.test(path) && entries.some(e => e.path.replace(/\.[^.]+$/, '') === path.replace(/\.[^.]+$/, '')) }
const cover = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='

export async function mockMusic(request, response, url, send, readJSON) {
  const host = url.searchParams.get('hostId') || '', path = url.searchParams.get('path') || ''
  if (request.method === 'GET' && url.pathname === '/api/v1/files/entry' && entries.some(item => item.path === path)) {
    send(response, 200, entries.find(item => item.path === path)); return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/entries') {
    const input = await readJSON(request)
    if (Array.isArray(input.paths) && input.paths.length > 0 && input.paths.length <= 32 && input.paths.every(value => typeof value === 'string' && value.startsWith(root + '/'))) {
      const found = input.paths.flatMap(value => {
        const entry = entries.find(item => item.path === value) || sidecars.get(host + ':' + value)?.entry
        return entry ? [entry] : []
      })
      send(response, 200, { entries: found, unavailable: input.paths.filter(value => !found.some(item => item.path === value)) }); return true
    }
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/transfer/sessions') {
    const input = await readJSON(request)
    if (input.operation === 'create' && input.input?.directory === root) {
      // This small fixture exercises the supported legacy receiver fallback.
      send(response, 404, { title: 'Music fixture uses legacy upload' }); return true
    }
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/entry' && sidecarPath(path)) {
    const saved = sidecars.get(host + ':' + path)
    send(response, saved ? 200 : 404, saved?.entry || { code: 'not_found', title: 'Sidecar not found' }); return true
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/content' && sidecarPath(path)) {
    const saved = sidecars.get(host + ':' + path)
    if (!saved) { send(response, 404, { code: 'not_found', title: 'Sidecar not found' }); return true }
    response.writeHead(200, { 'Content-Type': saved.type, 'Content-Length': saved.bytes.length }); response.end(saved.bytes); return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/upload' && path === root) {
    const name = url.searchParams.get('name') || '', target = root + '/' + name
    if (host.length > 128 || !sidecarPath(target) || /[\\/]/.test(name)) { send(response, 400, { title: 'Invalid fixture sidecar' }); return true }
    const key = host + ':' + target
    const pieces = []; let size = 0
    try { for await (const piece of request) { size += piece.length; if (size > 256 * 1024) { send(response, 413, { title: 'Fixture limit' }); return true }; pieces.push(piece) } }
    catch { return true }
    if (sidecars.has(key)) { send(response, 409, { code: 'file_conflict', title: 'Fixture file exists' }); return true }
    if (sidecars.size >= 64) { send(response, 429, { title: 'Fixture quota' }); return true }
    const bytes = Buffer.concat(pieces), entry = { ...entries[0], path: target, name, sizeBytes: size, resourceVersion: 'sidecar-v1' }
    sidecars.set(key, { host, entry, bytes, type: name.endsWith('.lrc') ? 'text/plain;charset=utf-8' : name.endsWith('.png') ? 'image/png' : 'image/jpeg' })
    send(response, 201, entry); return true
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files' && url.searchParams.get('path') === root) {
    const items = [...entries, ...[...sidecars.values()].filter(saved => saved.host === host).map(saved => saved.entry)]
    send(response, 200, { path: root, entries: items, total: items.length, totalKnown: true, offset: 0, scanTruncated: false })
    return true
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/content' && entries.some(e => e.path === url.searchParams.get('path'))) {
    const data = path === mp3Entry.path ? mp3 : wav
    const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range || '')
    const start = range ? Math.min(Number(range[1]), data.length - 1) : 0
    const end = range?.[2] ? Math.min(Number(range[2]), data.length - 1) : data.length - 1
    response.writeHead(range ? 206 : 200, { 'Content-Type': path === mp3Entry.path ? 'audio/mpeg' : 'audio/wav', 'Accept-Ranges': 'bytes', 'Content-Length': end - start + 1, ...(range ? { 'Content-Range': `bytes ${start}-${end}/${data.length}` } : {}) })
    response.end(data.subarray(start, end + 1)); return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/music/metadata') {
    const input = await readJSON(request)
    if (input.title === titles[3]) send(response, 502, { code: 'music_metadata_unavailable', title: '模拟来源暂不可用' })
    else if (input.title === titles[2]) send(response, 200, { matched: false })
    else send(response, 200, { matched: true, source: input.title === titles[1] ? 'qq' : 'netease', cover, lyrics: Array.from({ length: 180 }, (_, i) => `[${String(Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}.00]第 ${i + 1} 行模拟歌词，仅供界面预览`).join('\n') })
    return true
  }
  return false
}

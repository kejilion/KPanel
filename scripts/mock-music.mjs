// Synthetic, silent preview fixtures. This server never contacts a music platform.
const root = '/home/docker/navidrome/music'
const titles = ['网易云示例', 'QQ 示例', '无匹配示例', '来源失败示例']
const wav = Buffer.alloc(44 + 16000)
wav.write('RIFF'); wav.writeUInt32LE(wav.length - 8, 4); wav.write('WAVEfmt ', 8)
wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22)
wav.writeUInt32LE(8000, 24); wav.writeUInt32LE(16000, 28); wav.writeUInt16LE(2, 32)
wav.writeUInt16LE(16, 34); wav.write('data', 36); wav.writeUInt32LE(16000, 40)
const entries = titles.map(title => ({
  name: `示例歌手 - ${title}.wav`, path: `${root}/示例歌手 - ${title}.wav`, kind: 'file', sizeBytes: wav.length,
  mode: '-rw-r--r--', owner: 'demo', group: 'demo', modifiedAt: '2026-11-06T00:00:00Z',
  resourceVersion: 'music-preview-v1', editable: false, previewable: true,
}))
const cover = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='

export async function mockMusic(request, response, url, send, readJSON) {
  if (request.method === 'GET' && url.pathname === '/api/v1/files' && url.searchParams.get('path') === root) {
    send(response, 200, { path: root, entries, total: entries.length, totalKnown: true, offset: 0, scanTruncated: false })
    return true
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/content' && entries.some(e => e.path === url.searchParams.get('path'))) {
    const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range || '')
    const start = range ? Math.min(Number(range[1]), wav.length - 1) : 0
    const end = range?.[2] ? Math.min(Number(range[2]), wav.length - 1) : wav.length - 1
    response.writeHead(range ? 206 : 200, { 'Content-Type': 'audio/wav', 'Accept-Ranges': 'bytes', 'Content-Length': end - start + 1, ...(range ? { 'Content-Range': `bytes ${start}-${end}/${wav.length}` } : {}) })
    response.end(wav.subarray(start, end + 1)); return true
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/music/metadata') {
    const input = await readJSON(request)
    if (input.title === titles[3]) send(response, 502, { code: 'music_metadata_unavailable', title: '模拟来源暂不可用' })
    else if (input.title === titles[2]) send(response, 200, { matched: false })
    else send(response, 200, { matched: true, source: input.title === titles[1] ? 'qq' : 'netease', cover, lyrics: '[00:00.00]模拟歌词与封面，仅供界面预览\n[00:00.50]真实歌曲文件保持不变' })
    return true
  }
  return false
}

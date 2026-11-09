/** Read only a bounded prefix: never download a lossless album to find artwork. */
export const TAG_BUDGET = 2 * 1024 * 1024
export interface MusicTags { title?: string; artist?: string; album?: string; lyrics?: string; picture?: Blob }
const decoder = new TextDecoder()
const ascii = (bytes: Uint8Array) => decoder.decode(bytes)
function uint(bytes: Uint8Array, offset: number): number {
  return offset + 4 <= bytes.length ? new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength).getUint32(offset) : 0
}
function syncSafe(bytes: Uint8Array, offset: number): number { return ((bytes[offset]! & 127) << 21) | ((bytes[offset + 1]! & 127) << 14) | ((bytes[offset + 2]! & 127) << 7) | (bytes[offset + 3]! & 127) }
function legacyText(bytes: Uint8Array): string {
  // Some ID3 tags label UTF-8 or GBK bytes as ISO-8859-1. Only use GBK
  // when it decodes cleanly to multiple Chinese characters; Western tags
  // otherwise retain the ID3 default rather than guessing from one pair.
  try {
    const utf8 = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
    if (/[^\x00-\x7f]/u.test(utf8)) return utf8
  } catch { /* Try the common legacy Chinese encoding next. */ }
  try {
    const chinese = new TextDecoder('gbk', { fatal: true }).decode(bytes)
    let highPairs = 0
    for (let i = 0; i < bytes.length; i++) {
      if (bytes[i]! >= 0x80) { if (bytes[i + 1]! >= 0x80) highPairs++; i++ }
    }
    if (highPairs >= 2 && (chinese.match(/\p{Script=Han}/gu) ?? []).length >= 2) return chinese
  } catch { /* Keep the standard Western decoding below. */ }
  return new TextDecoder('windows-1252').decode(bytes)
}

export function readableMusicTag(value: string | undefined, fallback: string): string {
  if (!value || value.includes('\ufffd')) return fallback
  // An already-corrupted tag can be valid Unicode; do not replace a known
  // Chinese filename with a dense run of Western mojibake characters.
  const extended = (value.match(/[\u00c0-\u00ff]/gu) ?? []).length
  if (/\p{Script=Han}/u.test(fallback) && !/\p{Script=Han}/u.test(value) && extended >= 3 && extended / value.length >= .3) return fallback
  return value
}
function text(bytes: Uint8Array): string {
  if (!bytes.length) return ''
  const encoding = bytes[0] === 1 ? 'utf-16' : bytes[0] === 2 ? 'utf-16be' : 'utf-8'
  const decoded = bytes[0] === 0 ? legacyText(bytes.subarray(1)) : new TextDecoder(encoding).decode(bytes.subarray(1))
  return decoded.replace(/\0/g, '').trim().slice(0, 65536)
}
function terminated(bytes: Uint8Array, start: number, wide: boolean): number {
  for (let i = start; i < bytes.length - (wide ? 1 : 0); i += wide ? 2 : 1) if (bytes[i] === 0 && (!wide || bytes[i + 1] === 0)) return i + (wide ? 2 : 1)
  return bytes.length
}
function picture(bytes: Uint8Array): Blob | undefined {
  let type = ''
  if (bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255) type = 'image/jpeg'
  if (bytes[0] === 137 && ascii(bytes.subarray(1, 4)) === 'PNG') type = 'image/png'
  if (ascii(bytes.subarray(0, 4)) === 'RIFF' && ascii(bytes.subarray(8, 12)) === 'WEBP') type = 'image/webp'
  return type && bytes.length <= TAG_BUDGET ? new Blob([new Uint8Array(bytes)], { type }) : undefined
}
export function parseMusicTags(input: Uint8Array): MusicTags {
  const bytes = input.subarray(0, TAG_BUDGET), result: MusicTags = {}
  if (ascii(bytes.subarray(0, 3)) === 'ID3' && [3, 4].includes(bytes[3]!) && !(bytes[5]! & 0xc0)) {
    const end = Math.min(bytes.length, 10 + syncSafe(bytes, 6))
    for (let i = 10; i + 10 <= end;) {
      const id = ascii(bytes.subarray(i, i + 4)), size = bytes[3] === 4 ? syncSafe(bytes, i + 4) : uint(bytes, i + 4)
      if (!size || i + 10 + size > end) break
      const body = bytes.subarray(i + 10, i + 10 + size)
      // Skip compressed/encrypted/unsynchronized frames.
      if (bytes[i + 9] === 0) {
        if (id === 'TIT2') result.title = text(body)
        if (id === 'TPE1') result.artist = text(body)
        if (id === 'TALB') result.album = text(body)
        if (id === 'APIC') { const mimeEnd = terminated(body, 1, false); result.picture ||= picture(body.subarray(terminated(body, mimeEnd + 1, body[0] === 1 || body[0] === 2))) }
        if (id === 'USLT') { const content = body.subarray(terminated(body, 4, body[0] === 1 || body[0] === 2)); result.lyrics = text(new Uint8Array([body[0]!, ...content])) }
      }
      i += 10 + size
    }
  }
  if (ascii(bytes.subarray(0, 4)) === 'fLaC') {
    for (let i = 4; i + 4 <= bytes.length;) {
      const last = Boolean(bytes[i]! & 128), kind = bytes[i]! & 127
      const size = (bytes[i + 1]! << 16) | (bytes[i + 2]! << 8) | bytes[i + 3]!
      if (i + 4 + size > bytes.length) break
      const block = bytes.subarray(i + 4, i + 4 + size)
      if (kind === 4 && block.length >= 8) {
        const view = new DataView(block.buffer, block.byteOffset, block.byteLength)
        let at = 4 + view.getUint32(0, true)
        if (at + 4 <= block.length) {
          const count = Math.min(view.getUint32(at, true), 1024); at += 4
          for (let n = 0; n < count && at + 4 <= block.length; n++) {
            const length = view.getUint32(at, true); at += 4
            if (at + length > block.length) break
            const field = ascii(block.subarray(at, at + length)); at += length
            const equals = field.indexOf('='), key = field.slice(0, equals).toUpperCase(), value = field.slice(equals + 1).slice(0, 65536)
            if (key === 'TITLE') result.title = value
            if (key === 'ARTIST') result.artist = value
            if (key === 'ALBUM') result.album = value
            if (key === 'LYRICS' || key === 'UNSYNCEDLYRICS') result.lyrics = value
          }
        }
      }
      if (kind === 6 && block.length >= 32) {
        let at = 8 + uint(block, 4)
        if (at + 4 <= block.length) {
          at += 4 + uint(block, at) + 16
          const length = uint(block, at); at += 4
          if (length && at + length <= block.length) result.picture ||= picture(block.subarray(at, at + length))
        }
      }
      i += 4 + size
      if (last) break
    }
  }
  return result
}
export function musicVersionETag(resourceVersion: string) { return `"${resourceVersion}"` }
export class MusicVersionMismatchError extends Error {
  constructor() { super('Music resource version mismatch'); this.name = 'MusicVersionMismatchError' }
}

export async function readMusicTags(url: string, signal: AbortSignal, expectedResourceVersion?: string): Promise<MusicTags> {
  const headers = new Headers({ Range: 'bytes=0-' + (TAG_BUDGET - 1) })
  const response = await fetch(url, { headers, signal })
  if (response.status === 412) {
    await response.body?.cancel(); throw new MusicVersionMismatchError()
  }
  if (!response.ok) { await response.body?.cancel(); throw new Error('Music tags unavailable') }
  if (expectedResourceVersion && response.headers.get('ETag') !== musicVersionETag(expectedResourceVersion)) {
    await response.body?.cancel(); throw new MusicVersionMismatchError()
  }
  if (!response.body || (response.status === 206 && !response.headers.get('Content-Range')?.startsWith('bytes 0-'))) {
    await response.body?.cancel(); throw new Error('Music tags unavailable')
  }
  const reader = response.body.getReader(), chunks: Uint8Array[] = []
  let size = 0
  try {
    while (size < TAG_BUDGET) {
      const chunk = await reader.read()
      if (chunk.done) break
      const part = chunk.value.subarray(0, TAG_BUDGET - size); chunks.push(part); size += part.length
    }
  } finally { await reader.cancel() }
  const bytes = new Uint8Array(size); let offset = 0
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length }
  return parseMusicTags(bytes)
}

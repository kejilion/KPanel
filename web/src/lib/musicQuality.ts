import { fileAPIForHost } from './fileHostContext'
import type { Track } from './music'

export interface MusicQuality {
  lossless?: boolean
  bitrate?: number
  bitrateKind?: 'frame' | 'average' | 'pcm'
  sampleRate?: number
  bitsPerSample?: number
  vbr?: boolean
}
const prefixBytes = 64 * 1024
const text = (b: Uint8Array, at: number, size: number) => String.fromCharCode(...b.subarray(at, at + size))
const u32 = (b: Uint8Array, at: number, little = false) => at + 4 <= b.length ? new DataView(b.buffer, b.byteOffset, b.byteLength).getUint32(at, little) : 0
export function musicQualityKey(track: Track): string { return JSON.stringify([track.hostId, track.entry.path, track.entry.resourceVersion, track.entry.sizeBytes]) }
export function id3AudioOffset(b: Uint8Array): number {
  if (b.length < 10 || text(b, 0, 3) !== 'ID3' || ![2, 3, 4].includes(b[3]!)) return 0
  if ([6, 7, 8, 9].some(i => b[i]! > 127)) return 0
  return 10 + ((b[6]! << 21) | (b[7]! << 14) | (b[8]! << 7) | b[9]!) + (b[3] === 4 && (b[5]! & 16) ? 10 : 0)
}

function mp3Frame(b: Uint8Array, i: number) {
  if (i + 4 > b.length || b[i] !== 255 || (b[i + 1]! & 224) !== 224) return undefined
  const version = (b[i + 1]! >> 3) & 3, layer = (b[i + 1]! >> 1) & 3
  const index = b[i + 2]! >> 4, frequency = (b[i + 2]! >> 2) & 3
  if (version === 1 || layer !== 1 || index === 0 || index === 15 || frequency === 3) return undefined
  const sampleRate = [44100, 48000, 32000][frequency]! / (version === 3 ? 1 : version === 2 ? 2 : 4)
  const bitrate = (version === 3 ? [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320] : [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160])[index]!
  const frameBytes = Math.floor((version === 3 ? 144000 : 72000) * bitrate / sampleRate) + ((b[i + 2]! >> 1) & 1)
  return { version, bitrate, sampleRate, frameBytes, samples: version === 3 ? 1152 : 576, mono: (b[i + 3]! >> 6) === 3, crc: !(b[i + 1]! & 1) }
}

/** Parse format headers only; labels describe the encoding, not recording provenance. */
export function parseMusicQuality(b: Uint8Array, fileBytes: number, audioOnly = false): MusicQuality {
  if (!audioOnly && b.length >= 42 && text(b, 0, 4) === 'fLaC' && (b[4]! & 127) === 0 && ((b[5]! << 16) | (b[6]! << 8) | b[7]!) === 34) {
    const sampleRate = (b[18]! << 12) | (b[19]! << 4) | (b[20]! >> 4)
    const bitsPerSample = (((b[20]! & 1) << 4) | (b[21]! >> 4)) + 1
    const samples = (b[21]! & 15) * 2 ** 32 + u32(b, 22)
    if (!sampleRate || bitsPerSample < 4 || bitsPerSample > 32) return {}
    return { lossless: true, sampleRate, bitsPerSample, ...(samples && fileBytes > 0 ? { bitrate: Math.round(fileBytes * 8 * sampleRate / samples / 1000), bitrateKind: 'average' as const } : {}) }
  }
  if (!audioOnly && text(b, 0, 4) === 'RIFF' && text(b, 8, 4) === 'WAVE') {
    for (let at = 12, chunks = 0; at + 8 <= b.length && chunks++ < 256;) {
      const size = u32(b, at + 4, true), start = at + 8
      if (text(b, at, 4) === 'fmt ' && size >= 16 && start + 16 <= b.length) {
        const view = new DataView(b.buffer, b.byteOffset + start, Math.min(size, b.length - start))
        const format = view.getUint16(0, true), channels = view.getUint16(2, true), sampleRate = view.getUint32(4, true), bitsPerSample = view.getUint16(14, true)
        const byteRate = view.getUint32(8, true)
        if (![1, 3].includes(format) || !channels || !sampleRate || !bitsPerSample || byteRate !== sampleRate * channels * bitsPerSample / 8) return {}
        return { lossless: true, bitrate: Math.round(byteRate * 8 / 1000), bitrateKind: 'pcm', sampleRate, bitsPerSample }
      }
      at = start + size + (size & 1)
    }
    return {}
  }
  // APE/AIFF/DSD are recognized as lossless encodings; no invented bitrate.
  if (!audioOnly && b.length >= 32 && (text(b, 0, 4) === 'MAC ' || (text(b, 0, 4) === 'FORM' && text(b, 8, 4) === 'AIFF') || text(b, 0, 4) === 'DSD ')) return { lossless: true }
  const offset = audioOnly ? 0 : id3AudioOffset(b)
  if (!audioOnly && text(b, 0, 3) === 'ID3' && !offset) return {}
  for (let i = offset; i < Math.min(b.length - 4, offset + 4096); i++) {
    const first = mp3Frame(b, i)
    if (!first) continue
    const next = mp3Frame(b, i + first.frameBytes)
    if (!next || next.sampleRate !== first.sampleRate || next.version !== first.version) continue
    const side = first.version === 3 ? (first.mono ? 17 : 32) : (first.mono ? 9 : 17)
    const xing = i + 4 + (first.crc ? 2 : 0) + side, marker = text(b, xing, 4)
    if ((marker === 'Xing' || marker === 'Info') && xing + 16 <= b.length) {
      const flags = u32(b, xing + 4), frames = flags & 1 ? u32(b, xing + 8) : 0
      const bytes = flags & 2 ? u32(b, xing + 8 + (flags & 1 ? 4 : 0)) : 0
      if (frames && bytes) return { bitrate: Math.round(bytes * 8 * first.sampleRate / frames / first.samples / 1000), bitrateKind: 'average', sampleRate: first.sampleRate, vbr: marker === 'Xing' }
    }
    return { bitrate: first.bitrate, bitrateKind: 'frame', sampleRate: first.sampleRate, vbr: first.bitrate !== next.bitrate }
  }
  return {}
}

async function prefix(url: string, start: number, length: number, signal: AbortSignal): Promise<Uint8Array> {
  const response = await fetch(url, { headers: { Range: `bytes=${start}-${start + length - 1}` }, signal })
  if (!response.body) throw new Error('Music header unavailable')
  const reader = response.body.getReader()
  const pieces: Uint8Array[] = []
  let size = 0
  try {
    if (!response.ok || (start > 0 && response.status !== 206) || (response.status === 206 && !response.headers.get('Content-Range')?.startsWith(`bytes ${start}-`))) throw new Error('Music header unavailable')
    while (size < length) {
      const next = await reader.read()
      if (next.done) break
      const part = next.value.subarray(0, length - size); pieces.push(part); size += part.length
    }
  } finally { await reader.cancel() }
  const bytes = new Uint8Array(size); let at = 0
  for (const piece of pieces) { bytes.set(piece, at); at += piece.length }
  return bytes
}

export async function readMusicQuality(track: Track, signal: AbortSignal): Promise<MusicQuality> {
  const url = fileAPIForHost(track.hostId).contentUrl(track.entry.path)
  const bytes = await prefix(url, 0, prefixBytes, signal)
  const offset = id3AudioOffset(bytes)
  if (offset >= bytes.length && offset < track.entry.sizeBytes - 4) {
    return parseMusicQuality(await prefix(url, offset, 8192, signal), track.entry.sizeBytes, true)
  }
  return parseMusicQuality(bytes, track.entry.sizeBytes)
}

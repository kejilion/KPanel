import { afterEach, expect, it, vi } from 'vitest'
import { id3AudioOffset, parseMusicQuality, readMusicQuality } from './musicQuality'
import type { Track } from './music'
vi.mock('./fileHostContext', () => ({ fileAPIForHost: (host: string) => ({ contentUrl: (path: string) => 'https://fixture.test/' + host + path }) }))
afterEach(() => vi.unstubAllGlobals())
const track: Track = { hostId: 'node-a', title: 'song', artist: '', entry: { name: 'song.mp3', path: '/music/song.mp3', kind: 'file', sizeBytes: 1000000, resourceVersion: 'v1', mode: '', owner: '', group: '', modifiedAt: '', editable: false, previewable: true } }
function mp3() { const b = new Uint8Array(2200); b.set([255,251,224,0]); b.set([255,251,224,0],1044); return b }
it('reads sampled MPEG-1 Layer III bitrate and rejects missing or reserved frame headers', () => {
  expect(parseMusicQuality(mp3(),1000000)).toMatchObject({ bitrate:320,bitrateKind:'frame',sampleRate:44100 })
  const b=mp3(); b[1044]=0
  expect(parseMusicQuality(b,1000000)).toEqual({})
  expect(parseMusicQuality(new Uint8Array([255,251,0,0]),1000)).toEqual({})
})
it('reads Xing VBR average bitrate instead of presenting the first frame as the average', () => {
  const b=mp3(),v=new DataView(b.buffer);b.set(new TextEncoder().encode('Xing'),36)
  v.setUint32(40,3);v.setUint32(44,100);v.setUint32(48,60000)
  expect(parseMusicQuality(b,60000)).toMatchObject({bitrate:184,bitrateKind:'average',vbr:true})
})
it('reads FLAC STREAMINFO sample rate, bit depth and average file bitrate', () => {
  const b=new Uint8Array(42);b.set(new TextEncoder().encode('fLaC'));b[4]=128;b[7]=34
  new DataView(b.buffer).setBigUint64(18,(96000n<<44n)|(1n<<41n)|(23n<<36n)|9600000n)
  expect(parseMusicQuality(b,12000000)).toEqual({lossless:true,sampleRate:96000,bitsPerSample:24,bitrate:960,bitrateKind:'average'})
  expect(parseMusicQuality(b.subarray(0,20),12000000)).toEqual({})
})
it('distinguishes PCM WAV from a WAV container carrying compressed audio', () => {
  const b=new Uint8Array(44),v=new DataView(b.buffer)
  b.set(new TextEncoder().encode('RIFF'));b.set(new TextEncoder().encode('WAVEfmt '),8)
  v.setUint32(16,16,true);v.setUint16(20,1,true);v.setUint16(22,2,true);v.setUint32(24,44100,true);v.setUint32(28,176400,true);v.setUint16(34,16,true)
  expect(parseMusicQuality(b,1000000)).toMatchObject({lossless:true,bitrate:1411,bitrateKind:'pcm',bitsPerSample:16})
  v.setUint16(20,85,true);expect(parseMusicQuality(b,1000000)).toEqual({})
})
it('bounds oversized responses and skips large ID3 tags with a second bounded range on the same host', async () => {
  const head=new Uint8Array(65536);head.set([73,68,51,4,0,0,0,4,0,0]) // 65536-byte tag + 10-byte header.
  const fetcher=vi.fn().mockResolvedValueOnce(new Response(head,{status:206,headers:{'Content-Range':'bytes 0-65535/1000000'}})).mockResolvedValueOnce(new Response(mp3(),{status:206,headers:{'Content-Range':'bytes 65546-67745/1000000'}}))
  vi.stubGlobal('fetch',fetcher)
  expect(id3AudioOffset(head)).toBe(65546)
  expect(await readMusicQuality(track,new AbortController().signal)).toMatchObject({bitrate:320})
  expect(fetcher.mock.calls[0]?.[0]).toContain('/node-a/music/song.mp3')
  expect(fetcher.mock.calls[1]?.[1].headers.Range).toBe('bytes=65546-73737')
})
it('refuses a server that ignores a nonzero range instead of parsing the wrong bytes', async () => {
  const head=new Uint8Array(10);head.set([73,68,51,4,0,0,0,4,0,0])
  vi.stubGlobal('fetch',vi.fn().mockResolvedValueOnce(new Response(head)).mockResolvedValueOnce(new Response(mp3())))
  expect(await readMusicQuality(track,new AbortController().signal)).toEqual({})
})

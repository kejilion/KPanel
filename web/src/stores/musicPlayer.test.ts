// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createMusicPlayer } from './musicPlayer'
import type { Track } from '@/lib/music'
import type { MusicTags } from '@/lib/musicTags'
import type { OnlineMusic, matchOnlineMusic } from '@/lib/musicOnline'
vi.mock('@/lib/musicOnline', () => ({ matchOnlineMusic: vi.fn(async () => ({ matched: false })) }))

function track(name: string, hostId = ''): Track {
  return { hostId, title: name, artist: 'Artist', entry: { name: `${name}.flac`, path: `/music/${name}.flac`, kind: 'file', sizeBytes: 100, mode: '', owner: '', group: '', modifiedAt: '', resourceVersion: name, editable: false, previewable: true } }
}
class FakeAudio {
  src = ''; preload = ''; volume = 1; muted = false; currentTime = 0; duration = 100; error = null
  onplaying: (() => void) | null = null; onpause: (() => void) | null = null
  onwaiting: (() => void) | null = null; oncanplay: (() => void) | null = null
  ontimeupdate: (() => void) | null = null; onloadedmetadata: (() => void) | null = null
  onended: (() => void) | null = null; onerror: (() => void) | null = null
  play = vi.fn(async () => { this.onplaying?.() })
  pause = vi.fn(() => { this.onpause?.() })
  load = vi.fn()
  removeAttribute = vi.fn(() => { this.src = '' })
}
const players: ReturnType<typeof createMusicPlayer>[] = []
function setup(tags = vi.fn(async (): Promise<MusicTags> => ({})), text = vi.fn(async () => ''), online: typeof matchOnlineMusic = vi.fn(async (): Promise<OnlineMusic> => ({ matched: false }))) {
  const audios: FakeAudio[] = []
  const player = createMusicPlayer({ audio: () => { const audio = new FakeAudio(); audios.push(audio); return audio as unknown as HTMLAudioElement }, tags, text, online, random: () => 0 })
  players.push(player)
  return { player, audios, tags, text, online }
}
afterEach(() => { players.splice(0).forEach(player => player.stop()); vi.unstubAllGlobals(); vi.restoreAllMocks() })
describe('music listening session', () => {
  it('retries a failed restored track at its saved position', () => {
    const { player, audios } = setup()
    player.restore(track('A'), 42, 'off', false)
    audios[0]!.onerror?.()
    player.retry()
    expect(player.state.position).toBe(42)
    audios[1]!.onloadedmetadata?.()
    expect(audios[1]!.currentTime).toBe(42)
    expect(audios[1]!.play).toHaveBeenCalledTimes(1)
  })
  it('restores a checkpoint without autoplay and seeks when metadata arrives', async () => {
    const { player, audios } = setup()
    player.restore(track('A', 'edge'), 42, 'all', true)
    expect(player.state.current?.hostId).toBe('edge')
    expect(player.state.position).toBe(42)
    expect(player.state.restored).toBe(true)
    expect(player.state.playing).toBe(false)
    expect(audios[0]!.play).not.toHaveBeenCalled()
    audios[0]!.onloadedmetadata?.()
    expect(audios[0]!.currentTime).toBe(42)
    player.toggle(); await flushPromises()
    expect(audios[0]!.play).toHaveBeenCalledTimes(1)
    expect(player.state.restored).toBe(false)
    expect(player.state.repeat).toBe('all')
    expect(player.state.shuffle).toBe(true)
  })
  it('connects system media controls to playback, tracks and the progress bar', () => {
    const handlers = new Map<MediaSessionAction, MediaSessionActionHandler | null>()
    const session = { setActionHandler: vi.fn((action: MediaSessionAction, handler: MediaSessionActionHandler | null) => handlers.set(action, handler)), setPositionState: vi.fn(), playbackState: 'none' }
    vi.stubGlobal('navigator', { mediaSession: session })
    const { player, audios } = setup()
    player.replaceQueue([track('A'), track('B')])
    expect(handlers.get('seekbackward')).toBeNull()
    expect(handlers.get('seekforward')).toBeNull()
    audios[0]!.onloadedmetadata?.()
    handlers.get('seekto')?.({ action: 'seekto', seekTime: 25 })
    expect(player.state.position).toBe(25)
    handlers.get('pause')?.({ action: 'pause' })
    expect(player.state.playing).toBe(false)
    handlers.get('play')?.({ action: 'play' })
    expect(player.state.playing).toBe(true)
    handlers.get('nexttrack')?.({ action: 'nexttrack' })
    expect(player.state.current?.title).toBe('B')
    handlers.get('previoustrack')?.({ action: 'previoustrack' })
    expect(player.state.current?.title).toBe('A')
    player.stop()
    expect(handlers.get('nexttrack')).toBeNull()
    expect(handlers.get('previoustrack')).toBeNull()
  })
  it('still registers track controls if the browser does not support skip actions', () => {
    const setActionHandler = vi.fn((action: MediaSessionAction) => {
      if (action === 'seekbackward' || action === 'seekforward') throw new DOMException('Unsupported', 'NotSupportedError')
    })
    vi.stubGlobal('navigator', { mediaSession: { setActionHandler } })
    const { player } = setup()
    player.replaceQueue([track('A'), track('B')])
    expect(setActionHandler).toHaveBeenCalledWith('previoustrack', expect.any(Function))
    expect(setActionHandler).toHaveBeenCalledWith('nexttrack', expect.any(Function))
    expect(setActionHandler).toHaveBeenCalledWith('seekto', expect.any(Function))
  })
  it('keeps source host immutable when different hosts share a path', () => {
    const { player, audios } = setup()
    player.replaceQueue([track('same', 'a'), track('same', 'b')])
    expect(player.state.queue).toHaveLength(2)
    expect(audios[0]!.src).toContain('a')
    player.next()
    expect(player.state.current?.hostId).toBe('b')
    expect(audios[0]!.pause).toHaveBeenCalled()
    expect(audios[0]!.src).toBe('')
  })
  it('does not replace a curated queue when selecting another song', () => {
    const { player } = setup()
    player.replaceQueue([track('A'), track('B')]); player.choose(track('C', 'edge'))
    expect(player.state.queue.map(t => t.title)).toEqual(['A', 'B', 'C'])
    expect(player.state.current?.title).toBe('C')
  })
  it('honors play-next even when shuffle is enabled and moves existing entries', () => {
    const { player } = setup()
    player.replaceQueue([track('A'), track('B'), track('C')]); player.shuffle(true)
    player.enqueue(track('C'), true); player.next()
    expect(player.state.current?.title).toBe('C')
    expect(player.state.queue).toHaveLength(3)
    player.previous(); expect(player.state.current?.title).toBe('A')
  })
  it('plays a shuffle cycle without repetition even beyond the bounded back history', () => {
    const { player, audios } = setup()
    player.shuffle(true); player.replaceQueue(Array.from({ length: 220 }, (_, i) => track(String(i))))
    const heard = new Set([player.state.current!.title])
    for (let i = 1; i < 220; i++) { audios.at(-1)!.onended?.(); heard.add(player.state.current!.title) }
    expect(heard.size).toBe(220)
    const count = audios.length; audios.at(-1)!.onended?.()
    expect(audios).toHaveLength(count); expect(player.state.playing).toBe(false)
  }, 15000)
  it('stops at the end of ordered playback, loops the queue or repeats one explicitly', () => {
    const { player, audios } = setup()
    player.replaceQueue([track('A'), track('B')]); audios.at(-1)!.onended?.(); audios.at(-1)!.onended?.()
    expect(player.state.current?.title).toBe('B'); expect(player.state.playing).toBe(false)
    player.state.repeat = 'all'; audios.at(-1)!.onended?.(); expect(player.state.current?.title).toBe('A')
    player.state.repeat = 'one'; audios.at(-1)!.onended?.(); expect(player.state.current?.title).toBe('A')
  })
  it('does not replay a manually selected song later in the same shuffle cycle', () => {
    const { player, audios } = setup()
    player.shuffle(true); player.replaceQueue(['A', 'B', 'C', 'D'].map(name => track(name)))
    player.next(); player.choose(track('D')); audios.at(-1)!.onended?.()
    expect(player.state.current?.title).toBe('C')
    const count = audios.length; audios.at(-1)!.onended?.()
    expect(audios).toHaveLength(count)
  })
  it('can pause a pending play request without displaying a decoding error', async () => {
    const { player, audios } = setup()
    player.play(track('A')); player.pause()
    let reject!: (reason: unknown) => void
    audios[0]!.play.mockImplementationOnce(() => new Promise((_resolve, r) => { reject = r }))
    player.toggle(); audios[0]!.onwaiting?.(); player.toggle()
    reject(new DOMException('Playback interrupted', 'AbortError')); await flushPromises()
    expect(player.state.playing).toBe(false); expect(player.state.waiting).toBe(false); expect(player.state.error).toBe('')
  })
  it('removing the current item does not interrupt it or skip its successor', () => {
    const { player, audios } = setup()
    player.replaceQueue([track('A'), track('B'), track('C')]); player.next(); player.remove(track('B'))
    expect(player.state.current?.title).toBe('B'); expect(player.state.playing).toBe(true)
    audios.at(-1)!.onended?.(); expect(player.state.current?.title).toBe('C')
  })
  it('reorders upcoming songs and clears upcoming without stopping the current track', () => {
    const { player } = setup()
    player.replaceQueue([track('A'), track('B'), track('C')]); player.move(track('C'), -1)
    expect(player.state.queue.map(t => t.title)).toEqual(['A', 'C', 'B'])
    player.clearQueue(); expect(player.state.queue.map(t => t.title)).toEqual(['A']); expect(player.state.playing).toBe(true)
  })
  it('keeps the next song when an earlier entry is removed after the current one', () => {
    const { player, audios } = setup()
    player.replaceQueue(['A', 'B', 'C', 'D'].map(name => track(name))); player.next(); player.next()
    player.remove(track('C')); player.remove(track('A')); audios.at(-1)!.onended?.()
    expect(player.state.current?.title).toBe('D')
  })
  it('ignores stale metadata and queued events after a track changes', async () => {
    let resolve!: (value: MusicTags) => void
    const tags = vi.fn<() => Promise<MusicTags>>().mockImplementationOnce(() => new Promise(r => { resolve = r })).mockResolvedValue({ title: 'New title' })
    const { player, audios } = setup(tags)
    player.play(track('Old')); const stale = audios[0]!.onended
    player.play(track('New')); resolve({ title: 'Stale title', lyrics: 'Old lyrics' }); stale?.()
    await flushPromises()
    expect(player.state.current?.title).toBe('New title'); expect(player.state.lyrics).toBe(''); expect(audios).toHaveLength(2)
  })
  it('keeps readable filenames when embedded tags are visibly corrupted', async () => {
    const { player } = setup(vi.fn(async () => ({ title: 'ïèÇài', artist: 'ïÉæèÌÀ', album: 'bad\ufffd' })))
    player.play({ ...track('风歌青天'), artist: '影视原声' })
    await flushPromises()
    expect(player.state.current?.title).toBe('风歌青天')
    expect(player.state.current?.artist).toBe('影视原声')
    expect(player.state.album).toBe('')
  })
  it('retains valid accented artists even with Chinese filenames', async () => {
    const { player } = setup(vi.fn(async () => ({ title: '情歌', artist: 'Björk' })))
    player.play({ ...track('情歌'), artist: '歌手' }); await flushPromises()
    expect(player.state.current?.artist).toBe('Björk')
  })
  it('uses embedded lyrics when the sidecar fails and clamps seek/volume', async () => {
    const { player, audios } = setup(vi.fn(async () => ({ lyrics: 'embedded' })), vi.fn(async () => { throw new Error('offline') }))
    player.play({ ...track('A'), lyrics: '/music/A.lrc' }); await flushPromises()
    expect(player.state.lyrics).toBe('embedded'); expect(player.state.lyricsFailed).toBe(false)
    audios[0]!.onloadedmetadata?.(); player.seek(999); expect(audios[0]!.currentTime).toBe(100)
    player.setVolume(-1); expect(audios[0]!.volume).toBe(0)
    player.mute(); expect(audios[0]!.muted).toBe(true)
    player.stop(); expect(player.state.current).toBeUndefined(); expect(player.state.queue).toEqual([]); expect(audios[0]!.src).toBe('')
  })
})

describe('optional online metadata', () => {
  it('fills missing lyrics and cover without delaying playback', async () => {
    const online = vi.fn(async () => ({ matched: true, lyrics: '[00:01]online', cover: 'data:image/png;base64,AA==' }))
    const { player, audios } = setup(undefined, undefined, online)
    player.play(track('A')); expect(audios[0]!.play).toHaveBeenCalled()
    await flushPromises()
    expect(online).toHaveBeenCalledWith(expect.objectContaining({ title: 'A', artist: 'Artist' }), expect.any(AbortSignal))
    expect(player.state.lyrics).toBe('[00:01]online'); expect(player.cover.value).toBe('data:image/png;base64,AA==')
    expect(player.state.onlineStatus).toBe('matched')
  })
  it('keeps local resources and skips matching when both are present', async () => {
    const { player, online } = setup(vi.fn(async () => ({ lyrics: 'local' })))
    player.play({ ...track('A'), cover: '/cover.jpg' }); await flushPromises()
    expect(player.state.lyrics).toBe('local'); expect(online).not.toHaveBeenCalled()
  })
  it('only fills the missing resource', async () => {
    const { player } = setup(vi.fn(async () => ({ lyrics: 'local' })), undefined, vi.fn(async () => ({ matched: true, lyrics: 'online', cover: 'data:image/png;base64,AA==' })))
    player.play(track('A')); await flushPromises()
    expect(player.state.lyrics).toBe('local'); expect(player.cover.value).toBe('data:image/png;base64,AA==')
  })
  it('discards old results and cancels requests on a track change or stop', async () => {
    let finish!: (value: OnlineMusic) => void
    const online = vi.fn((_query: import('@/lib/musicOnline').MusicQuery, _signal: AbortSignal) => new Promise<OnlineMusic>(resolve => { finish = resolve }))
    const { player } = setup(undefined, undefined, online)
    player.play(track('A')); await flushPromises()
    const oldSignal = online.mock.calls[0]?.[1] as AbortSignal | undefined
    player.play(track('B')); finish({ matched: true, lyrics: 'old' }); await flushPromises()
    expect(oldSignal?.aborted).toBe(true); expect(player.state.lyrics).toBe('')
    player.stop(); finish({ matched: true, lyrics: 'late' }); await flushPromises()
    expect(player.state.lyrics).toBe(''); expect(player.cover.value).toBe('')
  })
  it('leaves audio playing on a source failure and reports no-match separately', async () => {
    const { player } = setup(undefined, undefined, vi.fn(async () => { throw Error('offline') }))
    player.play(track('A')); await flushPromises()
    expect(player.state.playing).toBe(true); expect(player.state.error).toBe(''); expect(player.state.onlineStatus).toBe('failed')
    const { player: empty } = setup(); empty.play(track('B')); await flushPromises()
    expect(empty.state.onlineStatus).toBe('empty'); expect(empty.state.playing).toBe(true)
  })
})

it('waits for duration when only a local filename is available', async () => {
  const { player, audios, online } = setup()
  player.play({ ...track('Song'), artist: '' }); await flushPromises()
  expect(online).not.toHaveBeenCalled()
  audios[0]!.duration = 205; audios[0]!.onloadedmetadata?.(); await flushPromises()
  expect(online).toHaveBeenCalledWith(expect.objectContaining({ artist: '', duration: 205 }), expect.any(AbortSignal))
})
it('cancels the duration wait when switching tracks', async () => {
  const { player, online } = setup()
  player.play({ ...track('Old'), artist: '' }); await flushPromises()
  player.play(track('New')); await flushPromises()
  expect(online).toHaveBeenCalledTimes(1)
  expect(online).toHaveBeenCalledWith(expect.objectContaining({ title: 'New' }), expect.any(AbortSignal))
})
it('ends the metadata wait within its budget', async () => {
  vi.useFakeTimers()
  try {
    const { player, online } = setup()
    player.play({ ...track('Song'), artist: '' }); await vi.advanceTimersByTimeAsync(1500)
    expect(online).toHaveBeenCalledWith(expect.objectContaining({ duration: 0 }), expect.any(AbortSignal))
  } finally { vi.useRealTimers() }
})

it('retains parsed title and artist in the queue after changing the current track', async () => {
  const { player } = setup(vi.fn(async () => ({ title: 'Parsed', artist: 'Singer' })))
  const a = track('A'), b = track('B')
  player.replaceQueue([a,b],a); await flushPromises()
  expect(player.state.queue[0]).toMatchObject({ title: 'Parsed', artist: 'Singer' })
  player.play(b); await flushPromises()
  expect(player.state.queue[0]).toMatchObject({ title: 'Parsed', artist: 'Singer' })
})

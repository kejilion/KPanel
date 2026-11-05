// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createMusicPlayer } from './musicPlayer'
import type { Track } from '@/lib/music'
import type { MusicTags } from '@/lib/musicTags'

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
function setup(tags = vi.fn(async (): Promise<MusicTags> => ({})), text = vi.fn(async () => '')) {
  const audios: FakeAudio[] = []
  const player = createMusicPlayer({ audio: () => { const audio = new FakeAudio(); audios.push(audio); return audio as unknown as HTMLAudioElement }, tags, text, random: () => 0 })
  players.push(player)
  return { player, audios, tags, text }
}
afterEach(() => { players.splice(0).forEach(player => player.stop()); vi.restoreAllMocks() })
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
  })
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

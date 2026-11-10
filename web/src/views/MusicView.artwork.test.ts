// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { clearMusicInfoMemory, saveMusicInfo } from '@/lib/musicInfoCache'
import { tracksFromEntries } from '@/lib/music'
import * as musicPlayer from '@/stores/musicPlayer'
import type { FileEntry } from '@/types/api'

const harness = vi.hoisted(() => ({ entries: [] as FileEntry[] }))
vi.mock('@/lib/fileHostContext', () => ({ fileAPIForHost: (host: string) => ({
  list: async () => ({ entries: harness.entries }),
  contentUrl: (path: string) => `/content/${host}${path}`,
}) }))
vi.mock('@/lib/useMusicQuality', () => ({ useMusicQuality: () => () => ({ bitrate: 1411, bitrateKind: 'pcm', lossless: true, sampleRate: 44100, bitsPerSample: 16 }) }))
import MusicView from './MusicView.vue'

let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; clearMusicInfoMemory(); vi.restoreAllMocks() })

async function setup(names = ['A.mp3', 'B.mp3']) {
  harness.entries = names.map(name => ({ name, path: `/music/${name}`, kind: 'file',
    sizeBytes: 100, mode: '', owner: '', group: '', modifiedAt: '', resourceVersion: name, editable: false, previewable: true }))
  const player = musicPlayer.createMusicPlayer()
  vi.spyOn(musicPlayer, 'useMusicPlayer').mockReturnValue(player)
  vi.spyOn(api.cluster, 'hosts').mockResolvedValue({ nodeId: 'local', items: [], total: 0, remoteTotal: 0, maxHosts: 100, pollIntervalSeconds: 30 })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/music', name: 'music', component: MusicView }] })
  await router.push('/music?path=/music')
  await router.isReady()
  wrapper = mount(MusicView, { global: { plugins: [router], stubs: { HostSwitcher: true } } })
  await flushPromises()
  return { player, view: wrapper, tracks: tracksFromEntries(harness.entries, ''), rows: () => wrapper!.findAll('.music-tracks .music-track__art') }
}

it('shows the current embedded or online artwork in its library row and isolates other tracks and hosts', async () => {
  const { player, tracks, rows } = await setup()
  expect(rows()[0]!.find('img').exists()).toBe(false)
  player.state.current = tracks[0]; player.state.embeddedCover = 'blob:embedded-A'
  await flushPromises()
  expect(rows()[0]!.get('img').attributes('src')).toBe('blob:embedded-A')
  expect(rows()[1]!.find('img').exists()).toBe(false)
  player.state.current = tracks[1]; player.state.embeddedCover = ''; player.state.onlineCover = 'data:image/png;base64,AAAA'
  await flushPromises()
  expect(rows()[0]!.find('img').exists()).toBe(false)
  expect(rows()[1]!.get('img').attributes('src')).toBe(player.cover.value)
  player.state.current = { ...tracks[1]!, hostId: 'another-host' }
  await flushPromises()
  expect(rows()[1]!.find('img').exists()).toBe(false)
})

it('replaces a failed thumbnail element when a new cover arrives for the same track', async () => {
  const { player, tracks, rows } = await setup()
  player.state.current = { ...tracks[0]!, cover: '/music/broken.jpg' }
  await flushPromises()
  const failed = rows()[0]!.get('img')
  await failed.trigger('error')
  expect((failed.element as HTMLImageElement).hidden).toBe(true)
  player.state.embeddedCover = 'blob:recovered-A'
  await flushPromises()
  const recovered = rows()[0]!.get('img')
  expect(recovered.element).not.toBe(failed.element)
  expect(recovered.attributes('src')).toBe('blob:recovered-A')
  expect((recovered.element as HTMLImageElement).hidden).toBe(false)
})

it('shows current cover and parsed song information in the playback queue', async () => {
  const { player, tracks, view } = await setup()
  player.state.queue = tracks
  player.state.current = { ...tracks[0]!, title: 'Parsed title', artist: 'Parsed artist' }
  player.state.onlineCover = 'data:image/png;base64,AAAA'
  await view.findAll('.music-tabs button')[2]!.trigger('click'); await flushPromises()
  const rows = view.findAll('.music-queue .music-track')
  expect(rows[0]!.get('.music-track__art img').attributes('src')).toBe(player.cover.value)
  expect(rows[0]!.text()).toContain('Parsed title'); expect(rows[0]!.text()).toContain('Parsed artist')
  expect(rows[1]!.find('.music-track__art img').exists()).toBe(false)
})

it('shows bitrate and lossless quality in rows and cover provenance, with compact attribution beside the lyrics title', async () => {
  const { player, tracks, view } = await setup()
  player.state.current = tracks[0]
  player.state.onlineSource = 'qq'; player.state.onlineStatus = 'matched'; player.state.saveStatus = 'failed'
  const retry = vi.spyOn(player, 'retrySave').mockResolvedValue()
  await flushPromises()
  expect(view.get('.music-track__format').text()).toContain('1411 kbps')
  expect(view.get('.music-track__format .music-lossless').text()).toBe('无损')
  expect(view.get('.music-provenance').text()).toContain('1411 kbps')
  expect(view.get('.music-provenance .music-lossless').text()).toBe('无损')
  const title = view.get('.music-lyric-title')
  expect(title.get('h3').element.nextElementSibling?.textContent).toBe('QQ')
  expect(title.get('.music-source-label').attributes('title')).toContain('QQ')
  await title.get('.music-save-retry').trigger('click'); expect(retry).toHaveBeenCalledOnce()
})

it('updates tags in place until the user changes the sort order', async () => {
  vi.spyOn(api.music, 'cache').mockResolvedValue({ items: [] })
  const { view, tracks } = await setup()
  const titles = () => view.findAll('.music-tracks .music-track__name strong').map(row => row.text())
  const firstRow = view.get('.music-tracks li').element
  expect(titles()).toEqual(['A', 'B'])

  await saveMusicInfo(tracks[0]!, { tags: { title: 'Z', artist: 'Band' } }, new AbortController().signal)
  await flushPromises()
  expect(titles()).toEqual(['Z', 'B'])
  expect(view.get('.music-tracks li').element).toBe(firstRow)

  await view.get('.music-search select').setValue('artist')
  expect(titles()).toEqual(['B', 'Z'])
})

it('keeps filename search results after tag lookup and expands long playback titles on demand', async () => {
  vi.spyOn(api.music, 'cache').mockResolvedValue({ items: [] })
  const { player, view, tracks } = await setup(['01 - live take.mp3'])
  const search = view.get('.music-search input')
  await search.setValue('band')
  expect(view.findAll('.music-tracks li')).toHaveLength(0)
  expect(view.get('.music-tracks').classes()).toContain('music-tracks--filtered')
  await saveMusicInfo(tracks[0]!, { tags: { title: 'Concert', artist: 'Band' } }, new AbortController().signal)
  await flushPromises()
  expect(view.findAll('.music-tracks li')).toHaveLength(1)
  expect(view.get('.music-track__name strong').text()).toBe('Concert')
  await search.setValue('live take'); expect(view.findAll('.music-tracks li')).toHaveLength(1)
  await search.setValue('Concert'); expect(view.findAll('.music-tracks li')).toHaveLength(1)
  await search.setValue(''); expect(view.get('.music-tracks').classes()).not.toContain('music-tracks--filtered')

  const title = 'A very long song title '.repeat(5).trim(), artist = 'A very long performer '.repeat(5).trim()
  clearMusicInfoMemory()
  player.state.current = { ...tracks[0]!, title, artist }
  await flushPromises()
  const control = view.get('.music-console__title')
  await control.trigger('click')
  expect(control.attributes('aria-expanded')).toBe('true')
  expect(control.classes()).toContain('is-expanded')
  expect(control.get('strong').text()).toBe(title)
  expect(control.get('span').text()).toBe(artist)
  await control.trigger('click')
  expect(control.attributes('aria-expanded')).toBe('false')
})

async function lyricsSetup() {
  const { player, view } = await setup()
  player.state.lyrics = Array.from({ length: 20 }, (_, i) => '[00:' + String(i).padStart(2, '0') + '.00]Line ' + i).join('\n')
  await flushPromises()
  const panel = view.get('.music-lyrics')
  Object.defineProperty(panel.element, 'clientHeight', { configurable: true, value: 100 })
  panel.findAll('button').forEach((line, i) => {
    Object.defineProperty(line.element, 'offsetTop', { configurable: true, value: i * 40 })
    Object.defineProperty(line.element, 'clientHeight', { configurable: true, value: 40 })
  })
  return { player, view, panel, following: () => view.get('.music-lyric-section .music-section-heading button').attributes('aria-pressed') }
}
it('keeps programmatic following active and pauses only after actual lyric scrolling', async () => {
  const { player, view, panel, following } = await lyricsSetup()
  await view.findAll('.music-tabs button')[1]!.trigger('click'); await flushPromises()
  player.state.position = 6; await flushPromises()
  expect(panel.element.scrollTop).toBe(210)
  await panel.trigger('scroll'); expect(following()).toBe('true')
  await panel.trigger('wheel'); await panel.trigger('touchmove'); expect(following()).toBe('true')
  panel.element.scrollTop = 300; await panel.trigger('scroll'); expect(following()).toBe('false')
  player.state.position = 8; await flushPromises(); expect(panel.element.scrollTop).toBe(300)
  await view.get('.music-lyric-section .music-section-heading button').trigger('click'); await flushPromises()
  expect(following()).toBe('true'); expect(panel.element.scrollTop).toBe(290)
})
it('always follows sidebar lyrics, including after leaving a paused focused view', async () => {
  const { player, view, panel } = await lyricsSetup()
  expect(view.find('.music-section-heading button[aria-pressed]').exists()).toBe(false)
  player.state.position = 6; await flushPromises(); expect(panel.element.scrollTop).toBe(210)
  panel.element.scrollTop = 300; await panel.trigger('scroll'); await flushPromises()
  expect(panel.element.scrollTop).toBe(210)
  await view.findAll('.music-tabs button')[1]!.trigger('click'); await flushPromises()
  panel.element.scrollTop = 300; await panel.trigger('scroll')
  await view.findAll('.music-tabs button')[0]!.trigger('click'); await flushPromises()
  player.state.position = 8; await flushPromises(); expect(panel.element.scrollTop).toBe(290)
})
it('re-centers on entering focused view and resumes following when selecting a lyric', async () => {
  const { player, view, panel, following } = await lyricsSetup()
  player.state.position = 7; await flushPromises()
  panel.element.scrollTop = 0
  await view.findAll('.music-tabs button')[1]!.trigger('click'); await flushPromises()
  expect(panel.element.scrollTop).toBe(250); expect(view.classes()).toContain('music-page--focus')
  panel.element.scrollTop = 320; await panel.trigger('scroll'); expect(following()).toBe('false')
  vi.spyOn(player, 'seek').mockImplementation(time => { player.state.position = time })
  await panel.findAll('button')[10]!.trigger('click'); await flushPromises()
  expect(player.seek).toHaveBeenCalledWith(10); expect(following()).toBe('true'); expect(panel.element.scrollTop).toBe(370)
})

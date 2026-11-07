// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/lib/api'
import type { GalleryItem } from '@/lib/gallery'
import type { CinemaMetadata } from '@/lib/cinemaMetadata'
import CinemaMetadataPanel from './CinemaMetadata.vue'

const film = { entry: { name: 'Inception.2010.mkv', path: '/home/videos/Inception.2010.mkv', resourceVersion: 'v1', modifiedAt: '2026-01-01', sizeBytes: 123, kind: 'file' }, folder: '/home/videos', kind: 'video', time: 1 } as GalleryItem
const movie: CinemaMetadata = { provider: 'wikipedia', id: 23270459, title: 'Inception', year: '2010', overview: 'A dream', kind: 'movie', url: 'https://en.wikipedia.org/?curid=23270459' }
let wrapper: VueWrapper<InstanceType<typeof CinemaMetadataPanel>>
beforeEach(() => { localStorage.clear(); vi.restoreAllMocks(); vi.useFakeTimers() })
afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })
function setup(props: Record<string, unknown> = {}) { wrapper = mount(CinemaMetadataPanel, { props: { items: [film], host: 'host-a', scanning: false, visible: true, ...props } }) }
const tick = async (time = 250) => { await vi.advanceTimersByTimeAsync(time); await flushPromises() }
async function edit() { await wrapper.setProps({ selected: film }); await wrapper.vm.focusSearch(); await flushPromises() }
async function click(label: string) { const button = wrapper.findAll('button').find(b => b.text() === label); expect(button).toBeDefined(); await button!.trigger('click'); await flushPromises() }
describe('automatic cinema metadata', () => {
  it('automatically fills information without exposing a scraping button and reuses it after reopening', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [movie] })
    setup(); expect(wrapper.find('section').exists()).toBe(false); await tick()
    expect(fetch).toHaveBeenCalledWith(expect.objectContaining({ title: 'Inception', provider: 'wikipedia', kind: 'movie' }), expect.any(AbortSignal))
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).get(film.entry.path)?.id).toBe(movie.id)
    wrapper.unmount(); setup(); await tick(); expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('chooses the TV provider and falls back to the encyclopedia if no exact result exists', async () => {
    const episode = { ...film, entry: { ...film.entry, name: 'The.Expanse.S01E01.mkv', path: '/home/videos/The.Expanse.S01E01.mkv' } }
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValueOnce({ items: [] }).mockResolvedValueOnce({ items: [{ ...movie, kind: 'tv', title: 'The Expanse' }] })
    setup({ items: [episode] }); await tick()
    expect(fetch.mock.calls.map(c => c[0].provider)).toEqual(['tvmaze', 'wikipedia'])
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).get(episode.entry.path)?.title).toBe('The Expanse')
  })
  it('keeps ambiguity unresolved instead of replacing it through another source', async () => {
    const episode = { ...film, entry: { ...film.entry, name: 'The.Expanse.S01E01.mkv' } }
    const show = { ...movie, title: 'The Expanse', kind: 'tv' as const, provider: 'tvmaze' as const }
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [show, { ...show, id: 2 }] })
    setup({ items: [episode] }); await tick(); expect(fetch).toHaveBeenCalledTimes(1)
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).size).toBe(0)
  })
  it('cools down misses across refreshes and retries when the file version changes', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [] })
    setup(); await tick(); wrapper.unmount(); setup(); await tick(); expect(fetch).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ items: [{ ...film, entry: { ...film.entry, resourceVersion: 'v2' } }] }); await tick(); expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('continues to newly displayed cards beyond the initial batch', async () => {
    const films = Array.from({ length: 49 }, (_, i) => ({ ...film, entry: { ...film.entry, name: `Film ${i}.mp4`, path: `/home/videos/Film ${i}.mp4` } }))
    const fetch = vi.spyOn(api.cinema, 'metadata').mockImplementation(async q => ({ items: [{ ...movie, title: q.title, year: undefined }] }))
    setup({ items: films, limit: 48 }); await tick(); expect(fetch).toHaveBeenCalledTimes(48)
    await wrapper.setProps({ limit: 49 }); await tick(); expect(fetch).toHaveBeenCalledTimes(49)
  })
  it('prioritizes filtered visible cards and ignores identity-equivalent recomputations', async () => {
    const films = Array.from({ length: 60 }, (_, i) => ({ ...film, entry: { ...film.entry, name: `Film ${i}.mp4`, path: `/home/videos/Film ${i}.mp4` } }))
    const fetch = vi.spyOn(api.cinema, 'metadata').mockImplementation(async q => ({ items: [{ ...movie, title: q.title, year: undefined }] }))
    setup({ items: films, candidates: [films[59]!] }); await tick()
    expect(fetch).toHaveBeenCalledTimes(1); expect(fetch.mock.calls[0]![0].title).toBe('Film 59')
    await wrapper.setProps({ candidates: [...[films[59]!]] }); await tick(); expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('skips a permanently invalid file query so later visible cards still receive metadata', async () => {
    const invalid = { ...film, entry: { ...film.entry, name: 'Invalid.mp4', path: '/home/videos/Invalid.mp4' } }
    const fetch = vi.spyOn(api.cinema, 'metadata').mockRejectedValueOnce(new ApiError('invalid', 400)).mockResolvedValue({ items: [movie] })
    setup({ items: [invalid, film] }); await tick()
    expect(fetch).toHaveBeenCalledTimes(2); expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).get(film.entry.path)?.id).toBe(movie.id)
    await tick(60_000); expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('pauses when hidden and rejects responses from a previous host', async () => {
    let resolve!: (v: { items: CinemaMetadata[] }) => void
    const fetch = vi.spyOn(api.cinema, 'metadata').mockImplementation(() => new Promise(r => { resolve = r }))
    setup({ visible: false }); await tick(); expect(fetch).not.toHaveBeenCalled()
    await wrapper.setProps({ visible: true }); await tick(); await wrapper.setProps({ host: 'host-b', visible: false })
    resolve({ items: [movie] }); await flushPromises(); expect(localStorage.getItem('kpanel:cinema-metadata:v1')).toBeNull()
  })
  it('backs off network failures and releases retry timers on unmount', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockRejectedValue(new Error('offline'))
    setup(); await tick(); await tick(59000); expect(fetch).toHaveBeenCalledTimes(1)
    await tick(1000); expect(fetch).toHaveBeenCalledTimes(2)
    wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
  })
  it('remembers an explicit opt out without requesting a key', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [] })
    setup(); wrapper.vm.openSettings(); await flushPromises(); await wrapper.find('input[type=checkbox]').setValue(false)
    wrapper.unmount(); setup(); await tick(); expect(fetch).not.toHaveBeenCalled()
  })
  it('allows manual search, correction and removal without automatic reapplication', async () => {
    vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [movie] })
    setup(); await edit(); await wrapper.find('form').trigger('submit'); await flushPromises(); await click('使用此资料')
    expect(wrapper.find('.metadata__selected').text()).toContain('Inception')
    await click('清除匹配'); await click('关闭'); await tick()
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).size).toBe(0)
  })
  it('uses a TMDB credential only in requests and clears it on scope changes', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [] })
    setup(); await edit(); await wrapper.find('select').setValue('tmdb'); await wrapper.find('input[type=password]').setValue('private.read.access.token')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(fetch).toHaveBeenCalledWith(expect.objectContaining({ credential: 'private.read.access.token' }), expect.any(AbortSignal))
    expect(JSON.stringify(localStorage)).not.toContain('private.read.access.token')
    await wrapper.setProps({ host: 'host-b' }); expect((wrapper.find('input[type=password]').element as HTMLInputElement).value).toBe('')
  })
})

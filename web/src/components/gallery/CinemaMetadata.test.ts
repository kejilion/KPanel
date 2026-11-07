// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import type { GalleryItem } from '@/lib/gallery'
import type { CinemaMetadata } from '@/lib/cinemaMetadata'
import CinemaMetadataPanel from './CinemaMetadata.vue'

const film = { entry: { name: 'Inception.2010.mkv', path: '/home/videos/Inception.2010.mkv', resourceVersion: 'v1', modifiedAt: '2026-01-01', sizeBytes: 123, kind: 'file' }, folder: '/home/videos', kind: 'video', time: 1 } as GalleryItem
const movie: CinemaMetadata = { provider: 'wikipedia', id: 23270459, title: 'Inception', year: '2010', overview: 'A dream', kind: 'movie', url: 'https://en.wikipedia.org/?curid=23270459' }
let wrapper: VueWrapper
const click = async (text: string) => { const button = wrapper.findAll('button').find(b => b.text().includes(text)); expect(button).toBeDefined(); await button!.trigger('click'); await flushPromises() }
beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })
afterEach(() => wrapper?.unmount())
function setup(selected = true) {
  wrapper = mount(CinemaMetadataPanel, { props: { items: [film], host: 'host-a', selected: selected ? film : undefined, scanning: false, visible: true } })
}
describe('cinema lookup journey', () => {
  it('searches, applies metadata, reloads saved matches and clears them without file writes', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [movie] })
    setup(); await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(fetch).toHaveBeenCalledWith(expect.objectContaining({ title: 'Inception', provider: 'wikipedia' }), expect.any(AbortSignal))
    await click('使用此资料')
    const updates = wrapper.emitted('update')!
    expect((updates.at(-1)![0] as Map<string, CinemaMetadata>).get(film.entry.path)?.id).toBe(movie.id)
    wrapper.unmount(); setup()
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).size).toBe(1)
    await click('清除匹配')
    expect((wrapper.emitted('update')!.at(-1)![0] as Map<string, CinemaMetadata>).size).toBe(0)
  })
  it('does not let a response from another host update the new library', async () => {
    let resolve!: (value: { items: CinemaMetadata[] }) => void
    vi.spyOn(api.cinema, 'metadata').mockImplementation(() => new Promise(r => { resolve = r }))
    setup(); await wrapper.find('form').trigger('submit'); await flushPromises()
    await wrapper.setProps({ host: 'host-b', selected: undefined })
    resolve({ items: [movie] }); await flushPromises()
    expect(wrapper.find('.metadata__results').exists()).toBe(false)
    expect(localStorage.getItem('kpanel:cinema-metadata:v1')).toBeNull()
  })
  it('reports failure, then permits retry', async () => {
    vi.spyOn(api.cinema, 'metadata').mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce({ items: [movie] })
    setup(); await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(wrapper.find('[role=alert]').text()).toContain('刮削失败')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(wrapper.find('.metadata__results').text()).toContain('Inception')
  })
  it('automatically applies exact matches and leaves ambiguous matches untouched', async () => {
    vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [movie, { ...movie, id: 2 }] })
    setup(); await wrapper.find('input[type=checkbox]').setValue(true); await flushPromises()
    expect(localStorage.getItem('kpanel:cinema-metadata:v1')).toBeNull()
    vi.mocked(api.cinema.metadata).mockResolvedValue({ items: [movie] })
    await click('重新刮削')
    expect(JSON.parse(localStorage.getItem('kpanel:cinema-metadata:v1')!)).toHaveLength(1)
    expect(wrapper.text()).toContain('本轮刮削完成')
  })
  it('uses a TMDB token only for requests and clears it on host changes', async () => {
    const fetch = vi.spyOn(api.cinema, 'metadata').mockResolvedValue({ items: [] })
    setup(); await wrapper.find('select').setValue('tmdb'); await wrapper.find('input[type=password]').setValue('private.read.access.token')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(fetch).toHaveBeenCalledWith(expect.objectContaining({ credential: 'private.read.access.token' }), expect.any(AbortSignal))
    expect(JSON.stringify(localStorage)).not.toContain('private.read.access.token')
    await wrapper.setProps({ host: 'host-b' })
    expect((wrapper.find('input[type=password]').element as HTMLInputElement).value).toBe('')
  })
})

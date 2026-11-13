// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import MusicTrackList from './MusicTrackList.vue'
import { trackKey, type Track } from '@/lib/music'

let callback: ResizeObserverCallback
const disconnect = vi.fn()
vi.stubGlobal('ResizeObserver', class {
  constructor(value: ResizeObserverCallback) { callback = value }
  observe() {}
  disconnect = disconnect
})
const mounted: ReturnType<typeof mount>[] = []
afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()); document.body.replaceChildren(); vi.restoreAllMocks() })
function tracks(count: number): Track[] {
  return Array.from({ length: count }, (_, index) => ({ hostId: '', title: `Song ${index}`, artist: '', entry: {
    name: `${index}.mp3`, path: `/music/${index}.mp3`, kind: 'file', sizeBytes: 100, resourceVersion: '1',
    mode: '', owner: '', group: '', modifiedAt: '', editable: false, previewable: true,
  } }))
}
async function setup() {
  const root = document.createElement('div'); document.body.append(root)
  Object.defineProperty(root, 'clientHeight', { value: 300, configurable: true })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this.hasAttribute('data-index')) {
      const top = Number(this.getAttribute('data-index')) * 76 - root.scrollTop
      return { top, bottom: top + 76, left: 0, width: 800, height: 76 } as DOMRect
    }
    return { top: this === root ? 0 : -root.scrollTop, left: 0, width: 800, height: 300 } as DOMRect
  })
  const wrapper = mount(MusicTrackList, { attachTo: root, props: { tracks: tracks(500), scrollRoot: root },
    slots: { default: ({ track, index }: { track: Track; index: number }) => h('li', { 'data-index': index }, track.title) } })
  mounted.push(wrapper)
  await flushPromises()
  await vi.waitFor(() => expect(wrapper.findAll('[data-index]').length).toBeLessThan(40))
  return { root, wrapper }
}
it('mounts a bounded window and reaches the last song without mounting the whole library', async () => {
  const { root, wrapper } = await setup()
  expect(wrapper.find('[data-index="0"]').exists()).toBe(true)
  expect(wrapper.find('[data-index="499"]').exists()).toBe(false)
  root.scrollTop = 76 * 490; root.dispatchEvent(new Event('scroll'))
  await vi.waitFor(() => expect(wrapper.find('[data-index="499"]').exists()).toBe(true))
  expect(wrapper.findAll('[data-index]').length).toBeLessThan(180)
  expect(wrapper.get('[data-music-spacer]').attributes('aria-hidden')).toBe('true')
  expect(wrapper.emitted('rangeChange')!.length).toBeGreaterThan(1)
})
it('uses measured row heights, responds to resize and keeps filtered results reachable', async () => {
  const { root, wrapper } = await setup()
  callback([0, 1].map((index) => ({ target: wrapper.get(`[data-index="${index}"]`).element,
    borderBoxSize: [{ blockSize: index === 0 ? 100 : 200 }] } as unknown as ResizeObserverEntry)), {} as ResizeObserver)
  await flushPromises()
  root.scrollTop = 10000
  callback([{ target: root } as unknown as ResizeObserverEntry], {} as ResizeObserver)
  await vi.waitFor(() => expect(wrapper.findAll('[data-index]')[0]!.attributes('data-index')).toBe('108'))
  callback([{ target: wrapper.get('[data-index="108"]').element, borderBoxSize: [{ blockSize: 80 }] } as unknown as ResizeObserverEntry], {} as ResizeObserver)
  expect(root.scrollTop).toBe(10004)
  const previous = wrapper.findAll('[data-index]').length
  Object.defineProperty(root, 'clientHeight', { value: 900, configurable: true })
  callback([{ target: root } as unknown as ResizeObserverEntry], {} as ResizeObserver)
  await vi.waitFor(() => expect(wrapper.findAll('[data-index]').length).toBeGreaterThan(previous))
  await wrapper.setProps({ tracks: tracks(1) })
  expect(wrapper.findAll('[data-index]')).toHaveLength(1)
  expect(wrapper.get('[data-index]').text()).toBe('Song 0')
})

it('updates the mounted range on scroll without waiting for another animation frame', async () => {
  const { root, wrapper } = await setup()
  vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(0)
  root.scrollTop = 10000; root.dispatchEvent(new Event('scroll'))
  await flushPromises()
  expect(Number(wrapper.findAll('[data-index]')[0]!.attributes('data-index'))).toBeGreaterThan(80)
  expect(wrapper.findAll('[data-index]').length).toBeLessThan(180)
})

it('reveals a changed song outside the mounted range and lets the user browse afterwards', async () => {
  const { root, wrapper } = await setup()
  await wrapper.setProps({ followKey: trackKey(wrapper.props('tracks')[490]!) })
  await flushPromises()
  expect(root.scrollTop).toBeGreaterThan(36000)
  const bounds = wrapper.get('[data-index="490"]').element.getBoundingClientRect()
  expect(bounds.top).toBeGreaterThanOrEqual(0); expect(bounds.bottom).toBeLessThanOrEqual(300)
  root.scrollTop = 10000; root.dispatchEvent(new Event('scroll'))
  await wrapper.setProps({ tracks: [...wrapper.props('tracks')] })
  await flushPromises()
  expect(root.scrollTop).toBe(10000)
  expect(wrapper.findAll('[data-index]').length).toBeLessThan(80)
})

it('keeps visible songs and missing search results steady, and follows only the latest song', async () => {
  const { root, wrapper } = await setup()
  const available = wrapper.props('tracks')
  await wrapper.setProps({ followKey: trackKey(available[0]!) }); await flushPromises()
  expect(root.scrollTop).toBe(0)
  await wrapper.setProps({ followKey: trackKey({ ...available[0]!, hostId: 'other-host' }) }); await flushPromises()
  expect(root.scrollTop).toBe(0)
  void wrapper.setProps({ followKey: trackKey(available[490]!) })
  await wrapper.setProps({ followKey: trackKey(available[10]!) }); await flushPromises()
  expect(root.scrollTop).toBeLessThan(1000)
  expect(wrapper.find('[data-index="10"]').exists()).toBe(true)
})

// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ClusterShareTheme from './ClusterShareTheme.vue'
import type { PublicClusterShareSnapshot } from '@/types/api'

const wrappers: ReturnType<typeof mount>[] = []
const snapshot: PublicClusterShareSnapshot = { title: 'Fleet', generatedAt: '2026-01-01', total: 0, online: 0, attention: 0, items: [],
  theme: { id: 'minimal', name: { 'zh-CN': '简约看板' }, fileBase: `/api/v1/cluster/share-themes/minimal/files/${'b'.repeat(32)}/` } }
const render = () => {
  const wrapper = mount(ClusterShareTheme, { props: { snapshot }, slots: { default: '<p class="native">Native page</p>' }, attachTo: document.body })
  wrappers.push(wrapper); return wrapper
}
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })
describe('isolated public theme', () => {
  it('only sends whitelisted public data after the exact opaque frame is ready', async () => {
    const wrapper = render(), frame = wrapper.get('iframe').element as HTMLIFrameElement
    expect(frame.sandbox?.toString() || frame.getAttribute('sandbox')).toBe('allow-scripts')
    expect(frame.getAttribute('referrerpolicy')).toBe('no-referrer')
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    const data = { source: 'kpanel-share-theme', type: 'ready' }
    window.dispatchEvent(new MessageEvent('message', { source: window, origin: 'null', data }))
    window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, origin: 'https://evil.test', data }))
    expect(post).not.toHaveBeenCalled()
    window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, origin: 'null', data }))
    await wrapper.vm.$nextTick()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ source: 'kpanel-share', schema: 1, data: expect.objectContaining({ title: 'Fleet' }) }), '*')
    expect(wrapper.find('.native').exists()).toBe(false)
    window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, origin: 'null', data: { source: 'kpanel-share-theme', type: 'resize', height: 1200 } }))
    await wrapper.vm.$nextTick()
    expect(frame.style.getPropertyValue('--theme-height')).toBe('1200px')
    for (const height of [-1, 1000000, '1000', NaN, 500.5]) {
      window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, origin: 'null', data: { source: 'kpanel-share-theme', type: 'resize', height } }))
    }
    await wrapper.vm.$nextTick()
    expect(frame.style.getPropertyValue('--theme-height')).toBe('1200px')
    await wrapper.setProps({ errorMessage: 'Refresh failed' })
    expect(wrapper.get('[role="alert"]').text()).toBe('Refresh failed')
    await wrapper.setProps({ snapshot: undefined })
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('.native').exists()).toBe(true)
  })
  it('times out to default and removes timers on unmount', async () => {
    vi.useFakeTimers()
    const wrapper = render()
    await vi.advanceTimersByTimeAsync(12_001)
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.text()).toContain('主题加载失败')
    expect(wrapper.find('.native').exists()).toBe(true)
    wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
  })
  it('lets a visitor recover without changing admin selection', async () => {
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('.native').exists()).toBe(true)
    expect(snapshot.theme?.id).toBe('minimal')
  })
  it('answers a protocol 2 package with the schema 2 snapshot and labels, and legacy packages with schema 1', async () => {
    const wrapper = render(), frame = wrapper.get('iframe').element as HTMLIFrameElement
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    window.dispatchEvent(new MessageEvent('message', { source: frame.contentWindow, origin: 'null', data: { source: 'kpanel-share-theme', type: 'ready', protocol: 2 } }))
    await wrapper.vm.$nextTick()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ schema: 2, labels: expect.objectContaining({ online: expect.any(String) }),
      data: expect.objectContaining({ counts: expect.objectContaining({ total: 0 }) }) }), '*')
    expect(JSON.stringify(post.mock.calls[0]![0])).not.toContain('secret')
    // Flags, system marks and region anchors arrive in a follow-up snapshot once the lazy asset chunk resolves.
    await vi.waitFor(() => expect(post.mock.calls.length).toBeGreaterThan(1))
  })
})

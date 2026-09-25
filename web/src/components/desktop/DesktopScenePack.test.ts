// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopScenePack from '@/components/desktop/DesktopScenePack.vue'
import { api } from '@/lib/api'
import { DESKTOP_SCENE_MOTION_KEY, resetSceneMotionPreferenceForTest } from '@/lib/desktopScenes/motionPreference'
import type { ScenePack } from '@/lib/scenePacks'

const installed: ScenePack = {
  id: 'orbital-station',
  version: '1.0.0',
  name: { 'zh-CN': '星港轨道', 'en-US': 'Orbital Station' },
  description: { 'zh-CN': '行星轨道', 'en-US': 'Planet orbit' },
  author: { name: 'KPanel' },
  license: 'MIT',
  tags: ['space'],
  theme: { brand: '#2f6fd0', neutral: '#33415a', signature: '#35b8d6' },
  cameras: [
    { id: 'panorama', name: { 'zh-CN': '全景' } },
    { id: 'station', name: { 'zh-CN': '近景' } },
    { id: 'sunrise', name: { 'zh-CN': '日出' } },
  ],
  sizeBytes: 650_000,
  installed: true,
  installedVersion: '1.0.0',
  fileBase: `/api/v1/desktop/scene-packs/orbital-station/files/${'a'.repeat(32)}/`,
  resourceVersion: `sha256:${'a'.repeat(64)}`,
}

function listPacks(packs: ScenePack[]) {
  return vi.spyOn(api.desktop, 'scenePacks').mockResolvedValue({ source: 'auto', sources: ['auto', 'github', 'mirror'], packs })
}

function frame(wrapper: VueWrapper): HTMLIFrameElement {
  return wrapper.get('iframe').element as HTMLIFrameElement
}

function fromPack(wrapper: VueWrapper, data: unknown): void {
  window.dispatchEvent(new MessageEvent('message', { data, source: frame(wrapper).contentWindow }))
}

async function mountPack(covered = false) {
  const wrapper = mount(DesktopScenePack, { props: { packId: 'orbital-station', covered }, attachTo: document.body })
  await flushPromises()
  return wrapper
}

describe('DesktopScenePack', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.stubGlobal('matchMedia', undefined)
    resetSceneMotionPreferenceForTest()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetSceneMotionPreferenceForTest()
  })

  it('starts a live scene from black in a scripts-only sandbox', async () => {
    listPacks([installed])
    const wrapper = await mountPack()
    expect(wrapper.find('img').exists()).toBe(false)
    const iframe = wrapper.get('iframe')
    expect(iframe.attributes('src')).toBe(`${installed.fileBase}index.html`)
    expect(iframe.attributes('sandbox')).toBe('allow-scripts')
    expect(iframe.attributes('referrerpolicy')).toBe('no-referrer')
    expect(iframe.attributes('allow')).toBe('')
    expect(iframe.attributes('tabindex')).toBe('-1')
    expect(wrapper.attributes('aria-hidden')).toBe('true')
    expect(wrapper.attributes('data-scene-pack-state')).toBe('loading')
    wrapper.unmount()
  })

  it('reveals the scene on ready and drives it with pause, resume and camera commands', async () => {
    listPacks([installed])
    const wrapper = await mountPack()
    const post = vi.spyOn(frame(wrapper).contentWindow!, 'postMessage')

    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'ready', cameras: ['panorama', 'station', 'sunrise'] })
    await nextTick()
    expect(wrapper.emitted('cameras')).toEqual([[['panorama', 'station', 'sunrise']]])
    expect(wrapper.attributes('data-scene-pack-state')).toBe('running')
    expect(wrapper.classes()).toContain('desktop-scene-pack--ready')
    expect(post).toHaveBeenLastCalledWith({ source: 'kpanel-desktop', type: 'resume' }, '*')

    await wrapper.setProps({ covered: true })
    expect(post).toHaveBeenLastCalledWith({ source: 'kpanel-desktop', type: 'pause' }, '*')
    await wrapper.setProps({ covered: false })
    expect(post).toHaveBeenLastCalledWith({ source: 'kpanel-desktop', type: 'resume' }, '*')

    ;(wrapper.vm as unknown as { nextCamera: () => void }).nextCamera()
    expect(post).toHaveBeenLastCalledWith({ source: 'kpanel-desktop', type: 'camera' }, '*')

    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'camera', index: 1 })
    expect(wrapper.emitted('camera')).toEqual([[1]])
    wrapper.unmount()
  })

  it('hides the scene before a keyboard reload so the torn-down frame never flashes', async () => {
    listPacks([installed])
    const wrapper = await mountPack()
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    const hard = new KeyboardEvent('keydown', { key: 'F5', shiftKey: true, cancelable: true })
    window.dispatchEvent(hard)
    expect(hard.defaultPrevented).toBe(false)
    expect(frame(wrapper).style.visibility).toBe('')

    const reload = new KeyboardEvent('keydown', { key: 'r', ctrlKey: true, cancelable: true })
    window.dispatchEvent(reload)
    expect(reload.defaultPrevented).toBe(true)
    expect(frame(wrapper).style.visibility).toBe('hidden')
    expect(raf).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('ignores messages that do not come from its own frame', async () => {
    listPacks([installed])
    const wrapper = await mountPack()
    window.dispatchEvent(new MessageEvent('message', { data: { source: 'kpanel-scene-pack', type: 'ready', cameras: ['a'] }, source: window }))
    window.dispatchEvent(new MessageEvent('message', { data: { source: 'kpanel-scene-pack', type: 'error', reason: 'x' } }))
    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'ready', cameras: 'all' })
    await nextTick()
    expect(wrapper.emitted('cameras')).toBeUndefined()
    expect(wrapper.emitted('failed')).toBeUndefined()
    expect(wrapper.attributes('data-scene-pack-state')).toBe('loading')
    wrapper.unmount()
  })

  it('falls back to the poster when the pack reports an error', async () => {
    listPacks([installed])
    const wrapper = await mountPack()
    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'error', reason: 'webgl-unavailable' })
    await nextTick()
    expect(wrapper.emitted('failed')).toHaveLength(1)
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.attributes('data-scene-pack-state')).toBe('failed')
    expect(wrapper.get('img').attributes('src')).toBe('/api/v1/desktop/scene-packs/orbital-station/poster')
    wrapper.unmount()
  })

  it('gives up on a pack that never reports ready', async () => {
    vi.useFakeTimers()
    listPacks([installed])
    const wrapper = await mountPack()
    await vi.advanceTimersByTimeAsync(19_999)
    expect(wrapper.emitted('failed')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper.emitted('failed')).toHaveLength(1)
    expect(wrapper.find('iframe').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps waiting while the pack reports progress, and shows it only when loading is slow', async () => {
    vi.useFakeTimers()
    listPacks([installed])
    const wrapper = await mountPack()
    // Quick loads stay black: no progress line in the first moments.
    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'progress', value: 0.2 })
    await nextTick()
    expect(wrapper.find('.desktop-scene-pack__progress').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(1999)
    expect(wrapper.find('.desktop-scene-pack__progress').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    const line = wrapper.get('.desktop-scene-pack__progress')
    expect(line.attributes('style')).toContain('--scene-pack-progress: 0.200')
    // Each progress message counts as a sign of life for the watchdog.
    for (let step = 1; step <= 3; step++) {
      await vi.advanceTimersByTimeAsync(15_000)
      fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'progress', value: 0.2 + step * 0.2 })
      await nextTick()
    }
    expect(wrapper.emitted('failed')).toBeUndefined()
    expect(wrapper.get('.desktop-scene-pack__progress').attributes('style')).toContain('--scene-pack-progress: 0.800')
    fromPack(wrapper, { source: 'kpanel-scene-pack', type: 'ready', cameras: ['panorama'] })
    await nextTick()
    expect(wrapper.find('.desktop-scene-pack__progress').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(wrapper.emitted('failed')).toBeUndefined()
    wrapper.unmount()
  })

  it('starts from where the pack was last found, and follows the pack list', async () => {
    listPacks([installed])
    const first = await mountPack()
    first.unmount()
    // Next time the frame starts before the list answers.
    let answer: (value: Awaited<ReturnType<typeof api.desktop.scenePacks>>) => void = () => {}
    vi.spyOn(api.desktop, 'scenePacks').mockReturnValue(new Promise((resolve) => { answer = resolve }))
    const wrapper = mount(DesktopScenePack, { props: { packId: 'orbital-station', covered: false }, attachTo: document.body })
    await nextTick()
    expect(wrapper.get('iframe').attributes('src')).toBe(`${installed.fileBase}index.html`)
    // After an update the list names a fresh install capability: the frame moves there.
    const updatedBase = `/api/v1/desktop/scene-packs/orbital-station/files/${'b'.repeat(32)}/`
    answer({ source: 'auto', sources: ['auto'], packs: [{ ...installed, fileBase: updatedBase }] })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toBe(`${updatedBase}index.html`)
    wrapper.unmount()
    // After an uninstall the scene is turned off and forgotten.
    listPacks([{ ...installed, installed: false, fileBase: null }])
    const removed = await mountPack()
    expect(removed.find('iframe').exists()).toBe(false)
    expect(removed.emitted('failed')).toHaveLength(1)
    removed.unmount()
    vi.spyOn(api.desktop, 'scenePacks').mockReturnValue(new Promise(() => {}))
    const later = mount(DesktopScenePack, { props: { packId: 'orbital-station', covered: false }, attachTo: document.body })
    await nextTick()
    expect(later.find('iframe').exists()).toBe(false)
    later.unmount()
  })

  it('never starts from a remembered file base that is not a same-origin path', async () => {
    window.localStorage.setItem('kpanel:scene-pack-file-base:v1', JSON.stringify({ 'orbital-station': 'https://evil.example/' }))
    vi.spyOn(api.desktop, 'scenePacks').mockReturnValue(new Promise(() => {}))
    const wrapper = mount(DesktopScenePack, { props: { packId: 'orbital-station', covered: false }, attachTo: document.body })
    await nextTick()
    expect(wrapper.find('iframe').exists()).toBe(false)
    wrapper.unmount()
  })

  it('never runs a pack that is not installed or has no same-origin file base', async () => {
    listPacks([{ ...installed, installed: false, fileBase: null }])
    const missing = await mountPack()
    expect(missing.find('iframe').exists()).toBe(false)
    expect(missing.emitted('failed')).toHaveLength(1)
    missing.unmount()

    listPacks([{ ...installed, fileBase: 'https://evil.example/' }])
    const foreign = await mountPack()
    expect(foreign.find('iframe').exists()).toBe(false)
    expect(foreign.emitted('failed')).toHaveLength(1)
    foreign.unmount()
  })

  it('keeps the poster without failing when the pack list is unreachable', async () => {
    vi.spyOn(api.desktop, 'scenePacks').mockRejectedValue(new Error('offline'))
    const wrapper = await mountPack()
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.emitted('failed')).toBeUndefined()
    expect(wrapper.attributes('data-scene-pack-state')).toBe('static')
    wrapper.unmount()
  })

  it('shows only the poster for reduced motion unless the person opted in', async () => {
    const matchMedia = vi.fn(() => ({ matches: true, addEventListener: vi.fn() }))
    vi.stubGlobal('matchMedia', matchMedia)
    resetSceneMotionPreferenceForTest()
    listPacks([installed])
    const still = await mountPack()
    expect(still.find('iframe').exists()).toBe(false)
    expect(still.attributes('data-scene-pack-state')).toBe('static')
    expect(still.find('img').exists()).toBe(true)
    still.unmount()

    window.localStorage.setItem(DESKTOP_SCENE_MOTION_KEY, 'always')
    resetSceneMotionPreferenceForTest()
    const live = await mountPack()
    expect(live.find('iframe').exists()).toBe(true)
    live.unmount()
  })
})

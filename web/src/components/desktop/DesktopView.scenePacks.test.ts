// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from '@/components/desktop/DesktopView.vue'
import { api } from '@/lib/api'
import { resetSceneMotionPreferenceForTest } from '@/lib/desktopScenes/motionPreference'
import type { ScenePack } from '@/lib/scenePacks'
import { resetDesktopModeForTest } from '@/stores/desktopMode'
import { useTheme } from '@/stores/theme'

const available: ScenePack = {
  id: 'orbital-station',
  version: '1.0.0',
  name: { 'zh-CN': '星港轨道', 'en-US': 'Orbital Station' },
  description: { 'zh-CN': '曲速抵达行星轨道', 'en-US': 'Drop out of warp above a planet' },
  author: { name: 'KPanel' },
  license: 'MIT',
  tags: ['space'],
  theme: { brand: '#2f6fd0', neutral: '#33415a', signature: '#35b8d6' },
  cameras: [
    { id: 'panorama', name: { 'zh-CN': '行星全景' } },
    { id: 'station', name: { 'zh-CN': '空间站特写' } },
    { id: 'sunrise', name: { 'zh-CN': '日出' } },
  ],
  sizeBytes: 650_000,
  installed: false,
  installedVersion: null,
  fileBase: null,
}
const installed: ScenePack = { ...available, installed: true, installedVersion: '1.0.0', fileBase: '/api/v1/desktop/scene-packs/orbital-station/files/' }
const community: ScenePack = { ...available, id: 'neon-city', name: { 'zh-CN': '霓虹都市' }, author: { name: 'someone' } }

let packs: ScenePack[]

function stubPackAPI() {
  vi.spyOn(api.desktop, 'scenePacks').mockImplementation(async () => ({ source: 'auto', sources: ['auto', 'github', 'mirror'], packs }))
  const install = vi.spyOn(api.desktop, 'installScenePack').mockImplementation(async (id) => {
    packs = packs.map((pack) => (pack.id === id ? installed : pack))
    return installed
  })
  const remove = vi.spyOn(api.desktop, 'deleteScenePack').mockImplementation(async (id) => {
    packs = packs.map((pack) => (pack.id === id ? available : pack))
  })
  const setSource = vi.spyOn(api.desktop, 'setScenePackSource').mockImplementation(async (source) => ({ source }))
  return { install, remove, setSource }
}

async function openWallpaperDialog(wrapper: VueWrapper): Promise<HTMLElement> {
  await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
  await nextTick()
  await wrapper.get('[data-context-action="wallpaper"]').trigger('click')
  await flushPromises()
  return document.body.querySelector<HTMLElement>('.desktop-scene-packs')!
}

function card(section: HTMLElement, id: string): HTMLElement {
  return section.querySelector<HTMLElement>(`[data-scene-pack-card="${id}"]`)!
}

function action(section: HTMLElement, id: string, name: string): HTMLButtonElement | null {
  return card(section, id).querySelector<HTMLButtonElement>(`[data-scene-pack-action="${name}"]`)
}

/** The pack layer is an async chunk; wait for it to mount and load its sandboxed page. */
async function sceneFrame(wrapper: VueWrapper): Promise<HTMLIFrameElement> {
  return vi.waitFor(() => wrapper.get('[data-scene-pack="orbital-station"] iframe').element as HTMLIFrameElement)
}

async function settle(): Promise<void> {
  for (let tick = 0; tick < 4; tick++) {
    await flushPromises()
    await nextTick()
  }
}

describe('DesktopView scene packs', () => {
  beforeEach(() => {
    packs = [available, community]
    resetDesktopModeForTest()
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    vi.stubGlobal('matchMedia', undefined)
    resetSceneMotionPreferenceForTest()
    Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', { configurable: true, value: vi.fn(() => null) })
    Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true })
    Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetSceneMotionPreferenceForTest()
    document.body.innerHTML = ''
  })

  it('lists repository packs with official and community badges and filters installed ones', async () => {
    expect(document.documentElement.dataset.desktopWallpaperScene).toBeUndefined()
    stubPackAPI()
    packs = [installed, community]
    const wrapper = mount(DesktopView, { attachTo: document.body })
    const section = await openWallpaperDialog(wrapper)

    expect(section.querySelectorAll('[data-scene-pack-card]')).toHaveLength(2)
    expect(card(section, 'orbital-station').textContent).toContain('星港轨道')
    expect(card(section, 'orbital-station').textContent).toContain('官方')
    expect(card(section, 'orbital-station').textContent).toContain('3 个机位')
    expect(card(section, 'neon-city').textContent).toContain('社区')
    expect(card(section, 'orbital-station').querySelector('img')?.getAttribute('src')).toBe(api.desktop.scenePackThumbURL('orbital-station', '1.0.0'))

    section.querySelector<HTMLButtonElement>('[data-scene-pack-filter="installed"]')!.click()
    await nextTick()
    expect(section.querySelectorAll('[data-scene-pack-card]')).toHaveLength(1)
    expect(card(section, 'orbital-station')).not.toBeNull()
    wrapper.unmount()
  })

  it('downloads, applies, switches cameras and deletes a pack', async () => {
    const { install, remove } = stubPackAPI()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    let section = await openWallpaperDialog(wrapper)

    expect(action(section, 'orbital-station', 'apply')).toBeNull()
    action(section, 'orbital-station', 'install')!.click()
    await settle()
    expect(install).toHaveBeenCalledWith('orbital-station')
    expect(action(section, 'orbital-station', 'install')).toBeNull()

    vi.useFakeTimers()
    action(section, 'orbital-station', 'apply')!.click()
    await vi.advanceTimersByTimeAsync(500)
    vi.useRealTimers()
    await settle()
    expect(window.localStorage.getItem('kpanel:desktop-wallpaper:v1')).toBe('pack:orbital-station')
    const wallpaper = wrapper.get('.desktop__wallpaper-image')
    expect(wallpaper.attributes('data-wallpaper')).toBe('pack:orbital-station')
    // A live scene starts from black (no poster, no aurora) and sets the panel to its one color scheme.
    expect(wrapper.get('.desktop__wallpaper').classes()).toContain('desktop__wallpaper--scene')
    // Every wallpaper surface (boot layer, loading placeholder) reads the same root flag.
    expect(document.documentElement.dataset.desktopWallpaperScene).toBe('live')
    const theme = useTheme()
    expect(theme.colors.value).toMatchObject({ brand: '#2f6fd0', neutral: '#33415a', signature: '#35b8d6' })
    const frame = await sceneFrame(wrapper)
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts')

    // The camera action appears once the running pack reports more than one shot.
    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    expect(wrapper.find('[data-context-action="scene-camera"]').exists()).toBe(false)
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    window.dispatchEvent(new MessageEvent('message', { data: { source: 'kpanel-scene-pack', type: 'ready', cameras: ['panorama', 'station', 'sunrise'] }, source: frame.contentWindow }))
    await nextTick()
    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    await wrapper.get('[data-context-action="scene-camera"]').trigger('click')
    expect(post).toHaveBeenLastCalledWith({ source: 'kpanel-desktop', type: 'camera' }, '*')
    // Changing camera never changes the theme.
    window.dispatchEvent(new MessageEvent('message', { data: { source: 'kpanel-scene-pack', type: 'camera', index: 1 }, source: frame.contentWindow }))
    await nextTick()
    expect(theme.colors.value).toMatchObject({ brand: '#2f6fd0', neutral: '#33415a', signature: '#35b8d6' })

    section = await openWallpaperDialog(wrapper)
    expect(card(section, 'orbital-station').classList).toContain('desktop-scene-pack-card--active')
    expect(action(section, 'orbital-station', 'apply')!.disabled).toBe(true)
    action(section, 'orbital-station', 'delete')!.click()
    await nextTick()
    expect(remove).not.toHaveBeenCalled()
    expect(action(section, 'orbital-station', 'delete')!.textContent).toContain('确认删除')
    action(section, 'orbital-station', 'delete')!.click()
    await settle()
    expect(remove).toHaveBeenCalledWith('orbital-station')
    expect(window.localStorage.getItem('kpanel:desktop-wallpaper:v1')).toBe('classic')
    expect(wrapper.find('[data-scene-pack]').exists()).toBe(false)
    expect(wrapper.get('.desktop__wallpaper-image').attributes('data-wallpaper')).toBe('classic')
    expect(wrapper.get('.desktop__wallpaper').classes()).not.toContain('desktop__wallpaper--scene')
    expect(document.documentElement.dataset.desktopWallpaperScene).toBeUndefined()
    wrapper.unmount()
    theme.resetColors()
  })

  it('keeps the still poster wallpaper for reduced motion', async () => {
    stubPackAPI()
    packs = [installed]
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('reduced-motion'), addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    resetSceneMotionPreferenceForTest()
    window.localStorage.setItem('kpanel:desktop-wallpaper:v1', 'pack:orbital-station')
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await settle()
    expect(wrapper.get('.desktop__wallpaper').classes()).not.toContain('desktop__wallpaper--scene')
    expect(document.documentElement.dataset.desktopWallpaperScene).toBeUndefined()
    expect(wrapper.get('.desktop__wallpaper-image').attributes('style')).toContain('/api/v1/desktop/scene-packs/orbital-station/poster')
    await vi.waitFor(() => expect(wrapper.get('[data-scene-pack="orbital-station"]').attributes('data-scene-pack-state')).toBe('static'))
    wrapper.unmount()
  })

  it('reports failed downloads on the card and keeps the pack available', async () => {
    const { install } = stubPackAPI()
    install.mockRejectedValueOnce(new Error('sha256 mismatch'))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    const section = await openWallpaperDialog(wrapper)
    action(section, 'orbital-station', 'install')!.click()
    await settle()
    expect(card(section, 'orbital-station').querySelector('[role="alert"]')?.textContent).toContain('下载失败')
    expect(action(section, 'orbital-station', 'install')).not.toBeNull()
    wrapper.unmount()
  })

  it('saves the download route and restores it when saving fails', async () => {
    const { setSource } = stubPackAPI()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    const section = await openWallpaperDialog(wrapper)
    const select = section.querySelector<HTMLSelectElement>('[data-scene-pack-source]')!
    expect([...select.options].map((option) => option.value)).toEqual(['auto', 'github', 'mirror'])

    select.value = 'mirror'
    select.dispatchEvent(new Event('change'))
    await settle()
    expect(setSource).toHaveBeenCalledWith('mirror')
    expect(select.value).toBe('mirror')

    setSource.mockRejectedValueOnce(new Error('forbidden'))
    select.value = 'github'
    select.dispatchEvent(new Event('change'))
    await settle()
    expect(select.value).toBe('mirror')
    wrapper.unmount()
  })

  it('offers a retry when the repository catalog cannot be loaded', async () => {
    const list = vi.spyOn(api.desktop, 'scenePacks').mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    const section = await openWallpaperDialog(wrapper)
    const retry = section.querySelector<HTMLButtonElement>('[role="alert"] button')!
    expect(section.textContent).toContain('无法读取场景仓库')

    list.mockResolvedValue({ source: 'auto', sources: ['auto'], packs: [available] })
    retry.click()
    await settle()
    expect(card(section, 'orbital-station')).not.toBeNull()
    wrapper.unmount()
  })

  it('returns to the classic wallpaper only when a failed pack is really gone', async () => {
    stubPackAPI()
    packs = [installed]
    window.localStorage.setItem('kpanel:desktop-wallpaper:v1', 'pack:orbital-station')
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await settle()
    expect(document.documentElement.dataset.desktopWallpaperScene).toBe('live')
    const frame = await sceneFrame(wrapper)
    window.dispatchEvent(new MessageEvent('message', { data: { source: 'kpanel-scene-pack', type: 'error', reason: 'context-lost' }, source: frame.contentWindow }))
    await settle()
    expect(window.localStorage.getItem('kpanel:desktop-wallpaper:v1')).toBe('pack:orbital-station')
    expect(wrapper.get('[data-scene-pack="orbital-station"]').attributes('data-scene-pack-state')).toBe('failed')
    wrapper.unmount()

    packs = [available]
    const removed = mount(DesktopView, { attachTo: document.body })
    await settle()
    expect(window.localStorage.getItem('kpanel:desktop-wallpaper:v1')).toBe('classic')
    expect(removed.find('[data-scene-pack]').exists()).toBe(false)
    removed.unmount()
  })
})

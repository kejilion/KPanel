// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { api } from '@/lib/api'
import { useDesktopWallpaper } from '@/lib/desktopWallpapers'
import { resetSceneMotionPreferenceForTest } from '@/lib/desktopScenes/motionPreference'
import type { ScenePack } from '@/lib/scenePacks'
import type { CustomWallpaper, CustomWallpaperList } from '@/types/api'
import DesktopScenePack from './DesktopScenePack.vue'
import DesktopWallpaperPicker from './DesktopWallpaperPicker.vue'

const pack = (capability: string, installedVersion = '1.0.0'): ScenePack => ({
  id: 'orbital-station', version: '1.0.1', installedVersion, installed: true,
  name: { 'zh-CN': '星港轨道' }, description: { 'zh-CN': '行星轨道' },
  cameras: [], tags: [], license: 'MIT', sizeBytes: 1000,
  fileBase: `/api/v1/desktop/scene-packs/orbital-station/files/${capability.repeat(32)}/`,
  resourceVersion: `sha256:${capability.repeat(64)}`,
})

describe('shared wallpaper pack updates', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.stubGlobal('matchMedia', undefined)
    resetSceneMotionPreferenceForTest()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetSceneMotionPreferenceForTest()
  })

  it.each([true, false])('restarts the shared wallpaper only after a successful update (success=%s)', async (success) => {
    let installed = pack('a')
    vi.spyOn(api.desktop, 'scenePacks').mockImplementation(async () => ({ source: 'auto', sources: ['auto', 'github', 'mirror'], packs: [installed] }))
    vi.spyOn(api.desktop, 'installScenePack').mockImplementation(async () => {
      if (!success) throw new Error('download failed')
      installed = pack('b', '1.0.1')
      return installed
    })
    const choice = useDesktopWallpaper()
    choice.select('pack:orbital-station')
    const wrapper = mount(defineComponent({
      setup: () => () => h('div', [
        h(DesktopWallpaperPicker, { visible: true }),
        h(DesktopScenePack, {
          key: choice.sceneRevision.value, packId: 'orbital-station', covered: false,
        }),
      ]),
    }))
    try {
      await flushPromises()
      const originalFrame = wrapper.get('iframe').element
      await wrapper.get('[data-scene-pack-action="install"]').trigger('click')
      await flushPromises()
      expect(api.desktop.installScenePack).toHaveBeenCalledWith('orbital-station', `sha256:${'a'.repeat(64)}`)
      expect(wrapper.get('iframe').attributes('src')).toBe(`${installed.fileBase}index.html`)
      expect(wrapper.get('iframe').element === originalFrame).toBe(!success)
      expect(choice.id.value).toBe('pack:orbital-station')
      expect(wrapper.find('.desktop-scene-pack-card__error').exists()).toBe(!success)
    } finally {
      wrapper.unmount()
    }
  })
})

const uploaded: CustomWallpaper = {
  id: '0123456789abcdef0123456789abcdef', name: '晚霞山谷', format: 'webp', width: 3840, height: 2160,
  imageBytes: 2_000_000, thumbBytes: 40_000, focusX: 700, focusY: 350, luminance: 55,
  theme: { brand: '#ed8545', neutral: '#5a4a52', signature: '#ed8545' },
  createdAt: '2026-09-25T00:00:00Z', imageDigest: 'a'.repeat(64),
}

function list(wallpapers: CustomWallpaper[]): CustomWallpaperList {
  const bytes = wallpapers.reduce((total, item) => total + item.imageBytes + item.thumbBytes, 0)
  return { wallpapers, usage: { count: wallpapers.length, bytes, maxCount: 12, maxBytes: 48 << 20 } }
}

async function settle(): Promise<void> {
  for (let tick = 0; tick < 4; tick++) await flushPromises()
}

describe('DesktopWallpaperPicker uploaded wallpapers', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.spyOn(api.desktop, 'scenePacks').mockResolvedValue({ source: 'auto', sources: ['auto'], packs: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('shows an empty collection with its limits and an upload tile', async () => {
    vi.spyOn(api.desktop, 'wallpapers').mockResolvedValue(list([]))
    const wrapper = mount(DesktopWallpaperPicker, { props: { visible: true }, attachTo: document.body })
    await settle()

    expect(wrapper.find('[data-custom-wallpaper-usage]').text()).toContain('0 KB')
    expect(wrapper.find('[data-custom-wallpaper-upload]').attributes('disabled')).toBeUndefined()
    expect(wrapper.findAll('[data-custom-wallpaper]')).toHaveLength(0)
  })

  it('selects an upload with its record and deletes it only on the second click', async () => {
    vi.spyOn(api.desktop, 'wallpapers').mockResolvedValue(list([uploaded]))
    const remove = vi.spyOn(api.desktop, 'deleteWallpaper').mockResolvedValue()
    const wrapper = mount(DesktopWallpaperPicker, { props: { visible: true }, attachTo: document.body })
    await settle()

    const item = wrapper.get(`[data-custom-wallpaper="${uploaded.id}"]`)
    expect(item.text()).toContain('晚霞山谷')
    await item.get('button[aria-pressed]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([`custom:${uploaded.id}`, uploaded])

    useDesktopWallpaper().select(`custom:${uploaded.id}`, uploaded)
    const deleteButton = wrapper.get('[data-custom-wallpaper-delete]')
    await deleteButton.trigger('click')
    expect(remove).not.toHaveBeenCalled()
    await deleteButton.trigger('click')
    await settle()
    expect(remove).toHaveBeenCalledWith(uploaded.id)
    expect(wrapper.findAll('[data-custom-wallpaper]')).toHaveLength(0)
    expect(useDesktopWallpaper().id.value).toBe('classic')
  })

  it('disables uploading once the collection is full', async () => {
    const full = list([uploaded])
    full.usage.count = full.usage.maxCount
    vi.spyOn(api.desktop, 'wallpapers').mockResolvedValue(full)
    const wrapper = mount(DesktopWallpaperPicker, { props: { visible: true }, attachTo: document.body })
    await settle()

    expect(wrapper.get('[data-custom-wallpaper-upload]').attributes('disabled')).toBeDefined()
  })
})

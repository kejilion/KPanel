// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { api } from '@/lib/api'
import { useDesktopWallpaper } from '@/lib/desktopWallpapers'
import { resetSceneMotionPreferenceForTest } from '@/lib/desktopScenes/motionPreference'
import type { ScenePack } from '@/lib/scenePacks'
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

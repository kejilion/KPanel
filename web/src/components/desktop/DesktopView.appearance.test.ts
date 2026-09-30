// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from './DesktopView.vue'
import { api } from '@/lib/api'
import { startAppearanceSync, stopAppearanceSync } from '@/lib/appearanceSync'
import { resetDesktopModeForTest } from '@/stores/desktopMode'

describe('authenticated desktop first wallpaper', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', { configurable: true, value: vi.fn(() => null) })
    Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true })
    Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true })
  })
  it.each(['orbit', 'pack:orbital-station'])('first mounts the server wallpaper %s without a default surface', async (wallpaper) => {
    window.localStorage.clear()
    let resolve!: (value: Awaited<ReturnType<typeof api.desktop.appearance>>) => void
    const appearance = vi.spyOn(api.desktop, 'appearance').mockReturnValue(new Promise((done) => { resolve = done }))
    const sync = startAppearanceSync()
    const wrapper = mount(DesktopView)
    try {
      expect(wrapper.find('.desktop__wallpaper').exists()).toBe(false)
      expect(wrapper.findAll('.desktop__icon').length).toBeGreaterThan(0)
      resolve({ configured: true, resourceVersion: 'sha256:remote', theme: 'light', colors: null, wallpaper, classicLevel: 'off' })
      await sync
      await nextTick()
      expect(wrapper.get('.desktop-wallpaper-surface').attributes('data-wallpaper')).toBe(wallpaper)
      expect(wrapper.get('.desktop-wallpaper-host').attributes('data-wallpaper-phase')).toBe('idle')
      expect(wrapper.get('.desktop-wallpaper-surface__image').attributes('src')).not.toBe('/wallpapers/kpanel-desktop.webp')
    } finally {
      wrapper.unmount()
      stopAppearanceSync()
      appearance.mockRestore()
    }
  })

})

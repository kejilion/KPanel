// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DesktopScene from '@/components/desktop/DesktopScene.vue'

let decodeFails = false
let reducedMotion = false

class FakeImage {
  decoding = ''
  src = ''
  decode(): Promise<void> {
    return decodeFails ? Promise.reject(new Error('decode')) : Promise.resolve()
  }
}

async function settle(): Promise<void> {
  // Poster decode, the lazy painter import and the ready flag each take a turn.
  await flushPromises()
  await vi.dynamicImportSettled()
  await flushPromises()
}

describe('DesktopScene', () => {
  beforeEach(() => {
    decodeFails = false
    reducedMotion = false
    vi.stubGlobal('Image', FakeImage)
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
      matches: query.includes('reduced-motion') && reducedMotion,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))
    Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', { configurable: true, value: vi.fn(() => null) })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('shows the poster stage and keeps motion running while the desktop is visible', async () => {
    const wrapper = mount(DesktopScene, { props: { scene: 'tide', covered: false, entrance: 'restore' } })
    await settle()
    const root = wrapper.get('.desktop-scene')
    expect(root.classes()).toEqual(expect.arrayContaining(['desktop-scene--ready', 'desktop-scene--live']))
    expect(root.attributes('data-scene-status')).toBe('running')
    expect(root.attributes('aria-hidden')).toBe('true')
    expect(wrapper.get('.desktop-scene__poster').attributes('style')).toContain('/wallpapers/scenes/tide.webp')
    expect(wrapper.findAll('.desktop-scene__layer').map((layer) => layer.classes().at(-1))).toEqual([
      'desktop-scene__layer--sun',
      'desktop-scene__layer--path',
    ])
    wrapper.unmount()
  })

  it('pauses while a window covers the desktop and resumes afterwards', async () => {
    const wrapper = mount(DesktopScene, { props: { scene: 'neon', covered: false, entrance: 'restore' } })
    await settle()
    await wrapper.setProps({ covered: true })
    expect(wrapper.get('.desktop-scene').classes()).toContain('desktop-scene--paused')
    expect(wrapper.get('.desktop-scene').attributes('data-scene-status')).toBe('paused')
    await wrapper.setProps({ covered: false })
    expect(wrapper.get('.desktop-scene').classes()).not.toContain('desktop-scene--paused')
    expect(wrapper.get('.desktop-scene').attributes('data-scene-status')).toBe('running')
    wrapper.unmount()
  })

  it('renders a still scene for reduced motion', async () => {
    reducedMotion = true
    vi.useFakeTimers()
    const wrapper = mount(DesktopScene, { props: { scene: 'sakura', covered: false, entrance: 'select' } })
    await vi.advanceTimersByTimeAsync(700)
    await settle()
    const root = wrapper.get('.desktop-scene')
    expect(root.classes()).toContain('desktop-scene--static')
    expect(root.classes()).not.toContain('desktop-scene--live')
    expect(root.attributes('data-scene-status')).toBe('static')
    wrapper.unmount()
  })

  it('leaves the static wallpaper in charge when the poster cannot load', async () => {
    decodeFails = true
    const wrapper = mount(DesktopScene, { props: { scene: 'aurora', covered: false, entrance: 'restore' } })
    await settle()
    expect(wrapper.find('.desktop-scene').exists()).toBe(false)
    wrapper.unmount()
  })

  it('waits for the wallpaper crossfade before a chosen scene takes over', async () => {
    vi.useFakeTimers()
    const wrapper = mount(DesktopScene, { props: { scene: 'aurora', covered: false, entrance: 'select' } })
    await settle()
    expect(wrapper.get('.desktop-scene').classes()).not.toContain('desktop-scene--ready')
    await vi.advanceTimersByTimeAsync(650)
    await settle()
    expect(wrapper.get('.desktop-scene').classes()).toContain('desktop-scene--ready')
    wrapper.unmount()
  })

  it('keeps only the current time of day in the chrono render tree', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 8, 24, 12, 0, 0))
    const wrapper = mount(DesktopScene, { props: { scene: 'chrono', covered: false, entrance: 'restore' } })
    await settle()
    const root = wrapper.get('.desktop-scene')
    expect(root.classes()).toContain('desktop-scene--phase-day')
    expect(root.classes()).not.toContain('desktop-scene--phase-night')
    expect((root.element as HTMLElement).style.getPropertyValue('--scene-day')).toBe('1.0000')
    const posters = wrapper.findAll('.desktop-scene__poster')
    expect(posters.map((poster) => [poster.attributes('data-phase'), Boolean(poster.attributes('style'))])).toEqual([
      ['golden', false],
      ['day', true],
      ['night', false],
    ])

    // Back from a hidden tab hours later: catch up at once instead of on the next minute tick.
    vi.setSystemTime(new Date(2026, 8, 24, 22, 0, 0))
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect((root.element as HTMLElement).style.getPropertyValue('--scene-night')).toBe('1.0000')
    expect(root.classes()).toContain('desktop-scene--phase-night')
    wrapper.unmount()
  })
})

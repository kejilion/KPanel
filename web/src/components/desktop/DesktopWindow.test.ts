// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { useRouter } from 'vue-router'
import DesktopWindow from './DesktopWindow.vue'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import { desktopBrowserHistoryKey } from '@/lib/desktopRouteKeys'
import type {
  DesktopBrowserHistory,
  DesktopBrowserHistoryPoint,
} from '@/lib/desktopBrowserHistory'

const routeMocks = vi.hoisted(() => ({
  resolveWindowComponent: vi.fn(),
}))

vi.mock('@/lib/desktopWindowRoute', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/desktopWindowRoute')>(),
  resolveWindowComponent: routeMocks.resolveWindowComponent,
}))

function setupViewport(): void {
  Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true })
}

function pointer(type: string, x: number, y: number, id = 1): PointerEvent {
  return new PointerEvent(type, {
    bubbles: true,
    clientX: x,
    clientY: y,
    pointerId: id,
    button: 0,
  })
}

function createBrowserHistoryFixture() {
  const listeners = new Set<(point: DesktopBrowserHistoryPoint) => void>()
  const history: DesktopBrowserHistory = {
    navigate: vi.fn().mockResolvedValue(undefined),
    go: vi.fn(),
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    dispose: vi.fn(),
  }
  return {
    history,
    emit(point: DesktopBrowserHistoryPoint) {
      for (const listener of [...listeners]) listener(point)
    },
  }
}

describe('DesktopWindow lazy view loading', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    setupViewport()
    routeMocks.resolveWindowComponent.mockReset()
  })

  it('shows a retryable error instead of hanging when a page chunk rejects', async () => {
    routeMocks.resolveWindowComponent.mockRejectedValue(new Error('chunk unavailable'))
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      props: {
        windowState,
        icon: () => null,
      },
    })

    await flushPromises()
    expect(wrapper.find('.desktop-window__loading').exists()).toBe(false)
    expect(wrapper.find('.desktop-window__load-error').exists()).toBe(true)

    await wrapper.find('.desktop-window__load-error button').trigger('click')
    await flushPromises()
    expect(routeMocks.resolveWindowComponent).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.desktop-window__load-error').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps lazily loaded page components out of Vue reactivity', async () => {
    routeMocks.resolveWindowComponent.mockResolvedValue({
      name: 'DesktopPageFixture',
      template: '<main data-testid="desktop-page-fixture" />',
    })
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      props: {
        windowState,
        icon: () => null,
      },
    })

    await flushPromises()
    expect(wrapper.find('[data-testid="desktop-page-fixture"]').exists()).toBe(true)
    expect(warning.mock.calls.flat().join(' ')).not.toContain('made a reactive object')
    wrapper.unmount()
    warning.mockRestore()
  })

  it('uses supplied application chrome and falls back when its icon fails', async () => {
    routeMocks.resolveWindowComponent.mockResolvedValue({
      name: 'ScriptPageFixture',
      template: '<main />',
    })
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/app-script/openclaw', 'desktop.scriptWindowTitle', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      props: {
        windowState,
        icon: { template: '<svg data-testid="fallback-script-icon" />' },
        iconUrl: '/api/v1/apps/openclaw/icon',
        title: 'OpenClaw 的脚本终端',
      },
    })

    await flushPromises()
    expect(wrapper.attributes('aria-label')).toBe('OpenClaw 的脚本终端')
    expect(wrapper.find('.desktop-window__title').text()).toContain('OpenClaw 的脚本终端')
    const image = wrapper.find('.desktop-window__app-glyph img')
    expect(image.attributes('src')).toBe('/api/v1/apps/openclaw/icon')

    await image.trigger('error')
    expect(wrapper.find('[data-testid="fallback-script-icon"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('records page navigation for native Back and restores the matching window', async () => {
    routeMocks.resolveWindowComponent.mockResolvedValue({
      name: 'NavigableDesktopPageFixture',
      setup() {
        const router = useRouter()
        return {
          openMonitoring: () => router.push('/monitoring'),
          openProcesses: () => router.push('/processes'),
          openSystem: () => router.push('/system'),
          goBack: () => router.back(),
        }
      },
      template: `
        <main>
          <button data-testid="open-monitoring" @click="openMonitoring">Monitoring</button>
          <button data-testid="open-processes" @click="openProcesses">Processes</button>
          <button data-testid="open-system" @click="openSystem">System</button>
          <button data-testid="go-back" @click="goBack">Back</button>
        </main>
      `,
    })
    const desktop = useDesktopMode()
    const nativeHistory = createBrowserHistoryFixture()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      props: {
        windowState,
        icon: () => null,
      },
      global: {
        provide: {
          [desktopBrowserHistoryKey as symbol]: nativeHistory.history,
        },
      },
    })

    await flushPromises()
    expect(wrapper.find('.desktop-window__back').exists()).toBe(false)

    await wrapper.get('[data-testid="open-monitoring"]').trigger('click')
    await flushPromises()
    expect(windowState.path).toBe('/monitoring')
    expect(nativeHistory.history.navigate).toHaveBeenLastCalledWith(
      { windowId: id, fullPath: '/overview' },
      { windowId: id, fullPath: '/monitoring' },
    )

    await wrapper.get('[data-testid="open-processes"]').trigger('click')
    await flushPromises()
    expect(windowState.path).toBe('/processes')
    expect(nativeHistory.history.navigate).toHaveBeenLastCalledWith(
      { windowId: id, fullPath: '/monitoring' },
      { windowId: id, fullPath: '/processes' },
    )

    await wrapper.get('[data-testid="open-system"]').trigger('click')
    await flushPromises()
    expect(windowState.path).toBe('/system')
    expect(windowState.titleKey).toBe('route.systemCenter')
    expect(desktop.windows.value).toHaveLength(1)
    expect(nativeHistory.history.navigate).toHaveBeenLastCalledWith(
      { windowId: id, fullPath: '/processes' },
      { windowId: id, fullPath: '/system' },
    )

    await wrapper.get('[data-testid="go-back"]').trigger('click')
    expect(nativeHistory.history.go).toHaveBeenCalledWith(-1)
    expect(windowState.path).toBe('/system')

    desktop.minimizeWindow(id)
    nativeHistory.emit({ windowId: id + 1, fullPath: '/overview' })
    expect(windowState.path).toBe('/system')
    expect(windowState.minimized).toBe(true)

    nativeHistory.emit({ windowId: id, fullPath: '/monitoring' })
    await vi.waitFor(() => expect(windowState.path).toBe('/monitoring'))
    expect(desktop.windows.value).toHaveLength(1)
    expect(windowState.minimized).toBe(false)

    nativeHistory.emit({ windowId: id, fullPath: '/overview' })
    await vi.waitFor(() => expect(windowState.path).toBe('/overview'))
    expect(desktop.focusedId.value).toBe(id)
    wrapper.unmount()
  })

  it.each(['pointerup', 'pointercancel'])('keeps an unsnapped window partially outside the desktop after %s', async (endEvent) => {
    routeMocks.resolveWindowComponent.mockResolvedValue({ template: '<main />' })
    const desktopRoot = document.createElement('div')
    desktopRoot.className = 'desktop'
    document.body.appendChild(desktopRoot)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    desktop.updateGeometry(id, { left: 200, top: 80, width: 880, height: 600 })
    const wrapper = mount(DesktopWindow, {
      attachTo: desktopRoot,
      props: { windowState, icon: () => null },
    })
    await flushPromises()
    const titlebar = wrapper.get('.desktop-window__titlebar').element
    titlebar.dispatchEvent(pointer('pointerdown', 250, 100))
    window.dispatchEvent(pointer('pointermove', 1050, 480))
    await flushPromises()
    expect(desktopRoot.querySelector('.desktop-window-snap-preview')).toBeNull()
    window.dispatchEvent(pointer(endEvent, 1050, 480))
    await flushPromises()
    expect(windowState.geometry).toEqual({ left: 1000, top: 460, width: 880, height: 600 })
    expect(windowState.snap).toBeNull()
    expect(windowState.maximized).toBe(false)
    expect(JSON.parse(window.localStorage.getItem('kejilion-panel-desktop-windows')!)[0].geometry).toEqual(windowState.geometry)
    wrapper.unmount()
    desktopRoot.remove()
  })

  it('previews a side snap, commits it on release, and restores it when dragged away', async () => {
    routeMocks.resolveWindowComponent.mockResolvedValue({ template: '<main />' })
    const desktopRoot = document.createElement('div')
    desktopRoot.className = 'desktop'
    document.body.appendChild(desktopRoot)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      attachTo: desktopRoot,
      props: { windowState, icon: () => null },
    })
    await flushPromises()

    const titlebar = wrapper.get('.desktop-window__titlebar').element
    titlebar.dispatchEvent(pointer('pointerdown', 500, 80))
    window.dispatchEvent(pointer('pointermove', 8, 300))
    await flushPromises()
    expect(desktopRoot.querySelector('.desktop-window-snap-preview')).not.toBeNull()
    window.dispatchEvent(pointer('pointerup', 8, 300))
    await flushPromises()

    expect(windowState.snap).toBe('left')
    expect(wrapper.classes()).toContain('desktop-window--snapped')
    expect(desktopRoot.querySelector('.desktop-window-snap-preview')).toBeNull()
    expect(wrapper.findAll('.desktop-window__resize')).toHaveLength(0)

    vi.spyOn(wrapper.element, 'getBoundingClientRect').mockReturnValue({
      left: 10,
      top: 10,
      width: 625,
      height: 718,
      right: 635,
      bottom: 728,
      x: 10,
      y: 10,
      toJSON: () => ({}),
    })
    titlebar.dispatchEvent(pointer('pointerdown', 300, 28, 2))
    window.dispatchEvent(pointer('pointermove', 310, 38, 2))
    window.dispatchEvent(pointer('pointermove', 520, 120, 2))
    window.dispatchEvent(pointer('pointerup', 520, 120, 2))
    await flushPromises()

    expect(windowState.snap).toBeNull()
    expect(windowState.maximized).toBe(false)
    expect(windowState.geometry.width).toBe(880)
    wrapper.unmount()
    desktopRoot.remove()
  })

  it('persists the visible floating layout when a restored snap drag is cancelled', async () => {
    routeMocks.resolveWindowComponent.mockResolvedValue({ template: '<main />' })
    const desktopRoot = document.createElement('div')
    desktopRoot.className = 'desktop'
    document.body.appendChild(desktopRoot)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const wrapper = mount(DesktopWindow, {
      attachTo: desktopRoot,
      props: { windowState, icon: () => null },
    })
    await flushPromises()

    desktop.snapWindow(id, 'left')
    vi.spyOn(wrapper.element, 'getBoundingClientRect').mockReturnValue({
      left: 10,
      top: 10,
      width: 625,
      height: 718,
      right: 635,
      bottom: 728,
      x: 10,
      y: 10,
      toJSON: () => ({}),
    })
    const titlebar = wrapper.get('.desktop-window__titlebar').element
    titlebar.dispatchEvent(pointer('pointerdown', 300, 28, 4))
    window.dispatchEvent(pointer('pointermove', 310, 38, 4))
    window.dispatchEvent(pointer('pointermove', 520, 120, 4))
    window.dispatchEvent(pointer('pointercancel', 520, 120, 4))
    await flushPromises()

    expect(windowState.snap).toBeNull()
    const persisted = JSON.parse(window.localStorage.getItem('kejilion-panel-desktop-windows') ?? '[]') as Array<{
      id: number
      snap?: string | null
      geometry: { left: number }
    }>
    const record = persisted.find((item) => item.id === id)
    expect(record?.snap).toBeNull()
    expect(record?.geometry.left).toBe(windowState.geometry.left)
    wrapper.unmount()
    desktopRoot.remove()
  })

  it('keeps compact touch windows stable instead of starting hidden drag geometry', async () => {
    setupViewport()
    Object.defineProperty(window, 'innerWidth', { value: 390, configurable: true })
    Object.defineProperty(window, 'innerHeight', { value: 844, configurable: true })
    routeMocks.resolveWindowComponent.mockResolvedValue({ template: '<main />' })
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const windowState = desktop.windows.value.find((item) => item.id === id)!
    const originalGeometry = { ...windowState.geometry }
    const wrapper = mount(DesktopWindow, {
      props: { windowState, icon: () => null },
    })
    await flushPromises()

    const titlebar = wrapper.get('.desktop-window__titlebar').element
    titlebar.dispatchEvent(pointer('pointerdown', 190, 24, 8))
    window.dispatchEvent(pointer('pointermove', 280, 100, 8))
    window.dispatchEvent(pointer('pointerup', 280, 100, 8))
    titlebar.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))

    expect(windowState.geometry).toEqual(originalGeometry)
    expect(windowState.maximized).toBe(false)
    expect(windowState.snap).toBeNull()
    wrapper.unmount()
  })
})

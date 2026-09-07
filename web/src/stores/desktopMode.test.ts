// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { initializeDesktopMode, resetDesktopModeForTest, useDesktopMode } from './desktopMode'

function makeStorage(): { store: Map<string, string>; getItem: (k: string) => string | null; setItem: (k: string, v: string) => void } {
  const store = new Map<string, string>()
  return {
    store,
    getItem: (key) => store.get(key) ?? null,
    setItem: (key, value) => {
      store.set(key, value)
    },
  }
}

function setupViewport(width: number, height: number): void {
  Object.defineProperty(window, 'innerWidth', { value: width, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: height, configurable: true })
}

describe('desktop mode', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    window.localStorage.clear()
  })

  it('starts in classic mode', () => {
    const desktop = useDesktopMode()
    expect(desktop.mode.value).toBe('classic')
    expect(desktop.windows.value).toHaveLength(0)
  })

  it('enters and leaves desktop mode', () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    expect(desktop.mode.value).toBe('desktop')
    desktop.enterClassic()
    expect(desktop.mode.value).toBe('classic')
  })

  it('opens a window with the requested route and title', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', true)
    const windowState = desktop.windows.value.find((item) => item.id === id)
    expect(windowState).toBeDefined()
    expect(windowState?.path).toBe('/overview')
    expect(windowState?.titleKey).toBe('route.overview')
    expect(windowState?.minimized).toBe(false)
    expect(windowState?.snap).toBeNull()
    expect(desktop.focusedId.value).toBe(id)
  })

  it('focuses the existing window for single-instance paths instead of opening a new one', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/terminal', 'route.terminal', false)
    const second = desktop.openWindow('/terminal', 'route.terminal', false)
    expect(second).toBe(first)
    expect(desktop.windows.value.filter((item) => item.path === '/terminal')).toHaveLength(1)
  })

  it('allows multiple windows for multi-instance paths', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/files', 'route.files', true)
    const second = desktop.openWindow('/files', 'route.files', true)
    expect(second).not.toBe(first)
    expect(desktop.windows.value.filter((item) => item.path === '/files')).toHaveLength(2)
  })

  it('minimizes and restores a window, moving focus to the top sibling', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/overview', 'route.overview', true)
    const second = desktop.openWindow('/files', 'route.files', true)
    desktop.minimizeWindow(second)
    expect(desktop.windows.value.find((item) => item.id === second)?.minimized).toBe(true)
    expect(desktop.focusedId.value).toBe(first)
    desktop.restoreWindow(second)
    expect(desktop.windows.value.find((item) => item.id === second)?.minimized).toBe(false)
    expect(desktop.focusedId.value).toBe(second)
  })

  it('brings the focused window to the front', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/overview', 'route.overview', true)
    const second = desktop.openWindow('/files', 'route.files', true)
    const firstZ = desktop.windows.value.find((item) => item.id === first)!.z
    const secondZ = desktop.windows.value.find((item) => item.id === second)!.z
    expect(secondZ).toBeGreaterThan(firstZ)
    desktop.focusWindow(first)
    expect(desktop.windows.value.find((item) => item.id === first)!.z).toBeGreaterThan(secondZ)
  })

  it('closes a window and reassigns focus', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/overview', 'route.overview', true)
    desktop.openWindow('/files', 'route.files', true)
    desktop.closeWindow(first)
    expect(desktop.windows.value.some((item) => item.id === first)).toBe(false)
    expect(desktop.focusedId.value).not.toBe(first)
  })

  it('updates window geometry', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', true)
    desktop.updateGeometry(id, { left: 40, top: 40, width: 900, height: 600 })
    expect(desktop.windows.value.find((item) => item.id === id)?.geometry).toMatchObject({ left: 40, top: 40, width: 900, height: 600 })
  })

  it('keeps the floating restore geometry while switching between snap targets', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const target = desktop.windows.value.find((item) => item.id === id)!
    const restoreGeometry = { ...target.geometry }

    desktop.snapWindow(id, 'left')
    expect(target.snap).toBe('left')
    expect(target.maximized).toBe(false)
    expect(target.geometry).toEqual(restoreGeometry)

    desktop.snapWindow(id, 'maximize')
    expect(target.snap).toBeNull()
    expect(target.maximized).toBe(true)
    expect(target.geometry).toEqual(restoreGeometry)

    const restored = desktop.restoreWindowForDrag(id, { ...restoreGeometry, left: 120, top: 80 })
    expect(target.maximized).toBe(false)
    expect(target.snap).toBeNull()
    expect(restored).toMatchObject({ left: 120, top: 80 })
  })

  it('persists partial offscreen positions through reload, snap restore and viewport changes', () => {
    setupViewport(1280, 800)
    initializeDesktopMode()
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const geometry = { left: 1000, top: 460, width: 880, height: 600 }
    desktop.updateGeometry(id, geometry, false)
    desktop.commitGeometry(id)
    expect(desktop.windows.value[0]?.geometry).toEqual(geometry)
    expect(window.localStorage.getItem('kpanel:desktop-window-sizes:v1')).toBeNull()

    resetDesktopModeForTest()
    initializeDesktopMode()
    expect(desktop.windows.value[0]?.geometry).toEqual(geometry)
    desktop.snapWindow(id, 'right')
    expect(desktop.restoreWindowForDrag(id, geometry)).toEqual(geometry)
    desktop.resizeForViewport({ width: 768, height: 600 })
    const resized = desktop.windows.value[0]!.geometry
    expect(resized.left).toBe(568)
    expect(resized.top).toBe(460)
    expect(resized.width).toBe(720)
  })

  it('restores valid side snap state and safely drops it on narrow viewports', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', JSON.stringify([{
      id: 2,
      path: '/overview',
      geometry: { left: 100, top: 80, width: 880, height: 600 },
      minimized: false,
      maximized: false,
      snap: 'right',
    }]))

    initializeDesktopMode(storage, { width: 1280, height: 800 })
    expect(useDesktopMode().windows.value[0]?.snap).toBe('right')
    resetDesktopModeForTest()
    initializeDesktopMode(storage, { width: 700, height: 800 })
    expect(useDesktopMode().windows.value[0]?.snap).toBeNull()
  })

  it('does not write window storage during geometry frames and commits once at gesture end', () => {
    setupViewport(1280, 800)
    initializeDesktopMode(window.localStorage, { width: 1280, height: 800 })
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', false)
    const before = window.localStorage.getItem('kejilion-panel-desktop-windows')

    desktop.updateGeometry(id, { left: 40, top: 50, width: 1040, height: 650 }, false)
    desktop.updateGeometry(id, { left: 60, top: 70, width: 1040, height: 650 }, false)
    expect(window.localStorage.getItem('kejilion-panel-desktop-windows')).toBe(before)

    desktop.commitGeometry(id)
    expect(window.localStorage.getItem('kejilion-panel-desktop-windows')).not.toBe(before)
    expect(window.localStorage.getItem('kpanel:desktop-window-sizes:v1')).toContain('1040')
  })

  it('remembers only the last user-resized size for a canonical application', () => {
    setupViewport(1440, 900)
    initializeDesktopMode(window.localStorage, { width: 1440, height: 900 })
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/overview', 'route.overview', false)
    desktop.updateGeometry(first, { left: 40, top: 50, width: 1040, height: 650 }, false)
    desktop.commitGeometry(first)
    desktop.closeWindow(first)

    const reopened = desktop.openWindow('/monitoring', 'route.monitoring', true)
    expect(desktop.windows.value.find((item) => item.id === reopened)?.geometry).toMatchObject({
      width: 1040,
      height: 650,
    })
    const raw = window.localStorage.getItem('kpanel:desktop-window-sizes:v1') ?? ''
    expect(raw).toContain('/overview')
    expect(raw).not.toContain('/monitoring')
  })

  it('never stores queries, file paths or script identifiers in size preferences', () => {
    setupViewport(1440, 900)
    initializeDesktopMode(window.localStorage, { width: 1440, height: 900 })
    const desktop = useDesktopMode()
    const sites = desktop.openWindow('/sites?domain=secret.example', 'route.sites', false)
    desktop.updateGeometry(sites, { left: 40, top: 40, width: 1000, height: 640 }, false)
    desktop.commitGeometry(sites)
    const files = desktop.openWindow('/files?path=/root/private', 'route.files', false)
    desktop.updateGeometry(files, { left: 60, top: 60, width: 960, height: 620 }, false)
    desktop.commitGeometry(files)
    const script = desktop.openWindow('/app-script/private-agent', 'desktop.scriptWindowTitle', false)
    desktop.updateGeometry(script, { left: 70, top: 70, width: 900, height: 620 }, false)
    desktop.commitGeometry(script)

    const raw = window.localStorage.getItem('kpanel:desktop-window-sizes:v1') ?? ''
    expect(raw).toContain('/sites')
    expect(raw).toContain('/files')
    expect(raw).toContain('/app-script')
    expect(raw).not.toContain('secret.example')
    expect(raw).not.toContain('/root/private')
    expect(raw).not.toContain('private-agent')
  })

  it('keeps a large-screen preference when opening on a temporarily smaller viewport', () => {
    setupViewport(1440, 900)
    initializeDesktopMode(window.localStorage, { width: 1440, height: 900 })
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/overview', 'route.overview', false)
    desktop.updateGeometry(first, { left: 40, top: 50, width: 1040, height: 650 }, false)
    desktop.commitGeometry(first)
    desktop.closeWindow(first)

    setupViewport(600, 500)
    const small = desktop.openWindow('/overview', 'route.overview', false)
    expect(desktop.windows.value.find((item) => item.id === small)?.geometry.width).toBeLessThan(1040)
    desktop.closeWindow(small)

    setupViewport(1440, 900)
    const largeAgain = desktop.openWindow('/overview', 'route.overview', false)
    expect(desktop.windows.value.find((item) => item.id === largeAgain)?.geometry.width).toBe(1040)
  })

  it('ignores oversized and invalid window size preference payloads', () => {
    const storage = makeStorage()
    storage.store.set('kpanel:desktop-window-sizes:v1', `[${' '.repeat(8_193)}]`)
    initializeDesktopMode(storage, { width: 1280, height: 800 })
    setupViewport(1280, 800)
    let desktop = useDesktopMode()
    let id = desktop.openWindow('/overview', 'route.overview', false)
    expect(desktop.windows.value.find((item) => item.id === id)?.geometry.width).toBe(880)

    resetDesktopModeForTest()
    storage.store.set('kpanel:desktop-window-sizes:v1', JSON.stringify([
      { path: '/overview', width: -1, height: 600 },
      { path: '/files?path=/secret', width: 900, height: 600 },
      { path: '/unknown', width: 900, height: 600 },
      { path: '/sites', width: 20_000, height: 600 },
    ]))
    initializeDesktopMode(storage, { width: 1280, height: 800 })
    desktop = useDesktopMode()
    id = desktop.openWindow('/overview', 'route.overview', false)
    expect(desktop.windows.value.find((item) => item.id === id)?.geometry.width).toBe(880)
  })

  it('caps the number of open windows to the fixed limit', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    for (let index = 0; index < 8; index += 1) {
      expect(desktop.openWindow('/ai', 'route.ai', true)).toBeGreaterThan(0)
    }
    expect(desktop.openWindow('/ai', 'route.ai', true)).toBe(0)
    expect(desktop.windows.value).toHaveLength(8)
  })

  it('navigates an existing single-instance window when a cross-app handoff supplies a full path', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const first = desktop.openWindow('/files?path=/home', 'route.files', false)
    const second = desktop.openWindow('/files?path=/home/web/site', 'route.files', false, true)

    expect(second).toBe(first)
    expect(desktop.windows.value).toHaveLength(1)
    expect(desktop.windows.value[0]?.path).toBe('/files?path=/home/web/site')
  })

  it('restores persisted mode on initialize', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-mode', 'desktop')
    initializeDesktopMode(storage, { width: 1280, height: 800 })
    expect(useDesktopMode().mode.value).toBe('desktop')
    resetDesktopModeForTest()
  })

  it('restores persisted windows with normalized geometry', () => {
    const storage = makeStorage()
    storage.store.set(
      'kejilion-panel-desktop-windows',
      JSON.stringify([
        { id: 7, path: '/overview', titleKey: 'route.overview', geometry: { left: 30, top: 40, width: 800, height: 500 }, minimized: false, maximized: false },
        { id: 8, path: '/files', titleKey: 'route.files', geometry: { left: 9999, top: 9999, width: 900, height: 600 }, minimized: false, maximized: false },
      ]),
    )
    initializeDesktopMode(storage, { width: 1280, height: 800 })
    const desktop = useDesktopMode()
    expect(desktop.windows.value).toHaveLength(2)
    expect(desktop.windows.value[0]!.path).toBe('/overview')
    // An unreachable persisted window retains a usable title-bar slice.
    expect(desktop.windows.value[1]!.geometry.left).toBe(1280 - 200)
    expect(desktop.focusedId.value).toBe(8)
    resetDesktopModeForTest()
  })

  it('restores a safe dynamic app script terminal route', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', JSON.stringify([
      { id: 9, path: '/app-script/openclaw', titleKey: 'route.apps', geometry: {}, minimized: false },
      { id: 10, path: '/app-script/bad/path', titleKey: 'desktop.scriptWindowTitle', geometry: {}, minimized: false },
    ]))

    initializeDesktopMode(storage, { width: 1280, height: 800 })
    const windows = useDesktopMode().windows.value
    expect(windows).toHaveLength(1)
    expect(windows[0]?.path).toBe('/app-script/openclaw')
    expect(windows[0]?.titleKey).toBe('desktop.scriptWindowTitle')
  })

  it('ignores malformed persisted windows', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', '{not json')
    initializeDesktopMode(storage, { width: 1280, height: 800 })
    expect(useDesktopMode().windows.value).toHaveLength(0)
    resetDesktopModeForTest()
  })

  it('rejects unknown persisted routes and derives trusted titles from the app catalogue', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', JSON.stringify([
      { id: 1, path: '/not-an-app', titleKey: 'route.settings', geometry: {}, minimized: false },
      { id: 2, path: '/files?path=/home', titleKey: '<script>', geometry: {}, minimized: false },
    ]))

    initializeDesktopMode(storage, { width: 1280, height: 800 })
    const windows = useDesktopMode().windows.value
    expect(windows).toHaveLength(1)
    expect(windows[0]?.path).toBe('/files?path=/home')
    expect(windows[0]?.titleKey).toBe('route.files')
  })

  it('rejects oversized persisted window payloads before parsing', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', `[${' '.repeat(64_001)}]`)

    initializeDesktopMode(storage, { width: 1280, height: 800 })
    expect(useDesktopMode().windows.value).toHaveLength(0)
  })

  it('persists and restores a bounded full path including its query', () => {
    window.localStorage.clear()
    setupViewport(1280, 800)
    initializeDesktopMode(window.localStorage, { width: 1280, height: 800 })
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/files', 'route.files', false)
    desktop.updateWindowRoute(id, '/files?path=/home/web/site', 'route.files')
    expect(window.localStorage.getItem('kejilion-panel-desktop-windows')).toContain('/files?path=/home/web/site')

    resetDesktopModeForTest()
    initializeDesktopMode(window.localStorage, { width: 1280, height: 800 })
    expect(useDesktopMode().windows.value[0]?.path).toBe('/files?path=/home/web/site')
    window.localStorage.clear()
  })

  it('allocates unique ids when persisted ids are invalid or duplicated', () => {
    const storage = makeStorage()
    storage.store.set('kejilion-panel-desktop-windows', JSON.stringify([
      { id: 2, path: '/overview', geometry: {}, minimized: false },
      { id: 1, path: '/files', geometry: {}, minimized: false },
      { id: 1, path: '/terminal', geometry: {}, minimized: false },
    ]))

    initializeDesktopMode(storage, { width: 1280, height: 800 })
    const ids = useDesktopMode().windows.value.map((item) => item.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(ids.every((id) => id > 0)).toBe(true)
  })

  it('tolerates unavailable storage without throwing', () => {
    const brokenStorage = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    }
    expect(() => initializeDesktopMode(brokenStorage, { width: 1280, height: 800 })).not.toThrow()
    const desktop = useDesktopMode()
    expect(() => desktop.openWindow('/overview', 'route.overview', true)).not.toThrow()
  })

  it('persists mode and windows to browser storage', () => {
    window.localStorage.clear()
    setupViewport(1280, 800)
    initializeDesktopMode(window.localStorage, { width: 1280, height: 800 })
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', true)
    expect(window.localStorage.getItem('kejilion-panel-desktop-mode')).toBe('desktop')
    expect(window.localStorage.getItem('kejilion-panel-desktop-windows')).toContain('/overview')
    window.localStorage.clear()
  })

  it('removes the focused id when the last window closes', () => {
    setupViewport(1280, 800)
    const desktop = useDesktopMode()
    const id = desktop.openWindow('/overview', 'route.overview', true)
    desktop.closeWindow(id)
    expect(desktop.windows.value).toHaveLength(0)
    expect(desktop.focusedId.value).toBe(0)
  })
})

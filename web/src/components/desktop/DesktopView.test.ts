// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from '@/components/desktop/DesktopView.vue'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import { useTheme } from '@/stores/theme'

function setupViewport(width: number, height: number): void {
  Object.defineProperty(window, 'innerWidth', { value: width, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: height, configurable: true })
}

function touchPointer(type: string, x = 40, y = 50, id = 1): PointerEvent {
  const event = new Event(type, { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperties(event, {
    button: { value: 0 },
    clientX: { value: x },
    clientY: { value: y },
    pointerId: { value: id },
    pointerType: { value: 'touch' },
  })
  return event
}

describe('DesktopView', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', {
      configurable: true,
      value: vi.fn(() => null),
    })
    setupViewport(1280, 800)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    Object.defineProperties(document, {
      fullscreenEnabled: { configurable: true, value: false },
      fullscreenElement: { configurable: true, value: null },
      exitFullscreen: { configurable: true, value: undefined },
    })
    Object.defineProperty(document.documentElement, 'requestFullscreen', {
      configurable: true,
      value: undefined,
    })
  })

  it('renders the icon grid for all desktop apps', () => {
    const wrapper = mount(DesktopView)
    const icons = wrapper.findAll('.desktop__icon')
    expect(icons.length).toBeGreaterThanOrEqual(11)
    wrapper.unmount()
  })

  it('selects an icon on single click and opens it on double click', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    const wrapper = mount(DesktopView)
    const icon = wrapper.find('.desktop__icon')

    await icon.trigger('click')
    expect(icon.classes()).toContain('desktop__icon--selected')
    expect(desktop.windows.value).toHaveLength(0)

    await icon.trigger('dblclick')
    await nextTick()
    expect(desktop.windows.value).toHaveLength(1)
    wrapper.unmount()
  })

  it('launches the system center from its dedicated desktop icon', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    const wrapper = mount(DesktopView)
    const icon = wrapper.findAll('.desktop__icon')
      .find((entry) => entry.find('.system-center-icon').exists())

    expect(icon).toBeDefined()
    await icon!.trigger('dblclick')
    await nextTick()

    expect(desktop.windows.value).toHaveLength(1)
    expect(desktop.windows.value[0]).toMatchObject({
      path: '/system',
      titleKey: 'route.systemCenter',
    })
    wrapper.unmount()
  })

  it('opens a desktop app on one touch tap', async () => {
    setupViewport(390, 844)
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    const wrapper = mount(DesktopView)
    const icon = wrapper.find('.desktop__icon')

    icon.element.dispatchEvent(touchPointer('pointerdown'))
    window.dispatchEvent(touchPointer('pointerup'))
    await icon.trigger('click')
    await icon.trigger('dblclick')
    await nextTick()

    expect(desktop.windows.value).toHaveLength(1)
    wrapper.unmount()
  })

  it('opens the desktop context menu on touch long press without launching the app', async () => {
    vi.useFakeTimers()
    setupViewport(390, 844)
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    const wrapper = mount(DesktopView)
    const icon = wrapper.find('.desktop__icon')

    icon.element.dispatchEvent(touchPointer('pointerdown', 60, 70))
    vi.advanceTimersByTime(520)
    await nextTick()

    expect(wrapper.find('.desktop__context-menu').exists()).toBe(true)
    window.dispatchEvent(touchPointer('pointerup', 60, 70))
    await icon.trigger('click')
    expect(desktop.windows.value).toHaveLength(0)
    wrapper.unmount()
  })

  it('renders a window when an app is opened', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', true)
    await nextTick()
    const wrapper = mount(DesktopView)
    await nextTick()
    expect(wrapper.find('.desktop-window').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders a taskbar item for each open window', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', true)
    desktop.openWindow('/files', 'route.files', true)
    await nextTick()
    const wrapper = mount(DesktopView)
    await nextTick()
    expect(wrapper.findAll('.desktop__taskbar-item').length).toBe(2)
    wrapper.unmount()
  })

  it('shows the window name on taskbar hover and closes it from the item context menu', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', false)
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()

    const taskbarItem = wrapper.find('.desktop__taskbar-item')
    expect(taskbarItem.attributes('title')).toBe(taskbarItem.find('.desktop__taskbar-label').text())

    await taskbarItem.trigger('contextmenu', { clientX: 620, clientY: 740 })
    await nextTick()
    const closeAction = wrapper.find('[data-context-action="close-window"]')
    expect(closeAction.exists()).toBe(true)
    expect(wrapper.find('[data-context-action="processes"]').exists()).toBe(false)

    await closeAction.trigger('click')
    vi.advanceTimersByTime(1)
    await nextTick()
    expect(desktop.windows.value).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows the classic-mode switch button', () => {
    const wrapper = mount(DesktopView)
    expect(wrapper.find('.desktop__classic-button').exists()).toBe(true)
    wrapper.unmount()
  })

  it('mirrors the classic Agent status and version in the taskbar', () => {
    const wrapper = mount(DesktopView, {
      props: {
        agent: {
          connected: true,
          compatible: true,
          readOnly: false,
          version: '0.48.3',
          protocolVersion: '1',
        },
      },
    })
    expect(wrapper.find('.desktop__taskbar-agent-status').text()).toContain('Agent 在线')
    expect(wrapper.find('.desktop__taskbar-agent > small').text()).toBe('v0.48.3')
    wrapper.unmount()
  })

  it('replaces the taskbar version with the classic update action', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView, {
      props: {
        agent: {
          connected: true,
          compatible: true,
          readOnly: false,
          version: '0.48.3',
          protocolVersion: '1',
        },
        kpanelUpdateAvailable: true,
        kpanelUpdateDescription: '当前版本 v0.48.3，发现可用更新',
      },
    })
    const update = wrapper.find('.desktop__taskbar-agent-update')
    expect(update.text()).toContain('更新可用')
    expect(wrapper.find('.desktop__taskbar-agent > small').exists()).toBe(false)
    await update.trigger('click')
    expect(desktop.windows.value[0]?.path).toBe('/apps?app=kpanel&action=update')
    wrapper.unmount()
  })

  it('moves keyboard focus to the desktop when its blank surface is pressed', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await wrapper.trigger('pointerdown')
    expect(document.activeElement).toBe(wrapper.element)
    wrapper.unmount()
  })

  it('makes closing immediate when reduced motion is requested', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', false)
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()

    await wrapper.find('.desktop-window__action--close').trigger('click')
    vi.advanceTimersByTime(1)
    await nextTick()

    expect(desktop.windows.value).toHaveLength(0)
    wrapper.unmount()
  })

  it('switches back to classic mode from the taskbar system area', async () => {
    let fullscreenElement: Element | null = document.documentElement
    const exitFullscreen = vi.fn(async () => {
      fullscreenElement = null
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    Object.defineProperties(document, {
      fullscreenElement: { configurable: true, get: () => fullscreenElement },
      exitFullscreen: { configurable: true, value: exitFullscreen },
    })
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    const wrapper = mount(DesktopView)
    await wrapper.find('.desktop__classic-button').trigger('click')
    await flushPromises()
    expect(exitFullscreen).not.toHaveBeenCalled()
    expect(fullscreenElement).toBe(document.documentElement)
    expect(desktop.mode.value).toBe('classic')
    wrapper.unmount()
  })

  it('places the wallpaper action below theme and fullscreen below wallpaper', async () => {
    let fullscreenElement: Element | null = null
    const requestFullscreen = vi.fn(async () => {
      fullscreenElement = document.documentElement
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    const exitFullscreen = vi.fn(async () => {
      fullscreenElement = null
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    Object.defineProperties(document, {
      fullscreenEnabled: { configurable: true, value: true },
      fullscreenElement: { configurable: true, get: () => fullscreenElement },
      exitFullscreen: { configurable: true, value: exitFullscreen },
    })
    Object.defineProperty(document.documentElement, 'requestFullscreen', {
      configurable: true,
      value: requestFullscreen,
    })
    const wrapper = mount(DesktopView)

    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    const actions = wrapper.findAll('[role="menuitem"]')
    const themeIndex = actions.findIndex((action) => action.attributes('data-context-action') === 'theme')
    const wallpaperIndex = actions.findIndex((action) => action.attributes('data-context-action') === 'wallpaper')
    const fullscreenIndex = actions.findIndex((action) => action.attributes('data-context-action') === 'fullscreen')
    expect(wallpaperIndex).toBe(themeIndex + 1)
    expect(fullscreenIndex).toBe(wallpaperIndex + 1)
    expect(actions[fullscreenIndex]?.text()).toContain('进入全屏')

    await actions[fullscreenIndex]!.trigger('click')
    await flushPromises()
    expect(requestFullscreen).toHaveBeenCalledOnce()
    expect(fullscreenElement).toBe(document.documentElement)

    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    const exitAction = wrapper.find('[data-context-action="fullscreen"]')
    expect(exitAction.text()).toContain('退出全屏')
    await exitAction.trigger('click')
    await flushPromises()
    expect(exitFullscreen).toHaveBeenCalledOnce()
    expect(fullscreenElement).toBeNull()
    wrapper.unmount()
  })

  it('waits for the context menu leave transition before changing theme', async () => {
    const theme = useTheme()
    theme.setTheme('light')
    const wrapper = mount(DesktopView, {
      global: { stubs: { transition: false } },
    })

    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(true)

    await wrapper.find('[data-context-action="theme"]').trigger('click')
    await nextTick()
    expect(theme.resolved.value).toBe('light')

    await new Promise((resolve) => window.setTimeout(resolve, 180))
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    expect(theme.resolved.value).toBe('dark')

    wrapper.unmount()
    theme.setTheme('system')
  })

  it('changes the wallpaper from the desktop menu and restores the saved choice', async () => {
    const wrapper = mount(DesktopView)

    expect(wrapper.find('.desktop__wallpaper-image').attributes('data-wallpaper')).toBe('classic')
    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    await wrapper.find('[data-context-action="wallpaper"]').trigger('click')
    await nextTick()

    const dialog = document.body.querySelector<HTMLElement>('.desktop-wallpaper-picker')
    const orbit = dialog?.querySelector<HTMLButtonElement>('[data-wallpaper-option="orbit"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.querySelectorAll('[data-wallpaper-option]')).toHaveLength(5)
    expect(orbit?.getAttribute('aria-checked')).toBe('false')

    orbit?.click()
    await nextTick()
    expect(wrapper.find('.desktop__wallpaper-image').attributes('data-wallpaper')).toBe('orbit')
    expect(wrapper.find('.desktop__wallpaper-image').attributes('style')).toContain('kpanel-desktop-orbit.webp')
    expect(window.localStorage.getItem('kpanel:desktop-wallpaper:v1')).toBe('orbit')
    expect(document.body.querySelector('.desktop-wallpaper-picker')).toBeNull()
    wrapper.unmount()

    const restored = mount(DesktopView)
    expect(restored.find('.desktop__wallpaper-image').attributes('data-wallpaper')).toBe('orbit')
    restored.unmount()
  })

  it('opens and closes a context menu on right-click', async () => {
    const wrapper = mount(DesktopView)
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(true)
    await wrapper.find('.desktop__context-menu button').trigger('click')
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens the process manager from the taskbar context menu', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/overview', 'route.overview', false)
    const wrapper = mount(DesktopView)

    await wrapper.find('.desktop__taskbar').trigger('contextmenu', { clientX: 500, clientY: 760 })
    await nextTick()
    const action = wrapper.find('[data-context-action="processes"]')
    expect(action.exists()).toBe(true)
    expect(action.text()).toContain('进程管理器')

    await action.trigger('click')
    await nextTick()
    expect(desktop.windows.value).toHaveLength(2)
    expect(desktop.windows.value.find((windowState) => windowState.path === '/processes')).toMatchObject({
      path: '/processes',
      titleKey: 'route.processes',
    })
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the context menu stable while the right mouse button is held', async () => {
    const wrapper = mount(DesktopView)
    await wrapper.trigger('contextmenu', { clientX: 240, clientY: 180 })
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(true)

    window.dispatchEvent(new MouseEvent('pointerdown', { button: 2, bubbles: true }))
    window.dispatchEvent(new MouseEvent('pointerdown', { button: 2, bubbles: true }))
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(true)

    window.dispatchEvent(new MouseEvent('pointerdown', { button: 0, bubbles: true }))
    await nextTick()
    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('supports arrow-key menu navigation and restores focus on Escape', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    wrapper.element.focus()
    await wrapper.trigger('contextmenu', { clientX: 200, clientY: 150 })
    await nextTick()
    const items = wrapper.findAll('[role="menuitem"]')
    expect(document.activeElement).toBe(items[0]?.element)

    await items[0]!.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(items[1]?.element)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(wrapper.find('.desktop__context-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.element)
    wrapper.unmount()
  })

  it('renders a window icon even when the app is not in the catalogue', async () => {
    const desktop = useDesktopMode()
    desktop.enterDesktop()
    desktop.openWindow('/unknown-page', 'route.settings', true)
    await nextTick()
    const wrapper = mount(DesktopView)
    await nextTick()
    expect(wrapper.findAll('.desktop-window').length).toBe(1)
    wrapper.unmount()
  })
})

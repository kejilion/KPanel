// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from './DesktopView.vue'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import { resetThemeForTest } from '@/stores/theme'
import type { DesktopEntries } from '@/lib/desktopEntries'
import { api } from '@/lib/api'
import type { DesktopWorkspace } from '@/types/api'

vi.mock('@/lib/desktopWindowRoute', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/desktopWindowRoute')>(),
  resolveWindowComponent: async () => ({
    template: '<main><input aria-label="Window field"><div class="xterm" tabindex="0"></div><button>Action</button></main>',
  }),
}))

vi.mock('@/lib/desktopEntries', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/desktopEntries')>(),
  loadDesktopEntries: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      sites: { ...actual.api.sites, appearance: vi.fn() },
      desktop: { ...actual.api.desktop, workspace: vi.fn() },
    },
  }
})

const mockedLoad = vi.mocked((await import('@/lib/desktopEntries')).loadDesktopEntries)

function workspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
  return {
    schemaVersion: 3,
    resourceVersion: `sha256:${'1'.repeat(64)}`,
    available: true,
    hiddenEntryKeys: ['app:kuma'],
    positions: {},
    widgetPositions: {},
    labels: {},
    shortcuts: [],
    ...overrides,
  }
}

function entries(): DesktopEntries {
  return {
    apps: [],
    sites: [],
    visible: [
      { key: 'site:blog', kind: 'site', id: 'blog', name: 'blog.example.com', launch: 'external', url: 'https://blog.example.com' },
      { key: 'app:nginx', kind: 'app', id: 'nginx', name: 'Nginx', launch: 'external', url: 'http://192.168.1.5:8080' },
      { key: 'app:kuma', kind: 'app', id: 'kuma', name: 'Uptime Kuma', launch: 'external', url: 'http://192.168.1.5:3001' },
    ],
    loadedAt: Date.now(),
  }
}

describe('DesktopView start menu', () => {
  let wrapper: VueWrapper
  const desktop = useDesktopMode()

  beforeEach(() => {
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    resetDesktopModeForTest()
    resetDesktopIconsForTest()
    resetThemeForTest()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
    mockedLoad.mockResolvedValue(entries())
    vi.mocked(api.sites.appearance).mockResolvedValue({})
    vi.mocked(api.desktop.workspace).mockResolvedValue(workspace())
    desktop.enterDesktop()
  })

  afterEach(() => {
    wrapper?.unmount()
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  async function mountDesktop() {
    wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    return wrapper
  }

  function startButton() {
    return wrapper.get<HTMLButtonElement>('.desktop__start-button')
  }

  async function openMenu() {
    await startButton().trigger('click')
    await flushPromises()
  }

  async function keydown(target: Element, init: KeyboardEventInit) {
    const event = new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init })
    target.dispatchEvent(event)
    await flushPromises()
    return event
  }

  it('turns the taskbar K into a labelled toggle for the start menu', async () => {
    await mountDesktop()
    expect(startButton().attributes()).toMatchObject({
      'aria-haspopup': 'dialog',
      'aria-expanded': 'false',
      'aria-controls': 'desktop-start-menu',
      'aria-label': '开始',
    })
    expect(startButton().find('.brand__mark').exists()).toBe(true)
    await openMenu()
    expect(startButton().attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(true)
    await openMenu()
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(startButton().element)
  })

  it('lists every desktop source, including hidden apps, sorted by kind and name', async () => {
    await mountDesktop()
    await openMenu()
    const keys = wrapper.findAll('#desktop-start-menu [role="option"]').map((option) => option.attributes('data-start-menu-key'))
    expect(keys.slice(0, 14).every((key) => key?.startsWith('nav:'))).toBe(true)
    expect(keys.slice(14)).toEqual(['app:nginx', 'app:kuma', 'site:blog'])
    expect(wrapper.get('[data-start-menu-key="app:kuma"]').text()).toContain('桌面已隐藏')
    // The desktop itself still hides the app.
    expect(wrapper.find('.desktop__icons [aria-label="Uptime Kuma"]').exists()).toBe(false)
  })

  it('launches system apps into windows and closes the menu', async () => {
    await mountDesktop()
    await openMenu()
    await wrapper.get('[data-start-menu-key="nav:/docker"]').trigger('click')
    await flushPromises()
    expect(desktop.windows.value.map((item) => item.path)).toEqual(['/docker'])
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
  })

  it('opens a hidden app through the same external-link confirmation as the desktop', async () => {
    await mountDesktop()
    await openMenu()
    await wrapper.get('[data-start-menu-key="app:kuma"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('确认跳转')
    expect(document.body.textContent).toContain('Uptime Kuma')
  })

  it('runs searched desktop actions such as the wallpaper picker', async () => {
    await mountDesktop()
    await openMenu()
    const search = wrapper.get<HTMLInputElement>('#desktop-start-menu input')
    await search.setValue('wallpaper')
    await search.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(document.body.textContent).toContain('更换桌面壁纸和主题')
  })

  it('toggles with Ctrl/Command+K outside text fields, terminals and dialogs', async () => {
    const [id] = [desktop.openWindow('/settings', 'route.settings', true)]
    await mountDesktop()
    const shell = wrapper.element as HTMLElement
    shell.focus()
    expect((await keydown(shell, { key: 'k', ctrlKey: true })).defaultPrevented).toBe(true)
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(true)
    const search = wrapper.get('#desktop-start-menu input').element
    expect(document.activeElement).toBe(search)
    await keydown(search, { key: 'K', metaKey: true })
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)

    const windowElement = wrapper.get(`#desktop-window-${id}`)
    for (const selector of ['input', '.xterm']) {
      const target = windowElement.get(selector).element as HTMLElement
      target.focus()
      expect((await keydown(target, { key: 'k', ctrlKey: true })).defaultPrevented).toBe(false)
      expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
    }
    const button = windowElement.get('button').element as HTMLElement
    button.focus()
    expect((await keydown(button, { key: 'k', ctrlKey: true })).defaultPrevented).toBe(true)
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(true)
    await keydown(document.activeElement!, { key: 'Escape' })

    document.body.classList.add('has-modal')
    try {
      shell.focus()
      expect((await keydown(shell, { key: 'k', ctrlKey: true })).defaultPrevented).toBe(false)
    } finally {
      document.body.classList.remove('has-modal')
    }
  })

  it('closes on outside presses but not on presses inside the menu', async () => {
    await mountDesktop()
    await openMenu()
    const menu = wrapper.get('#desktop-start-menu').element
    menu.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
    await nextTick()
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(true)
    wrapper.get('.desktop__icons').element.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0 }))
    await nextTick()
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
  })

  it('keeps the menu open when Escape only closes its language list', async () => {
    await mountDesktop()
    await openMenu()
    const trigger = wrapper.get('#desktop-start-menu .language-selector__trigger')
    await trigger.trigger('click')
    await keydown(trigger.element, { key: 'Escape' })
    expect(wrapper.find('#desktop-start-menu .language-selector__menu').exists()).toBe(false)
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(true)
  })

  it('closes the start menu when the taskbar context menu opens from the K button', async () => {
    await mountDesktop()
    await openMenu()
    await startButton().trigger('contextmenu', { clientX: 20, clientY: 780 })
    await flushPromises()
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
    expect(wrapper.find('.desktop__context-menu [data-context-action="processes"]').exists()).toBe(true)
  })

  it('returns focus to the K button on Escape', async () => {
    await mountDesktop()
    await openMenu()
    await keydown(wrapper.get('#desktop-start-menu input').element, { key: 'Escape' })
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(startButton().element)
  })

  it('asks the shell to sign out after desktop close guards pass', async () => {
    await mountDesktop()
    await openMenu()
    await wrapper.get('.desktop-start-menu__user').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('sign-out')).toHaveLength(1)
    expect(wrapper.find('#desktop-start-menu').exists()).toBe(false)

    await wrapper.setProps({ signingOut: true })
    await openMenu()
    expect(wrapper.get('.desktop-start-menu__user').attributes('disabled')).toBeDefined()
  })

  it('explains entry load failures and retries from the menu', async () => {
    mockedLoad.mockRejectedValueOnce(new Error('offline'))
    await mountDesktop()
    await openMenu()
    expect(wrapper.get('#desktop-start-menu [role="status"]').text()).toContain('已安装的应用和网站暂时无法读取。')
    await wrapper.get('.desktop-start-menu__retry').trigger('click')
    await flushPromises()
    expect(mockedLoad).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-start-menu-key="app:nginx"]').exists()).toBe(true)
  })
})

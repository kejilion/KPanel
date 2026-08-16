// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from '@/components/desktop/DesktopView.vue'
import DesktopShortcutDialog from '@/components/desktop/DesktopShortcutDialog.vue'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import type { DesktopEntries } from '@/lib/desktopEntries'
import { api } from '@/lib/api'
import type { DesktopWorkspace, DesktopWorkspaceUpdate } from '@/types/api'

vi.mock('@/lib/desktopEntries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/desktopEntries')>()
  return {
    ...actual,
    loadDesktopEntries: vi.fn(),
  }
})

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      sites: {
        ...actual.api.sites,
        appearance: vi.fn(),
      },
      desktop: {
        ...actual.api.desktop,
        workspace: vi.fn(),
        updateWorkspace: vi.fn(),
        uploadShortcutIcon: vi.fn(),
        removeShortcutIcon: vi.fn(),
      },
    },
  }
})

const mockedLoad = vi.mocked((await import('@/lib/desktopEntries')).loadDesktopEntries)
const mockedAppearance = vi.mocked(api.sites.appearance)
const mockedWorkspace = vi.mocked(api.desktop.workspace)
const mockedWorkspaceUpdate = vi.mocked(api.desktop.updateWorkspace)
const mockedUploadShortcutIcon = vi.mocked(api.desktop.uploadShortcutIcon)

function makeWorkspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
  return {
    schemaVersion: 1,
    resourceVersion: `sha256:${'1'.repeat(64)}`,
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    labels: {},
    shortcuts: [],
    ...overrides,
  }
}

function makeEntries(): DesktopEntries {
  return {
    apps: [],
    sites: [],
    visible: [
      { key: 'app:nginx', kind: 'app', id: 'nginx', name: 'Nginx', launch: 'external', url: 'http://192.168.1.5:8080', iconURL: '/api/v1/apps/nginx/icon', app: undefined },
      { key: 'app:openclaw', kind: 'app', id: 'openclaw', name: 'OpenClaw', launch: 'script', iconURL: '/api/v1/apps/openclaw/icon', app: undefined },
      { key: 'site:blog', kind: 'site', id: 'blog', name: 'blog.example.com', launch: 'external', url: 'https://blog.example.com', iconURL: '/api/v1/sites/blog/icon', site: undefined },
    ],
    loadedAt: Date.now(),
  }
}

describe('DesktopView dynamic entries', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    resetDesktopIconsForTest()
    window.localStorage.clear()
    window.scrollTo = vi.fn()
    window.open = vi.fn()
    mockedLoad.mockResolvedValue(makeEntries())
    mockedAppearance.mockReset()
    mockedAppearance.mockResolvedValue({})
    mockedWorkspace.mockReset()
    mockedWorkspace.mockResolvedValue(makeWorkspace())
    mockedWorkspaceUpdate.mockReset()
    mockedWorkspaceUpdate.mockImplementation(async (body: DesktopWorkspaceUpdate) => makeWorkspace({
      resourceVersion: `sha256:${'2'.repeat(64)}`,
      hiddenEntryKeys: body.hiddenEntryKeys,
      positions: body.positions,
      labels: body.labels,
      shortcuts: body.shortcuts.map((shortcut) => ({
        ...shortcut,
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      })),
    }))
    mockedUploadShortcutIcon.mockReset()
    mockedUploadShortcutIcon.mockResolvedValue({
      iconVersion: 'c'.repeat(64),
      iconURL: '/api/v1/desktop/shortcuts/icon',
    })
  })

  it('renders dynamic app and site icons alongside static nav icons', async () => {
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()
    const labels = wrapper.findAll('.desktop__icon-label').map((el) => el.text())
    expect(labels).toContain('Nginx')
    expect(labels).toContain('OpenClaw')
    expect(labels).toContain('blog.example.com')
    // Static nav icons still present.
    expect(labels).toContain('概览')
    expect(labels).not.toContain('不存在的应用')
    wrapper.unmount()
  })

  it('renders external URLs as img sources for dynamic entries', async () => {
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()
    const imgs = wrapper.findAll('.desktop__icon-img')
    const srcs = imgs.map((img) => img.attributes('src'))
    expect(srcs).toContain('/api/v1/apps/nginx/icon')
    expect(srcs).toContain('/api/v1/apps/openclaw/icon')
    expect(srcs).toContain('/api/v1/sites/blog/icon')
    expect(wrapper.findAll('.desktop__icon-glyph--dynamic')).toHaveLength(3)
    wrapper.unmount()
  })

  it('renders a branded website fallback when its favicon is unavailable', async () => {
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()

    const siteIcon = wrapper.find('button[title="blog.example.com"]')
    await siteIcon.find('.desktop__icon-img').trigger('error')

    expect(siteIcon.find('.desktop__site-fallback-letter').text()).toBe('B')
    expect(siteIcon.find('.desktop__site-fallback-badge').exists()).toBe(true)
    wrapper.unmount()
  })

  it('confirms before opening a website in the system browser', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="blog.example.com"]').trigger('dblclick')
    await nextTick()

    expect(desktop.windows.value).toHaveLength(0)
    expect(document.body.querySelector('.modal-panel--compact')).not.toBeNull()
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent)
      .toContain('blog.example.com')
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent)
      .toContain('https://blog.example.com')
    expect(document.body.querySelector('.desktop__external-confirm-identity strong')?.textContent)
      .toBe('blog.example.com')
    expect(document.body.querySelector('.desktop__external-confirm-identity code')?.textContent)
      .toBe('https://blog.example.com')
    expect(document.body.querySelector('.desktop__external-confirm dl')).toBeNull()
    expect(document.body.querySelector('.desktop__external-confirm > small')).toBeNull()
    const confirmIcon = document.body.querySelector<HTMLImageElement>(
      '.desktop__external-confirm-icon-image',
    )
    expect(confirmIcon?.getAttribute('src')).toBe('/api/v1/sites/blog/icon')
    expect(window.open).not.toHaveBeenCalled()

    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()
    expect(window.open).toHaveBeenCalledWith(
      'https://blog.example.com',
      '_blank',
      'noopener,noreferrer',
    )
    expect(document.body.querySelector('.desktop__external-confirm')).toBeNull()
    wrapper.unmount()
  })

  it('confirms before opening a URL-capable application in the system browser', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="Nginx"]').trigger('dblclick')
    await nextTick()

    expect(desktop.windows.value).toHaveLength(0)
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent).toContain('Nginx')
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent)
      .toContain('http://192.168.1.5:8080')
    expect(document.body.querySelector('.desktop__external-confirm-identity strong')?.textContent)
      .toBe('Nginx')
    expect(document.body.querySelector('.desktop__external-confirm-identity code')?.textContent)
      .toBe('http://192.168.1.5:8080')
    expect(document.body.querySelector<HTMLImageElement>('.desktop__external-confirm-icon-image')
      ?.getAttribute('src')).toBe('/api/v1/apps/nginx/icon')
    expect(window.open).not.toHaveBeenCalled()

    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()
    expect(window.open).toHaveBeenCalledWith(
      'http://192.168.1.5:8080',
      '_blank',
      'noopener,noreferrer',
    )
    wrapper.unmount()
  })

  it('shows a branded fallback in the confirmation when the website icon fails', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="blog.example.com"]').trigger('dblclick')
    await nextTick()
    document.body.querySelector<HTMLImageElement>('.desktop__external-confirm-icon-image')
      ?.dispatchEvent(new Event('error'))
    await nextTick()

    expect(document.body.querySelector('.desktop__external-confirm .desktop__site-fallback-letter')
      ?.textContent).toBe('B')
    expect(document.body.querySelector('.desktop__external-confirm .desktop__site-fallback-badge'))
      .not.toBeNull()
    wrapper.unmount()
  })

  it('routes the website context-menu action through the same confirmation', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="blog.example.com"]').trigger('contextmenu', { clientX: 120, clientY: 80 })
    await nextTick()
    const siteItems = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
    expect(siteItems[0]?.text()).toContain('在系统浏览器中打开')
    await siteItems[0]?.trigger('click')
    expect(window.open).not.toHaveBeenCalled()
    expect(document.body.querySelector('.desktop__external-confirm')).not.toBeNull()
    wrapper.unmount()
  })

  it('routes the website-detail action through the same confirmation', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="blog.example.com"]').trigger('contextmenu', { clientX: 120, clientY: 80 })
    await nextTick()
    await wrapper.findAll('.desktop__context-menu [role="menuitem"]')[1]?.trigger('click')
    await nextTick()

    expect(document.body.querySelector('.desktop__detail')).not.toBeNull()
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()

    expect(document.body.querySelector('.desktop__detail')).toBeNull()
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent)
      .toContain('https://blog.example.com')
    expect(window.open).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the application-detail action for URL applications', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="Nginx"]').trigger('contextmenu', { clientX: 80, clientY: 80 })
    await nextTick()
    const appItems = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
    expect(appItems).toHaveLength(3)
    await appItems[1]?.trigger('click')
    expect(desktop.windows.value[0]?.path).toBe('/apps?app=nginx')
    wrapper.unmount()
  })

  it('opens the matching application-market detail from an app context menu', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="Nginx"]').trigger('contextmenu', { clientX: 80, clientY: 80 })
    await nextTick()
    await wrapper.findAll('.desktop__context-menu [role="menuitem"]')[1]?.trigger('click')

    expect(desktop.windows.value).toHaveLength(1)
    expect(desktop.windows.value[0]?.path).toBe('/apps?app=nginx')
    expect(wrapper.find('.desktop__detail').exists()).toBe(false)
    wrapper.unmount()
  })

  it('offers persistent rename only for website icons', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await nextTick()
    await nextTick()

    await wrapper.find('button[title="Nginx"]').trigger('contextmenu', { clientX: 80, clientY: 80 })
    await nextTick()
    expect(wrapper.findAll('.desktop__context-menu [role="menuitem"]')).toHaveLength(3)

    await wrapper.find('button[title="blog.example.com"]').trigger('contextmenu', { clientX: 120, clientY: 80 })
    await nextTick()
    const siteItems = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
    expect(siteItems).toHaveLength(4)
    await siteItems[2]?.trigger('click')
    await nextTick()

    const input = document.body.querySelector<HTMLInputElement>('.desktop__rename-form input')
    expect(input).not.toBeNull()
    if (!input) throw new Error('rename input was not rendered')
    input.value = '我的博客'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await flushPromises()

    expect(mockedWorkspaceUpdate).toHaveBeenCalledWith(expect.objectContaining({
      labels: { 'site:blog': '我的博客' },
    }))
    expect(window.localStorage.getItem('kpanel:desktop-site-names:v1')).toBeNull()
    expect(wrapper.findAll('.desktop__icon-label').map((label) => label.text())).toContain('我的博客')
    wrapper.unmount()
  })

  it('prioritizes a local rename, then the website name, then the domain', async () => {
    mockedAppearance.mockResolvedValue({ name: 'Example Blog' })
    let wrapper = mount(DesktopView)
    await flushPromises()
    expect(wrapper.findAll('.desktop__icon-label').map((label) => label.text())).toContain('Example Blog')
    wrapper.unmount()

    window.localStorage.setItem(
      'kpanel:desktop-site-names:v1',
      JSON.stringify({ blog: 'My renamed blog' }),
    )
    wrapper = mount(DesktopView)
    await flushPromises()
    expect(wrapper.findAll('.desktop__icon-label').map((label) => label.text())).toContain(
      'My renamed blog',
    )
    wrapper.unmount()

    window.localStorage.clear()
    mockedAppearance.mockReset()
    mockedAppearance.mockRejectedValue(new Error('appearance unavailable'))
    wrapper = mount(DesktopView)
    await flushPromises()
    expect(wrapper.findAll('.desktop__icon-label').map((label) => label.text())).toContain(
      'blog.example.com',
    )
    wrapper.unmount()
  })

  it('launches a script-managed app directly into its management intent', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView)
    await nextTick()
    await nextTick()

    const icon = wrapper.find('button[title="OpenClaw"]')
    await icon.trigger('contextmenu', { clientX: 100, clientY: 100 })
    await nextTick()
    expect(wrapper.find('.desktop__context-menu [role="menuitem"]').text()).toContain('脚本管理')
    await icon.trigger('dblclick')
    await nextTick()

    expect(desktop.windows.value).toHaveLength(1)
    expect(desktop.windows.value[0]?.path).toBe('/app-script/openclaw')
    expect(desktop.windows.value[0]?.titleKey).toBe('desktop.scriptWindowTitle')
    expect(wrapper.find('.desktop-window__title').text()).toContain('OpenClaw 的脚本终端')
    expect(wrapper.find('.desktop-window__app-glyph img').attributes('src')).toBe('/api/v1/apps/openclaw/icon')
    expect(wrapper.find('.app-script-page__header').exists()).toBe(false)
    wrapper.unmount()
  })

  it('removes an installed app only from the desktop and restores it from the manager', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.find('button[title="Nginx"]').trigger('contextmenu', { clientX: 80, clientY: 80 })
    await nextTick()
    const remove = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('从桌面移除'))
    await remove?.trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('不会卸载应用')

    document.body.querySelector<HTMLButtonElement>('.modal-panel--compact .button--primary')?.click()
    await flushPromises()
    expect(mockedWorkspaceUpdate).toHaveBeenCalledWith(expect.objectContaining({
      hiddenEntryKeys: ['app:nginx'],
    }))
    expect(wrapper.find('button[title="Nginx"]').exists()).toBe(false)

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await nextTick()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('管理桌面图标'))
    await manage?.trigger('click')
    await nextTick()
    const restore = Array.from(document.body.querySelectorAll<HTMLButtonElement>('.desktop-icon-manager button'))
      .find((button) => button.textContent?.includes('恢复'))
    restore?.click()
    await flushPromises()
    expect(wrapper.find('button[title="Nginx"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders a persisted custom shortcut and deletes only that shortcut', async () => {
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: 'a'.repeat(32),
        name: '内部文档',
        description: '团队手册',
        url: 'https://docs.example.com/',
        iconVersion: 'b'.repeat(64),
        iconURL: `/api/v1/desktop/shortcuts/${'a'.repeat(32)}/icon?v=${'b'.repeat(64)}`,
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      }],
    }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    const shortcut = wrapper.find('button[title="内部文档"]')
    expect(shortcut.find('.desktop__icon-img').attributes('src')).toContain('/api/v1/desktop/shortcuts/')
    await shortcut.trigger('dblclick')
    await nextTick()
    expect(document.body.querySelector('.desktop__external-confirm')?.textContent).toContain('内部文档')
    document.body.querySelector<HTMLButtonElement>('.modal-panel__close')?.click()
    await nextTick()

    await shortcut.trigger('contextmenu', { clientX: 100, clientY: 100 })
    await nextTick()
    const remove = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('删除快捷方式'))
    await remove?.trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('不会访问或删除目标网站')
    const dialog = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
      .find((panel) => panel.textContent?.includes('确认删除快捷方式'))
    dialog?.querySelector<HTMLButtonElement>('.button--danger')?.click()
    await flushPromises()
    expect(mockedWorkspaceUpdate).toHaveBeenLastCalledWith(expect.objectContaining({ shortcuts: [] }))
    expect(wrapper.find('button[title="内部文档"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the icon manager mounted while adding a shortcut and restores it on close', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await nextTick()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('管理桌面图标'))
    await manage?.trigger('click')
    await flushPromises()

    const managerPanel = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
      .find((panel) => panel.textContent?.includes('管理桌面图标'))
    const addShortcut = Array.from(managerPanel?.querySelectorAll<HTMLButtonElement>('button') || [])
      .find((button) => button.textContent?.includes('添加快捷方式'))
    addShortcut?.focus()
    addShortcut?.click()
    await flushPromises()

    const openPanels = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
    expect(managerPanel?.isConnected).toBe(true)
    expect(openPanels).toHaveLength(2)
    expect(openPanels[0]?.textContent).toContain('管理桌面图标')
    expect(openPanels[1]?.textContent).toContain('添加桌面快捷方式')
    expect(document.body.querySelectorAll('.modal-backdrop')).toHaveLength(2)

    openPanels[1]?.querySelector<HTMLButtonElement>('.modal-panel__actions .icon-button')?.click()
    await nextTick()
    await nextTick()

    expect(document.body.querySelectorAll('.modal-panel')).toHaveLength(1)
    expect(managerPanel?.isConnected).toBe(true)
    expect(document.activeElement).toBe(addShortcut)
    wrapper.unmount()
  })

  it('reuses the created shortcut id when an icon upload is retried', async () => {
    mockedUploadShortcutIcon
      .mockRejectedValueOnce(new Error('upload failed'))
      .mockResolvedValueOnce({
        iconVersion: 'd'.repeat(64),
        iconURL: '/api/v1/desktop/shortcuts/retried/icon',
      })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const dialog = wrapper.findComponent(DesktopShortcutDialog)
    const draft = {
      id: '',
      name: '内部入口',
      description: '仅供测试',
      url: 'https://internal.example.com/',
    }
    const icon = new File([new Uint8Array([1, 2, 3])], 'icon.png', { type: 'image/png' })

    dialog.vm.$emit('save', draft, icon, false)
    await flushPromises()
    const firstID = mockedWorkspaceUpdate.mock.calls[0]?.[0].shortcuts[0]?.id
    expect(firstID).toMatch(/^[a-f0-9]{32}$/)
    expect(mockedUploadShortcutIcon).toHaveBeenLastCalledWith(firstID, icon)

    dialog.vm.$emit('save', draft, icon, false)
    await flushPromises()
    const retriedShortcuts = mockedWorkspaceUpdate.mock.calls[1]?.[0].shortcuts
    expect(retriedShortcuts).toHaveLength(1)
    expect(retriedShortcuts?.[0]?.id).toBe(firstID)
    expect(mockedUploadShortcutIcon.mock.calls.map(([id]) => id)).toEqual([firstID, firstID])
    wrapper.unmount()
  })
})

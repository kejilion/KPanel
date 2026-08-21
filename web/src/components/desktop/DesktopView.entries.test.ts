// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopView from '@/components/desktop/DesktopView.vue'
import DesktopShortcutDialog from '@/components/desktop/DesktopShortcutDialog.vue'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import type { DesktopEntries } from '@/lib/desktopEntries'
import {
  beginDesktopFileDrag,
  clearDesktopFileDrag,
  DESKTOP_FILE_DRAG_TYPE,
  NATIVE_FILE_DOWNLOAD_DRAG_TYPE,
} from '@/lib/desktopFileShortcuts'
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
      files: {
        ...actual.api.files,
        entry: vi.fn(),
        entries: vi.fn(),
        action: vi.fn(),
        archiveUrl: vi.fn(),
        createArchiveDownloadTicket: vi.fn(),
        createDownloadTicket: vi.fn(),
        upload: vi.fn(),
        transferFromPanel: vi.fn(),
      },
      cluster: {
        ...actual.api.cluster,
        hosts: vi.fn(),
      },
    },
  }
})

const mockedLoad = vi.mocked((await import('@/lib/desktopEntries')).loadDesktopEntries)
const mockedAppearance = vi.mocked(api.sites.appearance)
const mockedWorkspace = vi.mocked(api.desktop.workspace)
const mockedWorkspaceUpdate = vi.mocked(api.desktop.updateWorkspace)
const mockedUploadShortcutIcon = vi.mocked(api.desktop.uploadShortcutIcon)
const mockedFileEntry = vi.mocked(api.files.entry)
const mockedFileEntries = vi.mocked(api.files.entries)
const mockedFileAction = vi.mocked(api.files.action)
const mockedArchiveUrl = vi.mocked(api.files.archiveUrl)
const mockedCreateArchiveDownloadTicket = vi.mocked(api.files.createArchiveDownloadTicket)
const mockedCreateDownloadTicket = vi.mocked(api.files.createDownloadTicket)
const mockedFileUpload = vi.mocked(api.files.upload)
const mockedPanelTransfer = vi.mocked(api.files.transferFromPanel)
const mockedClusterHosts = vi.mocked(api.cluster.hosts)

function makeWorkspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
  return {
    schemaVersion: 3,
    resourceVersion: `sha256:${'1'.repeat(64)}`,
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    widgetPositions: {},
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

function internalFileDragEvent(type: string, dataTransfer: Record<string, unknown>, x = 120, y = 140): DragEvent {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperties(event, {
    dataTransfer: { value: dataTransfer },
    clientX: { value: x },
    clientY: { value: y },
  })
  return event as DragEvent
}

describe('DesktopView dynamic entries', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    resetDesktopIconsForTest()
    clearDesktopFileDrag()
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
      widgetPositions: body.widgetPositions,
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
    mockedPanelTransfer.mockReset()
    mockedFileEntry.mockReset()
    mockedFileEntry.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
    mockedFileEntries.mockReset()
    mockedFileEntries.mockResolvedValue({ entries: [], unavailable: [] })
    mockedClusterHosts.mockReset()
    mockedClusterHosts.mockResolvedValue({ nodeId: 'f'.repeat(32) } as Awaited<ReturnType<typeof api.cluster.hosts>>)
    mockedFileAction.mockReset()
    mockedFileAction.mockImplementation(async (input) => ({
      action: input.action,
      succeeded: [{ path: `${input.target}/${input.name}` }],
      failed: [],
    }))
    mockedArchiveUrl.mockReset()
    mockedArchiveUrl.mockImplementation((_entries, name) => `/api/v1/files/archive?name=${encodeURIComponent(name)}`)
    mockedCreateArchiveDownloadTicket.mockReset()
    mockedCreateArchiveDownloadTicket.mockResolvedValue({
      downloadUrl: '/api/v1/files/archive-download/test-ticket',
      expiresAt: '2026-08-20T08:00:00Z',
    })
    mockedCreateDownloadTicket.mockReset()
    mockedCreateDownloadTicket.mockResolvedValue({
      downloadUrl: '/api/v1/files/download/test-ticket',
      expiresAt: '2026-08-20T08:00:00Z',
    })
    mockedFileUpload.mockReset()
    mockedFileUpload.mockImplementation(async (path, file, _overwrite, onProgress) => {
      onProgress?.(45)
      onProgress?.(100)
      return {
        name: file.name,
        path: `${path}/${file.name}`,
        kind: 'file',
        sizeBytes: file.size,
        mode: '0644', owner: 'root', group: 'root',
        modifiedAt: '2026-08-14T00:00:00Z', resourceVersion: 'sha256:file',
        editable: true, previewable: true,
      }
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
      .find((item) => item.text().includes('桌面布局管理'))
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
        targetType: 'url',
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

  it('removes selected apps, sites, and shortcuts with one workspace update', async () => {
    const shortcutID = '9'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      positions: {
        'app:nginx': { x: 0.2, y: 0.1 },
        'site:blog': { x: 0.3, y: 0.2 },
        [`shortcut:${shortcutID}`]: { x: 0.4, y: 0.3 },
      },
      shortcuts: [{
        id: shortcutID,
        name: '内部文档',
        description: '',
        targetType: 'url',
        url: 'https://docs.example.com/',
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      }],
    }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.get('button[title="Nginx"]').trigger('click')
    await wrapper.get('button[title="blog.example.com"]').trigger('click', { ctrlKey: true })
    await wrapper.get('button[title="内部文档"]').trigger('click', { ctrlKey: true })
    const actions = wrapper.get('.desktop__selection-actions')
    expect(actions.text()).toContain('已选 3 项')
    await actions.findAll('button')[0]!.trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('文件和目录不会被删除')

    const dialog = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
      .find((panel) => panel.textContent?.includes('确认从桌面移除 3 项'))
    dialog?.querySelector<HTMLButtonElement>('.button--primary')?.click()
    await flushPromises()

    expect(mockedWorkspaceUpdate).toHaveBeenCalledTimes(1)
    expect(mockedWorkspaceUpdate).toHaveBeenCalledWith(expect.objectContaining({
      hiddenEntryKeys: expect.arrayContaining(['app:nginx', 'site:blog']),
      shortcuts: [],
      positions: expect.objectContaining({
        'app:nginx': { x: 0.2, y: 0.1 },
        'site:blog': { x: 0.3, y: 0.2 },
      }),
    }))
    expect(mockedWorkspaceUpdate.mock.calls[0]![0].positions).not.toHaveProperty(`shortcut:${shortcutID}`)
    expect(wrapper.find('.desktop__selection-actions').exists()).toBe(false)
    wrapper.unmount()
  })

  it('reuses an existing window for the same directory and keeps different directories independent', async () => {
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [
        {
          id: 'c'.repeat(32), name: 'nginx.conf', description: '', targetType: 'file', path: '/etc/nginx/nginx.conf',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
        {
          id: 'd'.repeat(32), name: '网站目录', description: '', targetType: 'directory', path: '/home/web',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
        {
          id: 'e'.repeat(32), name: 'mime.types', description: '', targetType: 'file', path: '/etc/nginx/mime.types',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
        {
          id: 'f'.repeat(32), name: 'Nginx 目录', description: '', targetType: 'directory', path: '/etc/nginx',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
      ],
    }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const desktop = useDesktopMode()

    expect(wrapper.get(`[data-icon-key="shortcut:${'d'.repeat(32)}"] svg`).classes()).toContain('lucide-folder-open')

    await wrapper.get(`[data-icon-key="shortcut:${'c'.repeat(32)}"] button`).trigger('dblclick')
    await wrapper.get(`[data-icon-key="shortcut:${'d'.repeat(32)}"] button`).trigger('dblclick')
    expect(desktop.windows.value.map((windowState) => windowState.path)).toEqual([
      '/files?path=%2Fetc%2Fnginx&file=%2Fetc%2Fnginx%2Fnginx.conf',
      '/files?path=%2Fhome%2Fweb',
    ])
    const firstWindowID = desktop.windows.value[0]!.id
    await wrapper.get(`[data-icon-key="shortcut:${'f'.repeat(32)}"] button`).trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(2)
    expect(desktop.focusedId.value).toBe(firstWindowID)

    await wrapper.get(`[data-icon-key="shortcut:${'e'.repeat(32)}"] button`).trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(2)
    expect(desktop.focusedId.value).toBe(firstWindowID)
    expect(desktop.windows.value[0]?.path).toBe(
      '/files?path=%2Fetc%2Fnginx&file=%2Fetc%2Fnginx%2Fmime.types',
    )

    await wrapper.get('button[title="文件"]').trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(3)
    expect(desktop.windows.value[2]?.path).toBe('/files')
    const rootWindowID = desktop.windows.value[2]!.id

    await wrapper.get('button[title="文件"]').trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(3)
    expect(desktop.focusedId.value).toBe(rootWindowID)
    wrapper.unmount()
  })

  it('creates a file shortcut at the desktop drop area without moving the source', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const values = new Map<string, string>()
    const types: string[] = []
    const dataTransfer = {
      types,
      effectAllowed: 'none',
      dropEffect: 'none',
      setData(type: string, value: string) {
        if (!types.includes(type)) types.push(type)
        values.set(type, value)
      },
      getData(type: string) {
        return values.get(type) || ''
      },
    }
    const start = internalFileDragEvent('dragstart', dataTransfer)
    expect(beginDesktopFileDrag(start, [{ name: 'nginx.conf', path: '/etc/nginx.conf', kind: 'file' }])).toBe(true)

    wrapper.element.dispatchEvent(internalFileDragEvent('dragover', dataTransfer))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.desktop__file-drop').exists()).toBe(true)
    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer))
    await flushPromises()

    const update = mockedWorkspaceUpdate.mock.calls.at(-1)?.[0]
    expect(update?.shortcuts).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'nginx.conf', targetType: 'file', path: '/etc/nginx.conf' }),
    ]))
    const shortcut = update?.shortcuts.find((item) => item.path === '/etc/nginx.conf')
    expect(shortcut && update?.positions[`shortcut:${shortcut.id}`]).toBeTruthy()
    expect(wrapper.find('.desktop__file-drop').exists()).toBe(false)
    wrapper.unmount()
  })

  it('downloads a desktop file shortcut from its context menu', async () => {
    const shortcutID = 'd'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: shortcutID, name: 'nginx.conf', description: '', targetType: 'file', path: '/home/nginx.conf',
        createdAt: '2026-08-20T00:00:00Z', updatedAt: '2026-08-20T00:00:00Z',
      }],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [{
        name: 'nginx.conf', path: '/home/nginx.conf', kind: 'file', resourceVersion: 'sha256:nginx',
        sizeBytes: 7, mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-20T00:00:00Z',
        editable: true, previewable: true,
      }],
      unavailable: [],
    })
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.get(`[data-icon-key="shortcut:${shortcutID}"] button`).trigger('contextmenu', {
      clientX: 120,
      clientY: 140,
    })
    await nextTick()
    const menuItems = wrapper.findAll('[role="menuitem"]')
    expect(menuItems.map((item) => item.text().trim())).toContain('下载')
    const download = menuItems.find((item) => item.text().trim() === '下载')
    await download!.trigger('click')
    await flushPromises()

    expect(mockedCreateDownloadTicket).toHaveBeenCalledWith('/home/nginx.conf')
    expect(mockedCreateArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mockedArchiveUrl).not.toHaveBeenCalled()
    expect(anchorClick).toHaveBeenCalledOnce()
    anchorClick.mockRestore()
    wrapper.unmount()
  })

  it('downloads a desktop directory through one short-lived archive ticket', async () => {
    const shortcutID = 'a'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: shortcutID, name: 'logs', description: '', targetType: 'directory', path: '/home/logs',
        createdAt: '2026-08-20T00:00:00Z', updatedAt: '2026-08-20T00:00:00Z',
      }],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [{
        name: 'logs', path: '/home/logs', kind: 'directory', resourceVersion: 'sha256:logs',
        sizeBytes: 0, mode: '0755', owner: 'root', group: 'root', modifiedAt: '2026-08-20T00:00:00Z',
        editable: false, previewable: false,
      }],
      unavailable: [],
    })
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.get(`[data-icon-key="shortcut:${shortcutID}"] button`).trigger('contextmenu', {
      clientX: 120,
      clientY: 140,
    })
    await nextTick()
    const download = wrapper.findAll('[role="menuitem"]')
      .find((item) => item.text().trim() === '下载 ZIP')
    await download!.trigger('click')
    await flushPromises()

    expect(mockedCreateArchiveDownloadTicket).toHaveBeenCalledWith([
      expect.objectContaining({ path: '/home/logs', resourceVersion: 'sha256:logs' }),
    ], 'logs.zip')
    expect(mockedCreateDownloadTicket).not.toHaveBeenCalled()
    expect(mockedArchiveUrl).not.toHaveBeenCalled()
    expect(anchorClick).toHaveBeenCalledOnce()
    anchorClick.mockRestore()
    wrapper.unmount()
  })

  it('does not silently download only part of a mixed desktop selection', async () => {
    const shortcutID = 'e'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: shortcutID, name: 'nginx.conf', description: '', targetType: 'file', path: '/home/nginx.conf',
        createdAt: '2026-08-20T00:00:00Z', updatedAt: '2026-08-20T00:00:00Z',
      }],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [{
        name: 'nginx.conf', path: '/home/nginx.conf', kind: 'file', resourceVersion: 'sha256:nginx',
        sizeBytes: 7, mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-20T00:00:00Z',
        editable: true, previewable: true,
      }],
      unavailable: [],
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    const shortcut = wrapper.get(`[data-icon-key="shortcut:${shortcutID}"] button`)
    await shortcut.trigger('click')
    await wrapper.get('button[title="Nginx"]').trigger('click', { ctrlKey: true })
    await shortcut.trigger('contextmenu', { clientX: 120, clientY: 140 })
    await nextTick()

    const labels = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .map((item) => item.text().trim())
    expect(labels).not.toContain('下载')
    expect(labels).not.toContain('下载 ZIP')
    expect(mockedCreateDownloadTicket).not.toHaveBeenCalled()
    expect(mockedCreateArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mockedArchiveUrl).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps DownloadURL, cross-panel transfer, and local movement on one desktop drag', async () => {
    const firstID = '1'.repeat(32)
    const secondID = '2'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [
        {
          id: firstID, name: 'one.txt', description: '', targetType: 'file', path: '/home/one.txt',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
        {
          id: secondID, name: 'app', description: '', targetType: 'directory', path: '/home/app',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
      ],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [
        {
          name: 'one.txt', path: '/home/one.txt', kind: 'file', resourceVersion: 'sha256:one',
          sizeBytes: 3, mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-15T00:00:00Z',
          editable: true, previewable: true,
        },
        {
          name: 'app', path: '/home/app', kind: 'directory', resourceVersion: 'sha256:app',
          sizeBytes: 0, mode: '0755', owner: 'root', group: 'root', modifiedAt: '2026-08-15T00:00:00Z',
          editable: false, previewable: false,
        },
      ],
      unavailable: [],
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const first = wrapper.get(`[data-icon-key="shortcut:${firstID}"]`)
    const second = wrapper.get(`[data-icon-key="shortcut:${secondID}"]`)
    expect(first.attributes('draggable')).toBe('true')
    expect(second.classes()).toContain('desktop__icon-slot--transfer-ready')
    expect(wrapper.get('[data-icon-key="app:nginx"]').attributes('draggable')).toBeUndefined()

    await first.get('button').trigger('click')
    await second.get('button').trigger('click', { ctrlKey: true })
    const values = new Map<string, string>()
    const types: string[] = []
    const dataTransfer = {
      types, effectAllowed: 'none', dropEffect: 'none',
      setDragImage: vi.fn(),
      setData(type: string, value: string) {
        if (!types.includes(type)) types.push(type)
        values.set(type, value)
      },
      getData(type: string) { return values.get(type) || '' },
    }
    first.element.dispatchEvent(internalFileDragEvent('dragstart', dataTransfer, 40, 40))
    expect(dataTransfer.setDragImage).toHaveBeenCalledWith(first.element, 40, 40)
    const payload = JSON.parse(values.get('application/x-kpanel-cross-panel-files-v2') || '{}')
    expect(payload).toMatchObject({
      version: 2,
      sourceNodeId: 'f'.repeat(32),
      entries: [
        { path: '/home/one.txt', resourceVersion: 'sha256:one' },
        { path: '/home/app', resourceVersion: 'sha256:app' },
      ],
    })
    expect(types).toEqual(expect.arrayContaining([
      DESKTOP_FILE_DRAG_TYPE,
      'application/x-kpanel-cross-panel-files-v2',
      NATIVE_FILE_DOWNLOAD_DRAG_TYPE,
    ]))
    expect(mockedArchiveUrl).toHaveBeenCalledWith(expect.arrayContaining([
      expect.objectContaining({ path: '/home/one.txt' }),
      expect.objectContaining({ path: '/home/app' }),
    ]), 'KPanel Desktop.zip')
    expect(mockedCreateArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(values.get(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toMatch(
      /^application\/zip:KPanel Desktop\.zip:https?:\/\//,
    )
    expect(values.get(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toContain(
      '/api/v1/files/archive?name=KPanel%20Desktop.zip',
    )

    const protectedHoverDataTransfer = {
      ...dataTransfer,
      getData: () => '',
    }
    wrapper.element.dispatchEvent(internalFileDragEvent('dragover', protectedHoverDataTransfer, 360, 260))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.desktop__file-drop').exists()).toBe(false)
    expect(protectedHoverDataTransfer.dropEffect).toBe('move')
    expect(first.attributes('style')).toContain('left: 320px')
    expect(first.attributes('style')).toContain('top: 220px')
    expect(second.classes()).toContain('desktop__icon-slot--dragging')
    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer, 360, 260))
    await wrapper.vm.$nextTick()
    expect(first.classes()).not.toContain('desktop__icon-slot--dragging')
    expect(second.classes()).not.toContain('desktop__icon-slot--dragging')
    first.element.dispatchEvent(internalFileDragEvent('dragend', dataTransfer, 360, 260))
    await flushPromises()

    const positions = mockedWorkspaceUpdate.mock.calls.at(-1)?.[0].positions
    expect(positions?.[`shortcut:${firstID}`]).toBeDefined()
    expect(positions?.[`shortcut:${secondID}`]).toBeDefined()
    expect(mockedPanelTransfer).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps local desktop shortcut movement available when cluster identity is unavailable', async () => {
    const shortcutID = '3'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: shortcutID, name: 'fast.txt', description: '', targetType: 'file', path: '/home/fast.txt',
        createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
      }],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [{
        name: 'fast.txt', path: '/home/fast.txt', kind: 'file', mime: 'text/plain', resourceVersion: 'sha256:fast',
        sizeBytes: 3, mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-15T00:00:00Z',
        editable: true, previewable: true,
      }],
      unavailable: [],
    })
    mockedClusterHosts.mockRejectedValue(new Error('cluster identity unavailable'))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const shortcut = wrapper.get(`[data-icon-key="shortcut:${shortcutID}"]`)
    const values = new Map<string, string>()
    const types: string[] = []
    const dataTransfer = {
      types, effectAllowed: 'none', dropEffect: 'none', setDragImage: vi.fn(),
      setData(type: string, value: string) {
        if (!types.includes(type)) types.push(type)
        values.set(type, value)
      },
      getData(type: string) { return values.get(type) || '' },
    }
    const identityAttemptsBeforeDrag = mockedClusterHosts.mock.calls.length
    shortcut.element.dispatchEvent(internalFileDragEvent('dragstart', dataTransfer, 40, 40))
    expect(types).toContain(DESKTOP_FILE_DRAG_TYPE)
    expect(types).not.toContain('application/x-kpanel-cross-panel-files-v2')
    expect(types).toContain(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)
    expect(values.get(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toMatch(
      /^text\/plain:fast\.txt:https?:\/\/[^/]+\/api\/v1\/files\/content\?path=%2Fhome%2Ffast\.txt&disposition=attachment$/,
    )
    expect(mockedClusterHosts.mock.calls.length).toBeGreaterThan(identityAttemptsBeforeDrag)
    Object.defineProperty(document, 'elementFromPoint', {
      configurable: true,
      value: vi.fn(() => wrapper.element),
    })
    shortcut.element.dispatchEvent(internalFileDragEvent('dragend', dataTransfer, 360, 260))
    await flushPromises()

    expect(mockedWorkspaceUpdate.mock.calls.at(-1)?.[0].positions[`shortcut:${shortcutID}`]).toBeDefined()
    expect(mockedPanelTransfer).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not turn cross-panel shortcut drags into local moves on dragend', async () => {
    const shortcutID = '4'.repeat(32)
    mockedWorkspace.mockResolvedValueOnce(makeWorkspace({
      shortcuts: [{
        id: shortcutID, name: 'remote.txt', description: '', targetType: 'file', path: '/home/remote.txt',
        createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
      }],
    }))
    mockedFileEntries.mockResolvedValue({
      entries: [{
        name: 'remote.txt', path: '/home/remote.txt', kind: 'file', resourceVersion: 'sha256:remote',
        sizeBytes: 3, mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-15T00:00:00Z',
        editable: true, previewable: true,
      }],
      unavailable: [],
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const shortcut = wrapper.get(`[data-icon-key="shortcut:${shortcutID}"]`)
    const values = new Map<string, string>()
    const types: string[] = []
    const dataTransfer = {
      types, effectAllowed: 'none', dropEffect: 'none', setDragImage: vi.fn(),
      setData(type: string, value: string) {
        if (!types.includes(type)) types.push(type)
        values.set(type, value)
      },
      getData(type: string) { return values.get(type) || '' },
    }
    shortcut.element.dispatchEvent(internalFileDragEvent('dragstart', dataTransfer, 40, 40))
    dataTransfer.dropEffect = 'copy'
    Object.defineProperty(document, 'elementFromPoint', {
      configurable: true,
      value: vi.fn(() => wrapper.element),
    })
    shortcut.element.dispatchEvent(internalFileDragEvent('dragend', dataTransfer, 360, 260))
    await flushPromises()

    expect(mockedWorkspaceUpdate).not.toHaveBeenCalled()

    dataTransfer.dropEffect = 'none'
    shortcut.element.dispatchEvent(internalFileDragEvent('dragstart', dataTransfer, 40, 40))
    wrapper.element.dispatchEvent(internalFileDragEvent('dragleave', dataTransfer, 360, 260))
    await nextTick()
    expect(shortcut.classes()).toContain('desktop__icon-slot--native-drag-hidden')

    wrapper.element.dispatchEvent(internalFileDragEvent('dragover', dataTransfer, 360, 260))
    await nextTick()
    expect(shortcut.classes()).not.toContain('desktop__icon-slot--native-drag-hidden')

    wrapper.element.dispatchEvent(internalFileDragEvent('dragleave', dataTransfer, 360, 260))
    dataTransfer.dropEffect = 'none'
    shortcut.element.dispatchEvent(internalFileDragEvent('dragend', dataTransfer, 360, 260))
    await flushPromises()

    expect(mockedWorkspaceUpdate).not.toHaveBeenCalled()
    expect(shortcut.classes()).not.toContain('desktop__icon-slot--native-drag-hidden')
    wrapper.unmount()
  })

  it('uploads an external operating-system file and creates a real server-file desktop entry', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    const dataTransfer = {
      types: ['Files'],
      items: [],
      files: [file],
      dropEffect: 'none',
    }

    wrapper.element.dispatchEvent(internalFileDragEvent('dragover', dataTransfer))
    await nextTick()
    expect(wrapper.get('.desktop__file-drop').text()).toContain('上传到 KPanel 桌面')

    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer, 190, 160))
    await flushPromises()

    expect(mockedFileAction).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'mkdir', target: '/home', name: 'KPanel Desktop' }),
      expect.any(AbortSignal),
    )
    expect(mockedFileUpload).toHaveBeenCalledWith(
      '/home/KPanel Desktop', file, false, expect.any(Function), expect.any(AbortSignal),
    )
    const update = mockedWorkspaceUpdate.mock.calls.at(-1)?.[0]
    expect(update?.shortcuts).toEqual(expect.arrayContaining([
      expect.objectContaining({
        name: 'notes.txt', targetType: 'file', path: '/home/KPanel Desktop/notes.txt',
      }),
    ]))
    expect(wrapper.get('.desktop-transfer').text()).toContain('已传到桌面')
    wrapper.unmount()
  })

  it('copies a cross-panel directory and creates its desktop entry after commit', async () => {
    const copied = {
      name: 'app', path: '/home/KPanel Desktop/app', kind: 'directory' as const,
      sizeBytes: 0, mode: '0755', owner: 'root', group: 'root',
      modifiedAt: '2026-08-15T00:00:00Z', resourceVersion: 'sha256:target',
      editable: false, previewable: false,
    }
    mockedPanelTransfer.mockImplementation(async (_input, onEvent) => {
      onEvent({ state: 'connecting' })
      onEvent({ state: 'transferring', loadedBytes: 1024, totalBytes: 2048 })
      onEvent({ state: 'committing', loadedBytes: 2048, totalBytes: 2048 })
      onEvent({ state: 'complete', loadedBytes: 2048, totalBytes: 2048, entry: copied })
      return copied
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const payload = JSON.stringify({
      version: 1, sourceNodeId: 'a'.repeat(32), name: 'app', path: '/app',
      kind: 'directory', resourceVersion: 'sha256:source',
    })
    const dataTransfer = {
      types: ['application/x-kpanel-cross-panel-file-v1'], dropEffect: 'none',
      getData: (type: string) => type === 'application/x-kpanel-cross-panel-file-v1' ? payload : '',
    }

    wrapper.element.dispatchEvent(internalFileDragEvent('dragover', dataTransfer))
    await nextTick()
    expect(wrapper.get('.desktop__file-drop').text()).toContain('从另一个 KPanel 复制')
    expect(wrapper.get('.desktop__file-drop').text()).toContain('/home/KPanel Desktop')

    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer, 190, 160))
    await flushPromises()
    expect(mockedPanelTransfer).toHaveBeenCalledWith({
      sourceNodeId: 'a'.repeat(32), path: '/app', resourceVersion: 'sha256:source',
      targetDirectory: '/home/KPanel Desktop',
    }, expect.any(Function), expect.any(AbortSignal))
    expect(mockedWorkspaceUpdate.mock.calls.at(-1)?.[0].shortcuts).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'app', targetType: 'directory', path: '/home/KPanel Desktop/app' }),
    ]))
    expect(wrapper.get('.desktop-transfer').text()).toContain('跨面板复制完成')
    wrapper.unmount()
  })

  it('copies a cross-panel multi-selection and creates all committed desktop entries together', async () => {
    mockedPanelTransfer.mockImplementation(async (input, onEvent) => {
      onEvent({ state: 'connecting' })
      onEvent({ state: 'complete' })
      const name = input.path.slice(input.path.lastIndexOf('/') + 1)
      return {
        name, path: `/home/KPanel Desktop/${name}`, kind: 'file' as const,
        sizeBytes: 4, mode: '0644', owner: 'root', group: 'root',
        modifiedAt: '2026-08-15T00:00:00Z', resourceVersion: `sha256:target-${name}`,
        editable: true, previewable: true,
      }
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const payload = JSON.stringify({
      version: 2,
      sourceNodeId: 'b'.repeat(32),
      entries: [
        { name: 'one.txt', path: '/one.txt', kind: 'file', resourceVersion: 'sha256:one' },
        { name: 'two.txt', path: '/two.txt', kind: 'file', resourceVersion: 'sha256:two' },
      ],
    })
    const dataTransfer = {
      types: ['application/x-kpanel-cross-panel-files-v2'], dropEffect: 'none',
      getData: (type: string) => type === 'application/x-kpanel-cross-panel-files-v2' ? payload : '',
    }

    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer, 190, 160))
    await flushPromises()

    expect(mockedPanelTransfer).toHaveBeenCalledTimes(2)
    expect(mockedWorkspaceUpdate.mock.calls.at(-1)?.[0].shortcuts).toEqual(expect.arrayContaining([
      expect.objectContaining({ name: 'one.txt', path: '/home/KPanel Desktop/one.txt' }),
      expect.objectContaining({ name: 'two.txt', path: '/home/KPanel Desktop/two.txt' }),
    ]))
    expect(wrapper.get('.desktop-transfer').text()).toContain('跨面板复制完成')
    expect(wrapper.get('.desktop-transfer').text()).toContain('2 个项目')
    wrapper.unmount()
  })

  it('uses a remembered existing host directory for later desktop uploads', async () => {
    window.localStorage.setItem('kpanel:desktop-upload-location:v1', '/srv/uploads')
    mockedFileEntry.mockImplementation(async (path) => {
      if (path === '/srv/uploads') {
        return {
          name: 'uploads', path, kind: 'directory', sizeBytes: 0, mode: '0755',
          owner: 'root', group: 'root', modifiedAt: '2026-08-14T00:00:00Z',
          resourceVersion: 'sha256:directory', editable: false, previewable: false,
        }
      }
      throw Object.assign(new Error('not found'), { status: 404 })
    })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const file = new File(['content'], 'custom.txt', { type: 'text/plain' })
    const dataTransfer = { types: ['Files'], items: [], files: [file], dropEffect: 'none' }

    wrapper.element.dispatchEvent(internalFileDragEvent('drop', dataTransfer, 190, 160))
    await flushPromises()

    expect(mockedFileUpload).toHaveBeenCalledWith(
      '/srv/uploads', file, false, expect.any(Function), expect.any(AbortSignal),
    )
    expect(mockedFileAction).not.toHaveBeenCalled()
    expect(mockedWorkspaceUpdate.mock.calls.at(-1)?.[0].shortcuts).toEqual(expect.arrayContaining([
      expect.objectContaining({ path: '/srv/uploads/custom.txt', targetType: 'file' }),
    ]))
    wrapper.unmount()
  })

  it('keeps the icon manager mounted while adding a shortcut and restores it on close', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await nextTick()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('桌面布局管理'))
    await manage?.trigger('click')
    await flushPromises()

    const managerPanel = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
      .find((panel) => panel.textContent?.includes('桌面布局管理'))
    const addShortcut = Array.from(managerPanel?.querySelectorAll<HTMLButtonElement>('button') || [])
      .find((button) => button.textContent?.includes('添加快捷方式'))
    addShortcut?.focus()
    addShortcut?.click()
    await flushPromises()

    const openPanels = Array.from(document.body.querySelectorAll<HTMLElement>('.modal-panel'))
    expect(managerPanel?.isConnected).toBe(true)
    expect(openPanels).toHaveLength(2)
    expect(openPanels[0]?.textContent).toContain('桌面布局管理')
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

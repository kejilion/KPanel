// @vitest-environment jsdom
import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  remoteDownload: vi.fn(),
  createRemoteDownloadJob: vi.fn(),
  remoteDownloadJobs: vi.fn(),
  remoteDownloadJob: vi.fn(),
  cancelRemoteDownloadJob: vi.fn(),
  deleteRemoteDownloadJob: vi.fn(),
  hosts: vi.fn(),
  route: { query: {} as Record<string, unknown> },
  push: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status = 0
    readonly code = 'request_failed'
  },
  api: {
    files: {
      list: mocks.list,
      remoteDownload: mocks.remoteDownload,
      createRemoteDownloadJob: mocks.createRemoteDownloadJob,
      remoteDownloadJobs: mocks.remoteDownloadJobs,
      remoteDownloadJob: mocks.remoteDownloadJob,
      cancelRemoteDownloadJob: mocks.cancelRemoteDownloadJob,
      deleteRemoteDownloadJob: mocks.deleteRemoteDownloadJob,
      entry: vi.fn(),
      text: vi.fn(),
      write: vi.fn(),
      action: vi.fn(),
      transferFromPanel: vi.fn(),
      trash: vi.fn(),
      upload: vi.fn(),
      contentUrl: vi.fn(() => ''),
      archiveUrl: vi.fn(() => ''),
      createDownloadTicket: vi.fn(),
      createArchiveDownloadTicket: vi.fn(),
      thumbnailUrl: vi.fn(() => ''),
    },
    cluster: { hosts: mocks.hosts },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: vi.fn(), danger: vi.fn(), show: vi.fn() }),
}))

function directory(path: string) {
  return {
    path,
    entries: [],
    offset: 0,
    total: 0,
    totalKnown: true,
    truncated: false,
    scanTruncated: false,
    readAt: '2026-08-22T00:00:00Z',
  }
}

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    x: left,
    y: top,
    left,
    top,
    right: left + width,
    bottom: top + height,
    width,
    height,
    toJSON: () => ({}),
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.route.query = {}
  mocks.list.mockResolvedValue(directory('/'))
  mocks.remoteDownloadJobs.mockResolvedValue({ items: [] })
  mocks.hosts.mockResolvedValue({ nodeId: 'local-node', items: [] })
})

function fileHost(id: string, isLocal: boolean, overrides: Record<string, unknown> = {}) {
  return {
    id,
    isLocal,
    kind: 'panel',
    name: isLocal ? '当前 KPanel' : 'edge-01',
    origin: isLocal ? 'https://center.example.com' : 'https://edge.example.com',
    transportSecurity: 'tls',
    remoteNodeId: isLocal ? 'local-node' : 'edge-node',
    federationProtocol: 'v2',
    scope: 'cluster.summary.read cluster.terminal.open cluster.files.read',
    terminalAvailable: true,
    fileTransferAvailable: !isLocal,
    mutualFileTransferAvailable: !isLocal,
    state: 'online',
    consecutiveFailures: 0,
    polling: false,
    resourceVersion: `${id}-version`,
    createdAt: '2026-08-22T00:00:00Z',
    updatedAt: '2026-08-22T00:00:00Z',
    ...overrides,
  }
}

describe('FilesView host switcher', () => {
  it('keeps the path toolbar and opens a capable remote host in its existing Files page', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('edge', false)],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      const trigger = wrapper.get('.file-host-switcher__trigger')
      expect(trigger.text()).toContain('当前主机')
      expect(trigger.text()).toContain('本机')
      expect(wrapper.get('.breadcrumbs').text()).toContain('根目录')

      await trigger.trigger('click')
      expect(wrapper.get('#file-host-switcher-menu').text()).toContain('edge-01')
      expect(wrapper.get('[data-file-host-id="local"]').attributes('aria-current')).toBe('true')

      await wrapper.get('[data-file-host-id="edge"]').trigger('click')
      expect(openSpy).toHaveBeenCalledWith('https://edge.example.com/files', '_blank', 'noopener,noreferrer')
      expect(wrapper.find('#file-host-switcher-menu').exists()).toBe(false)

      await trigger.trigger('click')
      const triggerElement = trigger.element as HTMLButtonElement
      triggerElement.focus()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await nextTick()
      expect(wrapper.find('#file-host-switcher-menu').exists()).toBe(false)
      expect(document.activeElement).toBe(triggerElement)
    } finally {
      wrapper.unmount()
      openSpy.mockRestore()
    }
  })

  it('reuses the remote Panel security entrance before appending the Files path', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('secure', false, {
        name: 'secure-01',
        origin: 'https://edge.example.com:8443',
        securityEntrancePath: 'panel-secure1',
      })],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      await wrapper.get('.file-host-switcher__trigger').trigger('click')
      await wrapper.get('[data-file-host-id="secure"]').trigger('click')
      expect(openSpy).toHaveBeenCalledWith(
        'https://edge.example.com:8443/panel-secure1/files',
        '_blank',
        'noopener,noreferrer',
      )
    } finally {
      wrapper.unmount()
      openSpy.mockRestore()
    }
  })

  it('requires the existing HTTP warning before opening an e2e_http Files page', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('http', false, {
        name: 'http-01',
        origin: 'http://edge.example.com:8080',
        transportSecurity: 'e2e_http',
        securityEntrancePath: 'panel-secure1',
      })],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      await wrapper.get('.file-host-switcher__trigger').trigger('click')
      await wrapper.get('[data-file-host-id="http"]').trigger('click')
      expect(confirmSpy).toHaveBeenCalledWith(expect.stringMatching(/HTTP|Session/))
      expect(openSpy).toHaveBeenCalledWith(
        'http://edge.example.com:8080/panel-secure1/files',
        '_blank',
        'noopener,noreferrer',
      )
    } finally {
      wrapper.unmount()
      openSpy.mockRestore()
      confirmSpy.mockRestore()
    }
  })

  it('does not open an e2e_http Files page after the warning is cancelled', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('http', false, {
        name: 'http-01',
        origin: 'http://edge.example.com:8080',
        transportSecurity: 'e2e_http',
        securityEntrancePath: 'panel-secure1',
      })],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      await wrapper.get('.file-host-switcher__trigger').trigger('click')
      await wrapper.get('[data-file-host-id="http"]').trigger('click')
      expect(confirmSpy).toHaveBeenCalledOnce()
      expect(openSpy).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
      openSpy.mockRestore()
      confirmSpy.mockRestore()
    }
  })

  it('opens a paired Panel file page even without cross-panel transfer capability', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('pending', false, {
        name: 'pending-01',
        fileTransferAvailable: false,
        mutualFileTransferAvailable: false,
      })],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      await wrapper.get('.file-host-switcher__trigger').trigger('click')
      expect(wrapper.get('[data-file-host-id="pending"]').text()).toContain('打开远端文件管理')
      await wrapper.get('[data-file-host-id="pending"]').trigger('click')
      expect(openSpy).toHaveBeenCalledWith('https://edge.example.com/files', '_blank', 'noopener,noreferrer')
      expect(mocks.push).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
      openSpy.mockRestore()
    }
  })

  it('keeps telemetry-only light nodes in the existing cluster management page', async () => {
    mocks.hosts.mockResolvedValue({
      nodeId: 'local-node',
      items: [fileHost('local', true), fileHost('light', false, {
        name: 'light-01',
        kind: 'light_node',
        origin: '',
        federationProtocol: 'light-v1',
        scope: 'cluster.summary.read',
        fileTransferAvailable: false,
        mutualFileTransferAvailable: false,
      })],
      total: 2,
      remoteTotal: 1,
      maxHosts: 100,
      pollIntervalSeconds: 30,
    })
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })

    try {
      await flushPromises()
      await wrapper.get('.file-host-switcher__trigger').trigger('click')
      expect(wrapper.get('[data-file-host-id="light"]').text()).toContain('文件互传未启用')
      await wrapper.get('[data-file-host-id="light"]').trigger('click')
      expect(mocks.push).toHaveBeenCalledWith({ name: 'cluster' })
    } finally {
      wrapper.unmount()
    }
  })
})

describe('FilesView remote download lifecycle', () => {
  it('backs off a failed active-task poll for ten seconds', async () => {
    vi.useFakeTimers()
    const activeJob = {
      id: 'a'.repeat(32),
      state: 'transferring',
      source: 'https://downloads.example.com',
      targetDirectory: '/',
      name: 'file.bin',
      loadedBytes: 1024,
      totalBytes: 4096,
      createdAt: '2026-08-23T00:00:00Z',
      updatedAt: '2026-08-23T00:00:01Z',
    }
    mocks.remoteDownloadJobs
      .mockResolvedValueOnce({ items: [activeJob] })
      .mockRejectedValueOnce(new Error('poll failed'))
      .mockResolvedValueOnce({ items: [activeJob] })
    const wrapper = shallowMount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })
    try {
      await flushPromises()
      expect(mocks.remoteDownloadJobs).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync(2_500)
      await flushPromises()
      expect(mocks.remoteDownloadJobs).toHaveBeenCalledTimes(2)

      await vi.advanceTimersByTimeAsync(2_500)
      await flushPromises()
      expect(mocks.remoteDownloadJobs).toHaveBeenCalledTimes(2)

      await vi.advanceTimersByTimeAsync(7_500)
      await flushPromises()
      expect(mocks.remoteDownloadJobs).toHaveBeenCalledTimes(3)
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it('restores persisted tasks on mount and does not cancel an active server task on unmount', async () => {
    mocks.remoteDownloadJobs.mockResolvedValueOnce({
      items: [{
        id: 'a'.repeat(32),
        state: 'transferring',
        source: 'https://downloads.example.com',
        targetDirectory: '/',
        name: 'file.bin',
        loadedBytes: 1024,
        totalBytes: 4096,
        createdAt: '2026-08-23T00:00:00Z',
        updatedAt: '2026-08-23T00:00:01Z',
      }],
    })
    const wrapper = shallowMount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })
    await flushPromises()

    expect(mocks.remoteDownloadJobs).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('file.bin')
    expect(wrapper.text()).toContain('关闭页面后仍会继续')
    const phase = wrapper.get('.remote-download-task__phase')
    expect(phase.attributes()).toMatchObject({
      role: 'status', 'aria-live': 'polite', 'aria-atomic': 'true',
    })
    expect(phase.text()).toBe('正在接收远程文件')
    expect(phase.text()).not.toContain('已接收')
    const bytes = wrapper.get('.remote-download-task__bytes')
    expect(bytes.text()).toContain('已接收')
    expect(bytes.attributes('role')).toBeUndefined()
    expect(bytes.attributes('aria-live')).toBeUndefined()

    wrapper.unmount()
    await flushPromises()

    expect(mocks.cancelRemoteDownloadJob).not.toHaveBeenCalled()
  })

  it('aborts only an in-flight task-list read when the page unmounts', async () => {
    let jobsSignal: AbortSignal | undefined
    mocks.remoteDownloadJobs.mockImplementationOnce((signal?: AbortSignal) => new Promise((_resolve, reject) => {
      jobsSignal = signal
      signal?.addEventListener(
        'abort',
        () => reject(new DOMException('Aborted', 'AbortError')),
        { once: true },
      )
    }))
    const wrapper = shallowMount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })
    await vi.waitFor(() => expect(jobsSignal).toBeDefined())

    wrapper.unmount()
    await flushPromises()

    expect(jobsSignal?.aborted).toBe(true)
    expect(mocks.cancelRemoteDownloadJob).not.toHaveBeenCalled()
  })
})

describe('FilesView context menu', () => {
  it('keeps the full file menu inside its desktop window and above the taskbar', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
    mocks.list.mockResolvedValue({
      ...directory('/'),
      total: 1,
      entries: [{
        name: 'nginx.conf',
        path: '/nginx.conf',
        kind: 'file',
        mode: '-rw-r--r--',
        owner: 'root',
        group: 'root',
        sizeBytes: 1024,
        modifiedAt: '2026-08-23T00:00:00Z',
        resourceVersion: 'sha256:file',
        mimeType: 'text/plain',
      }],
    })
    const desktop = document.createElement('div')
    desktop.className = 'desktop'
    const windowBody = document.createElement('div')
    windowBody.className = 'desktop-window__body'
    const taskbar = document.createElement('div')
    taskbar.className = 'desktop__taskbar'
    desktop.append(windowBody, taskbar)
    document.body.append(desktop)
    const wrapper = mount(FilesView, {
      attachTo: windowBody,
      global: {
        stubs: {
          ModalDialog: {
            props: ['open'],
            template: '<div v-if="open"><slot /></div>',
          },
        },
      },
    })
    await flushPromises()
    const originalBounds = HTMLElement.prototype.getBoundingClientRect
    const boundsSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this === desktop) return rect(0, 0, 1280, 800)
      if (this === windowBody) return rect(100, 80, 1080, 660)
      if (this === taskbar) return rect(8, 728, 1264, 56)
      if (this.matches('.file-context-menu')) return rect(0, 0, 196, 408)
      return originalBounds.call(this)
    })

    try {
      await wrapper.get('.file-row--entry').trigger('contextmenu', { button: 2, clientX: 1200, clientY: 760 })
      await flushPromises()

      const menu = document.body.querySelector<HTMLElement>('.file-context-menu')!
      const items = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')]
      expect(menu).not.toBeNull()
      expect(menu.style.left).toBe('976px')
      expect(menu.style.top).toBe('312px')
      expect(Number.parseFloat(menu.style.top) + 408).toBeLessThanOrEqual(720)
      expect(menu.style.getPropertyValue('--context-menu-max-height')).toBe('632px')
      expect(document.activeElement).toBe(items[0])
      expect(menu.dataset.contextMenuFocus).toBe('pointer')
      expect(items[0]?.hasAttribute('aria-selected')).toBe(false)

      menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }))
      expect(menu.dataset.contextMenuFocus).toBe('keyboard')
      expect(document.activeElement).toBe(items[1])
    } finally {
      wrapper.unmount()
      desktop.remove()
      boundsSpy.mockRestore()
    }
  })
})

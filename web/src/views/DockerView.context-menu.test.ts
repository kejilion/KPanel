// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import type { DockerInventory } from '@/types/api'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  backups: vi.fn(),
  environment: vi.fn(),
  jobs: vi.fn(),
  publicNetwork: vi.fn(),
  checkUpdate: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number

    constructor(message: string, status = 0) {
      super(message)
      this.status = status
    }
  },
  api: {
    docker: {
      checkUpdate: mocks.checkUpdate,
      inventory: mocks.inventory,
      backups: mocks.backups,
      environment: mocks.environment,
      jobs: mocks.jobs,
      job: vi.fn(),
      action: vi.fn(),
      composeProject: vi.fn(),
      exec: vi.fn(),
      logs: vi.fn(),
      stats: vi.fn(),
      task: vi.fn(),
    },
    system: { publicNetwork: mocks.publicNetwork, action: vi.fn() },
  },
}))

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

function inventory(): DockerInventory {
  return {
    available: true,
    version: '28.0.0',
    observedAt: '2026-08-23T00:00:00Z',
    containers: [{
      id: 'c'.repeat(64),
      name: 'web',
      image: 'nginx:alpine',
      state: 'running',
      access: 'managed',
      consistency: 'synced',
      ports: [],
      networks: ['bridge'],
      mounts: [],
      resourceVersion: 'sha256:container',
      allowedActions: ['logs', 'stats', 'exec', 'access', 'restart', 'pause', 'stop', 'remove'],
    }],
    images: [],
    networks: [],
    volumes: [],
  }
}

describe('Docker context menu', () => {
  let wrapper: VueWrapper
  let desktop: HTMLElement
  let windowBody: HTMLElement
  let taskbar: HTMLElement

  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
    mocks.inventory.mockResolvedValue(inventory())
    mocks.backups.mockResolvedValue({ items: [] })
    mocks.environment.mockResolvedValue({
      available: true,
      containers: 1,
      images: 0,
      mirrorPreset: 'official',
      registryMirrors: [],
      ipv6Enabled: false,
      daemonConfig: 'valid',
      observedAt: '2026-08-23T00:00:00Z',
    })
    mocks.jobs.mockResolvedValue({ items: [] })
    mocks.publicNetwork.mockResolvedValue({ ipv4: '203.0.113.10' })
    desktop = document.createElement('div')
    desktop.className = 'desktop'
    windowBody = document.createElement('div')
    windowBody.className = 'desktop-window__body'
    taskbar = document.createElement('div')
    taskbar.className = 'desktop__taskbar'
    desktop.append(windowBody, taskbar)
    document.body.append(desktop)
  })

  afterEach(() => {
    wrapper?.unmount()
    desktop.remove()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it.each(['current', 'available', 'fixed', 'unavailable'])('automatically checks %s and only exposes a passive available badge', async status => {
    vi.useFakeTimers()
    const container = inventory().containers[0]!
    if (status === 'unavailable') mocks.checkUpdate.mockRejectedValueOnce({ code: 'docker_update_registry_auth' })
    else mocks.checkUpdate.mockResolvedValueOnce({ containerId: container.id, image: container.image,
      resourceVersion: container.resourceVersion, status, updateAvailable: status === 'available', checkedAt: new Date().toISOString() })
    wrapper = mount(DockerView, { attachTo: windowBody })
    await flushPromises()
    expect(mocks.checkUpdate).not.toHaveBeenCalled()
    expect(wrapper.find('.docker-image-update').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(1_000)
    await flushPromises()
    expect(mocks.checkUpdate).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.docker-image-update').exists()).toBe(status === 'available')
    expect(wrapper.text()).not.toContain('检查时间')
    expect(wrapper.find('.docker-update-details').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('仓库拒绝访问')
    expect(wrapper.text()).not.toContain('有镜像更新')
    if (status === 'available') {
      const badge = wrapper.get('.docker-image-update')
      expect(badge.attributes('role')).toBe('img')
      expect(badge.attributes('title')).toBe('有镜像更新')
      expect(badge.find('button').exists()).toBe(false)
      await badge.trigger('click')
      expect(mocks.checkUpdate).toHaveBeenCalledTimes(1)
      expect(wrapper.find('.docker-image-update__button').exists()).toBe(false)
    }
  })

  it('pauses automatic checks while the browser is hidden and reuses completed results on return', async () => {
    vi.useFakeTimers()
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const container = inventory().containers[0]!
    mocks.checkUpdate.mockResolvedValue({ containerId: container.id, image: container.image,
      resourceVersion: container.resourceVersion, status: 'current', updateAvailable: false, checkedAt: new Date().toISOString() })
    wrapper = mount(DockerView, { attachTo: windowBody })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(mocks.checkUpdate).not.toHaveBeenCalled()
    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(1_000)
    expect(mocks.checkUpdate).toHaveBeenCalledTimes(1)
    visibility.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60_000)
    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(1_000)
    expect(mocks.checkUpdate).toHaveBeenCalledTimes(1)
  })

  it('measures the full container menu and keeps it above the desktop taskbar', async () => {
    wrapper = mount(DockerView, { attachTo: windowBody })
    await flushPromises()
    const originalBounds = HTMLElement.prototype.getBoundingClientRect
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this === desktop) return rect(0, 0, 1280, 800)
      if (this === windowBody) return rect(100, 80, 1080, 660)
      if (this === taskbar) return rect(8, 728, 1264, 56)
      if (this.matches('.docker-context-menu')) return rect(0, 0, 218, 430)
      return originalBounds.call(this)
    })

    const opener = wrapper.get<HTMLButtonElement>('.docker-context-trigger')
    opener.element.dispatchEvent(new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      clientX: 1200,
      clientY: 760,
      detail: 1,
    }))
    await flushPromises()

    const menu = document.body.querySelector<HTMLElement>('.docker-context-menu')!
    const items = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')]
    expect(menu).not.toBeNull()
    expect(menu.style.left).toBe('954px')
    expect(menu.style.top).toBe('290px')
    expect(Number.parseFloat(menu.style.top) + 430).toBeLessThanOrEqual(720)
    expect(menu.style.getPropertyValue('--context-menu-max-height')).toBe('632px')
    expect(document.activeElement).toBe(items[0])
    expect(menu.dataset.contextMenuFocus).toBe('pointer')
    expect(items[0]?.hasAttribute('aria-selected')).toBe(false)

    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }))
    expect(menu.dataset.contextMenuFocus).toBe('keyboard')
    expect(document.activeElement).toBe(items[1])

    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await flushPromises()
    expect(document.body.querySelector('.docker-context-menu')).toBeNull()
    expect(document.activeElement).toBe(opener.element)
  })
})

// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DesktopView from './DesktopView.vue'
import { api } from '@/lib/api'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import type { DesktopEntries, DesktopEntry } from '@/lib/desktopEntries'
import type { DesktopWorkspace, DesktopWorkspaceUpdate } from '@/types/api'

vi.mock('@/lib/desktopEntries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/desktopEntries')>()
  return { ...actual, loadDesktopEntries: vi.fn() }
})

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      desktop: {
        ...actual.api.desktop,
        workspace: vi.fn(),
        updateWorkspace: vi.fn(),
      },
    },
  }
})

const loadEntries = vi.mocked((await import('@/lib/desktopEntries')).loadDesktopEntries)
const loadWorkspace = vi.mocked(api.desktop.workspace)
const updateWorkspace = vi.mocked(api.desktop.updateWorkspace)

function pointer(
  type: string,
  x: number,
  y: number,
  { id = 1, pointerType = 'mouse', isPrimary = true } = {},
): Event {
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, clientX: x, clientY: y })
  Object.defineProperties(event, {
    pointerId: { value: id },
    pointerType: { value: pointerType },
    isPrimary: { value: isPrimary },
  })
  return event
}

function workspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
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

describe('DesktopView icon layout interaction', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetDesktopModeForTest()
    resetDesktopIconsForTest()
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
    loadEntries.mockResolvedValue({
      apps: [], sites: [], visible: [], loadedAt: Date.now(),
    } satisfies DesktopEntries)
    loadWorkspace.mockResolvedValue(workspace())
    updateWorkspace.mockImplementation(async (body: DesktopWorkspaceUpdate) => workspace({
      resourceVersion: `sha256:${'2'.repeat(64)}`,
      hiddenEntryKeys: body.hiddenEntryKeys,
      positions: body.positions,
      labels: body.labels,
    }))
  })

  it('persists one snapped drop and suppresses the click generated after dragging', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    const icon = slot.find('button')

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointermove', 145, 30))
    window.dispatchEvent(pointer('pointerup', 145, 30))
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledTimes(1)
    const saved = updateWorkspace.mock.calls[0]?.[0].positions['nav:/overview']
    expect(saved?.x).toBeGreaterThan(0)
    expect(saved?.y).toBe(0)

    await icon.trigger('click')
    await icon.trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(0)

    // A new intentional pointer gesture clears only the stale synthetic-click
    // guard, so the next real double click remains usable without a timer.
    slot.element.dispatchEvent(pointer('pointerdown', 145, 30))
    window.dispatchEvent(pointer('pointerup', 145, 30))
    await icon.trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(1)
    wrapper.unmount()
  })

  it('captures the active pointer and safely releases it after a drop', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    const setPointerCapture = vi.fn()
    const hasPointerCapture = vi.fn(() => true)
    const releasePointerCapture = vi.fn()
    Object.assign(slot.element, { setPointerCapture, hasPointerCapture, releasePointerCapture })

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    expect(setPointerCapture).not.toHaveBeenCalled()
    window.dispatchEvent(pointer('pointermove', 34, 30))
    expect(setPointerCapture).not.toHaveBeenCalled()
    window.dispatchEvent(pointer('pointermove', 145, 30))
    expect(setPointerCapture).toHaveBeenCalledWith(1)
    window.dispatchEvent(pointer('pointerup', 145, 30))
    await flushPromises()

    expect(hasPointerCapture).toHaveBeenCalledWith(1)
    expect(releasePointerCapture).toHaveBeenCalledWith(1)
    expect(updateWorkspace).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('does not capture a normal click and keeps child click and double-click activation', async () => {
    const desktop = useDesktopMode()
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    const icon = slot.find('button')
    const setPointerCapture = vi.fn()
    Object.assign(slot.element, { setPointerCapture })

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointerup', 30, 30))
    await icon.trigger('click')
    await flushPromises()

    expect(setPointerCapture).not.toHaveBeenCalled()
    expect(icon.classes()).toContain('desktop__icon--selected')

    await icon.trigger('dblclick')
    expect(desktop.windows.value).toHaveLength(1)
    wrapper.unmount()
  })

  it('keeps a cross-page drag aligned while bounded edge scrolling advances the work area', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const workArea = wrapper.find<HTMLElement>('.desktop__icons').element
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    let frameCallback: FrameRequestCallback | undefined
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frameCallback = callback
      return 71
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    Object.defineProperties(workArea, {
      clientHeight: { configurable: true, value: 300 },
      scrollHeight: { configurable: true, value: 1_200 },
    })
    vi.spyOn(workArea, 'getBoundingClientRect').mockReturnValue({
      x: 0, y: 0, top: 0, right: 600, bottom: 300, left: 0,
      width: 600, height: 300, toJSON: () => ({}),
    })

    slot.element.dispatchEvent(pointer('pointerdown', 30, 100))
    window.dispatchEvent(pointer('pointermove', 30, 290))
    const before = Number.parseFloat((slot.attributes('style') || '').match(/top:\s*([\d.]+)px/)?.[1] || '0')
    expect(frameCallback).toBeTypeOf('function')
    frameCallback?.(performance.now())
    await flushPromises()
    const after = Number.parseFloat((slot.attributes('style') || '').match(/top:\s*([\d.]+)px/)?.[1] || '0')

    expect(workArea.scrollTop).toBeGreaterThan(0)
    expect(after).toBeGreaterThan(before)
    window.dispatchEvent(pointer('pointercancel', 30, 290))
    expect(cancelFrame).toHaveBeenCalledWith(71)
    requestFrame.mockRestore()
    cancelFrame.mockRestore()
    wrapper.unmount()
  })

  it('cancels and restores the saved position when pointer capture is lost', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    Object.assign(slot.element, {
      setPointerCapture: vi.fn(),
      hasPointerCapture: vi.fn(() => false),
      releasePointerCapture: vi.fn(),
    })

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointermove', 145, 30))
    await flushPromises()
    expect(slot.classes()).toContain('desktop__icon-slot--dragging')

    slot.element.dispatchEvent(pointer('lostpointercapture', 145, 30))
    await flushPromises()

    expect(slot.classes()).not.toContain('desktop__icon-slot--dragging')
    expect(slot.attributes('style')).toContain('left: 0px')
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('cancels an active drag when a second pointer appears', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30, { pointerType: 'touch' }))
    window.dispatchEvent(pointer('pointermove', 145, 30, { pointerType: 'touch' }))
    await flushPromises()
    expect(slot.classes()).toContain('desktop__icon-slot--dragging')

    window.dispatchEvent(pointer('pointerdown', 220, 120, {
      id: 2,
      pointerType: 'touch',
      isPrimary: false,
    }))
    await flushPromises()

    expect(slot.classes()).not.toContain('desktop__icon-slot--dragging')
    expect(slot.attributes('style')).toContain('left: 0px')
    expect(document.body.classList.contains('desktop-icon-dragging')).toBe(false)
    window.dispatchEvent(pointer('pointerup', 145, 30))
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps saved positions for hidden entries while auto-arranging visible icons', async () => {
    loadWorkspace.mockResolvedValueOnce(workspace({
      hiddenEntryKeys: ['app:hidden'],
      positions: { 'app:hidden': { x: 0.75, y: 0.5 } },
    }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await flushPromises()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('管理桌面图标'))
    await manage?.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')?.click()
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledWith(expect.objectContaining({
      hiddenEntryKeys: ['app:hidden'],
      positions: expect.objectContaining({ 'app:hidden': { x: 0.75, y: 0.5 } }),
    }))
    wrapper.unmount()
  })

  it('keeps compact auto layout temporary and disables position writes', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 640 })
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await flushPromises()
    const items = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
    expect(items.slice(0, 3).map((item) => item.text())).toEqual([
      '刷新桌面',
      '添加快捷方式',
      '管理桌面图标',
    ])
    expect(items.some((item) => item.text().includes('自动整理'))).toBe(false)
    expect(items.some((item) => item.text().includes('恢复默认位置'))).toBe(false)
    expect(items.some((item) => item.text().includes('整理模式'))).toBe(false)

    await items[2]?.trigger('click')
    await flushPromises()
    const autoArrange = document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')
    expect(autoArrange?.disabled).toBe(true)
    autoArrange?.click()
    await flushPromises()
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('places icons beyond one compact page in the scroll surface without writing positions', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 320 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 568 })
    const extra: DesktopEntry = {
      key: 'app:extra',
      kind: 'app',
      id: 'extra',
      name: 'Extra app',
      launch: 'external',
      url: 'https://example.com',
    }
    loadEntries.mockResolvedValueOnce({
      apps: [extra], sites: [], visible: [extra], loadedAt: Date.now(),
    })

    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const extraSlot = wrapper.find('[data-icon-key="app:extra"]')
    const scrollSpace = wrapper.find('.desktop__icons-scroll-space')

    expect(extraSlot.attributes('style')).toContain('top: 500px')
    expect(Number.parseFloat((scrollSpace.attributes('style') || '').match(/height:\s*([\d.]+)px/)?.[1] || '0'))
      .toBeGreaterThan(480)
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps icons beyond the 512-position limit separate and refuses false auto-arrange success', async () => {
    const extras: DesktopEntry[] = Array.from({ length: 501 }, (_, index) => ({
      key: `app:extra-${index}`,
      kind: 'app',
      id: `extra-${index}`,
      name: `Extra ${index}`,
      launch: 'external',
      url: `https://example.com/${index}`,
    }))
    loadEntries.mockResolvedValueOnce({
      apps: extras, sites: [], visible: extras, loadedAt: Date.now(),
    })

    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const supported = wrapper.find('[data-icon-key="app:extra-499"]')
    const overflow = wrapper.find('[data-icon-key="app:extra-500"]')

    expect(overflow.attributes('style')).not.toBe(supported.attributes('style'))
    expect(overflow.attributes('style')).not.toContain('display: none')
    expect(wrapper.find('.desktop__icons-overflow-note').text()).toContain('另有 2 个图标')

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await flushPromises()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('管理桌面图标'))
    await manage?.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')?.click()
    await flushPromises()

    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

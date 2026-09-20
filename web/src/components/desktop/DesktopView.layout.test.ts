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

function deferred<T>(): {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
} {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
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
      widgetPositions: body.widgetPositions,
      labels: body.labels,
    }))
  })

  it('resizes both snapped panes from one accessible center divider and persists on release', async () => {
    window.localStorage.removeItem('kpanel:desktop-side-split:v1')
    const desktop = useDesktopMode()
    const leftId = desktop.openWindow('/overview', 'route.overview', false)
    const rightId = desktop.openWindow('/files', 'route.files', true)
    desktop.snapWindow(leftId, 'left')
    desktop.snapWindow(rightId, 'right')
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    const divider = wrapper.get('.desktop-window-split-resizer')
    expect(divider.attributes('role')).toBe('separator')
    expect(divider.attributes('aria-controls')).toBe(`desktop-window-${leftId} desktop-window-${rightId}`)
    expect(divider.attributes('aria-valuenow')).toBe('50')

    divider.element.dispatchEvent(pointer('pointerdown', 640, 300))
    expect(document.activeElement).toBe(divider.element)
    window.dispatchEvent(pointer('pointermove', 790, 300))
    window.dispatchEvent(pointer('pointerup', 790, 300))
    await flushPromises()

    expect(document.activeElement).not.toBe(divider.element)
    expect(desktop.sideSplitRatio.value).toBeCloseTo(0.62)
    expect(divider.attributes('aria-valuenow')).toBe('62')
    expect(wrapper.attributes('style')).toContain('--desktop-side-split-left-width: 775px')
    expect(wrapper.attributes('style')).toContain('--desktop-side-split-right-width: 475px')
    expect(Number(window.localStorage.getItem('kpanel:desktop-side-split:v1'))).toBeCloseTo(0.62)

    await divider.trigger('keydown', { key: 'ArrowLeft' })
    expect(desktop.sideSplitRatio.value).toBeLessThan(0.62)
    expect(Number(window.localStorage.getItem('kpanel:desktop-side-split:v1'))).toBeCloseTo(
      desktop.sideSplitRatio.value,
    )

    await divider.trigger('keydown', { key: 'Enter' })
    expect(desktop.sideSplitRatio.value).toBe(0.5)
    expect(Number(window.localStorage.getItem('kpanel:desktop-side-split:v1'))).toBe(0.5)
    wrapper.unmount()
  })

  it('starts ungrouped, creates a group from selection, collapses and dissolves without deleting entries', async () => {
    updateWorkspace.mockImplementation(async body => workspace({ positions: body.positions, groups: body.groups, resourceVersion: `sha256:${'3'.repeat(64)}` }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.find('.desktop-group').exists()).toBe(false)
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('style')).toContain('translate3d(')
    await wrapper.get('[data-icon-key="nav:/overview"] button').trigger('click')
    await wrapper.get('[data-icon-key="nav:/terminal"] button').trigger('click', { ctrlKey: true })
    const create = wrapper.findAll('.desktop__selection-actions button').find(item => item.text().includes('编为一组'))!
    await create.trigger('click')
    await flushPromises()
    expect(document.querySelector('.desktop-group-form')).toBeNull()
    expect(updateWorkspace).toHaveBeenCalledTimes(1)
    expect(updateWorkspace.mock.calls[0]![0].groups?.[0]?.name).toBe('新分组')
    expect(updateWorkspace.mock.calls[0]![0].groups?.[0]?.rows).toBe(0)
    expect(updateWorkspace.mock.calls[0]![0].groups?.[0]?.members).toEqual(['nav:/overview', 'nav:/terminal'])
    expect(updateWorkspace.mock.calls[0]![0].groups?.[0]?.columns).toBe(4)
    expect(wrapper.findAll('.desktop-group')).toHaveLength(1)
    expect(wrapper.get('[data-icon-key="nav:/files"]').attributes('data-group-member')).toBeUndefined()
    await wrapper.get('.desktop-group__toggle').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('aria-hidden')).toBe('true')
    await wrapper.get('.desktop-group__menu').trigger('click')
    await flushPromises()
    const dissolve = [...document.querySelectorAll<HTMLButtonElement>('.desktop-group__actions button')].find(el => el.textContent?.includes('解散分组'))!
    dissolve.click()
    await flushPromises()
    expect(wrapper.find('.desktop-group').exists()).toBe(false)
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('style')).not.toContain('display: none')
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('style')).toContain('translate3d(')
    expect(updateWorkspace.mock.calls.at(-1)![0].hiddenEntryKeys).toEqual([])
    wrapper.unmount()
  })

  it('restores selection after failed one-step creation and allows retry without a form', async () => {
    updateWorkspace.mockRejectedValueOnce(new Error('disk full'))
    updateWorkspace.mockImplementationOnce(async body => workspace({ positions: body.positions, groups: body.groups }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-icon-key="nav:/overview"] button').trigger('click')
    await wrapper.get('[data-icon-key="nav:/terminal"] button').trigger('click', { ctrlKey: true })
    const create = () => wrapper.findAll('.desktop__selection-actions button').find(item => item.text().includes('编为一组'))!
    await create().trigger('click'); await flushPromises()
    expect(wrapper.find('.desktop-group').exists()).toBe(false)
    expect(wrapper.get('.desktop__selection-actions').text()).toContain('2')
    expect(wrapper.get('.desktop-group-undo[role="alert"]').text()).toBeTruthy()
    expect(document.querySelector('.desktop-group-form')).toBeNull()
    await create().trigger('click'); await flushPromises()
    expect(wrapper.findAll('.desktop-group')).toHaveLength(1)
    expect(updateWorkspace).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('restores group state on failed saves and never leaves grouped icons lost', async () => {
    const group = { id: 'a'.repeat(32), name: '运维', columns: 3, collapsed: false, members: ['nav:/overview'] }
    loadWorkspace.mockResolvedValue(workspace({ groups: [group] }))
    updateWorkspace.mockRejectedValue(new Error('disk full'))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('.desktop-group__toggle').trigger('click')
    await flushPromises()
    expect(wrapper.get('.desktop-group__toggle').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('style')).not.toContain('display: none')
    wrapper.unmount()
  })

  it('reveals dissolution before the response and restores icons with a persistent error on failure', async () => {
    const group = { id: 'a'.repeat(32), name: '运维', columns: 3, collapsed: false, members: ['nav:/overview'] }
    loadWorkspace.mockResolvedValue(workspace({ groups: [group] }))
    const response = deferred<DesktopWorkspace>()
    updateWorkspace.mockReturnValue(response.promise)
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('.desktop-group__menu').trigger('click')
    await flushPromises()
    const dissolve = [...document.querySelectorAll<HTMLButtonElement>('.desktop-group__actions button')].find(el => el.textContent?.includes('解散分组'))!
    dissolve.click()
    await flushPromises()
    expect(wrapper.find('.desktop-group').exists()).toBe(false)
    expect(document.querySelector('.desktop-group-form')).toBeNull()
    response.reject(new Error('disk full'))
    await flushPromises()
    expect(wrapper.findAll('.desktop-group')).toHaveLength(1)
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('data-group-member')).toBe(group.id)
    expect(wrapper.get('.desktop-group-undo[role="alert"]').text()).toBeTruthy()
    wrapper.unmount()
  })

  it('restores the previous split after cancellation and hides the divider once no snapped window is visible', async () => {
    window.localStorage.removeItem('kpanel:desktop-side-split:v1')
    const desktop = useDesktopMode()
    const leftId = desktop.openWindow('/overview', 'route.overview', false)
    const rightId = desktop.openWindow('/files', 'route.files', true)
    desktop.snapWindow(leftId, 'left')
    desktop.snapWindow(rightId, 'right')
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    const divider = wrapper.get('.desktop-window-split-resizer')
    divider.element.dispatchEvent(pointer('pointerdown', 640, 300))
    window.dispatchEvent(pointer('pointermove', 790, 300))
    window.dispatchEvent(pointer('pointercancel', 790, 300))
    await flushPromises()

    expect(desktop.sideSplitRatio.value).toBe(0.5)
    expect(window.localStorage.getItem('kpanel:desktop-side-split:v1')).toBeNull()

    desktop.minimizeWindow(leftId)
    desktop.minimizeWindow(rightId)
    await flushPromises()
    expect(wrapper.find('.desktop-window-split-resizer').exists()).toBe(false)
    wrapper.unmount()
  })

  it('resizes a single snapped window from the divider and persists on release', async () => {
    window.localStorage.removeItem('kpanel:desktop-side-split:v1')
    const desktop = useDesktopMode()
    const leftId = desktop.openWindow('/overview', 'route.overview', false)
    desktop.snapWindow(leftId, 'left')
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    const divider = wrapper.get('.desktop-window-split-resizer')
    expect(divider.attributes('aria-controls')).toBe(`desktop-window-${leftId}`)

    divider.element.dispatchEvent(pointer('pointerdown', 640, 300))
    window.dispatchEvent(pointer('pointermove', 790, 300))
    window.dispatchEvent(pointer('pointerup', 790, 300))
    await flushPromises()

    expect(desktop.sideSplitRatio.value).toBeCloseTo(0.62)
    expect(wrapper.attributes('style')).toContain('--desktop-side-split-left-width: 775px')
    expect(Number(window.localStorage.getItem('kpanel:desktop-side-split:v1'))).toBeCloseTo(0.62)
    wrapper.unmount()
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

  it('persists a widget drop in widgetPositions and supports keyboard nudging', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const widget = wrapper.get('[aria-label="widget:clock"]')
    const handle = widget.find('.desktop-widget__drag-handle')

    handle.element.dispatchEvent(pointer('pointerdown', 900, 40))
    window.dispatchEvent(pointer('pointermove', 760, 180))
    window.dispatchEvent(pointer('pointerup', 760, 180))
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledTimes(1)
    expect(updateWorkspace.mock.calls[0]?.[0].widgetPositions['widget:clock']).toBeDefined()

    await widget.trigger('keydown', { key: 'ArrowLeft', ctrlKey: true })
    await flushPromises()
    expect(updateWorkspace).toHaveBeenCalledTimes(2)
    expect(updateWorkspace.mock.calls[1]?.[0].widgetPositions).toEqual(expect.objectContaining({
      'widget:clock': expect.any(Object),
    }))
    wrapper.unmount()
  })

  it('keeps the newest optimistic position while an earlier position write settles', async () => {
    const firstWrite = deferred<DesktopWorkspace>()
    const secondWrite = deferred<DesktopWorkspace>()
    updateWorkspace
      .mockImplementationOnce(() => firstWrite.promise)
      .mockImplementationOnce(() => secondWrite.promise)
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.get('[data-icon-key="nav:/overview"]')

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointermove', 145, 30))
    window.dispatchEvent(pointer('pointerup', 145, 30))
    await flushPromises()
    const firstPosition = updateWorkspace.mock.calls[0]![0].positions['nav:/overview']!

    slot.element.dispatchEvent(pointer('pointerdown', 145, 30))
    window.dispatchEvent(pointer('pointermove', 250, 30))
    window.dispatchEvent(pointer('pointerup', 250, 30))
    await flushPromises()
    const optimisticStyle = slot.attributes('style')
    expect(updateWorkspace).toHaveBeenCalledTimes(1)

    firstWrite.resolve(workspace({
      resourceVersion: `sha256:${'2'.repeat(64)}`,
      positions: { 'nav:/overview': firstPosition },
    }))
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledTimes(2)
    expect(slot.attributes('style')).toBe(optimisticStyle)
    const finalBody = updateWorkspace.mock.calls[1]![0]
    secondWrite.resolve(workspace({
      resourceVersion: `sha256:${'3'.repeat(64)}`,
      positions: finalBody.positions,
    }))
    await flushPromises()

    expect(slot.attributes('style')).toBe(optimisticStyle)
    wrapper.unmount()
  })

  it('does not roll back a newer optimistic position when an earlier write fails', async () => {
    const firstWrite = deferred<DesktopWorkspace>()
    const secondWrite = deferred<DesktopWorkspace>()
    updateWorkspace
      .mockImplementationOnce(() => firstWrite.promise)
      .mockImplementationOnce(() => secondWrite.promise)
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const slot = wrapper.get('[data-icon-key="nav:/overview"]')

    slot.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointermove', 145, 30))
    window.dispatchEvent(pointer('pointerup', 145, 30))
    await flushPromises()
    slot.element.dispatchEvent(pointer('pointerdown', 145, 30))
    window.dispatchEvent(pointer('pointermove', 250, 30))
    window.dispatchEvent(pointer('pointerup', 250, 30))
    await flushPromises()
    const optimisticStyle = slot.attributes('style')

    firstWrite.reject(new Error('earlier write failed'))
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledTimes(2)
    expect(slot.attributes('style')).toBe(optimisticStyle)
    const finalBody = updateWorkspace.mock.calls[1]![0]
    secondWrite.resolve(workspace({
      resourceVersion: `sha256:${'3'.repeat(64)}`,
      positions: finalBody.positions,
    }))
    await flushPromises()

    expect(slot.attributes('style')).toBe(optimisticStyle)
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

  it('selects intersecting icons with a desktop frame and exposes batch actions', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const root = wrapper.get<HTMLElement>('.desktop')
    const slots = wrapper.findAll<HTMLElement>('.desktop__icon-slot')
    vi.spyOn(slots[0]!.element, 'getBoundingClientRect').mockReturnValue({
      x: 20, y: 20, top: 20, right: 110, bottom: 116, left: 20,
      width: 90, height: 96, toJSON: () => ({}),
    })
    vi.spyOn(slots[1]!.element, 'getBoundingClientRect').mockReturnValue({
      x: 20, y: 120, top: 120, right: 110, bottom: 216, left: 20,
      width: 90, height: 96, toJSON: () => ({}),
    })

    root.element.dispatchEvent(pointer('pointerdown', 10, 10))
    window.dispatchEvent(pointer('pointermove', 125, 225))
    await flushPromises()
    expect(wrapper.find('.desktop__selection-box').exists()).toBe(true)
    window.dispatchEvent(pointer('pointerup', 125, 225))
    await flushPromises()

    expect(slots[0]!.find('button').classes()).toContain('desktop__icon--selected')
    expect(slots[1]!.find('button').classes()).toContain('desktop__icon--selected')
    expect(wrapper.find('.desktop__selection-box').exists()).toBe(false)
    expect(wrapper.find('.desktop__selection-actions').text()).toContain('已选 2 项')
    wrapper.unmount()
  })

  it('adds icons with Control and moves the selected group in one workspace write', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const overview = wrapper.get('[data-icon-key="nav:/overview"]')
    const terminal = wrapper.get('[data-icon-key="nav:/terminal"]')

    await overview.get('button').trigger('click')
    await terminal.get('button').trigger('click', { ctrlKey: true })
    expect(wrapper.find('.desktop__selection-actions').text()).toContain('已选 2 项')

    overview.element.dispatchEvent(pointer('pointerdown', 30, 30))
    window.dispatchEvent(pointer('pointermove', 250, 30))
    window.dispatchEvent(pointer('pointerup', 250, 30))
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledTimes(1)
    const positions = updateWorkspace.mock.calls[0]![0].positions
    expect(positions['nav:/overview']?.x).toBeGreaterThan(0)
    expect(positions['nav:/terminal']?.x).toBe(positions['nav:/overview']?.x)
    expect(positions['nav:/terminal']?.y).toBeGreaterThan(positions['nav:/overview']?.y || 0)
    wrapper.unmount()
  })

  it('cancels a selection frame without replacing the previous selection', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const overview = wrapper.get('[data-icon-key="nav:/overview"] button')
    await overview.trigger('click')

    wrapper.get('.desktop').element.dispatchEvent(pointer('pointerdown', 400, 300))
    window.dispatchEvent(pointer('pointermove', 520, 420))
    window.dispatchEvent(pointer('pointercancel', 520, 420))
    await flushPromises()

    expect(overview.classes()).toContain('desktop__icon--selected')
    expect(wrapper.find('.desktop__selection-box').exists()).toBe(false)
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('selects all icons from the keyboard and never removes fixed system entries', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const desktop = wrapper.get<HTMLElement>('.desktop')
    desktop.element.focus()

    await desktop.trigger('keydown', { key: 'a', ctrlKey: true })
    expect(wrapper.findAll('.desktop__icon--selected')).toHaveLength(12)
    expect(wrapper.find('.desktop__selection-actions').text()).toContain('已选 12 项')

    await desktop.trigger('keydown', { key: 'Delete' })
    await flushPromises()
    expect(updateWorkspace).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('固定系统入口不能从桌面移除')
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
    const before = Number.parseFloat((slot.attributes('style') || '').match(/translate3d\([^,]+,\s*([\d.]+)px/)?.[1] || '0')
    expect(frameCallback).toBeTypeOf('function')
    frameCallback?.(performance.now())
    await flushPromises()
    const after = Number.parseFloat((slot.attributes('style') || '').match(/translate3d\([^,]+,\s*([\d.]+)px/)?.[1] || '0')

    expect(workArea.scrollTop).toBeGreaterThan(0)
    expect(after).toBeGreaterThan(before)
    window.dispatchEvent(pointer('pointercancel', 30, 290))
    expect(cancelFrame).toHaveBeenCalledWith(71)
    requestFrame.mockRestore()
    cancelFrame.mockRestore()
    wrapper.unmount()
  })

  it('holds a newly expanded drag surface while the pointer returns from the bottom edge', async () => {
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    const workArea = wrapper.find<HTMLElement>('.desktop__icons').element
    const slot = wrapper.find('[data-icon-key="nav:/overview"]')
    let frameCallback: FrameRequestCallback | undefined
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frameCallback = callback
      return 72
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    Object.defineProperties(workArea, {
      clientHeight: { configurable: true, value: 300 },
      scrollHeight: { configurable: true, value: 300 },
    })
    vi.spyOn(workArea, 'getBoundingClientRect').mockReturnValue({
      x: 0, y: 0, top: 0, right: 600, bottom: 300, left: 0,
      width: 600, height: 300, toJSON: () => ({}),
    })

    const heightOf = () => Number.parseFloat(
      (wrapper.find('.desktop__icons-scroll-space').attributes('style') || '')
        .match(/height:\s*([\d.]+)px/)?.[1] || '0',
    )
    const initialHeight = heightOf()

    slot.element.dispatchEvent(pointer('pointerdown', 30, 100))
    window.dispatchEvent(pointer('pointermove', 30, 290))
    frameCallback?.(performance.now())
    await flushPromises()
    const expandedHeight = heightOf()

    expect(expandedHeight).toBeGreaterThan(initialHeight)

    window.dispatchEvent(pointer('pointermove', 30, 150))
    await flushPromises()
    expect(heightOf()).toBe(expandedHeight)

    window.dispatchEvent(pointer('pointerup', 30, 150))
    await flushPromises()
    expect(heightOf()).toBeLessThan(expandedHeight)

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
      widgetPositions: {
        'widget:clock': { x: 0.4073756432246998, y: 0 },
        'widget:monitor': { x: 0.7332761578044596, y: 0.3246753246753247 },
      },
    }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await flushPromises()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('桌面布局管理'))
    await manage?.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')?.click()
    await flushPromises()

    expect(updateWorkspace).toHaveBeenCalledWith(expect.objectContaining({
      hiddenEntryKeys: ['app:hidden'],
      positions: expect.objectContaining({ 'app:hidden': { x: 0.75, y: 0.5 } }),
      widgetPositions: expect.objectContaining({
        'widget:clock': { x: 0.4073756432246998, y: 0 },
        'widget:monitor': { x: 0.7332761578044596, y: 0.3246753246753247 },
      }),
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
      '桌面布局管理',
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

    expect(extraSlot.attributes('style')).toContain('translate3d(0px, 400px, 0)')
    expect(Number.parseFloat((scrollSpace.attributes('style') || '').match(/height:\s*([\d.]+)px/)?.[1] || '0'))
      .toBeGreaterThan(480)
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps group containers and members accessible when dynamic entries overflow the position budget', async () => {
    const extras: DesktopEntry[] = Array.from({ length: 512 }, (_, index) => ({
      key: `app:overflow-${index}`, kind: 'app', id: `overflow-${index}`,
      name: `Extra ${index}`, launch: 'external', url: `https://example.com/${index}`,
    }))
    loadEntries.mockResolvedValueOnce({ apps: extras, sites: [], visible: extras, loadedAt: Date.now() })
    loadWorkspace.mockResolvedValueOnce(workspace({ groups: [{
      id: 'a'.repeat(32), name: 'Group', members: ['nav:/overview'], columns: 3, collapsed: false,
    }] }))
    const wrapper = mount(DesktopView, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.get('.desktop-group').attributes('style')).not.toContain('display: none')
    expect(wrapper.get('[data-icon-key="nav:/overview"]').attributes('style')).not.toContain('display: none')
    expect(wrapper.get('[data-icon-key="app:overflow-511"]').attributes('style')).not.toContain('display: none')
    expect(wrapper.find('.desktop__icons-overflow-note').exists()).toBe(true)
    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  }, 10_000)

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
    expect(wrapper.find('.desktop__icons-overflow-note').text()).toContain('另有 1 个图标')

    await wrapper.trigger('contextmenu', { clientX: 220, clientY: 160 })
    await flushPromises()
    const manage = wrapper.findAll('.desktop__context-menu [role="menuitem"]')
      .find((item) => item.text().includes('桌面布局管理'))
    await manage?.trigger('click')
    await flushPromises()
    document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')?.click()
    await flushPromises()

    expect(updateWorkspace).not.toHaveBeenCalled()
    wrapper.unmount()
  }, 10_000)
})

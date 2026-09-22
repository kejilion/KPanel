// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { FileText, FolderOpen } from '@lucide/vue'
import DesktopEntryIcon from './DesktopEntryIcon.vue'

function pointer(
  type: string,
  { x = 40, y = 50, id = 1, pointerType = 'touch' } = {},
): PointerEvent {
  const event = new Event(type, { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperties(event, {
    button: { value: 0 },
    clientX: { value: x },
    clientY: { value: y },
    pointerId: { value: id },
    pointerType: { value: pointerType },
  })
  return event
}

function mountIcon() {
  return mount(DesktopEntryIcon, {
    attachTo: document.body,
    props: {
      label: '概览',
      gradient: 'linear-gradient(#0aa, #066)',
    },
  })
}

describe('DesktopEntryIcon touch interaction', () => {
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('prefers native navigation artwork and falls back when it cannot load', async () => {
    const wrapper = mount(DesktopEntryIcon, {
      props: {
        label: '概览',
        gradient: 'linear-gradient(#0aa, #066)',
        navIconURL: '/desktop-icons/overview-kpanel-flat-v1.webp',
        entry: {
          key: 'app:market-app',
          id: 'market-app',
          kind: 'app',
          name: '市场应用',
          launch: 'external',
          iconURL: '/market-icon.webp',
        },
      },
    })

    const image = wrapper.get('img')
    expect(image.attributes('src')).toBe('/desktop-icons/overview-kpanel-flat-v1.webp')
    expect(image.classes()).toContain('desktop__icon-img--native')

    await image.trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.get('.desktop__icon-monogram').text()).toBe('概')
    wrapper.unmount()
  })

  it('keeps an actionable fallback until each image loads and resets it for a new URL', async () => {
    const wrapper = mountIcon()
    await wrapper.setProps({ navIconURL: '/slow.webp' })
    expect(wrapper.get('.desktop__icon-monogram').text()).toBe('概')
    expect(wrapper.get('img').classes()).toContain('desktop__icon-img--loading')
    await wrapper.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('open')).toHaveLength(1)
    await wrapper.get('img').trigger('load')
    expect(wrapper.find('.desktop__icon-monogram').exists()).toBe(false)
    expect(wrapper.get('img').classes()).not.toContain('desktop__icon-img--loading')
    await wrapper.setProps({ navIconURL: '/other.webp' })
    expect(wrapper.find('.desktop__icon-monogram').exists()).toBe(true)
    await wrapper.get('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('.desktop__icon-monogram').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([
    ['file', FileText],
    ['directory', FolderOpen],
  ] as const)('renders %s shortcuts with unified shortcut artwork', (launch, icon) => {
    const wrapper = mount(DesktopEntryIcon, {
      props: {
        label: launch === 'file' ? 'README.md' : '项目',
        gradient: 'linear-gradient(#facc15, #ca8a04)',
        entry: {
          key: `shortcut:${launch}`,
          id: launch,
          kind: 'shortcut',
          name: launch,
          launch,
          icon,
        },
      },
    })

    expect(wrapper.get('.desktop__shortcut-artwork').classes())
      .toContain(`desktop__shortcut-artwork--${launch}`)
    expect(wrapper.find('.desktop__shortcut-link-badge').exists()).toBe(false)
    expect(wrapper.find('.desktop__shortcut-artwork > svg').exists()).toBe(launch === 'file')
    expect(wrapper.find('.desktop__shortcut-directory-image').exists()).toBe(launch === 'directory')
    if (launch === 'directory') {
      expect(wrapper.get('.desktop__shortcut-directory-image').attributes('src'))
        .toBe('/desktop-icons/folder-open-shortcut-kpanel-flat-v1.webp')
    }
    wrapper.unmount()
  })

  it('keeps Windows mouse selection and double-click activation', async () => {
    const wrapper = mountIcon()
    wrapper.element.dispatchEvent(pointer('pointerdown', { pointerType: 'mouse' }))

    await wrapper.trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
    expect(wrapper.emitted('open')).toBeUndefined()

    await wrapper.trigger('dblclick')
    expect(wrapper.emitted('open')).toHaveLength(1)
    wrapper.unmount()
  })

  it('opens once on a touch tap and ignores synthesized double-click events', async () => {
    const wrapper = mountIcon()
    wrapper.element.dispatchEvent(pointer('pointerdown'))
    window.dispatchEvent(pointer('pointerup'))

    await wrapper.trigger('click')
    await wrapper.trigger('click')
    await wrapper.trigger('dblclick')

    expect(wrapper.emitted('select')).toBeUndefined()
    expect(wrapper.emitted('open')).toHaveLength(1)
    wrapper.unmount()
  })

  it('opens the existing context menu on long press and consumes the following click', async () => {
    vi.useFakeTimers()
    const wrapper = mountIcon()
    wrapper.element.dispatchEvent(pointer('pointerdown', { x: 72, y: 84 }))

    vi.advanceTimersByTime(520)
    window.dispatchEvent(pointer('pointerup', { x: 72, y: 84 }))
    await wrapper.trigger('click')
    await wrapper.trigger('contextmenu')

    expect(wrapper.emitted('context')).toHaveLength(1)
    expect((wrapper.emitted('context')?.[0]?.[0] as MouseEvent).button).toBe(2)
    expect(wrapper.emitted('open')).toBeUndefined()
    wrapper.unmount()
  })

  it('cancels a long press when the finger moves or the pointer is cancelled', () => {
    vi.useFakeTimers()
    const moved = mountIcon()
    moved.element.dispatchEvent(pointer('pointerdown'))
    window.dispatchEvent(pointer('pointermove', { x: 56, y: 66 }))
    vi.advanceTimersByTime(600)
    expect(moved.emitted('context')).toBeUndefined()
    moved.unmount()

    const cancelled = mountIcon()
    cancelled.element.dispatchEvent(pointer('pointerdown', { id: 2 }))
    window.dispatchEvent(pointer('pointercancel', { id: 2 }))
    vi.advanceTimersByTime(600)
    expect(cancelled.emitted('context')).toBeUndefined()
    cancelled.unmount()
  })

  it.each([
    { key: 'ContextMenu' },
    { key: 'F10', shiftKey: true },
  ])('supports the $key keyboard context-menu shortcut', async (shortcut) => {
    const wrapper = mountIcon()
    await wrapper.trigger('keydown', shortcut)
    expect(wrapper.emitted('context')).toHaveLength(1)
    expect((wrapper.emitted('context')?.[0]?.[0] as MouseEvent).button).toBe(0)
    wrapper.unmount()
  })
})

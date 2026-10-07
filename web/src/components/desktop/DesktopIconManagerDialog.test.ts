// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { desktopWidgets } from '@/lib/desktopWidgets'
import DesktopIconManagerDialog from './DesktopIconManagerDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('DesktopIconManagerDialog', () => {
  it('groups widgets before desktop entries and keeps layout actions at the bottom', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
        widgets: desktopWidgets,
        hiddenWidgetKeys: [],
        canAutoArrange: true,
      },
    })
    await nextTick()

    const manager = document.body.querySelector<HTMLElement>('.desktop-icon-manager')
    expect(manager?.querySelector('.desktop-icon-manager__section--widgets')?.textContent).toContain('桌面小插件')
    expect(manager?.querySelector('.desktop-icon-manager__collections')?.textContent).toContain('自定义快捷方式')
    expect(manager?.querySelector('.desktop-icon-manager__collections')?.textContent).toContain('已从桌面移除')
    expect(manager?.querySelector('.desktop-icon-manager__layout-action')?.textContent).toContain('自动整理图标')
    wrapper.unmount()
  })

  it('emits the desired visibility when a widget is toggled', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
        widgets: desktopWidgets,
        hiddenWidgetKeys: ['widget:monitor'],
        canAutoArrange: true,
      },
    })
    await nextTick()

    const monitor = document.body.querySelector<HTMLElement>('[data-widget-key="widget:monitor"]')
    const toggle = monitor?.querySelector<HTMLButtonElement>('.desktop-icon-manager__widget-toggle')
    expect(toggle?.textContent).toContain('显示')
    toggle?.click()
    await nextTick()

    expect(wrapper.emitted('toggleWidget')?.[0]).toEqual(['widget:monitor', true])
    wrapper.unmount()
  })

  it('emits autoArrange from the layout action when wide-layout editing is available', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
        widgets: desktopWidgets,
        hiddenWidgetKeys: [],
        canAutoArrange: true,
      },
    })
    await nextTick()

    const action = document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')
    expect(action?.disabled).toBe(false)
    action?.click()
    await nextTick()

    expect(wrapper.emitted('autoArrange')).toHaveLength(1)
    wrapper.unmount()
  })

  it('disables autoArrange when the layout is compact or a workspace save is active', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
        widgets: desktopWidgets,
        hiddenWidgetKeys: [],
        canAutoArrange: false,
      },
    })
    await nextTick()

    const action = document.body.querySelector<HTMLButtonElement>('.desktop-icon-manager__layout-action')
    expect(action?.disabled).toBe(true)
    action?.click()
    expect(wrapper.emitted('autoArrange')).toBeUndefined()

    await wrapper.setProps({ canAutoArrange: true, busy: true })
    expect(action?.disabled).toBe(true)
    action?.click()
    expect(wrapper.emitted('autoArrange')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows a file target as a removable desktop reference without an edit action', async () => {
    const shortcut = {
      id: 'a'.repeat(32),
      name: 'nginx.conf',
      description: '',
      targetType: 'file' as const,
      path: '/etc/nginx/nginx.conf',
      createdAt: '2026-08-14T00:00:00Z',
      updatedAt: '2026-08-14T00:00:00Z',
    }
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [shortcut],
        widgets: desktopWidgets,
        hiddenWidgetKeys: [],
        canAutoArrange: true,
      },
    })
    await nextTick()

    expect(document.body.textContent).toContain('/etc/nginx/nginx.conf')
    expect(document.body.querySelector('.desktop__shortcut-artwork--file')).not.toBeNull()
    expect(document.body.querySelector('.desktop__shortcut-link-badge')).toBeNull()
    expect(document.body.querySelector('[aria-label="编辑快捷方式"]')).toBeNull()
    const remove = document.body.querySelector<HTMLButtonElement>('[aria-label="从桌面移除"]')
    expect(remove).not.toBeNull()
    remove?.click()
    await nextTick()

    expect(wrapper.emitted('remove')?.[0]).toEqual([shortcut])
    wrapper.unmount()
  })
})

// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopIconManagerDialog from './DesktopIconManagerDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('DesktopIconManagerDialog', () => {
  it('places custom shortcuts first and the layout action after the managed sections', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
        canAutoArrange: true,
      },
    })
    await nextTick()

    const manager = document.body.querySelector<HTMLElement>('.desktop-icon-manager')
    const children = Array.from(manager?.children || [])

    expect(children[1]?.textContent).toContain('自定义快捷方式')
    expect(children[2]?.textContent).toContain('已从桌面移除')
    expect(children[3]?.classList.contains('desktop-icon-manager__layout-action')).toBe(true)
    expect(children[3]?.textContent).toContain('自动整理图标')
    wrapper.unmount()
  })

  it('emits autoArrange from the layout action when wide-layout editing is available', async () => {
    const wrapper = mount(DesktopIconManagerDialog, {
      attachTo: document.body,
      props: {
        open: true,
        hiddenEntries: [],
        shortcuts: [],
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
        canAutoArrange: true,
      },
    })
    await nextTick()

    expect(document.body.textContent).toContain('/etc/nginx/nginx.conf')
    expect(document.body.querySelector('[aria-label="编辑快捷方式"]')).toBeNull()
    const remove = document.body.querySelector<HTMLButtonElement>('[aria-label="从桌面移除"]')
    expect(remove).not.toBeNull()
    remove?.click()
    await nextTick()

    expect(wrapper.emitted('remove')?.[0]).toEqual([shortcut])
    wrapper.unmount()
  })
})

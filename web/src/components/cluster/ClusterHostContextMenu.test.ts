// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import ClusterHostContextMenu, { clusterHostMenuGroups, type ClusterHostMenuRequest } from './ClusterHostContextMenu.vue'
import type { ClusterHost } from '@/types/api'

function host(overrides: Partial<ClusterHost> = {}): ClusterHost {
  return {
    id: 'host-1',
    name: '香港8+8',
    isLocal: false,
    kind: 'panel',
    terminalAvailable: true,
    fileManagementAvailable: true,
    polling: false,
    ...overrides,
  } as ClusterHost
}

const actions = (target: ClusterHost, address = 'https://hk.example.com') =>
  clusterHostMenuGroups(target, address).map((group) => group.map((item) => item.action))

describe('clusterHostMenuGroups', () => {
  it('offers the panel, tools, host actions and removal for a remote panel host', () => {
    expect(actions(host())).toEqual([
      ['open-panel'],
      ['history', 'terminal', 'files'],
      ['refresh', 'manage', 'copy-address'],
      ['remove'],
    ])
  })

  it('labels the local panel as current and never offers to remove it', () => {
    const groups = clusterHostMenuGroups(host({ isLocal: true }), 'https://local.example.com')
    expect(groups[0]![0]).toEqual({ action: 'open-panel', label: '当前面板' })
    expect(groups.flat().map((item) => item.action)).not.toContain('remove')
  })

  it('hides the panel entry for a light node and tools it cannot serve', () => {
    expect(actions(host({ kind: 'light_node', terminalAvailable: false, fileManagementAvailable: false }), '203.0.113.9'))
      .toEqual([['history'], ['refresh', 'manage', 'copy-address'], ['remove']])
    expect(actions(host({ kind: 'light_node', terminalAvailable: true, fileManagementAvailable: false }), ''))
      .toEqual([['history', 'terminal'], ['refresh', 'manage'], ['remove']])
  })

  it('shows files only when the host explicitly reports file management', () => {
    expect(actions(host({ fileManagementAvailable: undefined })).flat()).not.toContain('files')
    expect(actions(host({ terminalAvailable: false })).flat()).not.toContain('terminal')
  })

  it('disables refresh while a poll is already running', () => {
    const refresh = clusterHostMenuGroups(host({ polling: true }), '').flat().find((item) => item.action === 'refresh')
    expect(refresh?.disabled).toBe(true)
  })
})

describe('ClusterHostContextMenu', () => {
  afterEach(() => { document.body.innerHTML = '' })

  function request(overrides: Partial<ClusterHostMenuRequest> = {}): ClusterHostMenuRequest {
    return { host: host(), x: 40, y: 60, anchor: null, origin: 'pointer', address: 'https://hk.example.com', ...overrides }
  }

  async function openMenu(overrides: Partial<ClusterHostMenuRequest> = {}) {
    const wrapper = mount(ClusterHostContextMenu, { attachTo: document.body })
    await (wrapper.vm as unknown as { open: (value: ClusterHostMenuRequest) => Promise<void> }).open(request(overrides))
    await nextTick()
    return wrapper
  }

  const menu = () => document.body.querySelector<HTMLElement>('[role="menu"]')
  const items = () => [...document.body.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]

  it('renders a named menu with the host title and a separated danger item last', async () => {
    const wrapper = await openMenu()
    expect(menu()?.getAttribute('aria-label')).toContain('香港8+8')
    expect(menu()?.querySelector('.cluster-host-menu__title')?.textContent).toBe('香港8+8')
    expect(items().map((item) => item.textContent?.trim())).toEqual(['打开面板', '历史监控', '终端', '文件', '刷新', '管理', '复制地址', '移除主机'])
    expect(items().at(-1)?.classList.contains('k-context-menu__item--danger')).toBe(true)
    expect(menu()?.querySelectorAll('hr')).toHaveLength(3)
    wrapper.unmount()
  })

  it('emits the chosen action with the host and closes', async () => {
    const wrapper = await openMenu()
    items().find((item) => item.textContent?.includes('终端'))!.click()
    await nextTick()

    expect(wrapper.emitted('select')).toEqual([['terminal', expect.objectContaining({ id: 'host-1' })]])
    expect(menu()).toBeNull()
    wrapper.unmount()
  })

  it('ignores a disabled item', async () => {
    const wrapper = await openMenu({ host: host({ polling: true }) })
    const refresh = items().find((item) => item.textContent?.includes('刷新'))!
    expect(refresh.disabled).toBe(true)
    refresh.click()
    expect(wrapper.emitted('select')).toBeUndefined()
    wrapper.unmount()
  })

  it('moves focus with the arrow keys and wraps', async () => {
    const wrapper = await openMenu({ origin: 'keyboard' })
    expect(document.activeElement).toBe(items()[0])
    menu()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(document.activeElement).toBe(items()[1])
    menu()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }))
    expect(document.activeElement).toBe(items().at(-1))
    menu()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(document.activeElement).toBe(items()[0])
    wrapper.unmount()
  })

  it('closes on Escape and returns focus to the button that opened it', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    const wrapper = await openMenu({ opener, origin: 'keyboard' })
    menu()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    await nextTick()

    expect(menu()).toBeNull()
    expect(document.activeElement).toBe(opener)
    wrapper.unmount()
  })

  it('closes on an outside press but not on the opener press that toggles it', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    const wrapper = await openMenu({ opener })

    opener.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await nextTick()
    expect(menu()).not.toBeNull()

    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await nextTick()
    expect(menu()).toBeNull()
    wrapper.unmount()
  })

  it('exposes which host owns the open menu', async () => {
    const wrapper = await openMenu()
    const exposed = wrapper.vm as unknown as { isOpen: boolean; hostId: string | undefined }
    expect(exposed.isOpen).toBe(true)
    expect(exposed.hostId).toBe('host-1')
    wrapper.unmount()
  })
})

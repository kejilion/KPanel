// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick, ref } from 'vue'
import FileMediaMenu from './FileMediaMenu.vue'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'

let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = '' })

describe('file media menu', () => {
  it('opens a single menu and emits only the chosen media destination', async () => {
    wrapper = mount(FileMediaMenu, { attachTo: document.body })
    expect(document.querySelector('[role="menu"]')).toBeNull()
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true')
    const items = document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    expect([...items].map(item => item.textContent)).toEqual(['图库', '音乐', '影视'])
    items[1]!.click(); await nextTick()
    expect(wrapper.emitted('music')).toHaveLength(1)
    expect(wrapper.emitted('gallery')).toBeUndefined()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    await wrapper.get('button').trigger('click')
    document.querySelector<HTMLButtonElement>('[role="menuitem"]')!.click(); await nextTick()
    expect(wrapper.emitted('gallery')).toHaveLength(1)
  })
  it('supports arrow navigation and restores trigger focus on Escape', async () => {
    wrapper = mount(FileMediaMenu, { attachTo: document.body })
    await wrapper.get('button').trigger('keydown', { key: 'ArrowDown' })
    const items = document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    expect(document.activeElement).toBe(items[0])
    items[0]!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(document.activeElement).toBe(items[1])
    items[1]!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(wrapper.get('button').element)
  })
  it('dismisses on outside interaction, window hiding and unmount without orphaning the popup', async () => {
    const visible = ref(true)
    wrapper = mount(FileMediaMenu, { attachTo: document.body, global: { provide: { [desktopWindowVisibleKey as symbol]: visible } } })
    await wrapper.get('button').trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })); await nextTick()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    await wrapper.get('button').trigger('click'); visible.value = false; await nextTick()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    visible.value = true; await nextTick(); await wrapper.get('button').trigger('click')
    wrapper.unmount(); wrapper = undefined
    expect(document.querySelector('[role="menu"]')).toBeNull()
  })
})

// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DesktopGroupCard from './DesktopGroupCard.vue'
import DesktopGroupPreviewIcon from './DesktopGroupPreviewIcon.vue'

describe('collapsed desktop group previews', () => {
  const editable = (rename = vi.fn(async () => true)) => mount(DesktopGroupCard, {
    attachTo: document.body,
    props: { group: { id: 'a', name: '原名称', members: [], columns: 4, collapsed: false },
      count: 0, dropping: false, busy: false, cells: [], rename },
  })

  it('edits on a single name click and saves once with trimmed text and restored keyboard focus', async () => {
    const rename = vi.fn(async () => true)
    const wrapper = editable(rename)
    await wrapper.get('.desktop-group__name').trigger('click')
    const input = wrapper.get('input')
    expect(document.activeElement).toBe(input.element)
    await input.setValue('  新名称  ')
    await input.trigger('keydown', { key: 'Enter' })
    await input.trigger('blur')
    await flushPromises()
    expect(rename).toHaveBeenCalledExactlyOnceWith('新名称')
    expect(wrapper.find('input').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('.desktop-group__name').element)
    wrapper.unmount()
  })

  it('cancels empty/unchanged names and Escape without a write', async () => {
    const rename = vi.fn(async () => true)
    const wrapper = editable(rename)
    for (const value of ['', '   ', '原名称']) {
      await wrapper.get('.desktop-group__name').trigger('click')
      await wrapper.get('input').setValue(value)
      await wrapper.get('input').trigger('blur')
      expect(wrapper.find('input').exists()).toBe(false)
    }
    await wrapper.get('.desktop-group__name').trigger('click')
    const input = wrapper.get('input')
    await input.setValue('不要保存')
    await input.trigger('keydown', { key: 'Escape' })
    await input.trigger('blur')
    expect(rename).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not save or move while composing, then supports blur-save', async () => {
    const rename = vi.fn(async () => true)
    const wrapper = editable(rename)
    await wrapper.get('.desktop-group__name').trigger('click')
    const input = wrapper.get('input')
    await input.setValue('中文名称')
    await input.trigger('compositionstart')
    await input.trigger('keydown', { key: 'Enter', isComposing: true })
    await input.trigger('keydown', { key: 'ArrowLeft', ctrlKey: true })
    expect(rename).not.toHaveBeenCalled()
    expect(wrapper.emitted('nudge')).toBeUndefined()
    await input.trigger('compositionend')
    await input.trigger('blur')
    await flushPromises()
    expect(rename).toHaveBeenCalledExactlyOnceWith('中文名称')
    wrapper.unmount()
  })

  it('keeps a failed rename editable and allows retry without losing the draft', async () => {
    const rename = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true)
    const wrapper = editable(rename)
    await wrapper.setProps({ error: '保存失败' })
    await wrapper.get('.desktop-group__name').trigger('click')
    await wrapper.get('input').setValue('保留草稿')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('保留草稿')
    expect(wrapper.get('input').attributes('aria-invalid')).toBe('true')
    expect(wrapper.get('[role="alert"]').text()).toBe('保存失败')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(rename).toHaveBeenCalledTimes(2)
    expect(wrapper.find('input').exists()).toBe(false)
    wrapper.unmount()
  })

  it('provides a dismissible keyboard menu instead of a configuration form', async () => {
    const wrapper = editable()
    await wrapper.get('.desktop-group__menu').trigger('click')
    await flushPromises()
    const menu = document.querySelector<HTMLElement>('.desktop-group__actions')!
    const buttons = menu.querySelectorAll('button')
    expect(buttons).toHaveLength(2)
    expect(document.activeElement).toBe(buttons[0])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(document.activeElement).toBe(buttons[1])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(document.querySelector('.desktop-group__actions')).toBeNull()
    expect(document.activeElement).toBe(wrapper.get('.desktop-group__menu').element)
    await wrapper.get('.desktop-group__menu').trigger('click')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('.desktop-group__actions button')!.click()
    await flushPromises()
    expect(wrapper.find('input').exists()).toBe(true)
    wrapper.unmount()
    expect(document.querySelector('.desktop-group__actions')).toBeNull()
  })

  it('shows decorative previews only when collapsed without adding tab stops', async () => {
    const group = { id: 'a', name: '工具与设置', members: ['nav:/settings'], columns: 3, collapsed: true }
    const wrapper = mount(DesktopGroupCard, {
      props: { group, count: 1, dropping: false, busy: false, cells: [] },
      slots: { preview: '<span class="preview-test">icon</span>' },
    })
    const previews = wrapper.get('.desktop-group__previews')
    expect(previews.attributes('aria-hidden')).toBe('true')
    expect(previews.attributes('inert')).toBeDefined()
    expect(previews.find('button, a, [tabindex]').exists()).toBe(false)
    await wrapper.get('.desktop-group__toggle').trigger('click')
    expect(wrapper.emitted('toggle')).toHaveLength(1)
    await wrapper.setProps({ group: { ...group, collapsed: false } })
    expect(wrapper.find('.preview-test').exists()).toBe(false)
    wrapper.unmount()
  })

  it('falls back on failed images and retries when the source changes', async () => {
    const wrapper = mount(DesktopGroupPreviewIcon, { props: { label: '工具', iconURL: '/bad.webp' } })
    expect(wrapper.get('img').attributes('draggable')).toBe('false')
    expect(wrapper.get('img').attributes('referrerpolicy')).toBe('no-referrer')
    await wrapper.get('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toBe('工')
    await wrapper.setProps({ iconURL: '/new.webp' })
    expect(wrapper.get('img').attributes('src')).toBe('/new.webp')
    wrapper.unmount()
  })
})

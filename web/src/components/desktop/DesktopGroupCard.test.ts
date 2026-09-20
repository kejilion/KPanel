// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DesktopGroupCard from './DesktopGroupCard.vue'
import DesktopGroupPreviewIcon from './DesktopGroupPreviewIcon.vue'

describe('collapsed desktop group previews', () => {
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

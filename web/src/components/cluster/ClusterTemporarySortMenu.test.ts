// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it } from 'vitest'
import ClusterTemporarySortMenu from './ClusterTemporarySortMenu.vue'

describe('ClusterTemporarySortMenu', () => {
  const options = [
    { value: 'custom' as const, label: '自定义顺序' },
    { value: 'cpu' as const, label: 'CPU 使用率' },
    { value: 'memory' as const, label: '内存使用率' },
    { value: 'disk' as const, label: '磁盘使用率' },
    { value: 'traffic' as const, label: '总流量（收发合计）' },
  ]

  it('uses a themed listbox instead of a native select and emits the chosen key', async () => {
    const wrapper = mount(ClusterTemporarySortMenu, {
      props: { modelValue: 'custom', options, label: '临时排序方式', prefix: '临时排序' },
    })

    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.get('.cluster-temporary-sort-menu__trigger').attributes('aria-label'))
      .toBe('临时排序方式：自定义顺序')
    await wrapper.get('.cluster-temporary-sort-menu__trigger').trigger('click')
    expect(wrapper.get('[role="listbox"]').attributes('aria-label')).toBe('临时排序方式')
    expect(wrapper.get('[data-value="custom"]').attributes('aria-selected')).toBe('true')

    await wrapper.get('[data-value="memory"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([['memory']])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('supports arrow navigation and restores trigger focus on Escape', async () => {
    const wrapper = mount(ClusterTemporarySortMenu, {
      attachTo: document.body,
      props: { modelValue: 'cpu', options, label: '临时排序方式', prefix: '临时排序' },
    })
    const trigger = wrapper.get<HTMLButtonElement>('.cluster-temporary-sort-menu__trigger')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('[data-value="cpu"]').element)

    await wrapper.get('[role="listbox"]').trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(wrapper.get('[data-value="memory"]').element)

    await wrapper.get('[role="listbox"]').trigger('keydown', { key: 'Escape' })
    await nextTick()
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })
})

// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterRemainingValue from './ClusterRemainingValue.vue'
import type { ClusterHost, ClusterHostDetails } from '@/types/api'

const wrappers: ReturnType<typeof mount>[] = []
const hosts = [{ id: 'one', name: '<img src=x>主机' }, { id: 'two', name: 'second' }] as ClusterHost[]
function render(details: Record<string, ClusterHostDetails> = {}) {
  const wrapper = mount(ClusterRemainingValue, { props: { hosts, details }, global: { stubs: {
    ModalDialog: { props: ['open'], template: '<section v-if="open" role="dialog"><slot/><slot name="footer"/></section>' },
  } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 0, 1, 12)) })
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })

describe('remaining value summary and details', () => {
  it('opens an honest empty state with a path to edit and escapes host names', async () => {
    const wrapper = render()
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('—')
    await wrapper.get('.cluster-value__trigger').trigger('click')
    expect(wrapper.get('[role="status"]').text()).toContain('尚无可估算')
    expect(wrapper.text()).toContain('0 / 2 台')
    expect(wrapper.text()).toContain('<img src=x>主机')
    expect(wrapper.find('img').exists()).toBe(false)
    await wrapper.get('.cluster-value__result button').trigger('click')
    expect(wrapper.emitted('manage')?.[0]?.[0]).toEqual(hosts[0])
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('makes other currencies visible instead of adding unlike amounts', async () => {
    const wrapper = render({
      one: { price: '¥30/月', expiresOn: '2026-01-16' },
      two: { price: '$30/月', expiresOn: '2026-01-11' },
    })
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('¥15.00')
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('CNY · 另 1 种币种')
    await wrapper.get('.cluster-value__trigger').trigger('click')
    expect(wrapper.findAll('.cluster-value__group')).toHaveLength(2)
    expect(wrapper.text()).toContain('US$10.00')
    expect(wrapper.text()).toContain('2 / 2 台')
    await wrapper.setProps({ details: { one: { price: '¥30/月' } } })
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('—')
    expect(wrapper.text()).toContain('未填写到期日期')
  })

  it('updates on a calendar day change and releases its timer on unmount', async () => {
    vi.setSystemTime(new Date(2026, 0, 1, 23, 59, 30))
    const wrapper = render({ one: { price: '$30/月', expiresOn: '2026-01-02' } })
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('US$1.00')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(wrapper.get('.cluster-value__trigger').text()).toContain('US$0.00')
    await wrapper.get('.cluster-value__trigger').trigger('click')
    expect(wrapper.text()).toContain('已到期 · 剩余价值为 0')
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})

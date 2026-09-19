// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitoringChecksDialog from './MonitoringChecksDialog.vue'

const mocks = vi.hoisted(() => {
  class ApiError extends Error {
    code: string
    constructor(code: string, message = code) {
      super(message)
      this.code = code
    }
  }
  return { checks: vi.fn(), updateChecks: vi.fn(), ApiError }
})

vi.mock('@/lib/api', () => ({
  ApiError: mocks.ApiError,
  api: { monitoring: { checks: mocks.checks, updateChecks: mocks.updateChecks } },
}))
vi.mock('@/i18n/phrase', () => ({ phraseCatalogVersion: { value: 0 }, translatePhrase: (value: string) => value }))

const version = `sha256:${'a'.repeat(64)}`
const snapshot = {
  schemaVersion: 1, resourceVersion: version, available: true, maxItems: 16,
  items: [{ id: 'telecom-beijing', kind: 'ping' as const, name: '电信 · 北京', target: '219.141.136.10', operator: 'telecom', region: 'beijing' }],
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.checks.mockResolvedValue(snapshot)
  mocks.updateChecks.mockImplementation(async (input) => ({ ...snapshot, resourceVersion: `sha256:${'b'.repeat(64)}`, items: input.items }))
})

describe('MonitoringChecksDialog', () => {
  it('edits the unified list and saves one versioned replacement', async () => {
    const wrapper = mount(MonitoringChecksDialog, { attachTo: document.body, props: { open: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.get('.check-editor input').element).toHaveProperty('value', '电信 · 北京')

    const list = wrapper.get<HTMLElement>('.check-manager__list')
    list.element.scrollTop = 120
    await wrapper.findAll('button').find((button) => button.text().includes('添加检测项'))!.trigger('click')
    await flushPromises()
    const editors = wrapper.findAll('.check-editor')
    expect(editors).toHaveLength(2)
    expect(editors[0]!.get('input').element).toHaveProperty('value', '')
    expect(editors[1]!.get('input').element).toHaveProperty('value', '电信 · 北京')
    expect(editors[0]!.classes()).toContain('check-editor--ping')
    expect(list.element.scrollTop).toBe(0)
    expect(document.activeElement).toBe(editors[0]!.get('input').element)
    await editors[0]!.get('select').setValue('tcp')
    await flushPromises()
    expect(wrapper.findAll('.check-editor')[0]!.classes()).toContain('check-editor--tcp')
    const inputs = editors[0]!.findAll('input')
    await inputs[0]!.setValue('官网 TLS')
    await inputs[1]!.setValue('example.com:443')
    await wrapper.findAll('button').find((button) => button.text().includes('保存变更'))!.trigger('click')
    await flushPromises()

    expect(mocks.updateChecks).toHaveBeenCalledTimes(1)
    expect(mocks.updateChecks.mock.calls[0]?.[0]).toMatchObject({
      expectedResourceVersion: version,
      items: [
        expect.objectContaining({ kind: 'tcp', name: '官网 TLS', target: 'example.com:443' }),
        expect.objectContaining({ id: 'telecom-beijing' }),
      ],
    })
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('allows deleting every item and persists an intentionally empty list', async () => {
    const wrapper = mount(MonitoringChecksDialog, { props: { open: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.get('.check-editor__delete').trigger('click')
    expect(wrapper.text()).toContain('暂无检测项')
    await wrapper.findAll('button').find((button) => button.text().includes('保存变更'))!.trigger('click')
    await flushPromises()
    expect(mocks.updateChecks).toHaveBeenCalledWith({ expectedResourceVersion: version, items: [] })
  })

  it('reloads after a resource-version conflict', async () => {
    mocks.updateChecks.mockRejectedValueOnce(new mocks.ApiError('monitoring_checks_changed'))
    const wrapper = mount(MonitoringChecksDialog, { props: { open: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('保存变更'))!.trigger('click')
    await flushPromises()
    expect(mocks.checks).toHaveBeenCalledTimes(2)
  })
})

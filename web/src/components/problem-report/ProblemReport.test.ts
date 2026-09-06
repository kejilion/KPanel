// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProblemReportButton from './ProblemReportButton.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import { resetLocaleForTest, setLocale } from '@/i18n'
import en from '@/i18n/pages/ProblemReport/en-US'
import tw from '@/i18n/pages/ProblemReport/zh-TW'
import zh from '@/i18n/pages/ProblemReport/zh-CN'

const clipboard = vi.fn()
const objectURL = vi.fn(() => 'blob:local-report')
const revoke = vi.fn()
const global = { stubs: { ModalDialog: { props: ['open'], template: '<section v-if="open"><slot /><slot name="footer" /></section>' } } }
let wrapper: ReturnType<typeof mount> | undefined
beforeEach(() => {
  resetLocaleForTest()
  clipboard.mockReset().mockResolvedValue(undefined)
  objectURL.mockClear()
  revoke.mockClear()
  Object.defineProperty(navigator, 'clipboard', { value: { writeText: clipboard }, configurable: true })
  Object.defineProperty(URL, 'createObjectURL', { value: objectURL, configurable: true })
  Object.defineProperty(URL, 'revokeObjectURL', { value: revoke, configurable: true })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
})
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })
async function open() {
  wrapper = mount(ProblemReportButton, { props: { source: 'manual', feature: 'settings' }, global })
  await wrapper.get('.problem-report-open').trigger('click')
  return wrapper
}
function preview() { return (wrapper!.get('[data-report-preview]').element as HTMLTextAreaElement).value }

describe('problem report interactions', () => {
  it('creates no clipboard or download output until an explicit action', async () => {
    const view = await open()
    await view.get('[data-report-input="actual"]').setValue('The button did nothing')
    expect(clipboard).not.toHaveBeenCalled()
    expect(objectURL).not.toHaveBeenCalled()
    expect(JSON.parse(preview()).actual).toBe('The button did nothing')
  })
  it('copies exactly the preview after deleting diagnostic fields and editing text', async () => {
    const view = await open()
    await view.get('[data-report-field="capturedAt"]').setValue(false)
    await view.get('[data-report-input="expected"]').setValue('password=seed-secret')
    const value = preview()
    expect(value).not.toContain('seed-secret')
    expect(JSON.parse(value)).not.toHaveProperty('capturedAt')
    await view.get('[data-report-copy]').trigger('click')
    await flushPromises()
    expect(clipboard).toHaveBeenCalledWith(value)
    expect(view.get('[role="status"]').text()).toBe(zh.copied)
    await view.get('[data-report-input="actual"]').setValue('changed')
    expect(view.find('[role="status"]').exists()).toBe(false)
  })
  it('offers an explicit download after clipboard failure without automatically creating it', async () => {
    clipboard.mockRejectedValue(new Error('denied'))
    const view = await open()
    await view.get('[data-report-copy]').trigger('click')
    await flushPromises()
    expect(view.get('[role="status"]').text()).toBe(zh.copyFailed)
    expect(objectURL).not.toHaveBeenCalled()
    await view.get('[data-report-download]').trigger('click')
    expect(objectURL).toHaveBeenCalledOnce()
    expect(revoke).toHaveBeenCalledWith('blob:local-report')
    expect(view.get('[role="status"]').text()).toBe(zh.downloaded)
  })
  it('keeps a manually selectable preview when both browser output methods fail', async () => {
    const view = await open()
    objectURL.mockImplementationOnce(() => { throw new Error('unavailable') })
    await view.get('[data-report-download]').trigger('click')
    expect(view.get('[role="status"]').text()).toBe(zh.downloadFailed)
    expect(preview()).toContain('kpanel-local-problem-report/v1')
  })
  it('discards a closed draft and ignores the old copy completion after reopening', async () => {
    let finish!: () => void
    clipboard.mockImplementationOnce(() => new Promise<void>((resolve) => { finish = resolve }))
    const view = await open()
    await view.get('[data-report-input="actual"]').setValue('old draft')
    await view.get('[data-report-copy]').trigger('click')
    await view.findAll('button').find((button) => button.text() === zh.close)!.trigger('click')
    await view.get('.problem-report-open').trigger('click')
    finish()
    await flushPromises()
    expect(preview()).not.toContain('old draft')
    expect(view.find('[role="status"]').exists()).toBe(false)
  })
  it('preserves ErrorState retry and excludes its original message from report data', async () => {
    wrapper = mount(ErrorState, { props: { message: 'secret-error-body', reportError: { code: 'network_error', requestId: 'a'.repeat(32) } }, global })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    await wrapper.get('.problem-report-open').trigger('click')
    expect(preview()).not.toContain('secret-error-body')
    expect(JSON.parse(preview()).requestId).toBe('a'.repeat(32))
  })
  it('provides matching private language keys and switches the dialog without an API request', async () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
    expect(Object.keys(tw).sort()).toEqual(Object.keys(zh).sort())
    const view = await open()
    await setLocale('en-US', false)
    await flushPromises()
    expect(view.get('[data-report-copy]').text()).toBe(en.copy)
    await setLocale('zh-TW', false)
    await flushPromises()
    expect(view.get('[data-report-copy]').text()).toBe(tw.copy)
  })
})

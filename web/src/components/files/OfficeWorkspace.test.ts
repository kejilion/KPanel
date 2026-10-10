// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OfficeWorkspace from './OfficeWorkspace.vue'
import { resetLocaleForTest, setLocale } from '@/i18n'
import type { FileEntry, OfficeDocument } from '@/types/api'

const mocks = vi.hoisted(() => ({ office: vi.fn(), writeOffice: vi.fn() }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error { code = 'file_conflict' },
  api: { files: mocks },
}))
const entry: FileEntry = { name: 'demo.docx', path: '/demo.docx', kind: 'file', sizeBytes: 12,
  mode: '644', owner: 'demo', group: 'demo', modifiedAt: '', resourceVersion: 'v1', editable: false, previewable: true, officeEditable: true }
function document(): OfficeDocument { return { entry: { ...entry }, kind: 'docx', contentVersion: 'a'.repeat(64), notes: ['basic_layout'],
  sections: [{ name: '', items: [{ id: 'p1', kind: 'text', text: 'Original', editable: true }] }] } }
const wrappers: ReturnType<typeof mount>[] = []
async function setup() { const wrapper = mount(OfficeWorkspace, { props: { entry, hostId: 'remote-a' } }); wrappers.push(wrapper); await flushPromises(); return wrapper }
beforeEach(() => { vi.clearAllMocks(); resetLocaleForTest(); mocks.office.mockImplementation(async () => document()); mocks.writeOffice.mockResolvedValue({ entry: { ...entry, resourceVersion: 'v2' } }) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); resetLocaleForTest() })

describe('lightweight Office workspace', () => {
  it('saves a draft against its host and content versions and reloads confirmed content', async () => {
    const wrapper = await setup()
    await wrapper.get('.office-paragraph').trigger('click')
    await wrapper.get('.office-inspector input').setValue('Changed')
    expect(wrapper.get('.office-paragraph').text()).toBe('Changed')
    const changed = document(); changed.entry.resourceVersion = 'v2'; changed.sections[0]!.items[0]!.text = 'Changed'
    mocks.office.mockResolvedValueOnce(changed)
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    expect(mocks.writeOffice).toHaveBeenCalledWith('/demo.docx', [{ id: 'p1', text: 'Changed' }], 'v1', 'a'.repeat(64), 'remote-a')
    expect(wrapper.get('.office-paragraph').text()).toBe('Changed')
    expect(wrapper.emitted('dirty')).toEqual([[true], [false]])
    expect(wrapper.emitted('saving')).toEqual([[true], [false]])
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('keeps drafts after a failed save and supports Ctrl+S', async () => {
    const wrapper = await setup(); await wrapper.get('.office-paragraph').trigger('click')
    await wrapper.get('.office-inspector input').setValue('Keep me')
    mocks.writeOffice.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.get('.office-inspector input').trigger('keydown', { key: 's', ctrlKey: true }); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('草稿')
    expect((wrapper.get('.office-inspector input').element as HTMLInputElement).value).toBe('Keep me')
    expect(wrapper.get('.office-save').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
  it('requires reload after a successful write whose readback failed', async () => {
    const wrapper = await setup(); await wrapper.get('.office-paragraph').trigger('click'); await wrapper.get('.office-inspector input').setValue('Saved')
    mocks.office.mockRejectedValueOnce(new Error('readback unavailable'))
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('已保存')
    expect(wrapper.get('.office-save').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.office-inspector input').attributes('readonly')).toBeDefined()
  })
  it('does not show a late response from a previous host and clears stale content on failure', async () => {
    let resolve: (doc: OfficeDocument) => void = () => undefined
    mocks.office.mockReturnValueOnce(new Promise<OfficeDocument>(r => { resolve = r }))
    const wrapper = mount(OfficeWorkspace, { props: { entry, hostId: 'old' } }); wrappers.push(wrapper)
    mocks.office.mockRejectedValueOnce(new Error('new host unavailable'))
    await wrapper.setProps({ hostId: 'new' }); await flushPromises(); resolve(document()); await flushPromises()
    expect(wrapper.find('.office-paper').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(mocks.office).toHaveBeenLastCalledWith('/demo.docx', 'new', expect.any(AbortSignal))
  })
  it('shows sparse worksheets within a bounded grid and keeps formulas read only', async () => {
    const doc = document(); doc.kind = 'xlsx'; doc.sections[0] = { name: 'Data', rows: 1048576, columns: 16384,
      items: [{ id: 'formula', kind: 'cell', text: '24', formula: '=B1*2', editable: false, row: 1, column: 1 }] }
    mocks.office.mockResolvedValueOnce(doc)
    const wrapper = await setup()
    expect(wrapper.findAll('.office-grid td')).toHaveLength(480)
    await wrapper.get('.office-grid td button:not(:disabled)').trigger('click')
    expect(wrapper.get('.office-inspector textarea').attributes('readonly')).toBeDefined()
    expect(wrapper.get('code').text()).toBe('=B1*2')
  })
  it('localizes UI while treating file content as literal text', async () => {
    const doc = document(); doc.sections[0]!.items[0]!.text = '<script>文件</script>'; mocks.office.mockResolvedValueOnce(doc)
    const wrapper = await setup(); await setLocale('en-US'); await flushPromises()
    expect(wrapper.text()).toContain('Lightweight Office')
    expect(wrapper.get('.office-paragraph').text()).toBe('<script>文件</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.get('.office-paragraph span').attributes('data-i18n-ignore')).toBeDefined()
  })
  it('guards refresh while dirty or saving and removes its listener on unmount', async () => {
    const wrapper = await setup()
    const clean = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(clean); expect(clean.defaultPrevented).toBe(false)
    await wrapper.get('.office-paragraph').trigger('click')
    await wrapper.get('.office-inspector input').setValue('Unsaved')
    const dirty = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(dirty); expect(dirty.defaultPrevented).toBe(true)
    let finish: (value: { entry: FileEntry }) => void = () => undefined
    mocks.writeOffice.mockReturnValueOnce(new Promise<{ entry: FileEntry }>(resolve => { finish = resolve }))
    await wrapper.get('.office-save').trigger('click')
    const saving = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(saving); expect(saving.defaultPrevented).toBe(true)
    finish({ entry }); await flushPromises()
    const saved = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(saved); expect(saved.defaultPrevented).toBe(false)
    await wrapper.get('.office-paragraph').trigger('click')
    await wrapper.get('.office-inspector input').setValue('Another draft')
    wrapper.unmount(); wrappers.splice(wrappers.indexOf(wrapper), 1)
    const closed = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(closed); expect(closed.defaultPrevented).toBe(false)
  })
})

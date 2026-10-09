// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OfficeWorkspace from './OfficeWorkspace.vue'
import { resetLocaleForTest, setLocale } from '@/i18n'
import type { FileEntry, OfficeDocument } from '@/types/api'

const mocks = vi.hoisted(() => ({ office: vi.fn(), writeOffice: vi.fn() }))
const clipboard = vi.hoisted(() => ({ copyText: vi.fn(async () => true) }))
vi.mock('@/lib/clipboard', () => clipboard)
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error { code = 'file_conflict' },
  api: { files: mocks },
}))
const entry: FileEntry = { name: 'demo.docx', path: '/demo.docx', kind: 'file', sizeBytes: 12,
  mode: '644', owner: 'demo', group: 'demo', modifiedAt: '', resourceVersion: 'v1', editable: false, previewable: true, officeEditable: true }
function document(): OfficeDocument { return { entry: { ...entry }, kind: 'docx', contentVersion: 'a'.repeat(64), notes: ['basic_layout'],
  sections: [{ name: '', items: [{ id: 'p1', kind: 'text', text: 'Original', editable: true }, { id: 'p2', kind: 'text', text: 'Locked', editable: false }] }] } }
function sheet(): OfficeDocument {
  const doc = document(); doc.kind = 'xlsx'
  doc.sections = [{ name: 'Data', rows: 3, columns: 3, items: [
    { id: 'a1', kind: 'cell', text: 'Name', editable: true, row: 1, column: 1 },
    { id: 'b1', kind: 'cell', text: '2', editable: true, row: 1, column: 2 },
    { id: 'c1', kind: 'cell', text: '4', formula: '=B1*2', editable: false, row: 1, column: 3 },
    { id: 'b2', kind: 'cell', text: '5', editable: true, row: 2, column: 2 },
  ] }, { name: 'Notes', rows: 1, columns: 1, items: [{ id: 'n1', kind: 'cell', text: 'note', editable: true, row: 1, column: 1 }] }]
  return doc
}
function slides(): OfficeDocument {
  const doc = document(); doc.kind = 'pptx'
  doc.sections = [1, 2].map(n => ({ name: String(n), width: 12192000, height: 6858000, items: [
    { id: `t${n}`, kind: 'text', text: `Title ${n}`, editable: true, x: 914400, y: 914400, width: 9144000, height: 1371600, fontSize: 32, bold: true },
  ] }))
  return doc
}
const wrappers: ReturnType<typeof mount>[] = []
async function setup(doc = document()) {
  mocks.office.mockResolvedValueOnce(doc)
  const wrapper = mount(OfficeWorkspace, { props: { entry, hostId: 'remote-a' }, attachTo: window.document.body })
  wrappers.push(wrapper); await flushPromises(); return wrapper
}
async function edit(wrapper: ReturnType<typeof mount>, text: string, index = 0) {
  await wrapper.findAll('.office-paragraph .office-block__text')[index]!.trigger('click')
  await flushPromises()
  await wrapper.get('.office-text-editor').setValue(text)
}
const paragraph = (wrapper: ReturnType<typeof mount>, index = 0) => wrapper.findAll('.office-paragraph [data-office-text]')[index]!.text()
beforeEach(() => {
  vi.clearAllMocks(); resetLocaleForTest(); localStorage.clear()
  mocks.office.mockImplementation(async () => document()); mocks.writeOffice.mockResolvedValue({ entry: { ...entry, resourceVersion: 'v2' } })
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); resetLocaleForTest() })

describe('lightweight Office workspace', () => {
  it('edits text in place, saves against host and content versions and reloads confirmed content', async () => {
    const wrapper = await setup()
    await edit(wrapper, 'Changed')
    await wrapper.get('.office-text-editor').trigger('keydown', { key: 'Enter' })
    expect(wrapper.find('.office-text-editor').exists()).toBe(false)
    expect(paragraph(wrapper)).toBe('Changed')
    expect(wrapper.get('.office-status').text()).toContain('1 处未保存修改')
    const changed = document(); changed.entry.resourceVersion = 'v2'; changed.sections[0]!.items[0]!.text = 'Changed'
    mocks.office.mockResolvedValueOnce(changed)
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    expect(mocks.writeOffice).toHaveBeenCalledWith('/demo.docx', [{ id: 'p1', text: 'Changed' }], 'v1', 'a'.repeat(64), 'remote-a')
    expect(paragraph(wrapper)).toBe('Changed')
    expect(wrapper.get('.office-status').text()).toContain('已保存')
    expect(wrapper.emitted('dirty')).toEqual([[true], [false]])
    expect(wrapper.emitted('saving')).toEqual([[true], [false]])
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('keeps drafts after a failed save and supports Ctrl+S while editing', async () => {
    const wrapper = await setup(); await edit(wrapper, 'Keep me')
    mocks.writeOffice.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.get('.office-text-editor').trigger('keydown', { key: 's', ctrlKey: true }); await flushPromises()
    expect(wrapper.get('.office-alert').text()).toContain('草稿')
    expect(paragraph(wrapper)).toBe('Keep me')
    expect(wrapper.get('.office-save').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
  it('requires reload after a successful write whose readback failed and locks editing', async () => {
    const wrapper = await setup(); await edit(wrapper, 'Saved')
    mocks.office.mockRejectedValueOnce(new Error('readback unavailable'))
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    expect(wrapper.get('.office-alert').text()).toContain('已保存')
    expect(wrapper.get('.office-save').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.office-reload').attributes('disabled')).toBeUndefined()
    await wrapper.get('.office-paragraph .office-block__text').trigger('click')
    expect(wrapper.find('.office-text-editor').exists()).toBe(false)
  })
  it('does not show a late response from a previous host and offers recovery on failure', async () => {
    let resolve: (doc: OfficeDocument) => void = () => undefined
    mocks.office.mockReturnValueOnce(new Promise<OfficeDocument>(r => { resolve = r }))
    const wrapper = mount(OfficeWorkspace, { props: { entry, hostId: 'old' } }); wrappers.push(wrapper)
    mocks.office.mockRejectedValueOnce(new Error('new host unavailable'))
    await wrapper.setProps({ hostId: 'new' }); await flushPromises(); resolve(document()); await flushPromises()
    expect(wrapper.find('.office-page').exists()).toBe(false)
    expect(wrapper.get('.office-state--error').text()).toContain('无法打开此文档')
    expect(mocks.office).toHaveBeenLastCalledWith('/demo.docx', 'new', expect.any(AbortSignal))
    await wrapper.findAll('.office-state__actions button')[1]!.trigger('click')
    expect(wrapper.emitted('download')).toHaveLength(1)
  })
  it('cancels only the current edit with Escape without closing the surrounding dialog', async () => {
    const wrapper = await setup()
    await edit(wrapper, 'First'); await wrapper.get('.office-text-editor').trigger('keydown', { key: 'Enter' })
    const outer = vi.fn(); window.addEventListener('keydown', outer)
    await edit(wrapper, 'Second')
    await wrapper.get('.office-text-editor').trigger('keydown', { key: 'Escape' })
    window.removeEventListener('keydown', outer)
    expect(outer).not.toHaveBeenCalled()
    expect(paragraph(wrapper)).toBe('First')
  })
  it('keeps Word text on one line, ignores IME Enter and explains read-only content', async () => {
    const wrapper = await setup(); await edit(wrapper, 'a\nb\tc')
    const field = wrapper.get('.office-text-editor').element as HTMLTextAreaElement
    expect(field.value).toBe('a b c')
    await wrapper.get('.office-text-editor').trigger('keydown', { key: 'Enter', isComposing: true })
    expect(wrapper.find('.office-text-editor').exists()).toBe(true)
    await wrapper.get('.office-text-editor').trigger('blur')
    expect(paragraph(wrapper)).toBe('a b c')
    await wrapper.findAll('.office-paragraph .office-block__text')[1]!.trigger('click')
    expect(wrapper.find('.office-text-editor').exists()).toBe(false)
    expect(wrapper.get('.office-statusbar__hint').text()).toContain('仅支持查看')
  })
  it('lists pending changes with location and supports undo and discard', async () => {
    const wrapper = await setup(); await edit(wrapper, 'Draft'); await wrapper.get('.office-text-editor').trigger('blur')
    await wrapper.get('.office-changes-toggle').trigger('click'); await flushPromises()
    const change = wrapper.get('.office-change')
    expect(change.text()).toContain('第 1 段')
    expect(change.get('.office-change__after').text()).toBe('Draft')
    expect(change.get('.office-change__before').text()).toContain('Original')
    await change.findAll('.office-icon-button')[0]!.trigger('click'); await flushPromises()
    expect(clipboard.copyText).toHaveBeenCalledWith('Draft')
    expect(change.findAll('.office-icon-button')[0]!.attributes('aria-label')).toBe('已复制')
    await change.findAll('.office-icon-button')[1]!.trigger('click'); await flushPromises()
    expect(paragraph(wrapper)).toBe('Original')
    expect(wrapper.find('.office-changes').exists()).toBe(false)
    expect(window.document.activeElement).toBe(wrapper.get('.office-reload').element)
    await edit(wrapper, 'Again'); await wrapper.get('.office-text-editor').trigger('blur')
    await wrapper.get('.office-changes-toggle').trigger('click'); await flushPromises()
    await wrapper.get('.office-changes__footer button').trigger('click'); await flushPromises()
    expect(paragraph(wrapper)).toBe('Original')
    expect(wrapper.emitted('dirty')?.at(-1)).toEqual([false])
  })
  it('keeps the Word page after saving a long document', async () => {
    const doc = document(); doc.sections[0]!.items = Array.from({ length: 100 }, (_, i) => ({ id: `p${i}`, kind: 'text' as const, text: `P${i}`, editable: true }))
    const wrapper = await setup(structuredClone(doc))
    expect(wrapper.get('.office-pager').text()).toContain('1 / 2')
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await edit(wrapper, 'Changed'); await wrapper.get('.office-text-editor').trigger('blur')
    mocks.office.mockResolvedValueOnce(structuredClone(doc))
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    expect(mocks.writeOffice).toHaveBeenCalledWith('/demo.docx', [{ id: 'p80', text: 'Changed' }], 'v1', 'a'.repeat(64), 'remote-a')
    expect(wrapper.get('.office-pager').text()).toContain('2 / 2')
  })
  it('offers review and discard actions after a save conflict', async () => {
    const { ApiError } = await import('@/lib/api')
    const wrapper = await setup(); await edit(wrapper, 'Mine'); await wrapper.get('.office-text-editor').trigger('blur')
    mocks.writeOffice.mockRejectedValueOnce(new ApiError('conflict'))
    await wrapper.get('.office-save').trigger('click'); await flushPromises()
    const actions = wrapper.findAll('.office-alert__actions button')
    expect(actions.map(button => button.text())).toEqual(['查看修改', '放弃修改并重新读取', ''])
    await actions[1]!.trigger('click'); await flushPromises()
    expect(paragraph(wrapper)).toBe('Original')
    expect(mocks.office).toHaveBeenCalledTimes(2)
  })
  it('remembers a dismissed preview notice', async () => {
    const wrapper = await setup()
    expect(wrapper.find('.office-notice').exists()).toBe(true)
    await wrapper.get('.office-notice button').trigger('click')
    expect(wrapper.find('.office-notice').exists()).toBe(false)
    const again = await setup()
    expect(again.find('.office-notice').exists()).toBe(false)
    await again.get('.office-tool--icon').trigger('click')
    expect(again.find('.office-notice').exists()).toBe(true)
  })
  it('shows sparse worksheets within a bounded grid and keeps formulas read only', async () => {
    const doc = document(); doc.kind = 'xlsx'; doc.sections[0] = { name: 'Data', rows: 1048576, columns: 16384,
      items: [{ id: 'formula', kind: 'cell', text: '24', formula: '=B1*2', editable: false, row: 1, column: 1 }] }
    const wrapper = await setup(doc)
    expect(wrapper.findAll('.office-grid td')).toHaveLength(480)
    await wrapper.get('[data-office-id="formula"]').trigger('mousedown')
    const bar = wrapper.get('.office-formula-input')
    expect(bar.attributes('readonly')).toBeDefined()
    expect((bar.element as HTMLTextAreaElement).value).toBe('=B1*2')
    expect(wrapper.get('.office-chip').text()).toContain('公式')
    expect(wrapper.get('.office-sheet-nav__range').text()).toContain('1,048,576')
  })
  it('navigates sheets by keyboard, edits by typing and jumps from the name box', async () => {
    const wrapper = await setup(sheet())
    const grid = wrapper.get('.office-grid')
    await grid.trigger('keydown', { key: 'c', ctrlKey: true })
    expect(clipboard.copyText).toHaveBeenCalledWith('Name')
    expect(wrapper.find('.office-cell-editor').exists()).toBe(false)
    await grid.trigger('keydown', { key: 'ArrowRight' })
    expect((wrapper.get('.office-name-box').element as HTMLInputElement).value).toBe('B1')
    await grid.trigger('keydown', { key: '7' }); await flushPromises()
    const editor = wrapper.get('.office-cell-editor')
    expect((editor.element as HTMLTextAreaElement).value).toBe('7')
    expect((wrapper.get('.office-formula-input').element as HTMLTextAreaElement).value).toBe('7')
    await editor.trigger('keydown', { key: 'Enter' }); await flushPromises()
    expect((wrapper.get('.office-name-box').element as HTMLInputElement).value).toBe('B2')
    expect(wrapper.get('[data-office-id="b1"]').classes()).toContain('is-modified')
    await grid.trigger('keydown', { key: 'Delete' })
    expect(wrapper.get('[data-office-id="b2"] .office-cell__text').text()).toBe('')
    await wrapper.get('.office-name-box').setValue('zz9')
    await wrapper.get('.office-name-box').trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('.office-statusbar__hint').text()).toContain('B12')
    await wrapper.get('.office-name-box').setValue('c1')
    await wrapper.get('.office-name-box').trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('[data-office-id="c1"]').attributes('aria-selected')).toBe('true')
    await grid.trigger('keydown', { key: '9' })
    expect(wrapper.find('.office-cell-editor').exists()).toBe(false)
    await wrapper.findAll('.office-sheet-tab')[1]!.trigger('click')
    expect(wrapper.find('[data-office-id="n1"]').exists()).toBe(true)
    expect((wrapper.get('.office-name-box').element as HTMLInputElement).value).toBe('A1')
  })
  it('switches slides from thumbnails and the keyboard and edits slide text in place', async () => {
    const wrapper = await setup(slides())
    expect(wrapper.findAll('.office-thumb')).toHaveLength(2)
    expect(wrapper.get('.office-thumb').attributes('aria-label')).toContain('Title 1')
    await wrapper.findAll('.office-thumb')[1]!.trigger('click')
    expect(wrapper.get('.office-pager').text()).toContain('2 / 2')
    await wrapper.get('.office-stage').trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('.office-pager').text()).toContain('1 / 2')
    const box = wrapper.get('.office-textbox')
    expect(box.attributes('style')).toContain('--office-font: 0.03333')
    await box.get('.office-block__text').trigger('click'); await flushPromises()
    await wrapper.get('.office-text-editor').setValue('New title')
    await wrapper.get('.office-text-editor').trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('.office-textbox [data-office-text]').text()).toBe('New title')
    expect(wrapper.find('.office-thumb .office-thumb__text i').exists()).toBe(true)
  })
  it('localizes UI while treating file content as literal text', async () => {
    const doc = document(); doc.sections[0]!.items[0]!.text = '<script>文件</script>'
    const wrapper = await setup(doc); await setLocale('en-US'); await flushPromises()
    expect(wrapper.text()).toContain('Word document')
    expect(paragraph(wrapper)).toBe('<script>文件</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.get('.office-paragraph [data-office-text]').attributes('data-i18n-ignore')).toBeDefined()
  })
  it('guards refresh while dirty or saving and removes its listener on unmount', async () => {
    const wrapper = await setup()
    const clean = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(clean); expect(clean.defaultPrevented).toBe(false)
    await edit(wrapper, 'Unsaved')
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
    await edit(wrapper, 'Another draft')
    wrapper.unmount(); wrappers.splice(wrappers.indexOf(wrapper), 1)
    const closed = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(closed); expect(closed.defaultPrevented).toBe(false)
  })
})

// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { EditorView } from '@codemirror/view'
import { undo, redo } from '@codemirror/commands'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CodeEditor from './CodeEditor.vue'

const wrappers: ReturnType<typeof mount>[] = []
beforeEach(() => {
  Range.prototype.getClientRects = () => [] as unknown as DOMRectList
  Range.prototype.getBoundingClientRect = () => new DOMRect()
})
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.restoreAllMocks()
})
describe('CodeEditor document sessions', () => {
  it('restores the document, selection and undo history and rebinds events to the current instance', async () => {
    const props = {
      modelValue: 'original text',
      fileName: 'notes.txt',
      sizeBytes: 13,
      autoFocus: false,
    }
    const firstChange = vi.fn()
    const first = mount(CodeEditor, {
      props: { ...props, onChange: firstChange },
      attachTo: document.body,
    })
    wrappers.push(first)
    await vi.waitFor(() => expect(first.emitted('ready')).toHaveLength(1))
    const firstView = EditorView.findFromDOM(first.get<HTMLElement>('.cm-content').element)!
    firstView.dispatch({ changes: { from: 0, to: 8, insert: 'edited' }, selection: { anchor: 6 } })
    const session = first.vm.getSession()!
    expect(session.state.doc.toString()).toBe('edited text')
    expect(firstChange).toHaveBeenCalledOnce()
    first.unmount()
    wrappers.splice(0, 1)
    const second = mount(CodeEditor, {
      props: { ...props, session, lineWrap: true },
      attachTo: document.body,
    })
    wrappers.push(second)
    await vi.waitFor(() => expect(second.emitted('ready')).toHaveLength(1))
    const secondView = EditorView.findFromDOM(second.get<HTMLElement>('.cm-content').element)!
    expect(second.vm.getValue()).toBe('edited text')
    expect(secondView.state.selection.main.head).toBe(6)
    expect(secondView.lineWrapping).toBe(true)
    expect(undo(secondView)).toBe(true)
    expect(second.vm.getValue()).toBe('original text')
    expect(redo(secondView)).toBe(true)
    expect(second.vm.getValue()).toBe('edited text')
    expect(firstChange).toHaveBeenCalledOnce()
    expect(second.emitted('change')).toHaveLength(2)
  })
})

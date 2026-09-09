// @vitest-environment jsdom
import { EditorSelection, EditorState } from '@codemirror/state'
import { EditorView, type MouseSelectionStyle } from '@codemirror/view'
import { describe, expect, it } from 'vitest'
import { selectableLineNumbers } from './code-editor-line-selection'

// Use real state/ranges and the public gutter event bridge; geometry belongs
// to browser verification. One synthetic pixel represents one logical line.
function setup(doc = 'one\ntwo\nthree\nfour', selection = EditorSelection.cursor(0)) {
  const extensions = selectableLineNumbers()
  let state = EditorState.create({ doc, selection: EditorSelection.create([selection]), extensions })
  const contentDOM = document.createElement('div')
  const view = {
    get state() { return state },
    contentDOM, documentTop: 0,
    lineBlockAtHeight: (y: number) => state.doc.line(Math.max(1, Math.min(state.doc.lines, y))),
  } as unknown as EditorView
  let style: MouseSelectionStyle | null = null
  contentDOM.addEventListener('mousedown', event => {
    for (const create of state.facet(EditorView.mouseSelectionStyle)) {
      style = create(view, event)
      if (style) break
    }
  })
  // lineNumbers returns its public configuration facet as its first extension.
  const gutter = extensions[1] as unknown as [{ value: {
    domEventHandlers: { mousedown: (view: EditorView, line: unknown, event: Event) => boolean }
  } }]
  return {
    view,
    start(line: number, button = 0) {
      return gutter[0].value.domEventHandlers.mousedown(view, {}, new MouseEvent('mousedown', { clientY: line, button }))
    },
    select(line: number, extend = false, multiple = false) {
      if (!style) throw new Error('gutter gesture was not handled')
      return style.get(new MouseEvent('mousemove', { clientY: line }), extend, multiple)
    },
    map() {
      const previous = state
      const transaction = state.update({ changes: { from: 0, insert: 'new\n' } })
      state = transaction.state
      style?.update({ docChanged: true, changes: transaction.changes, startState: previous, state } as Parameters<MouseSelectionStyle['update']>[0])
    },
  }
}

describe('line number selection', () => {
  it('selects a whole line including its newline and leaves adjacent text intact on deletion', () => {
    const editor = setup()
    expect(editor.start(2)).toBe(true)
    const selection = editor.select(2)
    expect(editor.view.state.sliceDoc(selection.main.from, selection.main.to)).toBe('two\n')
    const selected = editor.view.state.update({ selection }).state
    expect(selected.update(selected.replaceSelection('')).state.doc.toString()).toBe('one\nthree\nfour')
  })

  it('drags both directions and extends from the original logical anchor', () => {
    const editor = setup()
    editor.start(3)
    const backwards = editor.select(1)
    expect([backwards.main.anchor, backwards.main.head]).toEqual([14, 0])
    const extended = setup(undefined, backwards.main)
    extended.start(4)
    expect(extended.select(4, true).main).toMatchObject({ from: 8, to: 18 })
  })

  it('adds disjoint whole lines without losing earlier ranges', () => {
    const editor = setup(undefined, EditorSelection.range(0, 4))
    editor.start(3)
    const selection = editor.select(3, false, true)
    expect(selection.ranges.map(range => editor.view.state.sliceDoc(range.from, range.to))).toEqual(['one\n', 'three\n'])
  })

  it('handles blank lines, last lines and empty documents', () => {
    for (const [doc, line, expected] of [['one\n\nlast', 2, '\n'], ['one\nlast', 2, 'last'], ['', 1, '']] as const) {
      const editor = setup(doc)
      editor.start(line)
      const { from, to } = editor.select(line).main
      expect(editor.view.state.sliceDoc(from, to)).toBe(expected)
    }
  })

  it('maps the drag anchor when the document changes', () => {
    const editor = setup()
    editor.start(2)
    editor.map()
    const { from, to } = editor.select(4).main
    expect(editor.view.state.sliceDoc(from, to)).toBe('two\nthree\n')
  })

  it('leaves right-click and ordinary content selection alone', () => {
    const editor = setup()
    expect(editor.start(2, 2)).toBe(false)
    for (const create of editor.view.state.facet(EditorView.mouseSelectionStyle)) {
      expect(create(editor.view, new MouseEvent('mousedown'))).toBeNull()
    }
  })
})

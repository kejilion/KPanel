import { EditorSelection, EditorState } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'

/** Route gutter gestures through CodeMirror's selection lifecycle (including
 * drag autoscroll, document mapping and teardown), selecting logical lines. */
export function selectableLineNumbers() {
  const gestures = new WeakMap<MouseEvent, number>()
  return [
    EditorState.allowMultipleSelections.of(true),
    lineNumbers({
      domEventHandlers: {
        mousedown(view, line, event) {
          if (!(event instanceof MouseEvent) || event.button !== 0) return false
          const gesture = new MouseEvent('mousedown', {
            bubbles: true, cancelable: true, button: 0, buttons: 1, detail: 3,
            clientX: event.clientX, clientY: event.clientY,
            shiftKey: event.shiftKey, ctrlKey: event.ctrlKey, metaKey: event.metaKey,
          })
          gestures.set(gesture, line.from)
          view.contentDOM.dispatchEvent(gesture)
          event.preventDefault()
          return true
        },
      },
    }),
    EditorView.mouseSelectionStyle.of((view, event) => {
      if (!gestures.has(event)) return null
      const initialEvent = event
      let start = gestures.get(event)!
      const lineAt = (event: MouseEvent) => {
        // The gutter already owns the clicked logical line, even when focusing
        // or measuring the editor changes its layout before selection starts.
        if (event === initialEvent) return view.state.doc.lineAt(start)
        // This public geometry read flushes pending layout before documentTop
        // is used; first focus can otherwise mix old padding with new blocks.
        view.lineBlockAtHeight(0)
        const block = view.lineBlockAtHeight(event.clientY - view.documentTop)
        return view.state.doc.lineAt(block.from)
      }
      let original = view.state.selection
      return {
        update(update) {
          if (update.docChanged) {
            start = update.changes.mapPos(start)
            original = original.map(update.changes)
          }
        },
        get(event, extend, multiple) {
          const { doc } = view.state
          // A backwards whole-line selection anchors at the next line's start.
          const anchor = extend ? original.main.anchor - (
            original.main.anchor > original.main.head && original.main.anchor > 0
              && doc.lineAt(original.main.anchor).from === original.main.anchor ? 1 : 0
          ) : start
          const first = doc.lineAt(anchor)
          const last = lineAt(event)
          const from = Math.min(first.from, last.from)
          const to = Math.min(doc.length, Math.max(first.to, last.to) + 1)
          const range = last.number < first.number
            ? EditorSelection.range(to, from)
            : EditorSelection.range(from, to)
          return multiple ? original.addRange(range) : EditorSelection.create([range])
        },
      }
    }),
  ]
}

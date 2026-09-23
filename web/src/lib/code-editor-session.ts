import type { EditorState } from '@codemirror/state'

/** In-memory only: immutable document, history and selection, without an idle DOM view. */
export interface CodeEditorSession {
  state: EditorState
  scrollTop: number
  scrollLeft: number
}

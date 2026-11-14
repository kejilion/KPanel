// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { clipboardFiles, keepsNativeClipboard } from './systemClipboard'
import { collectExternalFiles, MAX_EXTERNAL_DROP_FILES } from './desktopExternalDrop'

function paste(data: object | null): ClipboardEvent {
  return { clipboardData: data } as ClipboardEvent
}

describe('system clipboard files', () => {
  it('captures files synchronously, names screenshots and preserves named files and bytes', async () => {
    const image = new File(['pixels'], 'image.png', { type: 'image/png' })
    const document = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    const files = [image, document]
    const result = clipboardFiles(paste({ files, items: [] }))
    files.length = 0
    expect(result).toHaveLength(2)
    expect(result[0]!.name).toMatch(/^clipboard-\d+-1\.png$/)
    expect(result[0]!.type).toBe('image/png')
    expect(await new Promise<string>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.readAsText(result[0]!)
    })).toBe('pixels')
    expect(result[1]).toBe(document)
  })

  it('falls back to file items without duplicating the files list or interpreting text and HTML', () => {
    const file = new File(['a'], 'a.png', { type: 'image/png' })
    const getAsFile = vi.fn(() => file)
    const items = [{ kind: 'string', getAsFile }, { kind: 'file', getAsFile }]
    expect(clipboardFiles(paste({ files: [], items }))).toEqual([file])
    expect(getAsFile).toHaveBeenCalledTimes(1)
    expect(clipboardFiles(paste({ files: [file], items }))).toEqual([file])
    expect(getAsFile).toHaveBeenCalledTimes(1)
    expect(clipboardFiles(paste(null))).toEqual([])
    expect(clipboardFiles(paste({ files: [], items: [], getData: () => 'C:\\image.png' }))).toEqual([])
  })

  it('reuses upload name, count and byte limits before starting any upload', () => {
    expect(() => collectExternalFiles([new File([''], '../bad.png')])).toThrow()
    expect(() => collectExternalFiles(Array.from({ length: MAX_EXTERNAL_DROP_FILES + 1 }, () => new File([''], 'a')))).toThrow()
    const large = new File([''], 'large.png')
    Object.defineProperty(large, 'size', { value: 10 * 1024 ** 3 + 1 })
    expect(() => collectExternalFiles([large])).toThrow()
    expect(collectExternalFiles([new File([''], 'empty.txt')]).totalBytes).toBe(0)
  })

  it.each(['input', 'textarea', 'select', '[contenteditable=""]', '[role="textbox"]', '.cm-editor', '.xterm', '[role="dialog"]'])('preserves native clipboard in %s and descendants', (selector) => {
    const root = document.createElement(selector.startsWith('input') ? 'input' : selector === 'textarea' ? 'textarea' : selector === 'select' ? 'select' : 'div')
    if (selector.startsWith('.')) root.className = selector.slice(1)
    if (selector.startsWith('[')) {
      const [name, value] = selector.slice(1, -1).split('=')
      root.setAttribute(name!, value!.replaceAll('"', ''))
    }
    expect(keepsNativeClipboard(root)).toBe(true)
    const child = document.createElement('span')
    root.append(child)
    expect(keepsNativeClipboard(child)).toBe(true)
    expect(keepsNativeClipboard(document.createElement('div'))).toBe(false)
  })

  it('lets a non-modal desktop window route paste to its file pane while preserving nested editors and dialogs', () => {
    const window = document.createElement('section')
    window.className = 'desktop-window'
    window.setAttribute('role', 'dialog')
    const pane = document.createElement('section')
    const input = document.createElement('input')
    const modal = document.createElement('div')
    modal.setAttribute('role', 'dialog')
    const modalChild = document.createElement('span')
    modal.append(modalChild)
    window.append(pane); pane.append(input, modal)
    expect(keepsNativeClipboard(pane)).toBe(false)
    expect(keepsNativeClipboard(input)).toBe(true)
    expect(keepsNativeClipboard(modalChild)).toBe(true)
  })
})

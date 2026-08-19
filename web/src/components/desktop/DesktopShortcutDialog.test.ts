// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopShortcutDialog from './DesktopShortcutDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

describe('DesktopShortcutDialog', () => {
  it('rejects non-HTTP destinations before emitting a save', async () => {
    const wrapper = mount(DesktopShortcutDialog, {
      attachTo: document.body,
      props: { open: true },
    })
    await nextTick()
    const inputs = document.body.querySelectorAll<HTMLInputElement>('.desktop-shortcut-form input')
    const name = Array.from(inputs).find((input) => input.type === 'text')
    const url = Array.from(inputs).find((input) => input.type === 'url')
    if (!name || !url) throw new Error('shortcut fields missing')
    name.value = '危险入口'
    name.dispatchEvent(new Event('input', { bubbles: true }))
    url.value = 'javascript:alert(1)'
    url.dispatchEvent(new Event('input', { bubbles: true }))

    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()

    expect(document.body.querySelector('[role="alert"]')?.textContent).toContain('HTTP')
    expect(wrapper.emitted('save')).toBeUndefined()
    wrapper.unmount()
  })

  it('normalizes a valid HTTPS destination and collapses description controls to one line', async () => {
    const wrapper = mount(DesktopShortcutDialog, {
      attachTo: document.body,
      props: { open: true },
    })
    await nextTick()
    const name = document.body.querySelector<HTMLInputElement>('.desktop-shortcut-form input:not([type="file"]):not([type="url"])')
    const url = document.body.querySelector<HTMLInputElement>('.desktop-shortcut-form input[type="url"]')
    const description = document.body.querySelector<HTMLTextAreaElement>('.desktop-shortcut-form textarea')
    if (!name || !url || !description) throw new Error('shortcut fields missing')
    name.value = '  文档中心  '
    name.dispatchEvent(new Event('input', { bubbles: true }))
    url.value = 'https://docs.example.com'
    url.dispatchEvent(new Event('input', { bubbles: true }))
    description.value = ' \n 团队\t手册 \r\n 快速  入口  '
    description.dispatchEvent(new Event('input', { bubbles: true }))

    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()

    expect(wrapper.emitted('save')?.[0]).toEqual([
      {
        id: '',
        name: '文档中心',
        description: '团队 手册 快速 入口',
        targetType: 'url',
        url: 'https://docs.example.com/',
      },
      undefined,
      false,
    ])
    wrapper.unmount()
  })

  it('uses a CSP-compatible data URL for a selected icon preview', async () => {
    class PreviewFileReader {
      static readonly LOADING = 1
      readonly readyState = 1
      result: string | ArrayBuffer | null = null
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      onabort: (() => void) | null = null
      abort() {
        this.onabort?.()
      }
      readAsDataURL() {
        this.result = 'data:image/png;base64,iVBORw0KGgo='
        this.onload?.()
      }
    }
    vi.stubGlobal('FileReader', PreviewFileReader)
    const wrapper = mount(DesktopShortcutDialog, {
      attachTo: document.body,
      props: { open: true },
    })
    await nextTick()
    const input = document.body.querySelector<HTMLInputElement>('.desktop-shortcut-form input[type="file"]')
    if (!input) throw new Error('icon input missing')
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'icon.png', { type: 'image/png' })
    Object.defineProperty(input, 'files', { configurable: true, value: [file] })
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()

    const preview = document.body.querySelector<HTMLImageElement>('.desktop-shortcut-form__preview img')
    expect(preview?.src).toBe('data:image/png;base64,iVBORw0KGgo=')
    expect(preview?.src.startsWith('blob:')).toBe(false)
    wrapper.unmount()
  })

  it('ignores a stale file read after the dialog closes', async () => {
    const readers: ControlledFileReader[] = []
    class ControlledFileReader {
      static readonly LOADING = 1
      readonly readyState = 1
      result: string | ArrayBuffer | null = null
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      onabort: (() => void) | null = null
      constructor() {
        readers.push(this)
      }
      abort() {
        this.onabort?.()
      }
      readAsDataURL() {}
    }
    vi.stubGlobal('FileReader', ControlledFileReader)
    const wrapper = mount(DesktopShortcutDialog, {
      attachTo: document.body,
      props: { open: true },
    })
    await nextTick()
    const input = document.body.querySelector<HTMLInputElement>('.desktop-shortcut-form input[type="file"]')
    if (!input) throw new Error('icon input missing')
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'icon.png', { type: 'image/png' })
    Object.defineProperty(input, 'files', { configurable: true, value: [file] })
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()

    await wrapper.setProps({ open: false })
    const reader = readers[0]
    if (!reader) throw new Error('preview reader missing')
    reader.result = 'data:image/png;base64,stale'
    reader.onload?.()
    await nextTick()

    expect(document.body.querySelector('.desktop-shortcut-form__preview img')).toBeNull()
    wrapper.unmount()
  })
})

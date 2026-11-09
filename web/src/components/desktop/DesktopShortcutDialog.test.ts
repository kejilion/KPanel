// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { api } from '@/lib/api'
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

async function chooseCommandType(): Promise<void> {
  const select = document.body.querySelector<HTMLSelectElement>('[data-testid="shortcut-type"]')!
  select.value = 'command'
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}
describe('terminal command shortcuts', () => {
  it('imports a command copy and saves its exact multiline content', async () => {
    const command = "printf '你好'\ndf -h"
    const spy = vi.spyOn(api.terminals, 'commands').mockResolvedValue({ schemaVersion: 1, resourceVersion: 'v1', available: true, items: [{ id: 'saved', name: '系统', command }] })
    const wrapper = mount(DesktopShortcutDialog, { attachTo: document.body, props: { open: true } })
    await chooseCommandType()
    const select = document.body.querySelector<HTMLSelectElement>('#shortcut-import')!
    select.value = 'saved'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
    await nextTick()
    expect(wrapper.emitted('save')?.[0]?.[0]).toEqual({ id: '', name: '系统', description: '', targetType: 'command', command, hostId: 'local' })
    expect(document.body.querySelector('input[type="url"]')).toBeNull()
    wrapper.unmount(); spy.mockRestore()
  })
  it('keeps manual entry available after import fails and rejects empty commands', async () => {
    const spy = vi.spyOn(api.terminals, 'commands').mockRejectedValue(new Error('offline'))
    const wrapper = mount(DesktopShortcutDialog, { attachTo: document.body, props: { open: true } })
    await chooseCommandType()
    expect(document.body.textContent).toContain('加载')
    const name = document.body.querySelector<HTMLInputElement>('input:not([type="file"])')!
    name.value = 'test'; name.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
    await nextTick()
    expect(wrapper.emitted('save')).toBeUndefined()
    const command = document.body.querySelector<HTMLTextAreaElement>('[data-testid="shortcut-command"]')!
    command.value = 'uptime'; command.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
    await nextTick()
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ targetType: 'command', command: 'uptime' })
    wrapper.unmount(); spy.mockRestore()
  })
})

it('edits commands and removes the inactive target fields when switching types', async () => {
  const spy = vi.spyOn(api.terminals, 'commands').mockResolvedValue({ schemaVersion: 1, resourceVersion: 'v1', available: true, items: [] })
  const wrapper = mount(DesktopShortcutDialog, { attachTo: document.body, props: { open: true, shortcut: { id: 'a'.repeat(32), name: 'Load', description: '', targetType: 'command', command: 'uptime', createdAt: '', updatedAt: '' } } })
  await flushPromises()
  document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
  await nextTick()
  expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ targetType: 'command', command: 'uptime' })
  expect(wrapper.emitted('save')?.[0]?.[0]).not.toHaveProperty('url')
  const type = document.body.querySelector<HTMLSelectElement>('[data-testid="shortcut-type"]')!
  type.value = 'url'; type.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
  const url = document.body.querySelector<HTMLInputElement>('input[type="url"]')!
  url.value = 'https://example.com'; url.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
  await nextTick()
  expect(wrapper.emitted('save')?.[1]?.[0]).toMatchObject({ targetType: 'url', url: 'https://example.com/' })
  expect(wrapper.emitted('save')?.[1]?.[0]).not.toHaveProperty('command')
  wrapper.unmount(); spy.mockRestore()
})

it('saves an explicitly selected remote host and sends local when switched back', async () => {
  const remote = 'b'.repeat(32)
  const hostSpy = vi.spyOn(api.cluster, 'hosts').mockResolvedValue({ items: [{ id: remote, name: 'Remote Linux', isLocal: false, terminalAvailable: true }] } as Awaited<ReturnType<typeof api.cluster.hosts>>)
  const commandSpy = vi.spyOn(api.terminals, 'commands').mockResolvedValue({ schemaVersion: 1, resourceVersion: 'v1', available: true, items: [] })
  const w = mount(DesktopShortcutDialog, { attachTo: document.body, props: { open: true, shortcut: { id: 'a'.repeat(32), name: 'Load', description: '', targetType: 'remote-command', command: 'uptime', hostId: remote, createdAt: '', updatedAt: '' } } })
  await flushPromises()
  document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
  await nextTick()
  expect(w.emitted('save')?.[0]?.[0]).toMatchObject({ targetType: 'remote-command', hostId: remote })
  document.body.querySelector<HTMLButtonElement>('.host-switcher__trigger')!.click()
  await nextTick()
  document.body.querySelector<HTMLButtonElement>('[data-host-id="local"]')!.click()
  await nextTick()
  document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
  await nextTick()
  expect(w.emitted('save')?.[1]?.[0]).toMatchObject({ targetType: 'command', hostId: 'local' })
  w.unmount(); hostSpy.mockRestore(); commandSpy.mockRestore()
})

 it('uses the shortcut form for site identity without editable destinations or commands', async () => {
   const wrapper = mount(DesktopShortcutDialog, { attachTo: document.body, props: { open: true, site: {
     id: 'a'.repeat(32), name: '自定义站点', defaultName: '默认站点', url: 'https://example.com', domain: 'example.com',
     iconURL: '/custom', defaultIconURL: '/favicon', customIcon: true,
   } } })
   await nextTick()
   expect(document.body.textContent).toContain('编辑站点')
   expect(document.body.textContent).toContain('example.com')
   expect(document.body.querySelector('input[type="url"], select, textarea')).toBeNull()
   const restore = [...document.body.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent?.includes('恢复默认图标'))!
   restore.click(); await nextTick()
   expect(document.body.querySelector('img')?.getAttribute('src')).toBe('/favicon')
   const reset = [...document.body.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent?.includes('恢复默认名称'))!
   reset.click(); await nextTick()
   document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')!.click()
   await nextTick()
   expect(wrapper.emitted('save')?.[0]).toEqual([expect.objectContaining({ name: '默认站点', url: 'https://example.com/' }), undefined, true])
   wrapper.unmount()
 })

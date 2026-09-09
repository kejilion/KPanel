// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import ModalDialog from './ModalDialog.vue'

const wrappers: VueWrapper[] = []

interface DialogMountOptions {
  props?: {
    open: boolean
    title: string
    description?: string
    closeDisabled?: boolean
  }
  slots?: Record<string, () => ReturnType<typeof h>>
}

function mountDialog(options: DialogMountOptions = {}): VueWrapper {
  const wrapper = mount(ModalDialog, {
    attachTo: document.body,
    props: options.props || { open: true, title: 'Test dialog' },
    slots: options.slots,
  })
  wrappers.push(wrapper)
  return wrapper
}

async function settleFocus(): Promise<void> {
  await nextTick()
  await nextTick()
}

function panelAt(index = 0): HTMLElement {
  return document.querySelectorAll<HTMLElement>('.modal-panel')[index]!
}

afterEach(async () => {
  for (const wrapper of wrappers.splice(0).reverse()) wrapper.unmount()
  await nextTick()
  document.body.innerHTML = ''
})

describe('ModalDialog focus management', () => {
  it('associates its visible title and description with the dialog', () => {
    mountDialog({
      props: { open: true, title: 'Remote download', description: 'Save into /home.' },
    })

    const dialog = panelAt()
    const title = document.getElementById(dialog.getAttribute('aria-labelledby') || '')
    const description = document.getElementById(dialog.getAttribute('aria-describedby') || '')
    expect(title?.textContent).toBe('Remote download')
    expect(description?.textContent).toBe('Save into /home.')
  })

  it('focuses the first operable element when opened and restores its opener when closed', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()

    const wrapper = mountDialog({
      slots: {
        default: () => h('button', { 'data-test': 'body-action' }, 'Body action'),
      },
    })
    await settleFocus()

    const firstAction = panelAt().querySelector<HTMLElement>('button')!
    expect(document.activeElement).toBe(firstAction)

    await wrapper.setProps({ open: false })
    await settleFocus()
    expect(document.activeElement).toBe(opener)
  })

  it('wraps Tab and Shift+Tab within the top dialog', async () => {
    mountDialog({
      slots: {
        default: () => h('button', { 'data-test': 'middle-action' }, 'Middle action'),
        footer: () => h('button', { 'data-test': 'last-action' }, 'Last action'),
      },
    })
    await settleFocus()

    const dialog = panelAt()
    const first = dialog.querySelector<HTMLElement>('button')!
    const last = dialog.querySelector<HTMLElement>('[data-test="last-action"]')!

    last.focus()
    const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    window.dispatchEvent(forward)
    expect(forward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)

    const backward = new KeyboardEvent('keydown', {
      key: 'Tab',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })
    window.dispatchEvent(backward)
    expect(backward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
  })

  it('blocks close actions without breaking focus containment when close is disabled', async () => {
    const wrapper = mountDialog({
      props: { open: true, title: 'Locked dialog', closeDisabled: true },
      slots: {
        default: () => h('button', { 'data-test': 'body-action' }, 'Body action'),
        footer: () => h('button', { 'data-test': 'footer-action' }, 'Footer action'),
      },
    })
    await settleFocus()

    const backdrop = document.querySelector<HTMLElement>('.modal-backdrop')!
    const dialog = panelAt()
    const closeButton = dialog.querySelector<HTMLButtonElement>('.modal-panel__actions button')!
    const first = dialog.querySelector<HTMLElement>('[data-test="body-action"]')!
    const last = dialog.querySelector<HTMLElement>('[data-test="footer-action"]')!

    expect(closeButton.disabled).toBe(true)
    expect(document.activeElement).toBe(first)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    backdrop.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    closeButton.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(wrapper.emitted('close')).toBeUndefined()

    last.focus()
    const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    window.dispatchEvent(forward)
    expect(forward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)

    const backward = new KeyboardEvent('keydown', {
      key: 'Tab',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })
    window.dispatchEvent(backward)
    expect(backward.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)

    await wrapper.setProps({ closeDisabled: false })
    closeButton.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('uses the dialog panel as the focus target when no control is tabbable', async () => {
    const outside = document.createElement('button')
    document.body.append(outside)
    const wrapper = mountDialog()
    await settleFocus()

    const dialog = panelAt()
    for (const element of dialog.querySelectorAll<HTMLElement>('button, [tabindex]')) {
      if (element !== dialog) element.tabIndex = -1
    }
    outside.focus()

    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    window.dispatchEvent(tab)
    expect(tab.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(dialog)

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
  })

  it('lets only the top nested dialog handle Escape and restores focus into its parent', async () => {
    const pageOpener = document.createElement('button')
    document.body.append(pageOpener)
    pageOpener.focus()

    const parent = mountDialog({
      props: { open: true, title: 'Parent dialog' },
      slots: {
        default: () => h('button', { 'data-test': 'nested-opener' }, 'Open nested dialog'),
      },
    })
    await settleFocus()
    const nestedOpener = panelAt().querySelector<HTMLElement>('[data-test="nested-opener"]')!
    nestedOpener.focus()

    const nested = mountDialog({
      props: { open: true, title: 'Nested dialog' },
    })
    await settleFocus()
    expect(document.activeElement).toBe(panelAt(1).querySelector('button'))

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(nested.emitted('close')).toHaveLength(1)
    expect(parent.emitted('close')).toBeUndefined()

    await nested.setProps({ open: false })
    await settleFocus()
    expect(document.activeElement).toBe(nestedOpener)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(parent.emitted('close')).toHaveLength(1)
  })

  it('does not steal focus when a non-top parent dialog closes', async () => {
    const parent = mountDialog({
      props: { open: true, title: 'Parent dialog' },
      slots: {
        default: () => h('button', { 'data-test': 'nested-opener' }, 'Open nested dialog'),
      },
    })
    await settleFocus()
    panelAt().querySelector<HTMLElement>('[data-test="nested-opener"]')!.focus()

    mountDialog({ props: { open: true, title: 'Nested dialog' } })
    await settleFocus()
    const nestedAction = panelAt(1).querySelector<HTMLElement>('button')!
    expect(document.activeElement).toBe(nestedAction)

    await parent.setProps({ open: false })
    await settleFocus()
    expect(document.activeElement).toBe(nestedAction)
  })

  it('restores the opener when an open dialog is unmounted', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const wrapper = mountDialog()
    await settleFocus()

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    await settleFocus()

    expect(document.activeElement).toBe(opener)
  })
})

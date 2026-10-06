// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), open: vi.fn() }))
vi.mock('vue-router', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => ({ path: '/terminal', query: {} }) }))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: mocks.hosts }, terminals: { open: mocks.open } }, ApiError: class extends Error {} }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName'],
  setup(_props, { expose }) {
    expose({ closeSession: async () => {}, focusTerminal() {}, scheduleResize() {}, executeCommand() { return true } })
    return () => null
  },
}) }))

const windows = { id: 'win', name: 'Windows 测试机', platform: 'windows', kind: 'light_node', state: 'offline', terminalAvailable: false }
let wrapper: ReturnType<typeof mount> | undefined

function button(text: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find((item) => item.textContent?.includes(text))
  if (!found) throw new Error(`missing button ${text}`)
  return found
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mocks.hosts.mockResolvedValue({ items: [windows, { id: 'linux', name: 'Linux 测试机', kind: 'light_node', terminalAvailable: true }] })
  mocks.open.mockResolvedValue({ sessionId: 'shell', offset: 0 })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.innerHTML = '' })

describe('withdrawn Windows light-node access', () => {
  it('does not expose Windows PowerShell, file, or RDP entry points and keeps Linux terminal access', async () => {
    wrapper = mount(TerminalView, { attachTo: document.body })
    await flushPromises()

    expect(button('Windows 测试机').disabled).toBe(true)
    expect(document.body.textContent).not.toContain('远程桌面（RDP）')
    expect(document.body.textContent).not.toContain('管理员 PowerShell')
    expect(mocks.open).not.toHaveBeenCalled()

    button('Linux 测试机').click()
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('linux', 30, 120)
  })
})

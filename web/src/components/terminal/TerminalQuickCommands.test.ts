// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { resetTerminalCommandsForTest } from '@/stores/terminalCommands'
import TerminalQuickCommands from './TerminalQuickCommands.vue'

const storedCommand = {
  id: '1'.repeat(32),
  name: '容器列表',
  command: "docker ps --format '{{.Names}}'",
}

describe('TerminalQuickCommands', () => {
  beforeEach(() => {
    resetTerminalCommandsForTest()
    vi.restoreAllMocks()
    vi.spyOn(api.terminals, 'commands').mockResolvedValue({
      schemaVersion: 1,
      resourceVersion: `sha256:${'1'.repeat(64)}`,
      available: true,
      items: [storedCommand],
    })
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('shows names without exposing command bodies in the drawer and executes the selection', async () => {
    const wrapper = mount(TerminalQuickCommands, { props: { open: true } })
    await flushPromises()

    expect(wrapper.text()).toContain('容器列表')
    expect(wrapper.text()).not.toContain('docker ps')
    expect(wrapper.find('[aria-label="编辑“容器列表”"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="删除“容器列表”"]').exists()).toBe(true)

    await wrapper.get('.terminal-quick-command__run').trigger('click')
    expect(wrapper.emitted('execute')).toEqual([[storedCommand.command]])
    wrapper.unmount()
  })

  it('disables execution when the active terminal has finished', async () => {
    const wrapper = mount(TerminalQuickCommands, { props: { open: true, disabled: true } })
    await flushPromises()

    expect(wrapper.get('.terminal-quick-command__run').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[aria-label="添加命令"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('adds, edits, and deletes favorites through panel-owned updates', async () => {
    let revision = 1
    const update = vi.spyOn(api.terminals, 'updateCommands').mockImplementation(async (body) => {
      revision += 1
      return {
        schemaVersion: 1,
        resourceVersion: `sha256:${String(revision).repeat(64)}`,
        available: true,
        items: body.items,
      }
    })
    const wrapper = mount(TerminalQuickCommands, {
      attachTo: document.body,
      props: { open: true },
    })
    await flushPromises()

    await wrapper.get('[aria-label="编辑“容器列表”"]').trigger('click')
    const editName = document.body.querySelector<HTMLInputElement>('.terminal-command-form input')
    const editCommand = document.body.querySelector<HTMLTextAreaElement>('.terminal-command-form textarea')
    if (!editName || !editCommand) throw new Error('edit fields missing')
    editName.value = '容器状态'
    editName.dispatchEvent(new Event('input', { bubbles: true }))
    editCommand.value = 'docker ps -a'
    editCommand.dispatchEvent(new Event('input', { bubbles: true }))
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await flushPromises()

    expect(update.mock.calls[0]?.[0].items[0]).toMatchObject({
      id: storedCommand.id,
      name: '容器状态',
      command: 'docker ps -a',
    })
    expect(wrapper.text()).toContain('容器状态')
    expect(wrapper.text()).not.toContain('docker ps -a')

    await wrapper.get('[aria-label="删除“容器状态”"]').trigger('click')
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--danger')?.click()
    await flushPromises()
    expect(update.mock.calls[1]?.[0].items).toEqual([])

    await wrapper.get('[aria-label="添加命令"]').trigger('click')
    const addName = document.body.querySelector<HTMLInputElement>('.terminal-command-form input')
    const addCommand = document.body.querySelector<HTMLTextAreaElement>('.terminal-command-form textarea')
    if (!addName || !addCommand) throw new Error('add fields missing')
    addName.value = '系统负载'
    addName.dispatchEvent(new Event('input', { bubbles: true }))
    addCommand.value = 'uptime'
    addCommand.dispatchEvent(new Event('input', { bubbles: true }))
    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await flushPromises()

    expect(update.mock.calls[2]?.[0].items).toHaveLength(1)
    expect(update.mock.calls[2]?.[0].items[0]).toMatchObject({ name: '系统负载', command: 'uptime' })
    wrapper.unmount()
  })
})

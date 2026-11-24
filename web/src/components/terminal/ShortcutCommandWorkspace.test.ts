// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { desktopWindowLaunchKey, desktopWindowCloseGuardKey, desktopWindowTitleKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import ShortcutCommandWorkspace from './ShortcutCommandWorkspace.vue'

const mocks = vi.hoisted(() => ({ workspace: vi.fn(), hosts: vi.fn(), title: vi.fn(), list: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { terminals: { list: mocks.list }, desktop: { workspace: mocks.workspace }, cluster: { hosts: mocks.hosts } } }))
const id = 'a'.repeat(32)
const shortcut = { id, name: 'Load', targetType: 'command', command: 'uptime' }
let closeGuard: () => boolean | Promise<boolean>
const wrappers: ReturnType<typeof mount>[] = []
async function create(launch = true, visible = ref(true)) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/command-shortcut/:shortcutId', component: {} }] })
  await router.push('/command-shortcut/' + id)
  const wrapper = mount(ShortcutCommandWorkspace, { global: { plugins: [router], provide: {
    [desktopWindowTitleKey as symbol]: mocks.title,
    [desktopWindowLaunchKey as symbol]: () => launch,
    [desktopWindowVisibleKey as symbol]: visible,
    [desktopWindowCloseGuardKey as symbol]: { register: (guard: () => boolean | Promise<boolean>) => { closeGuard = guard; return () => undefined } },
  }, stubs: { ShortcutCommandTerminal: { name: 'ShortcutCommandTerminal', props: ['name', 'command', 'hostName', 'hostId', 'existingSession', 'shortcutId', 'shortcutVersion'], template: '<div data-testid="terminal">{{ command }}</div>' } } } })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
beforeEach(() => { vi.clearAllMocks(); mocks.list.mockResolvedValue({ items: [] }); mocks.workspace.mockResolvedValue({ available: true, shortcuts: [shortcut] }) })
afterEach(() => { for (const w of wrappers.splice(0)) w.unmount() })
describe('command shortcut desktop workspace', () => {
  it('loads the saved command by id and launches only for a new explicit window', async () => {
    const w = await create()
    expect(w.get('[data-testid="terminal"]').text()).toBe('uptime')
    expect(mocks.title).toHaveBeenLastCalledWith('Load · 本机 · 终端命令')
    expect(mocks.workspace).toHaveBeenCalledWith(expect.any(AbortSignal))
  })
  it('does not show the run action while a closing window is still mounted', async () => {
    const visible = ref(true)
    const w = await create(true, visible)
    w.getComponent({ name: 'ShortcutCommandTerminal' }).vm.$emit('close')
    visible.value = false
    await w.vm.$nextTick()

    expect(w.find('button').exists()).toBe(false)
  })
  it('restores idle and requires an explicit run with freshly read command data', async () => {
    const w = await create(false)
    expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
    expect(w.text()).toContain('不会自动重复执行')
    expect(w.get('button').text()).toBe('执行“Load”')
    mocks.workspace.mockResolvedValue({ available: true, shortcuts: [{ ...shortcut, command: 'df -h' }] })
    await w.get('button').trigger('click'); await flushPromises()
    expect(w.get('[data-testid="terminal"]').text()).toBe('df -h')
  })
  it('rejects deleted or unavailable shortcuts without opening a terminal', async () => {
    mocks.workspace.mockResolvedValue({ available: true, shortcuts: [] })
    const w = await create()
    expect(w.get('[role="alert"]').text()).toContain('已删除')
    expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  })
  it('recovers a failed workspace load through retry without auto-running', async () => {
    mocks.workspace.mockRejectedValueOnce(new Error('offline'))
    const w = await create()
    expect(w.get('[role="alert"]').text()).toContain('无法读取')
    await w.get('button').trigger('click'); await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  })
  it('does not launch after close while the workspace lookup is pending', async () => {
    let resolve!: (value: unknown) => void
    mocks.workspace.mockReturnValue(new Promise(done => { resolve = done }))
    const w = await create()
    expect(await closeGuard()).toBe(true)
    resolve({ available: true, shortcuts: [shortcut] }); await flushPromises()
    expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  })
})

it('opens the exact selected remote host using a fresh cluster lookup', async () => {
  const hostId = 'b'.repeat(32)
  mocks.workspace.mockResolvedValue({ available: true, shortcuts: [{ ...shortcut, targetType: 'remote-command', hostId }] })
  mocks.hosts.mockResolvedValue({ items: [{ id: hostId, name: 'Remote Linux', isLocal: false, terminalAvailable: true }] })
  const w = await create()
  expect(w.findComponent({ name: 'ShortcutCommandTerminal' }).props()).toMatchObject({ hostId, hostName: 'Remote Linux' })
  expect(mocks.hosts).toHaveBeenCalledWith(expect.any(AbortSignal))
  expect(mocks.title).toHaveBeenLastCalledWith('Load · Remote Linux · 终端命令')
  mocks.workspace.mockResolvedValue({ available: true, shortcuts: [{ ...shortcut, hostId: 'local' }] })
  await flushPromises()
  expect(mocks.title).toHaveBeenLastCalledWith('Load · Remote Linux · 终端命令')
  w.unmount()
  expect(mocks.title).toHaveBeenLastCalledWith()
})
it('never falls back to local when the saved remote host is missing or revoked', async () => {
  mocks.workspace.mockResolvedValue({ available: true, shortcuts: [{ ...shortcut, targetType: 'remote-command', hostId: 'b'.repeat(32) }] })
  mocks.hosts.mockResolvedValue({ items: [] })
  const w = await create()
  expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  expect(w.get('[role="alert"]').text()).toContain('目标主机')
})

it('rejects remote-command records without a remote id before any terminal lookup', async () => {
  mocks.workspace.mockResolvedValue({ available: true, shortcuts: [{ ...shortcut, targetType: 'remote-command' }] })
  const w = await create()
  expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  expect(w.get('[role="alert"]').text()).toContain('目标主机')
  expect(mocks.hosts).not.toHaveBeenCalled()
})

it('restores the retained shortcut session after refresh without a launch intent', async () => {
  const retained = { sessionId: 'old', hostId: 'local', shortcutId: id, retained: true, offset: 90 }
  mocks.list.mockResolvedValue({ items: [retained] })
  const w = await create(false)
  expect(w.findComponent({ name: 'ShortcutCommandTerminal' }).props('existingSession')).toEqual(retained)
})
it('does not launch when the retained session lookup fails', async () => {
  mocks.list.mockRejectedValue(new Error('offline'))
  const w = await create(true)
  expect(w.find('[data-testid="terminal"]').exists()).toBe(false)
  expect(w.find('[role="alert"]').exists()).toBe(true)
})

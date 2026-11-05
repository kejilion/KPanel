// @vitest-environment jsdom
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { desktopWindowCloseGuardKey } from '@/lib/desktopRouteKeys'
import ShortcutCommandTerminal from './ShortcutCommandTerminal.vue'
import type { TerminalSession } from '@/types/api'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ open: vi.fn(), prepareShortcut: vi.fn(), resolveShortcut: vi.fn(), preparePurpose: vi.fn(), resolvePurpose: vi.fn(), close: vi.fn(), detach: vi.fn(), markDetached: vi.fn(), closeOnUnload: vi.fn(), execute: vi.fn(), childClose: vi.fn() }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error { code = '' },
  api: { terminals: { open: mocks.open, prepareShortcut: mocks.prepareShortcut, resolveShortcut: mocks.resolveShortcut, preparePurpose: mocks.preparePurpose, resolvePurpose: mocks.resolvePurpose, close: mocks.close, detach: mocks.detach, closeOnUnload: mocks.closeOnUnload } },
}))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: { authenticated: true, user: { id: 'test-owner' } } }) }))
const HostTerminal = defineComponent({
  name: 'HostTerminal', props: ['sessionId', 'hostName', 'initialOffset', 'retained'], emits: ['stateChange'],
  setup(_, { expose }) { expose({ executeCommand: mocks.execute, closeSession: mocks.childClose, markDetached: mocks.markDetached }); return () => null },
})
const opened: TerminalSession = { sessionId: 'session-1', hostId: 'remote-1', offset: 0, createdAt: '' }
const wrappers: VueWrapper[] = []
let closeGuard: () => boolean | Promise<boolean>
function create(hostId?: string, hostName = 'Local server', retainedProps: { shortcutId?: string; shortcutVersion?: string; purpose?: 'site-management'; existingSession?: TerminalSession } = {}) {
  const wrapper = mount(ShortcutCommandTerminal, {
    props: { name: 'Load', command: 'uptime\ndf -h', hostName, hostId, ...retainedProps },
    global: { provide: { [desktopWindowCloseGuardKey as symbol]: { register: (guard: () => boolean | Promise<boolean>) => { closeGuard = guard; return () => undefined } } }, stubs: { HostTerminal, ModalDialog: { name: 'ModalDialog', props: ['closeDisabled'], emits: ['close'], template: '<div><slot /></div>' } } },
  })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  window.localStorage.clear()
  mocks.open.mockResolvedValue(opened)
  mocks.prepareShortcut.mockResolvedValue({ launchId: 'b'.repeat(64), expiresAt: '' })
  mocks.preparePurpose.mockResolvedValue({ launchId: 'b'.repeat(64), expiresAt: '' })
  mocks.resolvePurpose.mockRejectedValue(Object.assign(new ApiError('absent', 404, 'terminal_shortcut_session_not_found'), { code: 'terminal_shortcut_session_not_found' }))
  mocks.resolveShortcut.mockRejectedValue(Object.assign(new ApiError('absent', 404, 'terminal_shortcut_session_not_found'), { code: 'terminal_shortcut_session_not_found' }))
  mocks.close.mockResolvedValue({ closed: true })
  mocks.detach.mockRejectedValue(new Error('cleanup unavailable'))
  mocks.execute.mockReturnValue(true)
  mocks.childClose.mockResolvedValue(undefined)
})
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount() })

describe('shortcut command terminal lifecycle', () => {
  it('closes silently after saving an unconfirmed shortcut for cleanup', async () => {
    mocks.childClose.mockRejectedValueOnce(new Error('offline'))
    mocks.detach.mockResolvedValueOnce({ detached: true })
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32), shortcutVersion: 'v1' })
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.detach).toHaveBeenCalledWith('session-1')
    expect(mocks.markDetached).toHaveBeenCalledOnce()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it.each([false, true])('uses the fixed website entry without client command submission (legacy=%s)', async legacy => {
    if (legacy) mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('unsupported', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    mocks.open.mockResolvedValueOnce({ ...opened, hostId: 'local', retained: !legacy, purpose: 'site-management' })
    const wrapper = create('local', 'Local', { purpose: 'site-management' })
    await flushPromises()
    expect(mocks.preparePurpose).toHaveBeenCalledWith('site-management')
    expect(mocks.prepareShortcut).not.toHaveBeenCalled()
    expect(mocks.open).toHaveBeenCalledTimes(legacy ? 2 : 1)
    expect(mocks.open).toHaveBeenLastCalledWith('local', 30, 120, expect.objectContaining({ purpose: 'site-management', retained: !legacy, launchId: 'b'.repeat(64) }))
    wrapper.findComponent(HostTerminal).vm.$emit('stateChange', 'connected')
    await flushPromises()
    expect(mocks.execute).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(await closeGuard()).toBe(true)
    expect(mocks.childClose).toHaveBeenCalledTimes(1)
  })
  it('recovers a lost website launch identity when closing, without creating another command', async () => {
    mocks.open.mockRejectedValueOnce(new Error('lost response'))
    mocks.resolvePurpose.mockResolvedValueOnce({ ...opened, retained: true, purpose: 'site-management' })
    const wrapper = create('local', 'Local', { purpose: 'site-management' })
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.resolvePurpose).toHaveBeenCalledWith('site-management', 'b'.repeat(64), expect.any(AbortSignal))
    expect(mocks.close).toHaveBeenCalledWith(opened.sessionId)
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('cancels website preparation when explicitly closing before the intent response', async () => {
    let resolve!: (value: unknown) => void
    mocks.preparePurpose.mockReturnValueOnce(new Promise(done => { resolve = done }))
    create('local', 'Local', { purpose: 'site-management' })
    await flushPromises()
    const closing = closeGuard()
    resolve({ launchId: 'b'.repeat(64) })
    expect(await closing).toBe(true)
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.resolvePurpose).toHaveBeenCalledTimes(1)
  })
  it('preserves a retained website session after unmount during open', async () => {
    let resolve!: (value: TerminalSession) => void
    mocks.open.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = create('local', 'Local', { purpose: 'site-management' })
    await flushPromises()
    wrapper.unmount()
    resolve({ ...opened, retained: true, purpose: 'site-management' })
    await flushPromises()
    expect(mocks.close).not.toHaveBeenCalled()
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it('cancels a prepared intent without launching when close was requested during preparation', async () => {
    let resolve!: (value: unknown) => void
    mocks.prepareShortcut.mockReturnValueOnce(new Promise(done => { resolve = done }))
    mocks.resolveShortcut.mockRejectedValueOnce(Object.assign(new ApiError('absent', 404, 'terminal_shortcut_session_not_found'), { code: 'terminal_shortcut_session_not_found' }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    const closed = closeGuard()
    resolve({ launchId: 'b'.repeat(64) })
    expect(await closed).toBe(true)
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.resolveShortcut).toHaveBeenCalledWith('remote-1', 'a'.repeat(32), 'b'.repeat(64), expect.any(AbortSignal))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('closes a failed launch window only after the server confirms no shortcut session exists', async () => {
    mocks.open.mockRejectedValueOnce(new Error('rejected'))
    mocks.resolveShortcut.mockRejectedValueOnce(Object.assign(new ApiError('absent', 404, 'terminal_shortcut_session_not_found'), { code: 'terminal_shortcut_session_not_found' }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(mocks.close).not.toHaveBeenCalled()
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })
  it.each([false, true])('recovers a lost launch response before closing (retained=%s)', async retained => {
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('lost response', 0, 'network_error'), { code: 'network_error' }))
    mocks.resolveShortcut.mockResolvedValueOnce({ ...opened, retained })
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.resolveShortcut).toHaveBeenCalledWith('remote-1', 'a'.repeat(32), 'b'.repeat(64), expect.any(AbortSignal))
    expect(mocks.close).toHaveBeenCalledWith(opened.sessionId)
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(mocks.execute).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('closes without replaying when lost-launch lookup fails', async () => {
    mocks.open.mockRejectedValueOnce(new Error('lost response'))
    mocks.resolveShortcut.mockRejectedValueOnce(new Error('offline'))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(mocks.resolveShortcut).toHaveBeenCalledTimes(1)
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })
  it('automatically uses the saved shortcut temporarily after an explicit capability rejection', async () => {
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('old node', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    const wrapper = create('remote-1', 'Old node', { shortcutId: 'a'.repeat(32), shortcutVersion: 'v1' })
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(mocks.open).toHaveBeenLastCalledWith('remote-1', 30, 120, { retained: false, shortcutId: 'a'.repeat(32), shortcutVersion: 'v1', launchId: 'b'.repeat(64) })
    expect(mocks.prepareShortcut).toHaveBeenCalledTimes(1)
    wrapper.findComponent(HostTerminal).vm.$emit('stateChange', 'connected')
    await flushPromises()
    expect(mocks.execute).not.toHaveBeenCalled()
    expect(await closeGuard()).toBe(true)
  })
  it.each(['terminal_open_failed', 'network_error', 'unauthorized', 'terminal_limit'])('never automatically retries %s', async code => {
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('failed', 502, code), { code }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it('does not repeat a failed temporary fallback', async () => {
    mocks.open.mockRejectedValue(Object.assign(new ApiError('unsupported', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').text()).toContain('终端启动失败')
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it.each(['close', 'unmount'])('does not start a fallback after %s', async action => {
    let reject!: (reason: unknown) => void
    mocks.open.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    const closing = action === 'close' ? closeGuard() : undefined
    if (action === 'unmount') wrapper.unmount()
    reject(Object.assign(new ApiError('unsupported', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    await closing
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it('waits for and reclaims a temporary fallback when closing during its open', async () => {
    let resolve!: (value: TerminalSession) => void
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('unsupported', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    mocks.open.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = create('remote-1', 'Remote', { shortcutId: 'a'.repeat(32) })
    await flushPromises()
    const closing = closeGuard()
    expect(wrapper.emitted('close')).toHaveLength(1)
    resolve(opened)
    expect(await closing).toBe(true)
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith(opened.sessionId)
    expect(mocks.prepareShortcut).toHaveBeenCalledTimes(1)
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(mocks.execute).not.toHaveBeenCalled()
  })
  it('opens the local host, submits once after connection, and never replays on reconnect', async () => {
    const wrapper = create()
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledWith('local', 30, 120)
    expect(mocks.execute).not.toHaveBeenCalled()
    const terminal = wrapper.findComponent(HostTerminal)
    terminal.vm.$emit('stateChange', 'connected')
    terminal.vm.$emit('stateChange', 'connected')
    await flushPromises()
    terminal.vm.$emit('stateChange', 'reconnecting')
    terminal.vm.$emit('stateChange', 'connected')
    await flushPromises()
    expect(mocks.execute).toHaveBeenCalledTimes(1)
    expect(mocks.execute).toHaveBeenCalledWith('uptime\ndf -h')
  })

  it('reclaims a late session after the user closes during connection', async () => {
    let resolveOpen!: (value: TerminalSession) => void
    mocks.open.mockReturnValue(new Promise((resolve) => { resolveOpen = resolve }))
    const wrapper = create()
    await flushPromises()
    const closing = closeGuard()
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    resolveOpen(opened)
    await closing
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith(opened.sessionId)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(mocks.execute).not.toHaveBeenCalled()
  })

  it('reports open failure without submitting a command', async () => {
    mocks.open.mockRejectedValue(new Error('offline'))
    const wrapper = create()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('终端启动失败')
    expect(mocks.execute).not.toHaveBeenCalled()
  })

  it('closes locally without claiming remote termination when cleanup cannot be recorded', async () => {
    const wrapper = create()
    await flushPromises()
    mocks.childClose.mockRejectedValueOnce(new Error('offline'))
    expect(await closeGuard()).toBe(true)
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

it('reports a rejected submission and does not retry on reconnect', async () => {
  mocks.execute.mockReturnValue(false)
  const wrapper = create()
  await flushPromises()
  wrapper.findComponent(HostTerminal).vm.$emit('stateChange', 'connected')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').text()).toContain('命令未发送')
  wrapper.findComponent(HostTerminal).vm.$emit('stateChange', 'connected')
  await flushPromises()
  expect(mocks.execute).toHaveBeenCalledTimes(1)
})
it('reclaims a late open after unmount and falls back if close fails', async () => {
  let resolveOpen!: (value: TerminalSession) => void
  mocks.open.mockReturnValue(new Promise(resolve => { resolveOpen = resolve }))
  mocks.close.mockRejectedValue(new Error('offline'))
  const wrapper = create()
  wrapper.unmount()
  resolveOpen(opened)
  await flushPromises()
  expect(mocks.detach).toHaveBeenCalledWith(opened.sessionId)
  expect(mocks.execute).not.toHaveBeenCalled()
})

it('snapshots the selected host and never retargets a pending session when props change', async () => {
  let resolveOpen!: (value: TerminalSession) => void
  mocks.open.mockReturnValue(new Promise(resolve => { resolveOpen = resolve }))
  const remote = 'b'.repeat(64)
  const w = create(remote, 'Remote A')
  await w.setProps({ hostId: 'local', hostName: 'Changed local target' })
  resolveOpen(opened); await flushPromises()
  expect(mocks.open).toHaveBeenCalledOnce()
  expect(mocks.open).toHaveBeenCalledWith(remote, 30, 120)
  expect(w.findComponent({ name: 'HostTerminal' }).props('hostName')).toBe('Remote A')
  expect(w.find('[data-testid="command-target-host"]').exists()).toBe(false)
})

it('starts a saved shortcut once and stops it through the existing window close control', async () => {
  const shortcutId = 'a'.repeat(32)
  mocks.open.mockResolvedValue({ ...opened, retained: true, shortcutId })
  const wrapper = create('local', 'Local', { shortcutId, shortcutVersion: 'v1' })
  await flushPromises()
  expect(mocks.open).toHaveBeenCalledWith('local', 30, 120, { retained: true, shortcutId, shortcutVersion: 'v1', launchId: 'b'.repeat(64) })
  const child = wrapper.findComponent(HostTerminal)
  child.vm.$emit('stateChange', 'connected')
  child.vm.$emit('stateChange', 'connected')
  await flushPromises()
  expect(mocks.execute).not.toHaveBeenCalled()
  expect(wrapper.find('button').exists()).toBe(false)
  expect(await closeGuard()).toBe(true)
  wrapper.unmount()
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.childClose).toHaveBeenCalledOnce()
})
it('reattaches an existing shortcut without sending an open request or command', async () => {
  const shortcutId = 'a'.repeat(32)
  const wrapper = create('local', 'Local', { shortcutId, existingSession: { ...opened, retained: true, offset: 50, launchUncertain: true } })
  await flushPromises()
  expect(mocks.open).not.toHaveBeenCalled()
  expect(wrapper.findComponent(HostTerminal).props('initialOffset')).toBe(0)
  expect(wrapper.text()).toContain('启动结果未确认')
  mocks.childClose.mockRejectedValueOnce(new Error('offline'))
  expect(await closeGuard()).toBe(true)
  await flushPromises()
  expect(wrapper.emitted('close')).toHaveLength(1)
  expect(mocks.open).not.toHaveBeenCalled()
})

it('detaches a retained shortcut on page reload without closing or replaying it', async () => {
  const shortcutId = 'a'.repeat(32)
  mocks.open.mockResolvedValue({ ...opened, retained: true, shortcutId })
  const wrapper = create('local', 'Local', { shortcutId, shortcutVersion: 'v1' })
  await flushPromises()
  wrapper.unmount()
  expect(mocks.childClose).not.toHaveBeenCalled()
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.closeOnUnload).not.toHaveBeenCalled()
  const restored = create('local', 'Local', { shortcutId, existingSession: { ...opened, retained: true, shortcutId } })
  await flushPromises()
  expect(mocks.open).toHaveBeenCalledOnce()
  expect(restored.findComponent(HostTerminal).props('sessionId')).toBe(opened.sessionId)
  expect(mocks.execute).not.toHaveBeenCalled()
})

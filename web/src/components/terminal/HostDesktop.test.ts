// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import { ApiError } from '@/lib/api'
import HostDesktop from './HostDesktop.vue'

const mocks = vi.hoisted(() => ({ status: vi.fn(), save: vi.fn(), clear: vi.fn(), open: vi.fn(), close: vi.fn(), load: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api')>(),
  api: { desktops: { credentialStatus: mocks.status, saveCredentials: mocks.save, clearCredentials: mocks.clear, open: mocks.open, close: mocks.close, socket: (id: string) => `ws://localhost/${id}` } },
}))
vi.mock('@/lib/remoteDesktop', () => ({ loadRemoteDesktop: mocks.load }))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
function interaction() {
  const ended = deferred<void>()
  const values: Record<string, unknown> = {}
  const builder = {
    withUsername: vi.fn((value: string) => { values.username = value; return builder }),
    withPassword: vi.fn((value: string) => { values.password = value; return builder }),
    withServerDomain: vi.fn((value: string) => { values.domain = value; return builder }),
    withDestination: vi.fn(() => builder), withProxyAddress: vi.fn(() => builder), withAuthToken: vi.fn(() => builder),
    withDesktopSize: vi.fn(() => builder), withExtension: vi.fn(() => builder), build: vi.fn(() => values),
  }
  return {
    values, ended, builder, configBuilder: () => builder,
    connect: vi.fn().mockImplementation(() => pendingConnect?.promise ?? Promise.resolve({ run: () => ended.promise })),
    shutdown: vi.fn(() => ended.resolve()), setEnableClipboard: vi.fn(), setEnableAutoClipboard: vi.fn(),
    setKeyboardUnicodeMode: vi.fn(), setVisibility: vi.fn(), resize: vi.fn(), ctrlAltDel: vi.fn(),
  }
}
type Interaction = ReturnType<typeof interaction>
let views: Interaction[] = []
let emitReady = true
let pendingConnect: ReturnType<typeof deferred<{ run: () => Promise<void> }>> | undefined
const wrappers: VueWrapper[] = []
customElements.define('iron-remote-desktop', class extends HTMLElement {
  connectedCallback() {
    if (!emitReady) return
    const view = interaction()
    views.push(view)
    queueMicrotask(() => this.dispatchEvent(new CustomEvent('ready', { detail: { irgUserInteraction: view } })))
  }
})
const props = { hostId: 'windows-a', hostName: 'Windows 测试机', active: true }
async function create(hostId = props.hostId) {
  const wrapper = mount(HostDesktop, { props: { ...props, hostId }, attachTo: document.body })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
function button(wrapper: VueWrapper, text: string) {
  const result = wrapper.findAll('button').find(item => item.text() === text)
  if (!result) throw new Error(`Missing button: ${text}`)
  return result
}
async function fill(wrapper: VueWrapper, name = 'alice', secret = 'only-a-test-secret') {
  const fields = wrapper.findAll('input')
  await fields[0]!.setValue(name)
  await fields[1]!.setValue('EXAMPLE')
  await fields[2]!.setValue(secret)
}
beforeEach(() => {
  vi.clearAllMocks()
  resetLocaleForTest()
  localStorage.clear(); sessionStorage.clear()
  views = []; emitReady = true
  pendingConnect = undefined
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  mocks.status.mockResolvedValue({ saved: false })
  mocks.save.mockResolvedValue({ saved: true, username: 'alice', domain: 'EXAMPLE' })
  mocks.clear.mockResolvedValue({ saved: false })
  mocks.open.mockImplementation(async () => ({ sessionId: `desktop-${mocks.open.mock.calls.length}`, nonce: 'nonce' }))
  mocks.close.mockResolvedValue({ closed: true })
  mocks.load.mockResolvedValue({ Backend: {}, displayControl: vi.fn(), enableCredssp: vi.fn(), outboundMessageSizeLimit: vi.fn() })
})
afterEach(async () => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  await flushPromises()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('RDP saved sign-in lifecycle', () => {
  it('automatically connects the node-managed administrator without a password form or saved login', async () => {
    mocks.status.mockResolvedValue({ saved: false, managed: true })
    const credential = { username: 'kp_rdp_test', domain: 'TESTBOX', password: 'synthetic-admin-secret' }
    mocks.open.mockResolvedValue({ sessionId: 'managed-1', nonce: 'nonce', credentials: credential })
    const wrapper = await create()
    expect(mocks.open).toHaveBeenCalledWith(props.hostId, false, true)
    expect(mocks.save).not.toHaveBeenCalled()
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('专用管理员')
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
    expect(views[0]!.values).toMatchObject({ username: 'kp_rdp_test', domain: 'TESTBOX', password: 'synthetic-admin-secret' })
    expect(credential.password).toBe('')
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
    await button(wrapper, '断开并更换账户').trigger('click'); await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith('managed-1')
    expect(views[0]!.shutdown).toHaveBeenCalled()
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    expect(wrapper.find('input[type="password"]').element).toHaveProperty('value', '')
  })

  it('retries a failed managed administrator connection without requiring Windows credentials', async () => {
    mocks.status.mockResolvedValue({ saved: false, managed: true })
    mocks.open.mockRejectedValueOnce(new Error('unavailable')).mockResolvedValueOnce({ sessionId: 'managed-2', nonce: 'nonce', credentials: { username: 'kp_rdp_test', domain: 'TESTBOX', password: 'synthetic-retry' } })
    const wrapper = await create()
    expect(wrapper.text()).toContain('管理员桌面连接失败')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    await button(wrapper, '管理员一键连接').trigger('click'); await flushPromises()
    expect(mocks.open).toHaveBeenLastCalledWith(props.hostId, false, true)
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
  })

  it('revokes a managed lease that arrives after its tab closed', async () => {
    mocks.status.mockResolvedValue({ saved: false, managed: true })
    const pending = deferred<{ sessionId: string; nonce: string; credentials: { username: string; password: string } }>()
    mocks.open.mockReturnValueOnce(pending.promise)
    const wrapper = await create()
    wrapper.unmount()
    const credentials = { username: 'kp_rdp_test', password: 'synthetic-late' }
    pending.resolve({ sessionId: 'managed-late', nonce: 'nonce', credentials }); await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith('managed-late')
    expect(credentials.password).toBe('')
    expect(views).toHaveLength(0)
  })

  it.each(['resolve', 'reject'])('does not let a late %s of timeout close overwrite the retried session', async (outcome) => {
    vi.useFakeTimers()
    pendingConnect = deferred<{ run: () => Promise<void> }>()
    const pendingClose = deferred<{ closed: boolean }>()
    mocks.close.mockReturnValueOnce(pendingClose.promise)
    const wrapper = await create()
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(30_001); await flushPromises()
    pendingConnect = undefined
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
    if (outcome === 'resolve') pendingClose.resolve({ closed: true })
    else pendingClose.reject(new Error('late close failure'))
    await flushPromises()
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
    expect(wrapper.text()).not.toContain('关闭未确认')
    expect(views[1]!.shutdown).not.toHaveBeenCalled()
    expect(mocks.close).not.toHaveBeenCalledWith('desktop-2')
  })

  it('times out a stalled module load without opening a late session and permits retry', async () => {
    vi.useFakeTimers()
    const pending = deferred<Awaited<ReturnType<typeof import('@/lib/remoteDesktop').loadRemoteDesktop>>>()
    mocks.load.mockReturnValueOnce(pending.promise)
    const wrapper = await create()
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(30_001); await flushPromises()
    expect(wrapper.text()).toContain('远程桌面连接超时')
    expect(mocks.open).not.toHaveBeenCalled()
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
    pending.resolve({} as Awaited<ReturnType<typeof import('@/lib/remoteDesktop').loadRemoteDesktop>>); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(views[0]!.shutdown).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(views[0]!.shutdown).not.toHaveBeenCalled()
  })

  it('closes a timed-out negotiation and late completion never replaces the next connection', async () => {
    vi.useFakeTimers()
    pendingConnect = deferred<{ run: () => Promise<void> }>()
    const oldConnect = pendingConnect
    const wrapper = await create()
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(30_001); await flushPromises()
    expect(wrapper.text()).toContain('远程桌面连接超时')
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('desktop-1')
    expect(views[0]!.shutdown).toHaveBeenCalled()
    pendingConnect = undefined
    await fill(wrapper); await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    oldConnect.resolve({ run: () => Promise.resolve() }); await flushPromises()
    expect(wrapper.emitted('state-change')?.at(-1)).toEqual(['connected'])
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(mocks.close).not.toHaveBeenCalledWith('desktop-2')
    expect(views[1]!.shutdown).not.toHaveBeenCalled()
  })

  it('validates UTF-8 field limits before saving or opening a session', async () => {
    const wrapper = await create()
    await fill(wrapper, 'alice', '密'.repeat(342))
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('登录字段过长')
    expect(mocks.save).not.toHaveBeenCalled()
    expect(mocks.open).not.toHaveBeenCalled()
  })

  it('saves only after explicit submit, connects, and never persists or refills the password in the browser', async () => {
    const captured: unknown[] = []
    mocks.save.mockImplementation(async (...args: unknown[]) => { captured.push(structuredClone(args)); return { saved: true } })
    const storage = vi.spyOn(Storage.prototype, 'setItem')
    const wrapper = await create()
    expect(mocks.open).not.toHaveBeenCalled()
    await fill(wrapper)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(captured).toEqual([['windows-a', { username: 'alice', domain: 'EXAMPLE', password: 'only-a-test-secret' }]])
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('windows-a', false)
    expect(views[0]!.values.password).toBe('only-a-test-secret')
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('only-a-test-secret')
    expect(storage.mock.calls.flat().join('')).not.toContain('only-a-test-secret')
    expect(button(wrapper, '清除登录信息并断开').exists()).toBe(true)
    storage.mockRestore()
  })

  it('automatically uses saved credentials once per new tab; visibility changes never reconnect', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice', domain: 'EXAMPLE' })
    const returned = { username: 'alice', domain: 'EXAMPLE', password: 'saved-test-secret' }
    mocks.open.mockResolvedValue({ sessionId: 'saved-session', nonce: 'nonce', credentials: returned })
    const wrapper = await create()
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('windows-a', true)
    expect(views[0]!.values.password).toBe('saved-test-secret')
    expect(returned.password).toBe('')
    expect(wrapper.html()).not.toContain('saved-test-secret')
    await wrapper.setProps({ active: false }); await wrapper.setProps({ active: true }); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    expect(mocks.save).not.toHaveBeenCalled()
    expect(views[0]!.setVisibility).toHaveBeenLastCalledWith(true)
  })

  it('returns to an empty password form if saved credentials were invalidated before open', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockRejectedValue(new ApiError('missing', 409, 'desktop_credentials_missing'))
    const wrapper = await create()
    expect(wrapper.text()).toContain('已保存的登录信息不存在')
    expect(wrapper.get<HTMLInputElement>('input[type=password]').element.value).toBe('')
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('does not retry a failed automatic connection until the user requests it', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockRejectedValue(new Error('connection failed with untrusted detail'))
    const wrapper = await create()
    expect(wrapper.text()).toContain('远程桌面连接失败')
    expect(wrapper.text()).not.toContain('untrusted detail')
    await wrapper.setProps({ active: false }); await wrapper.setProps({ active: true }); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    await button(wrapper, '使用已保存账户连接').trigger('click'); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
  })

  it('keeps manual session-only connection available when status is unavailable', async () => {
    mocks.status.mockRejectedValue(new Error('vault unavailable'))
    const wrapper = await create()
    expect(wrapper.text()).toContain('无法读取已保存的登录信息')
    expect(wrapper.text()).not.toContain('保存并连接')
    await fill(wrapper)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mocks.save).not.toHaveBeenCalled()
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('windows-a', false)
  })

  it('offers manual sign-in when the vault becomes unavailable after a saved status', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockRejectedValue(new ApiError('vault unavailable', 503, 'desktop_credentials_unavailable'))
    const wrapper = await create()
    expect(wrapper.text()).toContain('无法读取已保存的登录信息')
    expect(wrapper.text()).not.toContain('保存并连接')
    expect(button(wrapper, '仅连接本次').exists()).toBe(true)
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('prevents clearing or changing accounts while an explicit save is still pending', async () => {
    const pending = deferred<{ saved: boolean }>()
    mocks.save.mockReturnValue(pending.promise)
    const wrapper = await create()
    await fill(wrapper)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('正在保存登录信息')
    expect(button(wrapper, '断开并更换账户').attributes('disabled')).toBeDefined()
    pending.resolve({ saved: true }); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('never opens a connection if saving fails, clears the submitted password, and allows retry', async () => {
    mocks.save.mockRejectedValue(new Error('secret must not appear'))
    const wrapper = await create()
    await fill(wrapper)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('登录信息保存失败，尚未连接')
    expect(wrapper.text()).not.toContain('secret must not appear')
    expect(wrapper.get<HTMLInputElement>('input[type=password]').element.value).toBe('')
    expect(mocks.open).not.toHaveBeenCalled()
  })

  it('changes account by closing the current RDP only, and session-only sign-in preserves saved credentials', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice', domain: 'EXAMPLE' })
    mocks.open.mockResolvedValueOnce({ sessionId: 'saved-session', nonce: 'nonce', credentials: { username: 'alice', domain: 'EXAMPLE', password: 'saved-test-secret' } })
    const wrapper = await create()
    await button(wrapper, '断开并更换账户').trigger('click'); await flushPromises()
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('saved-session')
    expect(wrapper.get<HTMLInputElement>('input[type=password]').element.value).toBe('')
    expect(wrapper.text()).toContain('仅连接本次不会替换已保存的账户')
    await fill(wrapper, 'bob', 'other-test-secret')
    await button(wrapper, '仅连接本次').trigger('click'); await flushPromises()
    expect(mocks.save).not.toHaveBeenCalled()
    expect(mocks.clear).not.toHaveBeenCalled()
    expect(mocks.open).toHaveBeenLastCalledWith('windows-a', false)
  })

  it('clears saved credentials and disconnects, then returns to the first-time form', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockResolvedValue({ sessionId: 'saved-session', nonce: 'nonce', credentials: { username: 'alice', domain: '', password: 'saved-test-secret' } })
    const wrapper = await create()
    await button(wrapper, '清除登录信息并断开').trigger('click'); await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith('saved-session')
    expect(mocks.clear).toHaveBeenCalledExactlyOnceWith('windows-a')
    expect(wrapper.get<HTMLInputElement>('input').element.value).toBe('')
    expect(wrapper.text()).not.toContain('清除登录信息')
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('retains a visible clear retry when clearing fails and does not reconnect silently', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockRejectedValue(new Error('offline'))
    mocks.clear.mockRejectedValue(new Error('vault unavailable'))
    const wrapper = await create()
    await button(wrapper, '清除登录信息').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('登录信息清除失败')
    expect(button(wrapper, '清除登录信息').exists()).toBe(true)
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('does not clear saved credentials or create another session until a failed close is confirmed', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    mocks.open.mockResolvedValue({ sessionId: 'saved-session', nonce: 'nonce', credentials: { username: 'alice', domain: '', password: 'saved-test-secret' } })
    const wrapper = await create()
    mocks.close.mockRejectedValue(new Error('close unconfirmed'))
    await button(wrapper, '清除登录信息并断开').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('关闭未确认')
    expect(mocks.clear).not.toHaveBeenCalled()
    await button(wrapper, '使用已保存账户连接').trigger('click'); await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })

  it('ignores and aborts a late status after the tab is closed', async () => {
    const pending = deferred<{ saved: boolean; username: string }>()
    mocks.status.mockReturnValue(pending.promise)
    const wrapper = await create()
    const signal = mocks.status.mock.calls[0]![1] as AbortSignal
    wrapper.unmount()
    pending.resolve({ saved: true, username: 'alice' }); await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(mocks.open).not.toHaveBeenCalled()
  })

  it('closes a late server session after unmount and clears returned secrets without connecting', async () => {
    mocks.status.mockResolvedValue({ saved: true, username: 'alice' })
    const pending = deferred<{ sessionId: string; nonce: string; credentials: { username: string; domain: string; password: string } }>()
    mocks.open.mockReturnValue(pending.promise)
    const wrapper = await create()
    wrapper.unmount()
    const returned = { sessionId: 'late', nonce: 'nonce', credentials: { username: 'alice', domain: '', password: 'late-test-secret' } }
    pending.resolve(returned); await flushPromises()
    expect(returned.credentials.password).toBe('')
    expect(mocks.close).toHaveBeenCalledWith('late')
    expect(views).toHaveLength(0)
  })

  it('cancels a pending component initialization and leaves another host tab connected', async () => {
    const first = await create('windows-a')
    await fill(first); await button(first, '仅连接本次').trigger('click'); await flushPromises()
    emitReady = false
    const second = await create('windows-b')
    await fill(second); await button(second, '仅连接本次').trigger('click'); await flushPromises()
    second.unmount(); await flushPromises()
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('desktop-2')
    expect(views[0]!.shutdown).not.toHaveBeenCalled()
    expect(first.emitted('state-change')?.at(-1)).toEqual(['connected'])
  })
})

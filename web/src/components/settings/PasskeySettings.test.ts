// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PasskeySettings from './PasskeySettings.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), configureOrigin: vi.fn(), disableOrigin: vi.fn(), registerBegin: vi.fn(), registerFinish: vi.fn(), remove: vi.fn(),
  createPasskey: vi.fn(), supported: true, reset: vi.fn(), replace: vi.fn(),
  state: { authenticated: true, user: { totpEnabled: true } as { totpEnabled: boolean } | undefined, expiresAt: 'later' as string | undefined, agent: {} as object | undefined },
}))
vi.mock('@/lib/api', () => ({ api: { auth: { passkeys: { list: mocks.list, configureOrigin: mocks.configureOrigin, disableOrigin: mocks.disableOrigin, registerBegin: mocks.registerBegin, registerFinish: mocks.registerFinish, delete: mocks.remove } } }, resetApiSecurityState: mocks.reset }))
vi.mock('@/lib/passkeys', () => ({ createPasskey: mocks.createPasskey, passkeysSupported: () => mocks.supported, passkeyError: (reason: Error) => reason.message }))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: mocks.state }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ replace: mocks.replace }) }))

let wrapper: ReturnType<typeof mount>
const credential = { id: 'old', name: 'Laptop', createdAt: '2026-09-01T12:00:00Z' }
const render = async () => { wrapper = mount(PasskeySettings); await flushPromises() }
const fillAuthentication = async () => {
  await wrapper.get('input[type="password"]').setValue('current-password')
  await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.supported = true
  mocks.state.authenticated = true
  mocks.state.user = { totpEnabled: true }
  mocks.list.mockResolvedValue({ available: true, rpId: 'panel.example.com', origin: 'https://panel.example.com', originManaged: false, credentials: [credential] })
  mocks.registerBegin.mockResolvedValue({ ceremonyId: 'registration', publicKey: { challenge: 'AA' } })
  mocks.createPasskey.mockResolvedValue({ id: 'new' })
  mocks.registerFinish.mockResolvedValue({ reauthenticate: true })
  mocks.remove.mockResolvedValue({ reauthenticate: true })
  mocks.disableOrigin.mockResolvedValue({ reauthenticate: true })
  mocks.configureOrigin.mockResolvedValue({ available: true, origin: 'https://panel.example.com', detectedOrigin: 'https://panel.example.com' })
  mocks.replace.mockResolvedValue(undefined)
})
afterEach(() => wrapper?.unmount())

describe('Passkey management', () => {
  it('offers the current trusted HTTPS entry for one-time confirmation', async () => {
    mocks.list.mockResolvedValueOnce({ available: false, rpId: '', detectedOrigin: 'https://panel.example.com', configurable: true, credentials: [] })
      .mockResolvedValueOnce({ available: true, rpId: 'panel.example.com', origin: 'https://panel.example.com', configurable: false, credentials: [] })
    await render()
    expect(wrapper.text()).toContain('检测到当前 HTTPS 入口')
    await wrapper.get('button.button--secondary').trigger('click')
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.configureOrigin).toHaveBeenCalledWith({ password: 'current-password', totpCode: '123456' }, expect.any(AbortSignal))
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(mocks.state.authenticated).toBe(true)
  })

  it('requires password and enabled second factor, then clears all local session state after registration', async () => {
    await render()
    await wrapper.get('button.button--primary').trigger('click')
    await wrapper.get('input[maxlength="64"]').setValue('Phone')
    await wrapper.get('input[type="password"]').setValue('current-password')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.registerBegin).toHaveBeenCalledWith({ name: 'Phone', password: 'current-password', totpCode: '123456' }, expect.any(AbortSignal))
    expect(mocks.registerFinish).toHaveBeenCalledWith({ ceremonyId: 'registration', credential: { id: 'new' } }, expect.any(AbortSignal))
    expect(mocks.state).toMatchObject({ authenticated: false, user: undefined, expiresAt: undefined, agent: undefined })
    expect(mocks.reset).toHaveBeenCalledOnce()
    expect(mocks.replace).toHaveBeenCalledWith({ name: 'login', query: { passkeyChanged: '1' } })
  })

  it('allows revoking old credentials even when the current domain or browser cannot register', async () => {
    mocks.supported = false
    mocks.list.mockResolvedValue({ available: false, rpId: '', credentials: [credential] })
    await render()
    expect(wrapper.text()).toContain('固定的 HTTPS 域名')
    expect(wrapper.get('button.button--primary').attributes('disabled')).toBeDefined()
    await wrapper.get('li button').trigger('click')
    expect(wrapper.text()).toContain('撤销「Laptop」')
    expect(mocks.remove).not.toHaveBeenCalled()
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.remove).toHaveBeenCalledWith({ id: 'old', password: 'current-password', totpCode: '123456' }, expect.any(AbortSignal))
    expect(mocks.createPasskey).not.toHaveBeenCalled()
  })

  it('disables Passkey without changing password or two-step verification and returns to login', async () => {
    await render()
    await wrapper.get('button.button--danger-text').trigger('click')
    expect(wrapper.text()).toContain('清空绑定域名')
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.disableOrigin).toHaveBeenCalledWith({ password: 'current-password', totpCode: '123456' }, expect.any(AbortSignal))
    expect(mocks.remove).not.toHaveBeenCalled()
    expect(mocks.registerBegin).not.toHaveBeenCalled()
    expect(mocks.state).toMatchObject({ authenticated: false, user: undefined, expiresAt: undefined, agent: undefined })
    expect(mocks.reset).toHaveBeenCalledOnce()
    expect(mocks.replace).toHaveBeenCalledWith({ name: 'login', query: { passkeyChanged: '1' } })
  })

  it('does not finish registration or log out after browser cancellation and clears sensitive inputs', async () => {
    mocks.createPasskey.mockRejectedValueOnce(new DOMException('Cancelled', 'NotAllowedError'))
    await render()
    await wrapper.get('button.button--primary').trigger('click')
    await wrapper.get('input[maxlength="64"]').setValue('Phone')
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.registerFinish).not.toHaveBeenCalled()
    expect(mocks.replace).not.toHaveBeenCalled()
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('[role="alert"]').text()).toBe('Cancelled')
    expect(mocks.state.authenticated).toBe(true)
  })

  it('distinguishes failed loading from an empty credential list and supports retry', async () => {
    mocks.list.mockRejectedValueOnce(new Error('Unavailable'))
    await render()
    expect(wrapper.text()).not.toContain('尚未绑定')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Laptop')
    expect(wrapper.text()).toContain('尚未使用')
  })

  it('aborts pending browser operations when leaving the settings page', async () => {
    let finish: ((value: { ceremonyId: string; publicKey: { challenge: string } }) => void) | undefined
    mocks.registerBegin.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    await wrapper.get('button.button--primary').trigger('click')
    await wrapper.get('input[maxlength="64"]').setValue('Phone')
    await fillAuthentication()
    await wrapper.get('form').trigger('submit')
    const signal = mocks.registerBegin.mock.calls[0]![1] as AbortSignal
    wrapper.unmount()
    finish?.({ ceremonyId: 'registration', publicKey: { challenge: 'AA' } })
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(mocks.createPasskey).not.toHaveBeenCalled()
    expect(mocks.registerFinish).not.toHaveBeenCalled()
  })
})

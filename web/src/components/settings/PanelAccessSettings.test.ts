// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PanelAccessSettings from './PanelAccessSettings.vue'
import { panelDomainQuery, panelDomainSwitchPath, panelReachedWithoutTrustedDomain } from '@/lib/panelAccess'

const mocks = vi.hoisted(() => ({
  get: vi.fn(), updateSelfSignedTLS: vi.fn(), checkDomain: vi.fn(),
  publicNetwork: vi.fn(),
  inventory: vi.fn(), action: vi.fn(), listSites: vi.fn(), createSite: vi.fn(),
  navigate: vi.fn(), calls: [] as string[],
  location: { protocol: 'http:', host: '203.0.113.5:8080', origin: 'http://203.0.113.5:8080' },
  route: { query: {} as Record<string, string> },
}))
vi.mock('@/lib/api', () => ({
  ApiError: class ApiError extends Error {},
  api: {
    system: { publicNetwork: mocks.publicNetwork },
    settings: { panelAccess: { get: mocks.get, updateSelfSignedTLS: mocks.updateSelfSignedTLS, checkDomain: mocks.checkDomain, certificateURL: () => '/api/v1/settings/panel-access/self-signed-tls/certificate' } },
    apps: { inventory: mocks.inventory, action: mocks.action },
    sites: { list: mocks.listSites, create: mocks.createSite },
  },
}))
vi.mock('@/lib/panelAccess', async () => {
  const actual = await vi.importActual<typeof import('@/lib/panelAccess')>('@/lib/panelAccess')
  return { ...actual, currentPanelLocation: () => mocks.location, navigatePanel: mocks.navigate }
})
vi.mock('vue-router', () => ({ useRoute: () => mocks.route }))

const plainStatus = {
  allowIpHosts: true, requestTls: false, requestProxyHttps: false, requestIpHost: true,
  selfSignedTls: { enabled: false },
}
const kpanelItem = {
  id: 'thirdparty-kpanel',
  runtime: { installed: true, accessMode: 'direct', resourceVersion: 'rv-1', ports: [{ type: 'tcp', publicPort: 8080 }] },
  capabilities: { add_domain: { enabled: true }, direct_access: { enabled: true } },
}

let wrapper: ReturnType<typeof mount>
const render = async () => { wrapper = mount(PanelAccessSettings); await flushPromises() }
const button = (text: string) => {
  const match = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  if (!match) throw new Error(`button ${text} missing`)
  return match
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.calls = []
  mocks.location = { protocol: 'http:', host: '203.0.113.5:8080', origin: 'http://203.0.113.5:8080' }
  mocks.route.query = {}
  mocks.get.mockResolvedValue(plainStatus)
  mocks.publicNetwork.mockResolvedValue({ ipv4: '203.0.113.5', ipv6: '2001:db8::5' })
  mocks.inventory.mockResolvedValue({ items: [kpanelItem] })
  mocks.listSites.mockResolvedValue({ items: [] })
  mocks.createSite.mockResolvedValue({ id: 'site-1', primaryDomain: 'panel.example.com', type: 'proxy', upstream: 'http://127.0.0.1:8080', enabled: true })
  mocks.checkDomain.mockResolvedValue({ domain: 'panel.example.com', reachable: true, matched: true })
  mocks.action.mockImplementation(async () => { mocks.calls.push('block'); return { ok: true } })
  mocks.navigate.mockImplementation((target: string) => { mocks.calls.push(`navigate ${target}`) })
})
afterEach(() => wrapper?.unmount())

describe('Panel access settings', () => {
  it('shows DNS records at the HTTPS domain input and tolerates a network lookup failure', async () => {
    await render()
    expect(wrapper.get('.dns-guide').text()).toContain('203.0.113.5')
    expect(wrapper.get('.dns-guide').text()).toContain('2001:db8::5')
    wrapper.unmount()
    mocks.publicNetwork.mockRejectedValueOnce(new Error('offline'))
    await render()
    expect(wrapper.get('.dns-guide').text()).toContain('暂未识别本机公网 IP')
    expect(wrapper.find('input[placeholder="panel.example.com"]').exists()).toBe(true)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('explains plain HTTP limits and binds, verifies, then jumps before the IP block can bite', async () => {
    await render()
    expect(wrapper.text()).toContain('HTTP 明文访问')
    expect(wrapper.text()).toContain('剪贴板')

    await wrapper.get('input[placeholder="panel.example.com"]').setValue('Panel.Example.com')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.createSite).toHaveBeenCalledWith({
      primaryDomain: 'panel.example.com', aliases: [], type: 'proxy', upstream: 'http://127.0.0.1:8080', enabled: true,
    })
    expect(mocks.checkDomain).toHaveBeenCalledWith('panel.example.com', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('新地址已可用：https://panel.example.com')
    expect(mocks.navigate).not.toHaveBeenCalled()

    await button('跳转到新地址').trigger('click')
    await flushPromises()

    expect(mocks.action).toHaveBeenCalledWith('thirdparty-kpanel', 'direct_access', { resourceVersion: 'rv-1', accessMode: 'domain_only' })
    expect(mocks.calls).toEqual(['block', 'navigate https://panel.example.com/'])
  })

  it('stays on the current address when the IP block cannot be submitted, and allows jumping without it', async () => {
    mocks.action.mockRejectedValueOnce(new Error('busy'))
    mocks.route.query = { domain: 'panel.example.com' }
    await render()
    expect(mocks.checkDomain).toHaveBeenCalledWith('panel.example.com', expect.any(AbortSignal))

    await button('跳转到新地址').trigger('click')
    await flushPromises()
    expect(mocks.navigate).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('屏蔽直连提交失败')

    await wrapper.get('input[type="checkbox"]').setValue(false)
    await button('跳转到新地址').trigger('click')
    await flushPromises()
    expect(mocks.action).toHaveBeenCalledTimes(1)
    expect(mocks.navigate).toHaveBeenCalledWith('https://panel.example.com/')
  })

  it('never offers the switch when the domain is not serving this panel', async () => {
    mocks.checkDomain.mockResolvedValueOnce({ domain: 'panel.example.com', reachable: false, matched: false, problem: 'certificate_invalid' })
    mocks.route.query = { domain: 'panel.example.com' }
    await render()
    expect(wrapper.text()).toContain('证书无效或尚未签发')
    expect(wrapper.findAll('button').some((candidate) => candidate.text().includes('跳转到新地址'))).toBe(false)
  })

  it('falls back to manual guidance when KPanel is not managed by the App Market', async () => {
    mocks.inventory.mockResolvedValueOnce({ items: [] })
    mocks.get.mockResolvedValueOnce({ ...plainStatus, allowIpHosts: false })
    await render()
    expect(wrapper.text()).toContain('k fd')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('#panel-access-tls-title').exists()).toBe(false)
  })

  it('enables self-signed HTTPS, shows the fingerprint and opens the HTTPS address', async () => {
    mocks.updateSelfSignedTLS.mockResolvedValueOnce({
      ...plainStatus,
      selfSignedTls: { enabled: true, fingerprintSha256: 'AA:BB', notAfter: '2028-12-01T00:00:00Z', hosts: ['203.0.113.5', '127.0.0.1'] },
    })
    await render()
    await button('启用自签 HTTPS').trigger('click')
    await flushPromises()

    expect(mocks.updateSelfSignedTLS).toHaveBeenCalledWith({ enabled: true, regenerate: false })
    expect(mocks.navigate).toHaveBeenCalledWith('https://203.0.113.5:8080/settings?section=panel-access')
    expect(wrapper.text()).toContain('AA:BB')
    expect(wrapper.text()).toContain('203.0.113.5, 127.0.0.1')
    expect(wrapper.text()).toContain('2028')
    expect(wrapper.get('a[download]').attributes('href')).toBe('/api/v1/settings/panel-access/self-signed-tls/certificate')

    await button('切换到 HTTPS 访问').trigger('click')
    expect(mocks.navigate).toHaveBeenCalledWith('https://203.0.113.5:8080/settings?section=panel-access')
  })

  it('requires confirmation before reissuing and leaves the self-signed address when disabling it', async () => {
    mocks.location = { protocol: 'https:', host: '203.0.113.5:8080', origin: 'https://203.0.113.5:8080' }
    const enabled = { ...plainStatus, requestTls: true, selfSignedTls: { enabled: true, fingerprintSha256: 'AA:BB' } }
    mocks.get.mockResolvedValueOnce(enabled)
    mocks.updateSelfSignedTLS.mockResolvedValue({ ...enabled, selfSignedTls: { enabled: false, fingerprintSha256: 'AA:BB' } })
    await render()
    expect(wrapper.text()).toContain('自签 HTTPS（已加密')

    await button('重新签发证书').trigger('click')
    expect(mocks.updateSelfSignedTLS).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('指纹会改变')

    await button('关闭自签 HTTPS').trigger('click')
    await flushPromises()
    expect(mocks.updateSelfSignedTLS).toHaveBeenCalledWith({ enabled: false, regenerate: false })
    expect(mocks.navigate).toHaveBeenCalledWith('http://203.0.113.5:8080/settings?section=panel-access')
  })
})

describe('panel access routing helpers', () => {
  it('accepts only real domain names from the App Market hand-off', () => {
    expect(panelDomainQuery(' Panel.Example.com ')).toBe('panel.example.com')
    expect(panelDomainQuery('203.0.113.5')).toBe('')
    expect(panelDomainQuery('evil.com/path')).toBe('')
    expect(panelDomainQuery(['panel.example.com'])).toBe('')
    expect(panelDomainSwitchPath('panel.example.com')).toBe('/settings?section=panel-access&domain=panel.example.com')
  })

  it('treats only an HTTPS domain page as safe for blocking IP access to KPanel itself', () => {
    const at = (protocol: string, host: string) => ({ protocol, host, origin: protocol + '//' + host })
    expect(panelReachedWithoutTrustedDomain(at('http:', 'panel.example.com'))).toBe(true)
    expect(panelReachedWithoutTrustedDomain(at('https:', '203.0.113.5:8080'))).toBe(true)
    expect(panelReachedWithoutTrustedDomain(at('https:', '[2001:db8::1]:8080'))).toBe(true)
    expect(panelReachedWithoutTrustedDomain(at('https:', 'panel.example.com'))).toBe(false)
    expect(panelReachedWithoutTrustedDomain(at('https:', 'panel.example.com:8443'))).toBe(false)
  })
})

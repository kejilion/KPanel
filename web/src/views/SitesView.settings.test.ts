// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SitesView from './SitesView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), capabilities: vi.fn(), accessPasswords: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api')>(),
  api: {
    agent: { capabilities: mocks.capabilities },
    sites: { list: mocks.list, installations: vi.fn().mockResolvedValue([]), accessPasswords: mocks.accessPasswords },
    system: { publicNetwork: vi.fn().mockResolvedValue({}) },
    terminals: { list: vi.fn().mockResolvedValue([]) },
  },
}))
vi.mock('@/stores/panel', () => ({ usePanelState: () => ({ isReadOnly: { value: false } }) }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: vi.fn(), danger: vi.fn() }) }))

describe('site settings entries', () => {
  it('uses the same initially collapsed certificate and password entries when creating and editing', async () => {
    mocks.list.mockResolvedValue({ items: [{
      id: 'a'.repeat(32), primaryDomain: 'example.com', domains: ['example.com'], type: 'static', enabled: true,
      health: 'healthy', consistency: 'synced', access: 'managed', source: 'panel',
      resourceVersion: `sha256:${'a'.repeat(64)}`, allowedActions: ['update', 'delete'], certificate: { status: 'valid' },
    }], total: 1 })
    mocks.capabilities.mockResolvedValue(['sites.write', 'sites.templates.install', 'sites.custom-certificate', 'sites.certificate-replace', 'sites.access-password.write'].map((id) => ({ id, enabled: true })))
    mocks.accessPasswords.mockResolvedValue([])
    const wrapper = mount(SitesView, { global: { stubs: {
      ModalDialog: { props: ['open', 'title'], template: '<div v-if="open"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>' },
      RouterLink: true, DnsResolutionGuide: true, AppInteractiveTerminal: true, ShortcutCommandTerminal: true,
      SiteFavicon: true, SiteAppearanceName: true, SiteDeleteDialog: true, SiteCertificateRenewDialog: true, LocalWebServicePicker: true,
    } } })
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find((item) => item.text() === text)!
    await button('新建网站').trigger('click')
    const createCertificate = wrapper.findAll('button').find((item) => item.text().startsWith('HTTPS 证书'))!
    const createPassword = wrapper.findAll('button').find((item) => item.text().startsWith('访问密码'))!
    expect(createCertificate.attributes('aria-expanded')).toBe('false')
    expect(createPassword.attributes('aria-expanded')).toBe('false')
    for (const button of [createCertificate, createPassword]) {
      expect(wrapper.find(`[id="${button.attributes('aria-controls')}"]`).exists()).toBe(true)
    }
    const certificateLabel = createCertificate.text(), passwordLabel = createPassword.text()
    await createCertificate.trigger('click')
    expect(createCertificate.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('fieldset.site-certificate-options legend.sr-only').text()).toBe('HTTPS 证书')
    expect(wrapper.text()).toContain('自动申请或复用')
    await button('使用自定义证书').trigger('click')
    await createCertificate.trigger('click')
    expect(createCertificate.attributes('aria-expanded')).toBe('false')
    expect(createCertificate.text()).toContain('自有证书信息未填写完整，请展开并完成后保存。')
    expect(wrapper.find('textarea[placeholder="-----BEGIN CERTIFICATE-----"]').exists()).toBe(false)
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await createCertificate.trigger('click')
    expect(wrapper.find('textarea[placeholder="-----BEGIN CERTIFICATE-----"]').exists()).toBe(true)
    await button('取消').trigger('click')
    await button('设置').trigger('click')
    expect(wrapper.get('.site-security-settings__note').text()).toContain('example.com')
    const editCertificate = wrapper.findAll('button').find((item) => item.text().startsWith('HTTPS 证书'))!
    const editPassword = wrapper.findAll('button').find((item) => item.text().startsWith('访问密码'))!
    expect(editCertificate.text()).toBe(certificateLabel)
    expect(editPassword.text()).toBe(passwordLabel)
    expect(editCertificate.attributes('aria-expanded')).toBe('false')
    expect(editPassword.attributes('aria-expanded')).toBe('false')
    for (const button of [editCertificate, editPassword]) {
      expect(wrapper.find(`[id="${button.attributes('aria-controls')}"]`).exists()).toBe(true)
    }
    await editCertificate.trigger('click')
    expect(editCertificate.attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('保存证书')
    await wrapper.get('textarea[placeholder="-----BEGIN PRIVATE KEY-----"]').setValue('synthetic-private-key')
    await editCertificate.trigger('click')
    expect(editCertificate.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('textarea[placeholder="-----BEGIN PRIVATE KEY-----"]').exists()).toBe(false)
    await editCertificate.trigger('click')
    expect((wrapper.get('textarea[placeholder="-----BEGIN PRIVATE KEY-----"]').element as HTMLTextAreaElement).value).toBe('synthetic-private-key')
    wrapper.unmount()
  })

  it('identifies a site when only security settings are available', async () => {
    mocks.list.mockResolvedValue({ items: [{
      id: 'b'.repeat(32), primaryDomain: 'wordpress.example.com', domains: ['wordpress.example.com'], type: 'wordpress', enabled: true,
      health: 'healthy', consistency: 'synced', access: 'managed', source: 'panel',
      resourceVersion: `sha256:${'b'.repeat(64)}`, allowedActions: ['delete'], certificate: { status: 'valid' },
    }], total: 1 })
    mocks.capabilities.mockResolvedValue(['sites.certificate-replace', 'sites.access-password.write'].map((id) => ({ id, enabled: true })))
    mocks.accessPasswords.mockResolvedValue([])
    const wrapper = mount(SitesView, { global: { stubs: {
      ModalDialog: { props: ['open', 'title'], template: '<div v-if="open"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>' },
      RouterLink: true, DnsResolutionGuide: true, AppInteractiveTerminal: true, ShortcutCommandTerminal: true,
      SiteFavicon: true, SiteAppearanceName: true, SiteDeleteDialog: true, SiteCertificateRenewDialog: true, LocalWebServicePicker: true,
    } } })
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === '设置')!.trigger('click')
    expect(wrapper.find('#site-form').exists()).toBe(false)
    expect(wrapper.get('.site-security-settings__note').text()).toContain('wordpress.example.com')
    await wrapper.findAll('button').find((item) => item.text().startsWith('HTTPS 证书'))!.trigger('click')
    expect(wrapper.text()).toContain('保存证书')
    expect(wrapper.text()).toContain('访问密码')
    wrapper.unmount()
  })
})

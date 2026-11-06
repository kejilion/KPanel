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
    const certificateLabel = createCertificate.text(), passwordLabel = createPassword.text()
    await createCertificate.trigger('click')
    expect(createCertificate.attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('自动申请或复用')
    await button('取消').trigger('click')
    await button('设置').trigger('click')
    const editCertificate = wrapper.findAll('button').find((item) => item.text().startsWith('HTTPS 证书'))!
    const editPassword = wrapper.findAll('button').find((item) => item.text().startsWith('访问密码'))!
    expect(editCertificate.text()).toBe(certificateLabel)
    expect(editPassword.text()).toBe(passwordLabel)
    expect(editCertificate.attributes('aria-expanded')).toBe('false')
    expect(editPassword.attributes('aria-expanded')).toBe('false')
    await editCertificate.trigger('click')
    expect(editCertificate.attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('保存证书')
    await wrapper.get('textarea[placeholder="-----BEGIN PRIVATE KEY-----"]').setValue('synthetic-private-key')
    await editCertificate.trigger('click')
    expect(editCertificate.attributes('aria-expanded')).toBe('false')
    await editCertificate.trigger('click')
    expect((wrapper.get('textarea[placeholder="-----BEGIN PRIVATE KEY-----"]').element as HTMLTextAreaElement).value).toBe('synthetic-private-key')
    wrapper.unmount()
  })
})

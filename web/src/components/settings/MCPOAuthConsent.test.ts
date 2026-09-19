// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MCPOAuthConsent from './MCPOAuthConsent.vue'
import { mcpAccess } from '@/lib/mcp'

vi.mock('@/lib/mcp', async original => ({ ...await original<typeof import('@/lib/mcp')>(), mcpAccess: { oauthRequest: vi.fn(), oauthConsent: vi.fn() } }))
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(mcpAccess.oauthRequest).mockResolvedValue({ id: 'request', clientName: 'Unverified client', redirectUri: 'https://client.example/callback', expiresAt: '2099-09-19T00:00:00Z' })
  vi.mocked(mcpAccess.oauthConsent).mockRejectedValue(new Error('connection lost'))
})
describe('OAuth consent scope and recovery', () => {
  it('shows the exact callback and requires renewed confirmation after permission changes', async () => {
    const wrapper = mount(MCPOAuthConsent, { props: { requestId: 'request', hosts: [{ id: 'local', name: 'This host' }], version: 'v1' } }); await flushPromises()
    expect(wrapper.get('code').text()).toBe('https://client.example/callback')
    const submit = wrapper.get('button[type=submit]')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.findAll('input[type=checkbox]').at(-1)!.setValue(true)
    expect(submit.attributes('disabled')).toBeUndefined()
    await wrapper.findAll('select')[0]!.setValue('manage')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.findAll('input[type=checkbox]').at(-1)!.setValue(true)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mcpAccess.oauthConsent).toHaveBeenCalledExactlyOnceWith('request', expect.objectContaining({ approve: true, hostIds: ['local'], write: true, fileRoots: [], expectedResourceVersion: 'v1' }))
    expect(wrapper.get('[role=alert]').text()).toContain('检查已授权客户端列表')
    wrapper.unmount()
  })
  it('allows denial without a permission confirmation and gives expired requests a recovery path', async () => {
    const props = { requestId: 'request', hosts: [{ id: 'local', name: 'This host' }], version: 'v1' }
    const wrapper = mount(MCPOAuthConsent, { props }); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '拒绝授权')!.trigger('click'); await flushPromises()
    expect(mcpAccess.oauthConsent).toHaveBeenCalledWith('request', expect.objectContaining({ approve: false, write: false, domains: [] }))
    wrapper.unmount()
    vi.mocked(mcpAccess.oauthRequest).mockRejectedValue(new Error('expired'))
    const expired = mount(MCPOAuthConsent, { props }); await flushPromises()
    expect(expired.find('form').exists()).toBe(false)
    expect(expired.get('[role=alert]').text()).toContain('重新连接')
    expired.unmount()
  })
})

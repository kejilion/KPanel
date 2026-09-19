// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MCPAccess from './MCPAccess.vue'
import { mcpAccess, type MCPSettings } from '@/lib/mcp'
import { api } from '@/lib/api'

vi.mock('@/lib/mcp', () => ({ mcpAccess: { get: vi.fn(), enable: vi.fn(), create: vi.fn(), revoke: vi.fn(), test: vi.fn() } }))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: vi.fn() } } }))
let wrapper: ReturnType<typeof mount>
const initial = (): MCPSettings => ({ access: { enabled: false, available: true, resourceVersion: 'r1', clients: [] }, endpoint: 'https://panel.test/mcp', transportReady: true, maxClients: 32, permission: 'inspect' })
const client = { id: 'a'.repeat(32), name: 'Assistant', hosts: [{ id: 'local', identity: 'identity' }], createdAt: '2026-09-19T00:00:00Z', expiresAt: '2099-09-19T00:00:00Z' }
const enabled = (): MCPSettings => ({ ...initial(), access: { ...initial().access, enabled: true, resourceVersion: 'r2' } })
const created = (): MCPSettings => ({ ...enabled(), access: { ...enabled().access, clients: [client], resourceVersion: 'r3' } })
const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(mcpAccess.get).mockResolvedValue(initial())
  vi.mocked(api.cluster.hosts).mockResolvedValue({ items: [{ id: 'local', name: 'Local', isLocal: true }, { id: 'remote', name: 'Remote', isLocal: false }] } as Awaited<ReturnType<typeof api.cluster.hosts>>)
})
afterEach(() => wrapper?.unmount())

describe('MCP access journey', () => {
  it('enables, creates a host-scoped credential, tests and revokes it without exposing it in the saved list', async () => {
    vi.mocked(mcpAccess.enable).mockResolvedValue(enabled())
    vi.mocked(mcpAccess.create).mockResolvedValue({ client, token: 'one-time-secret', settings: created() })
    vi.mocked(mcpAccess.test).mockResolvedValue()
    vi.mocked(mcpAccess.revoke).mockResolvedValue(enabled())
    wrapper = mount(MCPAccess); await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    await button('启用 MCP').trigger('click'); await flushPromises()
    expect(mcpAccess.enable).toHaveBeenCalledWith(true, 'r1')
    await wrapper.get('input[autocomplete="off"]').setValue('Assistant')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mcpAccess.create).toHaveBeenCalledWith({ name: 'Assistant', hostIds: ['local'], expiresInDays: 30, expectedResourceVersion: 'r2' })
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toContain('Bearer one-time-secret')
    expect(wrapper.get('.mcp-history').text()).not.toContain('one-time-secret')
    await button('测试连接').trigger('click'); await flushPromises()
    expect(mcpAccess.test).toHaveBeenCalledWith('one-time-secret')
    expect(wrapper.text()).toContain('连接成功')
    await button('撤销').trigger('click'); await flushPromises()
    expect(mcpAccess.revoke).toHaveBeenCalledWith(client.id, 'r3')
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.text()).toContain('尚未创建客户端')
  })
  it('keeps failed mutations recoverable and never retries credential creation automatically', async () => {
    vi.mocked(mcpAccess.get).mockResolvedValue(enabled())
    vi.mocked(mcpAccess.create).mockRejectedValue(new Error('connection lost'))
    wrapper = mount(MCPAccess); await flushPromises()
    await wrapper.get('input[autocomplete="off"]').setValue('Assistant')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mcpAccess.create).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toContain('刷新列表核对')
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(button('刷新接入状态').attributes('disabled')).toBeUndefined()
  })
  it('requires secure transport, a name and at least one explicit host', async () => {
    vi.mocked(mcpAccess.get).mockResolvedValue({ ...initial(), transportReady: false, endpoint: '' })
    wrapper = mount(MCPAccess); await flushPromises()
    expect(button('启用 MCP').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('HTTPS')
    vi.mocked(mcpAccess.get).mockResolvedValue(enabled())
    await button('刷新接入状态').trigger('click'); await flushPromises()
    await wrapper.get('input[autocomplete="off"]').setValue('Assistant')
    await wrapper.get('input[type="checkbox"][value="local"]').setValue(false)
    expect(button('创建巡检客户端').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(mcpAccess.create).not.toHaveBeenCalled()
  })
  it('does not reveal a credential when its create response arrives after unmount', async () => {
    vi.mocked(mcpAccess.get).mockResolvedValue(enabled())
    let resolve!: (value: Awaited<ReturnType<typeof mcpAccess.create>>) => void
    vi.mocked(mcpAccess.create).mockReturnValue(new Promise(r => { resolve = r }))
    wrapper = mount(MCPAccess); await flushPromises()
    await wrapper.get('input[autocomplete="off"]').setValue('Assistant')
    await wrapper.get('form').trigger('submit')
    wrapper.unmount(); resolve({ client, token: 'late-secret', settings: created() }); await flushPromises()
    expect(document.body.textContent).not.toContain('late-secret')
  })
})

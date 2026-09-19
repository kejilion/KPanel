// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MCPClusterGrants from './MCPClusterGrants.vue'
import MCPOperations from './MCPOperations.vue'
import { mcpAccess, type MCPClusterGrants as GrantState, type MCPOperation } from '@/lib/mcp'

vi.mock('@/lib/mcp', async original => ({ ...await original<typeof import('@/lib/mcp')>(), mcpAccess: { clusterGrants: vi.fn(), grantCluster: vi.fn(), revokeCluster: vi.fn(), operations: vi.fn(), decide: vi.fn() } }))
const initial = (): GrantState => ({ controllers: [{ id: 'center', name: 'Trusted center', fingerprint: 'verified-fingerprint' }], grants: { available: true, resourceVersion: 'g1', items: [] } })
beforeEach(() => { vi.clearAllMocks(); vi.mocked(mcpAccess.clusterGrants).mockResolvedValue(initial()) })

describe('MCP cluster and approval journeys', () => {
  it('requires explicit confirmation for the selected controller and resets it when permissions change', async () => {
    const wrapper = mount(MCPClusterGrants, { props: { enabled: true, transportReady: true } }); await flushPromises()
    const submit = wrapper.get('button[type=submit]')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.findAll('select')[0]!.setValue('center')
    const confirmation = wrapper.findAll('input[type=checkbox]').at(-1)!
    await confirmation.setValue(true)
    expect(submit.attributes('disabled')).toBeUndefined()
    await wrapper.findAll('select')[1]!.setValue('true')
    expect(submit.attributes('disabled')).toBeDefined()
    await confirmation.setValue(true)
    vi.mocked(mcpAccess.grantCluster).mockResolvedValue({ ...initial(), grants: { available: true, resourceVersion: 'g2', items: [{ controllerId: 'center', revision: 'r', policy: { write: true, operationVersions: { host_apps_list: 'hash' } }, createdAt: '2026-09-19T00:00:00Z', expiresAt: '2099-09-19T00:00:00Z' }] } })
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mcpAccess.grantCluster).toHaveBeenCalledWith(expect.objectContaining({ controllerId: 'center', write: true, expectedResourceVersion: 'g1', fileRoots: [] }))
    expect(wrapper.text()).toContain('集群管理授权已保存')
    vi.mocked(mcpAccess.revokeCluster).mockResolvedValue(initial())
    await wrapper.findAll('button').find(item => item.text() === '撤销集群管理授权')!.trigger('click'); await flushPromises()
    expect(mcpAccess.revokeCluster).toHaveBeenCalledWith('center', 'g2')
    wrapper.unmount()
  })
  it('opens concrete parameters before approval and keeps request failures recoverable', async () => {
    const item: MCPOperation = { operationId: 'op', digest: 'digest', clientId: 'client', hostId: 'remote', tool: 'host_app_action', state: 'pending', arguments: { appId: 'nginx', action: 'restart' }, createdAt: '2026-09-19T00:00:00Z', expiresAt: '2099-09-19T00:00:00Z', updatedAt: '2026-09-19T00:00:00Z' }
    vi.mocked(mcpAccess.operations).mockResolvedValue({ items: [item] })
    vi.mocked(mcpAccess.decide).mockRejectedValue(new Error('lost receipt'))
    const wrapper = mount(MCPOperations, { props: { clients: [{ id: 'client', name: 'Assistant' }], hosts: [{ id: 'remote', name: 'Target host' }] } }); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '审阅并批准')!.trigger('click')
    expect(wrapper.get('details').attributes('open')).toBeDefined()
    expect(wrapper.get('pre').text()).toContain('nginx')
    const approve = wrapper.findAll('button').find(button => button.text() === '批准并执行')!
    expect(approve.attributes('disabled')).toBeDefined()
    await wrapper.get('input[type=checkbox]').setValue(true); await approve.trigger('click'); await flushPromises()
    expect(mcpAccess.decide).toHaveBeenCalledExactlyOnceWith('op', 'digest', true)
    expect(wrapper.get('[role=alert]').text()).toContain('刷新记录核对')
    wrapper.unmount()
  })
})

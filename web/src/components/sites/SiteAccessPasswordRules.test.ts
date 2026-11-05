// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SiteAccessPasswordRules from './SiteAccessPasswordRules.vue'

const mocks = vi.hoisted(() => ({ accessPasswords: vi.fn(), updateAccessAccount: vi.fn(), updateAccessRule: vi.fn(), list: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error { status = 500 }, api: { sites: { accessPasswords: mocks.accessPasswords, updateAccessAccount: mocks.updateAccessAccount, updateAccessRule: mocks.updateAccessRule, list: mocks.list } } }))

beforeEach(() => { mocks.accessPasswords.mockReset(); mocks.updateAccessAccount.mockReset(); mocks.updateAccessRule.mockReset(); mocks.list.mockReset() })

describe('SiteAccessPasswordRules', () => {
  it('reads on demand and displays paths and account names', async () => {
    mocks.accessPasswords.mockResolvedValue([{ id: 'rule', scope: 'location', path: '/admin/', usernames: ['alice'], accountsReadable: true }])
    const wrapper = mount(SiteAccessPasswordRules, { props: { siteId: 'first' } })
    expect(mocks.accessPasswords).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(mocks.accessPasswords).toHaveBeenCalledWith('first', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('/admin/')
    expect(wrapper.text()).toContain('alice')
    await wrapper.setProps({ siteId: 'second' })
    expect(wrapper.text()).not.toContain('alice')
  })

  it('offers a retry after a failed read', async () => {
    mocks.accessPasswords.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([])
    const wrapper = mount(SiteAccessPasswordRules, { props: { siteId: 'first' } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role=alert]').text()).toContain('读取失败')
    await wrapper.get('[role=alert] button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('未发现可识别的访问密码规则')
  })

  it('distinguishes same-scope rules and explains that deletion affects one', async () => {
    const rule = { scope: 'server', path: '/', usernames: ['alice'], accountsReadable: true, resourceVersion: 'version' }
    mocks.accessPasswords.mockResolvedValue([{ ...rule, id: 'one' }, { ...rule, id: 'two' }])
    const wrapper = mount(SiteAccessPasswordRules, { props: { siteId: 'first', writable: true } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('站点级规则 (1/2)')
    expect(wrapper.text()).toContain('站点级规则 (2/2)')
    await wrapper.findAll('button').find((button) => button.text() === '删除规则')!.trigger('click')
    expect(wrapper.text()).toContain('只删除这一条，其他同范围的规则会保留。')
  })

  it('updates an account with the current rule version and clears the password', async () => {
    const rule = { id: 'rule', scope: 'location', path: '/admin/', usernames: ['alice'], accountsReadable: true, resourceVersion: 'version' }
    mocks.accessPasswords.mockResolvedValue([rule])
    mocks.updateAccessAccount.mockResolvedValue([{ ...rule, usernames: ['bob'], resourceVersion: 'next' }])
    const wrapper = mount(SiteAccessPasswordRules, { props: { siteId: 'first', writable: true } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const edit = wrapper.findAll('button').find((button) => button.text() === '修改')!
    await edit.trigger('click')
    await wrapper.get('input[autocomplete=off]').setValue('bob')
    await wrapper.get('input[type=password]').setValue('synthetic-secret')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(mocks.updateAccessAccount).toHaveBeenCalledWith('first', {
      ruleId: 'rule', resourceVersion: 'version', action: 'update', username: 'bob',
      previousUsername: 'alice', password: 'synthetic-secret',
    })
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
    expect(wrapper.text()).toContain('bob')
  })

  it('adds a rule using the site version and refreshes it after saving', async () => {
    mocks.accessPasswords.mockResolvedValue([])
    mocks.updateAccessRule.mockResolvedValue([])
    mocks.list.mockResolvedValue({ items: [{ id: 'first', resourceVersion: 'fresh-version' }] })
    const wrapper = mount(SiteAccessPasswordRules, { props: { siteId: 'first', siteVersion: 'old-version', writable: true } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === '添加规则')!.trigger('click')
    await wrapper.get('input[autocomplete=off]').setValue('alice')
    await wrapper.get('input[type=password]').setValue('synthetic-secret')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(mocks.updateAccessRule).toHaveBeenCalledWith('first', {
      action: 'add', resourceVersion: 'old-version', scope: 'server', path: undefined,
      username: 'alice', password: 'synthetic-secret',
    })
    expect(wrapper.emitted('changed')).toEqual([['fresh-version']])
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
  })
})

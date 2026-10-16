// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { AppMarketItem, LocalAppDocument } from '@/types/api'

const mocks = vi.hoisted(() => {
  class ApiError extends Error {
    constructor(message: string, readonly status = 0, readonly code = 'request_failed') {
      super(message)
    }
  }
  return {
    ApiError,
    localApp: vi.fn(),
    createLocalApp: vi.fn(),
    updateLocalApp: vi.fn(),
    deleteLocalApp: vi.fn(),
  }
})
vi.mock('@/lib/api', () => ({
  ApiError: mocks.ApiError,
  api: {
    apps: {
      localApp: mocks.localApp,
      createLocalApp: mocks.createLocalApp,
      updateLocalApp: mocks.updateLocalApp,
      deleteLocalApp: mocks.deleteLocalApp,
    },
  },
}))
vi.mock('@/lib/clipboard', () => ({ copyText: vi.fn(async () => true) }))

const { default: LocalAppManager } = await import('./LocalAppManager.vue')

function localItem(token: string, installed = false, state = 'not_installed'): AppMarketItem {
  return {
    id: `local-${token}`, source: 'local', token, slug: token, name_zh: `${token} 服务`, name_en: token,
    desc_zh: '', desc_en: '', cat: 'local', icon: '/app-icons/thirdparty-default.svg', iconSha256: '',
    installer: 'kejilion', capabilities: {},
    runtime: { installed, state, ports: [], accessMode: 'not_applicable', updateStatus: 'not_installed', detectedBy: [] },
  } as unknown as AppMarketItem
}

function documentFor(token: string, installed = false): LocalAppDocument {
  return {
    token, content: `local app_id="${token}"\nlocal docker_name="${token}"\ndocker_app_plus\n`,
    resourceVersion: `sha256:${'a'.repeat(64)}`, installed, containerName: token,
  }
}

const wrappers: VueWrapper[] = []
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.clearAllMocks()
})

function mountManager(apps: AppMarketItem[]): VueWrapper {
  const wrapper = mount(LocalAppManager, { props: { apps } })
  wrappers.push(wrapper)
  return wrapper
}

function buttonByText(wrapper: VueWrapper, label: string) {
  const found = wrapper.findAll('button').find((item) => item.text().includes(label))
  if (!found) throw new Error(`missing button ${label}`)
  return found
}

describe('LocalAppManager', () => {
  it('explains how to start when there are no local apps', () => {
    const wrapper = mountManager([])
    expect(wrapper.text()).toContain('还没有本地应用')
  })

  it('lists every local app with state-aware actions', () => {
    const wrapper = mountManager([
      localItem('notes', true, 'running'), localItem('draft'), localItem('stopped', true, 'exited'),
    ])
    const rows = wrapper.findAll('.local-apps__list li')
    expect(rows.map((row) => row.find('small').text())).toEqual([
      'draft.conf · 未安装', 'notes.conf · 运行中', 'stopped.conf · 已安装',
    ])
    expect(rows[0]!.text()).toContain('删除')
    expect(rows[0]!.text()).not.toContain('提交官方')
    expect(rows[1]!.text()).toContain('提交官方')
    expect(rows[1]!.text()).toContain('先卸载')
    expect(rows[1]!.text()).not.toContain('删除')
    expect(rows[2]!.text()).not.toContain('提交官方')
  })

  it('routes installed apps to uninstall instead of deleting their config', async () => {
    const item = localItem('notes', true, 'running')
    const wrapper = mountManager([item])
    await buttonByText(wrapper, '先卸载').trigger('click')
    expect(wrapper.emitted('manage')?.[0]).toEqual([item])
    expect(mocks.deleteLocalApp).not.toHaveBeenCalled()
  })

  it('deletes an uninstalled app with its current resource version after confirmation', async () => {
    mocks.localApp.mockResolvedValue(documentFor('draft'))
    mocks.deleteLocalApp.mockResolvedValue(undefined)
    const wrapper = mountManager([localItem('draft')])
    await buttonByText(wrapper, '删除').trigger('click')
    expect(wrapper.text()).toContain('确认删除？')
    await wrapper.find('.button--danger').trigger('click')
    await flushPromises()
    expect(mocks.deleteLocalApp).toHaveBeenCalledWith('draft', `sha256:${'a'.repeat(64)}`)
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })

  it('creates an app from the template renamed to the chosen name', async () => {
    mocks.createLocalApp.mockResolvedValue(documentFor('mynotes'))
    const wrapper = mountManager([])
    await buttonByText(wrapper, '新建本地应用').trigger('click')
    await wrapper.find('.local-editor__name input').setValue('mynotes')
    const content = (wrapper.find('textarea').element as HTMLTextAreaElement).value
    expect(content).toContain('local app_id="mynotes"')
    expect(content).toContain('local docker_name="mynotes"')
    await buttonByText(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(mocks.createLocalApp).toHaveBeenCalledWith(expect.objectContaining({
      token: 'mynotes', resourceVersion: '', content: expect.stringContaining('docker_app_plus'),
    }))
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })

  it('blocks invalid names before reaching the server', async () => {
    const wrapper = mountManager([])
    await buttonByText(wrapper, '新建本地应用').trigger('click')
    await wrapper.find('.local-editor__name input').setValue('-bad/name')
    expect(wrapper.text()).toContain('名称只能使用字母')
    expect(buttonByText(wrapper, '保存').attributes('disabled')).toBeDefined()
  })

  it('edits with the loaded resource version and offers a reload on conflicts', async () => {
    mocks.localApp.mockResolvedValue(documentFor('notes', true))
    mocks.updateLocalApp.mockRejectedValue(new mocks.ApiError('配置已在别处被修改', 409, 'resource_conflict'))
    const wrapper = mountManager([localItem('notes', true, 'running')])
    await buttonByText(wrapper, '编辑').trigger('click')
    await flushPromises()
    expect(wrapper.find('.local-editor__name input').attributes('readonly')).toBeDefined()
    expect(wrapper.text()).toContain('docker_name 不能修改')
    await buttonByText(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(mocks.updateLocalApp).toHaveBeenCalledWith('notes', expect.objectContaining({
      token: 'notes', resourceVersion: `sha256:${'a'.repeat(64)}`,
    }))
    expect(wrapper.text()).toContain('配置已在别处被修改')
    await buttonByText(wrapper, '重新加载').trigger('click')
    await flushPromises()
    expect(mocks.localApp).toHaveBeenCalledTimes(2)
  })

  it('prefills a GitHub new-file PR from a running local app', async () => {
    mocks.localApp.mockResolvedValue(documentFor('notes', true))
    const wrapper = mountManager([localItem('notes', true, 'running')])
    await buttonByText(wrapper, '提交官方').trigger('click')
    await flushPromises()
    const href = wrapper.find('a.button--primary').attributes('href') ?? ''
    expect(href.startsWith('https://github.com/kejilion/apps/new/main?filename=notes.conf&value=')).toBe(true)
    expect(decodeURIComponent(href)).toContain('local app_id="notes"')
  })
})

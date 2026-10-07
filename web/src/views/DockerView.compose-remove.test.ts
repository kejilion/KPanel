// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import type { DockerInventory } from '@/types/api'

const mocks = vi.hoisted(() => ({ inventory: vi.fn(), composeProject: vi.fn(), task: vi.fn(), danger: vi.fn() }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {},
  api: {
    docker: {
      inventory: mocks.inventory, composeProject: mocks.composeProject, task: mocks.task,
      backups: vi.fn().mockResolvedValue({ items: [] }), environment: vi.fn().mockResolvedValue({}),
      jobs: vi.fn().mockResolvedValue({ items: [] }), job: vi.fn(),
    },
    system: { publicNetwork: vi.fn().mockResolvedValue({}) },
  },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: vi.fn(), danger: mocks.danger }) }))

function button(text: string): HTMLButtonElement {
  const found = [...document.body.querySelectorAll('button')].find(item => item.textContent?.trim() === text)
  if (!found) throw new Error(`missing button: ${text}`)
  return found
}

describe('Compose project removal', () => {
  let wrapper: VueWrapper
  beforeEach(async () => {
    vi.clearAllMocks()
    window.localStorage.clear()
    mocks.inventory.mockResolvedValue({ available: true, observedAt: '', version: '28.0.0',
      containers: [], composeProjects: ['demo'], images: [], networks: [], volumes: [] } satisfies DockerInventory)
    mocks.composeProject.mockResolvedValue({ name: 'demo', workingDirectory: '/home/docker/demo',
      configFiles: [{ path: '/home/docker/demo/docker-compose.yml', name: 'docker-compose.yml',
        source: 'services:\n  app:\n    image: nginx:alpine\n' }], services: ['app'], resourceVersion: 'sha256:config' })
    wrapper = mount(DockerView, { attachTo: document.body })
    await flushPromises()
    button('管理 Compose').click()
    await flushPromises()
  })
  afterEach(() => { wrapper.unmount(); document.body.innerHTML = ''; vi.restoreAllMocks() })

  it('requires an ordinary confirmation and defaults to preserving data', async () => {
    button('删除项目').click()
    await flushPromises()
    expect(mocks.task).not.toHaveBeenCalled()
    const options = document.body.querySelector('.compose-remove-options')!
    const inputs = options.querySelectorAll<HTMLInputElement>('input')
    expect(inputs[0]!.checked).toBe(true)
    expect(inputs[1]!.checked).toBe(false)
    expect(options.textContent).toContain('默认保留数据卷')
    button('取消').click()
    await flushPromises()
    expect(mocks.task).not.toHaveBeenCalled()
    expect(document.body.querySelector('.compose-remove-options')).toBeNull()
  })

  it('submits the project identity/version and explicit data choices', async () => {
    mocks.task.mockResolvedValue({ id: '1'.repeat(32), action: 'compose_remove', status: 'queued', progress: 0 })
    button('删除项目').click()
    await flushPromises()
    button('确认执行').click()
    await flushPromises()
    expect(mocks.task).toHaveBeenCalledWith({ action: 'compose_remove', name: 'demo',
      expectedResourceVersion: 'sha256:config', removeComposeFiles: true, removeVolumes: false })
    expect(document.body.querySelector('.compose-remove-options')).toBeNull()
  })

  it('can retain configuration and opt into volume deletion', async () => {
    mocks.task.mockResolvedValue({ id: '2'.repeat(32), action: 'compose_remove', status: 'queued', progress: 0 })
    button('删除项目').click()
    await flushPromises()
    const inputs = document.body.querySelectorAll<HTMLInputElement>('.compose-remove-options input')
    inputs[0]!.click()
    inputs[1]!.click()
    await flushPromises()
    expect(document.body.textContent).toContain('项目仍会显示在列表中')
    expect(document.body.textContent).toContain('数据不可恢复')
    button('确认执行').click()
    await flushPromises()
    expect(mocks.task).toHaveBeenCalledWith(expect.objectContaining({ removeComposeFiles: false, removeVolumes: true }))
  })

  it('retains confirmation/options on submit failure so the user can retry', async () => {
    mocks.task.mockRejectedValueOnce(new Error('stale configuration'))
    button('删除项目').click()
    await flushPromises()
    button('确认执行').click()
    await flushPromises()
    expect(document.body.querySelector('.compose-remove-options')).not.toBeNull()
    expect(button('确认执行').disabled).toBe(false)
    expect(mocks.task).toHaveBeenCalledTimes(1)
  })
})

// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import type { DockerContainer, DockerInventory, DockerMaintenanceJob } from '@/types/api'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  action: vi.fn(),
  task: vi.fn(),
  job: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number

    constructor(message: string, status = 0) {
      super(message)
      this.status = status
    }
  },
  api: {
    docker: {
      inventory: mocks.inventory,
      action: mocks.action,
      task: mocks.task,
      job: mocks.job,
      jobs: vi.fn().mockResolvedValue({ items: [] }),
      backups: vi.fn().mockResolvedValue({ items: [] }),
      environment: vi.fn().mockResolvedValue({}),
      checkUpdate: vi.fn(),
      composeProject: vi.fn(),
      stats: vi.fn(),
    },
    system: { publicNetwork: vi.fn().mockResolvedValue({}) },
  },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const running = ['logs', 'stats', 'exec', 'access', 'restart', 'pause', 'stop', 'remove']

function container(name: string, state: DockerContainer['state'], extra: Partial<DockerContainer> = {}): DockerContainer {
  return {
    id: `${name}`.padEnd(64, '0'), name, image: `${name}:1`, state, access: 'managed', consistency: 'synced',
    ports: [], networks: ['bridge'], mounts: [], resourceVersion: `sha256:${name}`,
    allowedActions: state === 'running' ? running : state === 'paused' ? ['logs', 'unpause', 'stop', 'remove'] : ['logs', 'start', 'remove'],
    ...extra,
  }
}

let fixture: DockerInventory

function inventory(): DockerInventory {
  return {
    available: true, version: '28.0.0', observedAt: '2026-10-09T00:00:00Z',
    containers: [
      container('web', 'running', { project: 'shop' }),
      container('db', 'running', { project: 'shop', mounts: [{ type: 'volume', name: 'db-data', destination: '/var/lib/mysql' }] }),
      container('kejilion-panel', 'running', { image: 'ghcr.io/kjlion/kejilion-panel:1.25.0' }),
      container('worker', 'exited'),
      container('cache', 'paused'),
    ],
    composeProjects: ['shop'],
    images: [],
    networks: [],
    volumes: ['db-data', 'old-data', 'tmp-data'].map((name) => ({ name, driver: 'local', resourceVersion: `v-${name}` })),
  }
}

function job(status: DockerMaintenanceJob['status'], message = ''): DockerMaintenanceJob {
  return { id: 'e'.repeat(32), action: 'volume_remove', status, stage: status, progress: 100, message, createdAt: '' }
}

function rowCheckbox(name: string): HTMLInputElement {
  const box = document.body.querySelector<HTMLInputElement>(`input[aria-label="选择容器 ${name}"], input[aria-label="选择存储卷 ${name}"]`)
  if (!box) throw new Error(`missing checkbox: ${name}`)
  return box
}

function barButton(label: string): HTMLButtonElement {
  const found = [...document.body.querySelectorAll<HTMLButtonElement>('.docker-batch-bar button')]
    .find((item) => item.querySelector('span')?.textContent?.trim() === label)
  if (!found) throw new Error(`missing bar button: ${label}`)
  return found
}

function button(text: string): HTMLButtonElement {
  const found = [...document.body.querySelectorAll('button')].find((item) => item.textContent?.trim() === text)
  if (!found) throw new Error(`missing button: ${text}`)
  return found
}

function dialog(): HTMLElement {
  const found = document.body.querySelector<HTMLElement>('.docker-batch')
  if (!found) throw new Error('missing batch dialog')
  return found
}

describe('Docker batch operations', () => {
  let wrapper: VueWrapper

  beforeEach(async () => {
    vi.clearAllMocks()
    window.localStorage.clear()
    fixture = inventory()
    mocks.inventory.mockImplementation(async () => structuredClone(fixture))
    wrapper = mount(DockerView, { attachTo: document.body })
    await flushPromises()
  })

  afterEach(() => {
    wrapper.unmount()
    document.body.innerHTML = ''
  })

  it('shows the bar with per-action counts once containers are selected', async () => {
    expect(document.body.querySelector('.docker-batch-bar')).toBeNull()
    rowCheckbox('web').click()
    rowCheckbox('worker').click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar__count')?.textContent).toBe('已选 2 项')
    expect(barButton('启动').querySelector('small')?.textContent).toBe('1')
    expect(barButton('停止').querySelector('small')?.textContent).toBe('1')
    expect(barButton('删除').querySelector('small')?.textContent).toBe('2')
    expect(barButton('暂停').disabled).toBe(false)
    rowCheckbox('web').click()
    await flushPromises()
    expect(barButton('暂停').disabled).toBe(true)
    expect(barButton('暂停').title).toBe('所选项都不适用此操作')
    expect(rowCheckbox('worker').closest('tr')?.classList.contains('is-batch-selected')).toBe(true)
  })

  it('selects all, whole Compose groups and shift-click ranges', async () => {
    const all = document.body.querySelector<HTMLInputElement>('input[aria-label="选择全部容器"]')!
    all.click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar__count')?.textContent).toBe('已选 5 项')
    all.click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar')).toBeNull()

    const group = document.body.querySelector<HTMLInputElement>('input[aria-label="选择 shop 中的全部容器"]')!
    group.click()
    await flushPromises()
    expect(rowCheckbox('web').checked).toBe(true)
    expect(rowCheckbox('db').checked).toBe(true)
    rowCheckbox('db').click()
    await flushPromises()
    expect(group.indeterminate).toBe(true)
    expect(all.indeterminate).toBe(true)

    document.body.querySelector<HTMLButtonElement>('.docker-batch-bar__clear')!.click()
    await flushPromises()
    const order = [...document.body.querySelectorAll<HTMLInputElement>('tbody input[aria-label^="选择容器 "]')]
    order[0]!.click()
    order[3]!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: true }))
    await flushPromises()
    expect(order.map((item) => item.checked)).toEqual([true, true, true, true, false])
  })

  it('confirms with the full target list, runs items in order and reports partial failure', async () => {
    rowCheckbox('web').click()
    rowCheckbox('kejilion-panel').click()
    rowCheckbox('db').click()
    rowCheckbox('worker').click()
    await flushPromises()
    barButton('停止').click()
    await flushPromises()

    expect(document.body.textContent).toContain('批量停止容器')
    expect(document.body.textContent).toContain('将逐个停止 3 个容器。')
    expect(dialog().textContent).toContain('所选包含 KPanel 自身容器 kejilion-panel，会放在最后执行')
    expect([...dialog().querySelectorAll('.docker-batch__item strong')].map((item) => item.textContent))
      .toEqual(['db', 'web', 'kejilion-panel'])
    expect(dialog().querySelector('.docker-batch__skipped')?.textContent).toContain('worker')
    expect(mocks.action).not.toHaveBeenCalled()

    mocks.action.mockImplementation(async (id: string) => {
      if (id.startsWith('db')) throw new Error('daemon timeout')
      return { status: 'completed' }
    })
    button('确认执行（3 项）').click()
    await flushPromises()

    expect(mocks.action.mock.calls).toEqual([
      ['db'.padEnd(64, '0'), 'stop', 'sha256:db'],
      ['web'.padEnd(64, '0'), 'stop', 'sha256:web'],
      ['kejilion-panel'.padEnd(64, '0'), 'stop', 'sha256:kejilion-panel'],
    ])
    expect(dialog().textContent).toContain('已处理 3/3 · 成功 2 · 失败 1')
    expect(dialog().querySelector('.docker-batch__item.is-failed')?.textContent).toContain('daemon timeout')
    expect(mocks.inventory).toHaveBeenCalledTimes(2)

    // Retry re-reads the inventory so it carries the current resourceVersion.
    fixture.containers[1]!.resourceVersion = 'sha256:db-new'
    mocks.action.mockResolvedValue({ status: 'completed' })
    button('重试失败项（1）').click()
    await flushPromises()
    expect(mocks.action).toHaveBeenLastCalledWith('db'.padEnd(64, '0'), 'stop', 'sha256:db-new')
    expect(dialog().textContent).toContain('全部完成。')
    button('完成').click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch')).toBeNull()
  })

  it('treats a dropped connection while stopping the panel itself as unconfirmed', async () => {
    const { ApiError } = await import('@/lib/api')
    rowCheckbox('kejilion-panel').click()
    await flushPromises()
    barButton('重启').click()
    await flushPromises()
    mocks.action.mockRejectedValue(new ApiError('无法连接到面板服务', 0))
    button('确认执行（1 项）').click()
    await flushPromises()
    const item = dialog().querySelector('.docker-batch__item')!
    expect(item.classList.contains('is-unknown')).toBe(true)
    expect(item.textContent).toContain('KPanel 自身容器可能正在停止或重启')
    expect(document.body.querySelector('button')?.textContent).not.toContain('重试失败项')
  })

  it('keeps running in the background after the dialog closes and toasts the outcome', async () => {
    let release: () => void = () => undefined
    mocks.action.mockImplementationOnce(() => new Promise((resolve) => { release = () => resolve({ status: 'completed' }) }))
    mocks.action.mockResolvedValue({ status: 'completed' })
    rowCheckbox('web').click()
    rowCheckbox('db').click()
    await flushPromises()
    barButton('重启').click()
    await flushPromises()
    button('确认执行（2 项）').click()
    await flushPromises()
    button('后台运行').click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch')).toBeNull()
    expect(document.body.querySelector('.docker-batch-bar__progress')?.textContent).toContain('批量重启容器 0/2')
    expect(barButton('停止').disabled).toBe(true)

    release()
    await flushPromises()
    expect(mocks.success).toHaveBeenCalledWith('批量重启容器', '全部 2 项已完成')
    expect(document.body.querySelector('.docker-batch-bar__progress')).toBeNull()
  })

  it('turns right-click on a row inside a multi-selection into a menu for the selection', async () => {
    rowCheckbox('web').click()
    rowCheckbox('db').click()
    rowCheckbox('worker').click()
    await flushPromises()
    const menu = () => document.body.querySelector<HTMLElement>('.docker-context-menu')
    const rightClick = (name: string) => rowCheckbox(name).closest('tr')!
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 40 }))

    rightClick('db')
    await flushPromises()
    expect(menu()?.querySelector('.docker-context-menu__title')?.textContent).toBe('已选 3 项')
    const items = [...menu()!.querySelectorAll<HTMLButtonElement>('button')]
    expect(items.map((item) => [item.textContent?.replace(/\s+/g, ''), item.disabled])).toEqual([
      ['启动1', false], ['重启2', false], ['暂停2', false], ['停止2', false], ['删除3', false], ['取消选择', false],
    ])
    items[3]!.click()
    await flushPromises()
    expect(menu()).toBeNull()
    expect(document.body.textContent).toContain('将逐个停止 2 个容器。')
    ;[...document.body.querySelectorAll<HTMLButtonElement>('.modal-panel button')].find((item) => item.textContent?.trim() === '取消')!.click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch')).toBeNull()

    // An unselected row keeps its own menu and leaves the selection alone.
    rightClick('cache')
    await flushPromises()
    expect(menu()?.querySelector('.docker-context-menu__title')?.textContent).toBe('cache')
    document.body.click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar__count')?.textContent).toBe('已选 3 项')

    // The row's ⋯ button is always about that row.
    rowCheckbox('db').closest('tr')!.querySelector<HTMLButtonElement>('.docker-context-trigger')!.click()
    await flushPromises()
    expect(menu()?.querySelector('.docker-context-menu__title')?.textContent).toBe('db')
    document.body.click()
    await flushPromises()

    rightClick('web')
    await flushPromises()
    ;[...menu()!.querySelectorAll<HTMLButtonElement>('button')].find((item) => item.textContent?.includes('取消选择'))!.click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar')).toBeNull()
  })

  it('clears the selection when switching to the monitoring view', async () => {
    rowCheckbox('web').click()
    await flushPromises()
    button('监控').click()
    await flushPromises()
    expect(document.body.querySelector('.docker-batch-bar')).toBeNull()
    expect(document.body.querySelector('input[aria-label="选择容器 web"]')).toBeNull()
    button('管理').click()
    await flushPromises()
    expect(rowCheckbox('web').checked).toBe(false)
  })

  it('holds the single Docker job slot while a volume batch runs', async () => {
    let finish: (value: DockerMaintenanceJob) => void = () => undefined
    mocks.task.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    button('存储卷3').click()
    await flushPromises()
    rowCheckbox('old-data').click()
    await flushPromises()
    barButton('删除').click()
    await flushPromises()
    button('确认执行（1 项）').click()
    await flushPromises()
    button('后台运行').click()
    button('容器5').click()
    await flushPromises()
    expect(button('管理 Compose').disabled).toBe(true)
    finish(job('succeeded'))
    await flushPromises()
    expect(button('管理 Compose').disabled).toBe(false)
  })

  it('deletes unused volumes through background jobs and skips mounted ones', async () => {
    button('存储卷3').click()
    await flushPromises()
    document.body.querySelector<HTMLInputElement>('input[aria-label="选择全部存储卷"]')!.click()
    await flushPromises()
    expect(barButton('删除').querySelector('small')?.textContent).toBe('2')
    barButton('删除').click()
    await flushPromises()
    expect(dialog().querySelector('.docker-batch__skipped')?.textContent).toContain('仍被容器 db 挂载，Docker 不允许删除')
    expect(dialog().textContent).toContain('存储卷中的数据会被永久删除，无法恢复。')

    mocks.task.mockImplementation(async (input: { target: string }) =>
      input.target === 'tmp-data' ? job('failed', 'volume is in use') : job('succeeded'))
    button('确认执行（2 项）').click()
    await flushPromises()
    expect(mocks.task.mock.calls.map(([input]) => input)).toEqual([
      { action: 'volume_remove', target: 'old-data', expectedResourceVersion: 'v-old-data' },
      { action: 'volume_remove', target: 'tmp-data', expectedResourceVersion: 'v-tmp-data' },
    ])
    expect(dialog().querySelector('.docker-batch__item.is-failed')?.textContent).toContain('volume is in use')
  })
})

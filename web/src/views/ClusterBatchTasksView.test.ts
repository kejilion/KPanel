// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { computed } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterBatchTasksView from './ClusterBatchTasksView.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type {
  ClusterBatchTask,
  ClusterBatchTaskCatalog,
  ClusterHost,
  ClusterHostList,
} from '@/types/api'

const mocks = vi.hoisted(() => ({
  hosts: vi.fn(),
  batchActions: vi.fn(),
  batchTasks: vi.fn(),
  batchTask: vi.fn(),
  createBatchTask: vi.fn(),
  cancelBatchTask: vi.fn(),
  retryBatchTask: vi.fn(),
  deleteBatchTask: vi.fn(),
  toastSuccess: vi.fn(),
  toastDanger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    constructor(message: string, public status = 0, public code = 'request_failed') { super(message) }
  },
  api: { cluster: {
    hosts: mocks.hosts,
    batchActions: mocks.batchActions,
    batchTasks: mocks.batchTasks,
    batchTask: mocks.batchTask,
    createBatchTask: mocks.createBatchTask,
    cancelBatchTask: mocks.cancelBatchTask,
    retryBatchTask: mocks.retryBatchTask,
    deleteBatchTask: mocks.deleteBatchTask,
  } },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: mocks.toastSuccess, danger: mocks.toastDanger }),
}))

const now = '2026-09-15T08:00:00Z'

function host(overrides: Partial<ClusterHost> = {}): ClusterHost {
  return {
    id: 'local',
    isLocal: true,
    kind: 'panel',
    name: '当前 KPanel',
    origin: 'http://127.0.0.1:1800',
    transportSecurity: 'e2e_http',
    remoteNodeId: 'local',
    federationProtocol: 'v2',
    scope: 'cluster.summary.read cluster.terminal.open cluster.files.read cluster.system.maintenance',
    terminalAvailable: true,
    fileManagementAvailable: true,
    mutualFileTransferAvailable: true,
    batchTaskAvailable: true,
    panelVersion: '1.19.0',
    state: 'online',
    consecutiveFailures: 0,
    polling: false,
    resourceVersion: 'rv-local',
    createdAt: now,
    updatedAt: now,
    ...overrides,
  }
}

const legacy = host({
  id: 'a'.repeat(32),
  isLocal: false,
  name: '旧连接',
  origin: 'https://legacy.example.test',
  remoteNodeId: '1'.repeat(32),
  federationProtocol: 'v1',
  scope: 'cluster.summary.read cluster.terminal.open cluster.files.read',
  batchTaskAvailable: false,
  resourceVersion: 'rv-legacy',
})

const light = host({
  id: 'b'.repeat(32),
  isLocal: false,
  kind: 'light_node',
  name: '轻量节点',
  origin: 'https://light.example.test',
  remoteNodeId: '2'.repeat(32),
  federationProtocol: 'light-v1',
  scope: 'cluster.summary.read',
  terminalAvailable: false,
  fileManagementAvailable: false,
  mutualFileTransferAvailable: false,
  batchTaskAvailable: false,
  resourceVersion: 'rv-light',
})

const inventory: ClusterHostList = {
  items: [host(), legacy, light],
  total: 3,
  remoteTotal: 2,
  maxHosts: 100,
  pollIntervalSeconds: 30,
  nodeId: '3'.repeat(32),
}

const catalog: ClusterBatchTaskCatalog = {
  actions: [
    { id: 'refresh', risk: 'read', requiresTaskScope: false, supportsLegacyPanels: true, supportsLightNodes: false },
    { id: 'system-update', risk: 'write', requiresTaskScope: true, supportsLegacyPanels: false, supportsLightNodes: false },
    { id: 'reboot', risk: 'disruptive', requiresTaskScope: true, supportsLegacyPanels: false, supportsLightNodes: false },
  ],
  limits: {
    maxTasks: 100,
    maxActiveTasks: 4,
    maxTargets: 50,
    maxConcurrency: 4,
    minTimeoutSeconds: 60,
    maxTimeoutSeconds: 3600,
    defaultTimeoutSeconds: 3600,
  },
}

function task(overrides: Partial<ClusterBatchTask> = {}): ClusterBatchTask {
  return {
    id: 'c'.repeat(32),
    action: 'refresh',
    state: 'succeeded',
    concurrency: 2,
    timeoutSeconds: 3600,
    cancelRequested: false,
    total: 1,
    completed: 1,
    succeeded: 1,
    failed: 0,
    needsAttention: 0,
    cancelled: 0,
    unsupported: 0,
    createdAt: now,
    finishedAt: now,
    targets: [{
      hostId: 'local',
      hostName: '当前 KPanel',
      hostKind: 'panel',
      operationId: 'd'.repeat(32),
      state: 'succeeded',
      stage: 'completed',
      progress: 100,
      finishedAt: now,
    }],
    ...overrides,
  }
}

let wrapper: ReturnType<typeof mount> | undefined

async function openView(path = '/cluster/tasks') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/cluster/tasks', component: ClusterBatchTasksView },
      { path: '/cluster', component: { template: '<div />' } },
      { path: '/activity', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  wrapper = mount(ClusterBatchTasksView, {
    global: {
      plugins: [router],
      provide: { [desktopWindowActiveKey as symbol]: computed(() => true) },
      stubs: {
        ModalDialog: {
          props: ['open', 'title', 'description'],
          template: '<section v-if="open" class="test-modal"><h2>{{ title }}</h2><p>{{ description }}</p><slot /><footer><slot name="footer" /></footer></section>',
        },
        StatusBadge: { props: ['status', 'label'], template: '<span class="test-status">{{ label }}</span>' },
      },
    },
  })
  await flushPromises()
  return { view: wrapper, router }
}

describe('ClusterBatchTasksView', () => {
  beforeEach(() => {
    mocks.hosts.mockReset().mockResolvedValue(inventory)
    mocks.batchActions.mockReset().mockResolvedValue(catalog)
    mocks.batchTasks.mockReset().mockResolvedValue({ items: [], total: 0 })
    mocks.batchTask.mockReset().mockResolvedValue(task())
    mocks.createBatchTask.mockReset().mockResolvedValue(task())
    mocks.cancelBatchTask.mockReset().mockResolvedValue(task({ state: 'cancelled', succeeded: 0, cancelled: 1 }))
    mocks.retryBatchTask.mockReset().mockResolvedValue(task({ id: 'e'.repeat(32), parentTaskId: 'c'.repeat(32) }))
    mocks.deleteBatchTask.mockReset().mockResolvedValue({ deleted: true })
    mocks.toastSuccess.mockReset()
    mocks.toastDanger.mockReset()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
  })

  it('exposes only the server catalog and disables maintenance on legacy and lightweight hosts', async () => {
    const { view } = await openView()
    expect(view.text()).toContain('不接受命令、脚本、路径或自定义参数')
    expect(view.findAll('.batch-action')).toHaveLength(catalog.actions.length)
    expect(view.find('textarea').exists()).toBe(false)

    const update = view.findAll('.batch-action').find((item) => item.text().includes('系统更新'))
    expect(update).toBeDefined()
    await update!.trigger('click')
    const checkboxes = view.findAll<HTMLInputElement>('.batch-host input[type="checkbox"]')
    expect(checkboxes).toHaveLength(3)
    expect(checkboxes[0]!.attributes('disabled')).toBeUndefined()
    expect(checkboxes[1]!.attributes('disabled')).toBeDefined()
    expect(checkboxes[2]!.attributes('disabled')).toBeDefined()
    expect(view.text()).toContain('旧版连接不具备批量维护权限')
    expect(view.text()).toContain('轻量节点不提供系统维护动作')
  })

  it('creates a task with only fixed scheduling fields and opens its durable detail route', async () => {
    const created = task()
    mocks.createBatchTask.mockResolvedValue(created)
    mocks.batchTask.mockResolvedValue(created)
    const { view, router } = await openView()
    await view.findAll('.batch-host input[type="checkbox"]')[0]!.setValue(true)
    await view.get('.batch-submit button').trigger('submit')
    await flushPromises()
    expect(view.get('.test-modal').text()).toContain('刷新主机状态')
    const confirm = view.findAll('.test-modal footer button').find((item) => item.text().includes('确认创建任务'))
    await confirm!.trigger('click')
    await flushPromises()

    expect(mocks.createBatchTask).toHaveBeenCalledWith({
      action: 'refresh',
      hostIds: ['local'],
      concurrency: 2,
      timeoutSeconds: 3600,
      confirmDisruptive: false,
    })
    expect(mocks.batchTask).toHaveBeenCalledWith(created.id, expect.any(AbortSignal))
    expect(router.currentRoute.value.query.task).toBe(created.id)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('批量任务已创建', '1 台主机已进入任务队列。')
  })

  it('requires an explicit service-interruption acknowledgement before submitting reboot', async () => {
    const reboot = task({ action: 'reboot' })
    mocks.createBatchTask.mockResolvedValue(reboot)
    mocks.batchTask.mockResolvedValue(reboot)
    const { view } = await openView()
    const rebootAction = view.findAll('.batch-action').find((item) => item.text().includes('延迟重启'))
    await rebootAction!.trigger('click')
    await view.findAll('.batch-host input[type="checkbox"]')[0]!.setValue(true)
    await view.get('.batch-submit button').trigger('submit')
    await flushPromises()
    const submit = view.findAll<HTMLButtonElement>('.test-modal footer button').find((item) => item.text().includes('确认创建任务'))!
    expect(submit.attributes('disabled')).toBeDefined()
    await view.get('.batch-reboot-check input').setValue(true)
    expect(submit.attributes('disabled')).toBeUndefined()
    await submit.trigger('click')
    await flushPromises()
    expect(mocks.createBatchTask).toHaveBeenCalledWith(expect.objectContaining({
      action: 'reboot',
      confirmDisruptive: true,
    }))
  })

  it('requires a fresh warning and disruptive confirmation when retrying reboot', async () => {
    const failed = task({
      action: 'reboot',
      state: 'needs_attention',
      succeeded: 0,
      needsAttention: 1,
      targets: [{
        hostId: 'local', hostName: '当前 KPanel', hostKind: 'panel', operationId: '2'.repeat(32),
        state: 'needs_attention', stage: 'submission_interrupted', progress: 100,
        errorCode: 'batch_submission_outcome_unknown', message: '结果无法确认', finishedAt: now,
      }],
    })
    const retried = task({ id: 'e'.repeat(32), action: 'reboot', parentTaskId: failed.id })
    mocks.batchTasks.mockResolvedValue({ items: [{ ...failed, targets: undefined }], total: 1 })
    mocks.batchTask.mockResolvedValueOnce(failed).mockResolvedValue(retried)
    mocks.retryBatchTask.mockResolvedValue(retried)

    const { view } = await openView(`/cluster/tasks?task=${failed.id}`)
    const retry = view.findAll('.test-modal footer button').find((item) => item.text().includes('重试未成功主机'))
    await retry!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringMatching(/上次重启结果不确定.*再次安排重启.*中断服务/))
    expect(mocks.retryBatchTask).toHaveBeenCalledWith(failed.id, { confirmDisruptive: true })
  })

  it('restores an exact task from the URL and cancels only through the typed endpoint', async () => {
    const id = 'f'.repeat(32)
    const running = task({
      id,
      action: 'system-update',
      state: 'running',
      completed: 0,
      succeeded: 0,
      startedAt: now,
      finishedAt: undefined,
      targets: [{
        hostId: 'local', hostName: '当前 KPanel', hostKind: 'panel', operationId: '1'.repeat(32),
        executionId: 'maintenance-1', state: 'running', stage: 'packages', progress: 42, startedAt: now,
      }],
    })
    const cancelling = { ...running, state: 'cancelling' as const, cancelRequested: true }
    mocks.batchTasks.mockResolvedValue({ items: [{ ...running, targets: undefined }], total: 1 })
    mocks.batchTask.mockResolvedValue(running)
    mocks.cancelBatchTask.mockResolvedValue(cancelling)
    const { view } = await openView(`/cluster/tasks?task=${id}`)
    expect(mocks.batchTask).toHaveBeenCalledWith(id, expect.any(AbortSignal))
    expect(view.get('.test-modal').text()).toContain('maintenance-1')
    const cancel = view.findAll('.test-modal footer button').find((item) => item.text().includes('取消未提交主机'))
    await cancel!.trigger('click')
    await flushPromises()
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('已提交的维护动作会继续运行'))
    expect(mocks.cancelBatchTask).toHaveBeenCalledWith(id)
  })

  it('stops polling after both the task list and selected detail are terminal', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const completed = task()
    mocks.batchTasks.mockResolvedValue({ items: [{ ...completed, targets: undefined }], total: 1 })
    mocks.batchTask.mockResolvedValue(completed)
    try {
      await openView(`/cluster/tasks?task=${completed.id}`)
      const listCalls = mocks.batchTasks.mock.calls.length
      const detailCalls = mocks.batchTask.mock.calls.length
      await vi.advanceTimersByTimeAsync(3_100)
      await flushPromises()
      expect(mocks.batchTasks).toHaveBeenCalledTimes(listCalls)
      expect(mocks.batchTask).toHaveBeenCalledTimes(detailCalls)
    } finally {
      vi.useRealTimers()
    }
  })
})

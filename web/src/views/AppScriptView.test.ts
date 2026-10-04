// @vitest-environment jsdom
import { flushPromises, shallowMount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest, setLocale } from '@/i18n'
import { desktopWindowCloseGuardKey, desktopWindowLaunchKey, type DesktopWindowCloseGuardRegistry } from '@/lib/desktopRouteKeys'
import { resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import AppScriptView from './AppScriptView.vue'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  jobs: vi.fn(),
  job: vi.fn(),
  cancelJob: vi.fn(),
  action: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class ApiError extends Error {
    code?: string
  },
  api: {
    apps: {
      inventory: mocks.inventory,
      jobs: mocks.jobs,
      job: mocks.job,
      cancelJob: mocks.cancelJob,
      action: mocks.action,
    },
  },
}))

vi.mock('@/components/apps/AppInteractiveTerminal.vue', () => ({
  default: {
    name: 'AppInteractiveTerminal',
    props: ['jobId', 'inputOpen', 'kind'],
    template: '<div class="terminal-stub" />',
  },
}))

const app = {
  id: 'openclaw',
  name_zh: 'OpenClaw',
  name_en: 'OpenClaw',
  runtime: { installed: true, resourceVersion: 'rv-1' },
  capabilities: { manage: { enabled: true } },
}

const job = {
  id: 'job-1',
  appId: 'openclaw',
  appName: 'OpenClaw',
  action: 'manage',
  interactive: true,
  inputOpen: true,
  status: 'running',
  stage: 'interactive',
  progress: 20,
  logs: [],
  createdAt: '',
}

async function mountView(closeGuards?: DesktopWindowCloseGuardRegistry, consumeLaunch = () => true) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/app-script/:appId', component: AppScriptView }],
  })
  await router.push('/app-script/openclaw')
  await router.isReady()
  const wrapper = shallowMount(AppScriptView, {
    global: {
      plugins: [router],
      provide: {
        [desktopWindowLaunchKey as symbol]: consumeLaunch,
        ...(closeGuards ? { [desktopWindowCloseGuardKey as symbol]: closeGuards } : {}),
      },
      stubs: { AppInteractiveTerminal: true },
    },
  })
  await flushPromises()
  return wrapper
}

describe('dedicated desktop app script terminal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetDesktopModeForTest()
    window.localStorage.clear()
    mocks.inventory.mockResolvedValue({ items: [app] })
    mocks.jobs.mockResolvedValue({ items: [] })
    mocks.job.mockResolvedValue({ ...job, inputOpen: false, status: 'cancelled', stage: 'cancelled' })
    mocks.cancelJob.mockResolvedValue({ ...job, inputOpen: false, stage: 'cancelling' })
    mocks.action.mockResolvedValue(job)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    resetLocaleForTest()
  })

  it('starts the structured manage action without mounting the app marketplace', async () => {
    const wrapper = await mountView()

    expect(mocks.action).toHaveBeenCalledWith('openclaw', 'manage', { resourceVersion: 'rv-1' })
    expect(wrapper.find('.app-script-page__header').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props()).toMatchObject({
      jobId: 'job-1',
      kind: 'app',
    })
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe('job-1')
    wrapper.unmount()
  })

  it('reattaches to the same running manage job instead of starting a duplicate', async () => {
    mocks.jobs.mockResolvedValue({ items: [job] })
    const wrapper = await mountView()

    expect(mocks.action).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props('jobId')).toBe('job-1')
    wrapper.unmount()
  })

  it('restores a running task without creating a new task', async () => {
    mocks.jobs.mockResolvedValue({ items: [job] })
    const wrapper = await mountView(undefined, () => false)

    expect(mocks.action).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props('jobId')).toBe(job.id)
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe(job.id)
    wrapper.unmount()
  })

  it('keeps a cancelled task stopped when its saved window is restored', async () => {
    mocks.jobs.mockResolvedValue({ items: [{ ...job, status: 'cancelled', inputOpen: false }] })
    const wrapper = await mountView(undefined, () => false)

    expect(mocks.action).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('当前没有运行中的脚本任务')
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBeNull()
    wrapper.unmount()
  })

  it('starts a restored empty window only after an explicit start click', async () => {
    const wrapper = await mountView(undefined, () => false)
    expect(mocks.action).not.toHaveBeenCalled()

    await wrapper.find('[data-app-script-start]').trigger('click')
    await flushPromises()

    expect(mocks.action).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props('jobId')).toBe(job.id)
    wrapper.unmount()
  })

  it('does not launch when retrying an unavailable task list in a restored window', async () => {
    mocks.jobs.mockRejectedValueOnce(new Error('Agent offline'))
    const wrapper = await mountView(undefined, () => false)
    expect(wrapper.text()).toContain('Agent offline')

    await wrapper.find('.app-script-page__state.is-error button').trigger('click')
    await flushPromises()

    expect(mocks.action).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('当前没有运行中的脚本任务')
    wrapper.unmount()
  })

  it('does not reuse a consumed launch intent after the task ends and the page remounts', async () => {
    const desktop = useDesktopMode()
    const windowID = desktop.openWindow('/app-script/openclaw', 'desktop.scriptWindowTitle', false)
    const consumeLaunch = () => desktop.consumeWindowLaunch(windowID)
    const first = await mountView(undefined, consumeLaunch)
    expect(mocks.action).toHaveBeenCalledTimes(1)
    first.unmount()
    mocks.jobs.mockResolvedValue({ items: [{ ...job, status: 'cancelled', inputOpen: false }] })

    const restored = await mountView(undefined, consumeLaunch)
    expect(mocks.action).toHaveBeenCalledTimes(1)
    expect(restored.text()).toContain('当前没有运行中的脚本任务')
    restored.unmount()
  })

  it('starts this app while another application has an active shell', async () => {
    mocks.jobs.mockResolvedValue({ items: [{ ...job, id: 'other-job', appId: 'other-app' }] })
    const wrapper = await mountView()
    expect(mocks.action).toHaveBeenCalledWith('openclaw', 'manage', { resourceVersion: 'rv-1' })
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props('jobId')).toBe('job-1')
    wrapper.unmount()
  })

  it('localizes an active task conflict while preserving the app name', async () => {
    await setLocale('en-US', false)
    mocks.jobs.mockResolvedValue({ items: [{ ...job, action: 'update' }] })
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('An app task is already running: OpenClaw')
    wrapper.unmount()
  })

  it('stops the active process and waits for the terminal job before allowing the window to close', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    const unregister = vi.fn()
    const closeGuards: DesktopWindowCloseGuardRegistry = {
      register: vi.fn((candidate) => {
        guard = candidate
        return unregister
      }),
    }
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = await mountView(closeGuards)

    expect(guard).toBeTypeOf('function')
    await expect(Promise.resolve(guard?.())).resolves.toBe(true)
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('释放应用管理锁'))
    expect(mocks.cancelJob).toHaveBeenCalledWith('job-1')
    expect(mocks.job).toHaveBeenCalledWith('job-1')
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBeNull()

    wrapper.unmount()
    expect(unregister).toHaveBeenCalledOnce()
  })

  it('keeps the window open when the Agent cannot confirm process shutdown', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    const closeGuards: DesktopWindowCloseGuardRegistry = {
      register: (candidate) => {
        guard = candidate
        return vi.fn()
      },
    }
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mocks.cancelJob.mockRejectedValue(new Error('Agent offline'))
    const wrapper = await mountView(closeGuards)

    await expect(Promise.resolve(guard?.())).resolves.toBe(false)
    expect(wrapper.find('[role="alert"]').text()).toContain('Agent offline')
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe('job-1')
    wrapper.unmount()
  })

  it('keeps the process running when window closure is cancelled', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    const closeGuards: DesktopWindowCloseGuardRegistry = {
      register: (candidate) => {
        guard = candidate
        return vi.fn()
      },
    }
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = await mountView(closeGuards)

    await expect(Promise.resolve(guard?.())).resolves.toBe(false)
    expect(mocks.cancelJob).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

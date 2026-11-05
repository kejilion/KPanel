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
  owner: { authenticated: true, user: { id: '' } },
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
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: mocks.owner }) }))

vi.mock('@/components/apps/AppInteractiveTerminal.vue', () => ({
  default: {
    name: 'AppInteractiveTerminal',
    props: { jobId: String, inputOpen: Boolean, kind: String, headless: Boolean },
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
    mocks.owner.user.id = `app-test-${crypto.randomUUID()}`
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

    expect(mocks.action).toHaveBeenCalledWith('openclaw', 'manage', { resourceVersion: 'rv-1', jobId: expect.stringMatching(/^[a-f0-9]{32}$/) })
    expect(wrapper.find('.app-script-page__header').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'AppInteractiveTerminal' }).props()).toMatchObject({
      jobId: 'job-1',
      kind: 'app',
      // The window title bar already names the app and owns maximize.
      headless: true,
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
    expect(mocks.action).toHaveBeenCalledWith('openclaw', 'manage', { resourceVersion: 'rv-1', jobId: expect.stringMatching(/^[a-f0-9]{32}$/) })
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

  it('uses window close to stop the task without a redundant terminal toolbar', async () => {
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
    expect(mocks.cancelJob).not.toHaveBeenCalled()
    expect(confirm).not.toHaveBeenCalled()
    expect(wrapper.find('[data-app-script-stop]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('关闭窗口或刷新不会结束任务')
    expect(wrapper.element.children).toHaveLength(1)
    await expect(Promise.resolve(guard?.())).resolves.toBe(true)
    expect(confirm).not.toHaveBeenCalled()
    expect(mocks.cancelJob).toHaveBeenCalledWith('job-1', expect.any(AbortSignal))
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe('job-1')

    wrapper.unmount()
    expect(unregister).toHaveBeenCalledOnce()
  })

  it.each(['inventory', 'jobs'] as const)('closes during pending %s without stopping an unrelated later task', async stage => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    const closeGuards: DesktopWindowCloseGuardRegistry = {
      register: candidate => { guard = candidate; return vi.fn() },
    }
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mocks.jobs.mockResolvedValue({ items: [job] })
    let complete!: () => void
    let pendingSignal: AbortSignal | undefined
    mocks[stage].mockImplementationOnce((signal?: AbortSignal) => new Promise((resolve, reject) => {
      pendingSignal = signal
      complete = () => resolve({ items: stage === 'inventory' ? [app] : [job] })
      signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))
    const wrapper = await mountView(closeGuards)
    let settled = false
    const closing = Promise.resolve(guard?.()).then(result => { settled = true; return result })
    await flushPromises()
    expect(settled).toBe(true)
    expect(pendingSignal?.aborted).toBe(true)
    expect(mocks.cancelJob).not.toHaveBeenCalled()
    complete()
    await expect(closing).resolves.toBe(true)
    await flushPromises()
    expect(mocks.action).not.toHaveBeenCalled()
    expect(mocks.cancelJob).not.toHaveBeenCalled()
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBeNull()
    wrapper.unmount()
  })


  it('closes during an in-flight manage action and stops its returned task', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    let complete!: (value: typeof job) => void
    mocks.action.mockReturnValueOnce(new Promise(resolve => { complete = resolve }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = await mountView({ register: candidate => { guard = candidate; return vi.fn() } })
    let settled = false
    const closing = Promise.resolve(guard?.()).then(result => { settled = true; return result })
    await flushPromises()
    expect(settled).toBe(true)
    complete(job)
    await expect(closing).resolves.toBe(true)
    await flushPromises()
    expect(mocks.action).toHaveBeenCalledOnce()
    expect(mocks.cancelJob).toHaveBeenCalledWith('job-1', expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('keeps the exact pending job identity when a launch response is lost', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    mocks.action.mockReturnValueOnce(new Promise(() => undefined))
    mocks.cancelJob.mockRejectedValue(new Error('offline'))
    const wrapper = await mountView({ register: candidate => { guard = candidate; return vi.fn() } })
    const jobId = mocks.action.mock.calls[0]![2].jobId as string
    expect(await guard?.()).toBe(true)
    expect(mocks.cancelJob).toHaveBeenCalledWith(jobId, expect.any(AbortSignal))
    expect(JSON.parse(window.localStorage.getItem(`kpanel:pending-app-job-stops:${mocks.owner.user.id}`)!)).toContain(jobId)
    wrapper.unmount()
  })

  it('retains every uncertain launch identity when a user retries before closing', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    mocks.action.mockRejectedValueOnce(new Error('lost response')).mockReturnValueOnce(new Promise(() => undefined))
    mocks.cancelJob.mockRejectedValue(new Error('offline'))
    const wrapper = await mountView({ register: candidate => { guard = candidate; return vi.fn() } })
    await wrapper.find('.app-script-page__state.is-error button').trigger('click')
    await flushPromises()
    await wrapper.find('[data-app-script-start]').trigger('click')
    await flushPromises()
    const first = mocks.action.mock.calls[0]![2].jobId as string
    const second = mocks.action.mock.calls[1]![2].jobId as string
    expect(first).not.toBe(second)
    expect(await guard?.()).toBe(true)
    const pending = JSON.parse(window.localStorage.getItem(`kpanel:pending-app-job-stops:${mocks.owner.user.id}`)!)
    expect(pending).toEqual(expect.arrayContaining([first, second]))
    wrapper.unmount()
  })

  it('closes silently but preserves the task identity when shutdown cannot be confirmed', async () => {
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

    expect(await guard?.()).toBe(true)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe('job-1')
    wrapper.unmount()
  })

  it('does not ask for confirmation when ending the task', async () => {
    let guard: (() => boolean | Promise<boolean>) | undefined
    const closeGuards: DesktopWindowCloseGuardRegistry = {
      register: (candidate) => {
        guard = candidate
        return vi.fn()
      },
    }
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = await mountView(closeGuards)

    expect(await guard?.()).toBe(true)
    expect(window.confirm).not.toHaveBeenCalled()
    expect(mocks.cancelJob).toHaveBeenCalledWith('job-1', expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('keeps the task running across page refresh and restores the same terminal', async () => {
    const wrapper = await mountView()
    wrapper.unmount()

    expect(mocks.cancelJob).not.toHaveBeenCalled()
    expect(window.localStorage.getItem('kpanel:active-app-job')).toBe('job-1')
    mocks.jobs.mockResolvedValue({ items: [job] })
    const restored = await mountView(undefined, () => false)
    expect(mocks.action).toHaveBeenCalledOnce()
    expect(restored.findComponent({ name: 'AppInteractiveTerminal' }).props('jobId')).toBe('job-1')
    restored.unmount()
  })
})

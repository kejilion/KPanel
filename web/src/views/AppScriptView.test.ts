// @vitest-environment jsdom
import { flushPromises, shallowMount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest, setLocale } from '@/i18n'
import AppScriptView from './AppScriptView.vue'

const mocks = vi.hoisted(() => ({
  inventory: vi.fn(),
  jobs: vi.fn(),
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

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/app-script/:appId', component: AppScriptView }],
  })
  await router.push('/app-script/openclaw')
  await router.isReady()
  const wrapper = shallowMount(AppScriptView, {
    global: {
      plugins: [router],
      stubs: { AppInteractiveTerminal: true },
    },
  })
  await flushPromises()
  return wrapper
}

describe('dedicated desktop app script terminal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    mocks.inventory.mockResolvedValue({ items: [app] })
    mocks.jobs.mockResolvedValue({ items: [] })
    mocks.action.mockResolvedValue(job)
  })

  afterEach(() => {
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

  it('localizes an active task conflict while preserving the app name', async () => {
    await setLocale('en-US', false)
    mocks.jobs.mockResolvedValue({ items: [{ ...job, appId: 'other-app', appName: 'Other app' }] })
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('An app task is already running: Other app')
    wrapper.unmount()
  })
})

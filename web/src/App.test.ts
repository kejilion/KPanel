// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import App from './App.vue'
import { resetLocaleForTest, setLocale, t } from '@/i18n'
import { initializeDesktopMode, resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'

vi.mock('@/i18n/phrase', () => ({
  usePhraseCatalog: vi.fn(),
  installPhraseLocalization: () => () => {},
}))
vi.mock('@/components/feedback/ToastHost.vue', () => ({ default: { template: '<div />' } }))

let wrapper: VueWrapper | undefined

async function mountAt(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/overview', component: { template: '<div />' }, meta: { titleKey: 'route.overview' } },
      { path: '/settings', component: { template: '<div />' }, meta: { titleKey: 'route.settings' } },
      { path: '/login', component: { template: '<div />' }, meta: { titleKey: 'route.login', public: true } },
    ],
  })
  await router.push(path)
  await router.isReady()
  wrapper = mount(App, { global: { plugins: [router] } })
  await nextTick()
  return router
}

describe('browser tab title', () => {
  beforeEach(() => {
    resetDesktopModeForTest()
    resetLocaleForTest()
    window.localStorage.clear()
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.title = ''
  })

  it('follows the active mode, while public pages keep their route title', async () => {
    const router = await mountAt('/overview')
    expect(document.title).toBe('概览 · KPanel')

    const desktop = useDesktopMode()
    desktop.enterDesktop()
    await nextTick()
    expect(document.title).toBe('桌面模式 · KPanel')

    await router.push('/settings')
    expect(document.title).toBe('桌面模式 · KPanel')

    await setLocale('en-US', false)
    await nextTick()
    expect(document.title).toBe('Desktop mode · KPanel')

    await router.push('/login')
    expect(document.title).toBe(`${t('route.login')} · KPanel`)

    await router.push('/overview')
    desktop.enterClassic()
    await nextTick()
    expect(document.title).toBe(`${t('route.overview')} · KPanel`)
  })

  it('uses the desktop title after restoring the saved mode', async () => {
    useDesktopMode().enterDesktop()
    resetDesktopModeForTest()
    initializeDesktopMode()
    await mountAt('/settings')
    expect(document.title).toBe('桌面模式 · KPanel')
  })
})

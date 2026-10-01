// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FilesSplitWorkspace from './FilesSplitWorkspace.vue'

const harness = vi.hoisted(() => ({
  blockClose: false,
  blockPrimaryClose: false,
  busy: { primary: false, secondary: false } as Record<string, boolean>,
  toast: vi.fn(),
}))

vi.mock('@/stores/toast', () => ({ useToast: () => ({ show: harness.toast }) }))

// A light FilesView that reports what each pane resolves through injection.
vi.mock('@/views/FilesView.vue', async () => {
  const { defineComponent, h, inject, onBeforeUnmount } = await import('vue')
  const { useRoute, useRouter } = await import('vue-router')
  const { desktopWindowActiveKey, desktopWindowCloseGuardKey } = await import('@/lib/desktopRouteKeys')
  const { filesSplitControlKey } = await import('@/lib/filesSplit')
  return {
    default: defineComponent({
      setup() {
        const route = useRoute()
        const router = useRouter()
        const active = inject(desktopWindowActiveKey)
        const split = inject(filesSplitControlKey, undefined)
        const unregister = inject(desktopWindowCloseGuardKey, undefined)?.register(() => (
          !harness.blockClose && !(split?.role === 'primary' && harness.blockPrimaryClose)
        ))
        const unregisterBusy = split?.registerBusyCheck(() => Boolean(harness.busy[split.role]))
        onBeforeUnmount(() => {
          unregister?.()
          unregisterBusy?.()
        })
        return () => h('section', {
          class: 'stub-files files-page',
          tabindex: -1,
          'data-role': split?.role,
          'data-path': String(route.query.path ?? ''),
          'data-host': String(route.query.hostId ?? ''),
          'data-active': String(active?.value),
        }, [
          split?.open.value
            ? h('button', { class: 'close-pane', onClick: () => split.closePane() })
            : split?.role === 'primary' && split.available.value
              ? h('button', { class: 'open-split', onClick: () => split.openSplit() })
              : null,
          h('button', { class: 'go-var', onClick: () => router.push({ name: 'files', query: { path: '/var' } }) }),
          h('button', { class: 'go-cluster', onClick: () => router.push({ name: 'cluster' }) }),
        ])
      },
    }),
  }
})

let resize: ((entries: { contentRect: { width: number } }[]) => void) | undefined

class TestResizeObserver {
  constructor(callback: typeof resize) {
    resize = callback
  }

  observe(): void {}
  disconnect(): void {}
}

async function setWidth(width: number): Promise<void> {
  resize?.([{ contentRect: { width } }])
  await flushPromises()
}

const Blank = { render: () => null }

async function mountWorkspace(path = '/files?path=/srv&hostId=h1'): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/files', name: 'files', component: Blank },
      { path: '/cluster', name: 'cluster', component: Blank },
    ],
  })
  await router.push(path)
  const wrapper = mount(FilesSplitWorkspace, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

function pane(wrapper: VueWrapper, role: 'primary' | 'secondary') {
  return wrapper.find(`.stub-files[data-role="${role}"]`)
}

describe('FilesSplitWorkspace', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    window.localStorage.clear()
    harness.blockClose = false
    harness.blockPrimaryClose = false
    harness.busy = { primary: false, secondary: false }
    harness.toast.mockReset()
    resize = undefined
    vi.stubGlobal('ResizeObserver', TestResizeObserver)
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.unstubAllGlobals()
  })

  it('offers a second, independent pane only on a wide workspace', async () => {
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    expect(pane(wrapper, 'primary').find('.open-split').exists()).toBe(false)

    await setWidth(1400)
    await pane(wrapper, 'primary').get('.open-split').trigger('click')
    await flushPromises()

    const secondary = pane(wrapper, 'secondary')
    expect(secondary.attributes('data-path')).toBe('/srv')
    expect(secondary.attributes('data-host')).toBe('h1')
    expect(wrapper.get('.files-workspace').classes()).toContain('files-workspace--split')
    // 1400px splits into two ~692px panes: the compact list without owner/permission columns.
    expect(wrapper.findAll('.files-pane--compact')).toHaveLength(2)
    // ~692px cannot hold host, path and search on one toolbar row.
    expect(wrapper.findAll('.files-pane--toolbar-stacked')).toHaveLength(2)
    await setWidth(1800)
    expect(wrapper.findAll('.files-pane--toolbar-stacked')).toHaveLength(0)
    expect(wrapper.findAll('[role="region"]').map((region) => region.attributes('aria-label')))
      .toEqual(['主文件栏', '第二文件栏'])

    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
    await secondary.get('.go-var').trigger('click')
    await flushPromises()
    expect(pane(wrapper, 'secondary').attributes('data-path')).toBe('/var')
    expect(scrollTo).not.toHaveBeenCalled()
    expect(mounted.router.currentRoute.value.fullPath).toBe('/files?path=/srv&hostId=h1')
    expect(JSON.parse(window.localStorage.getItem('kpanel:files:split:v1') || '{}'))
      .toEqual({ open: true, secondaryPath: '/files?path=%2Fvar' })
  })

  it('gives keyboard shortcuts to the pane the user last touched', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({ open: true }))
    wrapper = (await mountWorkspace()).wrapper
    await setWidth(1400)

    expect(pane(wrapper, 'primary').attributes('data-active')).toBe('true')
    expect(pane(wrapper, 'secondary').attributes('data-active')).toBe('false')
    await pane(wrapper, 'secondary').trigger('pointerdown')
    expect(pane(wrapper, 'primary').attributes('data-active')).toBe('false')
    expect(pane(wrapper, 'secondary').attributes('data-active')).toBe('true')
    expect(wrapper.find('.files-pane--active .stub-files').attributes('data-role')).toBe('secondary')
  })

  it('restores the saved pane and keeps it mounted when the workspace narrows', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({
      open: true,
      secondaryPath: '/files?path=/opt&file=/opt/notes.txt',
    }))
    wrapper = (await mountWorkspace()).wrapper
    expect(pane(wrapper, 'secondary').exists()).toBe(false)

    await setWidth(1400)
    expect(pane(wrapper, 'secondary').attributes('data-path')).toBe('/opt')

    await setWidth(960)
    expect(wrapper.findAll('.files-pane--narrow')).toHaveLength(2)

    await setWidth(700)
    expect(pane(wrapper, 'secondary').exists()).toBe(true)
    expect(wrapper.get('.files-workspace').classes()).toContain('files-workspace--stacked')
    expect(pane(wrapper, 'primary').find('.close-pane').exists()).toBe(true)
    expect(pane(wrapper, 'secondary').find('.close-pane').exists()).toBe(true)
  })

  it('runs the pane close guard before removing the second pane', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({ open: true }))
    wrapper = (await mountWorkspace()).wrapper
    await setWidth(1400)

    harness.blockClose = true
    await pane(wrapper, 'secondary').get('.close-pane').trigger('click')
    await flushPromises()
    expect(pane(wrapper, 'secondary').exists()).toBe(true)

    harness.blockClose = false
    await pane(wrapper, 'secondary').get('.close-pane').trigger('click')
    await flushPromises()
    expect(pane(wrapper, 'secondary').exists()).toBe(false)
    expect(pane(wrapper, 'primary').attributes('data-active')).toBe('true')
    expect(JSON.parse(window.localStorage.getItem('kpanel:files:split:v1') || '{}').open).toBe(false)
  })

  it('closes the main pane by moving it to the second pane location', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({
      open: true,
      secondaryPath: '/files?path=/opt&hostId=h2',
    }))
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    await setWidth(1400)

    await pane(wrapper, 'primary').get('.close-pane').trigger('click')
    await flushPromises()

    expect(pane(wrapper, 'secondary').exists()).toBe(false)
    expect(mounted.router.currentRoute.value.fullPath).toBe('/files?path=/opt&hostId=h2')
    expect(pane(wrapper, 'primary').attributes('data-path')).toBe('/opt')
    expect(pane(wrapper, 'primary').attributes('data-host')).toBe('h2')
    expect(pane(wrapper, 'primary').find('.open-split').exists()).toBe(true)
    expect(document.activeElement).toBe(pane(wrapper, 'primary').element)
  })

  it('keeps both panes when the primary unsaved-work guard rejects closing', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({
      open: true, secondaryPath: '/files?path=/opt&hostId=h2',
    }))
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    await setWidth(1400)
    harness.blockPrimaryClose = true

    await pane(wrapper, 'primary').get('.close-pane').trigger('click')
    await flushPromises()

    expect(pane(wrapper, 'secondary').exists()).toBe(true)
    expect(mounted.router.currentRoute.value.fullPath).toBe('/files?path=/srv&hostId=h1')
    expect(JSON.parse(window.localStorage.getItem('kpanel:files:split:v1') || '{}').open).toBe(true)
  })

  it('keeps both panes when navigation to the remaining pane is cancelled', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({
      open: true, secondaryPath: '/files?path=/opt&hostId=h2',
    }))
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    await setWidth(1400)
    mounted.router.beforeEach(() => false)

    await pane(wrapper, 'primary').get('.close-pane').trigger('click')
    await flushPromises()

    expect(pane(wrapper, 'secondary').exists()).toBe(true)
    expect(mounted.router.currentRoute.value.fullPath).toBe('/files?path=/srv&hostId=h1')
  })

  it('refuses to close a pane while it would interrupt uploads or transfers', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({ open: true }))
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    await setWidth(1400)

    harness.busy.secondary = true
    await pane(wrapper, 'secondary').get('.close-pane').trigger('click')
    await pane(wrapper, 'primary').get('.close-pane').trigger('click')
    await flushPromises()
    expect(pane(wrapper, 'secondary').exists()).toBe(true)
    expect(harness.toast).toHaveBeenCalledTimes(2)

    harness.busy = { primary: true, secondary: false }
    await pane(wrapper, 'secondary').get('.close-pane').trigger('click')
    await flushPromises()
    // The main pane keeps running; only the second FilesView is removed.
    expect(pane(wrapper, 'secondary').exists()).toBe(false)
    expect(mounted.router.currentRoute.value.fullPath).toBe('/files?path=/srv&hostId=h1')
  })

  it('hands navigation outside the file manager to the page', async () => {
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({ open: true }))
    const mounted = await mountWorkspace()
    wrapper = mounted.wrapper
    await setWidth(1400)

    await pane(wrapper, 'secondary').get('.go-cluster').trigger('click')
    await flushPromises()
    expect(mounted.router.currentRoute.value.path).toBe('/cluster')
  })
})

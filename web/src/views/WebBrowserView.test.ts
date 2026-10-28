// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import WebBrowserView from './WebBrowserView.vue'
import { api } from '@/lib/api'
import { browserLinksKey, browserLaunchKey } from '@/lib/webBrowser'
import { desktopWindowTitlebarTargetKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'

const vncClients = vi.hoisted(() => [] as Array<{ canvas: HTMLCanvasElement; dispatchEvent(event: Event): boolean }>)
vi.mock('@novnc/novnc', () => ({ default: class extends EventTarget {
  canvas: HTMLCanvasElement
  viewOnly = false
  scaleViewport = false
  resizeSession = false
  constructor(target: HTMLElement) { super(); this.canvas = document.createElement('canvas'); target.append(this.canvas); vncClients.push(this) }
  disconnect() { this.canvas.remove() }
} }))
vi.mock('@/lib/api', () => ({ api: { browser: { status: vi.fn(), csrf: () => 'test-csrf' }, preferences: { get: vi.fn() } } }))
class Socket {
  static OPEN = 1
  static instances: Socket[] = []
  readyState = 1
  bufferedAmount = 0
  sent: Record<string, unknown>[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  constructor(public url: string) { Socket.instances.push(this) }
  send(data: string) { this.sent.push(JSON.parse(data)) }
  close() { this.readyState = 3 }
  state(tabs: unknown[] = [], active = '') { this.onmessage?.({ data: JSON.stringify({ type: 'state', tabs, active }) }) }
}
describe('desktop remote browser', () => {
  beforeEach(() => {
    const contexts = new WeakMap<HTMLCanvasElement, CanvasRenderingContext2D>()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function (this: HTMLCanvasElement) {
      if (!contexts.has(this)) contexts.set(this, { drawImage() {} } as unknown as CanvasRenderingContext2D)
      return contexts.get(this)!
    })
    vi.mocked(api.preferences.get).mockResolvedValue({ resourceVersion: 'test-version' })
    Socket.instances = []; vncClients.length = 0
    vi.stubGlobal('WebSocket', Socket)
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.mocked(api.browser.status).mockResolvedValue({ ready: true, maxTabs: 8 })
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
  async function render(visible = ref(true), url?: string, titlebar?: HTMLElement) {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/browser', component: { template: '<div />' } }] })
    await router.push('/browser')
    const wrapper = mount(WebBrowserView, { global: { plugins: [router], provide: { [desktopWindowTitlebarTargetKey as symbol]: ref(titlebar), [desktopWindowVisibleKey as symbol]: visible, [browserLaunchKey as symbol]: ref(url ? { url, request: 'test' } : undefined), [browserLinksKey as symbol]: ref([{ id: 'app', name: 'App', url: 'https://example.com' }]) } } })
    await flushPromises()
    return wrapper
  }
  async function attachVNC(width: number, height: number) {
    await flushPromises()
    const channel = Socket.instances.filter(item => String(item.url).includes('/browser/vnc/')).at(-1)!
    expect(channel).toBeDefined()
    channel.onmessage?.({ data: JSON.stringify({ ready: true }) })
    const client = vncClients.at(-1)!
    client.canvas.width = width; client.canvas.height = height
    client.dispatchEvent(new Event('connect'))
    client.canvas.getContext('2d')!.drawImage(client.canvas, 0, 0)
    await nextTick()
    return client.canvas
  }
  it('explains a missing optional component and keeps external opening available', async () => {
    vi.mocked(api.browser.status).mockResolvedValue({ ready: false, reason: 'browser_unconfigured', maxTabs: 8 })
    const wrapper = await render(ref(true), 'https://example.com/')
    expect(wrapper.text()).toContain('浏览器组件尚未启用')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(Socket.instances).toHaveLength(0)
    expect((wrapper.get('.web-browser__address input').element as HTMLInputElement).value).toBe('https://example.com/')
    expect(wrapper.findAll('button').some(button => button.text().includes('用系统浏览器打开') && !button.attributes('disabled'))).toBe(true)
    wrapper.unmount()
  })
  it('authenticates the socket and opens addresses through bounded commands', async () => {
    const wrapper = await render(), socket = Socket.instances[0]!
    socket.onopen?.()
    expect(socket.sent[0]).toEqual({ csrf: 'test-csrf' })
    socket.state(); await nextTick()
    await wrapper.get('.web-browser__address input').setValue('example.com')
    await wrapper.get('.web-browser__toolbar').trigger('submit')
    expect(socket.sent).toContainEqual({ type: 'new', url: 'https://example.com/' })
    expect(wrapper.text()).toContain('通过服务器访问')
    wrapper.unmount()
    expect(socket.sent).toContainEqual({ type: 'end' })
  })
  it('uses the saved start page for startup, Home and new tabs while preserving external launch intents', async () => {
    vi.mocked(api.preferences.get).mockResolvedValue({ resourceVersion: 'test-version', browser: { startURL: 'https://start.example/' } })
    const wrapper = await render(), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    expect(socket.sent).toContainEqual({ type: 'new', url: 'https://start.example/' })
    socket.state([{ id: 'tab-1', url: 'https://start.example/', title: 'Start', loading: false, back: false, forward: false }], 'tab-1'); await nextTick()
    await wrapper.get('button[aria-label="起始页"]').trigger('click')
    expect(socket.sent.at(-1)).toMatchObject({ type: 'navigate', tab: 'tab-1', url: 'https://start.example/' })
    await wrapper.get('button[aria-label="新建标签页"]').trigger('click')
    expect(socket.sent).toContainEqual({ type: 'new', url: 'https://start.example/' })
    wrapper.unmount()
    const launched = await render(ref(true), 'https://app.example/')
    const other = Socket.instances.at(-1)!; other.state(); await nextTick()
    expect(other.sent).toContainEqual({ type: 'new', url: 'https://app.example/' })
    expect(other.sent.some(command => command.url === 'https://start.example/')).toBe(false)
    launched.unmount()
  })
  it('integrates the eight-tab strip into the window titlebar', async () => {
    const target = document.createElement('div'); document.body.append(target)
    const wrapper = await render(ref(true), undefined, target)
    const socket = Socket.instances[0]!
    socket.state(Array.from({ length: 8 }, (_, index) => ({ id: `tab-${index}`, url: 'https://example.com/', title: `Page ${index}`, loading: false, back: false, forward: false })), 'tab-0')
    await nextTick()
    expect(target.querySelectorAll('[role="tab"]')).toHaveLength(8)
    expect(target.textContent).toContain('8/8')
    expect(wrapper.find('.web-browser__tabs').exists()).toBe(false)
    expect(target.querySelector<HTMLButtonElement>('button[title="新建标签页"]')?.disabled).toBe(true)
    await wrapper.get('button[title="起始页"]').trigger('click')
    await wrapper.get('.web-browser__address input').setValue('example.org')
    await wrapper.get('.web-browser__toolbar').trigger('submit')
    expect(socket.sent).toContainEqual({ type: 'navigate', tab: 'tab-0', url: 'https://example.org/' })
    wrapper.unmount(); expect(target.children).toHaveLength(0); target.remove()
  })
  it('keeps one add button and lets every tab, including the start tab, close', async () => {
    const wrapper = await render(), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    expect(wrapper.findAll('button[title="新建标签页"]')).toHaveLength(1)
    await wrapper.get('.web-browser__address input').setValue('unfinished.example')
    await wrapper.get('button[aria-label="关闭标签页"]').trigger('click')
    expect((wrapper.get('.web-browser__address input').element as HTMLInputElement).value).toBe('')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    socket.state([{ id: 'first', url: 'https://example.com/', title: 'First', loading: false, back: false, forward: false }], 'first')
    await nextTick(); await wrapper.get('[role="tab"]').trigger('click')
    await wrapper.get('button[title="新建标签页"]').trigger('click')
    expect(wrapper.findAll('.web-browser__tab')).toHaveLength(2)
    expect(wrapper.findAll('.web-browser__tab').every(tab => tab.findAll('button').length === 2)).toBe(true)
    await wrapper.get('button[aria-label="关闭标签页"]').trigger('click')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    expect(socket.sent).toContainEqual({ type: 'select', tab: 'first' })
    await wrapper.get('button[aria-label="关闭标签页 · First"]').trigger('click')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    socket.state([{ id: 'first', url: 'https://example.com/', title: 'First', loading: false, back: false, forward: false }], 'first')
    await nextTick()
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    expect(wrapper.get('[role="tab"]').text()).toContain('新标签页')
    await wrapper.get('.web-browser__address input').setValue('unfinished.example')
    await wrapper.get('button[aria-label="关闭标签页"]').trigger('click')
    expect((wrapper.get('.web-browser__address input').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })
  it('keeps page titles as text and pauses transmission when the desktop window is hidden', async () => {
    const visible = ref(true), wrapper = await render(visible), socket = Socket.instances[0]!
    socket.state([{ id: 'tab-1', url: 'https://example.com', title: '<img src=x>', loading: false, back: true, forward: false }], 'tab-1')
    await nextTick()
    await wrapper.get('[role="tab"]').trigger('click')
    expect(wrapper.find('[role="tab"] img').exists()).toBe(false)
    expect(wrapper.get('[role="tab"]').text()).toContain('<img src=x>')
    visible.value = false; await nextTick()
    expect(socket.sent).toContainEqual({ type: 'visible', visible: false })
    expect(socket.readyState).toBe(1)
    wrapper.unmount()
  })
  it('scales wheel units to the remote viewport and resizes when a window returns', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ close() {} }))
    const visible = ref(true), wrapper = await render(visible, 'https://example.com/'), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    socket.state([{ id: 'tab-1', url: 'https://example.com/', title: 'Page', loading: false, back: false, forward: false }], 'tab-1')
    await nextTick()
    await attachVNC(1600, 1000)
    const canvas = wrapper.get('.web-browser__vnc canvas')
    vi.spyOn(canvas.element, 'getBoundingClientRect').mockReturnValue({ left: 0, top: 0, width: 800, height: 500 } as DOMRect)
    vi.spyOn(wrapper.get('.web-browser__content').element, 'getBoundingClientRect').mockReturnValue({ width: 1920, height: 1080 } as DOMRect)
    for (const [deltaMode, deltaY, expected] of [[0, 120, 240], [1, 3, 96], [2, 1, 1000]]) {
      wrapper.get('.web-browser__frame').element.dispatchEvent(new WheelEvent('wheel', { clientX: 200, clientY: 100, deltaMode, deltaX: 0, deltaY, cancelable: true }))
      await vi.advanceTimersByTimeAsync(25)
      expect(socket.sent.at(-1)).toMatchObject({ type: 'mouse', action: 'mouseWheel', x: 400, y: 200, deltaX: 0, deltaY: expected })
    }
    visible.value = false; await nextTick()
    visible.value = true; await nextTick(); await nextTick()
    await vi.advanceTimersByTimeAsync(140)
    expect(socket.sent.at(-1)).toEqual({ type: 'resize', width: 1600, height: 900 })
    wrapper.unmount()
  })
  it('uses a mobile tab overview that retains capture and preserves selection and capacity', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener() {}, removeEventListener() {} })))
    const visible = ref(true), wrapper = await render(visible, 'https://example.com/'), socket = Socket.instances[0]!
    document.body.append(wrapper.element)
    socket.state(); await nextTick()
    const pages = Array.from({ length: 2 }, (_, index) => ({ id: `tab-${index}`, url: `https://example.com/${index}`, title: `Page ${index}`, loading: false, back: false, forward: false }))
    socket.state(pages, 'tab-0'); await nextTick()
    expect(wrapper.find('.web-browser__tabs').exists()).toBe(false)
    expect(wrapper.get('.web-browser__tab-switch').attributes('aria-label')).toBe('标签页 · 2/8')
    const canvas = await attachVNC(1280, 720)
    await wrapper.get('.web-browser__tab-switch').trigger('click'); await flushPromises()
    expect(socket.sent).not.toContainEqual({ type: 'visible', visible: false })
    expect(wrapper.get('.web-browser__vnc canvas').element).toBe(canvas)
    const choices = document.querySelectorAll<HTMLButtonElement>('.web-browser__tab-choice')
    expect(choices).toHaveLength(2)
    choices[1]!.click(); await flushPromises()
    expect(socket.sent).toContainEqual({ type: 'select', tab: 'tab-1' })
    expect(socket.sent).toContainEqual({ type: 'visible', visible: true })
    expect(document.querySelector('.web-browser__tab-choice')).toBeNull()
    socket.state(Array.from({ length: 8 }, (_, index) => ({ ...pages[0]!, id: `tab-${index}` })), 'tab-1'); await nextTick()
    await wrapper.get('.web-browser__tab-switch').trigger('click'); await flushPromises()
    const add = [...document.querySelectorAll<HTMLButtonElement>('.modal-panel__footer button')].find(button => button.textContent?.includes('新建标签页'))
    expect(add?.disabled).toBe(true)
    wrapper.unmount()
  })
  it('scrolls a touch drag without clicking or focusing the keyboard and turns a tap into one click', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener() {}, removeEventListener() {} })))
    vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ close() {} }))
    const wrapper = await render(ref(true), 'https://example.com/'), socket = Socket.instances[0]!
    document.body.append(wrapper.element)
    socket.state(); await nextTick()
    socket.state([{ id: 'tab-1', url: 'https://example.com/', title: 'Page', loading: false, back: false, forward: false }], 'tab-1'); await nextTick()
    await attachVNC(1600, 1000)
    const canvas = wrapper.get('.web-browser__vnc canvas')
    vi.spyOn(canvas.element, 'getBoundingClientRect').mockReturnValue({ left: 0, top: 0, width: 800, height: 500 } as DOMRect)
    const dispatch = (type: string, x: number, y: number) => {
      const event = new Event(type, { cancelable: true })
      for (const [name, value] of Object.entries({ pointerType: 'touch', pointerId: 1, clientX: x, clientY: y })) Object.defineProperty(event, name, { value })
      wrapper.get('.web-browser__frame').element.dispatchEvent(event)
    }
    socket.sent.length = 0
    dispatch('pointerdown', 150, 300); dispatch('pointermove', 150, 180); dispatch('pointerup', 150, 180)
    expect(socket.sent.some(command => command.action === 'mouseWheel' && command.deltaY === 240)).toBe(true)
    expect(socket.sent.some(command => command.action === 'mousePressed')).toBe(false)
    expect(document.activeElement).not.toBe(wrapper.get('textarea').element)
    dispatch('pointerdown', 150, 200); dispatch('pointerup', 150, 200)
    expect(socket.sent.slice(-2)).toEqual([{ tab: 'tab-1', type: 'mouse', action: 'mousePressed', x: 300, y: 400, button: 'left' }, { tab: 'tab-1', type: 'mouse', action: 'mouseReleased', x: 300, y: 400, button: 'left' }])
    expect(document.activeElement).toBe(wrapper.get('textarea').element)
    socket.sent.length = 0
    dispatch('pointerdown', 150, 200); dispatch('pointercancel', 150, 200); dispatch('pointerup', 150, 200)
    expect(socket.sent).toHaveLength(0)
    wrapper.unmount()
  })
  it('forwards a soft-keyboard return once and leaves composition commits alone', async () => {
    const wrapper = await render(ref(true), 'https://example.com/'), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    socket.state([{ id: 'tab-1', url: 'https://example.com/', title: 'Page', loading: false, back: false, forward: false }], 'tab-1'); await nextTick()
    await attachVNC(1280, 720)
    const input = wrapper.get('textarea')
    socket.sent.length = 0
    const enter = new InputEvent('beforeinput', { inputType: 'insertLineBreak', cancelable: true })
    input.element.dispatchEvent(enter)
    expect(enter.defaultPrevented).toBe(true)
    expect(socket.sent).toEqual([{ tab: 'tab-1', type: 'key', action: 'keyDown', key: 'Enter', code: 'Enter' }, { tab: 'tab-1', type: 'key', action: 'keyUp', key: 'Enter', code: 'Enter' }])
    socket.sent.length = 0
    await input.trigger('compositionstart')
    input.element.dispatchEvent(new InputEvent('beforeinput', { inputType: 'insertParagraph', cancelable: true }))
    expect(socket.sent).toHaveLength(0)
    wrapper.unmount()
  })
  it('waits for the resized framebuffer before mapping pointer input', async () => {
    vi.useFakeTimers()
    const visible = ref(true), wrapper = await render(visible, 'https://example.com/'), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    socket.state([{ id: 'tab-1', url: 'https://example.com/', title: 'Page', loading: false, back: false, forward: false }], 'tab-1'); await nextTick()
    const canvas = await attachVNC(1600, 900)
    const box = { left: 0, top: 0, width: 800, height: 500 } as DOMRect
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue(box)
    vi.spyOn(wrapper.get('.web-browser__content').element, 'getBoundingClientRect').mockReturnValue(box)
    visible.value = false; await nextTick(); visible.value = true; await nextTick(); await nextTick()
    await vi.advanceTimersByTimeAsync(140)
    expect(socket.sent).toContainEqual({ type: 'resize', width: 800, height: 500 })
    const nextCanvas = await attachVNC(1600, 900)
    vi.spyOn(nextCanvas, 'getBoundingClientRect').mockReturnValue(box)
    socket.sent.length = 0
    const pointer = (type: string) => { const event = new Event(type, { cancelable: true }); for (const [key, value] of Object.entries({ pointerType: 'mouse', clientX: 799, clientY: 499, button: 0, altKey: false, ctrlKey: false, metaKey: false, shiftKey: false })) Object.defineProperty(event, key, { value }); wrapper.get('.web-browser__frame').element.dispatchEvent(event) }
    pointer('pointermove')
    expect(socket.sent).toHaveLength(0)
    pointer('pointerup')
    expect(socket.sent.at(-1)).toMatchObject({ action: 'mouseReleased', x: 799, y: 499 })
    nextCanvas.width = 800; nextCanvas.height = 500; await nextTick(); await vi.advanceTimersByTimeAsync(40)
    pointer('pointermove')
    await vi.advanceTimersByTimeAsync(25)
    expect(socket.sent.at(-1)).toMatchObject({ action: 'mouseMoved', x: 799, y: 499 })
    wrapper.unmount()
  })
  it('retains a frozen frame and waits for a fresh paint after closing the active tab', async () => {
    const wrapper = await render(ref(true), 'https://example.com/'), socket = Socket.instances[0]!
    const tabs = [{ id: 'first', url: 'https://example.com/1', title: 'First', loading: false, back: false, forward: false }, { id: 'second', url: 'https://example.com/2', title: 'Second', loading: false, back: false, forward: false }]
    socket.state(); await nextTick(); socket.state(tabs, 'second'); await nextTick()
    const oldCanvas = await attachVNC(1280, 720)
    await wrapper.get('button[aria-label="关闭标签页 · Second"]').trigger('click')
    expect(wrapper.get('.web-browser__previous').isVisible()).toBe(true)
    socket.state([tabs[0]], 'first'); await flushPromises()
    const channel = Socket.instances.filter(item => String(item.url).includes('/browser/vnc/')).at(-1)!
    channel.onmessage?.({ data: JSON.stringify({ ready: true }) })
    const client = vncClients.at(-1)!
    client.dispatchEvent(new Event('connect')); await nextTick()
    socket.sent.length = 0
    await wrapper.get('textarea').setValue('must not reach the unseen tab')
    expect(socket.sent).toHaveLength(0)
    client.canvas.width = 1280; client.canvas.height = 720
    client.canvas.getContext('2d')!.drawImage(client.canvas, 0, 0); await nextTick()
    expect((wrapper.get('.web-browser__previous').element as HTMLCanvasElement).style.display).toBe('none')
    expect(wrapper.get('.web-browser__vnc canvas').element).not.toBe(oldCanvas)
    await wrapper.get('textarea').setValue('fresh')
    expect(socket.sent.at(-1)).toMatchObject({ type: 'text', tab: 'first', text: 'fresh' })
    wrapper.unmount()
  })
  it('restores the display when the control connection resumes the same tab', async () => {
    const wrapper = await render(ref(true), 'https://example.com/'), socket = Socket.instances[0]!
    const tabs = [{ id: 'first', url: 'https://example.com/', title: 'First', loading: false, back: false, forward: false }]
    socket.state(); await nextTick(); socket.state(tabs, 'first'); await nextTick()
    const oldCanvas = await attachVNC(1280, 720)
    socket.onclose?.(); await nextTick()
    expect(oldCanvas.parentElement).toBeNull()
    const retry = wrapper.findAll('button').find(button => button.text() === '重新连接')!
    await retry.trigger('click'); await flushPromises()
    const resumed = Socket.instances.filter(item => String(item.url).includes('/browser/stream/')).at(-1)!
    expect(resumed).not.toBe(socket)
    resumed.onmessage?.({ data: JSON.stringify({ type: 'error', code: 'browser_action_failed' }) }); await nextTick()
    resumed.state(tabs, 'first'); await nextTick()
    const newCanvas = await attachVNC(1280, 720)
    expect(newCanvas).not.toBe(oldCanvas)
    wrapper.unmount()
  })
  it('ignores clicks and scrolling outside the scaled framebuffer', async () => {
    const wrapper = await render(ref(true), 'https://example.com/'), socket = Socket.instances[0]!
    socket.state(); await nextTick()
    socket.state([{ id: 'first', url: 'https://example.com/', title: 'First', loading: false, back: false, forward: false }], 'first'); await nextTick()
    const canvas = await attachVNC(1280, 720)
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({ left: 100, top: 50, width: 600, height: 400 } as DOMRect)
    socket.sent.length = 0
    const down = new Event('pointerdown', { cancelable: true })
    for (const [key, value] of Object.entries({ pointerType: 'mouse', clientX: 20, clientY: 100, button: 0 })) Object.defineProperty(down, key, { value })
    wrapper.get('.web-browser__frame').element.dispatchEvent(down)
    wrapper.get('.web-browser__frame').element.dispatchEvent(new WheelEvent('wheel', { clientX: 20, clientY: 100, deltaY: 100, cancelable: true }))
    expect(socket.sent).toHaveLength(0)
    wrapper.unmount()
  })
})

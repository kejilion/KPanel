// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { useDocumentFullscreen } from './useDocumentFullscreen'

type FullscreenState = ReturnType<typeof useDocumentFullscreen>

function mountFullscreen() {
  let fullscreen: FullscreenState | undefined
  const wrapper = mount(defineComponent({
    setup() {
      fullscreen = useDocumentFullscreen()
      return () => h('div')
    },
  }))
  return { wrapper, fullscreen: fullscreen! }
}

function mockFullscreenEnvironment() {
  let fullscreenElement: Element | null = null
  const requestFullscreen = vi.fn(async () => {
    fullscreenElement = document.documentElement
    document.dispatchEvent(new Event('fullscreenchange'))
  })
  const exitFullscreen = vi.fn(async () => {
    fullscreenElement = null
    document.dispatchEvent(new Event('fullscreenchange'))
  })
  Object.defineProperties(document, {
    fullscreenEnabled: { configurable: true, value: true },
    fullscreenElement: { configurable: true, get: () => fullscreenElement },
    exitFullscreen: { configurable: true, value: exitFullscreen },
  })
  Object.defineProperty(document.documentElement, 'requestFullscreen', {
    configurable: true,
    value: requestFullscreen,
  })
  return { requestFullscreen, exitFullscreen }
}

afterEach(() => {
  Object.defineProperties(document, {
    fullscreenEnabled: { configurable: true, value: false },
    fullscreenElement: { configurable: true, value: null },
    exitFullscreen: { configurable: true, value: undefined },
  })
  Object.defineProperty(document.documentElement, 'requestFullscreen', {
    configurable: true,
    value: undefined,
  })
})

describe('document fullscreen', () => {
  it('tracks successful browser fullscreen entry and exit', async () => {
    const browser = mockFullscreenEnvironment()
    const { wrapper, fullscreen } = mountFullscreen()

    expect(fullscreen.supported.value).toBe(true)
    expect(fullscreen.active.value).toBe(false)
    await expect(fullscreen.enter()).resolves.toBe(true)
    expect(browser.requestFullscreen).toHaveBeenCalledOnce()
    expect(fullscreen.active.value).toBe(true)

    await expect(fullscreen.exit()).resolves.toBe(true)
    expect(browser.exitFullscreen).toHaveBeenCalledOnce()
    expect(fullscreen.active.value).toBe(false)
    wrapper.unmount()
  })

  it('keeps the viewport mode available when fullscreen is unsupported', async () => {
    const { wrapper, fullscreen } = mountFullscreen()
    expect(fullscreen.supported.value).toBe(false)
    await expect(fullscreen.enter()).resolves.toBe(false)
    expect(fullscreen.active.value).toBe(false)
    wrapper.unmount()
  })

  it('contains browser request rejections', async () => {
    mockFullscreenEnvironment()
    const rejection = vi.fn().mockRejectedValue(new DOMException('Denied', 'NotAllowedError'))
    Object.defineProperty(document.documentElement, 'requestFullscreen', {
      configurable: true,
      value: rejection,
    })
    const { wrapper, fullscreen } = mountFullscreen()

    await expect(fullscreen.enter()).resolves.toBe(false)
    expect(rejection).toHaveBeenCalledOnce()
    expect(fullscreen.active.value).toBe(false)
    wrapper.unmount()
  })
})

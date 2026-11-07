// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import CinemaPlayer from './CinemaPlayer.vue'

let view: ReturnType<typeof mount> | undefined
const props = { src: '/film.mp4', title: 'Film', backLabel: 'Back to details', fullscreenLabel: 'Fullscreen', fallbackLabel: 'Use fullscreen', errorLabel: 'Cannot play' }
beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', { configurable: true, writable: true, value: async () => {} })
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
})
afterEach(() => { view?.unmount(); view = undefined; document.body.innerHTML = ''; vi.restoreAllMocks(); delete (document as unknown as Record<string, unknown>).fullscreenElement })

it('requests fullscreen from mount and returns to details when fullscreen ends', async () => {
  let fullscreen: Element | null = null
  Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => fullscreen })
  const request = vi.spyOn(HTMLElement.prototype, 'requestFullscreen').mockImplementation(async function (this: HTMLElement) {
    fullscreen = this
    document.dispatchEvent(new Event('fullscreenchange'))
  })
  view = mount(CinemaPlayer, { props, attachTo: document.body })
  await flushPromises()
  expect(request).toHaveBeenCalledOnce()
  expect(document.querySelector('video')?.autoplay).toBe(true)
  fullscreen = null
  document.dispatchEvent(new Event('fullscreenchange'))
  expect(view.emitted('close')).toHaveLength(1)
})

it('keeps a working exit and fullscreen retry after browser rejection', async () => {
  vi.spyOn(HTMLElement.prototype, 'requestFullscreen').mockRejectedValue(new Error('denied'))
  view = mount(CinemaPlayer, { props, attachTo: document.body })
  await flushPromises()
  expect(document.querySelector('[role="status"]')?.textContent).toBe(props.fallbackLabel)
  document.querySelector<HTMLElement>('.cinema-player')?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  expect(view.emitted('close')).toHaveLength(1)
  document.querySelector('video')?.dispatchEvent(new Event('error'))
  await flushPromises()
  expect(document.querySelector('[role="alert"]')?.textContent).toBe(props.errorLabel)
  view.unmount(); view = undefined
  expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled()
  expect(document.body.classList.contains('has-modal')).toBe(false)
})

// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import CinemaPlayer from './CinemaPlayer.vue'

let view: VueWrapper | undefined, full: Element | null = null
const labels = { play: 'Play', pause: 'Pause', loading: 'Loading', retry: 'Retry', blocked: 'Press play', rewind: 'Back 10', forward: 'Forward 10', mute: 'Mute', unmute: 'Unmute', volume: 'Volume', seek: 'Position', speed: 'Speed', exitFullscreen: 'Exit fullscreen' }
const props = { src: '/film.mp4', title: 'Film', backLabel: 'Back to details', fullscreenLabel: 'Fullscreen', fallbackLabel: 'Use fullscreen', errorLabel: 'Cannot play', labels }
const element = () => document.querySelector<HTMLVideoElement>('video')!
function load(position = 0) {
  const v = element()
  Object.defineProperty(v, 'duration', { configurable: true, value: 120 })
  Object.defineProperty(v, 'videoWidth', { configurable: true, value: 1920 })
  v.dispatchEvent(new Event('loadedmetadata'))
  if (position) v.currentTime = position
  v.dispatchEvent(new Event('loadeddata'))
  return v
}
beforeEach(() => {
  full = null
  Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => full })
  Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', { configurable: true, writable: true, value: vi.fn(async function(this: HTMLElement) { full = this; document.dispatchEvent(new Event('fullscreenchange')) }) })
  Object.defineProperty(document, 'exitFullscreen', { configurable: true, writable: true, value: vi.fn(async () => { full = null; document.dispatchEvent(new Event('fullscreenchange')) }) })
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue()
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
})
afterEach(() => { view?.unmount(); view = undefined; document.body.innerHTML = ''; vi.restoreAllMocks(); vi.useRealTimers() })

it('requests fullscreen but keeps playback when fullscreen ends; explicit back exits', async () => {
  view = mount(CinemaPlayer, { props, attachTo: document.body }); await flushPromises()
  expect(HTMLElement.prototype.requestFullscreen).toHaveBeenCalledOnce()
  expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce()
  load(30)
  await document.exitFullscreen(); await flushPromises()
  expect(view.emitted('close')).toBeUndefined()
  expect(element().currentTime).toBe(30)
  document.querySelector<HTMLButtonElement>('[data-action=back]')!.click()
  expect(view.emitted('close')).toHaveLength(1)
})

it('restores before the first frame checkpoint, throttles ticks, and flushes pause/end', async () => {
  vi.useFakeTimers()
  view = mount(CinemaPlayer, { props: { ...props, startPosition: 42 }, attachTo: document.body })
  const v = load()
  expect(v.currentTime).toBe(42)
  expect(view.emitted('progress')?.at(-1)?.[0]).toMatchObject({ position: 42, completed: false })
  const count = view.emitted('progress')!.length
  v.currentTime = 51; v.dispatchEvent(new Event('timeupdate'))
  expect(view.emitted('progress')).toHaveLength(count)
  v.dispatchEvent(new Event('pause'))
  expect(view.emitted('progress')?.at(-1)).toEqual([{ position: 51, duration: 120, completed: false }, true])
  v.currentTime = 120; v.dispatchEvent(new Event('ended'))
  expect(view.emitted('progress')?.at(-1)?.[0]).toMatchObject({ completed: true })
  view.unmount(); view = undefined
  expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled()
  expect(document.body.classList.contains('has-modal')).toBe(false)
})

it('keeps exit/retry usable after fullscreen refusal and restores retry position', async () => {
  vi.mocked(HTMLElement.prototype.requestFullscreen).mockRejectedValue(new Error('denied'))
  view = mount(CinemaPlayer, { props, attachTo: document.body }); await flushPromises()
  const v = load(35); v.dispatchEvent(new Event('timeupdate'))
  v.dispatchEvent(new Event('error')); await flushPromises()
  expect(document.querySelector('[role=alert]')?.textContent).toContain(props.errorLabel)
  document.querySelector<HTMLButtonElement>('[role=alert] button')!.click()
  load()
  expect(v.currentTime).toBe(35)
  expect(document.querySelector('.cinema-player__notice')?.textContent).toBe(props.fallbackLabel)
  document.querySelector<HTMLElement>('.cinema-player')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  expect(view.emitted('close')).toHaveLength(1)
})

it('supports seek, volume, mute and speed without borrowing the native player UI', async () => {
  view = mount(CinemaPlayer, { props, attachTo: document.body }); await flushPromises()
  const v = load(30), root = document.querySelector<HTMLElement>('.cinema-player')!
  root.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
  expect(v.currentTime).toBe(40)
  root.dispatchEvent(new KeyboardEvent('keydown', { key: 'm', bubbles: true }))
  expect(v.muted).toBe(true)
  const select = document.querySelector('select')!; select.value = '1.5'; select.dispatchEvent(new Event('change', { bubbles: true }))
  expect(v.playbackRate).toBe(1.5)
  expect(v.controls).toBe(false)
})

it('preserves the requested resume point when the first media request fails before metadata', async () => {
  view = mount(CinemaPlayer, { props: { ...props, startPosition: 42 }, attachTo: document.body }); await flushPromises()
  element().dispatchEvent(new Event('error')); await flushPromises()
  document.querySelector<HTMLButtonElement>('[role=alert] button')!.click()
  load()
  expect(element().currentTime).toBe(42)
})

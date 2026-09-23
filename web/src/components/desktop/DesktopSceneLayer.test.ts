// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DesktopSceneLayer from './DesktopSceneLayer.vue'
import type { SceneHostOptions } from '@/lib/desktopScenes/sceneHost'

const hosts = vi.hoisted(() => [] as Array<{
  canvas: HTMLCanvasElement
  options: SceneHostOptions
  setPaused: ReturnType<typeof vi.fn>
  dispose: ReturnType<typeof vi.fn>
}>)

vi.mock('@/lib/desktopScenes/sceneHost', () => ({
  createSceneHost: (canvas: HTMLCanvasElement, options: SceneHostOptions) => {
    const host = {
      canvas,
      options,
      mode: options.preferWorker ? 'worker' : 'inline',
      resize: vi.fn(),
      setPaused: vi.fn(),
      setReducedMotion: vi.fn(),
      dispose: vi.fn(),
    }
    hosts.push(host)
    return host
  },
}))

describe('DesktopSceneLayer', () => {
  beforeEach(() => {
    hosts.length = 0
  })

  it('remounts a fresh canvas on the main thread when the worker cannot boot', async () => {
    const wrapper = mount(DesktopSceneLayer, { props: { sceneId: 'fireflies', active: true, occluded: false } })
    await flushPromises()
    expect(hosts).toHaveLength(1)
    expect(hosts[0]!.options.preferWorker).toBe(true)
    expect(wrapper.attributes('data-scene-mode')).toBe('worker')

    hosts[0]!.options.onError('worker')
    await flushPromises()
    expect(hosts).toHaveLength(2)
    expect(hosts[1]!.options.preferWorker).toBe(false)
    expect(hosts[1]!.canvas).not.toBe(hosts[0]!.canvas)
    expect(wrapper.attributes('data-scene-mode')).toBe('inline')
    expect(wrapper.emitted('error')).toBeUndefined()

    hosts[1]!.options.onReady()
    await flushPromises()
    expect(wrapper.classes()).toContain('desktop-scene--ready')
    expect(wrapper.emitted('ready')).toEqual([['fireflies']])
    wrapper.unmount()
    expect(hosts[1]!.dispose).toHaveBeenCalled()
  })

  it('reports download failures and follows pause conditions', async () => {
    const wrapper = mount(DesktopSceneLayer, { props: { sceneId: 'aurora', active: false, occluded: false } })
    expect(hosts[0]!.options.paused).toBe(true)
    await wrapper.setProps({ active: true })
    expect(hosts[0]!.setPaused).toHaveBeenLastCalledWith(false)
    await wrapper.setProps({ occluded: true })
    expect(hosts[0]!.setPaused).toHaveBeenLastCalledWith(true)

    hosts[0]!.options.onError('load')
    await flushPromises()
    expect(wrapper.emitted('error')).toEqual([['aurora']])
    expect(hosts).toHaveLength(1)
    wrapper.unmount()
  })
})

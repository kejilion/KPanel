// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import DesktopScenePackTransition from './DesktopScenePackTransition.vue'
import { resetSceneMotionPreferenceForTest } from '@/lib/desktopScenes/motionPreference'

function mountScenes() {
  const selected = ref('a')
  const mounted: string[] = []
  const stopped: string[] = []
  const Scene = defineComponent({
    props: { id: { type: String, required: true } },
    setup(props) {
      onMounted(() => mounted.push(props.id))
      onBeforeUnmount(() => stopped.push(props.id))
      return () => h('div', { 'data-scene': props.id }, [h('iframe')])
    },
  })
  const wrapper = mount(defineComponent({
    setup: () => () => h(DesktopScenePackTransition, null, {
      default: () => selected.value ? h(Scene, { key: selected.value, id: selected.value }) : null,
    }),
  }), { attachTo: document.body, global: { stubs: { transition: false } } })
  return { wrapper, selected, mounted, stopped }
}

describe('DesktopScenePackTransition', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    window.localStorage.clear()
    vi.stubGlobal('matchMedia', undefined)
    resetSceneMotionPreferenceForTest()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    resetSceneMotionPreferenceForTest()
    document.body.innerHTML = ''
  })

  it('retains the outgoing frame until black and mounts only the latest selection', async () => {
    const { wrapper, selected, mounted, stopped } = mountScenes()
    const oldFrame = wrapper.get('iframe').element
    selected.value = 'b'
    await nextTick()
    expect(stopped).toEqual(['a'])
    expect(oldFrame.isConnected).toBe(true)
    await vi.advanceTimersByTimeAsync(300)
    selected.value = 'c'
    await nextTick()
    expect(mounted).toEqual(['a'])
    expect(oldFrame.isConnected).toBe(true)
    expect(document.body.querySelectorAll('iframe')).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(400)
    expect(oldFrame.isConnected).toBe(false)
    expect(mounted).toEqual(['a', 'c'])
    expect(wrapper.get('[data-scene]').attributes('data-scene')).toBe('c')
    wrapper.unmount()
  })

  it('finishes leaving when the scene is cleared during another switch', async () => {
    const { wrapper, selected, mounted } = mountScenes()
    selected.value = 'b'
    await nextTick()
    await vi.advanceTimersByTimeAsync(100)
    selected.value = ''
    await nextTick()
    await vi.advanceTimersByTimeAsync(600)
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(mounted).toEqual(['a'])
    wrapper.unmount()
  })

  it('skips the exit wait with system reduced motion, including a live-scene override', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn() }))
    window.localStorage.setItem('kpanel:desktop-scene-motion:v1', 'always')
    resetSceneMotionPreferenceForTest()
    const { wrapper, selected, mounted } = mountScenes()
    selected.value = 'b'
    await nextTick()
    await vi.advanceTimersByTimeAsync(50)
    expect(mounted).toEqual(['a', 'b'])
    expect(wrapper.findAll('iframe')).toHaveLength(1)
    wrapper.unmount()
  })
})

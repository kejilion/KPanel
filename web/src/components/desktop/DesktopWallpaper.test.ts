// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import DesktopWallpaper from './DesktopWallpaper.vue'
import { resetSceneMotionPreferenceForTest, useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'

const Scene = defineComponent({
  props: ['packId', 'covered'], emits: ['cameras', 'failed'],
  setup: (props) => () => h('div', { class: 'test-scene', 'data-pack': props.packId, 'data-covered': String(props.covered) }),
})
function mountWallpaper(wallpaperId: 'classic' | `pack:${string}` = 'classic') {
  return mount(DesktopWallpaper, {
    props: { wallpaperId, revision: 0, covered: false }, attachTo: document.body,
    global: { stubs: { ScenePack: Scene } },
  })
}
type Wrapper = ReturnType<typeof mountWallpaper>
const phase = (wrapper: Wrapper) => wrapper.attributes('data-wallpaper-phase')
const surface = (wrapper: Wrapper) => wrapper.get('.desktop-wallpaper-surface')
async function black(wrapper: Wrapper): Promise<void> {
  await wrapper.get('.desktop-wallpaper-handoff').trigger('transitionend', { propertyName: 'opacity' })
  await nextTick()
}

describe('shared wallpaper handoff', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    window.localStorage.clear()
    window.sessionStorage.clear()
    vi.stubGlobal('matchMedia', undefined)
    resetSceneMotionPreferenceForTest()
    document.documentElement.style.setProperty('--desktop-wallpaper-image', 'url("old-orbital.webp")')
  })
  afterEach(() => {
    vi.useRealTimers(); vi.unstubAllGlobals(); resetSceneMotionPreferenceForTest()
    document.body.innerHTML = ''
    document.documentElement.style.removeProperty('--desktop-wallpaper-image')
  })

  it('starts with its own dark poster and no black entrance, even with a different boot image', () => {
    const wrapper = mountWallpaper('pack:neon-city')
    expect(phase(wrapper)).toBe('idle')
    expect(surface(wrapper).classes()).toContain('desktop-wallpaper-surface--scene')
    expect(wrapper.get('img').attributes('src')).toContain('/neon-city/poster')
    expect(surface(wrapper).attributes('style')).not.toContain('old-orbital')
    wrapper.unmount()
  })

  it('retains the outgoing static picture and brightness until black, then reveals the decoded scene poster', async () => {
    const wrapper = mountWallpaper()
    await wrapper.setProps({ wallpaperId: 'pack:neon-city' })
    expect(phase(wrapper)).toBe('leaving')
    expect(surface(wrapper).attributes('data-wallpaper')).toBe('classic')
    expect(surface(wrapper).classes()).not.toContain('desktop-wallpaper-surface--scene')
    await black(wrapper)
    expect(phase(wrapper)).toBe('waiting')
    expect(surface(wrapper).attributes('data-wallpaper')).toBe('pack:neon-city')
    await wrapper.get('img').trigger('load')
    expect(phase(wrapper)).toBe('entering')
    await black(wrapper)
    expect(phase(wrapper)).toBe('idle')
    wrapper.unmount()
  })

  it('pauses the departing scene and starts the static entrance only after departure', async () => {
    const wrapper = mountWallpaper('pack:neon-city')
    await wrapper.setProps({ wallpaperId: 'classic' })
    expect(wrapper.get('.test-scene').attributes('data-covered')).toBe('true')
    expect(wrapper.get('img').attributes('src')).toContain('/neon-city/poster')
    await black(wrapper)
    expect(wrapper.find('.test-scene').exists()).toBe(false)
    expect(phase(wrapper)).toBe('waiting')
    await wrapper.get('img').trigger('load')
    expect(phase(wrapper)).toBe('entering')
    wrapper.unmount()
  })

  it('mounts only the latest target through scene → still → scene and multiple static selections', async () => {
    const wrapper = mountWallpaper('pack:neon-city')
    await wrapper.setProps({ wallpaperId: 'classic' })
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.setProps({ wallpaperId: 'orbit' })
    await wrapper.setProps({ wallpaperId: 'pack:sea-and-sky' })
    expect(surface(wrapper).attributes('data-wallpaper')).toBe('pack:neon-city')
    await vi.advanceTimersByTimeAsync(400)
    expect(surface(wrapper).attributes('data-wallpaper')).toBe('pack:sea-and-sky')
    expect(wrapper.get('img').attributes('src')).toContain('/sea-and-sky/poster')
    expect(surface(wrapper).attributes('style')).not.toContain('old-orbital')
    wrapper.unmount()
  })

  it('lets ready reveal a scene while its poster is stalled, and bounds a missing poster wait', async () => {
    const wrapper = mountWallpaper()
    await wrapper.setProps({ wallpaperId: 'pack:neon-city' }); await black(wrapper)
    wrapper.findComponent(Scene).vm.$emit('cameras', ['city'])
    await nextTick()
    expect(phase(wrapper)).toBe('entering')
    expect(wrapper.emitted('cameras')).toEqual([[['city']]])
    await wrapper.setProps({ wallpaperId: 'pack:sea-and-sky' }); await black(wrapper)
    await vi.advanceTimersByTimeAsync(2000)
    expect(phase(wrapper)).toBe('entering')
    wrapper.unmount()
  })

  it('ignores outgoing scene callbacks and a superseded image decode', async () => {
    const wrapper = mountWallpaper('pack:neon-city')
    const oldScene = wrapper.findComponent(Scene)
    await wrapper.setProps({ wallpaperId: 'classic' })
    oldScene.vm.$emit('failed'); oldScene.vm.$emit('cameras', ['old'])
    expect(wrapper.emitted('failed')).toBeUndefined()
    expect(wrapper.emitted('cameras')).toBeUndefined()
    await black(wrapper)
    const staleImage = wrapper.get('img')
    await wrapper.setProps({ wallpaperId: 'pack:sea-and-sky' })
    await staleImage.trigger('load')
    expect(phase(wrapper)).toBe('waiting')
    expect(wrapper.get('img').attributes('src')).toContain('/sea-and-sky/poster')
    wrapper.unmount()
  })

  it('keeps pure static changes immediate and cancels an in-flight handoff for reduced motion', async () => {
    const wrapper = mountWallpaper()
    await wrapper.setProps({ wallpaperId: 'orbit' })
    expect(phase(wrapper)).toBe('idle')
    await wrapper.setProps({ wallpaperId: 'pack:neon-city' })
    expect(phase(wrapper)).toBe('leaving')
    useSceneMotionPreference().systemReducedMotion.value = true
    await nextTick()
    expect(phase(wrapper)).toBe('idle')
    expect(surface(wrapper).attributes('data-wallpaper')).toBe('pack:neon-city')
    expect(surface(wrapper).classes()).not.toContain('desktop-wallpaper-surface--scene')
    await vi.advanceTimersByTimeAsync(3000)
    expect(phase(wrapper)).toBe('idle')
    wrapper.unmount()
  })
})

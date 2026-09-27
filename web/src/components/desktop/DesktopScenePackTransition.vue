<script setup lang="ts">
import { nextTick } from 'vue'
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'

const { systemReducedMotion } = useSceneMotionPreference()

function leave(element: Element, done: () => void): void {
  if (systemReducedMotion.value || !element.matches('.desktop-scene-pack')) {
    // Finish after Vue has installed its out-in placeholder, without an animation delay.
    void nextTick(done)
    return
  }
  // Listen to the veil itself: Vue only detects animations on the root element.
  const finish = (event?: Event): void => {
    if (event && (event.target !== element || (event as AnimationEvent).animationName !== 'desktop-scene-pack-depart')) return
    window.clearTimeout(fallback)
    element.removeEventListener('animationend', finish)
    done()
  }
  // Still complete if CSS is unavailable or the animation is interrupted.
  const fallback = window.setTimeout(finish, 700)
  element.addEventListener('animationend', finish)
}
</script>

<template>
  <Transition name="desktop-scene-pack-switch" mode="out-in" :css="!systemReducedMotion" @leave="leave">
    <!-- A real empty child keeps rapid scene → still → scene changes in the same exit. -->
    <slot><span hidden aria-hidden="true" /></slot>
  </Transition>
</template>

<script setup lang="ts">
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'

const { systemReducedMotion } = useSceneMotionPreference()
</script>

<template>
  <!-- The black veil is a pseudo-element, so Vue needs its leave duration explicitly. -->
  <Transition name="desktop-scene-pack-switch" mode="out-in" :duration="{ enter: 0, leave: systemReducedMotion ? 0 : 600 }">
    <slot />
  </Transition>
</template>

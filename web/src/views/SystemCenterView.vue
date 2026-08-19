<script setup lang="ts">
import { usePhraseCatalog } from '@/i18n/phrase'
import OverviewView from './OverviewView.vue'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/SystemCenterView/en-US').then((module) => module.default)
  : import('@/i18n/pages/SystemCenterView/zh-TW').then((module) => module.default))
</script>

<template>
  <OverviewView system-center-only />
</template>

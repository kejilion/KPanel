<script setup lang="ts">
import { computed } from 'vue'
import almaSvg from 'simple-icons/icons/almalinux.svg?raw'
import alpineSvg from 'simple-icons/icons/alpinelinux.svg?raw'
import archSvg from 'simple-icons/icons/archlinux.svg?raw'
import centosSvg from 'simple-icons/icons/centos.svg?raw'
import debianSvg from 'simple-icons/icons/debian.svg?raw'
import fedoraSvg from 'simple-icons/icons/fedora.svg?raw'
import linuxSvg from 'simple-icons/icons/linux.svg?raw'
import manjaroSvg from 'simple-icons/icons/manjaro.svg?raw'
import opensuseSvg from 'simple-icons/icons/opensuse.svg?raw'
import redhatSvg from 'simple-icons/icons/redhat.svg?raw'
import rockySvg from 'simple-icons/icons/rockylinux.svg?raw'
import suseSvg from 'simple-icons/icons/suse.svg?raw'
import ubuntuSvg from 'simple-icons/icons/ubuntu.svg?raw'
import oracleIcon from '@/assets/os/oracle.png'

const props = defineProps<{
  distro: string
  label: string
  showTooltip?: boolean
}>()

interface OperatingSystemMark {
  svg?: string
  image?: string
  accent: string
  foreground?: string
}

function cleanBrandSvg(svg: string): string {
  return svg
    .replace(/\srole="img"/, '')
    .replace(/\sxmlns="[^"]+"/, '')
    .replace(/<title>.*?<\/title>/, '')
}

const linuxMark: OperatingSystemMark = {
  svg: cleanBrandSvg(linuxSvg),
  accent: 'FCC624',
  foreground: '#141816',
}

const icons: Record<string, OperatingSystemMark> = {
  windows: { svg: '<svg viewBox="0 0 24 24"><path d="M2 2h9v9H2zM13 2h9v9h-9zM2 13h9v9H2zM13 13h9v9h-9z"/></svg>', accent: '0078D4' },
  ubuntu: { svg: cleanBrandSvg(ubuntuSvg), accent: 'E95420' },
  debian: { svg: cleanBrandSvg(debianSvg), accent: 'A81D33' },
  centos: { svg: cleanBrandSvg(centosSvg), accent: '262577' },
  rocky: { svg: cleanBrandSvg(rockySvg), accent: '10B981' },
  alma: { svg: cleanBrandSvg(almaSvg), accent: '000000' },
  fedora: { svg: cleanBrandSvg(fedoraSvg), accent: '51A2DA' },
  alpine: { svg: cleanBrandSvg(alpineSvg), accent: '0D597F' },
  rhel: { svg: cleanBrandSvg(redhatSvg), accent: 'EE0000' },
  manjaro: { svg: cleanBrandSvg(manjaroSvg), accent: '35BFA4' },
  arch: { svg: cleanBrandSvg(archSvg), accent: '1793D1' },
  opensuse: { svg: cleanBrandSvg(opensuseSvg), accent: '73BA25' },
  suse: { svg: cleanBrandSvg(suseSvg), accent: '0C322C' },
  oracle: { image: oracleIcon, accent: 'C74634' },
  linux: linuxMark,
}

const mark = computed<OperatingSystemMark>(() =>
  Object.prototype.hasOwnProperty.call(icons, props.distro) ? icons[props.distro]! : linuxMark,
)
const style = computed(() => ({
  '--os-accent': `#${mark.value.accent}`,
  '--os-foreground': mark.value.foreground || '#ffffff',
}))
</script>

<template>
  <span
    class="os-identity__mark"
    :title="props.showTooltip === false ? undefined : props.label"
    :style="style"
    aria-hidden="true"
  >
    <img v-if="mark.image" :src="mark.image" alt="" aria-hidden="true" />
    <span v-else class="os-identity__svg" v-html="mark.svg || linuxSvg" />
  </span>
</template>

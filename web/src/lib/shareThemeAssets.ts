// Visual assets handed to protocol-2 share themes. Loaded lazily, and only for such themes,
// so the public page never pays for them otherwise. Everything here comes from bundled,
// reviewed packages: a theme receives bare SVG path data (to build its own <svg> and colour
// freely) and flag images as data: URLs (the theme CSP only allows its own files and data:).
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
import { regionCenters } from '@/components/cluster/globeData'
import type { PublicClusterShareSnapshot } from '@/types/api'
import { detectOperatingSystemIdentity } from './operatingSystem'

export type RegionCenters = Readonly<Record<string, readonly [number, number]>>
export interface ShareThemeSystemMark { path: string; image: string; accent: string }
export interface ShareThemeAssets { centers: RegionCenters; flags: Readonly<Record<string, string>>; systems: Readonly<Record<string, ShareThemeSystemMark>> }

// Same brand colours as OperatingSystemIcon, so themes and the default page agree.
const marks: Record<string, [svg: string, accent: string]> = {
  ubuntu: [ubuntuSvg, '#E95420'], debian: [debianSvg, '#A81D33'], centos: [centosSvg, '#262577'], rocky: [rockySvg, '#10B981'],
  alma: [almaSvg, '#000000'], fedora: [fedoraSvg, '#51A2DA'], alpine: [alpineSvg, '#0D597F'], rhel: [redhatSvg, '#EE0000'],
  manjaro: [manjaroSvg, '#35BFA4'], arch: [archSvg, '#1793D1'], opensuse: [opensuseSvg, '#73BA25'], suse: [suseSvg, '#0C322C'],
  linux: [linuxSvg, '#FCC624'],
}
const pathOf = (svg: string) => /\sd="([^"]+)"/.exec(svg)?.[1] || ''
const svgURL = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`

const flagSources = import.meta.glob('../../node_modules/circle-flags/flags/*.svg', { query: '?raw', import: 'default' }) as Record<string, () => Promise<string>>
const flagLoaders = new Map(Object.entries(flagSources).map(([path, load]) => [path.match(/\/([a-z0-9-]+)\.svg$/)?.[1]?.toUpperCase() || '', load]))

async function imageURL(url: string): Promise<string> {
  const blob = await (await fetch(url)).blob()
  return await new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = reject; reader.readAsDataURL(blob) })
}

export function systemKey(os?: string) { return detectOperatingSystemIdentity({ os }).key }

/** Resolves only the flags and marks this snapshot needs; a missing asset is simply left out. */
export async function loadShareThemeAssets(snapshot: PublicClusterShareSnapshot): Promise<ShareThemeAssets> {
  const codes = [...new Set(snapshot.items.map(host => (host.location.countryCode || '').trim().toUpperCase()).filter(code => /^[A-Z]{2}$/.test(code)))]
  const flags: Record<string, string> = {}
  await Promise.all(codes.map(async code => { const load = flagLoaders.get(code); if (load) try { flags[code] = svgURL(await load()) } catch { /* no flag */ } }))
  const systems: Record<string, ShareThemeSystemMark> = {}
  for (const key of new Set(snapshot.items.map(host => systemKey(host.os)))) {
    if (key === 'oracle') { try { systems.oracle = { path: '', image: await imageURL(oracleIcon), accent: '#C74634' } } catch { /* no mark */ } continue }
    const [svg, accent] = marks[key] || marks.linux!
    systems[key] = { path: pathOf(svg), image: '', accent }
  }
  return { centers: regionCenters, flags, systems }
}

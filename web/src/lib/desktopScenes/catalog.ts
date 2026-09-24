import type { MessageKey } from '@/i18n/messages/zh-CN'
import type { ThemeColorIntent } from '@/theme/colors'
import { dominantChronoPhase, type ChronoPhase } from './chrono'
import type { ScenePainterFactory } from './painterKit'

/**
 * Live desktop scenes. Every scene is a painted 16:9 poster (generated with
 * gpt-image and shipped as same-origin WebP), a few CSS light layers anchored
 * to poster coordinates, and an optional frame-capped particle painter.
 *
 * Scene ids share `kpanel:desktop-wallpaper:v1` with the static wallpapers, so
 * public/appearance-init.js keeps its own allow-list (checked by tests).
 */
export const DESKTOP_SCENE_IDS = ['chrono', 'tide', 'sakura', 'neon', 'aurora'] as const
export type DesktopSceneID = typeof DESKTOP_SCENE_IDS[number]

export const DESKTOP_SCENE_ASSET_ROOT = '/wallpapers/scenes'

/**
 * A light layer centered at (`x`, `y`) with size (`w`, `h`), all as fractions of
 * the poster; the layer tracks the poster through `background-size: cover`.
 */
export interface DesktopSceneLayer {
  id: string
  x: number
  y: number
  w: number
  h: number
  image?: string
  /** Seconds added to this layer's entrance and loop, for staggered lights. */
  delay?: number
}

export interface DesktopSceneDefinition {
  id: DesktopSceneID
  nameKey: MessageKey
  descriptionKey: MessageKey
  thumb: string
  colors: Readonly<ThemeColorIntent>
  /** Where the entrance push-in and the breathing zoom are centered. */
  focus: { x: number, y: number }
  entranceSeconds: number
  layers: readonly DesktopSceneLayer[]
  loadPainter: () => Promise<ScenePainterFactory>
}

const asset = (name: string) => `${DESKTOP_SCENE_ASSET_ROOT}/${name}.webp`

export function chronoPoster(phase: ChronoPhase): string {
  return asset(`chrono-${phase}`)
}

const CHRONO_LANTERNS: readonly (readonly [number, number])[] = [
  [0.105, 0.395], [0.133, 0.513], [0.889, 0.295], [0.816, 0.354], [0.91, 0.487], [0.979, 0.471], [0.805, 0.518],
]

export const DESKTOP_SCENES: readonly DesktopSceneDefinition[] = Object.freeze([
  {
    id: 'chrono',
    nameKey: 'desktop.sceneChrono',
    descriptionKey: 'desktop.sceneChronoDescription',
    thumb: asset('chrono-thumb'),
    colors: { brand: '#b0563f', neutral: '#4d5a5f', signatureLinked: false, signature: '#d49a48' },
    focus: { x: 0.52, y: 0.62 },
    entranceSeconds: 4.2,
    layers: [
      { id: 'sun', x: 0.9, y: 0.2, w: 0.55, h: 0.8 },
      { id: 'moon', x: 0.757, y: 0.101, w: 0.16, h: 0.28 },
      ...CHRONO_LANTERNS.map(([x, y], index) => ({ id: 'lantern', x, y, w: 0.055, h: 0.1, delay: index * 0.37 })),
    ],
    loadPainter: () => import('./scenes/chrono').then((module) => module.default),
  },
  {
    id: 'tide',
    nameKey: 'desktop.sceneTide',
    descriptionKey: 'desktop.sceneTideDescription',
    thumb: asset('tide-thumb'),
    colors: { brand: '#1b8499', neutral: '#57606d', signatureLinked: false, signature: '#df9a4c' },
    focus: { x: 0.68, y: 0.42 },
    entranceSeconds: 2.8,
    layers: [
      { id: 'sun', x: 0.68, y: 0.387, w: 0.36, h: 0.64 },
      { id: 'path', x: 0.678, y: 0.66, w: 0.13, h: 0.52 },
    ],
    loadPainter: () => import('./scenes/tide').then((module) => module.default),
  },
  {
    id: 'sakura',
    nameKey: 'desktop.sceneSakura',
    descriptionKey: 'desktop.sceneSakuraDescription',
    thumb: asset('sakura-thumb'),
    colors: { brand: '#c95f86', neutral: '#5b6275', signatureLinked: false, signature: '#3f86cf' },
    focus: { x: 0.6, y: 0.45 },
    entranceSeconds: 2.8,
    layers: [
      { id: 'sun', x: 0.97, y: 0.02, w: 0.5, h: 0.85 },
      { id: 'rays', x: 0.97, y: 0.02, w: 1.1, h: 1.7 },
      { id: 'branch', x: 0.8, y: 0.19, w: 0.44, h: 0.45, image: asset('sakura-branch') },
    ],
    loadPainter: () => import('./scenes/sakura').then((module) => module.default),
  },
  {
    id: 'neon',
    nameKey: 'desktop.sceneNeon',
    descriptionKey: 'desktop.sceneNeonDescription',
    thumb: asset('neon-thumb'),
    colors: { brand: '#8a57d8', neutral: '#3c3752', signatureLinked: false, signature: '#1fb3cc' },
    focus: { x: 0.66, y: 0.55 },
    entranceSeconds: 2.8,
    layers: [
      { id: 'tower', x: 0.667, y: 0.24, w: 0.13, h: 0.5 },
      { id: 'magenta', x: 0.472, y: 0.31, w: 0.075, h: 0.24, delay: 0.25 },
      { id: 'cyan', x: 0.305, y: 0.294, w: 0.065, h: 0.13, delay: 0.55 },
      { id: 'amber', x: 0.903, y: 0.223, w: 0.065, h: 0.25, delay: 0.85 },
      { id: 'azure', x: 0.834, y: 0.345, w: 0.055, h: 0.21, delay: 1.1 },
      { id: 'street', x: 0.5, y: 0.9, w: 1.02, h: 0.26 },
    ],
    loadPainter: () => import('./scenes/neon').then((module) => module.default),
  },
  {
    id: 'aurora',
    nameKey: 'desktop.sceneAurora',
    descriptionKey: 'desktop.sceneAuroraDescription',
    thumb: asset('aurora-thumb'),
    colors: { brand: '#178f7d', neutral: '#35495b', signatureLinked: false, signature: '#7768cf' },
    focus: { x: 0.5, y: 0.5 },
    entranceSeconds: 3.4,
    layers: [
      { id: 'horizon', x: 0.5, y: 0.55, w: 1.04, h: 0.16 },
      { id: 'veil', x: 0.5, y: 0.3, w: 1.16, h: 0.7, image: asset('aurora-veil') },
      { id: 'veil-echo', x: 0.54, y: 0.26, w: 1.3, h: 0.6, image: asset('aurora-veil'), delay: 0.6 },
      { id: 'cabin', x: 0.885, y: 0.68, w: 0.1, h: 0.1 },
    ],
    loadPainter: () => import('./scenes/aurora').then((module) => module.default),
  },
] satisfies DesktopSceneDefinition[])

export function isDesktopSceneID(value: unknown): value is DesktopSceneID {
  return typeof value === 'string' && (DESKTOP_SCENE_IDS as readonly string[]).includes(value)
}

export function findDesktopScene(id: DesktopSceneID): DesktopSceneDefinition {
  return DESKTOP_SCENES.find((scene) => scene.id === id)!
}

/** Poster shown under the live stage and before modules load. */
export function desktopScenePoster(id: DesktopSceneID, date = new Date()): string {
  return id === 'chrono' ? chronoPoster(dominantChronoPhase(date)) : asset(id)
}

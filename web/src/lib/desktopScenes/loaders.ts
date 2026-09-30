import { createLazyModuleLoader } from '@/lib/lazyModuleLoader'
import type { SceneModule } from './types'

// Each scene is its own chunk, fetched from the panel origin the first time it is shown.
export const DESKTOP_SCENE_IDS = ['daylight', 'seaside', 'aurora', 'rain', 'fireflies', 'nebula'] as const
export type DesktopSceneID = typeof DESKTOP_SCENE_IDS[number]

export function isDesktopSceneID(value: unknown): value is DesktopSceneID {
  return typeof value === 'string' && (DESKTOP_SCENE_IDS as readonly string[]).includes(value)
}

export const sceneModules = createLazyModuleLoader<DesktopSceneID, SceneModule>({
  daylight: () => import('./scenes/daylight'),
  seaside: () => import('./scenes/seaside'),
  aurora: () => import('./scenes/aurora'),
  rain: () => import('./scenes/rain'),
  fireflies: () => import('./scenes/fireflies'),
  nebula: () => import('./scenes/nebula'),
})

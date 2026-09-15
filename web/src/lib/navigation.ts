import type { Component } from 'vue'
import { reactive } from 'vue'
import { createLazyModuleLoader } from './lazyModuleLoader'

interface RouteViewModule {
  default: Component
}

const routeViewLoaders = {
  '/setup': () => import('@/views/SetupView.vue'),
  '/login': () => import('@/views/LoginView.vue'),
  '/overview': () => import('@/views/OverviewView.vue'),
  '/system': () => import('@/views/SystemCenterView.vue'),
  '/monitoring': () => import('@/views/MonitoringView.vue'),
  '/processes': () => import('@/views/ProcessManagerView.vue'),
  '/cluster': () => import('@/views/ClusterView.vue'),
  '/sites': () => import('@/views/SitesView.vue'),
  '/sites/environment': () => import('@/views/EnvironmentView.vue'),
  '/apps': () => import('@/views/AppsView.vue'),
  '/files': () => import('@/views/FilesView.vue'),
  '/terminal': () => import('@/views/TerminalView.vue'),
  '/diagnostics': () => import('@/views/DiagnosticsView.vue'),
  '/docker': () => import('@/views/DockerView.vue'),
  '/activity': () => import('@/views/ActivityView.vue'),
  '/settings': () => import('@/views/SettingsView.vue'),
} satisfies Record<string, () => Promise<RouteViewModule>>

export type NavigationRoutePath = keyof typeof routeViewLoaders

const routeViews = createLazyModuleLoader<NavigationRoutePath, RouteViewModule>(routeViewLoaders)

export const routeNavigationState = reactive({
  pending: false,
  targetPath: '',
  failureSequence: 0,
})

export function loadNavigationRoute(path: NavigationRoutePath): Promise<RouteViewModule> {
  return routeViews.load(path)
}

export function prefetchNavigationRoute(path: string): Promise<boolean> {
  if (!(path in routeViewLoaders)) return Promise.resolve(false)
  return routeViews.prefetch(path as NavigationRoutePath)
}

export function beginRouteNavigation(path: string): void {
  routeNavigationState.pending = true
  routeNavigationState.targetPath = path
}

export function finishRouteNavigation(path: string): void {
  if (routeNavigationState.targetPath !== path) return
  routeNavigationState.pending = false
  routeNavigationState.targetPath = ''
}

export function failRouteNavigation(path: string): void {
  if (routeNavigationState.targetPath !== path) return
  routeNavigationState.pending = false
  routeNavigationState.targetPath = ''
  routeNavigationState.failureSequence += 1
}

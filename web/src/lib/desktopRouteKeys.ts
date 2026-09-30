import type { InjectionKey, Ref } from 'vue'
import type { Router, RouteLocationNormalizedLoaded } from 'vue-router'
import type { DesktopBrowserHistory } from '@/lib/desktopBrowserHistory'
import { routeLocationKey, routerKey, routerViewLocationKey } from 'vue-router'

/**
 * Typed handles to vue-router's internal injection keys.
 *
 * These keys are imported by name from the same module specifier the rest of
 * the app uses, so they are the same module instance in dev and prod. Desktop
 * windows provide overrides under these keys so a window's page resolves
 * `useRoute()` / `useRouter()` / `RouterView` against the window's own
 * in-memory router instead of the app router.
 */

export const windowRouterKey = routerKey as InjectionKey<Router>
export const windowRouteKey = routeLocationKey as InjectionKey<RouteLocationNormalizedLoaded>
export const windowRouterViewKey = routerViewLocationKey as InjectionKey<unknown>

/** Whether a desktop window is both focused and visible. */
export const desktopWindowActiveKey = Symbol('desktop-window-active') as InjectionKey<Readonly<Ref<boolean>>>

/**
 * Whether a desktop window is on screen (not minimized or closing), focused or
 * not. Live views such as terminals keep streaming in unfocused windows that
 * the user can still see, and pause only when this turns false.
 */
export const desktopWindowVisibleKey = Symbol('desktop-window-visible') as InjectionKey<Readonly<Ref<boolean>>>

/** Native document-history bridge shared by all independent desktop windows. */
export const desktopBrowserHistoryKey = Symbol(
  'desktop-browser-history',
) as InjectionKey<DesktopBrowserHistory>

export interface DesktopWindowCloseGuardRegistry {
  register: (guard: () => boolean | Promise<boolean>) => () => void
}

/** Register page-specific checks that must pass before a desktop window closes. */
export const desktopWindowCloseGuardKey = Symbol(
  'desktop-window-close-guard',
) as InjectionKey<DesktopWindowCloseGuardRegistry>

export interface DesktopCloseGuardCoordinator {
  register: (scope: number | string, guard: () => boolean | Promise<boolean>) => () => void
  checkAll: () => Promise<boolean>
}

const applicationCloseGuards = new Map<number | string, Set<() => boolean | Promise<boolean>>>()

/**
 * Shared close coordinator for both the classic RouterView and desktop
 * windows. Keeping it outside DesktopView lets AppShell protect unsaved work
 * before it unmounts the classic view while entering desktop mode.
 */
export const desktopCloseGuardCoordinator: DesktopCloseGuardCoordinator = {
  register(scope, guard) {
    const guards = applicationCloseGuards.get(scope) ?? new Set()
    guards.add(guard)
    applicationCloseGuards.set(scope, guards)
    return () => {
      guards.delete(guard)
      if (!guards.size) applicationCloseGuards.delete(scope)
    }
  },
  async checkAll() {
    for (const guards of applicationCloseGuards.values()) {
      for (const guard of [...guards]) {
        if (!(await guard())) return false
      }
    }
    return true
  },
}

export const desktopCloseGuardCoordinatorKey = Symbol(
  'desktop-close-guard-coordinator',
) as InjectionKey<DesktopCloseGuardCoordinator>

import { createRouter, createWebHistory } from 'vue-router'
import AppShell from '@/components/layout/AppShell.vue'
import {
  beginRouteNavigation,
  failRouteNavigation,
  finishRouteNavigation,
  loadNavigationRoute,
} from '@/lib/navigation'
import { resolveNavigationScroll } from '@/lib/navigationScroll'
import { useSession } from '@/stores/session'
import type { MessageKey } from '@/i18n/messages/zh-CN'

declare module 'vue-router' {
  interface RouteMeta {
    titleKey?: MessageKey
    public?: boolean
    guestOnly?: boolean
    skipSessionCheck?: boolean
  }
}

export const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: (to, from, savedPosition) => resolveNavigationScroll(to.path, from.path, savedPosition),
  routes: [
    {
      path: '/setup',
      name: 'setup',
      component: () => loadNavigationRoute('/setup'),
      meta: { titleKey: 'route.setup', public: true, guestOnly: true },
    },
    {
      path: '/login',
      name: 'login',
      component: () => loadNavigationRoute('/login'),
      meta: { titleKey: 'route.login', public: true, guestOnly: true },
    },
    {
      path: '/share/file/:token',
      name: 'file-share',
      component: () => import('@/views/FileShareView.vue'),
      meta: { titleKey: 'route.fileShare', public: true, skipSessionCheck: true },
    },
    {
      path: '/share/:token',
      name: 'cluster-share',
      component: () => import('@/views/ClusterShareView.vue'),
      meta: { titleKey: 'route.clusterShare', public: true, skipSessionCheck: true },
    },
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', redirect: '/overview' },
        {
          path: 'overview',
          name: 'overview',
          component: () => loadNavigationRoute('/overview'),
          meta: { titleKey: 'route.overview' },
        },
        {
          path: 'system',
          name: 'system',
          component: () => loadNavigationRoute('/system'),
          meta: { titleKey: 'route.systemCenter' },
        },
        {
          path: 'monitoring',
          name: 'monitoring',
          component: () => loadNavigationRoute('/monitoring'),
          meta: { titleKey: 'route.monitoring' },
        },
        {
          path: 'processes',
          name: 'processes',
          component: () => loadNavigationRoute('/processes'),
          meta: { titleKey: 'route.processes' },
        },
        {
          path: 'cluster',
          name: 'cluster',
          component: () => loadNavigationRoute('/cluster'),
          meta: { titleKey: 'route.cluster' },
        },
        {
          path: 'ai',
          name: 'ai',
          component: () => import('@/views/AiView.vue'),
          meta: { titleKey: 'route.ai' },
        },
        {
          path: 'ai/s/:sessionId',
          name: 'ai-session',
          component: () => import('@/views/AiView.vue'),
          meta: { titleKey: 'route.ai' },
        },
        {
          path: 'sites',
          name: 'sites',
          component: () => loadNavigationRoute('/sites'),
          meta: { titleKey: 'route.sites' },
        },
        {
          path: 'sites/environment',
          name: 'sites-environment',
          component: () => loadNavigationRoute('/sites/environment'),
          meta: { titleKey: 'route.environment' },
        },
        {
          path: 'apps',
          name: 'apps',
          component: () => loadNavigationRoute('/apps'),
          meta: { titleKey: 'route.apps' },
        },
        {
          path: 'files',
          name: 'files',
          component: () => loadNavigationRoute('/files'),
          meta: { titleKey: 'route.files' },
        },
        {
          path: 'terminal',
          name: 'terminal',
          component: () => loadNavigationRoute('/terminal'),
          meta: { titleKey: 'route.terminal' },
        },
        {
          path: 'diagnostics',
          name: 'diagnostics',
          component: () => loadNavigationRoute('/diagnostics'),
          meta: { titleKey: 'route.diagnostics' },
        },
        {
          path: 'docker',
          name: 'docker',
          component: () => loadNavigationRoute('/docker'),
          meta: { titleKey: 'route.docker' },
        },
        {
          path: 'activity',
          name: 'activity',
          component: () => loadNavigationRoute('/activity'),
          meta: { titleKey: 'route.activity' },
        },
        {
          path: 'jobs',
          redirect: { path: '/activity', query: { tab: 'jobs' } },
        },
        {
          path: 'audit',
          redirect: { path: '/activity', query: { tab: 'audit' } },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => loadNavigationRoute('/settings'),
          meta: { titleKey: 'route.settings' },
        },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/overview' },
  ],
})

router.beforeEach(async (to) => {
  beginRouteNavigation(to.path)
  if (to.meta.skipSessionCheck) return true
  const session = useSession()
  if (!session.state.checked) await session.refresh()

  if (session.state.setupRequired) {
    return to.name === 'setup' ? true : { name: 'setup' }
  }

  if (to.name === 'setup') {
    return session.state.authenticated ? { name: 'overview' } : { name: 'login' }
  }

  if (!to.meta.public && !session.state.authenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if (to.meta.guestOnly && session.state.authenticated) {
    return { name: 'overview' }
  }

  return true
})

router.afterEach((to) => {
  finishRouteNavigation(to.path)
})

router.onError((_error, to) => {
  failRouteNavigation(to.path)
})

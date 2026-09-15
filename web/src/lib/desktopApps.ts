import type { Component } from 'vue'
import {
  Boxes,
  Bot,
  ClipboardList,
  Container,
  Folder,
  HeartPulse,
  LayoutDashboard,
  Network,
  Settings,
  SquareTerminal,
  Store,
} from '@lucide/vue'
import type { MessageKey } from '@/i18n/messages/zh-CN'
import SystemCenterIcon from '@/components/icons/SystemCenterIcon.vue'

/**
 * Desktop application catalogue. Mirrors the left-navigation items in
 * AppShell.vue, one icon per route page. The terminal is a single-instance app
 * (opening it twice focuses the existing window).
 */

export interface DesktopApp {
  path: string
  labelKey: MessageKey
  icon: Component
  /** Native artwork used by desktop tiles, windows, and the taskbar. */
  desktopIconURL?: string
  allowMultiple: boolean
  /** App-tile gradient so each icon reads like a distinct application. */
  gradient: [string, string]
}

export const desktopApps: DesktopApp[] = [
  {
    path: '/overview',
    labelKey: 'route.overview',
    icon: LayoutDashboard,
    desktopIconURL: '/desktop-icons/overview-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#2dd4bf', '#0f766e'],
  },
  {
    path: '/ai',
    labelKey: 'route.ai',
    icon: Bot,
    desktopIconURL: '/desktop-icons/ai-kpanel-flat-v1.webp',
    allowMultiple: true,
    gradient: ['#a78bfa', '#6d28d9'],
  },
  {
    path: '/sites',
    labelKey: 'route.sites',
    icon: Boxes,
    desktopIconURL: '/desktop-icons/sites-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#60a5fa', '#1d4ed8'],
  },
  {
    path: '/apps',
    labelKey: 'route.apps',
    icon: Store,
    desktopIconURL: '/desktop-icons/apps-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#fbbf24', '#d97706'],
  },
  {
    path: '/docker',
    labelKey: 'route.docker',
    icon: Container,
    desktopIconURL: '/desktop-icons/docker-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#22d3ee', '#0369a1'],
  },
  {
    path: '/files',
    labelKey: 'route.files',
    icon: Folder,
    desktopIconURL: '/desktop-icons/files-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#facc15', '#ca8a04'],
  },
  {
    path: '/terminal',
    labelKey: 'route.terminal',
    icon: SquareTerminal,
    desktopIconURL: '/desktop-icons/terminal-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#94a3b8', '#1e293b'],
  },
  {
    path: '/diagnostics',
    labelKey: 'route.diagnostics',
    icon: HeartPulse,
    desktopIconURL: '/desktop-icons/diagnostics-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#fb7185', '#be123c'],
  },
  {
    path: '/cluster',
    labelKey: 'route.cluster',
    icon: Network,
    desktopIconURL: '/desktop-icons/cluster-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#818cf8', '#4338ca'],
  },
  {
    path: '/system',
    labelKey: 'route.systemCenter',
    icon: SystemCenterIcon,
    desktopIconURL: '/desktop-icons/system-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#8b5cf6', '#4338ca'],
  },
  {
    path: '/activity',
    labelKey: 'route.activity',
    icon: ClipboardList,
    desktopIconURL: '/desktop-icons/activity-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#34d399', '#047857'],
  },
  {
    path: '/settings',
    labelKey: 'route.settings',
    icon: Settings,
    desktopIconURL: '/desktop-icons/settings-kpanel-flat-v1.webp',
    allowMultiple: false,
    gradient: ['#cbd5e1', '#64748b'],
  },
]

/** Taskbar metadata for dynamic per-application script terminal windows. */
export const desktopScriptApp: DesktopApp = {
  path: '/app-script',
  labelKey: 'desktop.scriptWindowTitle',
  icon: SquareTerminal,
  allowMultiple: false,
  gradient: ['#64748b', '#0f172a'],
}

/** Neutral gradient for windows whose app is not in the catalogue. */
export const DEFAULT_WINDOW_GRADIENT: [string, string] = ['#a78bfa', '#6d28d9']

export function desktopRoutePath(fullPath: string): string {
  const queryIndex = fullPath.indexOf('?')
  const hashIndex = fullPath.indexOf('#')
  const end = [queryIndex, hashIndex]
    .filter((index) => index >= 0)
    .reduce((minimum, index) => Math.min(minimum, index), fullPath.length)
  return fullPath.slice(0, end)
}

export function canonicalDesktopAppPath(path: string): string {
  path = desktopRoutePath(path)
  if (path === '/monitoring') return '/overview'
  if (path === '/sites/environment') return '/sites'
  if (path.startsWith('/ai/s/')) return '/ai'
  if (path === '/jobs' || path === '/audit') return '/activity'
  return path
}

export function findDesktopApp(path: string): DesktopApp | undefined {
  const appPath = canonicalDesktopAppPath(path)
  if (/^\/app-script\/[A-Za-z0-9_-]{1,128}$/.test(appPath)) return desktopScriptApp
  return desktopApps.find((app) => app.path === appPath)
}

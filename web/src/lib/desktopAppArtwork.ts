import type { DesktopApp } from './desktopApps'
import type { DesktopShellStyle } from '@/stores/desktopShellStyle'

/** Original, local artwork for the fixed system launchers only. */
const windowsIconNames: Readonly<Record<string, string>> = {
  '/browser': 'browser',
  '/virtual-machines': 'virtualmachines',
  '/overview': 'overview',
  '/monitoring': 'monitoring',
  '/processes': 'processes',
  '/ai': 'ai',
  '/sites': 'sites',
  '/apps': 'apps',
  '/docker': 'docker',
  '/files': 'files',
  '/music': 'music',
  '/gallery': 'gallery',
  '/cinema': 'cinema',
  '/terminal': 'terminal',
  '/diagnostics': 'diagnostics',
  '/cluster': 'cluster',
  '/system': 'system',
  '/activity': 'activity',
  '/settings': 'settings',
  '/guides': 'guides',
  '/offers': 'offers',
}

export function desktopAppIconURL(app: DesktopApp | undefined, style: DesktopShellStyle): string | undefined {
  if (!app) return undefined
  const name = windowsIconNames[app.path]
  const version = app.path === '/browser' ? 'v2' : 'v1'
  return style !== 'modern' && name
    ? `/desktop-icons/${style}/${name}-${version}.svg`
    : app.desktopIconURL
}

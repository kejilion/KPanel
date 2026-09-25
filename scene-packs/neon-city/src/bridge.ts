/**
 * Messages between the KPanel desktop (parent) and a scene pack running in a
 * sandboxed, opaque-origin iframe. Commands only affect rendering, so the pack
 * accepts them from its parent window without needing the parent's origin.
 */
export type HostCommand =
  | { source: 'kpanel-desktop', type: 'pause' }
  | { source: 'kpanel-desktop', type: 'resume' }
  | { source: 'kpanel-desktop', type: 'camera', index?: number }

export type PackEvent =
  | { source: 'kpanel-scene-pack', type: 'progress', value: number }
  | { source: 'kpanel-scene-pack', type: 'ready', cameras: readonly string[] }
  | { source: 'kpanel-scene-pack', type: 'camera', index: number }
  | { source: 'kpanel-scene-pack', type: 'error', reason: string }

export function postToHost(event: PackEvent): void {
  if (window.parent !== window) window.parent.postMessage(event, '*')
}

export function onHostCommand(handler: (command: HostCommand) => void): void {
  window.addEventListener('message', (event) => {
    if (event.source !== window.parent) return
    const data = event.data as Partial<HostCommand> | null
    if (!data || data.source !== 'kpanel-desktop') return
    if (data.type === 'pause' || data.type === 'resume' || data.type === 'camera') handler(data as HostCommand)
  })
}

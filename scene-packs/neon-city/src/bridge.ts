/**
 * Messages between the KPanel desktop (parent) and a scene pack running in a
 * sandboxed, opaque-origin iframe. Commands only affect rendering, so the pack
 * accepts them from its parent window without needing the parent's origin.
 * When the scene runs in a worker (see runtime.ts), the page passes events up
 * and commands down; the scene uses the same two calls either way.
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

let send = (event: PackEvent): void => {
  if (window.parent !== window) window.parent.postMessage(event, '*')
}
const handlers: ((command: HostCommand) => void)[] = []

export function postToHost(event: PackEvent): void {
  send(event)
}

export function onHostCommand(handler: (command: HostCommand) => void): void {
  handlers.push(handler)
}

/** For the runtime: where events go instead (from a worker, to its page). */
export function sendEventsVia(sender: (event: PackEvent) => void): void {
  send = sender
}

/** For the runtime: hands a command from the desktop to the scene. */
export function deliverCommand(command: HostCommand): void {
  for (const handler of handlers) handler(command)
}

/** The command in a message to the page, if it is one from the desktop (the parent window). */
export function hostCommand(event: MessageEvent): HostCommand | undefined {
  if (event.source !== window.parent) return undefined
  const data = event.data as Partial<HostCommand> | null
  if (!data || data.source !== 'kpanel-desktop') return undefined
  return data.type === 'pause' || data.type === 'resume' || data.type === 'camera' ? data as HostCommand : undefined
}

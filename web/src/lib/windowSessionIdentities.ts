// Window snapshots contain only opaque identities, never terminal output or commands.
// sessionStorage keeps restored desktop windows separate within each browser tab.
export function windowIdentityKey(kind: string, owner: string, windowId: string | undefined, target = ''): string {
  if (!owner || !windowId) return ''
  return ['kpanel:window-identities', kind, owner, windowId, target].map(encodeURIComponent).join(':')
}

export function readWindowIdentities(key: string, pattern: RegExp, limit: number): string[] {
  if (!key) return []
  try {
    const value: unknown = JSON.parse(window.sessionStorage.getItem(key) || '[]')
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((id): id is string => typeof id === 'string' && pattern.test(id)))].slice(0, limit)
  } catch { return [] }
}

export function writeWindowIdentities(key: string, ids: string[]): void {
  if (!key) return
  try { window.sessionStorage.setItem(key, JSON.stringify(ids)) } catch { /* best effort */ }
}

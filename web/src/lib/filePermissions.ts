// File entries normally contain Go FileMode.String(), with numeric modes also
// accepted for nodes returning octal permissions.
export function permissionValue(mode: string): string | undefined {
  if (/^0?[0-7]{3}$/.test(mode)) return mode.slice(-3)
  const match = /([r-][w-][x-])([r-][w-][x-])([r-][w-][x-])$/.exec(mode)
  if (!match) return undefined
  return match.slice(1).map((group) => String(
    (group[0] === 'r' ? 4 : 0) + (group[1] === 'w' ? 2 : 0) + (group[2] === 'x' ? 1 : 0),
  )).join('')
}

export function commonPermissionValue(entries: readonly { mode: string }[]): string {
  const first = entries[0] && permissionValue(entries[0].mode)
  return first && entries.every((entry) => permissionValue(entry.mode) === first) ? first : ''
}

/** Accept only the canonical absolute paths used by file-manager routes. */
export function requestedFilePath(value: unknown): string | undefined {
  const candidate = Array.isArray(value) ? value[0] : value
  if (
    typeof candidate !== 'string'
    || !candidate.startsWith('/')
    || candidate.length > 4096
    || candidate.includes('\0')
    || candidate.includes('\\')
  ) return undefined
  if (candidate !== '/' && candidate.slice(1).split('/').some((part) => !part || part === '.' || part === '..')) {
    return undefined
  }
  return candidate
}

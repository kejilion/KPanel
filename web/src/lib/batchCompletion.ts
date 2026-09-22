// Batch execution completion marker. The user's command is wrapped in a shell
// group on the same input line as a printf of the group's exit status, so the
// shell itself — not whatever program reads stdin — emits the marker as soon
// as the command finishes. The nonce is random per host run, so neither the
// echoed input line (which contains the literal `%s`) nor command output can
// forge a completion.

export function createBatchMarker(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return `__KPANEL_DONE_${Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')}_`
}

/**
 * The newline before `}` terminates trailing comments and background `&`,
 * and heredocs inside the group keep working. POSIX sh supports the same form.
 */
export function wrapBatchCommand(command: string, marker: string): string {
  const body = command.replace(/[\r\n]+$/, '')
  return `{ ${body}\n}; printf '\\n${marker}%s__\\n' "$?"\r`
}

export function batchExitCode(output: string, marker: string): number | null {
  const match = new RegExp(`${marker}(\\d{1,3})__`).exec(output)
  if (!match) return null
  const code = Number(match[1])
  return Number.isInteger(code) && code >= 0 && code <= 255 ? code : null
}

/** Removes the wrapper's echo and marker lines from displayed output. */
export function stripBatchWrapper(output: string, command: string, marker: string): string {
  const firstLine = command.replace(/[\r\n]+$/, '').split(/\r?\n/, 1)[0] ?? ''
  const lines = output.split('\n').filter((line) => !line.includes(marker))
  const index = lines.findIndex((line) => line.includes(`{ ${firstLine}`))
  if (index >= 0) lines[index] = lines[index]!.replace(`{ ${firstLine}`, firstLine)
  return lines.join('\n').trimEnd()
}

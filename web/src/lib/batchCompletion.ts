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
export function wrapBatchCommand(command: string, marker: string, shell: 'posix' | 'powershell' = 'posix'): string {
  if (shell === 'powershell') {
    const payload = `\$global:LASTEXITCODE=0; try { & {\n${command}\n}; if (-not $?) { if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }; exit 1 }; exit $LASTEXITCODE } catch { Write-Error $_; exit 1 }`
    const bytes = new Uint8Array(payload.length * 2)
    for (let index = 0; index < payload.length; index++) { const code = payload.charCodeAt(index); bytes[index * 2] = code & 255; bytes[index * 2 + 1] = code >>> 8 }
    let binary = ''
    for (const byte of bytes) binary += String.fromCharCode(byte)
    return `& (Join-Path $PSHOME $(if ($PSVersionTable.PSEdition -eq 'Core') {'pwsh.exe'} else {'powershell.exe'})) -NoProfile -NonInteractive -EncodedCommand ${btoa(binary)}; [Console]::WriteLine(("\x60n${marker}{0}__" -f $LASTEXITCODE))\r`
  }
  const body = command.replace(/[\r\n]+$/, '')
  return `{ ${body}\n}; printf '\\n${marker}%s__\\n' "$?"\r`
}

export function batchExitCode(output: string, marker: string, shell: 'posix' | 'powershell' = 'posix'): number | null {
  const match = new RegExp(`${marker}(-?\\d{1,10})__`).exec(output)
  if (!match) return null
  const code = Number(match[1])
  return Number.isInteger(code) && (shell === 'powershell' ? code >= -2147483648 && code <= 2147483647 : code >= 0 && code <= 255) ? code : null
}

/** Removes the wrapper's echo and marker lines from displayed output. */
export function stripBatchWrapper(output: string, command: string, marker: string): string {
  const firstLine = command.replace(/[\r\n]+$/, '').split(/\r?\n/, 1)[0] ?? ''
  const lines = output.split('\n').filter((line) => !line.includes(marker))
  if (!firstLine.trim()) return lines.join('\n').trimEnd()
  // The shell may echo the wrapped line more than once (prompt redraws, line
  // wrapping); strip the group prefix from every echo, not just the first.
  return lines
    .map((line) => line.replaceAll(`{ ${firstLine}`, firstLine))
    .join('\n')
    .trimEnd()
}

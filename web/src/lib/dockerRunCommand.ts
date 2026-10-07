import type { DockerContainerRunCommand, DockerRunOption } from '@/types/api'

/**
 * One piece of a rendered command line. `concealed` is what to show while
 * sensitive values are hidden: the visible prefix (such as `DB_PASSWORD=`)
 * followed by a mask. `text` is always the real, shell-quoted value.
 */
export interface RunCommandSegment {
  text: string
  concealed?: { prefix: string }
}

export interface FormattedRunCommand {
  /** Display lines; continuation backslashes are added by the renderer. */
  lines: RunCommandSegment[][]
  /** Follow-up `docker network connect` commands, one per line. */
  followUps: RunCommandSegment[][]
  /** The complete command with real values, ready to paste into a shell. */
  text: string
  /** How many values are hidden while concealed. */
  secrets: number
}

export const runCommandMask = '••••••'

const strongSecretWords = /password|passwd|secret|token|credential|apikey|privatekey|accesskey|requirepass|masterauth/
const weakSecretWords = new Set(['pass', 'pwd', 'key', 'auth', 'salt', 'cookie', 'pin'])
const urlCredentials = /[a-z][a-z0-9+.-]*:\/\/[^/\s:@]+:[^/\s@]+@/i

/** Whether a variable, label or flag name usually carries a secret value. */
export function isSensitiveName(name: string): boolean {
  const lower = name.toLowerCase()
  if (strongSecretWords.test(lower.replace(/[^a-z0-9]/g, ''))) return true
  return lower.split(/[^a-z0-9]+/).some((word) => weakSecretWords.has(word))
}

export function shellQuote(value: string): string {
  if (value === '') return "''"
  if (/^[A-Za-z0-9@%+=:,./_-]+$/.test(value)) return value
  return `'${value.replaceAll("'", `'\\''`)}'`
}

function plain(text: string): RunCommandSegment {
  return { text }
}

function hidden(value: string, prefix: string): RunCommandSegment {
  return { text: shellQuote(value), concealed: { prefix } }
}

// KEY=VALUE pairs keep the key readable and hide only the value.
function assignment(value: string): RunCommandSegment {
  const separator = value.indexOf('=')
  const name = separator > 0 ? value.slice(0, separator) : ''
  if ((name && isSensitiveName(name)) || urlCredentials.test(value)) return hidden(value, name ? `${name}=` : '')
  return plain(shellQuote(value))
}

function optionSegments(option: DockerRunOption): RunCommandSegment[] {
  if (option.value === undefined) return [plain(option.flag)]
  const flag = plain(`${option.flag} `)
  switch (option.flag) {
    case '-e':
    case '--label':
    case '--log-opt':
      return [flag, assignment(option.value)]
    default:
      return [flag, urlCredentials.test(option.value) ? hidden(option.value, '') : plain(shellQuote(option.value))]
  }
}

// Application flags such as `--requirepass x` or `--password=x` after the image.
function commandSegments(command: string[]): RunCommandSegment[] {
  const segments: RunCommandSegment[] = []
  let hideNext = false
  for (const argument of command) {
    if (hideNext) {
      segments.push(hidden(argument, ''))
      hideNext = false
      continue
    }
    const flag = argument.match(/^(--?[A-Za-z0-9_.-]+)(=.*)?$/)
    if (flag && isSensitiveName(flag[1]!.replace(/^-+/, ''))) {
      if (flag[2] !== undefined) segments.push(hidden(argument, `${flag[1]}=`))
      else {
        segments.push(plain(shellQuote(argument)))
        hideNext = true
      }
      continue
    }
    segments.push(urlCredentials.test(argument) ? hidden(argument, '') : plain(shellQuote(argument)))
  }
  return segments
}

function joinSegments(segments: RunCommandSegment[]): RunCommandSegment[] {
  const result: RunCommandSegment[] = []
  segments.forEach((segment, index) => {
    if (index > 0) result.push(plain(' '))
    result.push(segment)
  })
  return result
}

export function lineText(line: RunCommandSegment[]): string {
  return line.map((segment) => segment.text).join('')
}

export function formatRunCommand(spec: DockerContainerRunCommand): FormattedRunCommand {
  const options = [...spec.options]
  const head: RunCommandSegment[] = [plain('docker run')]
  // Leading switches (-d, -it) read naturally on the first line.
  while (options.length && options[0]!.value === undefined) {
    head.push(plain(' '), plain(options.shift()!.flag))
  }
  const lines: RunCommandSegment[][] = [head]
  for (const option of options) lines.push(optionSegments(option))
  lines.push(joinSegments([plain(shellQuote(spec.image)), ...commandSegments(spec.command)]))

  const followUps = spec.networks.map((network) => {
    const parts: RunCommandSegment[] = [plain('docker network connect')]
    if (network.ip) parts.push(plain(`--ip ${shellQuote(network.ip)}`))
    if (network.ipv6) parts.push(plain(`--ip6 ${shellQuote(network.ipv6)}`))
    for (const alias of network.aliases || []) parts.push(plain(`--alias ${shellQuote(alias)}`))
    parts.push(plain(shellQuote(network.name)), plain(shellQuote(spec.name)))
    return joinSegments(parts)
  })

  const secrets = [...lines, ...followUps].flat().filter((segment) => segment.concealed).length
  const runText = lines.map(lineText).join(' \\\n  ')
  const text = [runText, ...followUps.map(lineText)].join('\n')
  return { lines, followUps, text, secrets }
}

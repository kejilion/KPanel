import type {
  DockerContainerCreateEnvironment,
  DockerContainerCreateMount,
  DockerContainerCreatePort,
  DockerMaintenanceInput,
} from '@/types/api'
import { yamlLanguage } from '@codemirror/lang-yaml'

export const maxComposeSourceBytes = 24 * 1024

export interface DockerDeploymentDiagnostic {
  code: string
  message: string
  hint?: string
  from: number
  to: number
  line: number
  column: number
  endLine: number
  endColumn: number
}

export interface DockerComposeVariable {
  name: string
  defaultValue?: string
  required: boolean
}

export type DockerDeploymentAnalysis =
  | { kind: 'empty' }
  | { kind: 'invalid'; message: string; diagnostics: DockerDeploymentDiagnostic[] }
  | { kind: 'docker-run'; input: DockerMaintenanceInput }
  | { kind: 'compose'; compose: string; projectName: string; services: string[] }

interface ShellToken {
  value: string
  from: number
  to: number
}

interface TokenizeResult {
  tokens: ShellToken[]
  diagnostic?: DockerDeploymentDiagnostic
}

export function analyzeDockerDeployment(source: string): DockerDeploymentAnalysis {
  const value = source.trim()
  if (!value) return { kind: 'empty' }
  if (new TextEncoder().encode(source).length > maxComposeSourceBytes) {
    return invalid(source, 'source_too_large', '部署内容不能超过 24 KiB。', 0, source.length,
      '请删除不需要的注释或拆分为更小的 Compose 项目。')
  }
  if (looksLikeDockerRun(value)) return parseDockerRun(source)
  if (/^\s*(?:name\s*:|version\s*:|services\s*:)/m.test(value)) {
    const composeSyntax = analyzeComposeSyntax(source)
    if (composeSyntax.diagnostics.length) return invalidDiagnostics(composeSyntax.diagnostics)
    const services = composeSyntax.services
    if (!services.length) {
      const servicesOffset = Math.max(0, source.search(/^\s*services\s*:/m))
      return invalid(source, 'compose_services_missing', '没有识别到 Compose services，请粘贴完整的 Compose YAML。',
        servicesOffset, lineEndOffset(source, servicesOffset), '请在 services: 下至少定义一个服务，例如 web:。')
    }
    const nameMatch = value.match(/^name\s*:\s*["']?([^\s"'#]+)["']?\s*(?:#.*)?$/m)
    if (nameMatch && !/^[a-z0-9][a-z0-9_-]{0,62}$/.test(nameMatch[1] || '')) {
      const from = Math.max(0, source.indexOf(nameMatch[1] || ''))
      return invalid(source, 'compose_project_name_invalid', 'Compose 项目名称格式无效。', from,
        from + (nameMatch[1]?.length || 1), 'name 仅支持小写字母、数字、连字符和下划线，最长 63 个字符。')
    }
    const explicitName = nameMatch?.[1]
    return {
      kind: 'compose',
      compose: source,
      projectName: explicitName || normalizeProjectName(services[0] || 'stack'),
      services,
    }
  }
  if (/\bdocker\s+compose\b/.test(value)) {
    const from = Math.max(0, source.search(/docker\s+compose/))
    return invalid(source, 'compose_command_only', '请粘贴 Compose YAML，而不是只有 docker compose up 命令。',
      from, lineEndOffset(source, from), '请复制 compose.yaml 或 docker-compose.yml 的完整内容。')
  }
  return invalid(source, 'deployment_kind_unknown', '请粘贴一条 docker run 命令，或完整的 Compose YAML。',
    firstContentOffset(source), lineEndOffset(source, firstContentOffset(source)), '内容应以 docker run 开始，或包含顶层 services:。')
}

export function composeEnvironmentVariables(source: string): DockerComposeVariable[] {
  const variables = new Map<string, DockerComposeVariable>()
  const interpolation = /\$\$|\$(?:\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-+?])([^}]*))?\}|([A-Za-z_][A-Za-z0-9_]*))/g
  for (const match of source.matchAll(interpolation)) {
    if (match[0] === '$$') continue
    const name = match[1] || match[4]
    if (!name) continue
    const operator = match[2] || ''
    const defaultValue = operator === '-' || operator === ':-' ? match[3] || '' : undefined
    const required = defaultValue === undefined && operator !== '+' && operator !== ':+'
    const previous = variables.get(name)
    const nextRequired = Boolean(previous?.required || required)
    variables.set(name, {
      name,
      defaultValue: nextRequired ? undefined : previous?.defaultValue ?? defaultValue,
      required: nextRequired,
    })
  }
  return [...variables.values()]
}

function looksLikeDockerRun(value: string): boolean {
  return /^(?:sudo\s+)?docker\s+run(?:\s|$)/.test(value.replace(/\\\r?\n/g, ' '))
}

function parseDockerRun(source: string): DockerDeploymentAnalysis {
  const tokenized = tokenizeShell(source)
  if (tokenized.diagnostic) return invalidDiagnostics([tokenized.diagnostic])
  const tokens = tokenized.tokens
  if (tokens[0]?.value === 'sudo') tokens.shift()
  if (tokens.shift()?.value !== 'docker' || tokens.shift()?.value !== 'run') {
    return invalid(source, 'docker_run_expected', '只支持单条 docker run 命令。', 0,
      Math.max(1, lineEndOffset(source, 0)), '命令应以 docker run 开始。')
  }

  let name = ''
  let network = 'bridge'
  // Preserve Docker CLI semantics for pasted commands. The manual form keeps
  // KPanel's friendlier unless-stopped default.
  let restartPolicy: DockerMaintenanceInput['restartPolicy'] = 'no'
  const ports: DockerContainerCreatePort[] = []
  const mounts: DockerContainerCreateMount[] = []
  const environment: DockerContainerCreateEnvironment[] = []
  let image = ''
  const command: string[] = []

  const nextValue = (index: number, inline: string | undefined, option: ShellToken): [string, number] | DockerDeploymentAnalysis => {
    if (inline !== undefined) return [inline, index]
    const value = tokens[index + 1]
    if (!value || isShellOperator(value.value)) {
      return invalid(source, 'docker_option_value_missing', `${option.value} 缺少参数。`, option.from, option.to,
        `请在 ${option.value} 后填写参数值。`)
    }
    return [value.value, index + 1]
  }

  for (let index = 0; index < tokens.length; index += 1) {
    const shellToken = tokens[index]!
    const token = shellToken.value
    if (isShellOperator(token)) {
      return invalid(source, 'shell_operator_unsupported', '一次只能部署一条 docker run 命令，不能包含管道、重定向或后续 Shell 命令。',
        shellToken.from, shellToken.to, '请只保留 docker run 命令；安装脚本和后续命令不能在此执行。')
    }
    if (image) {
      command.push(token)
      continue
    }
    if (!token.startsWith('-')) {
      image = token
      continue
    }
    if (token === '-d' || token === '--detach') continue
    if (/^-[dit]+$/.test(token) && token.includes('d') && !token.includes('i') && !token.includes('t')) continue
    if (token === '-i' || token === '-t' || token === '-it' || token === '-ti' || token === '--interactive' || token === '--tty') {
      return invalid(source, 'interactive_container_unsupported', '交互式 -it 容器不适合后台部署，请移除该参数或改用 Compose。',
        shellToken.from, shellToken.to, '删除 -i/-t，或使用 Compose 配置长期运行该服务。')
    }

    const [option, inline] = token.startsWith('--') && token.includes('=')
      ? [token.slice(0, token.indexOf('=')), token.slice(token.indexOf('=') + 1)]
      : [token, undefined]
    if (option === '--name') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      ;[name, index] = result
      continue
    }
    if (option === '--network' || option === '--net') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      ;[network, index] = result
      continue
    }
    if (option === '--restart') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      const [value, nextIndex] = result
      if (!['no', 'always', 'unless-stopped', 'on-failure'].includes(value)) {
        const valueToken = tokens[nextIndex] || shellToken
        return invalid(source, 'restart_policy_invalid', `不支持的重启策略：${value}`, valueToken.from, valueToken.to,
          '可选值：no、always、unless-stopped、on-failure。')
      }
      restartPolicy = value as DockerMaintenanceInput['restartPolicy']
      index = nextIndex
      continue
    }
    if (option === '-p' || option === '--publish') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      const parsed = parsePort(result[0])
      if (typeof parsed === 'string') {
        const valueToken = tokens[result[1]] || shellToken
        return invalid(source, 'port_mapping_invalid', parsed, valueToken.from, valueToken.to,
          '示例：-p 8080:80 或 -p 127.0.0.1:8080:80/tcp。')
      }
      ports.push(parsed)
      index = result[1]
      continue
    }
    if (option === '-v' || option === '--volume') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      const parsed = parseVolume(result[0])
      if (typeof parsed === 'string') {
        const valueToken = tokens[result[1]] || shellToken
        return invalid(source, 'volume_mapping_invalid', parsed, valueToken.from, valueToken.to,
          '示例：-v /home/docker/app:/data:rw。')
      }
      mounts.push(parsed)
      index = result[1]
      continue
    }
    if (option === '--mount') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      const parsed = parseMount(result[0])
      if (typeof parsed === 'string') {
        const valueToken = tokens[result[1]] || shellToken
        return invalid(source, 'mount_invalid', parsed, valueToken.from, valueToken.to,
          '示例：--mount type=bind,source=/home/app,target=/data。')
      }
      mounts.push(parsed)
      index = result[1]
      continue
    }
    if (option === '-e' || option === '--env') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      const separator = result[0].indexOf('=')
      if (separator <= 0) {
        const valueToken = tokens[result[1]] || shellToken
        return invalid(source, 'environment_invalid', `${option} 必须使用 NAME=VALUE。`, valueToken.from, valueToken.to,
          '示例：-e TZ=Asia/Shanghai。')
      }
      environment.push({ name: result[0].slice(0, separator), value: result[0].slice(separator + 1) })
      index = result[1]
      continue
    }
    if (option === '--pull') {
      const result = nextValue(index, inline, shellToken)
      if (!Array.isArray(result)) return result
      if (result[0] !== 'missing') {
        const valueToken = tokens[result[1]] || shellToken
        return invalid(source, 'pull_policy_unsupported', '当前仅支持 Docker 默认的 --pull=missing；如需更新镜像请先在镜像页拉取。',
          valueToken.from, valueToken.to, '改为 --pull=missing，或先从镜像页更新镜像。')
      }
      index = result[1]
      continue
    }
    return invalid(source, 'docker_option_unsupported', `暂不支持 ${option}；复杂参数建议改用 Compose YAML。`,
      shellToken.from, shellToken.to, '将此命令转换为 Compose YAML，可保留更多 Docker 参数。')
  }

  if (!image) {
    return invalid(source, 'docker_image_missing', 'docker run 命令缺少镜像。', Math.max(0, source.length - 1), source.length,
      '请在所有 docker run 选项后填写镜像，例如 nginx:alpine。')
  }
  return {
    kind: 'docker-run',
    input: {
      action: 'container_create',
      name,
      image,
      network,
      restartPolicy,
      command,
      ports,
      mounts,
      environment,
    },
  }
}

function tokenizeShell(source: string): TokenizeResult {
  const tokens: ShellToken[] = []
  let current = ''
  let currentFrom = -1
  let quote: "'" | '"' | '' = ''
  let quoteFrom = -1
  let escaped = false
  let escapeFrom = -1
  const flush = (to: number) => {
    if (current) tokens.push({ value: current, from: Math.max(0, currentFrom), to })
    current = ''
    currentFrom = -1
  }
  for (let index = 0; index < source.length; index += 1) {
    const char = source[index]!
    if (escaped) {
      if (char === '\n') {
        escaped = false
        continue
      }
      if (char === '\r' && source[index + 1] === '\n') {
        escaped = false
        index += 1
        continue
      }
      if (currentFrom < 0) currentFrom = escapeFrom
      current += char
      escaped = false
      continue
    }
    if (char === '\\' && quote !== "'") {
      if (currentFrom < 0) currentFrom = index
      escaped = true
      escapeFrom = index
      continue
    }
    if (quote) {
      if (char === quote) quote = ''
      else current += char
      continue
    }
    if (char === "'" || char === '"') {
      if (currentFrom < 0) currentFrom = index
      quote = char
      quoteFrom = index
      continue
    }
    if (/\s/.test(char)) {
      flush(index)
      continue
    }
    if (';|<>&'.includes(char)) {
      flush(index)
      const pair = source.slice(index, index + 2)
      if (pair === '&&' || pair === '||' || pair === '>>' || pair === '<<') {
        tokens.push({ value: pair, from: index, to: index + 2 })
        index += 1
      } else tokens.push({ value: char, from: index, to: index + 1 })
      continue
    }
    if (currentFrom < 0) currentFrom = index
    current += char
  }
  if (escaped) {
    return { tokens, diagnostic: diagnostic(source, 'shell_escape_unclosed', '命令末尾存在未完成的转义符。',
      escapeFrom, escapeFrom + 1, '删除末尾反斜杠，或在下一行继续填写命令。') }
  }
  if (quote) {
    return { tokens, diagnostic: diagnostic(source, 'shell_quote_unclosed', '命令中存在未闭合的引号。',
      quoteFrom, Math.max(quoteFrom + 1, source.length), `请补上对应的 ${quote} 引号。`) }
  }
  flush(source.length)
  return { tokens }
}

function analyzeComposeSyntax(source: string): {
  diagnostics: DockerDeploymentDiagnostic[]
  services: string[]
} {
  const tabOffset = source.search(/^\s*\t\s*\S/m)
  if (tabOffset >= 0) {
    const actualTab = source.indexOf('\t', tabOffset)
    return {
      diagnostics: [diagnostic(source, 'yaml_tab_indent', 'YAML 缩进不能使用 Tab。', actualTab, actualTab + 1,
        '请将 Tab 替换为空格；建议每层使用 2 个空格。')],
      services: [],
    }
  }
  const tree = yamlLanguage.parser.parse(source)
  const cursor = tree.cursor()
  const diagnostics: DockerDeploymentDiagnostic[] = []
  do {
    if (cursor.type.isError) {
      const from = Math.min(cursor.from, Math.max(0, source.length - 1))
      const to = Math.max(from + 1, cursor.to)
      diagnostics.push(diagnostic(source, 'yaml_syntax_error', 'YAML 语法不完整或缩进有误。', from, to,
        '检查这一行附近的冒号、引号、列表短横线和缩进层级。'))
      if (diagnostics.length >= 8) break
    }
  } while (cursor.next())
  return {
    diagnostics: deduplicateDiagnostics(diagnostics),
    services: diagnostics.length ? [] : composeServices(source, tree.topNode),
  }
}

function invalid(
  source: string,
  code: string,
  message: string,
  from: number,
  to: number,
  hint?: string,
): DockerDeploymentAnalysis {
  return invalidDiagnostics([diagnostic(source, code, message, from, to, hint)])
}

function invalidDiagnostics(diagnostics: DockerDeploymentDiagnostic[]): DockerDeploymentAnalysis {
  const normalized = diagnostics.length ? diagnostics : [{
    code: 'deployment_invalid', message: '部署内容无效。', from: 0, to: 1,
    line: 1, column: 1, endLine: 1, endColumn: 2,
  }]
  return { kind: 'invalid', message: normalized[0]!.message, diagnostics: normalized }
}

function diagnostic(
  source: string,
  code: string,
  message: string,
  from: number,
  to: number,
  hint?: string,
): DockerDeploymentDiagnostic {
  const safeFrom = Math.max(0, Math.min(from, source.length))
  const safeTo = Math.max(safeFrom, Math.min(Math.max(to, safeFrom + 1), source.length))
  const start = offsetPosition(source, safeFrom)
  const end = offsetPosition(source, safeTo)
  return {
    code, message, hint, from: safeFrom, to: safeTo,
    line: start.line, column: start.column, endLine: end.line, endColumn: end.column,
  }
}

function offsetPosition(source: string, offset: number): { line: number; column: number } {
  const before = source.slice(0, offset)
  const lines = before.split(/\r?\n/)
  return { line: lines.length, column: (lines.at(-1)?.length || 0) + 1 }
}

function firstContentOffset(source: string): number {
  const match = source.match(/\S/)
  return match?.index || 0
}

function lineEndOffset(source: string, offset: number): number {
  const end = source.indexOf('\n', Math.max(0, offset))
  return end < 0 ? source.length : end
}

function deduplicateDiagnostics(items: DockerDeploymentDiagnostic[]): DockerDeploymentDiagnostic[] {
  const seen = new Set<string>()
  return items.filter((item) => {
    const key = `${item.code}:${item.from}:${item.to}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function isShellOperator(value: string): boolean {
  return [';', '|', '||', '&', '&&', '>', '>>', '<', '<<'].includes(value)
}

function parsePort(value: string): DockerContainerCreatePort | string {
  let protocol: 'tcp' | 'udp' = 'tcp'
  if (value.endsWith('/udp')) {
    protocol = 'udp'
    value = value.slice(0, -4)
  } else if (value.endsWith('/tcp')) value = value.slice(0, -4)
  let hostIp = '0.0.0.0'
  let publicValue = ''
  let privateValue = ''
  if (value.startsWith('[')) {
    const end = value.indexOf(']:')
    if (end < 0) return `端口映射格式无效：${value}`
    hostIp = value.slice(1, end)
    const parts = value.slice(end + 2).split(':')
    publicValue = parts[0] || ''
    privateValue = parts[1] || ''
  } else {
    const parts = value.split(':')
    if (parts.length === 2) {
      publicValue = parts[0] || ''
      privateValue = parts[1] || ''
    } else if (parts.length === 3) {
      hostIp = parts[0] || ''
      publicValue = parts[1] || ''
      privateValue = parts[2] || ''
    }
    else return `端口映射必须使用 主机端口:容器端口：${value}`
  }
  const publicPort = Number(publicValue)
  const privatePort = Number(privateValue)
  if (![publicPort, privatePort].every((port) => Number.isInteger(port) && port >= 1 && port <= 65535)) {
    return `端口必须是 1-65535 的整数：${value}`
  }
  return { publicPort, privatePort, protocol, hostIp }
}

function parseVolume(value: string): DockerContainerCreateMount | string {
  const parts = value.split(':')
  if (parts.length < 2 || parts.length > 3) return `存储挂载格式无效：${value}`
  const [source, target, mode = 'rw'] = parts
  if (!source || !target?.startsWith('/')) return `存储挂载必须包含来源和容器绝对路径：${value}`
  if (!['ro', 'rw'].includes(mode)) return `暂不支持挂载模式 ${mode}，请改用 Compose。`
  return { type: source.startsWith('/') ? 'bind' : 'volume', source, target, readOnly: mode === 'ro' }
}

function parseMount(value: string): DockerContainerCreateMount | string {
  const options = new Map<string, string>()
  let readOnly = false
  for (const part of value.split(',')) {
    const [rawKey, ...rest] = part.split('=')
    const key = rawKey?.trim() || ''
    if (key === 'readonly' || key === 'ro') {
      readOnly = true
      continue
    }
    options.set(key, rest.join('=').trim())
  }
  const type = options.get('type') || 'volume'
  const source = options.get('source') || options.get('src') || ''
  const target = options.get('target') || options.get('dst') || options.get('destination') || ''
  if ((type !== 'bind' && type !== 'volume') || !source || !target.startsWith('/')) {
    return `--mount 需要有效的 type、source 和 target：${value}`
  }
  return { type, source, target, readOnly }
}

function composeServices(source: string, topNode: ReturnType<typeof yamlLanguage.parser.parse>['topNode']): string[] {
  const document = topNode.getChild('Document')
  const root = document?.getChild('BlockMapping')
  if (!root) return []
  for (const pair of root.getChildren('Pair')) {
    const key = pair.getChild('Key')
    if (!key || yamlKey(source.slice(key.from, key.to)) !== 'services') continue
    const mapping = pair.getChild('BlockMapping') || pair.getChild('FlowMapping')
    if (!mapping) return []
    const services: string[] = []
    for (const service of mapping.getChildren('Pair')) {
      const serviceKey = service.getChild('Key')
      if (!serviceKey) continue
      const name = yamlKey(source.slice(serviceKey.from, serviceKey.to))
      if (/^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(name)) services.push(name)
    }
    return [...new Set(services)]
  }
  return []
}

function yamlKey(value: string): string {
  const trimmed = value.trim()
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      const decoded = JSON.parse(trimmed)
      return typeof decoded === 'string' ? decoded : trimmed
    } catch {
      return trimmed
    }
  }
  if (trimmed.startsWith("'") && trimmed.endsWith("'")) {
    return trimmed.slice(1, -1).replace(/''/g, "'")
  }
  return trimmed
}

function normalizeProjectName(value: string): string {
  const normalized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return (normalized || 'stack').slice(0, 63)
}

import { describe, expect, it } from 'vitest'
import { analyzeDockerDeployment, composeEnvironmentVariables } from './dockerDeployment'

describe('docker deployment input', () => {
  it('turns a multiline docker run command into structured input', () => {
    const result = analyzeDockerDeployment(`docker run -d \\
      --name demo \\
      -p 127.0.0.1:8080:80/tcp \\
      -v /home/docker/demo:/data:ro \\
      -e TZ=Asia/Shanghai \\
      --restart always nginx:alpine --config /data/app.yml`)
    expect(result.kind).toBe('docker-run')
    if (result.kind !== 'docker-run') return
    expect(result.input).toMatchObject({
      name: 'demo', image: 'nginx:alpine', restartPolicy: 'always',
      command: ['--config', '/data/app.yml'],
      ports: [{ hostIp: '127.0.0.1', publicPort: 8080, privatePort: 80, protocol: 'tcp' }],
      mounts: [{ type: 'bind', source: '/home/docker/demo', target: '/data', readOnly: true }],
      environment: [{ name: 'TZ', value: 'Asia/Shanghai' }],
    })
  })

  it('keeps Docker restart semantics when --restart is omitted', () => {
    const result = analyzeDockerDeployment('docker run -d nginx:alpine')
    expect(result.kind).toBe('docker-run')
    if (result.kind !== 'docker-run') return
    expect(result.input.restartPolicy).toBe('no')
  })

  it('recognizes Compose YAML and derives a project name', () => {
    const result = analyzeDockerDeployment(`services:
  web:
    image: nginx:alpine
  redis:
    image: redis:alpine
`)
    expect(result).toMatchObject({ kind: 'compose', projectName: 'web', services: ['web', 'redis'] })
  })

  it('recognizes flow-style and quoted Compose service keys from the YAML tree', () => {
    expect(analyzeDockerDeployment(`services: { web: { image: nginx:alpine }, 'db-worker': { image: postgres:17 } }`))
      .toMatchObject({ kind: 'compose', projectName: 'web', services: ['web', 'db-worker'] })
  })

  it('rejects chained shell commands instead of executing text', () => {
    const result = analyzeDockerDeployment('docker run -d nginx:alpine && curl https://example.com/install.sh | sh')
    expect(result).toMatchObject({ kind: 'invalid' })
  })

  it('explains that a Compose command without YAML is incomplete', () => {
    expect(analyzeDockerDeployment('docker compose up -d')).toMatchObject({
      kind: 'invalid', message: '请粘贴 Compose YAML，而不是只有 docker compose up 命令。',
      diagnostics: [{ code: 'compose_command_only', line: 1, column: 1 }],
    })
  })

  it('locates an unclosed quote in a multiline docker run command', () => {
    const result = analyzeDockerDeployment(`docker run -d \\
  --name "demo \\
  nginx:alpine`)
    expect(result).toMatchObject({
      kind: 'invalid',
      diagnostics: [{ code: 'shell_quote_unclosed', line: 2, column: 10 }],
    })
  })

  it('locates unsupported shell chaining instead of only returning a message', () => {
    const result = analyzeDockerDeployment(`docker run -d nginx:alpine \\
  && curl https://example.com/install.sh`)
    expect(result).toMatchObject({
      kind: 'invalid',
      diagnostics: [{ code: 'shell_operator_unsupported', line: 2, column: 3 }],
    })
  })

  it('keeps diagnostic positions aligned when pasted commands have leading blank lines', () => {
    const result = analyzeDockerDeployment(`\n\n docker run -d nginx:alpine && curl example.com`)
    expect(result).toMatchObject({
      kind: 'invalid',
      diagnostics: [{ code: 'shell_operator_unsupported', line: 3, column: 29 }],
    })
  })

  it('reports YAML indentation errors with a line and column', () => {
    const result = analyzeDockerDeployment(`services:
  web:
\t image: nginx:alpine`)
    expect(result).toMatchObject({
      kind: 'invalid',
      diagnostics: [{ code: 'yaml_tab_indent', line: 3, column: 1 }],
    })
  })

  it('reports malformed YAML from the lightweight syntax tree', () => {
    const result = analyzeDockerDeployment(`services:
  web
    image: nginx:alpine`)
    expect(result.kind).toBe('invalid')
    if (result.kind !== 'invalid') return
    expect(result.diagnostics[0]).toMatchObject({ code: 'yaml_syntax_error' })
    expect(result.diagnostics[0]?.line).toBeGreaterThanOrEqual(2)
  })
})

describe('compose environment variables', () => {
  it('collects required variables and defaults without treating escaped dollars as interpolation', () => {
    expect(composeEnvironmentVariables(`services:
  app:
    image: "demo:\${TAG:-latest}"
    environment:
      TOKEN: "\${TOKEN:?TOKEN is required}"
      EMPTY: "\${OPTIONAL}"
      LITERAL: "$$NOT_A_VARIABLE"
`)).toEqual([
      { name: 'TAG', defaultValue: 'latest', required: false },
      { name: 'TOKEN', defaultValue: undefined, required: true },
      { name: 'OPTIONAL', defaultValue: undefined, required: true },
    ])
  })

  it('deduplicates variables and keeps the strictest occurrence', () => {
    expect(composeEnvironmentVariables('image: "demo:$TAG-\${TAG:-latest}-\${TAG}"')).toEqual([
      { name: 'TAG', defaultValue: undefined, required: true },
    ])
  })
})

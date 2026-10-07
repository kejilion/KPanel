import { describe, expect, it } from 'vitest'
import { analyzeDockerDeployment } from '@/lib/dockerDeployment'
import { formatRunCommand, isSensitiveName, lineText, shellQuote } from '@/lib/dockerRunCommand'
import type { DockerContainerRunCommand } from '@/types/api'

function spec(overrides: Partial<DockerContainerRunCommand> = {}): DockerContainerRunCommand {
  return {
    containerId: 'a'.repeat(64),
    name: 'web',
    image: 'nginx:alpine',
    options: [{ flag: '-d' }, { flag: '--name', value: 'web' }],
    command: [],
    networks: [],
    unsupported: [],
    imageDefaults: true,
    collectedAt: '2026-10-07T08:00:00Z',
    ...overrides,
  }
}

describe('docker run command formatting', () => {
  it('quotes only values the shell would split or expand', () => {
    expect(shellQuote('nginx:alpine')).toBe('nginx:alpine')
    expect(shellQuote('/home/web/conf.d:/etc/nginx/conf.d:ro')).toBe('/home/web/conf.d:/etc/nginx/conf.d:ro')
    expect(shellQuote('')).toBe("''")
    expect(shellQuote('daemon off;')).toBe("'daemon off;'")
    expect(shellQuote("it's $HOME")).toBe("'it'\\''s $HOME'")
  })

  it('puts leading switches on the first line and one option per line after it', () => {
    const result = formatRunCommand(spec({
      options: [
        { flag: '-d' }, { flag: '-it' }, { flag: '--name', value: 'web' },
        { flag: '--restart', value: 'unless-stopped' }, { flag: '-p', value: '8080:80' },
        { flag: '--privileged' },
      ],
      command: ['nginx', '-g', 'daemon off;'],
    }))
    expect(result.lines.map(lineText)).toEqual([
      'docker run -d -it',
      '--name web',
      '--restart unless-stopped',
      '-p 8080:80',
      '--privileged',
      "nginx:alpine nginx -g 'daemon off;'",
    ])
    expect(result.text).toBe([
      'docker run -d -it \\',
      '  --name web \\',
      '  --restart unless-stopped \\',
      '  -p 8080:80 \\',
      '  --privileged \\',
      "  nginx:alpine nginx -g 'daemon off;'",
    ].join('\n'))
    expect(result.secrets).toBe(0)
  })

  it('hides likely secrets in display but keeps real values in the copied text', () => {
    const result = formatRunCommand(spec({
      image: 'redis:7',
      options: [
        { flag: '-d' },
        { flag: '-e', value: 'TZ=Asia/Shanghai' },
        { flag: '-e', value: 'MYSQL_ROOT_PASSWORD=p@ss word' },
        { flag: '-e', value: 'APP_KEY=base64:abc' },
        { flag: '-e', value: 'DATABASE_URL=postgres://app:hunter2@db:5432/app' },
        { flag: '--label', value: 'traefik.http.middlewares.auth.basicauth.users=admin:$apr1$x' },
        { flag: '--log-opt', value: 'splunk-token=abc' },
      ],
      command: ['redis-server', '--requirepass', 'hunter2', '--appendonly', 'yes', '--api-token=xyz'],
    }))
    const concealed = result.lines.flat().filter((segment) => segment.concealed)
    expect(concealed.map((segment) => segment.concealed!.prefix)).toEqual([
      'MYSQL_ROOT_PASSWORD=', 'APP_KEY=', 'DATABASE_URL=',
      'traefik.http.middlewares.auth.basicauth.users=', 'splunk-token=', '', '--api-token=',
    ])
    expect(result.secrets).toBe(7)
    expect(result.text).toContain("-e 'MYSQL_ROOT_PASSWORD=p@ss word'")
    expect(result.text).toContain('redis-server --requirepass hunter2 --appendonly yes --api-token=xyz')
    expect(result.lines.flat().find((segment) => segment.text === 'TZ=Asia/Shanghai')?.concealed).toBeUndefined()
  })

  it('recognises common secret names without hiding ordinary ones', () => {
    for (const name of ['DB_PASSWORD', 'POSTGRES_PASSWORD', 'GITHUB_TOKEN', 'SECRET_KEY_BASE', 'AWS_ACCESS_KEY_ID', 'pwd', 'masterauth', 'PRIVATE_KEY']) {
      expect(isSensitiveName(name), name).toBe(true)
    }
    for (const name of ['TZ', 'PUID', 'PATH', 'LANG', 'KEYBOARD_LAYOUT', 'MONKEY', 'PASSIVE_PORTS', 'traefik.enable']) {
      expect(isSensitiveName(name), name).toBe(false)
    }
  })

  it('hides secret option values that begin with a dash', () => {
    const result = formatRunCommand(spec({ command: ['redis-server', '--requirepass', '-hunter2'] }))
    const secret = result.lines.flat().find((segment) => segment.concealed)

    expect(secret?.concealed).toEqual({ prefix: '' })
    expect(secret?.text).toBe('-hunter2')
    expect(result.secrets).toBe(1)
  })

  it('adds docker network connect lines for additional networks', () => {
    const result = formatRunCommand(spec({
      networks: [{ name: 'frontend', ip: '172.31.0.5', aliases: ['www'] }, { name: 'monitor' }],
    }))
    expect(result.followUps.map(lineText)).toEqual([
      'docker network connect --ip 172.31.0.5 --alias www frontend web',
      'docker network connect monitor web',
    ])
    expect(result.text.split('\n').slice(-2)).toEqual([
      'docker network connect --ip 172.31.0.5 --alias www frontend web',
      'docker network connect monitor web',
    ])
  })

  it('round-trips the options the deployment editor understands', () => {
    const result = formatRunCommand(spec({
      name: 'app',
      image: 'ghcr.io/acme/app:2',
      options: [
        { flag: '-d' },
        { flag: '--name', value: 'app' },
        { flag: '--restart', value: 'always' },
        { flag: '--network', value: 'backend' },
        { flag: '-p', value: '127.0.0.1:8080:80' },
        { flag: '-p', value: '5353:53/udp' },
        { flag: '-v', value: '/home/docker/app:/data' },
        { flag: '-v', value: 'cache:/cache:ro' },
        { flag: '-e', value: 'GREETING=hello world' },
        { flag: '-e', value: "QUOTE=it's" },
      ],
      command: ['--config', '/data/app.yml'],
    }))
    const analysis = analyzeDockerDeployment(result.text)
    expect(analysis.kind).toBe('docker-run')
    if (analysis.kind !== 'docker-run') return
    expect(analysis.input).toMatchObject({
      name: 'app',
      image: 'ghcr.io/acme/app:2',
      network: 'backend',
      restartPolicy: 'always',
      command: ['--config', '/data/app.yml'],
      environment: [{ name: 'GREETING', value: 'hello world' }, { name: 'QUOTE', value: "it's" }],
    })
    expect(analysis.input.ports).toHaveLength(2)
    expect(analysis.input.mounts).toHaveLength(2)
  })
})

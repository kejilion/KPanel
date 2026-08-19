import { describe, expect, it } from 'vitest'
import { dockerComposeGroupAccent, groupDockerContainers } from './dockerComposeGroups'
import type { DockerContainer } from '@/types/api'

function container(name: string, project?: string, service?: string, state: DockerContainer['state'] = 'running'): DockerContainer {
  return {
    id: name, name, image: 'example:latest', state, project, service,
    access: 'managed', consistency: 'synced', ports: [], networks: [], mounts: [],
  }
}

describe('Docker Compose container groups', () => {
  it('keeps Compose services together and standalone containers in one final group', () => {
    const groups = groupDockerContainers([
      container('worker', 'demo', 'worker', 'exited'),
      container('redis'),
      container('web', 'demo', 'web'),
      container('api', 'api-stack', 'api'),
    ])
    expect(groups.map((group) => group.key)).toEqual(['compose:api-stack', 'compose:demo', 'standalone'])
    expect(groups[1]).toMatchObject({ name: 'demo', services: ['web', 'worker'], running: 1 })
    expect(groups[1]?.containers.map((item) => item.name)).toEqual(['worker', 'web'])
  })

  it('omits the standalone group when every container belongs to Compose', () => {
    expect(groupDockerContainers([container('web', 'demo', 'web')])).toHaveLength(1)
  })

  it('keeps managed Compose projects visible after every container is deleted', () => {
    expect(groupDockerContainers([], 'smart', ['demo', 'demo'])).toEqual([{
      key: 'compose:demo', kind: 'compose', name: 'demo', containers: [], services: [], running: 0,
    }])
  })

  it('assigns stable, varied accents from the Compose project name', () => {
    expect(dockerComposeGroupAccent('wordpress')).toBe(dockerComposeGroupAccent('wordpress'))
    expect(dockerComposeGroupAccent('monitoring')).not.toBe(dockerComposeGroupAccent('shop'))
    expect(new Set(['wordpress', 'monitoring', 'media', 'shop'].map(dockerComposeGroupAccent)).size).toBeGreaterThan(1)
    expect(dockerComposeGroupAccent('中文项目')).toMatch(/^#[0-9a-f]{6}$/)
  })

  it('orders Compose groups by their earliest container creation time', () => {
    const groups = groupDockerContainers([
      { ...container('new-worker', 'new-stack', 'worker'), createdAt: '2026-08-18T10:00:00Z' },
      { ...container('old-web', 'old-stack', 'web'), createdAt: '2026-08-10T10:00:00Z' },
      { ...container('new-web', 'new-stack', 'web'), createdAt: '2026-08-12T10:00:00Z' },
      { ...container('old-worker', 'old-stack', 'worker'), createdAt: '2026-08-11T10:00:00Z' },
      container('missing', 'missing-stack', 'web'),
    ], 'created-desc')

    expect(groups.map((group) => group.name)).toEqual(['new-stack', 'old-stack', 'missing-stack'])
    expect(groups[0]?.createdAtMs).toBe(Date.parse('2026-08-12T10:00:00Z'))
    expect(groups[1]?.createdAtMs).toBe(Date.parse('2026-08-10T10:00:00Z'))
    expect(groupDockerContainers(groups.flatMap((group) => group.containers), 'created-asc').map((group) => group.name)).toEqual([
      'old-stack', 'new-stack', 'missing-stack',
    ])
  })

  it('follows name direction for group sorting and keeps smart sorting stable', () => {
    const containers = [
      container('web', 'alpha'),
      container('web', 'beta'),
      container('web', 'gamma'),
    ]

    expect(groupDockerContainers(containers, 'name-desc').map((group) => group.name)).toEqual(['gamma', 'beta', 'alpha'])
    expect(groupDockerContainers(containers, 'smart').map((group) => group.name)).toEqual(['alpha', 'beta', 'gamma'])
  })
})

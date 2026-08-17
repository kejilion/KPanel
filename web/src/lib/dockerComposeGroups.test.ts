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
    expect(groupDockerContainers([], ['demo', 'demo'])).toEqual([{
      key: 'compose:demo', kind: 'compose', name: 'demo', containers: [], services: [], running: 0,
    }])
  })

  it('assigns stable, varied accents from the Compose project name', () => {
    expect(dockerComposeGroupAccent('wordpress')).toBe(dockerComposeGroupAccent('wordpress'))
    expect(dockerComposeGroupAccent('monitoring')).not.toBe(dockerComposeGroupAccent('shop'))
    expect(new Set(['wordpress', 'monitoring', 'media', 'shop'].map(dockerComposeGroupAccent)).size).toBeGreaterThan(1)
    expect(dockerComposeGroupAccent('中文项目')).toMatch(/^#[0-9a-f]{6}$/)
  })
})

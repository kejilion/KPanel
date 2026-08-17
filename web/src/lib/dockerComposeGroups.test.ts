import { describe, expect, it } from 'vitest'
import { groupDockerContainers } from './dockerComposeGroups'
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
})

import { describe, expect, it } from 'vitest'
import {
  sortDockerContainers,
  sortDockerImages,
  sortDockerNetworks,
  sortDockerVolumes,
} from './dockerSorting'
import type { DockerContainer, DockerImage, DockerNetwork, DockerVolume } from '@/types/api'

const container = (name: string, state: DockerContainer['state']): DockerContainer => ({
  id: name,
  name,
  image: 'example:latest',
  state,
  access: 'managed',
  consistency: 'unknown',
  ports: [],
  networks: [],
  mounts: [],
})

describe('Docker resource sorting', () => {
  it('puts active containers first and uses natural numeric name order', () => {
    const items = [container('worker', 'exited'), container('api-10', 'running'), container('api-2', 'running')]
    expect(sortDockerContainers(items, 'smart').map((item) => item.name)).toEqual(['api-2', 'api-10', 'worker'])
    expect(sortDockerContainers(items, 'name-desc').map((item) => item.name)).toEqual(['worker', 'api-10', 'api-2'])
    expect(items.map((item) => item.name)).toEqual(['worker', 'api-10', 'api-2'])
  })

  it('sorts containers by creation time in either direction and keeps missing dates last', () => {
    const items = [
      { ...container('missing', 'running'), createdAt: undefined },
      { ...container('new', 'running'), createdAt: '2026-08-18T10:00:00Z' },
      { ...container('old', 'running'), createdAt: '2026-08-17T10:00:00Z' },
    ]

    expect(sortDockerContainers(items, 'created-desc').map((item) => item.name)).toEqual(['new', 'old', 'missing'])
    expect(sortDockerContainers(items, 'created-asc').map((item) => item.name)).toEqual(['old', 'new', 'missing'])
    expect(items.map((item) => item.name)).toEqual(['missing', 'new', 'old'])
  })

  it('puts images and volumes currently in use first', () => {
    const images = [
      { id: 'i1', tags: ['unused:latest'], sizeBytes: 1, inUse: false },
      { id: 'i2', tags: ['used:latest'], sizeBytes: 1, inUse: true },
    ] satisfies DockerImage[]
    const volumes = [
      { name: 'unused', driver: 'local', inUse: false },
      { name: 'used', driver: 'local', inUse: true },
    ] satisfies DockerVolume[]
    expect(sortDockerImages(images, 'smart')[0]?.id).toBe('i2')
    expect(sortDockerVolumes(volumes, 'smart')[0]?.name).toBe('used')
  })

  it('puts the busiest Docker networks first', () => {
    const networks = [
      { id: 'n1', name: 'idle', driver: 'bridge', containers: 0 },
      { id: 'n2', name: 'busy', driver: 'bridge', containers: 4 },
    ] satisfies DockerNetwork[]
    expect(sortDockerNetworks(networks, 'smart').map((item) => item.name)).toEqual(['busy', 'idle'])
  })
})

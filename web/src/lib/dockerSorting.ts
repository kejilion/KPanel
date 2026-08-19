import type { DockerContainer, DockerImage, DockerNetwork, DockerVolume } from '@/types/api'

export type ResourceSort = 'smart' | 'name-asc' | 'name-desc'
export type ContainerSort = ResourceSort | 'created-asc' | 'created-desc'

const nameCollator = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })
const containerStateRank: Record<DockerContainer['state'], number> = {
  running: 0,
  restarting: 1,
  paused: 2,
  created: 3,
  exited: 4,
  dead: 5,
  unknown: 6,
}

function compareName(left: string, right: string, sort: ResourceSort): number {
  return sort === 'name-desc'
    ? nameCollator.compare(right, left)
    : nameCollator.compare(left, right)
}

function compareCreatedAt(left?: string, right?: string, descending = false): number {
  const leftTime = left ? Date.parse(left) : Number.NaN
  const rightTime = right ? Date.parse(right) : Number.NaN
  const leftValid = Number.isFinite(leftTime)
  const rightValid = Number.isFinite(rightTime)
  if (!leftValid && !rightValid) return 0
  if (!leftValid) return 1
  if (!rightValid) return -1
  return descending ? rightTime - leftTime : leftTime - rightTime
}

export function sortDockerContainers(items: DockerContainer[], sort: ContainerSort): DockerContainer[] {
  return [...items].sort((left, right) => {
    if (sort === 'created-asc' || sort === 'created-desc') {
      return compareCreatedAt(left.createdAt, right.createdAt, sort === 'created-desc') || compareName(left.name, right.name, 'name-asc')
    }
    if (sort !== 'smart') return compareName(left.name, right.name, sort)
    return containerStateRank[left.state] - containerStateRank[right.state] || compareName(left.name, right.name, sort)
  })
}

export function sortDockerImages(items: DockerImage[], sort: ResourceSort): DockerImage[] {
  return [...items].sort((left, right) => {
    const leftName = left.tags[0] || left.id
    const rightName = right.tags[0] || right.id
    if (sort !== 'smart') return compareName(leftName, rightName, sort)
    return Number(right.inUse) - Number(left.inUse) || compareName(leftName, rightName, sort)
  })
}

export function sortDockerNetworks(items: DockerNetwork[], sort: ResourceSort): DockerNetwork[] {
  return [...items].sort((left, right) => {
    if (sort !== 'smart') return compareName(left.name, right.name, sort)
    return (right.containers || 0) - (left.containers || 0) || compareName(left.name, right.name, sort)
  })
}

export function sortDockerVolumes(items: DockerVolume[], sort: ResourceSort): DockerVolume[] {
  return [...items].sort((left, right) => {
    if (sort !== 'smart') return compareName(left.name, right.name, sort)
    return Number(right.inUse) - Number(left.inUse) || compareName(left.name, right.name, sort)
  })
}

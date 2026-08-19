import type { DockerContainer } from '@/types/api'
import type { ContainerSort } from './dockerSorting'

export interface DockerContainerGroup {
  key: string
  kind: 'compose' | 'standalone'
  name: string
  containers: DockerContainer[]
  services: string[]
  running: number
  createdAtMs?: number
}

const composeGroupAccents = ['#25b99a', '#4f86e8', '#9a6bd6', '#2ca8c2', '#d08b45', '#c084b8'] as const

export function dockerComposeGroupAccent(projectName: string): string {
  let hash = 0
  for (const character of projectName) {
    hash = (Math.imul(hash, 31) + (character.codePointAt(0) ?? 0)) >>> 0
  }
  return composeGroupAccents[hash % composeGroupAccents.length] ?? composeGroupAccents[0]
}

const groupNameCollator = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })

function compareGroups(left: DockerContainerGroup, right: DockerContainerGroup, sort: ContainerSort): number {
  if (sort === 'created-asc' || sort === 'created-desc') {
    const leftCreatedAt = left.createdAtMs
    const rightCreatedAt = right.createdAtMs
    const leftValid = leftCreatedAt !== undefined
    const rightValid = rightCreatedAt !== undefined
    if (!leftValid && !rightValid) return groupNameCollator.compare(left.name, right.name)
    if (!leftValid) return 1
    if (!rightValid) return -1
    const byCreatedAt = sort === 'created-desc'
      ? rightCreatedAt - leftCreatedAt
      : leftCreatedAt - rightCreatedAt
    return byCreatedAt || groupNameCollator.compare(left.name, right.name)
  }

  return sort === 'name-desc'
    ? groupNameCollator.compare(right.name, left.name)
    : groupNameCollator.compare(left.name, right.name)
}

export function groupDockerContainers(
  containers: DockerContainer[],
  sort: ContainerSort = 'smart',
  managedProjects: string[] = [],
): DockerContainerGroup[] {
  const compose = new Map<string, DockerContainer[]>()
  const sortByCreatedAt = sort === 'created-asc' || sort === 'created-desc'
  const composeCreatedAt = new Map<string, number>()
  const standalone: DockerContainer[] = []
  for (const container of containers) {
    if (!container.project) {
      standalone.push(container)
      continue
    }
    const group = compose.get(container.project) || []
    group.push(container)
    compose.set(container.project, group)
    if (sortByCreatedAt && container.createdAt) {
      const timestamp = Date.parse(container.createdAt)
      const earliest = composeCreatedAt.get(container.project)
      if (Number.isFinite(timestamp) && (earliest === undefined || timestamp < earliest)) {
        composeCreatedAt.set(container.project, timestamp)
      }
    }
  }
  for (const project of managedProjects) {
    if (project && !compose.has(project)) compose.set(project, [])
  }
  const groups: DockerContainerGroup[] = [...compose.entries()].map(([name, items]) => ({
    key: `compose:${name}`,
    kind: 'compose' as const,
    name,
    containers: items,
    services: [...new Set(items.map((item) => item.service).filter((item): item is string => Boolean(item)))].sort(
      (left, right) => groupNameCollator.compare(left, right),
    ),
    running: items.filter((item) => item.state === 'running').length,
    createdAtMs: composeCreatedAt.get(name),
  }))
  groups.sort((left, right) => compareGroups(left, right, sort))
  if (standalone.length) {
    groups.push({
      key: 'standalone', kind: 'standalone', name: '独立容器', containers: standalone,
      services: [], running: standalone.filter((item) => item.state === 'running').length,
    })
  }
  return groups
}

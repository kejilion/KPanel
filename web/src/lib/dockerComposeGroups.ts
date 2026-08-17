import type { DockerContainer } from '@/types/api'

export interface DockerContainerGroup {
  key: string
  kind: 'compose' | 'standalone'
  name: string
  containers: DockerContainer[]
  services: string[]
  running: number
}

export function groupDockerContainers(containers: DockerContainer[]): DockerContainerGroup[] {
  const compose = new Map<string, DockerContainer[]>()
  const standalone: DockerContainer[] = []
  for (const container of containers) {
    if (!container.project) {
      standalone.push(container)
      continue
    }
    const group = compose.get(container.project) || []
    group.push(container)
    compose.set(container.project, group)
  }
  const groups: DockerContainerGroup[] = [...compose.entries()].map(([name, items]) => ({
    key: `compose:${name}`,
    kind: 'compose' as const,
    name,
    containers: items,
    services: [...new Set(items.map((item) => item.service).filter((item): item is string => Boolean(item)))].sort(
      (left, right) => left.localeCompare(right, undefined, { numeric: true, sensitivity: 'base' }),
    ),
    running: items.filter((item) => item.state === 'running').length,
  }))
  groups.sort((left, right) => left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' }))
  if (standalone.length) {
    groups.push({
      key: 'standalone', kind: 'standalone', name: '独立容器', containers: standalone,
      services: [], running: standalone.filter((item) => item.state === 'running').length,
    })
  }
  return groups
}

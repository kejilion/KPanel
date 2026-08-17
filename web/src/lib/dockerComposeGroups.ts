import type { DockerContainer } from '@/types/api'

export interface DockerContainerGroup {
  key: string
  kind: 'compose' | 'standalone'
  name: string
  containers: DockerContainer[]
  services: string[]
  running: number
}

const composeGroupAccents = ['#25b99a', '#4f86e8', '#9a6bd6', '#2ca8c2', '#d08b45', '#c084b8'] as const

export function dockerComposeGroupAccent(projectName: string): string {
  let hash = 0
  for (const character of projectName) {
    hash = (Math.imul(hash, 31) + (character.codePointAt(0) ?? 0)) >>> 0
  }
  return composeGroupAccents[hash % composeGroupAccents.length] ?? composeGroupAccents[0]
}

export function groupDockerContainers(containers: DockerContainer[], managedProjects: string[] = []): DockerContainerGroup[] {
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
  for (const project of managedProjects) {
    if (project && !compose.has(project)) compose.set(project, [])
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

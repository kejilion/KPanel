import { describe, expect, it, vi } from 'vitest'
import {
  createDockerBatchRun,
  dockerBatchNotes,
  dockerBatchSelfWarning,
  DockerBatchUnconfirmedError,
  executeDockerBatch,
  isKPanelContainer,
  normalizeImageReference,
  planContainerBatch,
  planImageBatch,
  planNetworkBatch,
  planVolumeBatch,
  runDockerBatchTask,
  summarizeDockerBatch,
} from './dockerBatch'
import type { DockerContainer, DockerImage, DockerMaintenanceJob, DockerNetwork, DockerVolume } from '@/types/api'

const runningActions = ['logs', 'stats', 'restart', 'pause', 'stop', 'remove']

function container(name: string, state: DockerContainer['state'], extra: Partial<DockerContainer> = {}): DockerContainer {
  const allowedActions = state === 'running' ? runningActions
    : state === 'paused' ? ['logs', 'unpause', 'restart', 'stop', 'remove']
      : ['logs', 'start', 'remove']
  return {
    id: `${name}-id`, name, image: `${name}:latest`, state, access: 'managed', consistency: 'synced',
    ports: [], networks: ['bridge'], mounts: [], allowedActions, resourceVersion: `sha256:${name}`, ...extra,
  }
}

function image(id: string, tags: string[], extra: Partial<DockerImage> = {}): DockerImage {
  return { id, tags, sizeBytes: 1024, inUse: false, resourceVersion: `v-${id}`, ...extra }
}

describe('container batch planning', () => {
  it('maps start onto start or unpause and skips containers already running', () => {
    const plan = planContainerBatch([
      container('web', 'running'), container('worker', 'exited'), container('cache', 'paused'),
    ], 'start')
    expect(plan.items.map((item) => [item.label, item.operation])).toEqual([
      ['worker', { kind: 'container', id: 'worker-id', action: 'start', resourceVersion: 'sha256:worker' }],
      ['cache', { kind: 'container', id: 'cache-id', action: 'unpause', resourceVersion: 'sha256:cache' }],
    ])
    expect(plan.items[1]!.note).toBe('已暂停，将继续运行')
    expect(plan.skipped).toEqual([{ key: 'web-id', label: 'web', reason: '已在运行' }])
    expect(dockerBatchNotes(plan)).toContain('已暂停的容器会继续运行，其余容器正常启动。')
  })

  it('explains why stopped containers cannot be stopped, paused or restarted', () => {
    const stopped = [container('a', 'exited')]
    expect(planContainerBatch(stopped, 'stop').skipped[0]!.reason).toBe('已停止')
    expect(planContainerBatch(stopped, 'pause').skipped[0]!.reason).toBe('未在运行')
    expect(planContainerBatch(stopped, 'restart').skipped[0]!.reason).toBe('未在运行')
    expect(planContainerBatch([container('b', 'paused')], 'pause').skipped[0]!.reason).toBe('已处于暂停状态')
    expect(planContainerBatch([container('c', 'running', { resourceVersion: undefined })], 'stop').skipped[0]!.reason)
      .toBe('缺少资源版本，请刷新后重试')
  })

  it('keeps KPanel itself selectable but moves it to the end with a warning', () => {
    const panel = container('kejilion-panel', 'running', { image: 'docker.io/kjlion/kejilion-panel@sha256:abc' })
    const plan = planContainerBatch([panel, container('web', 'running'), container('db', 'running')], 'stop')
    expect(plan.items.map((item) => item.label)).toEqual(['web', 'db', 'kejilion-panel'])
    expect(plan.items[2]!.self).toBe(true)
    expect(dockerBatchSelfWarning(plan)).toContain('所选包含 KPanel 自身容器 kejilion-panel，会放在最后执行')
    expect(dockerBatchSelfWarning(planContainerBatch([container('web', 'running')], 'stop'))).toBe('')
  })

  it('recognises the panel container by name or by image from any registry', () => {
    expect(isKPanelContainer({ name: 'kejilion-panel', image: 'custom:1' })).toBe(true)
    expect(isKPanelContainer({ name: 'kp', image: 'ghcr.io/kjlion/kejilion-panel:1.25.0' })).toBe(true)
    expect(isKPanelContainer({ name: 'kp', image: 'kjlion/kejilion-panel' })).toBe(true)
    expect(isKPanelContainer({ name: 'kp', image: 'kjlion/kejilion-panel-helper:1' })).toBe(false)
    expect(isKPanelContainer({ name: 'kp', image: 'nginx:alpine' })).toBe(false)
  })
})

describe('image references', () => {
  it('normalises Docker Hub shorthand, implicit latest and digests', () => {
    expect(normalizeImageReference('nginx')).toBe('nginx:latest')
    expect(normalizeImageReference('docker.io/library/nginx:alpine')).toBe('nginx:alpine')
    expect(normalizeImageReference('library/redis')).toBe('redis:latest')
    expect(normalizeImageReference('localhost:5000/team/app')).toBe('localhost:5000/team/app:latest')
    expect(normalizeImageReference('ghcr.io/a/b@sha256:ff')).toBe('ghcr.io/a/b@sha256:ff')
    expect(normalizeImageReference(`sha256:${'a'.repeat(64)}`)).toBe(`sha256:${'a'.repeat(64)}`)
  })
})

describe('image batch planning', () => {
  const images = [
    image('sha256:1', ['nginx:alpine', 'nginx:1.27-alpine']),
    image('sha256:2', ['redis:7']),
    image('sha256:3', []),
    image('sha256:4', ['old:1'], { resourceVersion: undefined }),
  ]
  const containers = [
    container('web', 'running', { image: 'docker.io/library/nginx:alpine' }),
    container('cache', 'exited', { image: 'redis:7' }),
  ]

  it('annotates images that containers reference without blocking the deletion', () => {
    const plan = planImageBatch(images, containers, 'image_remove')
    expect(plan.items.map((item) => [item.label, item.note])).toEqual([
      ['nginx:alpine', '运行中的容器 web 正在使用'],
      ['redis:7', '仍被已停止的容器 cache 引用'],
      ['未标记镜像 3', undefined],
    ])
    expect(plan.items[0]!.detail).toContain('nginx:1.27-alpine')
    expect(plan.items[0]!.operation).toEqual({ kind: 'task', input: { action: 'image_remove', target: 'sha256:1', expectedResourceVersion: 'v-sha256:1' } })
    expect(plan.skipped).toEqual([{ key: 'sha256:4', label: 'old:1', reason: '缺少资源版本，请刷新后重试' }])
  })

  it('pulls the first tag and skips untagged images', () => {
    const plan = planImageBatch(images, containers, 'image_pull')
    expect(plan.items.map((item) => item.operation)).toEqual([
      { kind: 'task', input: { action: 'image_pull', image: 'nginx:alpine' } },
      { kind: 'task', input: { action: 'image_pull', image: 'redis:7' } },
      { kind: 'task', input: { action: 'image_pull', image: 'old:1' } },
    ])
    expect(plan.skipped.map((item) => item.reason)).toEqual(['没有标签，无法拉取更新'])
  })
})

describe('network and volume batch planning', () => {
  const networks: DockerNetwork[] = ['bridge', 'web_default', 'legacy', 'spare'].map((name) => ({
    id: `${name}-net`, name, driver: 'bridge', resourceVersion: `v-${name}`,
  }))
  const containers = [
    container('web', 'running', { networks: ['web_default'], mounts: [{ type: 'volume', name: 'web-data', destination: '/data' }] }),
    container('old', 'exited', { networks: ['legacy'], mounts: [{ type: 'volume', name: 'old-data', destination: '/data' }] }),
  ]

  it('skips predefined networks and networks running containers use', () => {
    const plan = planNetworkBatch(networks, containers)
    expect(plan.skipped.map((item) => [item.label, item.reason])).toEqual([
      ['bridge', 'Docker 预置网络，不能删除'],
      ['web_default', '运行中的容器 web 仍连接此网络，Docker 不允许删除'],
    ])
    expect(plan.items.map((item) => [item.label, item.note])).toEqual([
      ['legacy', '已停止的容器 old 仍配置此网络'],
      ['spare', undefined],
    ])
    expect(dockerBatchNotes(plan)[0]).toContain('网络删除后')
  })

  it('skips volumes any container mounts, running or stopped', () => {
    const volumes: DockerVolume[] = ['web-data', 'old-data', 'free'].map((name) => ({ name, driver: 'local', resourceVersion: `v-${name}` }))
    const plan = planVolumeBatch(volumes, containers)
    expect(plan.skipped.map((item) => item.reason)).toEqual([
      '仍被容器 web 挂载，Docker 不允许删除',
      '仍被容器 old 挂载，Docker 不允许删除',
    ])
    expect(plan.items.map((item) => item.operation)).toEqual([
      { kind: 'task', input: { action: 'volume_remove', target: 'free', expectedResourceVersion: 'v-free' } },
    ])
  })

  it('shortens long container lists without language-specific words', () => {
    const many = ['a', 'b', 'c', 'd', 'e'].map((name) => container(name, 'running', { mounts: [{ type: 'volume', name: 'shared', destination: '/x' }] }))
    const plan = planVolumeBatch([{ name: 'shared', driver: 'local', resourceVersion: 'v' }], many)
    expect(plan.skipped[0]!.reason).toBe('仍被容器 a, b, c +2 挂载，Docker 不允许删除')
  })
})

describe('batch execution', () => {
  const plan = planContainerBatch([container('a', 'running'), container('b', 'running'), container('c', 'running')], 'stop')

  it('runs items in order and keeps going after a failure', async () => {
    const run = createDockerBatchRun(plan)
    const seen: string[] = []
    await executeDockerBatch(run, async (item) => {
      seen.push(item.label)
      if (item.label === 'b') throw new Error('boom')
    }, (reason) => (reason as Error).message, new AbortController().signal)
    expect(seen).toEqual(['a', 'b', 'c'])
    expect(run.items.map((item) => [item.status, item.message])).toEqual([['succeeded', ''], ['failed', 'boom'], ['succeeded', '']])
    expect(run.phase).toBe('finished')
    expect(summarizeDockerBatch(run)).toEqual({ total: 3, done: 3, succeeded: 2, failed: 1, cancelled: 0, unknown: 0 })
  })

  it('stops dispatching after a stop request but lets the current item finish', async () => {
    const run = createDockerBatchRun(plan)
    await executeDockerBatch(run, async (item) => {
      if (item.label === 'a') run.phase = 'stopping'
    }, String, new AbortController().signal)
    expect(run.items.map((item) => item.status)).toEqual(['succeeded', 'cancelled', 'cancelled'])
    expect(run.phase).toBe('finished')
  })

  it('marks unconfirmed and aborted results instead of calling them failures', async () => {
    const run = createDockerBatchRun(plan)
    const controller = new AbortController()
    await executeDockerBatch(run, async (item) => {
      if (item.label === 'a') throw new DockerBatchUnconfirmedError('connection lost')
      controller.abort()
      throw new DOMException('Aborted', 'AbortError')
    }, String, controller.signal)
    expect(run.items.map((item) => [item.status, item.message])).toEqual([
      ['unknown', 'connection lost'], ['unknown', '已离开页面，结果未确认'], ['cancelled', ''],
    ])
  })
})

describe('background task items', () => {
  const timing = { pollDelay: 0, conflictDelay: 0, conflictAttempts: 3, pollFailures: 2 }
  const job = (status: DockerMaintenanceJob['status'], message = ''): DockerMaintenanceJob =>
    ({ id: 'f'.repeat(32), action: 'volume_remove', status, stage: status, progress: 0, message, createdAt: '' })

  it('waits for the job to finish and surfaces its failure message', async () => {
    const jobs = [job('running'), job('failed', 'volume is in use')]
    const client = { submit: vi.fn().mockResolvedValue(job('queued')), job: vi.fn(async () => jobs.shift()!) }
    await expect(runDockerBatchTask({ action: 'volume_remove', target: 'v' }, client, new AbortController().signal, timing))
      .rejects.toThrow('volume is in use')
    expect(client.job).toHaveBeenCalledTimes(2)
  })

  it('waits out a job started elsewhere before submitting', async () => {
    const conflict = Object.assign(new Error('busy'), { status: 409, code: 'docker_task_conflict' })
    const client = {
      submit: vi.fn().mockRejectedValueOnce(conflict).mockResolvedValueOnce(job('succeeded')),
      job: vi.fn(),
    }
    await runDockerBatchTask({ action: 'volume_remove', target: 'v' }, client, new AbortController().signal, timing)
    expect(client.submit).toHaveBeenCalledTimes(2)
    expect(client.job).not.toHaveBeenCalled()
  })

  it('does not retry other submission errors', async () => {
    const stale = Object.assign(new Error('changed'), { status: 409, code: 'resource_conflict' })
    const client = { submit: vi.fn().mockRejectedValue(stale), job: vi.fn() }
    await expect(runDockerBatchTask({ action: 'volume_remove', target: 'v' }, client, new AbortController().signal, timing))
      .rejects.toBe(stale)
    expect(client.submit).toHaveBeenCalledTimes(1)
  })

  it('reports an unconfirmed result when the job can no longer be read', async () => {
    const client = { submit: vi.fn().mockResolvedValue(job('queued')), job: vi.fn().mockRejectedValue(new Error('offline')) }
    await expect(runDockerBatchTask({ action: 'volume_remove', target: 'v' }, client, new AbortController().signal, timing))
      .rejects.toBeInstanceOf(DockerBatchUnconfirmedError)
    expect(client.job).toHaveBeenCalledTimes(2)
  })
})

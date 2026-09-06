import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { dockerUpdateTTL, useDockerImageUpdates, type DockerImageUpdateResult } from './dockerImageUpdate'
import type { DockerContainer } from '@/types/api'

const item = (id = 'a'): DockerContainer => ({ id: id.repeat(64), name: id, image: 'redis:7', resourceVersion: 'v1', state: 'running', access: 'managed', consistency: 'synced', ports: [], networks: [], mounts: [] })
const response = (container = item()): DockerImageUpdateResult => ({ containerId: container.id, image: container.image, resourceVersion: container.resourceVersion!, status: 'current', updateAvailable: false, checkedAt: new Date().toISOString() })
afterEach(() => vi.useRealTimers())

describe('Docker image checks', () => {
  it('only checks on demand and expires successful results', async () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const request = vi.fn().mockResolvedValue(response())
    const checks = scope.run(() => useDockerImageUpdates(ref([item()]), ref(true), request))!
    expect(request).not.toHaveBeenCalled()
    await checks.check(item())
    expect(checks.entries.value[item().id]?.status).toBe('current')
    await vi.advanceTimersByTimeAsync(dockerUpdateTTL)
    expect(checks.entries.value[item().id]?.status).toBe('expired')
    expect(request).toHaveBeenCalledTimes(1)
    scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('bounds concurrent checks and discards results after resource replacement or deactivation', async () => {
    const scope = effectScope(), containers = ref([item(), item('b'), item('c')]), active = ref(true)
    const resolves: ((result: DockerImageUpdateResult) => void)[] = []
    const request = vi.fn((_id, _version, _signal) => new Promise<DockerImageUpdateResult>(resolve => resolves.push(resolve)))
    const checks = scope.run(() => useDockerImageUpdates(containers, active, request))!
    const first = checks.check(item()), second = checks.check(item('b'))
    await checks.check(item('c'))
    expect(request).toHaveBeenCalledTimes(2)
    containers.value = [{ ...item(), resourceVersion: 'v2' }, item('b')]
    expect(request.mock.calls[0]![2].aborted).toBe(true)
    active.value = false
    expect(request.mock.calls[1]![2].aborted).toBe(true)
    resolves[0]!(response()); resolves[1]!(response(item('b')))
    await Promise.all([first, second])
    expect(checks.entries.value).toEqual({})
    expect(checks.busy.value).toBe(false)
    scope.stop()
  })

  it.each(['reject', 'wrong identity', 'unknown status'])('shows unavailable for %s and can retry', async kind => {
    const scope = effectScope(), request = vi.fn()
    if (kind === 'reject') request.mockRejectedValueOnce(new Error('registry denied'))
    else request.mockResolvedValueOnce({ ...response(), ...(kind === 'wrong identity' ? { containerId: 'other' } : { status: 'unexpected' }) })
    request.mockResolvedValue(response())
    const checks = scope.run(() => useDockerImageUpdates(ref([item()]), ref(true), request))!
    await checks.check(item())
    expect(checks.entries.value[item().id]?.status).toBe('unavailable')
    await checks.check(item())
    expect(checks.entries.value[item().id]?.status).toBe('current')
    scope.stop()
  })
})

import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { dockerUpdateTTL, useDockerImageUpdates, type DockerImageUpdateResult } from './dockerImageUpdate'
import type { DockerContainer } from '@/types/api'

const item = (id = 'a'): DockerContainer => ({ id: id.repeat(64), name: id, image: 'redis:7', resourceVersion: 'v1', state: 'running', access: 'managed', consistency: 'synced', ports: [], networks: [], mounts: [] })
const response = (container = item()): DockerImageUpdateResult => ({ containerId: container.id, image: container.image, resourceVersion: container.resourceVersion!, status: 'current', updateAvailable: false, checkedAt: new Date().toISOString() })
afterEach(() => vi.useRealTimers())

describe('Docker image checks', () => {
  it('automatically checks, reuses results across refresh and inactivity, and rechecks after expiry', async () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const request = vi.fn().mockResolvedValue(response())
    const containers = ref([item()]), active = ref(true)
    const checks = scope.run(() => useDockerImageUpdates(containers, active, request))!
    expect(request).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1_000)
    expect(checks.entries.value[item().id]?.status).toBe('current')
    containers.value = [{ ...item() }]
    active.value = false
    await vi.advanceTimersByTimeAsync(dockerUpdateTTL)
    expect(request).toHaveBeenCalledTimes(1)
    active.value = true
    expect(checks.entries.value[item().id]?.status).toBe('expired')
    await vi.advanceTimersByTimeAsync(1_000)
    expect(request).toHaveBeenCalledTimes(2)
    scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('automatically drains a bounded scan without retrying failed images in a tight loop', async () => {
    vi.useFakeTimers()
    const scope = effectScope(), containers = ref([item(), item('b'), item('c')])
    const request = vi.fn(async (id: string) => {
      if (id === item('b').id) throw { code: 'docker_update_digest_missing', message: 'sensitive registry error' }
      return response(containers.value.find(container => container.id === id)!)
    })
    const checks = scope.run(() => useDockerImageUpdates(containers, ref(true), request))!
    await vi.advanceTimersByTimeAsync(60_000)
    expect(request).toHaveBeenCalledTimes(3)
    expect(checks.entries.value[item('b').id]).toMatchObject({ status: 'unavailable', reason: 'docker_update_digest_missing' })
    expect(JSON.stringify(checks.entries.value)).not.toContain('sensitive')
    scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('caps automatic results at 200 without an eviction scan loop', async () => {
    vi.useFakeTimers()
    const scope = effectScope(), containers = ref(Array.from({ length: 205 }, (_, i) => item(String(i))))
    const request = vi.fn(async (id: string) => response(containers.value.find(container => container.id === id)!))
    const checks = scope.run(() => useDockerImageUpdates(containers, ref(true), request))!
    await vi.advanceTimersByTimeAsync(300_000)
    expect(request).toHaveBeenCalledTimes(200)
    expect(Object.keys(checks.entries.value)).toHaveLength(200)
    await checks.check(containers.value[204]!)
    await vi.advanceTimersByTimeAsync(5_000)
    expect(request).toHaveBeenCalledTimes(201)
    expect(Object.keys(checks.entries.value)).toHaveLength(200)
    scope.stop()
  })

  it('resumes pending automatic checks with at most two in flight and aborts on dispose', async () => {
    vi.useFakeTimers()
    const scope = effectScope(), containers = ref([item(), item('b'), item('c')])
    const resolves: ((result: DockerImageUpdateResult) => void)[] = []
    const request = vi.fn((_id, _version, _signal) => new Promise<DockerImageUpdateResult>(resolve => resolves.push(resolve)))
    const checks = scope.run(() => useDockerImageUpdates(containers, ref(true), request))!
    await vi.advanceTimersByTimeAsync(3_000)
    expect(request).toHaveBeenCalledTimes(2)
    resolves[0]!(response())
    await vi.advanceTimersByTimeAsync(1_000)
    expect(request).toHaveBeenCalledTimes(3)
    scope.stop()
    expect(request.mock.calls[1]![2].aborted).toBe(true)
    expect(request.mock.calls[2]![2].aborted).toBe(true)
    resolves[1]!(response(item('b'))); resolves[2]!(response(item('c')))
    await vi.advanceTimersByTimeAsync(1_000)
    expect(checks.entries.value).toEqual({})
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

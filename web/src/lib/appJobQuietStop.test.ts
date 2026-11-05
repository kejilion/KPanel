// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { requestQuietAppJobStop, retryPendingAppJobStops } from './appJobQuietStop'

const mocks = vi.hoisted(() => ({ cancel: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: { apps: { cancelJob: mocks.cancel } },
  ApiError: class extends Error { code = '' },
}))

describe('quiet application job stop', () => {
  beforeEach(() => { vi.useFakeTimers(); mocks.cancel.mockReset(); window.localStorage.clear() })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })
  it('retries a failed cancellation without replaying the job', async () => {
    mocks.cancel.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ status: 'running', stage: 'cancelling' })
    requestQuietAppJobStop('job-1', 'owner', () => 'owner')
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.cancel).toHaveBeenCalledTimes(2)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:owner:job-1')).toBeNull()
  })
  it('retains failures for the same owner and does not retry after account change', async () => {
    let owner = 'owner'
    mocks.cancel.mockRejectedValue(new Error('offline'))
    requestQuietAppJobStop('job-2', owner, () => owner)
    await vi.advanceTimersByTimeAsync(0)
    owner = 'other'
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.cancel).toHaveBeenCalledTimes(1)
    retryPendingAppJobStops('other', () => owner)
    expect(mocks.cancel).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:owner:job-2')).toBe('1')
  })
  it('bounds automatic retries and resumes them when the same owner returns', async () => {
    mocks.cancel.mockRejectedValue(new Error('offline'))
    requestQuietAppJobStop('job-3', 'owner', () => 'owner')
    await vi.advanceTimersByTimeAsync(4000)
    expect(mocks.cancel).toHaveBeenCalledTimes(3)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:owner:job-3')).toBe('1')
    mocks.cancel.mockResolvedValueOnce({ status: 'cancelled' })
    retryPendingAppJobStops('owner', () => 'owner')
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.cancel).toHaveBeenCalledTimes(4)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:owner:job-3')).toBeNull()
  })
  it('does not evict an older pending stop when more than 32 identities are recorded', async () => {
    mocks.cancel.mockRejectedValue(new Error('offline'))
    for (let index = 0; index < 40; index++) requestQuietAppJobStop(`bulk-${index}`, 'bulk-owner', () => 'bulk-owner')
    await vi.advanceTimersByTimeAsync(0)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:bulk-owner:bulk-0')).toBe('1')
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:bulk-owner:bulk-39')).toBe('1')
  })
  it('continues to retry records saved in the earlier array format', async () => {
    window.localStorage.setItem('kpanel:pending-app-job-stops:legacy-owner', JSON.stringify(['legacy-job']))
    mocks.cancel.mockResolvedValue({ status: 'cancelled' })
    retryPendingAppJobStops('legacy-owner', () => 'legacy-owner')
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.cancel).toHaveBeenCalledWith('legacy-job', expect.any(AbortSignal))
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-app-job-stops:legacy-owner')!)).toEqual([])
  })
  it('continues to look for a submitted job that appears after the first retries', async () => {
    const missing = Object.assign(new ApiError('missing', 404, 'app_job_not_found'), { code: 'app_job_not_found' })
    mocks.cancel.mockRejectedValueOnce(missing).mockRejectedValueOnce(missing).mockRejectedValueOnce(missing).mockResolvedValueOnce({ status: 'cancelled' })
    requestQuietAppJobStop('late-job', 'late-owner', () => 'late-owner')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.cancel).toHaveBeenCalledTimes(4)
    expect(window.localStorage.getItem('kpanel:pending-app-job-stops:late-owner:late-job')).toBeNull()
  })
})

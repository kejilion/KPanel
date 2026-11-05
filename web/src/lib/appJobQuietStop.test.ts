// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-app-job-stops:owner')!)).toEqual([])
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
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-app-job-stops:owner')!)).toEqual(['job-2'])
  })
  it('bounds automatic retries and resumes them when the same owner returns', async () => {
    mocks.cancel.mockRejectedValue(new Error('offline'))
    requestQuietAppJobStop('job-3', 'owner', () => 'owner')
    await vi.advanceTimersByTimeAsync(4000)
    expect(mocks.cancel).toHaveBeenCalledTimes(3)
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-app-job-stops:owner')!)).toEqual(['job-3'])
    mocks.cancel.mockResolvedValueOnce({ status: 'cancelled' })
    retryPendingAppJobStops('owner', () => 'owner')
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.cancel).toHaveBeenCalledTimes(4)
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-app-job-stops:owner')!)).toEqual([])
  })
})

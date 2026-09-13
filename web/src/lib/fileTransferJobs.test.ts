// @vitest-environment jsdom
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { FileTransferJob, FileTransferJobInput } from '@/types/api'

const mocks = vi.hoisted(() => ({ transferJobs: vi.fn(), createTransferJob: vi.fn(), changeTransferJob: vi.fn(), notify: vi.fn() }))
vi.mock('./api', () => ({ api: { files: mocks }, ApiError: class extends Error { status = 503 } }))
vi.mock('./fileWindowTransfer', () => ({ notifyFileDirectoriesChanged: mocks.notify }))
let lib: typeof import('./fileTransferJobs')
const input: FileTransferJobInput = { id:'a'.repeat(32),sourceNodeId:'b'.repeat(32),targetHostId:'c'.repeat(32),targetDirectory:'/target',items:[{path:'/one',resourceVersion:'v1'}] }
const job = (state = 'queued'): FileTransferJob => ({...input,state,items:[{...input.items[0]!,state,loadedBytes:0,totalBytes:0,retryable:false}],createdAt:'2026-09-13T00:00:00Z',updatedAt:'2026-09-13T00:00:00Z'})
const stops: (() => void)[] = []
beforeEach(async () => {
  vi.resetModules(); vi.resetAllMocks(); vi.useFakeTimers()
  vi.spyOn(document,'hidden','get').mockReturnValue(false)
  mocks.transferJobs.mockResolvedValue({items:[]})
  lib=await import('./fileTransferJobs')
})
afterEach(() => {for(const stop of stops.splice(0)) stop();vi.useRealTimers();vi.restoreAllMocks()})

it('shares polling across windows, refreshes only the actual target and stops when the last window closes',async()=>{
  mocks.transferJobs.mockResolvedValue({items:[job()]})
  const a=lib.observeTransferJobs();const b=lib.observeTransferJobs();stops.push(b)
  await flushPromises();expect(mocks.transferJobs).toHaveBeenCalledTimes(1)
  a();await vi.advanceTimersByTimeAsync(2000)
  expect(mocks.transferJobs).toHaveBeenCalledTimes(2)
  mocks.transferJobs.mockResolvedValue({items:[job('complete')]})
  await vi.advanceTimersByTimeAsync(2000)
  expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(['/target'],undefined,[],input.targetHostId)
  b();stops.length=0;await vi.advanceTimersByTimeAsync(60000)
  expect(mocks.transferJobs).toHaveBeenCalledTimes(3)
  expect(lib.transferJobs.value[0]?.state).toBe('complete')
})

it('reconciles a lost creation receipt without submitting a second copy',async()=>{
  mocks.createTransferJob.mockRejectedValue(new Error('connection lost'))
  mocks.transferJobs.mockResolvedValue({items:[job()]})
  expect(await lib.createTransferJob(input)).toMatchObject({id:input.id,state:'queued'})
  expect(mocks.createTransferJob).toHaveBeenCalledTimes(1)
})

it('keeps results and stops repeated failed discovery until an explicit refresh recovers',async()=>{
  lib.transferJobs.value=[job('complete')]
  mocks.transferJobs.mockRejectedValue(new Error('offline'))
  stops.push(lib.observeTransferJobs());await flushPromises()
  await vi.advanceTimersByTimeAsync(180000)
  expect(mocks.transferJobs).toHaveBeenCalledTimes(4)
  expect(lib.transferJobs.value[0]?.state).toBe('complete')
  expect(lib.transferJobsError.value).toBe('offline')
  mocks.transferJobs.mockResolvedValue({items:[job('complete')]})
  await lib.refreshTransferJobs()
  expect(lib.transferJobsError.value).toBe('')
})

it('cancels through the task endpoint and accepts a refreshed terminal state',async()=>{
  mocks.changeTransferJob.mockResolvedValue(undefined)
  mocks.transferJobs.mockResolvedValue({items:[job('cancelled')]})
  await lib.changeTransferJob(input.id,'cancel')
  expect(mocks.changeTransferJob).toHaveBeenCalledWith(input.id,'cancel')
  expect(lib.transferJobs.value[0]?.state).toBe('cancelled')
})

it('discards an older list response after clearing a record',async()=>{
  lib.transferJobs.value=[job('complete')]
  let finish!:(value:unknown)=>void
  mocks.transferJobs.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve})).mockResolvedValue({items:[]})
  const old=lib.refreshTransferJobs();await flushPromises()
  const signal=mocks.transferJobs.mock.calls[0]![0] as AbortSignal
  mocks.changeTransferJob.mockResolvedValue(undefined)
  const clearing=lib.changeTransferJob(input.id,'clear');await flushPromises()
  expect(signal.aborted).toBe(true)
  finish({items:[job('complete')]});await old;await clearing
  expect(lib.transferJobs.value).toEqual([])
  expect(mocks.transferJobs).toHaveBeenCalledTimes(2)
})

it('refreshes completed jobs first discovered after reopening, once per host and directory',async()=>{
  mocks.transferJobs.mockResolvedValue({items:[job('complete'),{...job('complete'),id:'f'.repeat(32)}]})
  await lib.refreshTransferJobs()
  expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(['/target'],undefined,[],input.targetHostId)
  await lib.refreshTransferJobs()
  expect(mocks.notify).toHaveBeenCalledTimes(1)
})

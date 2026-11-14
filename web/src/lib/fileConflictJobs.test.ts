// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { FileConflictBatch, cancelFileConflicts, pendingFileConflict } from './fileConflicts'
import { decideConflictJob, runConflictJob } from './fileConflictJobs'
import type { FileActionInput, FileArchiveJob, FileEntry } from '@/types/api'

const entry = (path: string): FileEntry => ({ name:path.split('/').at(-1)!,path,kind:'file',resourceVersion:'v1',sizeBytes:3,mode:'0644',owner:'root',group:'root',modifiedAt:'2026-11-14T00:00:00Z',editable:true,previewable:true })
const waiting = (): FileArchiveJob => ({id:'a'.repeat(32),action:'copy',name:'a.txt',target:'/d',sources:['/s/a.txt'],state:'waiting_input',revision:3,conflict:{source:entry('/s/a.txt'),target:entry('/d/a.txt')},entries:0,processedBytes:0,createdAt:'2026-11-14T00:00:00Z',updatedAt:'2026-11-14T00:00:00Z',result:{action:'copy',succeeded:[],failed:[]}})
afterEach(() => {cancelFileConflicts();vi.useRealTimers()})

it('keeps a task-scoped decision bound to the durable task revision', async () => {
  vi.useFakeTimers()
  let job = waiting()
  const api = {createArchiveJob:vi.fn(async(_input: FileActionInput)=>job),archiveJobs:vi.fn(async()=>({items:[job]})),changeArchiveJob:vi.fn(),decideArchiveJob:vi.fn(async()=>{job={...job,state:'complete',conflict:undefined,result:{action:'copy',succeeded:[{path:'/s/a.txt',destination:'/d/a.txt'}],failed:[]}}})}
  const batch = new FileConflictBatch('复制','h')
  const task = runConflictJob(api,{action:'copy',sources:['/s/a.txt'],target:'/d'},batch,'h')
  await flushPromises();pendingFileConflict.value!.decide('overwrite',true)
  await vi.advanceTimersByTimeAsync(300)
  const result=await task
  expect(api.decideArchiveJob).toHaveBeenCalledWith(job.id,{policy:'overwrite',applyToRest:true,revision:3})
  expect(result.succeeded[0]?.destination).toBe('/d/a.txt')
  expect(api.createArchiveJob.mock.calls[0]?.[0]).toMatchObject({conflictPolicies:{file:'ask',directory:'ask'}})
  batch.close()
})

it('presents only one prompt when two windows observe the same conflict', async () => {
  const job=waiting(),api={createArchiveJob:vi.fn(),archiveJobs:vi.fn(),changeArchiveJob:vi.fn(),decideArchiveJob:vi.fn()}
  const a=new FileConflictBatch('复制','h'),b=new FileConflictBatch('复制','h')
  const first=decideConflictJob(api,job,a,'h'),second=decideConflictJob(api,job,b,'h')
  await flushPromises();pendingFileConflict.value!.decide('skip',false)
  await Promise.all([first,second]);expect(api.decideArchiveJob).toHaveBeenCalledTimes(1)
  expect(pendingFileConflict.value).toBeUndefined();a.close();b.close()
})

it('detaches a closed window without cancelling the persisted task', async () => {
  const job=waiting(),api={createArchiveJob:vi.fn(),archiveJobs:vi.fn(),changeArchiveJob:vi.fn(),decideArchiveJob:vi.fn()}
  const batch=new FileConflictBatch('复制','h')
  const task=decideConflictJob(api,job,batch,'h').catch(error=>error.name)
  await flushPromises();batch.close()
  expect(await task).toBe('AbortError');expect(api.changeArchiveJob).not.toHaveBeenCalled()
})

it('cancels only the selected task when its conflict dialog is dismissed', async () => {
  const job=waiting(),api={createArchiveJob:vi.fn(),archiveJobs:vi.fn(),changeArchiveJob:vi.fn(),decideArchiveJob:vi.fn()}
  const batch=new FileConflictBatch('复制','h')
  const task=decideConflictJob(api,job,batch,'h').catch(error=>error.name)
  await flushPromises();pendingFileConflict.value!.cancel()
  expect(await task).toBe('AbortError');expect(api.changeArchiveJob).toHaveBeenCalledWith(job.id,'cancel')
})

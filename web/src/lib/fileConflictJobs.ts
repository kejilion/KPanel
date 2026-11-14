import type { FileActionInput, FileActionResult, FileArchiveJob, FileConflict, FileConflictDecision, FileRemoteDownloadJob } from '@/types/api'
import { FileConflictBatch, isFileBatchCancelled } from './fileConflicts'

export interface ConflictJobAPI {
  createArchiveJob: (input: FileActionInput) => Promise<FileArchiveJob>
  archiveJobs: (signal?: AbortSignal) => Promise<{ items: FileArchiveJob[] }>
  decideArchiveJob: (id: string, decision: FileConflictDecision) => Promise<unknown>
  changeArchiveJob: (id: string, operation: 'cancel' | 'clear') => Promise<unknown>
}

const decisions = new Map<string, Promise<void>>()
export const activeConflictJob = (job: FileArchiveJob) => ['queued', 'running', 'cancelling', 'waiting_input'].includes(job.state)

// One tab can contain several windows observing the same durable task. Share
// only its pending presentation, never another task's remembered strategy.
export async function decideConflictJob(api: ConflictJobAPI, job: FileArchiveJob, batch: FileConflictBatch, host: string): Promise<void> {
  if (!job.conflict || job.state !== 'waiting_input') return
  const key = `${host}:${job.id}:${job.revision}`
  return presentDecision(key,job.conflict,job.revision!,batch,decision=>api.decideArchiveJob(job.id,decision),()=>api.changeArchiveJob(job.id,'cancel'))
}

export async function decideRemoteConflict(api: {decideRemoteDownloadJob:(id:string,decision:FileConflictDecision)=>Promise<unknown>;cancelRemoteDownloadJob:(id:string)=>Promise<unknown>},job:FileRemoteDownloadJob,batch:FileConflictBatch):Promise<void>{
  if (!job.conflict || job.state !== 'waiting_input') return
  return presentDecision(`remote:${job.id}:${job.revision}`,job.conflict,job.revision!,batch,decision=>api.decideRemoteDownloadJob(job.id,decision),()=>api.cancelRemoteDownloadJob(job.id))
}

async function presentDecision(key:string,conflict:FileConflict,revision:number,batch:FileConflictBatch,decide:(decision:FileConflictDecision)=>Promise<unknown>,cancel:()=>Promise<unknown>):Promise<void>{
  const existing = decisions.get(key)
  if (existing) return existing
  const pending = (async () => {
    try {
      const choice = await batch.prompt(conflict.source,conflict.target,conflict.changed)
      await decide({ ...choice, revision })
    } catch (error) {
      if (isFileBatchCancelled(error) && batch.userCancelled) await cancel()
      throw error
    }
  })()
  decisions.set(key, pending)
  try { await pending } finally { if (decisions.get(key) === pending) decisions.delete(key) }
}

function pause(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted()
    const finish = () => { signal.removeEventListener('abort', abort); resolve() }
    const timer = setTimeout(finish, 250)
    const abort = () => { clearTimeout(timer); reject(signal.reason) }
    signal.addEventListener('abort', abort, { once: true })
  })
}

export async function runConflictJob(api: ConflictJobAPI, input: FileActionInput, batch: FileConflictBatch, host: string): Promise<FileActionResult & { skipped: string[]; cancelled: boolean }> {
  let job = await api.createArchiveJob({ ...input, conflictPolicies: { file: 'ask', directory: 'ask' } })
  try {
    while (activeConflictJob(job)) {
      batch.signal.throwIfAborted()
      if (job.state === 'waiting_input') await decideConflictJob(api, job, batch, host)
      await pause(batch.signal)
      const next = (await api.archiveJobs(batch.signal)).items.find(item => item.id === job.id)
      if (!next) throw new Error('文件任务状态已不可用，请刷新后核对目标目录。')
      job = next
    }
  } catch (error) {
    if (!isFileBatchCancelled(error) && !batch.signal.aborted) throw error
    if (!batch.userCancelled) return { ...job.result, skipped: job.result.skipped || [], cancelled: true }
    if (job.state!=='waiting_input') await api.changeArchiveJob(job.id,'cancel')
    // A cancellation request is not a terminal receipt. Read back a racing
    // commit before changing a cut clipboard or reporting the final outcome.
    for (let attempt=0;attempt<20;attempt++) {
      const current=(await api.archiveJobs()).items.find(item=>item.id===job.id)
      if (!current) throw new Error('停止结果尚未确认，请在文件任务中核对。')
      job=current
      if (!activeConflictJob(job)) return {...job.result,skipped:job.result.skipped||[],cancelled:job.state==='cancelled'||job.state==='interrupted'}
      await new Promise(resolve=>setTimeout(resolve,250))
    }
    throw new Error('停止请求已发送，请在文件任务中确认最终结果。')
  }
  return { ...job.result, skipped: job.result.skipped || [], cancelled: job.state === 'cancelled' || job.state === 'interrupted' }
}

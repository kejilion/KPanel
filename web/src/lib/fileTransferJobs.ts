import { shallowRef } from 'vue'
import { api, ApiError } from './api'
import { notifyFileDirectoriesChanged } from './fileWindowTransfer'
import type { FileTransferJob, FileTransferJobInput } from '@/types/api'

export const transferJobs = shallowRef<FileTransferJob[]>([])
export const transferJobsError = shallowRef('')
export const transferJobsLoading = shallowRef(false)
export const transferJobActive = (state: string) => ['queued', 'running', 'connecting', 'transferring', 'committing'].includes(state)
let observers = 0
let timer: ReturnType<typeof setTimeout> | undefined
let request: Promise<void> | undefined
let controller: AbortController | undefined
let failures = 0

export function newTransferJobID(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('')
}

function schedule(): void {
  if (timer) clearTimeout(timer)
  if (!observers || document.hidden || failures >= 4) return
  timer = setTimeout(() => { void refreshTransferJobs() }, failures ? Math.min(30000, 3000 * 2 ** failures) : transferJobs.value.some(j => transferJobActive(j.state)) ? 2000 : 15000)
}

export function refreshTransferJobs(): Promise<void> {
  if (request) return request
  const current = new AbortController()
  controller = current
  transferJobsLoading.value = true
  request = (async () => {
    try {
      const response = await api.files.transferJobs(current.signal)
      if (current.signal.aborted) return
      if (!Array.isArray(response.items) || response.items.length > 32) throw new Error('无法读取传输任务，请刷新重试。')
      const previous = new Map(transferJobs.value.map(job => [job.id, job]))
      transferJobs.value = response.items
      transferJobsError.value = ''
      failures = 0
      const changed = new Map<string, Set<string>>()
      for (const job of response.items) {
        const old = previous.get(job.id)
        if (job.items.some((item, i) => item.state === 'complete' && old?.items[i]?.state !== 'complete')) {
          const paths = changed.get(job.targetHostId) || new Set<string>()
          paths.add(job.targetDirectory)
          changed.set(job.targetHostId, paths)
        }
      }
      for (const [hostId, paths] of changed) notifyFileDirectoriesChanged([...paths], undefined, [], hostId)
    } catch (error) {
      if (current.signal.aborted) return
      failures++
      if (error instanceof ApiError && [401, 403].includes(error.status)) transferJobs.value = []
      transferJobsError.value = error instanceof Error ? error.message : '无法读取传输任务，请刷新重试。'
    } finally {
      if (controller === current) controller = undefined
      transferJobsLoading.value = false
      request = undefined
      schedule()
    }
  })()
  return request
}

const resume = () => { if (!document.hidden) { failures = 0; void refreshTransferJobs() } }

export function observeTransferJobs(): () => void {
  if (++observers === 1) {
    document.addEventListener('visibilitychange', resume)
    window.addEventListener('focus', resume)
    failures = 0
    void refreshTransferJobs()
  }
  return () => {
    if (--observers > 0) return
    if (timer) clearTimeout(timer)
    timer = undefined
    controller?.abort()
    document.removeEventListener('visibilitychange', resume)
    window.removeEventListener('focus', resume)
  }
}

export async function createTransferJob(input: FileTransferJobInput): Promise<FileTransferJob> {
  try {
    const job = await api.files.createTransferJob(input)
    controller?.abort()
    transferJobs.value = [job, ...transferJobs.value.filter(item => item.id !== job.id)].slice(0, 32)
    transferJobsError.value = ''
    failures = 0
    schedule()
    return job
  } catch (error) {
    // A lost POST response is not proof that creation failed. Reconcile the
    // exact client-generated ID; never blindly submit a second copy.
    await refreshAfterMutation()
    const accepted = transferJobs.value.find(job => job.id === input.id || input.retryOf && job.retryOf === input.retryOf)
    if (accepted) return accepted
    if (error instanceof ApiError && error.status >= 400) throw error
    throw new Error('提交结果暂时无法确认，请刷新任务记录。')
  }
}

export async function changeTransferJob(id: string, operation: 'cancel' | 'clear'): Promise<void> {
  await api.files.changeTransferJob(id, operation)
  await refreshAfterMutation()
}

async function refreshAfterMutation(): Promise<void> {
  const stale = request
  controller?.abort()
  if (stale) await stale
  await refreshTransferJobs()
}

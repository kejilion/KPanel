import type { CrossPanelFileTransferEvent, FileEntry, FileRemoteDownloadJob } from '@/types/api'

interface JobWatcher {
  get: (id: string) => Promise<FileRemoteDownloadJob>
  cancel: (id: string) => Promise<unknown>
}

async function wait(signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) return
  await new Promise<void>((resolve) => {
    const done = (): void => { clearTimeout(timer); signal?.removeEventListener('abort', done); resolve() }
    const timer = setTimeout(done, 500)
    signal?.addEventListener('abort', done, { once: true })
  })
}

// Closing a view detaches its watcher; explicit cancellation cancels the job.
// The job ID and target binding survive every observation of its state.
export async function watchFileTransferJob(initial: FileRemoteDownloadJob, watcher: JobWatcher,
  onEvent: (event: CrossPanelFileTransferEvent) => void, signal?: AbortSignal): Promise<FileEntry> {
  const validate = (value: FileRemoteDownloadJob): void => {
    if (value.id !== initial.id || value.sourceKind !== 'cross-host' || value.source !== initial.source
      || value.targetDirectory !== initial.targetDirectory || value.targetHostId !== initial.targetHostId
      || value.entry && (value.entry.path !== `${initial.targetDirectory === '/' ? '' : initial.targetDirectory}/${value.entry.name}`
        || !['file', 'directory'].includes(value.entry.kind))) throw new Error('面板返回了无效的传输任务。')
  }
  let job = initial
  let failures = 0
  for (;;) {
    validate(job)
    if (job.state === 'complete' && job.entry) {
      onEvent({ state: 'complete', loadedBytes: job.loadedBytes, totalBytes: job.totalBytes, entry: job.entry })
      return job.entry
    }
    if (signal?.aborted) {
      if (signal.reason !== 'file-transfer-detach') {
        try {
          await watcher.cancel(job.id)
          const finished = await watcher.get(job.id)
          validate(finished)
          if (finished.state === 'complete' && finished.entry) {
            onEvent({ state: 'complete', loadedBytes: finished.loadedBytes, totalBytes: finished.totalBytes, entry: finished.entry })
            return finished.entry
          }
        } catch { /* The task list remains available to confirm a racing commit. */ }
      }
      throw new DOMException('文件传输已取消。', 'AbortError')
    }
    if (['cancelled', 'interrupted', 'error'].includes(job.state)) {
      onEvent({ state: 'error', code: job.code, detail: '文件传输未完成，请查看任务列表或重试。' })
      throw new Error('文件传输未完成，请查看任务列表或重试。')
    }
    onEvent({ state: job.state === 'confirming' ? 'committing' : job.state === 'transferring' ? 'transferring' : 'connecting',
      loadedBytes: job.loadedBytes, totalBytes: job.totalBytes })
    await wait(signal)
    if (signal?.aborted) continue
    try { job = await watcher.get(initial.id); failures = 0 } catch (error) {
      if (++failures > 3) throw error
    }
  }
}

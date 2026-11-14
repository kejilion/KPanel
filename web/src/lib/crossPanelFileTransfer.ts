import { FileTransferSkippedError, FileTransferPartialError } from './fileTransferJob'
import type { CrossPanelFileDragItem, CrossPanelFileDragPayload } from './desktopFileShortcuts'
import type { CrossPanelFileTransferEvent, CrossPanelFileTransferInput, FileEntry } from '@/types/api'

export interface CrossPanelBatchProgress {
  index: number
  total: number
  completed: number
  source: CrossPanelFileDragItem
  event: CrossPanelFileTransferEvent
}

export interface CrossPanelBatchSuccess {
  source: CrossPanelFileDragItem
  entry: FileEntry
}

export interface CrossPanelBatchFailure {
  source: CrossPanelFileDragItem
  detail: string
}

export interface CrossPanelBatchResult {
  succeeded: CrossPanelBatchSuccess[]
  failed: CrossPanelBatchFailure[]
  skipped?: CrossPanelFileDragItem[]
  cancelled: boolean
}

type TransferOne = (
  input: CrossPanelFileTransferInput,
  onEvent: (event: CrossPanelFileTransferEvent) => void,
  signal?: AbortSignal,
) => Promise<FileEntry>

function isAbort(error: unknown, signal?: AbortSignal): boolean {
  return Boolean(signal?.aborted || (error instanceof DOMException && error.name === 'AbortError'))
}

export async function transferCrossPanelFileBatch(
  payload: CrossPanelFileDragPayload,
  targetDirectory: string,
  transferOne: TransferOne,
  onProgress?: (progress: CrossPanelBatchProgress) => void,
  signal?: AbortSignal,
): Promise<CrossPanelBatchResult> {
  const batchId = crypto.randomUUID().replaceAll('-', '')
  const result: CrossPanelBatchResult = { succeeded: [], failed: [], cancelled: false }
  for (const [index, source] of payload.entries.entries()) {
    if (signal?.aborted) {
      result.cancelled = true
      break
    }
    try {
      const entry = await transferOne({
        batchId, conflictPolicies: {file:'ask',directory:'ask'},
        sourceNodeId: payload.sourceNodeId,
        path: source.path,
        resourceVersion: source.resourceVersion,
        targetDirectory,
      }, (event) => onProgress?.({
        index,
        total: payload.entries.length,
        completed: result.succeeded.length + result.failed.length,
        source,
        event,
      }), signal)
      result.succeeded.push({ source, entry })
    } catch (error) {
      if (error instanceof FileTransferSkippedError) { (result.skipped ||= []).push(source); continue }
      if (error instanceof FileTransferPartialError) { result.succeeded.push({source,entry:error.entry}); (result.skipped ||= []).push(source); continue }
      if (isAbort(error, signal)) {
        result.cancelled = true
        break
      }
      result.failed.push({
        source,
        detail: error instanceof Error ? error.message : '跨主机复制失败。',
      })
    }
  }
  return result
}

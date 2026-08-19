// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { transferCrossPanelFileBatch } from './crossPanelFileTransfer'

const payload = {
  sourceNodeId: 'a'.repeat(32),
  entries: [
    { name: 'one.txt', path: '/one.txt', kind: 'file' as const, resourceVersion: 'sha256:one' },
    { name: 'app', path: '/app', kind: 'directory' as const, resourceVersion: 'sha256:app' },
  ],
}

function copied(name: string, kind: 'file' | 'directory') {
  return {
    name, path: `/target/${name}`, kind, sizeBytes: 4, mode: '0644', owner: 'root', group: 'root',
    modifiedAt: '2026-08-15T00:00:00Z', resourceVersion: `sha256:target-${name}`,
    editable: kind === 'file', previewable: kind === 'file',
  }
}

describe('cross-panel file batch', () => {
  it('copies sequentially and keeps later items running after one failure', async () => {
    const transfer = vi.fn()
      .mockRejectedValueOnce(new Error('source changed'))
      .mockImplementationOnce(async (_input, onEvent) => {
        onEvent({ state: 'transferring', loadedBytes: 2, totalBytes: 4 })
        return copied('app', 'directory')
      })
    const progress = vi.fn()

    const result = await transferCrossPanelFileBatch(payload, '/target', transfer, progress)

    expect(transfer).toHaveBeenCalledTimes(2)
    expect(transfer.mock.calls[1]?.[0]).toEqual({
      sourceNodeId: 'a'.repeat(32), path: '/app', resourceVersion: 'sha256:app',
      targetDirectory: '/target',
    })
    expect(result.failed).toEqual([{ source: payload.entries[0], detail: 'source changed' }])
    expect(result.succeeded[0]?.entry.path).toBe('/target/app')
    expect(progress).toHaveBeenCalledWith(expect.objectContaining({ index: 1, total: 2 }))
  })

  it('stops at cancellation and preserves already completed results', async () => {
    const controller = new AbortController()
    const transfer = vi.fn()
      .mockResolvedValueOnce(copied('one.txt', 'file'))
      .mockImplementationOnce(async () => {
        controller.abort()
        throw new DOMException('Aborted', 'AbortError')
      })

    const result = await transferCrossPanelFileBatch(payload, '/target', transfer, undefined, controller.signal)

    expect(result.cancelled).toBe(true)
    expect(result.succeeded.map(({ entry }) => entry.name)).toEqual(['one.txt'])
    expect(result.failed).toHaveLength(0)
  })
})

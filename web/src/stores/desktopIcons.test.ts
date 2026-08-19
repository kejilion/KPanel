import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api } from '@/lib/api'
import { resetDesktopIconsForTest, useDesktopIcons } from './desktopIcons'
import type { DesktopWorkspace, DesktopWorkspaceUpdate } from '@/types/api'

function version(character: string): string {
  return `sha256:${character.repeat(64)}`
}

function workspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
  return {
    schemaVersion: 2,
    resourceVersion: version('1'),
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    labels: {},
    shortcuts: [],
    ...overrides,
  }
}

describe('desktop icon workspace store', () => {
  beforeEach(() => {
    resetDesktopIconsForTest()
    vi.restoreAllMocks()
  })

  it('loads the server-owned workspace', async () => {
    vi.spyOn(api.desktop, 'workspace').mockResolvedValue(workspace({
      hiddenEntryKeys: ['app:nginx'],
    }))
    const store = useDesktopIcons()

    await store.load()

    expect(store.loaded.value).toBe(true)
    expect(store.workspace.value.hiddenEntryKeys).toEqual(['app:nginx'])
  })

  it('serializes quick mutations against the latest confirmed resource version', async () => {
    vi.spyOn(api.desktop, 'workspace').mockResolvedValue(workspace())
    let writes = 0
    const update = vi.spyOn(api.desktop, 'updateWorkspace')
      .mockImplementation(async (body: DesktopWorkspaceUpdate) => {
        writes += 1
        return workspace({
          resourceVersion: version(String(writes + 1)),
          hiddenEntryKeys: body.hiddenEntryKeys,
          positions: body.positions,
          labels: body.labels,
        })
      })
    const store = useDesktopIcons()
    await store.load()

    const first = store.mutate((draft) => draft.hiddenEntryKeys.push('app:nginx'))
    const second = store.mutate((draft) => draft.labels['site:blog'] = '博客')
    await Promise.all([first, second])

    expect(update).toHaveBeenCalledTimes(2)
    expect(update.mock.calls[0]?.[0].expectedResourceVersion).toBe(version('1'))
    expect(update.mock.calls[1]?.[0].expectedResourceVersion).toBe(version('2'))
    expect(update.mock.calls[1]?.[0]).toMatchObject({
      hiddenEntryKeys: ['app:nginx'],
      labels: { 'site:blog': '博客' },
    })
  })

  it('refreshes the remote winner after a version conflict without retrying over it', async () => {
    const remote = workspace({
      resourceVersion: version('9'),
      labels: { 'site:blog': '远端名称' },
    })
    const load = vi.spyOn(api.desktop, 'workspace')
      .mockResolvedValueOnce(workspace())
      .mockResolvedValueOnce(remote)
    const update = vi.spyOn(api.desktop, 'updateWorkspace').mockRejectedValue(
      new ApiError('conflict', 409, 'resource_version_conflict'),
    )
    const store = useDesktopIcons()
    await store.load()

    await expect(store.mutate((draft) => draft.labels['site:blog'] = '本地名称'))
      .rejects.toMatchObject({ status: 409 })

    expect(update).toHaveBeenCalledTimes(1)
    expect(load).toHaveBeenCalledTimes(2)
    expect(store.workspace.value).toEqual(remote)
  })
})

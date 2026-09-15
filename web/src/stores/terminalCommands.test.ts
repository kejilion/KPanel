import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/lib/api'
import { resetTerminalCommandsForTest, useTerminalCommands } from './terminalCommands'
import type { TerminalQuickCommands, TerminalQuickCommandsUpdate } from '@/types/api'

function version(character: string): string {
  return `sha256:${character.repeat(64)}`
}

function snapshot(overrides: Partial<TerminalQuickCommands> = {}): TerminalQuickCommands {
  return {
    schemaVersion: 1,
    resourceVersion: version('1'),
    available: true,
    items: [],
    ...overrides,
  }
}

describe('terminal command favorites store', () => {
  beforeEach(() => {
    resetTerminalCommandsForTest()
    vi.restoreAllMocks()
  })

  it('loads commands owned by the panel', async () => {
    vi.spyOn(api.terminals, 'commands').mockResolvedValue(snapshot({
      items: [{ id: '1'.repeat(32), name: '容器列表', command: 'docker ps' }],
    }))
    const store = useTerminalCommands()

    await store.load()

    expect(store.loaded.value).toBe(true)
    expect(store.commands.value.items[0]?.name).toBe('容器列表')
  })

  it('serializes mutations against the latest confirmed resource version', async () => {
    vi.spyOn(api.terminals, 'commands').mockResolvedValue(snapshot())
    let writes = 0
    const update = vi.spyOn(api.terminals, 'updateCommands')
      .mockImplementation(async (body: TerminalQuickCommandsUpdate) => {
        writes += 1
        return snapshot({ resourceVersion: version(String(writes + 1)), items: body.items })
      })
    const store = useTerminalCommands()
    await store.load()

    const first = store.mutate((draft) => draft.items.push({
      id: '1'.repeat(32), name: '容器列表', command: 'docker ps',
    }))
    const second = store.mutate((draft) => draft.items.push({
      id: '2'.repeat(32), name: '系统负载', command: 'uptime',
    }))
    await Promise.all([first, second])

    expect(update).toHaveBeenCalledTimes(2)
    expect(update.mock.calls[0]?.[0].expectedResourceVersion).toBe(version('1'))
    expect(update.mock.calls[1]?.[0].expectedResourceVersion).toBe(version('2'))
    expect(update.mock.calls[1]?.[0].items).toHaveLength(2)
  })

  it('loads the remote winner after a version conflict without overwriting it', async () => {
    const remote = snapshot({
      resourceVersion: version('9'),
      items: [{ id: '9'.repeat(32), name: '远端命令', command: 'uptime' }],
    })
    const load = vi.spyOn(api.terminals, 'commands')
      .mockResolvedValueOnce(snapshot())
      .mockResolvedValueOnce(remote)
    const update = vi.spyOn(api.terminals, 'updateCommands').mockRejectedValue(
      new ApiError('conflict', 409, 'terminal_commands_changed'),
    )
    const store = useTerminalCommands()
    await store.load()

    await expect(store.mutate((draft) => draft.items.push({
      id: '1'.repeat(32), name: '本地命令', command: 'df -h',
    }))).rejects.toMatchObject({ status: 409 })

    expect(update).toHaveBeenCalledTimes(1)
    expect(load).toHaveBeenCalledTimes(2)
    expect(store.commands.value).toEqual(remote)
  })
})

import { createSSRApp, ssrContextKey, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import { ApiError, api } from '@/lib/api'
import type { DockerContainer, TerminalSession } from '@/types/api'

vi.mock('@/lib/api', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/lib/api')>()
  return { ...original, api: { ...original.api, terminals: { open: vi.fn(), close: vi.fn() } } }
})

interface Bindings {
  consoleOpen: Ref<boolean>
  consoleOpening: Ref<boolean>
  consoleSession: Ref<TerminalSession | undefined>
  consoleTarget: Ref<DockerContainer | undefined>
  consoleError: Ref<string>
  openConsole: (container: DockerContainer) => Promise<void>
  closeConsole: () => void
}

function setupView(): Bindings {
  const component = DockerView as unknown as { setup: (props: object, context: { expose: () => void }) => Bindings }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try { return app.runWithContext(() => component.setup({}, { expose: () => undefined })) }
  finally { warn.mockRestore() }
}

function container(id = 'a'): DockerContainer {
  return { id: id.repeat(64), name: `container-${id}`, image: 'alpine', state: 'running', resourceVersion: `version-${id}`, access: 'managed', consistency: 'synced', ports: [], networks: [], mounts: [] }
}

function session(id = 'session-a'): TerminalSession {
  return { sessionId: id, hostId: 'local', offset: 0, createdAt: '2026-10-10T00:00:00Z' }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

beforeEach(() => {
  vi.resetAllMocks()
  const storage = new Map<string, string>()
  vi.stubGlobal('window', { localStorage: { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) } })
  vi.mocked(api.terminals.close).mockResolvedValue({ closed: true })
})

afterEach(() => { vi.unstubAllGlobals() })

describe('Docker container terminal', () => {
  it('opens only one shared terminal with the exact selected container snapshot', async () => {
    const opening = deferred<TerminalSession>()
    vi.mocked(api.terminals.open).mockReturnValue(opening.promise)
    const view = setupView()
    const target = container()
    const pending = view.openConsole(target)
    await view.openConsole(target)
    expect(api.terminals.open).toHaveBeenCalledExactlyOnceWith('local', 30, 120, { containerId: target.id, resourceVersion: 'version-a' })
    target.name = 'changed-in-list'
    expect(view.consoleTarget.value?.name).toBe('container-a')
    opening.resolve(session())
    await pending
    await view.openConsole(target)
    expect(api.terminals.open).toHaveBeenCalledTimes(1)
    expect(view.consoleSession.value?.sessionId).toBe('session-a')
    expect(view.consoleOpening.value).toBe(false)
  })

  it('reclaims a session whose open completes after the window closes', async () => {
    const opening = deferred<TerminalSession>()
    vi.mocked(api.terminals.open).mockReturnValue(opening.promise)
    const view = setupView()
    const pending = view.openConsole(container())
    view.closeConsole()
    opening.resolve(session())
    await pending
    expect(api.terminals.close).toHaveBeenCalledWith('session-a')
    expect(view.consoleSession.value).toBeUndefined()
    expect(view.consoleOpen.value).toBe(false)
  })

  it('closes an accepted session even before the terminal child has mounted', async () => {
    vi.mocked(api.terminals.open).mockResolvedValue(session())
    const view = setupView()
    await view.openConsole(container())
    view.closeConsole()
    expect(api.terminals.close).toHaveBeenCalledExactlyOnceWith('session-a')
    expect(view.consoleSession.value).toBeUndefined()
  })

  it('does not let a late response for A replace B or clear B loading state', async () => {
    const a = deferred<TerminalSession>()
    const b = deferred<TerminalSession>()
    vi.mocked(api.terminals.open).mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
    const view = setupView()
    const first = view.openConsole(container('a'))
    view.closeConsole()
    const second = view.openConsole(container('b'))
    a.resolve(session('a-session'))
    await first
    expect(view.consoleOpening.value).toBe(true)
    expect(view.consoleTarget.value?.id).toBe('b'.repeat(64))
    expect(view.consoleSession.value).toBeUndefined()
    expect(api.terminals.close).toHaveBeenCalledWith('a-session')
    b.resolve(session('b-session'))
    await second
    expect(view.consoleSession.value?.sessionId).toBe('b-session')
    expect(api.terminals.open).toHaveBeenLastCalledWith('local', 30, 120, { containerId: 'b'.repeat(64), resourceVersion: 'version-b' })
  })

  it('ignores an old failure after opening a different container', async () => {
    const old = deferred<TerminalSession>()
    vi.mocked(api.terminals.open).mockReturnValueOnce(old.promise).mockResolvedValueOnce(session('b-session'))
    const view = setupView()
    const pending = view.openConsole(container())
    await view.openConsole(container('b'))
    old.reject(new Error('old failure'))
    await pending
    expect(view.consoleError.value).toBe('')
    expect(view.consoleSession.value?.sessionId).toBe('b-session')
  })

  it.each([
    ['terminal_limit', '已达到终端会话上限，请先关闭不用的终端。'],
    ['resource_conflict', '容器状态已变化，请刷新列表后重新打开终端。'],
    ['terminal_open_failed', '容器终端启动失败，请检查容器状态与 Agent 终端服务。'],
  ])('shows %s and can reopen after failure', async (code, message) => {
    vi.mocked(api.terminals.open).mockRejectedValueOnce(new ApiError('failed', 409, code)).mockResolvedValueOnce(session())
    const view = setupView()
    await view.openConsole(container())
    expect(view.consoleError.value).toBe(message)
    expect(view.consoleSession.value).toBeUndefined()
    expect(view.consoleOpening.value).toBe(false)
    await view.openConsole(container())
    expect(view.consoleError.value).toBe('')
    expect(view.consoleSession.value?.sessionId).toBe('session-a')
  })

  it('never falls back to a host shell when the container version is missing', async () => {
    const target = container()
    target.resourceVersion = undefined
    const view = setupView()
    await view.openConsole(target)
    expect(api.terminals.open).not.toHaveBeenCalled()
    expect(view.consoleError.value).not.toBe('')
  })
})

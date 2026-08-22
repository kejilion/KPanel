import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, normalizeList, resetApiSecurityState } from './api'
import type { SystemOverview } from '@/types/api'

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers)
  headers.set('content-type', headers.get('content-type') || 'application/json')
  return new Response(JSON.stringify(body), { ...init, headers })
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  resetApiSecurityState()
})

describe('API client', () => {
  it('streams cross-panel file progress and returns the committed entry', async () => {
    const entry = {
      name: 'app', path: '/home/KPanel Desktop/app', kind: 'directory' as const,
      sizeBytes: 0, mode: 'drwxr-xr-x', owner: 'root', group: 'root',
      modifiedAt: '2026-08-15T00:00:00Z', resourceVersion: `sha256:${'a'.repeat(64)}`,
      editable: false, previewable: false,
    }
    const stream = [
      { state: 'connecting' },
      { state: 'transferring', loadedBytes: 1024, totalBytes: 2048 },
      { state: 'complete', loadedBytes: 2048, totalBytes: 2048, entry },
    ].map((event) => JSON.stringify(event)).join('\n') + '\n'
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(stream, {
      status: 200,
      headers: { 'content-type': 'application/x-ndjson' },
    }))
    vi.stubGlobal('fetch', fetchMock)
    const events: string[] = []

    await expect(api.files.transferFromPanel({
      sourceNodeId: 'a'.repeat(32), path: '/app', resourceVersion: 'sha256:source',
      targetDirectory: '/home/KPanel Desktop',
    }, (event) => events.push(event.state))).resolves.toEqual(entry)
    expect(events).toEqual(['connecting', 'transferring', 'complete'])
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/files/transfers')
    expect(fetchMock.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      method: 'POST', credentials: 'same-origin', cache: 'no-store',
    }))
  })

  it('builds a same-origin, path-safe site icon URL', () => {
    expect(api.sites.iconURL('a'.repeat(32))).toBe(
      `/api/v1/sites/${'a'.repeat(32)}/icon`,
    )
    expect(api.sites.iconURL('site/id ?')).toBe(
      '/api/v1/sites/site%2Fid%20%3F/icon',
    )
  })

  it('uses the authenticated desktop workspace contract and path-safe custom icon URL', async () => {
    const current = {
      schemaVersion: 1 as const,
      resourceVersion: `sha256:${'1'.repeat(64)}`,
      available: true,
      hiddenEntryKeys: [],
      positions: {},
      labels: {},
      shortcuts: [],
    }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(current))
      .mockResolvedValueOnce(jsonResponse({ ...current, resourceVersion: `sha256:${'2'.repeat(64)}` }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.desktop.workspace()).resolves.toEqual(current)
    await api.desktop.updateWorkspace({
      expectedResourceVersion: current.resourceVersion,
      hiddenEntryKeys: ['app:nginx'],
      positions: {},
      widgetPositions: {},
      labels: {},
      shortcuts: [],
    })

    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/desktop/workspace')
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/v1/desktop/workspace')
    expect(fetchMock.mock.calls[1]?.[1]).toEqual(expect.objectContaining({ method: 'PUT' }))
    expect(api.desktop.shortcutIconURL('id /?', 'a'.repeat(64))).toBe(
      `/api/v1/desktop/shortcuts/id%20%2F%3F/icon?v=${'a'.repeat(64)}`,
    )
  })

  it('loads a site appearance only through the authenticated same-origin API', async () => {
    const id = 'a'.repeat(32)
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ name: '科技狮网站' }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.sites.appearance(id)).resolves.toEqual({ name: '科技狮网站' })
    expect(fetchMock.mock.calls[0]?.[0]).toBe(`/api/v1/sites/${id}/appearance`)
    expect(fetchMock.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      credentials: 'same-origin',
      cache: 'no-store',
    }))
  })

  it('detects a server that still requires bootstrap', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ required: true }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.auth.status()).resolves.toMatchObject({
      setupRequired: true,
      authenticated: false,
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/bootstrap',
      expect.objectContaining({ credentials: 'same-origin', cache: 'no-store' }),
    )
  })

  it('keeps the CSRF token in memory and sends it on mutations', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ required: false }))
      .mockResolvedValueOnce(
        jsonResponse({
          user: { id: 'user-1', username: 'admin', role: 'administrator' },
          csrfToken: 'csrf-secret',
          expiresAt: '2026-07-25T12:00:00Z',
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.auth.status()).resolves.toMatchObject({
      setupRequired: false,
      authenticated: true,
      user: { username: 'admin' },
    })
    await api.auth.logout()

    const logoutInit = fetchMock.mock.calls[2]?.[1] as RequestInit
    expect(new Headers(logoutInit.headers).get('x-csrf-token')).toBe('csrf-secret')
    expect(logoutInit.method).toBe('POST')
  })

  it('parses the stable problem+json error contract', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce(
        jsonResponse(
          {
            title: '请求冲突',
            status: 409,
            code: 'resource_version_conflict',
            detail: '资源已被外部修改。',
            requestId: 'req-123',
          },
          { status: 409, headers: { 'content-type': 'application/problem+json' } },
        ),
      ),
    )

    const error = await api.sites.list().catch((reason: unknown) => reason)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      status: 409,
      code: 'resource_version_conflict',
      message: '资源已被外部修改。',
      requestId: 'req-123',
    })
  })

  it('does not expose raw reverse-proxy HTML errors in the interface', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce(
        new Response('<html><head><title>400 Bad Request</title></head><body>nginx</body></html>', {
          status: 400,
          headers: { 'content-type': 'text/html' },
        }),
      ),
    )

    const error = await api.sites.list().catch((reason: unknown) => reason)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      status: 400,
      message: '请求被反向代理或安全网关拒绝（HTTP 400）',
    })
  })

  it('normalizes site create and update responses from the Agent contract', async () => {
    const createdRaw = {
      id: 'a'.repeat(32),
      primaryDomain: 'example.com',
      domains: ['example.com', 'www.example.com'],
      kind: 'reverse_proxy',
      enabled: true,
      health: 'healthy',
      tls: {
        enabled: true,
        status: 'valid',
        expiresAt: '2026-12-31T00:00:00Z',
        source: 'acme',
      },
      target: 'http://127.0.0.1:3000',
      origin: 'web',
      consistency: 'in_sync',
      resourceVersion: `sha256:${'b'.repeat(64)}`,
      allowedActions: ['update'],
      artifacts: [{ kind: 'nginx', path: '/etc/nginx/conf.d/example.com.conf', hash: 'abc' }],
      warnings: [],
      reconciledAt: '2026-07-25T10:00:00Z',
    }
    const updatedRaw = {
      ...createdRaw,
      domains: ['example.com'],
      target: 'http://127.0.0.1:4000',
      resourceVersion: `sha256:${'c'.repeat(64)}`,
      reconciledAt: '2026-07-25T10:05:00Z',
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(createdRaw))
      .mockResolvedValueOnce(jsonResponse(updatedRaw))
    vi.stubGlobal('fetch', fetchMock)

    const created = await api.sites.create({
      primaryDomain: 'example.com',
      aliases: ['www.example.com'],
      type: 'proxy',
      upstream: 'http://127.0.0.1:3000',
      enabled: true,
    })
    const updated = await api.sites.update(createdRaw.id, {
      primaryDomain: 'example.com',
      aliases: [],
      type: 'proxy',
      upstream: 'http://127.0.0.1:4000',
      enabled: true,
      expectedResourceVersion: createdRaw.resourceVersion,
    })

    expect(created).toMatchObject({
      id: createdRaw.id,
      type: 'proxy',
      upstream: createdRaw.target,
      consistency: 'synced',
      access: 'managed',
      source: 'panel',
      certificate: {
        status: 'valid',
        issuer: 'acme',
        expiresAt: '2026-12-31T00:00:00Z',
      },
    })
    expect(updated).toMatchObject({
      domains: ['example.com'],
      upstream: updatedRaw.target,
      resourceVersion: updatedRaw.resourceVersion,
      observedAt: updatedRaw.reconciledAt,
    })

    const createInit = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/sites')
    expect(createInit.method).toBe('POST')
    expect(JSON.parse(String(createInit.body))).not.toHaveProperty('expectedResourceVersion')

    const updateInit = fetchMock.mock.calls[1]?.[1] as RequestInit
    expect(fetchMock.mock.calls[1]?.[0]).toBe(`/api/v1/sites/${createdRaw.id}`)
    expect(updateInit.method).toBe('PATCH')
    expect(JSON.parse(String(updateInit.body))).toMatchObject({
      upstream: 'http://127.0.0.1:4000',
      expectedResourceVersion: createdRaw.resourceVersion,
    })
  })

  it('sends full site deletion to the script adapter without a browser resource version gate', async () => {
    const id = 'a'.repeat(32)
    const version = `sha256:${'b'.repeat(64)}`
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        id,
        primaryDomain: 'example.com',
        status: 'deleted',
        mode: 'full',
        resourceVersion: version,
        removed: ['nginx_config', 'document_root'],
        databaseDropped: false,
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.sites.remove(id, 'example.com')).resolves.toMatchObject({
      status: 'deleted',
      mode: 'full',
      primaryDomain: 'example.com',
    })
    const requestInit = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(fetchMock.mock.calls[0]?.[0]).toBe(`/api/v1/sites/${id}`)
    expect(requestInit.method).toBe('DELETE')
    expect(JSON.parse(String(requestInit.body))).toEqual({
      primaryDomain: 'example.com',
    })
  })

  it('shows the actionable validation field instead of the generic problem title', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            title: 'Validation failed',
            status: 422,
            code: 'validation_failed',
            fieldErrors: {
              primaryDomain: '站点域名无效，请刷新后重试。',
            },
          },
          { status: 422 },
        ),
      ),
    )

    await expect(api.sites.remove('a'.repeat(32), 'example.com')).rejects.toMatchObject({
      message: '站点域名无效，请刷新后重试。',
    })
  })

  it('polls an asynchronous WordPress installation until the reconciled site is ready', async () => {
    vi.useFakeTimers()
    const jobID = 'd'.repeat(32)
    const installedRaw = {
      id: 'e'.repeat(32),
      primaryDomain: 'blog.example.com',
      domains: ['blog.example.com'],
      kind: 'wordpress',
      enabled: true,
      health: 'healthy',
      tls: {
        enabled: true,
        status: 'valid',
        expiresAt: '2026-12-31T00:00:00Z',
        source: 'acme',
      },
      target: 'php',
      documentRoot: '/home/web/html/blog.example.com/wordpress',
      origin: 'web',
      consistency: 'in_sync',
      resourceVersion: `sha256:${'f'.repeat(64)}`,
      allowedActions: [],
      artifacts: [],
      warnings: [],
      reconciledAt: '2026-07-26T10:00:00Z',
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse(
          {
            id: jobID,
            domain: 'blog.example.com',
            status: 'queued',
            stage: 'queued',
            progress: 0,
            message: 'queued',
            createdAt: '2026-07-26T09:59:55Z',
          },
          { status: 202 },
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: jobID,
          domain: 'blog.example.com',
          status: 'running',
          stage: 'database',
          progress: 38,
          message: 'installing',
          createdAt: '2026-07-26T09:59:55Z',
          startedAt: '2026-07-26T09:59:56Z',
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse(
          {
            type: 'about:blank',
            title: 'Agent unavailable',
            status: 503,
            code: 'agent_unavailable',
          },
          { status: 503 },
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: jobID,
          domain: 'blog.example.com',
          status: 'succeeded',
          stage: 'completed',
          progress: 100,
          message: 'completed',
          site: installedRaw,
          createdAt: '2026-07-26T09:59:55Z',
          startedAt: '2026-07-26T09:59:56Z',
          endedAt: '2026-07-26T10:00:00Z',
        }),
      )
    vi.stubGlobal('fetch', fetchMock)
    const onProgress = vi.fn()

    const created = api.sites.create(
      {
        primaryDomain: 'blog.example.com',
        aliases: [],
        type: 'wordpress',
        enabled: true,
      },
      onProgress,
    )
    await vi.advanceTimersByTimeAsync(6_000)

    await expect(created).resolves.toMatchObject({
      primaryDomain: 'blog.example.com',
      type: 'wordpress',
      rootPath: '/home/web/html/blog.example.com/wordpress',
      consistency: 'synced',
      access: 'managed',
      source: 'panel',
    })
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/sites',
      `/api/v1/site-installations/${jobID}`,
      `/api/v1/site-installations/${jobID}`,
      `/api/v1/site-installations/${jobID}`,
    ])
    expect(onProgress).toHaveBeenCalledWith({
      id: jobID,
      domain: 'blog.example.com',
      status: 'running',
      stage: 'database',
      progress: 38,
      message: 'installing',
    })
    expect(onProgress).toHaveBeenCalledWith({
      id: jobID,
      domain: 'blog.example.com',
      status: 'running',
      stage: 'reconnecting',
      progress: 38,
      message: 'Agent 暂时不可用，后台建站任务不受影响，正在自动重连。',
    })
  })

  it('treats scripted static templates as background installation jobs', async () => {
    const jobID = 'a'.repeat(32)
    const siteID = 'b'.repeat(32)
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(
        {
          id: jobID,
          domain: 'static.example.com',
          recipe: 'static-site',
          status: 'succeeded',
          stage: 'completed',
          progress: 100,
          message: 'completed',
          site: {
            id: siteID,
            primaryDomain: 'static.example.com',
            domains: ['static.example.com'],
            kind: 'static',
            enabled: true,
            health: 'healthy',
            documentRoot: '/home/web/html/static.example.com',
            origin: 'web',
            consistency: 'in_sync',
            resourceVersion: `sha256:${'c'.repeat(64)}`,
            allowedActions: [],
            artifacts: [],
            warnings: [],
          },
          createdAt: '2026-07-29T04:00:00Z',
          startedAt: '2026-07-29T04:00:01Z',
          finishedAt: '2026-07-29T04:00:10Z',
        },
        { status: 202 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      api.sites.create({
        primaryDomain: 'static.example.com',
        aliases: [],
        type: 'static',
        enabled: true,
      }),
    ).resolves.toMatchObject({
      id: siteID,
      primaryDomain: 'static.example.com',
      type: 'static',
      rootPath: '/home/web/html/static.example.com',
    })
  })

  it('surfaces safe site installation events and the script failure reason', async () => {
    const jobID = 'c'.repeat(32)
    const events = [
      {
        stage: 'preflight',
        progress: 10,
        message: '正在校验 WordPress 域名与现有站点',
        at: '2026-07-27T10:00:00Z',
      },
      {
        stage: 'failed',
        progress: 35,
        message: '建站失败：证书签发失败（脚本退出码 1）',
        at: '2026-07-27T10:00:05Z',
      },
    ]
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse(
        {
          id: jobID,
          domain: 'blog.example.com',
          status: 'failed',
          stage: 'failed',
          progress: 35,
          message: events[1]?.message,
          events,
        },
        { status: 202 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onProgress = vi.fn()

    await expect(
      api.sites.create(
        {
          primaryDomain: 'blog.example.com',
          aliases: [],
          type: 'wordpress',
          enabled: true,
        },
        onProgress,
      ),
    ).rejects.toMatchObject({
      code: 'site_install_failed',
      message: '建站失败：证书签发失败（脚本退出码 1）',
    })
    expect(onProgress).toHaveBeenLastCalledWith({
      id: jobID,
      domain: 'blog.example.com',
      status: 'failed',
      stage: 'failed',
      progress: 35,
      message: '建站失败：证书签发失败（脚本退出码 1）',
      events,
    })
  })

  it('sends only the typed system action payload through the protected mutation path', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        action: 'dns',
        status: 'succeeded',
        changed: true,
        message: 'DNS 已更新',
        appliedAt: '2026-07-26T03:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.system.action({ action: 'dns', servers: ['1.1.1.1', '8.8.8.8'] })).resolves.toMatchObject({
      action: 'dns',
      changed: true,
    })
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/actions')
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'dns',
      servers: ['1.1.1.1', '8.8.8.8'],
    })
  })

  it('loads system collection resources only from their dedicated endpoints', async () => {
    const envelope = { resourceVersion: 'rv-1', entries: [], total: 0, truncated: false }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(envelope))
      .mockResolvedValueOnce(jsonResponse(envelope))
      .mockResolvedValueOnce(jsonResponse(envelope))
      .mockResolvedValueOnce(jsonResponse({
        resourceVersion: 'rv-firewall',
        backend: 'iptables-nft',
        inputPolicy: 'DROP',
        rules: [],
        total: 0,
        truncated: false,
        pingAllowed: true,
        ddosEnabled: false,
      }))
	  .mockResolvedValueOnce(jsonResponse({ ...envelope, observedAt: '2026-08-10T08:00:00Z' }))
		  .mockResolvedValueOnce(jsonResponse({
			resourceVersion: 'rv-traffic', enabled: false, health: 'disabled',
		rxBytes: 0, txBytes: 0, rxThresholdGiB: 0, txThresholdGiB: 0, resetDay: 0,
			observedAt: '2026-08-10T08:00:00Z',
		  }))
		  .mockResolvedValueOnce(jsonResponse({
			resourceVersion: 'rv-accounts', accounts: [], total: 0, truncated: false,
			sshPolicy: { passwordAuthentication: false, publicKeyAuthentication: true, rootLogin: 'key-only' },
			observedAt: '2026-08-11T08:00:00Z',
		  }))
		  .mockResolvedValueOnce(jsonResponse({
			resourceVersion: 'a'.repeat(64), items: [], maintenance: { state: 'idle', progress: 0, rebootRequired: false },
			observedAt: '2026-08-11T08:00:00Z',
		  }))
    vi.stubGlobal('fetch', fetchMock)

    await api.system.hosts()
    await api.system.cron()
    await api.system.networkInterfaces()
    await api.system.firewall()
	await api.system.portUsage()
		await api.system.trafficShutdown()
		await api.system.accounts()
		await api.system.systemTuning()

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/system/hosts',
      '/api/v1/system/cron',
      '/api/v1/system/network-interfaces',
      '/api/v1/system/firewall',
	  '/api/v1/system/port-usage',
		  '/api/v1/system/traffic-shutdown',
		  '/api/v1/system/accounts',
		  '/api/v1/system/system-tuning',
    ])
    expect(fetchMock.mock.calls.every((call) => (call[1] as RequestInit | undefined)?.method === 'GET')).toBe(true)
  })

  it('normalizes empty account collections from older Agent responses', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({
      resourceVersion: 'a'.repeat(64),
      accounts: [{
        username: 'root', uid: 0, gid: 0, home: '/root', shell: '/bin/bash',
        kind: 'root', passwordStatus: 'enabled', role: 'root', groups: null, sshKeys: null,
      }],
      total: 1,
      truncated: false,
      sshPolicy: { passwordAuthentication: true, publicKeyAuthentication: true, rootLogin: 'enabled' },
      observedAt: '2026-08-11T08:00:00Z',
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.system.accounts()).resolves.toMatchObject({
      accounts: [{ username: 'root', groups: [], sshKeys: [] }],
    })
  })

  it('submits exact typed system resource actions without protocol or raw shell fields', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        action: 'firewall-open-port',
        status: 'succeeded',
        changed: true,
        message: 'applied',
        resourceVersion: 'rv-2',
        appliedAt: '2026-08-10T08:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.system.resourceAction({
      action: 'firewall-open-port',
      port: 443,
      expectedResourceVersion: 'rv-1',
    })

    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/resource-actions')
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'firewall-open-port',
      port: 443,
      expectedResourceVersion: 'rv-1',
    })
  })

	it('submits a typed traffic shutdown action without shell fields', async () => {
		const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
			action: 'enable', status: 'succeeded', changed: true, message: 'applied',
			resourceVersion: 'rv-2', appliedAt: '2026-08-10T08:00:00Z',
		}))
		vi.stubGlobal('fetch', fetchMock)

		await api.system.trafficShutdownAction({
			action: 'enable', expectedResourceVersion: 'rv-1',
			rxThresholdGiB: 100, txThresholdGiB: 200, resetDay: 5,
		})

		expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/traffic-shutdown/actions')
		expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
			action: 'enable', expectedResourceVersion: 'rv-1',
			rxThresholdGiB: 100, txThresholdGiB: 200, resetDay: 5,
		})
	})

	it('submits typed account actions without raw shell fields', async () => {
		const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
			action: 'create', status: 'succeeded', changed: true, message: 'applied',
			resourceVersion: 'rv-2', appliedAt: '2026-08-11T08:00:00Z',
		}))
		vi.stubGlobal('fetch', fetchMock)
		await api.system.accountAction({
			action: 'create', expectedResourceVersion: 'rv-1', username: 'operator',
			role: 'passwordless-admin', credential: 'key', secret: 'ssh-ed25519 AAAA laptop',
		})
		expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/account-actions')
		expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
			action: 'create', expectedResourceVersion: 'rv-1', username: 'operator',
			role: 'passwordless-admin', credential: 'key', secret: 'ssh-ed25519 AAAA laptop',
		})
	})

	it('submits only fixed system tuning item IDs', async () => {
		const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ status: 'accepted' }))
		vi.stubGlobal('fetch', fetchMock)
		await api.system.systemTuningAction({
			action: 'apply', items: ['bbr', 'kernel-auto'], expectedResourceVersion: 'a'.repeat(64),
		})
		expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/system-tuning/actions')
		expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
			action: 'apply', items: ['bbr', 'kernel-auto'], expectedResourceVersion: 'a'.repeat(64),
		})
	})

  it('submits only the selected system cleanup policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        action: 'cleanup',
        status: 'succeeded',
        changed: true,
        message: 'queued',
        appliedAt: '2026-07-26T03:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.system.action({ action: 'cleanup', maintenancePolicy: 'standard' })

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'cleanup',
      maintenancePolicy: 'standard',
    })
  })

  it('submits a typed reboot action without a confirmation password', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        action: 'reboot',
        status: 'accepted',
        changed: true,
        message: 'queued',
        appliedAt: '2026-07-26T03:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.system.action({ action: 'reboot' })

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'reboot',
    })
  })

  it('submits only the typed SSH defense target state', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        action: 'ssh-defense',
        status: 'accepted',
        changed: true,
        message: 'queued',
        appliedAt: '2026-07-26T03:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.system.action({ action: 'ssh-defense', enabled: true })

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'ssh-defense',
      enabled: true,
    })
  })

  it('submits only the fixed BBRv3 action and policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        action: 'bbrv3',
        status: 'accepted',
        changed: true,
        message: 'queued',
        appliedAt: '2026-07-30T03:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.system.action({ action: 'bbrv3', maintenancePolicy: 'install' })

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(init.body))).toEqual({
      action: 'bbrv3',
      maintenancePolicy: 'install',
    })
  })

  it('keeps the overview compatible with an older Agent without management fields', async () => {
    const collectedAt = '2026-07-25T10:00:00Z'
    const system = {
      hostname: 'legacy-host',
      os: 'Debian 12',
      osId: 'debian',
      osLike: ['linux'],
      kernel: '6.1.0',
      architecture: 'amd64',
      uptimeSeconds: 120,
      load: { one: 0.2, five: 0.1, fifteen: 0.1 },
      cpu: { model: 'AMD EPYC Test CPU', cores: 2, frequencyMHz: 2450.5, usagePercent: 5 },
      memory: {
        totalBytes: 1024,
        availableBytes: 512,
        usedBytes: 512,
        usagePercent: 50,
        swapTotalBytes: 256,
        swapUsedBytes: 64,
      },
      disks: [],
      network: { receivedBytes: 100, sentBytes: 200, tcpConnections: 12, udpConnections: 3 },
      publicNetwork: {
        ipv4: '203.0.113.8',
        ipv6: '2001:db8::8',
        isp: 'AS64500 Example Network',
        country: 'CN',
        countryCode: 'CN',
        region: 'Shanghai',
        city: 'Shanghai',
        timezone: 'Asia/Shanghai',
        source: 'ipinfo.io',
        updatedAt: collectedAt,
      },
      collectedAt,
    }
    let resolveSystem!: (response: Response) => void
    const systemResponse = new Promise<Response>((resolve) => {
      resolveSystem = resolve
    })
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => systemResponse)
      .mockResolvedValueOnce(
        jsonResponse({
          status: 'ok',
          version: '0.1.3',
          protocolVersion: 'v1',
          readOnly: false,
          checkedAt: collectedAt,
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ items: [{ id: 'system.read', enabled: true }] }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({ available: false, containers: 0, running: 0, stopped: 0, images: 0, collectedAt }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse(system.publicNetwork))
      .mockResolvedValueOnce(
        jsonResponse({
          schemaVersion: 1,
          source: 'fixture',
          scriptSha256: 'fixture',
          catalogMode: 'embedded',
          categories: [],
          items: [{ id: 'app-1' }, { id: 'app-2' }, { id: 'app-3' }, { id: 'app-4' }],
          installed: 3,
          running: 2,
          updateAvailable: 1,
          collectedAt,
        }),
      )
    vi.stubGlobal('fetch', fetchMock)

    const updates: SystemOverview[] = []
    const overviewRequest = api.overview.get(undefined, (value) => updates.push(value))
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/system/summary')
    resolveSystem(jsonResponse(system))
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(8))
    const overview = await overviewRequest

    expect(overview.hostname).toBe('legacy-host')
    expect(overview.osId).toBe('debian')
    expect(overview.osLike).toEqual(['linux'])
    expect(overview.management).toMatchObject({
      ssh: {
        ports: [],
        source: 'unknown',
        defense: {
          available: false,
          installed: false,
          running: false,
          enabled: false,
          autostart: false,
          banned: 0,
        },
      },
      dns: { servers: [], manager: 'unknown' },
      swap: {
        totalBytes: 256,
        usedBytes: 64,
        activeDevices: 0,
        path: '/swapfile',
        fileExists: false,
        fileActive: false,
        legacyExists: false,
        legacyActive: false,
        otherActiveDevices: 0,
      },
      packageSources: [],
      maintenance: { state: 'idle', progress: 0, rebootRequired: false },
      ipPreference: 'unknown',
      bbr: { supported: false, enabled: false, available: [] },
      bbrv3: {
        available: false,
        supported: false,
        installed: false,
        active: false,
        rebootRequired: false,
      },
      capabilities: { 'system.read': { enabled: true } },
    })
    expect(overview.cpu).toMatchObject({
      model: 'AMD EPYC Test CPU',
      cores: 2,
      frequencyMHz: 2450.5,
    })
    expect(overview.load).toMatchObject({ one: 0.2, five: 0.1, fifteen: 0.1 })
    expect(overview.network).toMatchObject({
      tcpConnections: 12,
      udpConnections: 3,
      rateAvailable: false,
    })
    expect(overview.publicNetwork).toMatchObject({
      ipv4: '203.0.113.8',
      ipv6: '2001:db8::8',
      isp: 'AS64500 Example Network',
      country: 'CN',
      countryCode: 'CN',
      city: 'Shanghai',
      source: 'ipinfo.io',
    })
    expect(overview.apps).toEqual({ total: 4, installed: 3, running: 2, updateAvailable: 1 })
    expect(updates).toHaveLength(7)
    expect(overview).toBe(updates.at(-1))
    expect(new Set(updates.map((value) => value.cpu)).size).toBe(1)
    const capabilityUpdates = updates.filter(
      (value) => value.management.capabilities['system.read'],
    )
    expect(capabilityUpdates.length).toBeGreaterThan(1)
    expect(new Set(capabilityUpdates.map((value) => value.management.capabilities)).size).toBe(1)

    const nextCollectedAt = '2026-07-25T10:00:20Z'
    fetchMock
      .mockResolvedValueOnce(jsonResponse({
        ...system,
        network: { ...system.network, receivedBytes: 500, sentBytes: 600 },
        collectedAt: nextCollectedAt,
      }))
      .mockResolvedValueOnce(jsonResponse({
        status: 'ok',
        version: '0.1.3',
        protocolVersion: 'v1',
        readOnly: false,
        checkedAt: nextCollectedAt,
      }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({
        available: false,
        containers: 0,
        running: 0,
        stopped: 0,
        images: 0,
        collectedAt: nextCollectedAt,
      }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse(system.publicNetwork))
      .mockResolvedValueOnce(jsonResponse({
        schemaVersion: 1,
        categories: [],
        items: [],
        installed: 0,
        running: 0,
        updateAvailable: 0,
        collectedAt: nextCollectedAt,
      }))

    const refreshedOverview = await api.overview.get()
    expect(refreshedOverview.network).toMatchObject({
      receiveBytesPerSecond: 20,
      transmitBytesPerSecond: 20,
      rateAvailable: true,
    })
  })

  it('limits the initial file directory page while preserving offset pagination', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        path: '/home',
        entries: [],
        offset: 100,
        total: 500,
        truncated: true,
        nextOffset: 200,
        readAt: '2026-08-05T14:00:00Z',
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await api.files.list('/home', { offset: 100 })

    const requestURL = String(fetchMock.mock.calls[0]?.[0])
    expect(requestURL).toContain('/api/v1/files?')
    expect(requestURL).toContain('limit=100')
    expect(requestURL).toContain('offset=100')
  })

  it('encodes the exact file path for desktop shortcut target lookup', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({
      name: 'app config.json',
      path: '/home/app config.json',
      kind: 'file',
      sizeBytes: 12,
      mode: '-rw-------',
      owner: 'root',
      group: 'root',
      modifiedAt: '2026-08-14T00:00:00Z',
      resourceVersion: 'v1',
      editable: true,
      previewable: true,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.files.entry('/home/app config.json')

    const requestURL = String(fetchMock.mock.calls[0]?.[0])
    expect(requestURL).toContain('/api/v1/files/entry?')
    expect(requestURL).toContain('path=%2Fhome%2Fapp+config.json')
  })

  it('loads bounded desktop shortcut metadata in one POST request', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({
      entries: [{ name: 'app', path: '/home/app', kind: 'directory', resourceVersion: 'sha256:app' }],
      unavailable: ['/missing'],
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.files.entries(['/home/app', '/missing'])

    const [requestURL, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(requestURL).toContain('/api/v1/files/entries')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ paths: ['/home/app', '/missing'] })
  })

  it('creates an archive download ticket with paths, resource versions, and name', async () => {
    const ticket = {
      downloadUrl: '/api/v1/files/archive-download/test-ticket',
      expiresAt: '2026-08-20T00:05:00Z',
    }
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(ticket))
    vi.stubGlobal('fetch', fetchMock)
    const entries = [
      { path: '/home/one.txt', resourceVersion: 'sha256:one' },
      { path: '/home/logs', resourceVersion: 'sha256:logs' },
    ]

    await expect(api.files.createArchiveDownloadTicket(entries, 'home.zip')).resolves.toEqual(ticket)

    const [requestURL, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(requestURL).toBe('/api/v1/files/archive-download-tickets')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({
      sources: ['/home/one.txt', '/home/logs'],
      expectedResourceVersions: {
        '/home/one.txt': 'sha256:one',
        '/home/logs': 'sha256:logs',
      },
      name: 'home.zip',
    })
  })

  it('uses the hash-only file share contract for lookup, rotation, removal, and public metadata', async () => {
    const token = 'a'.repeat(43)
    const id = 'b'.repeat(22)
    const created = {
      id,
      createdAt: '2026-08-22T00:00:00Z',
      expiresAt: '2026-08-29T00:00:00Z',
      linksAvailable: true,
      sharePath: `/share/file/${token}`,
      directPath: `/f/${token}`,
    }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ share: null }))
      .mockResolvedValueOnce(jsonResponse(created, { status: 201 }))
      .mockResolvedValueOnce(jsonResponse({ shares: [{ ...created, path: '/home/photo.png' }] }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(jsonResponse({
        name: 'photo.png',
        mime: 'image/png',
        sizeBytes: 1024,
        expiresAt: created.expiresAt,
        directPath: created.directPath,
        downloadPath: `${created.directPath}?download=1`,
      }))
    vi.stubGlobal('fetch', fetchMock)

    await api.files.share('/home/photo.png', 'sha256:file')
    await expect(api.files.createShare({
      path: '/home/photo.png',
      expectedResourceVersion: 'sha256:file',
      expectedShareID: '',
      expiresIn: '7d',
    })).resolves.toEqual(created)
    await expect(api.files.shares()).resolves.toEqual({
      shares: [{ ...created, path: '/home/photo.png' }],
    })
    await api.files.deleteShare(id)
    await api.files.publicShare(token)

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      '/api/v1/files/shares?path=%2Fhome%2Fphoto.png&resourceVersion=sha256%3Afile',
    )
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/v1/files/shares')
    expect(fetchMock.mock.calls[1]?.[1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      path: '/home/photo.png',
      expectedResourceVersion: 'sha256:file',
      expectedShareID: '',
      expiresIn: '7d',
    })
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/v1/files/shares')
    expect((fetchMock.mock.calls[2]?.[1] as RequestInit).method).toBe('GET')
    expect(fetchMock.mock.calls[3]?.[0]).toBe(`/api/v1/files/shares/${id}`)
    expect((fetchMock.mock.calls[3]?.[1] as RequestInit).method).toBe('DELETE')
    expect(fetchMock.mock.calls[4]?.[0]).toBe(`/api/v1/public/file-shares/${token}`)
    expect((fetchMock.mock.calls[4]?.[1] as RequestInit).method).toBe('GET')
  })

  it('aborts an in-flight binary file upload through the caller signal', async () => {
    class UploadXHR {
      static latest?: UploadXHR
      upload: { onprogress?: (event: ProgressEvent) => void } = {}
      withCredentials = false
      responseType: XMLHttpRequestResponseType = ''
      response: unknown
      status = 0
      onerror?: () => void
      onabort?: () => void
      onload?: () => void
      abort = vi.fn(() => this.onabort?.())
      open = vi.fn()
      setRequestHeader = vi.fn()
      send = vi.fn()

      constructor() {
        UploadXHR.latest = this
      }
    }
    vi.stubGlobal('XMLHttpRequest', UploadXHR)
    const controller = new AbortController()
    const operation = api.files.upload('/home', new File(['hello'], 'notes.txt'), false, undefined, controller.signal)

    controller.abort()

    await expect(operation).rejects.toMatchObject({ name: 'AbortError' })
    expect(UploadXHR.latest?.abort).toHaveBeenCalledTimes(1)
  })

  it('normalizes kejilion.sh and legacy swap artifacts separately', async () => {
    const collectedAt = '2026-07-26T05:00:00Z'
    const system = {
      hostname: 'swap-host',
      os: 'Debian 13',
      architecture: 'amd64',
      uptimeSeconds: 120,
      load: { one: 0.2, five: 0.1, fifteen: 0.1 },
      cpu: { cores: 2, usagePercent: 5 },
      memory: {
        totalBytes: 8 * 1024 ** 3,
        availableBytes: 6 * 1024 ** 3,
        usedBytes: 2 * 1024 ** 3,
        usagePercent: 25,
        swapTotalBytes: 3 * 1024 ** 3,
        swapUsedBytes: 128 * 1024,
      },
      disks: [],
      network: { receivedBytes: 100, sentBytes: 200 },
      management: {
        ssh: {
          ports: [22],
          source: 'default',
          defense: {
            available: true,
            installed: true,
            running: true,
            enabled: true,
            autostart: true,
            jail: 'sshd',
            banned: 4,
            message: 'Fail2Ban SSH jail 正在防御',
          },
        },
        swap: {
          activeDevices: 3,
          path: '/swapfile',
          fileExists: true,
          fileActive: true,
          fileSizeBytes: 1024 ** 3,
          fileUsedBytes: 128 * 1024,
          legacyExists: true,
          legacyActive: true,
          legacySizeBytes: 2 * 1024 ** 3,
          otherActiveDevices: 1,
          otherSwapTotalBytes: 512 * 1024 ** 2,
          otherSwapUsedBytes: 0,
        },
      },
      collectedAt,
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(system))
      .mockResolvedValueOnce(
        jsonResponse({
          status: 'ok',
          version: '0.5.1',
          protocolVersion: 'v1alpha1',
          readOnly: false,
          checkedAt: collectedAt,
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ items: [{ id: 'system.swap.write', enabled: true }] }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({ available: false, containers: 0, running: 0, stopped: 0, images: 0, collectedAt }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
    vi.stubGlobal('fetch', fetchMock)

    const overview = await api.overview.get()

    expect(overview.management.swap).toMatchObject({
      totalBytes: 3 * 1024 ** 3,
      activeDevices: 3,
      path: '/swapfile',
      fileExists: true,
      fileActive: true,
      fileSizeBytes: 1024 ** 3,
      legacyExists: true,
      legacyActive: true,
      legacySizeBytes: 2 * 1024 ** 3,
      otherActiveDevices: 1,
      otherSwapTotalBytes: 512 * 1024 ** 2,
    })
    expect(overview.management.ssh.defense).toEqual({
      available: true,
      installed: true,
      running: true,
      enabled: true,
      autostart: true,
      jail: 'sshd',
      banned: 4,
      message: 'Fail2Ban SSH jail 正在防御',
    })
  })

  it('uses only the supported jobs limit query and normalizes job records', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      jsonResponse({
        items: [
          {
            id: 'req-123',
            action: 'docker.restart',
            origin: 'web',
            state: 'succeeded',
            progress: 100,
            stage: 'completed',
            targetKind: 'container',
            targetId: 'abc123',
            targetLabel: 'nginx',
            createdAt: '2026-07-25T10:00:00Z',
            startedAt: '2026-07-25T10:00:01Z',
            finishedAt: '2026-07-25T10:00:02Z',
          },
        ],
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await api.jobs.list({ limit: 3 })

    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/jobs?limit=3')
    expect(result).toMatchObject({
      total: 1,
      items: [
        {
          id: 'req-123',
          action: 'docker.restart',
          resourceType: 'container',
          resourceName: 'nginx',
          status: 'succeeded',
          progress: 100,
          actor: 'web',
          source: 'web',
          stages: [{ name: 'completed', status: 'succeeded' }],
        },
      ],
    })
  })

  it('sends the observed resource version with Docker lifecycle actions', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ required: false }))
      .mockResolvedValueOnce(
        jsonResponse({
          user: { id: 'user-1', username: 'admin', role: 'admin' },
          csrfToken: 'csrf-secret',
          expiresAt: '2026-07-25T12:00:00Z',
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ containerId: 'abc123def456', action: 'restart', status: 'completed' }))
    vi.stubGlobal('fetch', fetchMock)

    await api.auth.status()
    await api.docker.action('abc123def456', 'restart', `sha256:${'a'.repeat(64)}`)

    const actionInit = fetchMock.mock.calls[2]?.[1] as RequestInit
    expect(actionInit.method).toBe('POST')
    expect(new Headers(actionInit.headers).get('x-csrf-token')).toBe('csrf-secret')
    expect(JSON.parse(String(actionInit.body))).toEqual({
      resourceVersion: `sha256:${'a'.repeat(64)}`,
    })
  })

  it('uses dedicated typed endpoints for Docker environment, backups, stats, and console', async () => {
    const containerID = 'a'.repeat(64)
    const resourceVersion = `sha256:${'b'.repeat(64)}`
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({
          available: true,
          engineVersion: '28.1.1',
          containers: 1,
          images: 2,
          mirrorPreset: 'official',
          registryMirrors: [],
          ipv6Enabled: false,
          daemonConfig: 'missing',
          observedAt: '2026-07-27T12:00:00Z',
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(
        jsonResponse({
          containerId: containerID,
          cpuPercent: 1.25,
          memoryBytes: 1024,
          memoryLimitBytes: 2048,
          memoryPercent: 50,
          networkRxBytes: 10,
          networkTxBytes: 20,
          blockReadBytes: 30,
          blockWriteBytes: 40,
          pids: 2,
          collectedAt: '2026-07-27T12:00:00Z',
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          containerId: containerID,
          exitCode: 0,
          output: 'uid=0(root)',
          truncated: false,
          finishedAt: '2026-07-27T12:00:01Z',
        }),
      )
    vi.stubGlobal('fetch', fetchMock)

    await api.docker.environment()
    await api.docker.backups()
    await api.docker.stats(containerID)
    await api.docker.exec(containerID, resourceVersion, 'id')

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/docker/environment',
      '/api/v1/docker/backups',
      `/api/v1/docker/containers/${containerID}/stats`,
      `/api/v1/docker/containers/${containerID}/exec`,
    ])
    const execInit = fetchMock.mock.calls[3]?.[1] as RequestInit
    expect(execInit.method).toBe('POST')
    expect(JSON.parse(String(execInit.body))).toEqual({
      resourceVersion,
      command: 'id',
    })
  })

  it('uses the typed cluster endpoints and protects every cluster mutation with CSRF', async () => {
    const hostID = 'host id/1'
    const encodedHostID = encodeURIComponent(hostID)
    const controllerID = 'controller id/1'
    const encodedControllerID = encodeURIComponent(controllerID)
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/auth/bootstrap') return jsonResponse({ required: false })
      if (url === '/api/v1/auth/session') {
        return jsonResponse({
          user: { id: 'user-1', username: 'admin', role: 'administrator' },
          csrfToken: 'cluster-csrf-secret',
          expiresAt: '2026-07-29T12:00:00Z',
        })
      }
      if (url === '/api/v1/cluster/hosts' && (init?.method || 'GET') === 'GET') {
        return jsonResponse({
          items: [],
          total: 0,
          remoteTotal: 0,
          maxHosts: 100,
          pollIntervalSeconds: 30,
          nodeId: 'local-node',
        })
      }
      if (url === '/api/v1/cluster/controllers' && (init?.method || 'GET') === 'GET') {
        return jsonResponse([])
      }
      if (url === '/api/v1/cluster/pairing-codes/v2') {
        return jsonResponse({
          code: 'pair.secret',
          scope: 'cluster.summary.read',
          expiresAt: '2026-07-29T12:05:00Z',
        })
      }
      if (url === '/api/v1/cluster/light-enrollments') {
        return jsonResponse({
          command: "bash <(curl -fsSL https://example.com/kejilion.sh) kpanel node join 'kpl1.token'",
          expiresAt: '2026-07-29T12:05:00Z',
        })
      }
      if (url.endsWith('/refresh')) return jsonResponse({ id: hostID, polling: true })
      if (url.includes('/cluster/controllers/')) return jsonResponse({ deleted: true })
      if (url.includes('/cluster/hosts/') && init?.method === 'DELETE') {
        return jsonResponse({ deleted: true, remoteRevoked: true })
      }
      return jsonResponse({ id: hostID })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.auth.status()
    await api.cluster.hosts()
    await api.cluster.host(hostID)
    await api.cluster.add({
      name: '香港节点',
      origin: 'https://hk.example.com',
      pairingCode: 'pair.secret',
    })
    await api.cluster.rename(hostID, {
      name: '香港生产节点',
      expectedResourceVersion: 'host-version-1',
    })
    await api.cluster.remove(hostID, 'host-version-2')
    await api.cluster.refresh(hostID)
    await api.cluster.enableMutualFiles(hostID)
    await api.cluster.createPairingCode()
    await api.cluster.createLightEnrollment()
    await api.cluster.controllers()
    await api.cluster.revokeController(controllerID)

    const clusterCalls = fetchMock.mock.calls.slice(2)
    expect(clusterCalls.map(([url]) => url)).toEqual([
      '/api/v1/cluster/hosts',
      `/api/v1/cluster/hosts/${encodedHostID}`,
      '/api/v1/cluster/hosts',
      `/api/v1/cluster/hosts/${encodedHostID}`,
      `/api/v1/cluster/hosts/${encodedHostID}`,
      `/api/v1/cluster/hosts/${encodedHostID}/refresh`,
      `/api/v1/cluster/hosts/${encodedHostID}/mutual-files`,
      '/api/v1/cluster/pairing-codes/v2',
      '/api/v1/cluster/light-enrollments',
      '/api/v1/cluster/controllers',
      `/api/v1/cluster/controllers/${encodedControllerID}`,
    ])

    expect(clusterCalls.map(([, init]) => (init as RequestInit).method)).toEqual([
      'GET',
      'GET',
      'POST',
      'PATCH',
      'DELETE',
      'POST',
      'POST',
      'POST',
      'POST',
      'GET',
      'DELETE',
    ])
    expect(JSON.parse(String((clusterCalls[2]?.[1] as RequestInit).body))).toEqual({
      name: '香港节点',
      origin: 'https://hk.example.com',
      pairingCode: 'pair.secret',
    })
    expect(JSON.parse(String((clusterCalls[3]?.[1] as RequestInit).body))).toEqual({
      name: '香港生产节点',
      expectedResourceVersion: 'host-version-1',
    })
    expect(JSON.parse(String((clusterCalls[4]?.[1] as RequestInit).body))).toEqual({
      expectedResourceVersion: 'host-version-2',
    })
    expect((clusterCalls[5]?.[1] as RequestInit).body).toBeUndefined()
    expect((clusterCalls[6]?.[1] as RequestInit).body).toBeUndefined()
    expect((clusterCalls[7]?.[1] as RequestInit).body).toBeUndefined()
    expect((clusterCalls[8]?.[1] as RequestInit).body).toBeUndefined()

    const mutationCalls = clusterCalls.filter(
      ([, init]) => (init as RequestInit).method !== 'GET',
    )
    mutationCalls.forEach(([, init]) => {
      expect(new Headers((init as RequestInit).headers).get('x-csrf-token')).toBe(
        'cluster-csrf-secret',
      )
    })
  })

  it('separates authenticated share settings from the anonymous public snapshot', async () => {
    const token = 'a'.repeat(64)
    const settings = {
      enabled: true,
      title: 'My fleet',
      description: 'Public status',
      sharePath: `/share/${token}`,
      resourceVersion: 'share-v1',
    }
    const fetchMock = vi.fn(async (input: string | URL | Request, _init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/auth/bootstrap') return jsonResponse({ required: false })
      if (url === '/api/v1/auth/session') {
        return jsonResponse({
          user: { id: 'user-1', username: 'admin', role: 'administrator' },
          csrfToken: 'share-csrf-secret',
          expiresAt: '2026-08-15T13:00:00Z',
        })
      }
      if (url.startsWith('/api/v1/public/cluster-share/')) {
        return jsonResponse({
          title: settings.title, generatedAt: '2026-08-15T12:00:00Z',
          total: 0, online: 0, attention: 0, items: [],
        })
      }
      return jsonResponse(settings)
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.auth.status()
    await api.cluster.shareSettings()
    await api.cluster.updateShare({
      enabled: true,
      title: settings.title,
      description: settings.description,
      expectedResourceVersion: settings.resourceVersion,
    })
    await api.cluster.resetShareToken(settings.resourceVersion)
    await api.cluster.publicShare(token)

    const calls = fetchMock.mock.calls.slice(2)
    expect(calls.map(([url]) => url)).toEqual([
      '/api/v1/cluster/share',
      '/api/v1/cluster/share',
      '/api/v1/cluster/share/token',
      `/api/v1/public/cluster-share/${token}`,
    ])
    expect(calls.map(([, init]) => (init as RequestInit).method)).toEqual([
      'GET', 'PUT', 'POST', 'GET',
    ])
    expect(new Headers((calls[1]?.[1] as RequestInit).headers).get('x-csrf-token')).toBe('share-csrf-secret')
    expect(new Headers((calls[2]?.[1] as RequestInit).headers).get('x-csrf-token')).toBe('share-csrf-secret')
    expect(new Headers((calls[3]?.[1] as RequestInit).headers).get('x-csrf-token')).toBeNull()
  })

  it('keeps terminal output objects intact instead of treating their data field as an envelope', async () => {
    const sessionID = 'a'.repeat(64)
    const output = {
      data: 'cm9vdEBrZWppbGlvbjokIA',
      offset: 0,
      nextOffset: 19,
      truncated: false,
      closed: false,
    }
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(output))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.terminals.output(sessionID, 0)).resolves.toEqual(output)
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      `/api/v1/terminal-sessions/${sessionID}/output?offset=0&wait=1000`,
    )
  })

  it('normalizes list responses without a total field', () => {
    expect(normalizeList({ items: ['a', 'b'] } as { items: string[]; total: number })).toEqual({
      items: ['a', 'b'],
      total: 2,
    })
  })

  it('requests monitoring history with a fixed same-origin range query', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({
      range: '7d',
      startedAt: '2026-07-24T00:00:00Z',
      endedAt: '2026-07-31T00:00:00Z',
      bucketSeconds: 900,
      host: [],
      containers: [],
      storage: {
        enabled: true,
        retentionDays: 30,
        hostIntervalSeconds: 60,
        containerIntervalSeconds: 300,
        maxContainers: 32,
        storageBytes: 0,
        maxStorageBytes: 134217728,
        lastContainerTotal: 0,
        lastContainerRecorded: 0,
        lastContainerFailed: 0,
        lastContainerTruncated: 0,
        lastDockerAvailable: false,
        storageLimitReached: false,
      },
      scannedBytes: 0,
      skippedLines: 0,
      truncatedSeries: 0,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.monitoring.history('7d')).resolves.toMatchObject({ range: '7d' })
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/monitoring/history?range=7d')
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe('GET')

    fetchMock.mockResolvedValueOnce(jsonResponse({ range: '30d' }))
    await expect(api.monitoring.history('30d')).resolves.toMatchObject({ range: '30d' })
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/v1/monitoring/history?range=30d')

    fetchMock.mockResolvedValueOnce(jsonResponse({ range: '12m' }))
    await expect(api.monitoring.history('12m')).resolves.toMatchObject({ range: '12m' })
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/v1/monitoring/history?range=12m')

    fetchMock.mockResolvedValueOnce(jsonResponse({ range: '12m' }))
    await expect(api.monitoring.history('12m', {
      start: '2026-08-04T00:00:00.000Z',
      end: '2026-08-05T00:00:00.000Z',
    })).resolves.toMatchObject({ range: '12m' })
    expect(fetchMock.mock.calls[3]?.[0]).toBe(
      '/api/v1/monitoring/history?range=12m&start=2026-08-04T00%3A00%3A00.000Z&end=2026-08-05T00%3A00%3A00.000Z',
    )
  })

  it('requests the bounded process list and sends only a fixed process signal action', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [], total: 0, summary: { total: 0 } }))
      .mockResolvedValueOnce(jsonResponse({ action: 'process-signal', changed: true }))
    vi.stubGlobal('fetch', fetchMock)

    await api.system.processes({ search: 'nginx', sort: 'memory', order: 'desc', limit: 200 })
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/system/processes?q=nginx&sort=memory&order=desc&limit=200',
    )
    await api.system.action({
      action: 'process-signal',
      pid: 4321,
      startTimeTicks: 987654,
      signal: 'term',
    })
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/v1/system/actions')
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      action: 'process-signal',
      pid: 4321,
      startTimeTicks: 987654,
      signal: 'term',
    })
  })

  it('normalizes null and legacy null-item list responses', () => {
    expect(normalizeList<string>(null)).toEqual({ items: [], total: 0 })
    expect(normalizeList({ items: null } as unknown as { items: string[]; total: number })).toEqual({
      items: [],
      total: 0,
    })
  })
})

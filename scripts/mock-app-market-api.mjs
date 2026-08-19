import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const mockPort = Number.parseInt(process.env.KPANEL_MOCK_API_PORT || '8080', 10)
if (!Number.isInteger(mockPort) || mockPort < 1024 || mockPort > 65535) {
  throw new Error('KPANEL_MOCK_API_PORT must be an integer between 1024 and 65535')
}
const catalog = JSON.parse(await readFile(join(root, 'internal', 'appmarket', 'catalog.json'), 'utf8'))
const legacy = JSON.parse(await readFile(join(root, 'internal', 'appmarket', 'legacy-apps.json'), 'utf8'))
const mockNewApp = catalog.apps.find((app) => app.token === 'speedtest')
if (mockNewApp) mockNewApp.addedAt = new Date().toISOString().slice(0, 10)
const legacyByNumber = new Map(legacy.apps.map((item) => [item.num, item]))
const installed = new Map([
  ['speedtest', { state: 'running', direct: true }],
  ['it-tools', { state: 'running', direct: false }],
  ['openlist', { state: 'running', direct: true }],
  ['n8n', { state: 'exited', direct: false }],
])
const adapted = new Set(['speedtest', 'it-tools', 'dosgame'])
const appJobs = new Map()
const diagnosticJobs = new Map()
const diagnosticCatalog = {
  categories: [
    { id: 'access', name: 'IP 与解锁' },
    { id: 'network', name: '网络线路' },
    { id: 'hardware', name: '硬件性能' },
    { id: 'comprehensive', name: '综合评测' },
  ],
  items: [
    {
      id: 'chatgpt',
      category: 'access',
      name: 'ChatGPT 解锁检测',
      description: '检测当前出口 IP 的 ChatGPT 可用性',
      sourceUrl: 'https://cdn.jsdelivr.net/gh/missuo/OpenAI-Checker/openai.sh',
      estimatedMinutes: 2,
      impact: 'light',
    },
    {
      id: 'ip-quality',
      category: 'access',
      name: 'IP 质量体检',
      description: '检测 IP 风险、信誉、邮件与流媒体质量',
      sourceUrl: 'https://IP.Check.Place',
      estimatedMinutes: 8,
      impact: 'network',
    },
    {
      id: 'superspeed',
      category: 'network',
      name: 'SuperSpeed 三网测速',
      description: '执行国内三网节点带宽测试',
      sourceUrl: 'https://git.io/superspeed_uxh',
      estimatedMinutes: 15,
      impact: 'intensive',
    },
    {
      id: 'net-quality',
      category: 'network',
      name: '网络质量体检',
      description: '检测延迟、抖动、丢包和网络质量',
      sourceUrl: 'https://Net.Check.Place',
      estimatedMinutes: 10,
      impact: 'network',
    },
    {
      id: 'yabs',
      category: 'hardware',
      name: 'YABS 性能测试',
      description: '测试 CPU、磁盘与网络；无 Swap 时按脚本创建 1 GiB /swapfile',
      sourceUrl: 'https://yabs.sh',
      estimatedMinutes: 30,
      impact: 'intensive',
    },
    {
      id: 'nodequality',
      category: 'comprehensive',
      name: 'NodeQuality 综合测评',
      description: '运行 NodeQuality 节点质量综合测试',
      sourceUrl: 'https://run.NodeQuality.com',
      estimatedMinutes: 30,
      impact: 'intensive',
    },
  ],
}

const items = catalog.apps.map((app) => {
  const mapping = legacyByNumber.get(app.num) || {}
  const runtime = installed.get(app.token)
  const isAdapted = adapted.has(app.token)
  const isStandard = app.source === 'thirdparty' || mapping.usesDockerApp
  const isRunning = runtime?.state === 'running'
  const port = mapping.defaultPort || 0
  return {
    ...app,
    defaultPort: port,
    installPortConfigurable: Boolean(port && (isAdapted || mapping.usesDockerApp)),
    installer: isAdapted ? 'declarative' : isStandard ? 'kejilion' : 'guided',
    runtime: runtime
      ? {
          installed: true,
          state: runtime.state,
          status: isRunning ? 'Up 3 hours' : 'Exited (0) 20 minutes ago',
          containerId: 'a'.repeat(64),
          containerName: mapping.container || app.slug,
          image: mapping.image || `${app.slug}:latest`,
          ports: port
            ? [
                {
                  privatePort: app.token === 'it-tools' ? 80 : 8080,
                  publicPort: port,
                  ip: runtime.direct ? '0.0.0.0' : '127.0.0.1',
                  type: 'tcp',
                },
              ]
            : [],
          accessMode: runtime.direct ? 'direct' : 'domain_only',
          updateStatus: 'check_required',
          resourceVersion: `sha256:${'b'.repeat(64)}`,
          detectedBy: ['docker', 'appno'],
        }
      : {
          installed: false,
          state: 'not_installed',
          ports: [],
          accessMode: 'not_applicable',
          updateStatus: 'not_installed',
          detectedBy: [],
        },
    capabilities: {
      install: {
        enabled: (isAdapted || isStandard) && !runtime,
        reason: runtime ? '应用已安装' : isStandard ? '' : '该应用需要专属配置向导',
      },
      start: { enabled: Boolean(runtime && !isRunning), reason: '当前状态不允许启动' },
      stop: { enabled: Boolean(runtime && isRunning), reason: '当前状态不允许停止' },
      restart: { enabled: Boolean(runtime && isRunning), reason: '当前状态不允许重启' },
      check_update: { enabled: Boolean(runtime) },
      update: { enabled: Boolean(runtime && isAdapted), reason: '只允许更新配置完全匹配的声明式应用' },
      uninstall: { enabled: Boolean(runtime && isAdapted), reason: '现有应用由 kejilion.sh 管理' },
      add_domain: { enabled: Boolean(runtime && port) },
      direct_access: { enabled: Boolean(runtime && isAdapted), reason: '仅声明式适配应用支持安全切换' },
    },
  }
})

const inventory = {
  schemaVersion: 1,
  source: catalog.source,
  scriptSha256: legacy.scriptSha256,
  catalogMode: 'live',
  categories: catalog.categories,
  items,
  installed: installed.size,
  running: [...installed.values()].filter((item) => item.state === 'running').length,
  updateAvailable: 0,
  collectedAt: new Date().toISOString(),
}

function materializeJob(job) {
  const elapsed = Date.now() - job.created
  const progress = Math.min(100, Math.max(5, Math.floor(elapsed / 80)))
  return {
    id: job.id,
    appId: job.app.id,
    appName: job.app.name_zh,
    action: 'install',
    status: progress >= 100 ? 'succeeded' : 'running',
    stage: progress >= 100 ? 'completed' : progress < 30 ? 'runtime' : 'installing',
    progress,
    message: progress >= 100 ? '应用安装完成' : '正在执行 kejilion.sh 应用安装函数',
    logs: [
      'KPANEL_PROGRESS 5 正在校验端口与宿主机环境',
      progress >= 30 ? 'KPANEL_PROGRESS 30 正在执行 kejilion.sh 应用安装函数' : '',
    ].filter(Boolean),
    createdAt: new Date(job.created).toISOString(),
    startedAt: new Date(job.created + 100).toISOString(),
    ...(progress >= 100 ? { finishedAt: new Date().toISOString() } : {}),
  }
}

function materializeDiagnosticJob(job) {
  const elapsed = Date.now() - job.created
  const progress = Math.min(100, Math.max(10, Math.floor(elapsed / 80)))
  return {
    id: job.id,
    checkId: job.check.id,
    checkName: job.check.name,
    category: job.check.category,
    sourceUrl: job.check.sourceUrl,
    estimatedMinutes: job.check.estimatedMinutes,
    impact: job.check.impact,
    status: progress >= 100 ? 'succeeded' : 'running',
    stage: progress >= 100 ? 'completed' : 'running',
    progress,
    message: progress >= 100 ? '体检完成，完整跑分结果已保存在任务日志' : '第三方体检脚本正在运行，结果将持续写入日志',
    logs: [
      `KPanel 体检：${job.check.name}`,
      `来源：${job.check.sourceUrl}`,
      '',
      '正在检测系统与网络环境…',
      progress >= 40 ? 'CPU benchmark score: 8241' : '',
      progress >= 70 ? 'Disk 4k read: 118.4 MB/s' : '',
      progress >= 100 ? 'KPANEL_TEST_RESULT succeeded ' + job.check.id : '',
    ].filter((line) => line !== ''),
    createdAt: new Date(job.created).toISOString(),
    startedAt: new Date(job.created + 100).toISOString(),
    ...(progress >= 100 ? { finishedAt: new Date().toISOString() } : {}),
  }
}

const systemSummary = {
  hostname: 'kpanel-demo',
  os: 'Debian GNU/Linux 13',
  kernel: '6.12.0-amd64',
  architecture: 'amd64',
  uptimeSeconds: 864000,
  load: { one: 0.38, five: 0.42, fifteen: 0.36 },
  cpu: { model: 'AMD EPYC', cores: 4, frequencyMHz: 2445, usagePercent: 18.6 },
  memory: {
    totalBytes: 8 * 1024 ** 3,
    availableBytes: 5.1 * 1024 ** 3,
    usedBytes: 2.9 * 1024 ** 3,
    usagePercent: 36.25,
    swapTotalBytes: 2 * 1024 ** 3,
    swapUsedBytes: 128 * 1024 ** 2,
  },
  disks: [
    {
      device: '/dev/vda1',
      mountPoint: '/',
      fileSystem: 'ext4',
      totalBytes: 80 * 1024 ** 3,
      usedBytes: 27 * 1024 ** 3,
      usagePercent: 33.75,
    },
  ],
  network: {
    receivedBytes: 24 * 1024 ** 3,
    sentBytes: 11 * 1024 ** 3,
    tcpConnections: 96,
    udpConnections: 18,
  },
  publicNetwork: {
    ipv4: '203.0.113.10',
    ipv6: '2001:db8::10',
    isp: 'KPanel Visual Test',
    country: 'China',
    region: 'Shanghai',
    city: 'Shanghai',
    timezone: 'Asia/Shanghai',
  },
  management: {
    ssh: { ports: [22, 2222], source: 'configured' },
    dns: { servers: ['1.1.1.1', '2606:4700:4700::1111'], manager: 'systemd-resolved' },
    timezone: 'Asia/Shanghai',
    swap: {
      activeDevices: 1,
      path: '/swapfile',
      fileExists: true,
      fileActive: true,
      fileSizeBytes: 2 * 1024 ** 3,
      fileUsedBytes: 128 * 1024 ** 2,
      legacyExists: false,
      legacyActive: false,
      legacySizeBytes: 0,
      otherActiveDevices: 0,
      otherSwapTotalBytes: 0,
      otherSwapUsedBytes: 0,
    },
    packageManager: 'apt',
    packageSources: ['https://deb.debian.org/debian'],
    maintenance: { state: 'idle', progress: 0, rebootRequired: true },
    ipPreference: 'ipv4',
    kernelOptimization: { enabled: true, profile: '均衡优化模式', source: 'kejilion' },
    bbr: {
      supported: true,
      enabled: true,
      congestionControl: 'bbr',
      defaultQDisc: 'fq',
      available: ['reno', 'cubic', 'bbr'],
    },
  },
  collectedAt: new Date().toISOString(),
}

const systemCapabilities = [
  'hostname',
  'ssh-port',
  'dns',
  'timezone',
  'swap',
  'mirror',
  'ip-preference',
  'kernel-tuning',
  'bbr',
  'update',
  'cleanup',
  'reboot',
].map((action) => ({ id: `system.${action}.write`, enabled: true, methods: ['POST'] }))

const environmentResourceVersion = `sha256:${'f'.repeat(64)}`
const environmentSummary = {
  protocolVersion: '1',
  state: 'installed',
  profile: 'full',
  health: 'healthy',
  webRoot: '/home/web',
  diskBytes: 8.4 * 1024 ** 3,
  siteCount: 6,
  databaseCount: 4,
  certificateCount: 6,
  composeValid: true,
  nginxValid: true,
  resourceVersion: environmentResourceVersion,
  scriptVersion: '2026.07.28',
  latestBackup: 'web_20260728103000.tar.gz',
  portConflicts: [],
  components: [
    {
      name: 'nginx',
      required: true,
      exists: true,
      running: true,
      state: 'running',
      image: 'nginx:alpine',
      version: '1.31.3',
      repoDigest: 'sha256:visual-nginx',
      updateStatus: 'current',
      updateReason: '',
    },
    {
      name: 'mysql',
      required: true,
      exists: true,
      running: true,
      state: 'running',
      image: 'mysql:8.4',
      version: '8.4',
      repoDigest: 'sha256:visual-mysql',
      updateStatus: 'unknown',
      updateReason: 'Registry 暂时不可访问，无法判断最新版本',
    },
    {
      name: 'php',
      required: true,
      exists: true,
      running: true,
      state: 'running',
      image: 'php:8.4-fpm',
      version: '8.4',
      repoDigest: 'sha256:visual-php',
      updateStatus: 'available',
      updateReason: '',
    },
    {
      name: 'php74',
      required: false,
      exists: true,
      running: false,
      state: 'exited',
      image: 'php:7.4-fpm',
      version: '7.4',
      repoDigest: 'sha256:visual-php74',
      updateStatus: 'unknown',
      updateReason: '兼容容器按需启动',
    },
    {
      name: 'redis',
      required: true,
      exists: true,
      running: true,
      state: 'running',
      image: 'redis:alpine',
      version: '8.2',
      repoDigest: 'sha256:visual-redis',
      updateStatus: 'current',
      updateReason: '',
    },
  ],
  protection: { fail2ban: true, waf: true, cloudflare: false, ddos: true },
  optimization: { mode: 'standard', gzip: true, brotli: true, zstd: false },
  observedAt: new Date().toISOString(),
}

const environmentCatalog = {
  protocolVersion: '1',
  installProfiles: [
    { id: 'full', label: '完整 LDNMP' },
    { id: 'nginx', label: '仅 Nginx' },
  ],
  protectionActions: [
    'fail2ban-install',
    'fail2ban-uninstall',
    'unban-all',
    'waf-on',
    'waf-off',
    'ddos-on',
    'ddos-off',
    'cloudflare-fail2ban',
    'cloudflare-shield',
  ],
  optimizationActions: ['standard', 'high', 'gzip-on', 'gzip-off', 'brotli-on', 'brotli-off', 'zstd-on', 'zstd-off'],
  updateComponents: [
    { id: 'nginx', versions: ['latest', '1.31'] },
    { id: 'mysql', versions: ['latest', '8.4', '8.0'] },
    { id: 'php', versions: ['latest', '8.4', '8.3'] },
    { id: 'redis', versions: ['latest', '8.2'] },
    { id: 'all', versions: ['latest'] },
  ],
}

const environmentVisualJob = {
  id: 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
  action: 'backup.create',
  target: 'web',
  status: 'succeeded',
  stage: 'complete',
  progress: 100,
  message: '冷备已完成并通过 SHA-256 校验',
  createdAt: new Date(Date.now() - 180_000).toISOString(),
  startedAt: new Date(Date.now() - 178_000).toISOString(),
  finishedAt: new Date(Date.now() - 32_000).toISOString(),
}

const environmentVisualTerminal = [
  '\u001b[36mKPANEL_LDNMP_PROTOCOL 1\u001b[0m',
  'KPANEL_LDNMP_EVENT {"stage":"backup","progress":35,"message":"正在停止原先运行的组件"}',
  '\u001b[33m正在创建 /home/web 冷备归档...\u001b[0m',
  '\u001b[32mSHA-256 校验通过，原运行状态已恢复。\u001b[0m',
  'KPANEL_LDNMP_RESULT {"status":"succeeded","message":"冷备创建成功"}',
  '',
].join('\n')

function send(response, status, body) {
  const data = JSON.stringify(body)
  response.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(data),
    'Cache-Control': 'no-store',
  })
  response.end(data)
}

async function readJSON(request) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > 65_536) throw new Error('request body exceeds visual mock limit')
    chunks.push(chunk)
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}')
}

createServer(async (request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1:8080')
  if (request.method === 'GET' && url.pathname === '/api/v1/auth/bootstrap') {
    send(response, 200, { required: false })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/auth/session') {
    send(response, 200, {
      user: { id: 'visual-test', username: 'admin', displayName: 'Admin', role: 'owner' },
      csrfToken: 'visual-test-csrf',
      expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/agent/health') {
    send(response, 200, {
      status: 'ok',
      version: '0.16.0',
      protocolVersion: 'v1',
      readOnly: false,
      checkedAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/apps') {
    send(response, 200, inventory)
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/app-jobs') {
    send(response, 200, { items: [...appJobs.values()].map(materializeJob) })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/diagnostics') {
    send(response, 200, diagnosticCatalog)
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/diagnostic-jobs') {
    send(response, 200, { items: [...diagnosticJobs.values()].map(materializeDiagnosticJob) })
    return
  }
  const diagnosticJobMatch = url.pathname.match(/^\/api\/v1\/diagnostic-jobs\/([a-f0-9]{32})$/)
  if (request.method === 'GET' && diagnosticJobMatch) {
    const job = diagnosticJobs.get(diagnosticJobMatch[1])
    send(response, job ? 200 : 404, job ? materializeDiagnosticJob(job) : { title: '任务不存在' })
    return
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/diagnostic-jobs') {
    const id = `${Date.now().toString(16).padStart(16, '0')}${'d'.repeat(16)}`.slice(-32)
    const job = { id, check: diagnosticCatalog.items[4], created: Date.now() }
    diagnosticJobs.set(id, job)
    send(response, 202, materializeDiagnosticJob(job))
    return
  }
  const appJobMatch = url.pathname.match(/^\/api\/v1\/app-jobs\/([a-f0-9]{32})$/)
  if (request.method === 'GET' && appJobMatch) {
    const job = appJobs.get(appJobMatch[1])
    send(response, job ? 200 : 404, job ? materializeJob(job) : { title: '任务不存在' })
    return
  }
  const appInstallMatch = url.pathname.match(/^\/api\/v1\/apps\/([^/]+)\/install$/)
  const appInstallPortMatch = url.pathname.match(/^\/api\/v1\/apps\/([^/]+)\/install-port$/)
  if (request.method === 'GET' && appInstallPortMatch) {
    const port = Number(url.searchParams.get('port'))
    send(response, 200, {
      port,
      available: port !== 8080,
      conflicts:
        port === 8080
          ? [{ source: 'listener', protocol: 'tcp' }]
          : [],
      checkedAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'POST' && appInstallMatch) {
    const app = items.find((item) => item.id === appInstallMatch[1])
    if (!app) {
      send(response, 404, { title: '应用不存在' })
      return
    }
    const id = `${Date.now().toString(16).padStart(16, '0')}${'a'.repeat(16)}`.slice(-32)
    const job = { id, app, created: Date.now() }
    appJobs.set(id, job)
    send(response, 202, materializeJob(job))
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/system/summary') {
    send(response, 200, systemSummary)
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/sites') {
    send(response, 200, {
      items: [
        {
          id: 'c'.repeat(32),
          primaryDomain: 'tools.example.com',
          domains: ['tools.example.com'],
          kind: 'reverse_proxy',
          enabled: true,
          health: 'healthy',
          consistency: 'in_sync',
          origin: 'web',
          target: 'http://127.0.0.1:8064',
          resourceVersion: `sha256:${'d'.repeat(64)}`,
          allowedActions: ['update'],
        },
      ],
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/web-environment') {
    send(response, 200, { ...environmentSummary, observedAt: new Date().toISOString() })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/web-environment/catalog') {
    send(response, 200, environmentCatalog)
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/web-environment/backups') {
    send(response, 200, {
      items: [
        {
          id: 'web_20260728103000.tar.gz',
          sizeBytes: 3.2 * 1024 ** 3,
          createdAt: new Date(Date.now() - 4_200_000).toISOString(),
          verified: true,
          format: 'kejilion-ldnmp-v1',
        },
        {
          id: 'web_20260724181500.tar.gz',
          sizeBytes: 3.1 * 1024 ** 3,
          createdAt: new Date(Date.now() - 345_600_000).toISOString(),
          verified: false,
          format: 'legacy',
        },
      ],
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/web-environment/jobs') {
    send(response, 200, { items: [environmentVisualJob] })
    return
  }
  if (
    request.method === 'GET' &&
    url.pathname === `/api/v1/web-environment/jobs/${environmentVisualJob.id}`
  ) {
    send(response, 200, environmentVisualJob)
    return
  }
  if (
    request.method === 'GET' &&
    url.pathname === `/api/v1/web-environment/jobs/${environmentVisualJob.id}/terminal`
  ) {
    const offset = Math.max(0, Number(url.searchParams.get('offset') || 0))
    const data = Buffer.from(environmentVisualTerminal).subarray(offset)
    send(response, 200, {
      dataBase64: data.toString('base64'),
      nextOffset: offset + data.length,
      finished: true,
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/capabilities') {
    send(response, 200, {
      items: [
        { id: 'apps.install', enabled: true, methods: ['POST'] },
        { id: 'diagnostics.run', enabled: true, methods: ['GET', 'POST'] },
        { id: 'web.environment.read', enabled: true, methods: ['GET'] },
        { id: 'web.environment.install', enabled: true, methods: ['POST'] },
        { id: 'web.environment.protection.write', enabled: true, methods: ['POST'] },
        { id: 'web.environment.optimization.write', enabled: true, methods: ['POST'] },
        { id: 'web.environment.update', enabled: true, methods: ['POST'] },
        { id: 'web.environment.backup', enabled: true, methods: ['GET', 'POST'] },
        { id: 'web.environment.restore', enabled: true, methods: ['POST'] },
        { id: 'web.environment.uninstall', enabled: true, methods: ['POST'] },
        ...systemCapabilities,
        { id: 'system.reinstall', enabled: false, reason: '需要带外控制台' },
      ],
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/docker/summary') {
    send(response, 200, {
      available: true,
      serverVersion: '28.3.2',
      containers: 5,
      running: 4,
      paused: 0,
      stopped: 1,
      images: 12,
      collectedAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/docker/containers') {
    send(response, 200, { items: [] })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/system/processes') {
    const processes = [
      ['paneld', 1824, 'kpanel', 13.8, 96_468_992, 18, 'R'],
      ['postgres', 948, 'postgres', 8.6, 334_102_528, 24, 'S'],
      ['nginx', 1271, 'www-data', 4.2, 28_921_856, 6, 'S'],
      ['dockerd', 721, 'root', 2.9, 151_519_232, 31, 'S'],
      ['redis-server', 1106, 'redis', 1.7, 41_943_040, 5, 'S'],
      ['node', 2038, 'deploy', 1.1, 187_695_104, 12, 'S'],
      ['systemd-journal', 312, 'root', 0.4, 50_331_648, 1, 'S'],
      ['sshd', 867, 'root', 0.2, 12_582_912, 1, 'S'],
      ['containerd', 655, 'root', 0.1, 73_400_320, 14, 'S'],
      ['bash', 2270, 'admin', 0, 5_242_880, 1, 'S'],
    ].map(([name, pid, user, cpuPercent, memoryBytes, threads, state]) => ({
      name, pid, user, cpuPercent, memoryBytes, threads, state,
      parentPid: pid === 1824 ? 1 : 1824,
      userId: user === 'root' ? 0 : 1000,
      nice: 0,
      startTimeTicks: Number(pid) * 1000 + 42,
    }))
    const query = (url.searchParams.get('q') || '').toLowerCase()
    const items = processes.filter((item) =>
      !query || item.name.toLowerCase().includes(query) || item.user.toLowerCase().includes(query) || String(item.pid).includes(query))
    send(response, 200, {
      items,
      total: items.length,
      summary: {
        cpuPercent: 33.6,
        memoryUsedBytes: 2_630_615_040,
        memoryTotalBytes: 8_589_934_592,
        total: processes.length,
        running: 1,
        sleeping: 9,
        stopped: 0,
        zombie: 0,
      },
      scanned: processes.length,
      truncated: false,
      sampleDuration: 302_000_000,
      collectedAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/system/actions') {
    const input = await readJSON(request)
    const isProcessSignal = input.action === 'process-signal'
    send(response, 200, {
      action: input.action || 'reboot',
      status: isProcessSignal ? 'completed' : 'accepted',
      changed: true,
      message: isProcessSignal
        ? `视觉测试：已向 PID ${input.pid} 发送 SIG${String(input.signal || '').toUpperCase()}`
        : '视觉测试：重启任务已模拟排队',
      appliedAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'POST' || request.method === 'DELETE') {
    send(response, 200, {
      action: url.pathname.split('/').at(-1),
      status: 'completed',
      resourceVersion: `sha256:${'e'.repeat(64)}`,
    })
    return
  }
  send(response, 404, { title: 'Not found', status: 404, code: 'not_found' })
}).listen(mockPort, '127.0.0.1', () => {
  process.stdout.write(`KPanel visual mock API: http://127.0.0.1:${mockPort}\n`)
})

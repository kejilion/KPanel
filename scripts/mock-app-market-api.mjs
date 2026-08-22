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
let domainSiteDeleted = false
const mockSharedImage = await readFile(join(root, 'web', 'public', 'wallpapers', 'kpanel-desktop.webp'))
const mockFileVersion = `sha256:${'a'.repeat(64)}`
const mockFiles = [
  {
    name: 'kpanel-desktop.webp', path: '/kpanel-desktop.webp', kind: 'file', mime: 'image/webp',
    sizeBytes: mockSharedImage.length, mode: '-rw-r--r--', owner: 'root', group: 'root',
    modifiedAt: '2026-08-20T08:30:00Z', resourceVersion: mockFileVersion,
    editable: false, previewable: true,
  },
  {
    name: 'release-notes.md', path: '/release-notes.md', kind: 'file', mime: 'text/markdown',
    sizeBytes: 2846, mode: '-rw-r--r--', owner: 'deploy', group: 'deploy',
    modifiedAt: '2026-08-21T12:10:00Z', resourceVersion: `sha256:${'b'.repeat(64)}`,
    editable: true, previewable: true,
  },
  {
    name: 'backups', path: '/backups', kind: 'directory', sizeBytes: 4096,
    mode: 'drwxr-xr-x', owner: 'root', group: 'root', modifiedAt: '2026-08-19T03:20:00Z',
    resourceVersion: `sha256:${'c'.repeat(64)}`, editable: false, previewable: false,
  },
]
let mockFileShareCounter = 0
let mockFileShares = [
  {
    id: 'o'.repeat(22), path: '/archive/retired-release.iso', createdAt: '2026-08-01T09:00:00Z',
    linksAvailable: false,
  },
]
const mockShareTokens = new Map()
const diagnosticCatalog = {
  categories: [
    { id: 'core', name: 'KPanel 核心体检' },
    { id: 'access', name: 'IP 与解锁' },
    { id: 'network', name: '网络线路' },
    { id: 'hardware', name: '硬件性能' },
    { id: 'comprehensive', name: '综合评测' },
  ],
  items: [
    {
      id: 'native-comprehensive',
      category: 'core',
      name: 'KPanel 核心综合体检',
      description: '由 KPanel 原生探针完成 CPU、内存、硬盘、路由、延迟、测速和 IP 基础质量检测',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 3,
      impact: 'network',
    },
    {
      id: 'native-cpu',
      category: 'core',
      name: 'CPU 原生跑分',
      description: '固定时长本地运算，回显实际 CPU 吞吐',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'light',
    },
    {
      id: 'native-memory',
      category: 'core',
      name: '内存原生跑分',
      description: '受控内存复制吞吐测试',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'light',
    },
    {
      id: 'native-disk',
      category: 'core',
      name: '硬盘原生跑分',
      description: '受控临时文件顺序读写测试',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'intensive',
    },
    {
      id: 'native-route',
      category: 'core',
      name: '出口路由基础检测',
      description: '回显出口 ASN、地区和边缘节点',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'network',
    },
    {
      id: 'native-latency',
      category: 'core',
      name: '延迟原生检测',
      description: '多探测点回显平均延迟、抖动和失败率',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'network',
    },
    {
      id: 'native-speed',
      category: 'core',
      name: '网速原生测速',
      description: '固定体积下载与上传吞吐测试',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'network',
    },
    {
      id: 'native-ip-quality',
      category: 'core',
      name: 'IP 基础质量检测',
      description: '回显公网 IP、ASN、IPv4/IPv6 和反向解析',
      sourceUrl: '',
      provider: 'native',
      estimatedMinutes: 1,
      impact: 'network',
    },
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
  const native = job.check.provider === 'native'
  return {
    id: job.id,
    checkId: job.check.id,
    checkName: job.check.name,
    category: job.check.category,
    sourceUrl: job.check.sourceUrl,
    ...(job.check.provider ? { provider: job.check.provider } : {}),
    estimatedMinutes: job.check.estimatedMinutes,
    impact: job.check.impact,
    status: progress >= 100 ? 'succeeded' : 'running',
    stage: progress >= 100 ? 'completed' : 'running',
    progress,
    message: progress >= 100
      ? (native ? '原生体检完成，真实结果已汇总' : '体检完成，完整跑分结果已保存在任务日志')
      : (native ? 'KPanel 原生探针正在运行，结果将持续回显' : '第三方体检脚本正在运行，结果将持续写入日志'),
    logs: [
      `KPanel 体检：${job.check.name}`,
      ...(native ? ['检测引擎：KPanel Native Diagnostics v1'] : [`来源：${job.check.sourceUrl}`]),
      '',
      native ? '正在执行原生探针…' : '正在检测系统与网络环境…',
      progress >= 20 && native ? 'CPU：AMD EPYC 7B12 · 原生运算吞吐：1842 K ops/s' : '',
      progress >= 35 && native ? '内存复制吞吐：18.42 GiB/s · 测试块：32.00 MiB' : '',
      progress >= 50 && native ? '顺序写入：486.00 MiB/s · 顺序读取：1.12 GiB/s' : '',
      progress >= 65 && native ? '延迟：平均 38.24 ms · 抖动 7.81 ms · 丢包 0.0%' : '',
      progress >= 80 && native ? '测速：↓ 682.40 Mbps · ↑ 94.70 Mbps' : '',
      progress >= 90 && native ? '公网 IP：203.0.113.10 · ASN：AS64500 · IPv4 已连接 · IPv6 已连接' : '',
      progress >= 40 && !native ? 'CPU benchmark score: 8241' : '',
      progress >= 70 && !native ? 'Disk 4k read: 118.4 MB/s' : '',
      progress >= 100 && !native ? 'KPANEL_TEST_RESULT succeeded ' + job.check.id : '',
    ].filter((line) => line !== ''),
    ...(progress >= 100 && native
      ? {
          summary: {
            parser: 'kpanel-native-v1',
            dimensions: {
              performance: { metrics: [
                { key: 'cpu_model', value: 'AMD EPYC 7B12' },
                { key: 'cpu_cores', value: '4' },
                { key: 'cpu_score', value: '1842 KPS' },
                { key: 'memory_score', value: '18.42 GiB/s' },
                { key: 'disk_write', value: '486.00 MiB/s' },
                { key: 'disk_read', value: '1.12 GiB/s' },
              ] },
              route: { metrics: [{ key: 'path', value: 'AS64500 · SIN' }, { key: 'edge', value: 'SIN' }] },
              latency: { metrics: [
                { key: 'average', value: '38.24 ms' },
                { key: 'jitter', value: '7.81 ms' },
                { key: 'loss', value: '0.0%' },
              ] },
              speed: { metrics: [{ key: 'download', value: '682.40 Mbps' }, { key: 'upload', value: '94.70 Mbps' }] },
              ip: { metrics: [
                { key: 'public_ip', value: '203.0.113.10' },
                { key: 'asn', value: 'AS64500' },
                { key: 'country', value: 'CN' },
                { key: 'ipv4_ipv6', value: 'IPv4 已连接 · IPv6 已连接' },
                { key: 'quality', value: '基础信息已采集；未接入第三方信誉库' },
              ] },
            },
          },
        }
      : {}),
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

let desktopWorkspaceRevision = 1
let desktopWorkspace = {
  schemaVersion: 3,
  resourceVersion: `sha256:${'1'.repeat(64)}`,
  available: true,
  hiddenEntryKeys: [],
  hiddenWidgetKeys: [],
  positions: {},
  widgetPositions: {
    'widget:clock': { x: 0.82, y: 0 },
    'widget:monitor': { x: 0.82, y: 0.32 },
  },
  labels: {},
  shortcuts: [],
}

function commitDesktopWorkspace(input) {
  desktopWorkspaceRevision += 1
  desktopWorkspace = {
    schemaVersion: 3,
    resourceVersion: `sha256:${String(desktopWorkspaceRevision).padStart(64, '0')}`,
    available: true,
    hiddenEntryKeys: Array.isArray(input.hiddenEntryKeys) ? input.hiddenEntryKeys : [],
    hiddenWidgetKeys: Array.isArray(input.hiddenWidgetKeys) ? input.hiddenWidgetKeys : [],
    positions: input.positions && typeof input.positions === 'object' ? input.positions : {},
    widgetPositions: input.widgetPositions && typeof input.widgetPositions === 'object' ? input.widgetPositions : {},
    labels: input.labels && typeof input.labels === 'object' ? input.labels : {},
    shortcuts: Array.isArray(input.shortcuts) ? input.shortcuts : [],
  }
  return desktopWorkspace
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

function sendBinary(response, status, body, contentType, disposition = 'inline') {
  response.writeHead(status, {
    'Content-Type': contentType,
    'Content-Length': body.length,
    'Content-Disposition': disposition,
    'Cache-Control': 'public, max-age=0, must-revalidate',
    'Cross-Origin-Resource-Policy': 'cross-origin',
    'X-Content-Type-Options': 'nosniff',
  })
  response.end(body)
}

function fileShareAdminView(record, token = '') {
  return {
    id: record.id,
    ...(record.path ? { path: record.path } : {}),
    createdAt: record.createdAt,
    ...(record.expiresAt ? { expiresAt: record.expiresAt } : {}),
    linksAvailable: Boolean(token),
    ...(token ? { sharePath: `/share/file/${token}`, directPath: `/f/${token}` } : {}),
  }
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
  if (request.method === 'GET' && url.pathname === '/api/v1/desktop/workspace') {
    send(response, 200, desktopWorkspace)
    return
  }
  if (request.method === 'PUT' && url.pathname === '/api/v1/desktop/workspace') {
    const input = await readJSON(request)
    if (input.expectedResourceVersion !== desktopWorkspace.resourceVersion) {
      send(response, 409, { title: 'Desktop workspace changed', code: 'desktop_workspace_changed' })
      return
    }
    send(response, 200, commitDesktopWorkspace(input))
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
  if (request.method === 'GET' && url.pathname === '/api/v1/files') {
    const requestedPath = url.searchParams.get('path') || '/'
    const search = (url.searchParams.get('search') || '').trim().toLowerCase()
    const entries = requestedPath === '/'
      ? mockFiles.filter((entry) => !search || entry.name.toLowerCase().includes(search))
      : []
    send(response, 200, {
      path: requestedPath, entries, offset: 0, total: entries.length, totalKnown: true,
      truncated: false, scanTruncated: false, readAt: new Date().toISOString(),
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/entry') {
    const entry = mockFiles.find((item) => item.path === url.searchParams.get('path'))
    send(response, entry ? 200 : 404, entry || { title: '文件不存在', status: 404, code: 'not_found' })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/content') {
    if (url.searchParams.get('path') === '/kpanel-desktop.webp') {
      sendBinary(response, 200, mockSharedImage, 'image/webp', 'inline; filename="kpanel-desktop.webp"')
    } else {
      send(response, 404, { title: '文件不存在', status: 404, code: 'not_found' })
    }
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/files/shares') {
    const filePath = url.searchParams.get('path')
    if (filePath) {
      const record = mockFileShares.find((item) => item.path === filePath)
      send(response, 200, { share: record ? fileShareAdminView(record) : null })
    } else {
      send(response, 200, { shares: mockFileShares.map((record) => fileShareAdminView(record)) })
    }
    return
  }
  if (request.method === 'POST' && url.pathname === '/api/v1/files/shares') {
    const input = await readJSON(request)
    const entry = mockFiles.find((item) => item.path === input.path && item.kind === 'file')
    if (!entry || entry.resourceVersion !== input.expectedResourceVersion) {
      send(response, 409, { title: '文件已发生变化，请刷新后重试', status: 409, code: 'file_share_changed' })
      return
    }
    mockFileShareCounter += 1
    const id = mockFileShareCounter.toString(36).padStart(22, 's').slice(-22)
    const token = mockFileShareCounter.toString(36).padStart(43, 'v').slice(-43)
    const createdAt = new Date().toISOString()
    const expiresAt = input.expiresIn === 'never'
      ? undefined
      : new Date(Date.now() + (input.expiresIn === '30d' ? 30 : 7) * 86_400_000).toISOString()
    for (const [existingToken, record] of mockShareTokens) {
      if (record.path === input.path) mockShareTokens.delete(existingToken)
    }
    const record = { id, path: input.path, createdAt, expiresAt, linksAvailable: false }
    mockFileShares = [...mockFileShares.filter((item) => item.path !== input.path), record]
    mockShareTokens.set(token, record)
    send(response, 201, fileShareAdminView(record, token))
    return
  }
  const fileShareDeleteMatch = url.pathname.match(/^\/api\/v1\/files\/shares\/([A-Za-z0-9_-]{22})$/)
  if (request.method === 'DELETE' && fileShareDeleteMatch) {
    const index = mockFileShares.findIndex((item) => item.id === fileShareDeleteMatch[1])
    if (index < 0) {
      send(response, 404, { title: '分享不存在', status: 404, code: 'not_found' })
      return
    }
    const [deleted] = mockFileShares.splice(index, 1)
    for (const [token, record] of mockShareTokens) {
      if (record.id === deleted.id) mockShareTokens.delete(token)
    }
    response.writeHead(204, { 'Cache-Control': 'no-store' })
    response.end()
    return
  }
  const publicFileShareMatch = url.pathname.match(/^\/api\/v1\/public\/file-shares\/([A-Za-z0-9_-]{43})$/)
  if (request.method === 'GET' && publicFileShareMatch) {
    const record = mockShareTokens.get(publicFileShareMatch[1])
    if (!record) {
      send(response, 404, { title: 'Not found', status: 404, code: 'not_found' })
      return
    }
    send(response, 200, {
      name: 'kpanel-desktop.webp', mime: 'image/webp', sizeBytes: mockSharedImage.length,
      ...(record.expiresAt ? { expiresAt: record.expiresAt } : {}),
      directPath: `/f/${publicFileShareMatch[1]}`,
      downloadPath: `/f/${publicFileShareMatch[1]}?download=1`,
    })
    return
  }
  const publicFileContentMatch = url.pathname.match(/^\/f\/([A-Za-z0-9_-]{43})$/)
  if ((request.method === 'GET' || request.method === 'HEAD') && publicFileContentMatch) {
    if (!mockShareTokens.has(publicFileContentMatch[1])) {
      send(response, 404, { title: 'Not found', status: 404, code: 'not_found' })
      return
    }
    if (request.method === 'HEAD') {
      response.writeHead(200, {
        'Content-Type': 'image/webp', 'Content-Length': mockSharedImage.length,
        'Content-Disposition': url.searchParams.get('download') === '1'
          ? 'attachment; filename="kpanel-desktop.webp"'
          : 'inline; filename="kpanel-desktop.webp"',
        'Cache-Control': 'public, max-age=0, must-revalidate',
        'Cross-Origin-Resource-Policy': 'cross-origin',
      })
      response.end()
      return
    }
    sendBinary(
      response, 200, mockSharedImage, 'image/webp',
      url.searchParams.get('download') === '1'
        ? 'attachment; filename="kpanel-desktop.webp"'
        : 'inline; filename="kpanel-desktop.webp"',
    )
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
    const job = { id, check: diagnosticCatalog.items.find((item) => item.id === 'native-comprehensive'), created: Date.now() }
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
      items: domainSiteDeleted ? [] : [
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
          allowedActions: ['update', 'delete'],
        },
      ],
    })
    return
  }
  if (request.method === 'DELETE' && url.pathname === `/api/v1/sites/${'c'.repeat(32)}`) {
    const input = await readJSON(request)
    if (input.primaryDomain !== 'tools.example.com' || Object.keys(input).length !== 1) {
      send(response, 422, { title: '站点域名无效', code: 'validation_failed' })
      return
    }
    domainSiteDeleted = true
    send(response, 200, {
      id: 'c'.repeat(32),
      primaryDomain: 'tools.example.com',
      status: 'deleted',
      mode: 'full',
      resourceVersion: `sha256:${'d'.repeat(64)}`,
      removed: ['nginx_config'],
      databaseDropped: false,
    })
    return
  }
  if (request.method === 'GET' && url.pathname === '/api/v1/cluster/hosts') {
    send(response, 200, {
      items: [
        {
          id: 'e'.repeat(32),
          isLocal: true,
          name: 'kpanel-demo',
          kind: 'panel',
          origin: 'https://panel.example.com',
          transportSecurity: 'tls',
          remoteNodeId: 'local-node',
          federationProtocol: 'v1',
          scope: 'cluster.summary.read',
          terminalAvailable: true,
          mutualFileTransferAvailable: false,
          state: 'online',
          consecutiveFailures: 0,
          polling: true,
          resourceVersion: `sha256:${'f'.repeat(64)}`,
          createdAt: new Date(Date.now() - 86_400_000).toISOString(),
          updatedAt: new Date().toISOString(),
        },
        {
          id: 'a'.repeat(32),
          isLocal: false,
          name: 'edge-melbourne',
          kind: 'light_node',
          origin: 'https://edge.example.com',
          transportSecurity: 'tls',
          remoteNodeId: 'remote-node',
          federationProtocol: 'v1',
          scope: 'cluster.summary.read',
          terminalAvailable: false,
          mutualFileTransferAvailable: false,
          state: 'degraded',
          consecutiveFailures: 1,
          lastError: '延迟偏高，保留上次成功快照',
          polling: true,
          resourceVersion: `sha256:${'a'.repeat(64)}`,
          createdAt: new Date(Date.now() - 3_600_000).toISOString(),
          updatedAt: new Date().toISOString(),
        },
      ],
      total: 2,
      remoteTotal: 1,
      maxHosts: 16,
      pollIntervalSeconds: 30,
      nodeId: 'local-node',
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
        { id: 'sites.delete', enabled: true, methods: ['DELETE'] },
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

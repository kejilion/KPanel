<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type Component } from 'vue'
import { RouterLink } from 'vue-router'
import { rememberOverviewViewport, restoreOverviewViewport } from '@/lib/overviewViewport'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/OverviewView/en-US').then((module) => module.default)
  : import('@/i18n/pages/OverviewView/zh-TW').then((module) => module.default))
import {
  Activity,
  ArrowLeftRight,
  ArrowDownToLine,
  ArrowUpFromLine,
  Bolt,
  Box,
  Boxes,
  ChevronRight,
  CircleAlert,
  Clock3,
  Cpu,
  Database,
  Gauge,
  Globe2,
  HardDrive,
  KeyRound,
  MemoryStick,
  Network,
  ListTree,
  Pencil,
  Power,
  RefreshCw,
  RefreshCcw,
  Server,
  Settings2,
  ShieldCheck,
  Timer,
  Wrench,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import MetricCard from '@/components/overview/MetricCard.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import CronManagerDialog from '@/components/overview/CronManagerDialog.vue'
import FirewallManagerDialog from '@/components/overview/FirewallManagerDialog.vue'
import HostsManagerDialog from '@/components/overview/HostsManagerDialog.vue'
import NetworkInterfacesDialog from '@/components/overview/NetworkInterfacesDialog.vue'
import PortUsageDialog from '@/components/overview/PortUsageDialog.vue'
import TrafficShutdownDialog from '@/components/overview/TrafficShutdownDialog.vue'
import AccountManagementDialog from '@/components/overview/AccountManagementDialog.vue'
import SSHDefenseDialog from '@/components/overview/SSHDefenseDialog.vue'
import SystemTuningDialog from '@/components/overview/SystemTuningDialog.vue'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import {
  clampPercent,
  formatBytes,
  formatDateTime,
  formatDuration,
  formatHostDateTime,
  formatPercent,
  formatRate,
} from '@/lib/format'
import {
  customPreset,
  detectDNSPreset,
  detectMirrorPreset,
  dnsServersForPreset,
  dnsPresets,
  parseDNSServers,
  timezonePresets,
  type MirrorPreset,
} from '@/lib/systemPresets'
import { usePanelState } from '@/stores/panel'
import { useToast } from '@/stores/toast'
import type { SystemActionInput, SystemOverview } from '@/types/api'

const props = withDefaults(defineProps<{
  systemCenterOnly?: boolean
}>(), {
  systemCenterOnly: false,
})

const data = ref<SystemOverview>()
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const waitingForNextSample = '等待下一次采样'
const panel = usePanelState()
const toast = useToast()
const processManagerLabel = '进程管理器'
const windowActive = inject(desktopWindowActiveKey, computed(() => true))
let controller: AbortController | undefined
let refreshTimer: number | undefined
let maintenanceController: AbortController | undefined
let maintenanceRefreshTimer: number | undefined
let refreshActive = false

interface ManagementTool {
  id: string
  title: string
  description: string
  value: string
  detail: string
  capability: string
  safety: string
  icon: Component
  tone?: 'blue' | 'violet' | 'amber' | 'danger'
  recommended?: boolean
}

interface SystemCenterSection {
  id: 'maintenance' | 'basic' | 'security' | 'network' | 'performance'
  title: string
  description: string
  icon: Component
  iconTone?: 'blue' | 'violet' | 'amber'
  tools: ManagementTool[]
}

type KernelProfile = 'high' | 'balanced' | 'web' | 'stream' | 'game' | 'off'
type BBRv3Policy = 'install' | 'update' | 'uninstall'
type ResourceDialogID = 'hosts' | 'cron' | 'network-interfaces' | 'firewall' | 'port-usage' | 'traffic-shutdown' | 'accounts' | 'ssh-defense' | 'system-tuning'

const mirrorPresets: Array<{
  value: MirrorPreset
  title: string
  route: string
  description: string
}> = [
  {
    value: 'cn-default',
    title: '中国大陆',
    route: '阿里云',
    description: '对应脚本“中国大陆【默认】”，选择 LinuxMirrors 默认列表首选线路。',
  },
  {
    value: 'cn-edu',
    title: '中国教育网',
    route: '北京大学',
    description: '对应脚本“中国大陆【教育网】”，适合教育网或校园网络。',
  },
  {
    value: 'abroad',
    title: '海外地区',
    route: 'xTom 香港',
    description: '对应脚本“海外地区”，使用 LinuxMirrors 海外线路。',
  },
  {
    value: 'smart',
    title: '智能切换',
    route: '自动判断',
    description: 'CN 使用华为云；其他地区使用发行版官方源，与 kejilion.sh 智能逻辑一致。',
  },
]

const selectedTool = ref<ManagementTool>()
const selectedResourceDialog = ref<ResourceDialogID>()
const actionRunning = ref(false)
const actionForm = reactive({
  hostname: '',
  port: 2222,
  dns: '',
  dnsPreset: customPreset,
  timezone: 'Asia/Shanghai',
  timezonePreset: 'Asia/Shanghai',
  swapPreset: '1024' as '0' | '1024' | '2048' | '4096' | 'custom',
  swapSizeMiB: 1024,
  mirrorPreset: 'smart' as MirrorPreset,
  preference: 'ipv4' as 'ipv4' | 'system_default',
  profile: 'balanced' as KernelProfile,
  bbrEnabled: true,
  bbrv3Policy: 'install' as BBRv3Policy,
  maintenancePolicy: 'full' as 'full' | 'cache' | 'standard',
})

watch(
  () => actionForm.dns,
  (value) => {
    actionForm.dnsPreset = detectDNSPreset(value)
  },
)

const loadPercent = computed(() => {
  const cores = Number(data.value?.load.unit || 1)
  return clampPercent(((data.value?.load.value || 0) / Math.max(cores, 1)) * 100)
})

const agentLabel = computed(() => {
  const agent = data.value?.agent
  if (!agent?.connected) return { status: 'offline', label: 'Agent 离线' }
  if (!agent.compatible) return { status: 'incompatible', label: '版本不兼容' }
  if (agent.readOnly) return { status: 'read_only', label: '写入依赖未就绪' }
  return { status: 'connected', label: '运行正常' }
})

const cpuFrequencyLabel = computed(() => {
  const frequencyMHz = data.value?.cpu.frequencyMHz
  if (!frequencyMHz || !Number.isFinite(frequencyMHz)) return '未识别'
  return frequencyMHz >= 1000 ? `${(frequencyMHz / 1000).toFixed(2)} GHz` : `${frequencyMHz.toFixed(0)} MHz`
})

const publicLocation = computed(() => {
  const network = data.value?.publicNetwork
  return [network?.country, network?.region, network?.city].filter(Boolean).join(' · ') || '未获取'
})

const hasPublicIPv6 = computed(() => Boolean(data.value?.publicNetwork.ipv6?.includes(':')))

watch(hasPublicIPv6, (available) => {
  if (available && selectedTool.value?.id === 'dns' && actionForm.dnsPreset !== customPreset) {
    applyDNSPreset()
  }
})

const publicCountryCode = computed(() => {
  const network = data.value?.publicNetwork
  const code = (network?.countryCode || (network?.country?.length === 2 ? network.country : '')).toUpperCase()
  if (!/^[A-Z]{2}$/.test(code)) return ''
  return code
})

const osIdentity = computed(() => detectOperatingSystemIdentity(data.value))

const networkAlgorithm = computed(() => {
  const bbr = data.value?.management.bbr
  return [bbr?.congestionControl, bbr?.defaultQDisc].filter(Boolean).join(' · ') || '未识别'
})

const hostObservedTime = computed(() =>
  formatHostDateTime(data.value?.observedAt, data.value?.management.timezone),
)

const basicSettings = computed<ManagementTool[]>(() => {
  if (!data.value) return []
  const management = data.value.management
  const swapPercent = management.swap.totalBytes
    ? (management.swap.usedBytes / management.swap.totalBytes) * 100
    : 0
  const swapDetails = [
    management.swap.fileActive
      ? `/swapfile ${formatBytes(management.swap.fileSizeBytes)} 已启用`
      : management.swap.fileExists
        ? `/swapfile ${formatBytes(management.swap.fileSizeBytes)} 未启用`
        : '/swapfile 尚未创建',
  ]
  if (management.swap.legacyExists || management.swap.legacyActive) {
    swapDetails.push(`待合并旧版 KPanel Swap ${formatBytes(management.swap.legacySizeBytes)}`)
  }
  if (management.swap.otherActiveDevices) {
    swapDetails.push(`保留 ${management.swap.otherActiveDevices} 个其他 Swap`)
  }
  return [
    {
      id: 'hostname',
      title: '主机名',
      description: '对应 kejilion.sh 的“修改主机名”。',
      value: data.value.hostname || '未命名主机',
      detail: '同步识别系统当前 hostname',
      capability: 'system.hostname.write',
      safety: '写入前校验 Linux hostname 规则，并原子更新 hostname 与 hosts 映射。',
      icon: Pencil,
    },
    {
      id: 'ssh-port',
      title: 'SSH 端口',
      description: '调用 kejilion.sh 的“修改 SSH 端口”非交互适配入口。',
      value: management.ssh.ports.length ? management.ssh.ports.join('、') : '待 Agent 升级',
      detail: management.ssh.source === 'default' ? 'OpenSSH 默认端口' : '来自 sshd 配置',
      capability: 'system.ssh-port.write',
      safety: '由本机可信 kejilion.sh 执行原有 SSH 修改主业务；KPanel 负责结构化校验、执行前备份和结果回读。云安全组仍需单独放行新端口。',
      icon: KeyRound,
      tone: 'blue',
    },
    {
      id: 'ssh-defense',
      title: 'SSH 防御',
      description: '对应 kejilion.sh 的 `k f2b`，使用 Fail2Ban 防止 SSH 暴力破解。',
      value:
        management.maintenance.state === 'running' &&
        management.maintenance.action === 'ssh-defense'
          ? `正在${management.maintenance.policy === 'enable' ? '开启' : '关闭'} · ${management.maintenance.progress}%`
          : management.ssh.defense.enabled
            ? '已开启'
            : management.ssh.defense.installed
              ? '已关闭'
              : '未安装',
      detail: management.ssh.defense.enabled
        ? `${management.ssh.defense.jail || 'sshd'} jail · 当前封禁 ${management.ssh.defense.banned} 个 IP`
        : management.ssh.defense.message || '开启时安装并启用 Fail2Ban，关闭时保留现有配置',
      capability: 'system.ssh-defense.read',
      safety: '通过可信 kejilion.sh 管理 Fail2Ban；支持轻量策略、封禁 IP 与信任地址管理。',
      icon: ShieldCheck,
      tone: management.ssh.defense.enabled ? undefined : 'amber',
    },
    {
      id: 'timezone',
      title: '系统时区',
      description: '对应 kejilion.sh 的“设置系统时区”。',
      value: management.timezone || '待 Agent 升级',
      detail: '显示宿主机实际时区',
      capability: 'system.timezone.write',
      safety: '仅接受 IANA 时区数据库中的有效名称，变更后立即回读验证。',
      icon: Timer,
      tone: 'amber',
    },
    {
      id: 'swap',
      title: '虚拟内存',
      description: '对应 kejilion.sh 的“设置虚拟内存”。',
      value: management.swap.totalBytes
        ? `${formatBytes(management.swap.totalBytes)} · 已用 ${formatPercent(swapPercent)}`
        : '未启用',
      detail: swapDetails.join(' · '),
      capability: 'system.swap.write',
      safety: '与 kejilion.sh 统一管理 /swapfile；调整时自动合并旧版 KPanel Swap，不清除 Swap 分区或第三方 swapfile。',
      icon: MemoryStick,
      tone: management.swap.legacyExists || management.swap.legacyActive ? 'amber' : undefined,
    },
    {
      id: 'mirror',
      title: '系统更新源',
      description: '对应 kejilion.sh 的“换系统更新源”。',
      value: management.packageSources[0] || '未识别',
      detail: management.packageManager ? `${management.packageManager.toUpperCase()} 软件源` : '等待 Agent 识别',
      capability: 'system.mirror.write',
      safety: '修改前备份源文件，执行语法与连通性测试；测试失败自动恢复。',
      icon: Globe2,
      tone: 'blue',
    },
    {
      id: 'system-tuning',
      title: '系统综合调优',
      description: '沿用 kejilion.sh 原有 12 项调优流程，可按需勾选后一次执行。',
      value:
        management.maintenance.state === 'running' && management.maintenance.action === 'system-tuning'
          ? `正在调优 · ${management.maintenance.progress}%`
          : capabilityState('system.tuning.read').enabled
            ? '12 项可选 · 默认全选'
            : '适配器未就绪',
      detail: '更新、清理、Swap、SSH、防御、防火墙、BBR、时区、DNS、IPv4、工具与网络参数',
      capability: 'system.tuning.read',
      safety: '每项由固定 kejilion.sh 协议执行并回读验证；外部脚本固定提交与 SHA-256，首项失败即停止。',
      icon: Wrench,
      tone: 'violet',
    },
    {
		id: 'accounts',
		title: '账户管理',
		description: '管理 Linux 账户、密码、SSH 公钥和 Root 登录策略。',
		value: capabilityState('system.accounts.read').enabled ? '打开后读取真实账户' : '适配器未就绪',
		detail: '创建账户、修改密码、密钥登录与禁用 Root',
		capability: 'system.accounts.read',
		safety: '密码和公钥正文仅通过受限 stdin 交给 kejilion.sh；写入使用资源版本、SSH 语法校验和失败回滚。',
		icon: KeyRound,
		tone: 'amber',
	},
	{
      id: 'cron',
      title: '定时任务管理',
      description: '读取并管理 kejilion.sh 兼容的系统定时任务。',
      value: capabilityState('system.cron.read').enabled ? '打开后读取真实任务' : '适配器未就绪',
      detail: '支持按行添加、修改和删除 Cron 记录',
      capability: 'system.cron.read',
      safety: '只通过类型化适配器修改脚本兼容的定时任务，写入使用资源版本校验并在完成后重新读取。',
      icon: Clock3,
      tone: 'violet',
    },
  ]
})

function kernelProfileLabel(profile?: string): string {
  if (profile === '均衡优化模式') return profile
  return profile || '自定义'
}

function managementDetailLabel(detail: string): string {
  if (/^拥塞算法 .* · 队列 /.test(detail)) return detail
  return detail
}

const networkTools = computed<ManagementTool[]>(() => {
  if (!data.value) return []
  const management = data.value.management
  const bbrv3 = management.bbrv3
  const bbrv3Reason: Record<string, string> = {
    protocol_unavailable: '请更新本机 kejilion.sh 以启用 BBRv3 管理协议',
    status_failed: '脚本未能读取 BBRv3 状态',
    invalid_protocol: '脚本返回的 BBRv3 状态无法识别',
    unsupported_release: '仅支持 Debian 12 / Ubuntu 24 或更高版本',
    arm64_external_installer_untrusted: 'ARM64 原模块依赖未固定来源，暂不开放面板执行',
    unsupported_distribution: '当前仅支持 Debian / Ubuntu',
    missing_dependencies: 'APT 或 DPKG 依赖不完整',
  }
  let bbrv3Value = bbrv3.active
    ? '已生效'
    : bbrv3.installed
      ? bbrv3.rebootRequired
        ? '已安装 · 待重启'
        : '已安装'
      : bbrv3.supported
        ? '可安装'
        : '当前主机不支持'
  let bbrv3Detail = [
    bbrv3.runningKernel ? `内核 ${bbrv3.runningKernel}` : '',
    bbrv3.installedKernel && bbrv3.installedKernel !== bbrv3.runningKernel
      ? `待切换 ${bbrv3.installedKernel}`
      : '',
    bbrv3.congestionControl ? `拥塞算法 ${bbrv3.congestionControl}` : '',
    bbrv3.defaultQDisc ? `队列 ${bbrv3.defaultQDisc}` : '',
  ].filter(Boolean).join(' · ')
  if (!bbrv3.available || !bbrv3.supported) {
    bbrv3Detail = bbrv3Reason[bbrv3.reason || ''] || bbrv3Detail || '等待 Agent 识别'
  }
  if (management.maintenance.action === 'bbrv3' && management.maintenance.state !== 'idle') {
    if (management.maintenance.state === 'running') {
      bbrv3Value = `进行中 · ${management.maintenance.progress}%`
    } else if (management.maintenance.state === 'failed') {
      bbrv3Value = '上次执行失败'
    } else {
      bbrv3Value = management.maintenance.rebootRequired ? '已完成 · 待重启' : '已完成'
    }
    bbrv3Detail = management.maintenance.message || bbrv3Detail
  }
  return [
    {
      id: 'dns',
      title: 'DNS 地址',
      description: '对应 kejilion.sh 的“DNS 优化/修改 DNS”。',
      value: management.dns.servers.length ? management.dns.servers.join(' · ') : '未识别',
      detail: `解析器：${management.dns.manager || 'unknown'}`,
      capability: 'system.dns.write',
      safety: '通过本机可信 kejilion.sh 的固定 DNS 协议执行；systemd-resolved 使用原生配置，其他管理器沿用脚本的 resolv.conf 写入与锁定语义。',
      icon: Network,
      tone: 'violet',
    },
    {
      id: 'hosts',
      title: '本地 Hosts',
      description: '直接读取并管理 kejilion.sh 使用的本机 Hosts 事实。',
      value: capabilityState('system.hosts.read').enabled ? '打开后读取真实记录' : '适配器未就绪',
      detail: '支持类型化添加和按行删除主机映射',
      capability: 'system.hosts.read',
      safety: '写入携带资源版本并由 Agent 验证地址、主机名和目标行，完成后重新读取 /etc/hosts。',
      icon: ListTree,
      tone: 'blue',
    },
    {
      id: 'network-interfaces',
      title: '网卡管理',
      description: '读取网卡真实状态并通过固定动作启用或停用接口。',
      value: capabilityState('system.network-interfaces.read').enabled ? '打开后读取网卡状态' : '适配器未就绪',
      detail: '显示接口状态、硬件地址和本机地址',
      capability: 'system.network-interfaces.read',
      safety: '只接受已发现的接口名称和启用状态，使用资源版本避免旧页面覆盖新网络状态。',
      icon: Network,
      tone: 'amber',
    },
    {
      id: 'firewall',
      title: '防火墙',
      description: '按 kejilion.sh 固定动作管理端口、IP、Ping 与 DDoS 防护。',
      value: capabilityState('system.firewall.read').enabled ? '打开后读取真实规则' : '适配器未就绪',
      detail: '识别实际防火墙后端、INPUT 策略与规则',
      capability: 'system.firewall.read',
      safety: '不接受原始规则或 Shell；所有动作使用固定字段和资源版本，完成后重新读取真实规则。',
      icon: ShieldCheck,
      tone: 'danger',
    },
	{
		id: 'port-usage',
		title: '端口占用查看',
		description: '通过 kejilion.sh 查看当前 TCP / UDP 监听端口和占用进程。',
		value: capabilityState('system.port-usage.read').enabled ? '打开后读取实时端口' : '适配器未就绪',
		detail: '最多显示 512 条，支持按端口、进程和 PID 筛选',
		capability: 'system.port-usage.read',
		safety: '只读调用 ss 的固定参数，由脚本限制单行、总量和返回条数。',
		icon: ListTree,
		tone: 'blue',
	},
	{
		id: 'traffic-shutdown',
		title: '限流自动关机',
		description: '累计接收或发送流量达到阈值后自动关闭服务器。',
		value: capabilityState('system.traffic-shutdown.read').enabled ? '打开后读取真实状态' : '适配器未就绪',
		detail: '阈值、累计流量和每月重启日均由脚本回读',
		capability: 'system.traffic-shutdown.read',
		safety: '脚本只维护自己的文件和 cron 标记区块；写入带资源版本、事务回滚和明确关机确认。',
		icon: Power,
		tone: 'danger',
	},
    {
      id: 'ip-preference',
      title: 'V4 / V6 优先',
      description: '对应 kejilion.sh 的“设置 v4/v6 优先级”。',
      value: management.ipPreference === 'ipv4' ? 'IPv4 优先' : management.ipPreference === 'system_default' ? '系统默认' : '未识别',
      detail: management.ipPreference === 'ipv4' ? '已识别 gai.conf 规则' : '未发现 Kejilion IPv4 优先规则',
      capability: 'system.ip-preference.write',
      safety: '只维护一条带 KPanel 标识的 gai.conf 规则，撤销时不删除用户原配置。',
      icon: ArrowLeftRight,
      tone: 'violet',
    },
    {
      id: 'kernel',
      title: '内核优化',
      description: '对应 kejilion.sh 的“Linux 内核调优管理”。',
      value: management.kernelOptimization.enabled
        ? kernelProfileLabel(management.kernelOptimization.profile)
        : '未启用',
      detail: management.kernelOptimization.source === 'kejilion' ? '已识别 Kejilion 产物' : '系统默认参数',
      capability: 'system.kernel-tuning.write',
      safety: '五种预设与 kejilion.sh 共用产物，并按宿主机内存自适应；固定参数逐项应用，失败时恢复上一版本。',
      icon: Settings2,
      tone: 'amber',
    },
    {
      id: 'bbr',
      title: 'BBR 加速',
      description: '对应 kejilion.sh 的“开启 BBR 加速”。',
      value: management.bbr.enabled ? '已启用' : management.bbr.supported ? '可启用' : '当前内核不支持',
      detail: [
        management.bbr.congestionControl ? `拥塞算法 ${management.bbr.congestionControl}` : '',
        management.bbr.defaultQDisc ? `队列 ${management.bbr.defaultQDisc}` : '',
      ]
        .filter(Boolean)
        .join(' · ') || '等待 Agent 识别',
      capability: 'system.bbr.write',
      safety: '先确认内核暴露 bbr 算法，再写入独立 sysctl 配置并回读生效状态。',
      icon: Bolt,
    },
    {
      id: 'bbrv3',
      title: 'BBRv3 管理',
      description: '对应 kejilion.sh 的“BBRv3 管理”。',
      value: bbrv3Value,
      detail: bbrv3Detail,
      capability: 'system.bbrv3.write',
      safety: '复用 kejilion.sh 的 XanMod 安装、更新与卸载流程；由独立 systemd 任务执行，不自动重启服务器。',
      icon: Gauge,
      tone: bbrv3.rebootRequired ? 'amber' : 'blue',
    },
  ]
})

const maintenanceRunning = computed(() => data.value?.management.maintenance.state === 'running')

function scheduleMaintenanceRefresh(): void {
  if (maintenanceRefreshTimer) window.clearTimeout(maintenanceRefreshTimer)
  if (!refreshActive || !maintenanceRunning.value) return
  maintenanceRefreshTimer = window.setTimeout(async () => {
    maintenanceController?.abort()
    maintenanceController = new AbortController()
    try {
      const maintenance = await api.system.maintenance(maintenanceController.signal)
      if (data.value) data.value.management.maintenance = maintenance
      if (maintenance.state !== 'running') await load(true)
    } catch (reason) {
      if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
        // The regular overview refresh remains active; a transient poll failure is retried.
      }
    } finally {
      scheduleMaintenanceRefresh()
    }
  }, 3_000)
}

watch(maintenanceRunning, () => scheduleMaintenanceRefresh())

const maintenanceTools = computed<ManagementTool[]>(() => {
  if (!data.value) return []
  const management = data.value.management
  const maintenance = management.maintenance
  const packageManager = (management.packageManager || 'apt').toUpperCase()
  const valueFor = (action: 'update' | 'cleanup', idleValue: string): string => {
    if (maintenance.action !== action || maintenance.state === 'idle') return idleValue
    if (maintenance.state === 'running') return `进行中 · ${maintenance.progress}%`
    if (maintenance.state === 'failed') return '上次执行失败'
    return maintenance.rebootRequired ? '已完成 · 建议重启' : '上次执行成功'
  }
  const detailFor = (action: 'update' | 'cleanup', idleDetail: string): string => {
    if (maintenance.action !== action || maintenance.state === 'idle') return idleDetail
    return maintenance.message || idleDetail
  }
  return [
    {
      id: 'system-update',
      title: '系统更新',
      description: '对应 kejilion.sh 的“系统更新”，后台刷新软件包索引并完整升级系统。',
      value: valueFor('update', `${packageManager} 完整更新`),
      detail: detailFor('update', `使用 ${packageManager} 的原生锁与非交互模式；完成后提示是否需要重启。`),
      capability: 'system.update.write',
      safety: '使用独立 systemd 后台任务执行宿主机对应的软件包管理器固定序列；不接受包名或命令，不自动重启服务器。',
      icon: RefreshCw,
      tone: 'blue',
    },
    {
      id: 'system-cleanup',
      title: '系统清理',
      description: '对应 kejilion.sh 的“系统清理”，提供缓存清理和标准安全清理。',
      value: valueFor('cleanup', '缓存 · 无用依赖 · 旧日志'),
      detail: detailFor('cleanup', '不执行 Docker prune，不清空 /var/log、/tmp、网站目录或 KPanel 备份。'),
      capability: 'system.cleanup.write',
      safety: `仅调用固定 ${packageManager} 与 journalctl 参数；标准模式保留最近 7 天日志并限制 journal 最大 500 MiB。`,
      icon: Database,
      tone: 'violet',
    },
    {
      id: 'system-reboot',
      title: '重启服务器',
      description: '安排宿主机重启，面板和当前连接会短暂离线。',
      value: maintenance.rebootRequired ? '系统建议重启' : '受控延时重启',
      detail: '确认后延迟约 15 秒执行，为审计记录和页面响应预留时间。',
      capability: 'system.reboot.write',
      safety: '创建 systemd 延时重启单元；即使维护任务正在运行，管理员确认后也可直接重启。',
      icon: Power,
      tone: 'danger',
    },
  ]
})

const reinstallTool = computed<ManagementTool>(() => ({
  id: 'reinstall',
  title: '重装系统',
  description: '对应 kejilion.sh 的“重装系统”。此操作会清除系统并导致面板离线。',
  value: '高风险操作',
  detail: '等待非交互任务适配器',
  capability: 'system.reinstall',
  safety: '当前缺少将系统镜像、发行版版本和重装后凭证传给 kejilion.sh 的非交互协议；这是待实现功能，不是产品限制。',
  icon: RefreshCcw,
  tone: 'danger',
}))

function capabilityState(id: string): { enabled: boolean; reason: string } {
  const capability = data.value?.management.capabilities[id]
  return {
    enabled: Boolean(capability?.enabled),
    reason: capability?.reason || '当前 Agent 仅提供状态读取',
  }
}

const overviewSystemToolIDs = ['swap', 'ssh-port', 'dns', 'ip-preference', 'bbr', 'system-tuning'] as const
const recommendedSystemToolIDs = new Set([
  'system-update',
  'system-cleanup',
  'swap',
  'ssh-defense',
  'dns',
  'bbr',
])

const overviewSystemToolTitles: Record<(typeof overviewSystemToolIDs)[number], string> = {
  swap: '虚拟内存',
  'ssh-port': 'SSH 端口',
  dns: 'DNS 优化',
  'ip-preference': 'V4 / V6 优先',
  bbr: 'BBR 管理',
  'system-tuning': '综合调优',
}

const overviewSystemTools = computed<ManagementTool[]>(() => {
  const tools = new Map(
    [...basicSettings.value, ...networkTools.value].map((tool) => [tool.id, tool]),
  )
  return overviewSystemToolIDs.flatMap((id) => {
    const tool = tools.get(id)
    return tool ? [{ ...tool, title: overviewSystemToolTitles[id] }] : []
  })
})

const systemCenterSections = computed<SystemCenterSection[]>(() => {
  const tools = new Map(
    [...basicSettings.value, ...networkTools.value].map((tool) => [tool.id, tool]),
  )
  const select = (ids: string[]) => ids.flatMap((id) => {
    const tool = tools.get(id)
    return tool ? [{ ...tool, recommended: recommendedSystemToolIDs.has(tool.id) }] : []
  })

  return [
    {
      id: 'maintenance',
      title: '日常维护',
      description: '系统更新、空间清理与可控重启',
      icon: RefreshCw,
      iconTone: 'violet',
      tools: maintenanceTools.value.map((tool) => ({
        ...tool,
        recommended: recommendedSystemToolIDs.has(tool.id),
      })),
    },
    {
      id: 'basic',
      title: '基础配置',
      description: '主机身份、资源与常用系统参数',
      icon: Wrench,
      tools: select(['swap', 'hostname', 'timezone', 'mirror', 'cron']),
    },
    {
      id: 'security',
      title: '登录与安全',
      description: '账户、SSH 与入站访问保护',
      icon: ShieldCheck,
      iconTone: 'amber',
      tools: select(['ssh-port', 'ssh-defense', 'accounts', 'firewall']),
    },
    {
      id: 'network',
      title: '网络与流量',
      description: '解析、网卡、端口与流量策略',
      icon: Network,
      iconTone: 'blue',
      tools: select(['dns', 'port-usage', 'network-interfaces', 'ip-preference', 'hosts', 'traffic-shutdown']),
    },
    {
      id: 'performance',
      title: '性能优化',
      description: '系统、内核与网络综合调优',
      icon: Gauge,
      iconTone: 'violet',
      tools: select(['system-tuning', 'bbr', 'kernel', 'bbrv3']),
    },
  ]
})

const resourceCapabilityNames: Record<ResourceDialogID, string> = {
  hosts: 'system.hosts',
  cron: 'system.cron',
  'network-interfaces': 'system.network-interfaces',
  firewall: 'system.firewall',
	'port-usage': 'system.port-usage',
		'traffic-shutdown': 'system.traffic-shutdown',
		accounts: 'system.accounts',
		'ssh-defense': 'system.ssh-defense',
		'system-tuning': 'system.tuning',
}

const resourceCapabilityUnavailableReasons: Record<ResourceDialogID, string> = {
  hosts: '当前 Agent 的 Hosts 适配器未就绪。',
  cron: '当前 Agent 的定时任务适配器未就绪。',
  'network-interfaces': '当前 Agent 的网卡适配器未就绪。',
  firewall: '当前 Agent 的防火墙适配器未就绪。',
	'port-usage': '当前 Agent 的端口占用适配器未就绪。',
		'traffic-shutdown': '当前 Agent 的限流关机适配器未就绪。',
		accounts: '当前 Agent 的账户管理适配器未就绪。',
		'ssh-defense': '当前 Agent 的 SSH 防御适配器未就绪。',
		'system-tuning': '当前 Agent 的系统综合调优能力尚未就绪。',
}

function isResourceDialogID(id: string): id is ResourceDialogID {
  return id in resourceCapabilityNames
}

function resourceCapability(id: ResourceDialogID, mode: 'read' | 'write'): { enabled: boolean; reason: string } {
  const capabilityID = `${resourceCapabilityNames[id]}.${mode}`
  const capability = data.value?.management.capabilities[capabilityID]
  if (!capability) {
    return { enabled: false, reason: resourceCapabilityUnavailableReasons[id] }
  }
  return { enabled: Boolean(capability.enabled), reason: capability.reason || '' }
}

function toolAvailabilityLabel(tool: ManagementTool): string {
  if (!isResourceDialogID(tool.id)) return capabilityState(tool.capability).enabled ? '可配置' : '适配器未实现'
  if (!resourceCapability(tool.id, 'read').enabled) return '适配器未就绪'
  return resourceCapability(tool.id, 'write').enabled ? '可管理' : '仅查看'
}

function openTool(tool: ManagementTool): void {
  if (isResourceDialogID(tool.id)) {
    selectedResourceDialog.value = tool.id
    return
  }
  const management = data.value?.management
  actionForm.hostname = data.value?.hostname || ''
  actionForm.port = nextSSHPort(management?.ssh.ports || [])
  actionForm.dns = (management?.dns.servers || []).filter((server) => server !== '127.0.0.53').join('\n')
  actionForm.dnsPreset = detectDNSPreset(actionForm.dns)
  if (tool.id === 'dns' && actionForm.dnsPreset !== customPreset && hasPublicIPv6.value) applyDNSPreset()
  actionForm.timezone = management?.timezone || ''
  actionForm.timezonePreset = timezonePresets.some((preset) => preset.value === actionForm.timezone)
    ? actionForm.timezone
    : customPreset
  const preferredSwapBytes = management?.swap.legacySizeBytes || management?.swap.fileSizeBytes || 0
  actionForm.swapSizeMiB = preferredSwapBytes
    ? Math.max(1, Math.round(preferredSwapBytes / 1024 / 1024))
    : 1024
  actionForm.swapPreset = [1024, 2048, 4096].includes(actionForm.swapSizeMiB)
    ? String(actionForm.swapSizeMiB) as '1024' | '2048' | '4096'
    : 'custom'
  actionForm.mirrorPreset = detectMirrorPreset(management?.packageSources || [])
  actionForm.preference = management?.ipPreference === 'ipv4' ? 'ipv4' : 'system_default'
  actionForm.profile = detectKernelProfile(
    management?.kernelOptimization.enabled,
    management?.kernelOptimization.profile,
  )
  actionForm.bbrEnabled = !management?.bbr.enabled
  actionForm.bbrv3Policy = management?.bbrv3.installed ? 'update' : 'install'
  actionForm.maintenancePolicy = tool.id === 'system-cleanup' ? 'standard' : 'full'
  selectedTool.value = tool
}

function maintenanceActionFor(toolID: string): 'update' | 'cleanup' | 'ssh-defense' | 'bbrv3' | undefined {
  if (toolID === 'system-update') return 'update'
  if (toolID === 'system-cleanup') return 'cleanup'
  if (toolID === 'ssh-defense') return 'ssh-defense'
  if (toolID === 'bbrv3') return 'bbrv3'
  return undefined
}

function nextSSHPort(ports: number[]): number {
  for (const candidate of [2222, 22022, 2022, 22222]) {
    if (!ports.includes(candidate)) return candidate
  }
  return 2222
}

function detectKernelProfile(enabled = false, label = ''): KernelProfile {
  if (!enabled) return 'off'
  if (label.includes('高性能')) return 'high'
  if (label.includes('网站')) return 'web'
  if (label.includes('直播')) return 'stream'
  if (label.includes('游戏')) return 'game'
  return 'balanced'
}

function applyDNSPreset(): void {
  const preset = dnsPresets.find((item) => item.value === actionForm.dnsPreset)
  if (preset) actionForm.dns = dnsServersForPreset(preset, hasPublicIPv6.value).join('\n')
}

function applyTimezonePreset(): void {
  if (actionForm.timezonePreset !== customPreset) actionForm.timezone = actionForm.timezonePreset
}

function applySwapPreset(): void {
  if (actionForm.swapPreset !== 'custom') {
    actionForm.swapSizeMiB = Number(actionForm.swapPreset)
  }
}

function closeTool(): void {
  if (actionRunning.value) return
  selectedTool.value = undefined
}

function closeResourceDialog(): void {
  selectedResourceDialog.value = undefined
}

const actionInput = computed<SystemActionInput | undefined>(() => {
  const id = selectedTool.value?.id
  switch (id) {
    case 'hostname':
      return { action: 'hostname', hostname: actionForm.hostname.trim().toLowerCase() }
    case 'ssh-port':
      return { action: 'ssh-port', port: Number(actionForm.port) }
    case 'dns':
      return {
        action: 'dns',
        servers: parseDNSServers(actionForm.dns),
      }
    case 'timezone':
      return { action: 'timezone', timezone: actionForm.timezone.trim() }
    case 'swap':
      return { action: 'swap', swapSizeMiB: Number(actionForm.swapSizeMiB) }
    case 'mirror':
      return { action: 'mirror', mirrorPreset: actionForm.mirrorPreset }
    case 'ip-preference':
      return { action: 'ip-preference', preference: actionForm.preference }
    case 'kernel':
      return { action: 'kernel-tuning', profile: actionForm.profile }
    case 'bbr':
      return { action: 'bbr', enabled: actionForm.bbrEnabled }
    case 'bbrv3':
      return { action: 'bbrv3', maintenancePolicy: actionForm.bbrv3Policy }
    case 'system-update':
      return { action: 'update', maintenancePolicy: 'full' }
    case 'system-cleanup':
      return { action: 'cleanup', maintenancePolicy: actionForm.maintenancePolicy }
    case 'system-reboot':
      return { action: 'reboot' }
    default:
      return undefined
  }
})

const actionValid = computed(() => {
  const input = actionInput.value
  if (!input || !selectedTool.value || !capabilityState(selectedTool.value.capability).enabled) return false
  if (
    (input.action === 'update' ||
      input.action === 'cleanup' ||
      input.action === 'bbrv3') &&
    maintenanceRunning.value
  ) {
    return false
  }
  switch (input.action) {
    case 'hostname':
      return Boolean(input.hostname && /^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$/.test(input.hostname))
    case 'ssh-port':
      return Number.isInteger(input.port) && Number(input.port) >= 1 && Number(input.port) <= 65535
    case 'dns':
      return Boolean(
        input.servers?.length &&
        input.servers.length <= 4 &&
        input.servers.filter((server) => server.includes(':')).length <= 2 &&
        input.servers.filter((server) => !server.includes(':')).length <= 2,
      )
    case 'timezone':
      return Boolean(input.timezone && !input.timezone.includes('..'))
    case 'swap':
      return input.swapSizeMiB === 0 || Boolean(input.swapSizeMiB && input.swapSizeMiB >= 1)
    case 'reboot':
      return true
    case 'bbrv3':
      return Boolean(
        data.value?.management.bbrv3.supported ||
        (input.maintenancePolicy === 'uninstall' && data.value?.management.bbrv3.installed),
      )
    default:
      return true
  }
})

async function executeAction(): Promise<void> {
  const tool = selectedTool.value
  const input = actionInput.value
  if (!tool || !input || !actionValid.value || actionRunning.value) return
  actionRunning.value = true
  try {
    const result = await api.system.action(input)
    if (
      input.action === 'update' ||
      input.action === 'cleanup' ||
      input.action === 'bbrv3'
    ) {
      toast.success(`${tool.title}任务已提交`, result.message)
    } else if (input.action === 'reboot') {
      toast.success('服务器重启已安排', result.message)
    } else {
      toast.success(result.changed ? `${tool.title}已更新` : `${tool.title}无需变更`, result.message)
    }
    if (input.action !== 'reboot') await load(true)
    selectedTool.value = undefined
  } catch (reason) {
    toast.danger(`${tool.title}执行失败`, reason instanceof ApiError ? reason.message : 'Agent 未能完成该操作。')
  } finally {
    actionRunning.value = false
  }
}

async function load(silent = false): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''

  try {
    const onPartial = data.value
      ? undefined
      : (partial: SystemOverview) => {
          data.value = partial
          loading.value = false
          panel.setAgent(partial.agent)
        }
    data.value = await api.overview.get(controller.signal, onPartial)
    panel.setAgent(data.value.agent)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取主机状态。'
  } finally {
    loading.value = false
    refreshing.value = false
    if (!props.systemCenterOnly) {
      await nextTick()
      if (restoreOverviewViewport(pageElement.value) === 'desktop') {
        overviewSystemSection.value?.scrollIntoView({ block: 'start' })
      }
    }
  }
}

const pageElement = ref<HTMLElement>()
const overviewSystemSection = ref<HTMLElement>()

function rememberViewportBeforeSystemCenter(): void {
  rememberOverviewViewport(pageElement.value)
}

onMounted(() => {
  if (windowActive.value) {
    refreshActive = true
    void load()
    refreshTimer = window.setInterval(() => void load(true), 20_000)
  }
})

watch(windowActive, (active) => {
  refreshActive = active
  if (!active) {
    controller?.abort()
    if (refreshTimer) window.clearInterval(refreshTimer)
    refreshTimer = undefined
    return
  }
  void load(Boolean(data.value))
  if (!refreshTimer) refreshTimer = window.setInterval(() => void load(true), 20_000)
})

onBeforeUnmount(() => {
  refreshActive = false
  controller?.abort()
  maintenanceController?.abort()
  if (refreshTimer) window.clearInterval(refreshTimer)
  if (maintenanceRefreshTimer) window.clearTimeout(maintenanceRefreshTimer)
})
</script>

<template>
  <div ref="pageElement" class="page">
    <PageHeader
      :title="props.systemCenterOnly ? '系统中心' : '服务器概览'"
      :description="props.systemCenterOnly
        ? '集中管理系统维护、基础设置、网络工具与系统重装。'
        : '实时查看服务器资源与服务状态，并快速进入常用系统管理工具。'"
    />

    <LoadingState v-if="loading" :rows="4" cards />
    <ErrorState v-else-if="error && !data" :message="error" @retry="load()" />

    <template v-else-if="data">
      <div v-if="error" class="inline-alert inline-alert--warning" role="status">
        自动刷新暂时失败，正在显示上一次观测结果。
      </div>

      <section
        v-if="!props.systemCenterOnly"
        class="realtime-monitoring"
        aria-labelledby="realtime-monitoring-title"
      >
        <header class="realtime-monitoring__header">
          <div>
            <span class="realtime-monitoring__icon"><Activity :size="18" /></span>
            <div>
              <h2 id="realtime-monitoring-title">实时监控</h2>
              <p>点击指标查看历史趋势</p>
            </div>
          </div>
          <div class="realtime-monitoring__actions">
            <span v-if="data" class="observed-at">
              <Clock3 :size="15" /> {{ formatDateTime(data.observedAt) }}
            </span>
            <button class="icon-button" type="button" :disabled="refreshing" title="刷新系统状态" aria-label="刷新系统状态" @click="load(true)">
              <RefreshCw :size="16" :class="{ spin: refreshing }" />
            </button>
            <RouterLink class="button button--secondary button--small" to="/processes">
              <ListTree :size="16" />
              {{ processManagerLabel }}
            </RouterLink>
            <RouterLink class="button button--secondary button--small" to="/monitoring">
              查看历史
              <ChevronRight :size="16" />
            </RouterLink>
          </div>
        </header>

        <div class="metric-grid" aria-label="主机实时资源">
          <RouterLink
            class="metric-link"
            :to="{ path: '/monitoring', query: { metric: 'cpu' } }"
            aria-label="查看 CPU 历史趋势"
          >
            <MetricCard
              label="CPU"
              :icon="Cpu"
              :value="formatPercent(data.cpu.percent)"
              :percent="data.cpu.percent"
              detail="当前总使用率"
            />
          </RouterLink>
          <RouterLink
            class="metric-link"
            :to="{ path: '/monitoring', query: { metric: 'memory' } }"
            aria-label="查看内存历史趋势"
          >
            <MetricCard
              label="内存"
              :icon="MemoryStick"
              tone="blue"
              :value="formatPercent(data.memory.percent)"
              :percent="data.memory.percent"
              :detail="`${formatBytes(data.memory.value)} / ${formatBytes(data.memory.total)}`"
            />
          </RouterLink>
          <RouterLink
            class="metric-link"
            :to="{ path: '/monitoring', query: { metric: 'disk' } }"
            aria-label="查看系统盘历史趋势"
          >
            <MetricCard
              label="系统盘"
              :icon="HardDrive"
              tone="violet"
              :value="formatPercent(data.disk.percent)"
              :percent="data.disk.percent"
              :detail="`${formatBytes(data.disk.value)} / ${formatBytes(data.disk.total)}`"
            />
          </RouterLink>
          <RouterLink
            class="metric-link"
            :to="{ path: '/monitoring', query: { metric: 'load' } }"
            aria-label="查看系统负载历史趋势"
          >
            <MetricCard
              label="1 分钟负载"
              :icon="Gauge"
              tone="amber"
              :value="data.load.value.toFixed(2)"
              :percent="loadPercent"
              :detail="`${data.load.unit || '—'} 个 CPU 核心`"
            />
          </RouterLink>
          <RouterLink
            class="metric-link"
            :to="{ path: '/monitoring', query: { metric: 'network' } }"
            aria-label="查看网络历史趋势"
          >
            <MetricCard
              label="实时网络"
              :icon="Network"
              tone="blue"
              :value="data.network.rateAvailable
                ? formatRate(data.network.receiveBytesPerSecond + data.network.transmitBytesPerSecond)
                : '—'"
              :detail="data.network.rateAvailable
                ? `↓ ${formatRate(data.network.receiveBytesPerSecond)} · ↑ ${formatRate(data.network.transmitBytesPerSecond)}`
                : waitingForNextSample"
            />
          </RouterLink>
        </div>
      </section>

      <div v-if="!props.systemCenterOnly" class="overview-grid">
        <section class="panel-card panel-card--system">
          <header class="panel-card__header">
            <div>
              <span class="panel-card__icon"><Server :size="18" /></span>
              <div>
                <h2>{{ data.hostname || '未命名主机' }}</h2>
                <p>对齐 kejilion.sh 的 k info 主机事实</p>
              </div>
            </div>
            <StatusBadge :status="agentLabel.status" :label="agentLabel.label" />
          </header>

          <dl class="detail-list detail-list--grid">
            <div class="detail-list__wide">
              <dt>操作系统</dt>
              <dd class="os-identity">
                <OperatingSystemIcon :distro="osIdentity.key" :label="osIdentity.label" />
                <span>{{ data.os || '—' }}</span>
              </dd>
            </div>
            <div>
              <dt>内核</dt>
              <dd>{{ data.kernel || '—' }}</dd>
            </div>
            <div>
              <dt>架构</dt>
              <dd>{{ data.architecture || '—' }}</dd>
            </div>
            <div class="detail-list__wide">
              <dt>CPU 型号</dt>
              <dd>{{ data.cpu.model || '未识别' }}</dd>
            </div>
            <div>
              <dt>CPU 规格</dt>
              <dd>{{ data.cpu.cores }} 核 · {{ cpuFrequencyLabel }}</dd>
            </div>
            <div>
              <dt>系统负载（1 / 5 / 15 分钟）</dt>
              <dd>{{ data.load.one.toFixed(2) }} / {{ data.load.five.toFixed(2) }} / {{ data.load.fifteen.toFixed(2) }}</dd>
            </div>
            <div>
              <dt>宿主机时间</dt>
              <dd>{{ hostObservedTime }}</dd>
            </div>
            <div>
              <dt>运行时间</dt>
              <dd>{{ formatDuration(data.uptimeSeconds) }}</dd>
            </div>
          </dl>

          <div class="network-summary">
            <div>
              <span><ArrowDownToLine :size="16" /> 累计接收</span>
              <strong>{{ formatBytes(data.network.totalReceivedBytes) }}</strong>
            </div>
            <div>
              <span><ArrowUpFromLine :size="16" /> 累计发送</span>
              <strong>{{ formatBytes(data.network.totalTransmittedBytes) }}</strong>
            </div>
          </div>
        </section>

        <section class="panel-card">
          <header class="panel-card__header">
            <div>
              <span class="panel-card__icon panel-card__icon--blue"><Globe2 :size="18" /></span>
              <div>
                <h2>网络与位置</h2>
                <p>宿主机出口网络与解析状态</p>
              </div>
            </div>
            <span class="management-read-state" :class="{ 'is-warning': !data.publicNetwork.source }">
              <ShieldCheck v-if="data.publicNetwork.source" :size="14" />
              <CircleAlert v-else :size="14" />
              {{ data.publicNetwork.source ? '外网已识别' : '外网查询不可用' }}
            </span>
          </header>

          <dl class="detail-list detail-list--grid">
            <div class="detail-list__wide">
              <dt>运营商</dt>
              <dd>{{ data.publicNetwork.isp || '未获取' }}</dd>
            </div>
            <div>
              <dt>公网 IPv4</dt>
              <dd class="detail-list__mono">{{ data.publicNetwork.ipv4 || '未获取' }}</dd>
            </div>
            <div>
              <dt>公网 IPv6</dt>
              <dd class="detail-list__mono">{{ data.publicNetwork.ipv6 || '未获取' }}</dd>
            </div>
            <div class="detail-list__wide">
              <dt>地理位置</dt>
              <dd class="location-identity">
                <CountryFlagIcon
                  v-if="publicCountryCode"
                  :country-code="publicCountryCode"
                  :label="data.publicNetwork.country || publicCountryCode"
                />
                <span>{{ publicLocation }}</span>
              </dd>
            </div>
            <div class="detail-list__wide">
              <dt>DNS 地址</dt>
              <dd class="detail-list__mono">
                {{ data.management.dns.servers.length ? data.management.dns.servers.join(' · ') : '未识别' }}
              </dd>
            </div>
            <div>
              <dt>网络算法</dt>
              <dd>{{ networkAlgorithm }}</dd>
            </div>
            <div>
              <dt>TCP / UDP 连接数</dt>
              <dd>{{ data.network.tcpConnections }} / {{ data.network.udpConnections }}</dd>
            </div>
          </dl>

          <p class="k-info-note">
            <template v-if="data.publicNetwork.source">
              {{ data.publicNetwork.source }} · 30 分钟缓存 · 更新于 {{ formatDateTime(data.publicNetwork.updatedAt) }}
            </template>
            <template v-else>
              其他主机信息不依赖外网查询；可在 Agent 环境变量中关闭该项。
            </template>
          </p>
        </section>
      </div>

      <section v-if="!props.systemCenterOnly" class="panel-card panel-card--resource-overview">
        <header class="panel-card__header">
          <div>
            <span class="panel-card__icon panel-card__icon--violet"><Activity :size="18" /></span>
            <div>
              <h2>资源与一致性</h2>
              <p>网站、Docker 与审计事实来源</p>
            </div>
          </div>
        </header>

        <div class="resource-summary resource-summary--horizontal">
          <RouterLink to="/sites" class="resource-summary__item">
            <span class="resource-summary__icon resource-summary__icon--brand"><Globe2 :size="20" /></span>
            <span>
              <strong>{{ data.sites?.total ?? '—' }}</strong>
              <small>已发现网站</small>
            </span>
            <em v-if="data.sites">{{ data.sites.drifted }} 个待核对</em>
          </RouterLink>
          <RouterLink to="/docker" class="resource-summary__item">
            <span class="resource-summary__icon resource-summary__icon--blue"><Box :size="20" /></span>
            <span>
              <strong>{{ data.containers?.total ?? '—' }}</strong>
              <small>Docker 容器</small>
            </span>
            <em v-if="data.containers">{{ data.containers.running }} 个运行中</em>
          </RouterLink>
          <RouterLink to="/apps" class="resource-summary__item resource-summary__item--stacked">
            <span class="resource-summary__icon resource-summary__icon--amber"><Boxes :size="20" /></span>
            <span>
              <strong>{{ data.apps?.installed ?? '—' }}</strong>
              <small>已安装应用</small>
            </span>
            <em v-if="data.apps">
              {{ data.apps.running }} 个运行中 · 共 {{ data.apps.total }} 个
            </em>
          </RouterLink>
          <RouterLink to="/audit" class="resource-summary__item">
            <span class="resource-summary__icon resource-summary__icon--violet"><ShieldCheck :size="20" /></span>
            <span>
              <strong>完整</strong>
              <small>操作审计</small>
            </span>
            <em>查看记录</em>
          </RouterLink>
        </div>
      </section>

      <template v-if="props.systemCenterOnly">
        <div class="system-center-layout">
          <section
            v-for="section in systemCenterSections"
            :id="`system-center-${section.id}`"
            :key="section.id"
            class="panel-card system-center-section"
            :aria-labelledby="`system-center-${section.id}-title`"
          >
            <header class="panel-card__header system-center-section__header">
              <div>
                <span
                  class="panel-card__icon"
                  :class="section.iconTone ? `panel-card__icon--${section.iconTone}` : ''"
                >
                  <component :is="section.icon" :size="18" />
                </span>
                <div>
                  <h2 :id="`system-center-${section.id}-title`">{{ section.title }}</h2>
                  <p>{{ section.description }}</p>
                </div>
              </div>
              <span
                v-if="section.id === 'maintenance' && maintenanceRunning"
                class="management-read-state"
              >
                <RefreshCw :size="14" class="spin" />
                {{ data.management.maintenance.progress }}%
              </span>
              <span
                v-else-if="
                  section.id === 'maintenance' && data.management.maintenance.rebootRequired
                "
                class="management-read-state is-warning"
              >
                <CircleAlert :size="14" /> 建议重启
              </span>
              <span v-else class="system-center-section__count">{{ section.tools.length }} 项</span>
            </header>

            <div class="system-center-grid">
              <button
                v-for="tool in section.tools"
                :key="tool.id"
                class="system-tool"
                :class="{ 'is-featured': tool.id === 'system-tuning' }"
                type="button"
                @click="openTool(tool)"
              >
                <span class="system-tool__top">
                  <span class="system-tool__icon" :class="tool.tone ? `is-${tool.tone}` : ''">
                    <component :is="tool.icon" :size="19" />
                  </span>
                  <span class="system-tool__badges">
                    <span v-if="tool.recommended" class="system-tool__recommend">推荐</span>
                    <span class="system-tool__state">
                      {{
                        section.id === 'maintenance'
                          ? maintenanceRunning && maintenanceActionFor(tool.id)
                            ? data.management.maintenance.action === maintenanceActionFor(tool.id)
                              ? `进行中 ${data.management.maintenance.progress}%`
                              : '任务占用'
                            : capabilityState(tool.capability).enabled
                              ? '可执行'
                              : '依赖未就绪'
                          : toolAvailabilityLabel(tool)
                      }}
                    </span>
                  </span>
                </span>
                <strong>{{ tool.title }}</strong>
                <span>{{ tool.value }}</span>
                <small>{{ managementDetailLabel(tool.detail) }}</small>
                <span
                  v-if="
                    section.id === 'maintenance' &&
                    maintenanceRunning &&
                    data.management.maintenance.action === maintenanceActionFor(tool.id)
                  "
                  class="maintenance-progress"
                  role="progressbar"
                  aria-label="系统维护进度"
                  :aria-valuenow="data.management.maintenance.progress"
                  aria-valuemin="0"
                  aria-valuemax="100"
                >
                  <span :style="{ width: `${data.management.maintenance.progress}%` }"></span>
                </span>
              </button>
            </div>
          </section>

          <section
            id="system-center-danger"
            class="panel-card system-center-section system-center-section--danger"
            aria-labelledby="system-center-danger-title"
          >
            <header class="panel-card__header system-center-section__header">
              <div>
                <span class="panel-card__icon panel-card__icon--danger">
                  <CircleAlert :size="18" />
                </span>
                <div>
                  <h2 id="system-center-danger-title">危险操作</h2>
                  <p>可能导致服务中断或数据丢失，操作前请确认恢复通道</p>
                </div>
              </div>
              <span class="system-center-section__count is-danger">1 项</span>
            </header>

            <div class="danger-zone" aria-labelledby="reinstall-title">
              <span class="danger-zone__icon"><CircleAlert :size="21" /></span>
              <span class="danger-zone__body">
                <strong id="reinstall-title">重装系统</strong>
                <small>清除系统数据并导致 KPanel 离线。没有带外恢复通道时保持锁定。</small>
              </span>
              <button
                class="button button--danger button--small"
                type="button"
                @click="openTool(reinstallTool)"
              >
                查看安全要求
              </button>
            </div>
          </section>
        </div>
      </template>

      <section
        v-else
        ref="overviewSystemSection"
        class="panel-card overview-system-management"
        aria-labelledby="overview-system-management-title"
      >
        <header class="panel-card__header overview-system-management__header">
          <div>
            <span class="panel-card__icon panel-card__icon--violet"><Settings2 :size="18" /></span>
            <div>
              <h2 id="overview-system-management-title">系统管理</h2>
              <p>常用系统设置集中入口，其余功能由系统中心承接。</p>
            </div>
          </div>
          <RouterLink
            class="button button--secondary button--small overview-system-management__more"
            to="/system"
            @click="rememberViewportBeforeSystemCenter"
          >
            更多设置
            <ChevronRight :size="16" />
          </RouterLink>
        </header>

        <div class="overview-system-grid" aria-label="6 项常用功能">
          <button
            v-for="tool in overviewSystemTools"
            :key="tool.id"
            class="overview-system-card"
            type="button"
            @click="openTool(tool)"
          >
            <span class="overview-system-card__top">
              <span class="overview-system-card__icon" :class="tool.tone ? `is-${tool.tone}` : ''">
                <component :is="tool.icon" :size="20" />
              </span>
              <span class="overview-system-card__state">{{ toolAvailabilityLabel(tool) }}</span>
            </span>
            <span class="overview-system-card__body">
              <strong>{{ tool.title }}</strong>
              <span>{{ tool.value }}</span>
              <small>{{ managementDetailLabel(tool.detail) }}</small>
            </span>
            <ChevronRight class="overview-system-card__arrow" :size="17" />
          </button>
        </div>
      </section>

      <section v-if="!props.systemCenterOnly && data.services.length" class="panel-card">
        <header class="panel-card__header">
          <div>
            <span class="panel-card__icon panel-card__icon--violet"><Database :size="18" /></span>
            <div>
              <h2>核心服务</h2>
              <p>关键服务状态</p>
            </div>
          </div>
        </header>
        <div class="service-grid">
          <div v-for="service in data.services" :key="service.id" class="service-item">
            <span class="service-item__icon"><Boxes :size="17" /></span>
            <span class="service-item__details">
              <strong>{{ service.name }}</strong>
              <small>{{ service.version || service.detail || '未报告版本' }}</small>
            </span>
            <StatusBadge :status="service.state" />
          </div>
        </div>
      </section>
    </template>

    <ModalDialog
      :open="Boolean(selectedTool)"
      :title="selectedTool?.title || '系统工具'"
      :description="selectedTool?.description"
      @close="closeTool"
    >
      <div v-if="selectedTool" class="management-dialog">
        <div class="management-dialog__current" :class="{ 'is-danger': selectedTool.tone === 'danger' }">
          <span>当前状态</span>
          <strong>{{ selectedTool.value }}</strong>
          <small>{{ selectedTool.detail }}</small>
        </div>

        <div class="management-dialog__section">
          <span class="management-dialog__section-icon"><ShieldCheck :size="17" /></span>
          <div>
            <strong>执行与回滚规则</strong>
            <p>{{ selectedTool.safety }}</p>
          </div>
        </div>

        <div v-if="capabilityState(selectedTool.capability).enabled" class="management-form">
          <label v-if="selectedTool.id === 'hostname'" class="field">
            <span>新主机名</span>
            <input v-model.trim="actionForm.hostname" maxlength="253" autocomplete="off" placeholder="server.example" />
            <small>仅允许小写字母、数字、连字符和点。</small>
          </label>
          <label v-else-if="selectedTool.id === 'ssh-port'" class="field">
            <span>新的 SSH 端口</span>
            <input v-model.number="actionForm.port" type="number" min="1" max="65535" inputmode="numeric" />
            <small>成功后替换原 SSH 监听端口；当前 SSH 会话通常不会立即断开。</small>
          </label>
          <div v-else-if="selectedTool.id === 'dns'" class="form-stack compact">
            <label class="field">
              <span>常用 DNS 方案</span>
              <select v-model="actionForm.dnsPreset" @change="applyDNSPreset">
                <option :value="customPreset">自定义 DNS 地址</option>
                <option v-for="preset in dnsPresets" :key="preset.value" :value="preset.value">
                  {{ preset.label }}
                </option>
              </select>
              <small>
                {{
                  hasPublicIPv6
                    ? '已检测到公网 IPv6，预设会同时填充 2 个 IPv4 和 2 个 IPv6 地址。'
                    : '未检测到公网 IPv6，预设仅填充 IPv4 地址。'
                }}
              </small>
            </label>
            <label class="field">
              <span>DNS 服务器</span>
              <textarea v-model.trim="actionForm.dns" rows="4" placeholder="1.1.1.1&#10;8.8.8.8"></textarea>
              <small>每行一个地址，最多 2 个 IPv4 和 2 个 IPv6；由 kejilion.sh 按宿主机 DNS 后端应用。</small>
            </label>
          </div>
          <div v-else-if="selectedTool.id === 'timezone'" class="form-stack compact">
            <label class="field">
              <span>常用城市与时区</span>
              <select v-model="actionForm.timezonePreset" @change="applyTimezonePreset">
                <option v-for="preset in timezonePresets" :key="preset.value" :value="preset.value">
                  {{ preset.label }}
                </option>
                <option :value="customPreset">其他 IANA 时区…</option>
              </select>
              <small>选择城市后自动使用对应 IANA 时区，夏令时由系统规则处理。</small>
            </label>
            <label v-if="actionForm.timezonePreset === customPreset" class="field">
              <span>自定义 IANA 时区</span>
              <input
                v-model.trim="actionForm.timezone"
                maxlength="128"
                autocomplete="off"
                placeholder="例如 Europe/Amsterdam"
              />
              <small>必须是服务器 `/usr/share/zoneinfo` 中存在的时区名称。</small>
            </label>
          </div>
          <div v-else-if="selectedTool.id === 'swap'" class="form-stack compact">
            <label class="field">
              <span>虚拟内存方案</span>
              <select v-model="actionForm.swapPreset" @change="applySwapPreset">
                <option value="1024">1 GiB（kejilion.sh 默认）</option>
                <option value="2048">2 GiB</option>
                <option value="4096">4 GiB</option>
                <option value="custom">自定义大小…</option>
                <option value="0">停用 /swapfile</option>
              </select>
              <small>直接创建或调整 `/swapfile`，脚本端和 Web 端会读取同一份产物。</small>
            </label>
            <label v-if="actionForm.swapPreset === 'custom'" class="field">
              <span>自定义大小（MiB）</span>
              <input
                v-model.number="actionForm.swapSizeMiB"
                type="number"
                min="1"
                step="1"
                inputmode="numeric"
              />
              <small>允许任意正整数 MiB；按 kejilion.sh 直接调整 `/swapfile`，底层命令失败时恢复原状态。</small>
            </label>
            <div
              v-if="data?.management.swap.legacyExists || data?.management.swap.legacyActive"
              class="inline-alert inline-alert--warning"
            >
              检测到旧版 KPanel Swap。执行后会移除旧文件，并由所选大小的 `/swapfile`
              替代，不会把两者容量相加，也不会改动其他 Swap。
            </div>
          </div>
          <fieldset v-else-if="selectedTool.id === 'mirror'" class="mirror-route-field">
            <legend>选择更新源区域</legend>
            <div class="mirror-route-grid">
              <label
                v-for="preset in mirrorPresets"
                :key="preset.value"
                class="mirror-route-card"
                :class="{ 'is-selected': actionForm.mirrorPreset === preset.value }"
              >
                <input v-model="actionForm.mirrorPreset" type="radio" :value="preset.value" />
                <span class="mirror-route-card__body">
                  <strong>{{ preset.title }}</strong>
                  <small>{{ preset.route }}</small>
                  <p>{{ preset.description }}</p>
                </span>
                <span class="mirror-route-card__check" aria-hidden="true"></span>
              </label>
            </div>
            <small class="mirror-route-field__note">
              只修改已识别的 Debian / Ubuntu 发行版源；Docker、NodeSource 等第三方源保持不变。
              执行过程不升级软件、不清缓存。
            </small>
          </fieldset>
          <label v-else-if="selectedTool.id === 'ip-preference'" class="field">
            <span>地址选择优先级</span>
            <select v-model="actionForm.preference">
              <option value="ipv4">IPv4 优先</option>
              <option value="system_default">系统默认（通常 IPv6 优先）</option>
            </select>
          </label>
          <label v-else-if="selectedTool.id === 'kernel'" class="field">
            <span>Kejilion 内核调优模式</span>
            <select v-model="actionForm.profile">
              <option value="high">高性能优化：激进内存与网络参数</option>
              <option value="balanced">均衡优化</option>
              <option value="web">网站优化：超高并发连接队列</option>
              <option value="stream">直播优化：大 UDP 缓冲区</option>
              <option value="game">游戏服优化：低延迟优先</option>
              <option value="off">还原默认设置</option>
            </select>
            <small>
              与 kejilion.sh 共用 `/etc/sysctl.d/99-kejilion-optimize.conf`，并根据服务器内存自动调整。
              在线测速型“自动调优”仍请在脚本中执行，Web 不下载或执行远程 Shell。
            </small>
          </label>
          <label v-else-if="selectedTool.id === 'bbr'" class="field">
            <span>目标状态</span>
            <select v-model="actionForm.bbrEnabled">
              <option :value="true">启用 BBR + fq</option>
              <option :value="false">停用并恢复 cubic + fq_codel</option>
            </select>
          </label>
          <label v-else-if="selectedTool.id === 'bbrv3'" class="field">
            <span>BBRv3 操作</span>
            <select v-model="actionForm.bbrv3Policy">
              <option v-if="!data?.management.bbrv3.installed" value="install">安装 XanMod BBRv3 内核</option>
              <option v-if="data?.management.bbrv3.installed" value="update">更新 XanMod BBRv3 内核</option>
              <option v-if="data?.management.bbrv3.installed" value="uninstall">卸载 XanMod BBRv3 内核</option>
            </select>
            <small>
              安装、更新或卸载内核后需要重启才能完成切换；KPanel 只提交后台任务，不会自动重启。
            </small>
          </label>
          <label v-else-if="selectedTool.id === 'system-update'" class="field">
            <span>更新方式</span>
            <select v-model="actionForm.maintenancePolicy">
              <option value="full">完整更新：刷新索引并升级全部软件包</option>
            </select>
            <small>可能更新内核并重启部分系统服务，但不会自动重启服务器。</small>
          </label>
          <label v-else-if="selectedTool.id === 'system-cleanup'" class="field">
            <span>清理范围</span>
            <select v-model="actionForm.maintenancePolicy">
              <option value="cache">仅清理软件包缓存</option>
              <option value="standard">标准清理：无用依赖、缓存和旧日志</option>
            </select>
            <small>不会清理 Docker、网站文件、数据库、`/tmp` 或 KPanel 配置备份。</small>
          </label>
          <div v-else-if="selectedTool.id === 'system-reboot'" class="form-stack compact">
            <div class="inline-alert inline-alert--danger">
              <CircleAlert :size="17" />
              <span>重启会立即中断 SSH、网站请求和面板连接。请先确认没有正在执行的业务任务。</span>
            </div>
            <small>执行时间固定，不能从 Web 传入任意命令或延迟参数。</small>
          </div>

        </div>

        <div
          class="inline-alert"
          :class="capabilityState(selectedTool.capability).enabled ? 'inline-alert--info' : 'inline-alert--warning'"
        >
          <CircleAlert :size="17" />
          <span>
            {{
              selectedTool.id === 'bbrv3' &&
              !data?.management.bbrv3.supported &&
              !(actionForm.bbrv3Policy === 'uninstall' && data?.management.bbrv3.installed)
                ? '当前系统或脚本来源不满足 BBRv3 受控执行条件，请查看上方状态说明。'
                : maintenanceRunning &&
              (selectedTool.id === 'system-update' ||
                selectedTool.id === 'system-cleanup' ||
                selectedTool.id === 'bbrv3')
                ? '已有系统维护任务正在后台执行，请等待完成后再提交新任务。'
                : capabilityState(selectedTool.capability).enabled
                  ? selectedTool.id === 'system-update' ||
                    selectedTool.id === 'system-cleanup' ||
                    selectedTool.id === 'bbrv3'
                    ? '任务由独立 systemd 单元执行；关闭浏览器不会中断，页面将持续读取进度。'
                    : selectedTool.id === 'system-reboot'
                      ? '请求成功后，Agent 会创建固定的延时重启单元；约 15 秒后面板离线属于正常现象。'
                    : '该操作使用固定参数执行器，并在完成后回读宿主机真实状态。'
                : capabilityState(selectedTool.capability).reason
            }}
          </span>
        </div>
      </div>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="actionRunning" @click="closeTool">关闭</button>
        <button
          class="button button--primary"
          type="button"
          :disabled="!actionValid || actionRunning"
          @click="executeAction"
        >
          <ShieldCheck :size="16" />
          {{
            actionRunning
              ? selectedTool?.id === 'system-reboot'
                ? '正在安排重启…'
                : '正在执行并验证…'
              : capabilityState(selectedTool?.capability || '').enabled
                ? selectedTool?.id === 'bbrv3' && !actionValid
                  ? '当前不可执行'
                  : '确认执行'
                : '当前不可执行'
          }}
        </button>
      </template>
    </ModalDialog>

    <HostsManagerDialog
      :open="selectedResourceDialog === 'hosts'"
      :readable="resourceCapability('hosts', 'read').enabled"
      :writable="resourceCapability('hosts', 'write').enabled"
      :unavailable-reason="resourceCapability('hosts', 'read').reason"
      @close="closeResourceDialog"
    />
    <CronManagerDialog
      :open="selectedResourceDialog === 'cron'"
      :readable="resourceCapability('cron', 'read').enabled"
      :writable="resourceCapability('cron', 'write').enabled"
      :unavailable-reason="resourceCapability('cron', 'read').reason"
      @close="closeResourceDialog"
    />
    <NetworkInterfacesDialog
      :open="selectedResourceDialog === 'network-interfaces'"
      :readable="resourceCapability('network-interfaces', 'read').enabled"
      :writable="resourceCapability('network-interfaces', 'write').enabled"
      :unavailable-reason="resourceCapability('network-interfaces', 'read').reason"
      @close="closeResourceDialog"
    />
    <FirewallManagerDialog
      :open="selectedResourceDialog === 'firewall'"
      :readable="resourceCapability('firewall', 'read').enabled"
      :writable="resourceCapability('firewall', 'write').enabled"
      :unavailable-reason="resourceCapability('firewall', 'read').reason"
      @close="closeResourceDialog"
    />
	<PortUsageDialog
		:open="selectedResourceDialog === 'port-usage'"
		:readable="resourceCapability('port-usage', 'read').enabled"
		:unavailable-reason="resourceCapability('port-usage', 'read').reason"
		@close="closeResourceDialog"
	/>
	<TrafficShutdownDialog
		:open="selectedResourceDialog === 'traffic-shutdown'"
		:readable="resourceCapability('traffic-shutdown', 'read').enabled"
		:writable="resourceCapability('traffic-shutdown', 'write').enabled"
		:unavailable-reason="resourceCapability('traffic-shutdown', 'read').reason"
		@close="closeResourceDialog"
	/>
	<AccountManagementDialog
		:open="selectedResourceDialog === 'accounts'"
		:readable="resourceCapability('accounts', 'read').enabled"
		:writable="resourceCapability('accounts', 'write').enabled"
		:unavailable-reason="resourceCapability('accounts', 'read').reason"
		@close="closeResourceDialog"
	/>
	<SSHDefenseDialog
		:open="selectedResourceDialog === 'ssh-defense'"
		:readable="resourceCapability('ssh-defense', 'read').enabled"
		:writable="resourceCapability('ssh-defense', 'write').enabled"
		:unavailable-reason="resourceCapability('ssh-defense', 'read').reason"
		@close="closeResourceDialog"
	/>
	<SystemTuningDialog
		:open="selectedResourceDialog === 'system-tuning'"
		:readable="resourceCapability('system-tuning', 'read').enabled"
		:writable="resourceCapability('system-tuning', 'write').enabled"
		:unavailable-reason="resourceCapability('system-tuning', 'read').reason"
		@close="closeResourceDialog"
	/>
  </div>
</template>

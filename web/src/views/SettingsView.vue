<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type CSSProperties } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/SettingsView/en-US').then((module) => module.default)
  : import('@/i18n/pages/SettingsView/zh-TW').then((module) => module.default))
import QRCode from 'qrcode'
import {
  Check,
  Clock3,
  Copy,
  Download,
  Image as ImageIcon,
  ImageOff,
  ExternalLink,
  KeyRound,
  Layers,
  Languages,
  LoaderCircle,
  Monitor,
  Moon,
  Palette,
  RefreshCw,
  Scale,
  Search,
  Server,
  ShieldCheck,
  Sun,
  UserRound,
  X,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import BackupCenter from '@/components/settings/BackupCenter.vue'
import PasskeySettings from '@/components/settings/PasskeySettings.vue'
import MCPAccess from '@/components/settings/MCPAccess.vue'
import DesktopWallpaperPicker from '@/components/desktop/DesktopWallpaperPicker.vue'
import KPanelUpdateDialog from '@/components/update/KPanelUpdateDialog.vue'
import ProblemReportHelp from '@/components/problem-report/ProblemReportHelp.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api, resetApiSecurityState } from '@/lib/api'
import { formatDateTime, relativeTime } from '@/lib/format'
import {
  isKPanelUpdateSettingsIntent,
  kpanelAppUpdatePath,
  releaseFromAutomaticUpdate,
} from '@/lib/kpanelUpdate'
import { usePanelState } from '@/stores/panel'
import { useSession } from '@/stores/session'
import { useTheme, type ThemePreference } from '@/stores/theme'
import {
  DEFAULT_THEME_COLORS,
  THEME_COLOR_PRESETS,
  THEME_COLOR_KEYS,
  deriveThemeTokens,
  normalizeHexColor,
  normalizeThemeColors,
  type ThemeColorIntent,
  type ThemeColorKey,
  type ThemeMode,
} from '@/theme/colors'
import { moveRadioFocus } from '@/theme/radioGroup'
import { useClassicWallpaper, type ClassicWallpaperLevel } from '@/lib/classicWallpaper'
import { useDesktopWallpaper } from '@/lib/desktopWallpapers'
import { useToast } from '@/stores/toast'
import { useI18n, type SupportedLocale } from '@/i18n'
import type { AutomaticUpdateStatus, KPanelReleaseInfo, TOTPEnrollment, TOTPStatus } from '@/types/api'

const route = useRoute()
const router = useRouter()
const session = useSession()
const panel = usePanelState()
const theme = useTheme()
const toast = useToast()
const i18n = useI18n()

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

type SettingsCategoryId = 'all' | 'account' | 'appearance' | 'data' | 'system' | 'support'
type SettingsSectionId =
  | 'help'
  | 'account-overview'
  | 'username'
  | 'password'
  | 'security-entrance'
  | 'totp'
  | 'passkeys'
  | 'language'
  | 'appearance'
  | 'wallpaper'
  | 'backup'
  | 'mcp'
  | 'version-updates'
  | 'agent'
  | 'license'

interface SettingsSectionDefinition {
  id: SettingsSectionId
  category: Exclude<SettingsCategoryId, 'all'>
  title: string
  description: string
  keywords: string[]
}

const settingsCategories: Array<{ id: SettingsCategoryId; label: string }> = [
  { id: 'all', label: '全部设置' },
  { id: 'account', label: '账户与安全' },
  { id: 'appearance', label: '外观与语言' },
  { id: 'data', label: '数据与接入' },
  { id: 'system', label: '系统与更新' },
  { id: 'support', label: '帮助与关于' },
]

const settingsSections: SettingsSectionDefinition[] = [
  { id: 'help', category: 'support', title: '帮助与问题报告', description: '生成问题报告并获取排查帮助', keywords: ['反馈', '诊断', '日志', '报告'] },
  { id: 'account-overview', category: 'account', title: '管理账户', description: '当前登录身份与会话信息', keywords: ['管理员', 'Session', '登录', '身份验证'] },
  { id: 'username', category: 'account', title: '修改用户名', description: '更新当前管理员账户的登录名称', keywords: ['账号', '名称'] },
  { id: 'password', category: 'account', title: '修改密码', description: '更新当前管理员账户的登录凭据', keywords: ['凭据', '登录'] },
  { id: 'security-entrance', category: 'account', title: '登录安全入口', description: '隐藏常规登录路径', keywords: ['安全路径', '公网扫描', '撞库'] },
  { id: 'totp', category: 'account', title: '两步验证', description: '身份验证器与恢复码', keywords: ['TOTP', '2FA', '验证码', '恢复码'] },
  { id: 'passkeys', category: 'account', title: 'Passkey 通行密钥', description: '设备验证与凭证管理', keywords: ['Passkey', 'WebAuthn', '指纹', '安全密钥'] },
  { id: 'language', category: 'appearance', title: '语言', description: '选择界面显示语言', keywords: ['简体中文', '繁体中文', 'English'] },
  { id: 'appearance', category: 'appearance', title: '外观', description: '主题配色与明暗模式', keywords: ['主题', '浅色', '深色', '颜色'] },
  { id: 'wallpaper', category: 'appearance', title: '壁纸', description: '桌面与经典模式共用的壁纸、3D 场景与透出程度', keywords: ['背景', '壁纸', '3D', '场景', '经典模式', '透明'] },
  { id: 'backup', category: 'data', title: '备份中心', description: '备份、恢复与数据保护', keywords: ['备份', '恢复', '导出'] },
  { id: 'mcp', category: 'data', title: 'MCP 接入', description: '外部工具与访问配置', keywords: ['MCP', 'API', 'Token', '令牌', '接入'] },
  { id: 'version-updates', category: 'system', title: '版本更新', description: '更新通道与自动安装', keywords: ['版本', '升级', '稳定版', '预览版', '自动更新'] },
  { id: 'agent', category: 'system', title: '宿主机 Agent', description: '面板唯一的特权操作边界', keywords: ['Agent', '协议', '能力', '宿主机'] },
  { id: 'license', category: 'support', title: '开源许可', description: 'GNU AGPL v3.0 only', keywords: ['许可证', '源码', 'AGPL'] },
]

const settingsSearch = ref('')
const activeSettingsCategory = ref<SettingsCategoryId>(
  isKPanelUpdateSettingsIntent(route.query.section) ? 'system' : 'all',
)
const settingsBrowser = ref<HTMLElement>()
const normalizedSettingsSearch = computed(() => settingsSearch.value.trim().toLocaleLowerCase())

function settingsSectionMatchesSearch(section: SettingsSectionDefinition): boolean {
  const query = normalizedSettingsSearch.value
  if (!query) return true
  return [section.title, section.description, ...section.keywords].some((value) => (
    value.toLocaleLowerCase().includes(query) || phrase(value).toLocaleLowerCase().includes(query)
  ))
}

const visibleSettingsSectionIds = computed(() => new Set(
  settingsSections
    .filter((section) => (
      (activeSettingsCategory.value === 'all' || section.category === activeSettingsCategory.value) &&
      settingsSectionMatchesSearch(section)
    ))
    .map((section) => section.id),
))

const visibleSettingsSectionCount = computed(() => visibleSettingsSectionIds.value.size)
const settingsResultSummary = computed(() => phrase(
  settingsSearch.value.trim()
    ? `找到 ${visibleSettingsSectionCount.value} 项设置`
    : `显示 ${visibleSettingsSectionCount.value} 项设置`,
))

function isSettingsSectionVisible(id: SettingsSectionId): boolean {
  return visibleSettingsSectionIds.value.has(id)
}

function settingsCategoryCount(category: SettingsCategoryId): number {
  return settingsSections.filter((section) => (
    (category === 'all' || section.category === category) && settingsSectionMatchesSearch(section)
  )).length
}

function clearSettingsSectionIntent(): void {
  if (!isKPanelUpdateSettingsIntent(route.query.section)) return
  const query = { ...route.query }
  delete query.section
  void router.replace({ query })
}

function selectSettingsCategory(category: SettingsCategoryId): void {
  activeSettingsCategory.value = category
  clearSettingsSectionIntent()
}

function onSettingsSearchInput(): void {
  clearSettingsSectionIntent()
}

function clearSettingsSearch(): void {
  settingsSearch.value = ''
  clearSettingsSectionIntent()
}

function resetSettingsFilters(): void {
  settingsSearch.value = ''
  activeSettingsCategory.value = 'all'
  clearSettingsSectionIntent()
}

function localeLabel(locale: SupportedLocale): string {
  if (locale === 'zh-CN') return i18n.t('common.locale.zhCN')
  if (locale === 'zh-TW') return i18n.t('common.locale.zhTW')
  return i18n.t('common.locale.enUS')
}
const refreshing = ref(false)
const changingPassword = ref(false)
const passwordSubmitted = ref(false)
const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})
const changingUsername = ref(false)
const usernameSubmitted = ref(false)
const usernameForm = reactive({
  newUsername: session.state.user?.username || '',
  currentPassword: '',
})
const usernamePasswordUnlocked = ref(false)
const capabilities = ref<Array<{ id: string; enabled: boolean; reason?: string }>>([])
const securityEntry = ref<{ enabled: boolean; path?: string; resourceVersion: string }>()
const securityEntryPath = ref('')
const savingSecurityEntry = ref(false)
const totpStatus = ref<TOTPStatus>()
const totpStatusError = ref('')
const totpEnrollment = ref<TOTPEnrollment>()
const totpQRCode = ref('')
const totpBusy = ref(false)
const totpAction = ref<'idle' | 'enroll' | 'verify' | 'recovery' | 'rotate' | 'disable'>('idle')
const totpError = ref('')
const recoveryCodes = ref<string[]>([])
const totpForm = reactive({ currentPassword: '', code: '', secondFactor: '' })
const automaticUpdate = ref<AutomaticUpdateStatus>()
const automaticUpdateError = ref('')
const savingAutomaticUpdate = ref(false)
const checkingAutomaticUpdate = ref(false)
const installingAutomaticUpdate = ref(false)
const manuallyUpdatingKPanel = ref(false)
const automaticUpdateSection = ref<HTMLElement>()
const kpanelUpdateDialogOpen = ref(false)
const kpanelRelease = ref<KPanelReleaseInfo>()
const kpanelReleaseLoading = ref(false)
const kpanelReleaseError = ref('')
let kpanelReleaseController: AbortController | undefined
let kpanelReleaseTimer: ReturnType<typeof setTimeout> | undefined

const securityEntryUrl = computed(() => {
  if (!securityEntry.value?.enabled || !securityEntry.value.path || typeof window === 'undefined') return ''
  return `${window.location.origin}/${securityEntry.value.path}`
})

function relativeTimeLabel(value?: string): string {
  const label = relativeTime(value)
  if (label === '现在') return label
  return label
}

const passwordChecks = computed(() => [
  { label: '至少 12 个字符', valid: passwordForm.newPassword.length >= 12 },
  {
    label: '包含字母和数字',
    valid: /[A-Za-z]/.test(passwordForm.newPassword) && /\d/.test(passwordForm.newPassword),
  },
])

const canChangePassword = computed(
  () =>
    passwordForm.currentPassword.length > 0 &&
    passwordChecks.value.every((item) => item.valid) &&
    passwordForm.newPassword === passwordForm.confirmPassword,
)

const usernameValid = computed(() => /^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$/.test(usernameForm.newUsername))
const canChangeUsername = computed(
  () =>
    usernameForm.currentPassword.length > 0 &&
    usernameValid.value &&
    usernameForm.newUsername !== session.state.user?.username,
)

const agentState = computed(() => {
  const agent = panel.state.agent
  if (!agent?.connected) return { status: 'offline', label: '离线' }
  if (!agent.compatible) return { status: 'incompatible', label: '不兼容' }
  if (agent.readOnly) return { status: 'read_only', label: '写入依赖未就绪' }
  return { status: 'connected', label: '正常' }
})

const automaticUpdateState = computed(() => {
  switch (automaticUpdate.value?.state) {
    case 'waiting': return { status: 'warning', label: automaticUpdate.value.channel === 'preview' ? '观察预览版' : '观察稳定版' }
    case 'available': return { status: 'pending', label: '可立即安装' }
    case 'queued': return { status: 'running_job', label: '安装任务已排队' }
    case 'updating': return { status: 'running_job', label: '正在更新' }
    case 'succeeded': return { status: 'connected', label: '最近更新成功' }
    case 'failed': return { status: 'failed_rolled_back', label: '更新失败，已尝试回退' }
    case 'blocked': return { status: 'warning', label: '此版本已暂停' }
    case 'check_failed': return { status: 'warning', label: '检查失败' }
    case 'idle': return { status: 'connected', label: '已是最新版本' }
    default: return { status: 'stopped', label: '自动安装关闭' }
  }
})

const automaticUpdateChannelLabel = computed(() => (
  automaticUpdate.value?.channel === 'preview' ? '预览版' : '稳定版'
))

const manualUpdateButtonLabel = computed(() => (
  automaticUpdate.value?.canInstall && automaticUpdate.value.candidateVersion
    ? `手动更新至 ${automaticUpdate.value.candidateVersion}`
    : '手动检查并更新'
))

const automaticUpdateScheduleNote = computed(() =>
  `systemd 宿主机每天本地时间 04:00 检查，并随机延迟最多 30 分钟。自动安装前观察 ${automaticUpdate.value?.observationHours ?? 24} 小时；手动立即安装可跳过等待。切换前会冷备份 Panel 与 Agent 数据，失败时自动恢复原版本和数据。退出预览版计划不会自动降级。`,
)

const automaticUpdateNotice = computed(() => {
  const status = automaticUpdate.value
  if (!status) return ''
  if (status.state === 'failed') return '本次更新未完成；系统已自动尝试恢复原版本和更新前数据。'
  if (status.state === 'blocked') return `版本 ${status.failedVersion || status.candidateVersion || '—'} 更新失败后已暂停，不会自动重复尝试。`
  if (status.state === 'queued') return `版本 ${status.candidateVersion || '—'} 已排队，宿主机将在后台完成更新。`
  if (status.state === 'waiting' && status.candidateVersion && status.enabled) return `已发现 ${status.candidateVersion}，连续观察 ${status.observationHours} 小时后自动安装，也可以立即安装。`
  if (status.state === 'waiting' && status.candidateVersion) return `已发现 ${status.candidateVersion}；自动安装已关闭，你仍可以立即安装。`
  if (status.state === 'available' && status.candidateVersion && status.enabled) return `${status.candidateVersion} 已通过观察期，将在下次定时任务中安装，也可以立即安装。`
  if (status.state === 'available' && status.candidateVersion) return `${status.candidateVersion} 已可安装；自动安装仍保持关闭。`
  return ''
})

const themeModes: Array<{ id: ThemePreference; label: string; description: string; icon: typeof Sun }> = [
  { id: 'light', label: '浅色', description: '始终使用明亮界面', icon: Sun },
  { id: 'dark', label: '深色', description: '始终使用低亮度界面', icon: Moon },
  { id: 'system', label: '跟随系统', description: '随设备设置自动切换', icon: Monitor },
]

const classicWallpaper = useClassicWallpaper()
const desktopWallpaper = useDesktopWallpaper()
desktopWallpaper.refresh()
const classicWallpaperLevels: Array<{ id: ClassicWallpaperLevel; label: string; description: string; icon: typeof Sun }> = [
  { id: 'off', label: '关闭', description: '纯色背景，信息最清晰', icon: ImageOff },
  { id: 'ambient', label: '氛围', description: '壁纸透出页边、侧栏与顶栏，卡片不透明', icon: ImageIcon },
  { id: 'clear', label: '通透', description: '卡片也半透明，接近桌面模式的观感', icon: Layers },
]

const themeColorFields: Array<{ key: ThemeColorKey; label: string; pickerLabel: string; description: string }> = [
  { key: 'brand', label: '主题色', pickerLabel: '主题色颜色选择器', description: '用于按钮、链接、选中态、焦点和进度' },
  { key: 'neutral', label: '界面基调', pickerLabel: '界面基调颜色选择器', description: '决定背景、卡片、侧栏和桌面环境的明暗与冷暖倾向' },
  { key: 'signature', label: '点缀色', pickerLabel: '点缀色颜色选择器', description: '仅用于当前项细线和活动任务等少量身份标记' },
]
const colorPreviewModes: Array<{ id: ThemeMode; label: string }> = [
  { id: 'light', label: '浅色预览' },
  { id: 'dark', label: '深色预览' },
]
const colorDraft = reactive<ThemeColorIntent>({ ...theme.colors.value })
const colorInputs = reactive<Record<ThemeColorKey, string>>({
  brand: colorDraft.brand,
  neutral: colorDraft.neutral,
  signature: colorDraft.signature,
})
const colorErrors = reactive<Record<ThemeColorKey, string>>({ brand: '', neutral: '', signature: '' })
const colorPreviewMode = ref<ThemeMode>(theme.resolved.value)

const hasColorErrors = computed(() => THEME_COLOR_KEYS.some((key) => Boolean(colorErrors[key])))
const colorDraftDirty = computed(() => {
  const applied = theme.colors.value
  return colorDraft.brand !== applied.brand
    || colorDraft.neutral !== applied.neutral
    || colorDraft.signatureLinked !== applied.signatureLinked
    || colorDraft.signature !== applied.signature
})
const colorPreviewTokens = computed(() => deriveThemeTokens(normalizeThemeColors(colorDraft), colorPreviewMode.value))
const colorPreviewStyle = computed(() => ({
  ...colorPreviewTokens.value,
  colorScheme: colorPreviewMode.value,
}) as CSSProperties)
const activeThemePresetId = computed(() => (
  THEME_COLOR_PRESETS.find((preset) => sameThemeColors(colorDraft, preset.colors))?.id ?? null
))

function syncColorDraft(value: ThemeColorIntent): void {
  Object.assign(colorDraft, value)
  for (const key of THEME_COLOR_KEYS) {
    colorInputs[key] = value[key]
    colorErrors[key] = ''
  }
}

function sameThemeColors(left: Readonly<ThemeColorIntent>, right: Readonly<ThemeColorIntent>): boolean {
  return left.brand === right.brand
    && left.neutral === right.neutral
    && left.signatureLinked === right.signatureLinked
    && left.signature === right.signature
}

function selectThemeColorPreset(colors: Readonly<ThemeColorIntent>): void {
  syncColorDraft({ ...colors })
}

function themeColorPresetStyle(colors: Readonly<ThemeColorIntent>): CSSProperties {
  return {
    ...deriveThemeTokens({ ...colors }, colorPreviewMode.value),
    colorScheme: colorPreviewMode.value,
  } as CSSProperties
}

watch(() => theme.colors.value, (next, previous) => {
  if (sameThemeColors(colorDraft, previous)) syncColorDraft(next)
})

function updateThemeColor(key: ThemeColorKey, event: Event): void {
  const input = event.currentTarget as HTMLInputElement
  const raw = input.value.trim()
  colorInputs[key] = raw
  const normalized = normalizeHexColor(raw)
  if (!normalized) {
    colorErrors[key] = '请输入 3 或 6 位 Hex 颜色，例如 #315d7d'
    return
  }
  colorErrors[key] = ''
  colorDraft[key] = normalized
  if (input.type === 'color') colorInputs[key] = normalized
}

function applyThemeColors(): void {
  if (hasColorErrors.value || !colorDraftDirty.value) return
  const next = normalizeThemeColors(colorDraft)
  if (sameThemeColors(next, DEFAULT_THEME_COLORS)) {
    theme.resetColors()
    syncColorDraft({ ...DEFAULT_THEME_COLORS })
    toast.success('已恢复默认配色')
    return
  }
  theme.setColors(next)
  syncColorDraft(next)
  toast.success('配色已应用', '浅色和深色层级已自动生成。')
}

function cancelThemeColorChanges(): void {
  syncColorDraft(theme.colors.value)
}

function resetThemeColors(): void {
  theme.resetColors()
  syncColorDraft({ ...DEFAULT_THEME_COLORS })
  toast.success('已恢复默认配色')
}

async function refreshAgent(): Promise<void> {
  refreshing.value = true
  try {
    const [health, capabilityResult] = await Promise.all([api.agent.health(), api.agent.capabilities()])
    panel.setAgent(health)
    session.state.agent = health
    capabilities.value = capabilityResult
    toast.success('连接状态已更新')
  } catch (reason) {
    toast.danger('无法连接 Agent', reason instanceof ApiError ? reason.message : '请检查宿主机服务。')
  } finally {
    refreshing.value = false
  }
}

async function saveAutomaticUpdatePolicy(
  enabled: boolean,
  channel: 'stable' | 'preview',
  successMessage: string,
): Promise<boolean> {
  if (!automaticUpdate.value || savingAutomaticUpdate.value) return false
  savingAutomaticUpdate.value = true
  try {
    automaticUpdate.value = await api.settings.automaticUpdate.update({
      enabled,
      channel,
      expectedResourceVersion: automaticUpdate.value.resourceVersion,
    })
    toast.success(successMessage)
    return true
  } catch (reason) {
    toast.danger('自动更新设置失败', reason instanceof ApiError ? reason.message : '请刷新后重试。')
    return false
  } finally {
    savingAutomaticUpdate.value = false
  }
}

async function saveAutomaticUpdate(enabled: boolean): Promise<void> {
  if (!automaticUpdate.value) return
  await saveAutomaticUpdatePolicy(
    enabled,
    automaticUpdate.value.channel,
    enabled ? '自动安装已启用' : '自动安装已关闭',
  )
}

async function toggleAutomaticUpdate(event: Event): Promise<void> {
  const input = event.currentTarget as HTMLInputElement
  await saveAutomaticUpdate(input.checked)
  input.checked = automaticUpdate.value?.enabled ?? false
}

async function togglePreviewProgram(event: Event): Promise<void> {
  const input = event.currentTarget as HTMLInputElement
  if (!automaticUpdate.value) return
  const nextChannel = input.checked ? 'preview' : 'stable'
  if (nextChannel === 'preview' && typeof globalThis.confirm === 'function' && !globalThis.confirm(
    '预览版可能包含尚未充分验证的功能。加入后只会切换更新来源，不会自动安装；是否继续？',
  )) {
    input.checked = false
    return
  }
  const saved = await saveAutomaticUpdatePolicy(
    automaticUpdate.value.enabled,
    nextChannel,
    nextChannel === 'preview'
      ? '已加入预览版计划'
      : '已切换到稳定版通道；当前版本不会自动降级',
  )
  input.checked = automaticUpdate.value?.channel === 'preview'
  if (saved) await checkAutomaticUpdate(false)
}

async function checkAutomaticUpdate(showToast = true): Promise<AutomaticUpdateStatus | undefined> {
  if (checkingAutomaticUpdate.value || savingAutomaticUpdate.value) return undefined
  checkingAutomaticUpdate.value = true
  try {
    const status = await api.settings.automaticUpdate.check()
    automaticUpdate.value = status
    if (showToast) toast.success(`${automaticUpdateChannelLabel.value}检查完成`)
    return status
  } catch (reason) {
    toast.danger(`${automaticUpdateChannelLabel.value}检查失败`, reason instanceof ApiError ? reason.message : '请检查网络后重试。')
    return undefined
  } finally {
    checkingAutomaticUpdate.value = false
  }
}

async function installAutomaticUpdate(): Promise<void> {
  const status = automaticUpdate.value
  if (!status?.canInstall || installingAutomaticUpdate.value || savingAutomaticUpdate.value) return
  installingAutomaticUpdate.value = true
  try {
    automaticUpdate.value = await api.settings.automaticUpdate.install({
      expectedResourceVersion: status.resourceVersion,
    })
    stopKPanelReleaseRequest()
    kpanelUpdateDialogOpen.value = false
    toast.success('更新任务已启动', '关闭网页不会中断宿主机更新。')
  } catch (reason) {
    toast.danger('立即安装失败', reason instanceof ApiError ? reason.message : '请刷新状态后重试。')
  } finally {
    installingAutomaticUpdate.value = false
  }
}

function stopKPanelReleaseRequest(): void {
  kpanelReleaseController?.abort()
  kpanelReleaseController = undefined
  if (kpanelReleaseTimer !== undefined) clearTimeout(kpanelReleaseTimer)
  kpanelReleaseTimer = undefined
  kpanelReleaseLoading.value = false
}

async function loadKPanelRelease(status = automaticUpdate.value): Promise<void> {
  if (!status?.candidateVersion || !status.candidateImageDigest) return
  stopKPanelReleaseRequest()
  const embedded = releaseFromAutomaticUpdate(status)
  if (embedded) kpanelRelease.value = embedded
  if (embedded?.notes?.length || embedded?.upgradeNotes?.length) {
    kpanelReleaseError.value = ''
    return
  }

  const controller = new AbortController()
  kpanelReleaseController = controller
  kpanelReleaseLoading.value = true
  kpanelReleaseError.value = ''
  kpanelReleaseTimer = setTimeout(() => controller.abort(), 6_000)
  try {
    const release = await api.settings.kpanelRelease.get(status.channel, controller.signal)
    if (
      kpanelReleaseController === controller &&
      automaticUpdate.value?.candidateImageDigest === status.candidateImageDigest
    ) {
      kpanelRelease.value = release
    }
  } catch (reason) {
    if (kpanelReleaseController === controller) {
      kpanelReleaseError.value = reason instanceof ApiError
        ? reason.message
        : i18n.t('kpanelUpdate.loadFailed')
    }
  } finally {
    if (kpanelReleaseController === controller) stopKPanelReleaseRequest()
  }
}

function openKPanelUpdateDialog(status: AutomaticUpdateStatus): void {
  kpanelRelease.value = releaseFromAutomaticUpdate(status)
  kpanelReleaseError.value = ''
  kpanelUpdateDialogOpen.value = true
  void loadKPanelRelease(status)
}

function closeKPanelUpdateDialog(): void {
  if (installingAutomaticUpdate.value) return
  stopKPanelReleaseRequest()
  kpanelUpdateDialogOpen.value = false
}

async function manuallyUpdateKPanel(): Promise<void> {
  const status = automaticUpdate.value
  if (
    !status ||
    manuallyUpdatingKPanel.value ||
    checkingAutomaticUpdate.value ||
    installingAutomaticUpdate.value ||
    savingAutomaticUpdate.value ||
    status.state === 'queued' ||
    status.state === 'updating'
  ) return

  manuallyUpdatingKPanel.value = true
  try {
    const checked = status.canInstall ? status : await checkAutomaticUpdate(false)
    if (!checked) return
    if (!checked.canInstall) {
      toast.show('当前没有可手动安装的更新', {
        message: checked.state === 'blocked'
          ? '该候选版本因上次更新失败已暂停。'
          : `${automaticUpdateChannelLabel.value}已是最新版本。`,
      })
      return
    }
    openKPanelUpdateDialog(checked)
  } finally {
    manuallyUpdatingKPanel.value = false
  }
}

async function focusAutomaticUpdateSection(): Promise<void> {
  if (!isKPanelUpdateSettingsIntent(route.query.section)) return
  settingsSearch.value = ''
  activeSettingsCategory.value = 'system'
  await nextTick()
  // Vue Router restores the destination scroll position after the view mounts.
  // Wait for that pass so it cannot overwrite this section-level navigation.
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  const navigationTarget = settingsBrowser.value || automaticUpdateSection.value
  navigationTarget?.scrollIntoView?.({ block: 'start' })
  automaticUpdateSection.value?.focus({ preventScroll: true })
}

async function changePassword(): Promise<void> {
  passwordSubmitted.value = true
  if (!canChangePassword.value || changingPassword.value) return

  changingPassword.value = true
  try {
    await api.settings.changePassword(passwordForm.currentPassword, passwordForm.newPassword)
  } catch (reason) {
    toast.danger('密码修改失败', reason instanceof ApiError ? reason.message : '请确认当前密码后重试。')
    changingPassword.value = false
    return
  }

  passwordForm.currentPassword = ''
  passwordForm.newPassword = ''
  passwordForm.confirmPassword = ''
  passwordSubmitted.value = false
  changingPassword.value = false

  toast.success('密码已修改', '请使用新密码重新登录。')
  await endAuthenticatedSession()
}

async function changeUsername(): Promise<void> {
  usernameSubmitted.value = true
  if (!canChangeUsername.value || changingUsername.value) return

  changingUsername.value = true
  try {
    await api.settings.changeUsername(usernameForm.currentPassword, usernameForm.newUsername)
  } catch (reason) {
    toast.danger('用户名修改失败', reason instanceof ApiError ? reason.message : '请确认当前密码和新用户名后重试。')
    changingUsername.value = false
    return
  }

  usernameForm.currentPassword = ''
  usernameSubmitted.value = false
  changingUsername.value = false
  toast.success('用户名已修改', '请使用新用户名重新登录。')
  await endAuthenticatedSession()
}

function unlockUsernamePassword(): void {
  usernamePasswordUnlocked.value = true
}

async function saveSecurityEntry(enabled: boolean, regenerate = false): Promise<void> {
  if (!securityEntry.value || savingSecurityEntry.value) return
  savingSecurityEntry.value = true
  try {
    const updated = await api.settings.securityEntrance.update({
      enabled,
      path: securityEntryPath.value,
      regenerate,
      expectedResourceVersion: securityEntry.value.resourceVersion,
    })
    securityEntry.value = updated
    securityEntryPath.value = updated.path || ''
    toast.success(enabled ? '安全入口已启用' : '安全入口已关闭')
  } catch (reason) {
    toast.danger('安全入口更新失败', reason instanceof ApiError ? reason.message : '请刷新后重试。')
  } finally {
    savingSecurityEntry.value = false
  }
}

async function copySecurityEntry(): Promise<void> {
  if (!securityEntryUrl.value) return
  try {
    await copyText(securityEntryUrl.value)
    toast.success('安全入口已复制')
  } catch {
    toast.danger('复制失败', '请手动选择并复制入口地址。')
  }
}

async function copyText(value: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      // HTTP/IP deployments may not expose the Clipboard API. Fall back to
      // the same local selection flow used elsewhere in KPanel.
    }
  }
  const input = document.createElement('textarea')
  input.value = value
  input.style.position = 'fixed'
  input.style.opacity = '0'
  document.body.appendChild(input)
  input.select()
  const succeeded = document.execCommand('copy')
  input.remove()
  if (!succeeded) throw new Error('copy unavailable')
}

function resetTOTPFlow(): void {
  totpAction.value = 'idle'
  totpEnrollment.value = undefined
  totpQRCode.value = ''
  totpError.value = ''
  recoveryCodes.value = []
  totpForm.currentPassword = ''
  totpForm.code = ''
  totpForm.secondFactor = ''
}

async function startTOTPEnrollment(): Promise<void> {
  if (!totpForm.currentPassword || totpBusy.value) return
  totpBusy.value = true
  totpError.value = ''
  try {
    const enrollment = await api.settings.totp.startEnrollment(totpForm.currentPassword)
    totpEnrollment.value = enrollment
    totpQRCode.value = await QRCode.toDataURL(enrollment.otpauthUri, {
      width: 220,
      margin: 1,
      errorCorrectionLevel: 'M',
      color: { dark: '#09231d', light: '#ffffff' },
    })
    totpForm.currentPassword = ''
    totpAction.value = 'verify'
  } catch (reason) {
    totpError.value = reason instanceof ApiError ? reason.message : '无法开始两步验证配置。'
  } finally {
    totpBusy.value = false
  }
}

async function confirmTOTPEnrollment(): Promise<void> {
  if (!totpEnrollment.value || !/^\d{6}$/.test(totpForm.code) || totpBusy.value) return
  totpBusy.value = true
  totpError.value = ''
  try {
    const result = await api.settings.totp.confirmEnrollment(totpEnrollment.value.id, totpForm.code)
    recoveryCodes.value = result.recoveryCodes
    totpStatus.value = { enabled: true, enabledAt: new Date().toISOString(), recoveryCodesRemaining: result.recoveryCodes.length }
    totpAction.value = 'recovery'
  } catch (reason) {
    totpError.value = reason instanceof ApiError ? reason.message : '验证码校验失败。'
    totpForm.code = ''
  } finally {
    totpBusy.value = false
  }
}

async function submitTOTPManagement(): Promise<void> {
  if (!totpForm.currentPassword || !totpForm.secondFactor || totpBusy.value) return
  totpBusy.value = true
  totpError.value = ''
  try {
    if (totpAction.value === 'rotate') {
      const result = await api.settings.totp.regenerateRecoveryCodes(totpForm.currentPassword, totpForm.secondFactor)
      recoveryCodes.value = result.recoveryCodes
      totpAction.value = 'recovery'
      return
    }
    await api.settings.totp.disable(totpForm.currentPassword, totpForm.secondFactor)
    toast.success('两步验证已关闭', '请使用密码重新登录。')
    await finishTOTPFlow()
  } catch (reason) {
    totpError.value = reason instanceof ApiError ? reason.message : '两步验证操作失败。'
  } finally {
    totpBusy.value = false
  }
}

async function copyRecoveryCodes(): Promise<void> {
  try {
    await copyText(recoveryCodes.value.join('\n'))
    toast.success('恢复码已复制')
  } catch {
    toast.danger('复制失败', '请手动选择并保存恢复码。')
  }
}

async function loadTOTPStatus(): Promise<void> {
  totpStatusError.value = ''
  try {
    totpStatus.value = await api.settings.totp.status()
  } catch (reason) {
    totpStatus.value = undefined
    totpStatusError.value = reason instanceof ApiError ? reason.message : '无法读取两步验证状态。'
  }
}

function downloadRecoveryCodes(): void {
  const blob = new Blob([
    `KPanel 两步验证恢复码\n生成时间：${new Date().toLocaleString()}\n\n${recoveryCodes.value.join('\n')}\n`,
  ], { type: 'text/plain;charset=utf-8' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = 'kpanel-recovery-codes.txt'
  link.click()
  URL.revokeObjectURL(link.href)
}

async function finishTOTPFlow(): Promise<void> {
  await endAuthenticatedSession()
}

async function endAuthenticatedSession(): Promise<void> {
  resetApiSecurityState()
  session.state.authenticated = false
  session.state.user = undefined
  session.state.expiresAt = undefined
  session.state.agent = undefined
  await router.replace({ name: 'login' })
}

onMounted(async () => {
  const [capabilityResult, entranceResult, totpResult, automaticUpdateResult] = await Promise.allSettled([
    api.agent.capabilities(),
    api.settings.securityEntrance.get(),
    api.settings.totp.status(),
    api.settings.automaticUpdate.get(),
  ])
  capabilities.value = capabilityResult.status === 'fulfilled' ? capabilityResult.value : []
  if (entranceResult.status === 'fulfilled') {
    securityEntry.value = entranceResult.value
    securityEntryPath.value = entranceResult.value.path || ''
  }
  if (totpResult.status === 'fulfilled') {
    totpStatus.value = totpResult.value
  } else {
    totpStatusError.value = totpResult.reason instanceof ApiError ? totpResult.reason.message : '无法读取两步验证状态。'
  }
  if (automaticUpdateResult.status === 'fulfilled') {
    automaticUpdate.value = automaticUpdateResult.value
  } else {
    automaticUpdateError.value = automaticUpdateResult.reason instanceof ApiError
      ? automaticUpdateResult.reason.message
      : '无法读取自动更新状态。'
  }
  await focusAutomaticUpdateSection()
})

watch(() => route.query.section, () => {
  void focusAutomaticUpdateSection()
})

onBeforeUnmount(stopKPanelReleaseRequest)
</script>

<template>
  <div class="page page--narrow">
    <PageHeader title="设置" description="管理账户、安全验证和当前设备偏好；宿主机策略仍由 Agent 统一执行。" />

    <section ref="settingsBrowser" class="settings-browser panel-card" aria-label="设置导航">
      <label class="settings-browser__search">
        <Search :size="18" aria-hidden="true" />
        <input
          v-model="settingsSearch"
          type="search"
          placeholder="搜索设置，例如密码、主题或更新"
          aria-label="搜索设置"
          @input="onSettingsSearchInput"
        />
        <button
          v-if="settingsSearch"
          type="button"
          aria-label="清除搜索"
          @click="clearSettingsSearch"
        ><X :size="16" /></button>
      </label>
      <div class="settings-browser__categories" role="tablist" aria-label="设置分类">
        <button
          v-for="category in settingsCategories"
          :key="category.id"
          type="button"
          role="tab"
          :aria-selected="activeSettingsCategory === category.id"
          :class="{ 'is-active': activeSettingsCategory === category.id }"
          @click="selectSettingsCategory(category.id)"
        >
          <span>{{ phrase(category.label) }}</span>
          <small>{{ settingsCategoryCount(category.id) }}</small>
        </button>
      </div>
      <p class="sr-only" role="status" aria-live="polite">
        {{ settingsResultSummary }}
      </p>
    </section>

    <ProblemReportHelp v-show="isSettingsSectionVisible('help')" />

    <section v-show="isSettingsSectionVisible('account-overview')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><ShieldCheck :size="19" /></span>
        <div><h2>管理账户</h2><p>当前登录身份与会话信息</p></div>
        <button class="icon-button" type="button" :disabled="refreshing" title="检查 Agent 连接" aria-label="检查 Agent 连接" @click="refreshAgent">
          <RefreshCw :size="16" :class="{ spin: refreshing }" />
        </button>
      </header>
      <div class="account-card">
        <span class="avatar avatar--large">{{ session.state.user?.username?.slice(0, 1).toUpperCase() || 'A' }}</span>
        <div>
          <strong>{{ session.state.user?.displayName || session.state.user?.username || '管理员' }}</strong>
          <small>{{ session.state.user?.role || 'administrator' }}</small>
        </div>
        <StatusBadge status="connected" label="当前会话" subtle />
      </div>
      <dl class="settings-list">
        <div>
          <dt><Clock3 :size="17" /> Session 到期时间</dt>
          <dd>{{ formatDateTime(session.state.expiresAt) }}</dd>
        </div>
        <div>
          <dt><KeyRound :size="17" /> 身份验证</dt>
          <dd>{{ session.state.user?.totpEnabled ? '已启用 TOTP' : '密码登录' }}</dd>
        </div>
      </dl>
      <p class="settings-note">账户安全设置由 KPanel 本机保存，不依赖 Agent 或 kejilion.sh。</p>
    </section>

    <section v-show="isSettingsSectionVisible('username')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><UserRound :size="19" /></span>
        <div><h2>修改用户名</h2><p>更新当前管理员账户的登录名称</p></div>
      </header>
      <form class="form-stack password-form" novalidate @submit.prevent="changeUsername">
        <label class="field">
          <span>新用户名</span>
          <input
            v-model.trim="usernameForm.newUsername"
            type="text"
            name="new-username"
            autocomplete="username"
            maxlength="32"
            :aria-invalid="usernameSubmitted && (!usernameValid || usernameForm.newUsername === session.state.user?.username)"
            required
          />
          <small>3–32 个字符，以字母或数字开头，可使用点、下划线和连字符。</small>
          <small v-if="usernameSubmitted && usernameForm.newUsername === session.state.user?.username">新用户名不能与当前用户名相同。</small>
        </label>
        <label class="field">
          <span>当前密码</span>
          <input
            v-model="usernameForm.currentPassword"
            type="password"
            name="username-current-password"
            autocomplete="current-password"
            :readonly="!usernamePasswordUnlocked"
            :aria-invalid="usernameSubmitted && usernameForm.currentPassword.length === 0"
            @focus="unlockUsernamePassword"
            @pointerdown="unlockUsernamePassword"
            required
          />
        </label>
        <button class="button button--primary" type="submit" :disabled="changingUsername">
          <LoaderCircle v-if="changingUsername" class="spin" :size="17" />
          <template v-if="changingUsername">正在修改…</template>
          <template v-else>修改用户名</template>
        </button>
      </form>
      <p class="settings-note">修改成功后所有现有会话会立即失效；密码、两步验证和恢复码保持不变。</p>
    </section>

    <section v-show="isSettingsSectionVisible('password')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><KeyRound :size="19" /></span>
        <div><h2>修改密码</h2><p>更新当前管理员账户的登录凭据</p></div>
      </header>
      <form class="form-stack password-form" novalidate @submit.prevent="changePassword">
        <label class="field">
          <span>当前密码</span>
          <input
            v-model="passwordForm.currentPassword"
            type="password"
            name="current-password"
            autocomplete="current-password"
            :aria-invalid="passwordSubmitted && passwordForm.currentPassword.length === 0"
            required
          />
          <small v-if="passwordSubmitted && passwordForm.currentPassword.length === 0">请输入当前密码。</small>
        </label>

        <label class="field">
          <span>新密码</span>
          <input
            v-model="passwordForm.newPassword"
            type="password"
            name="new-password"
            autocomplete="new-password"
            minlength="12"
            :aria-invalid="passwordSubmitted && !passwordChecks.every((item) => item.valid)"
            required
          />
        </label>

        <div class="password-checks" aria-label="新密码要求">
          <span v-for="check in passwordChecks" :key="check.label" :class="{ 'is-valid': check.valid }">
            <i aria-hidden="true" /> {{ check.label }}
          </span>
        </div>

        <label class="field">
          <span>确认新密码</span>
          <input
            v-model="passwordForm.confirmPassword"
            type="password"
            name="confirm-password"
            autocomplete="new-password"
            minlength="12"
            :aria-invalid="passwordSubmitted && passwordForm.newPassword !== passwordForm.confirmPassword"
            required
          />
          <small v-if="passwordSubmitted && passwordForm.newPassword !== passwordForm.confirmPassword">
            两次输入的密码不一致。
          </small>
        </label>

        <button class="button button--primary" type="submit" :disabled="changingPassword">
          <LoaderCircle v-if="changingPassword" class="spin" :size="17" />
          {{ changingPassword ? '正在修改…' : '修改密码' }}
        </button>
      </form>
      <p class="settings-note">修改成功后当前会话将立即失效，需要使用新密码重新登录。</p>
    </section>

    <section v-show="isSettingsSectionVisible('security-entrance')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><ShieldCheck :size="19" /></span>
        <div><h2>登录安全入口</h2><p>隐藏常规登录路径，减少公网扫描与撞库噪声</p></div>
        <StatusBadge
          :status="securityEntry?.enabled ? 'connected' : 'idle'"
          :label="securityEntry?.enabled ? '已启用' : '未启用'"
        />
      </header>
      <div v-if="securityEntry" class="security-entry-form">
        <label class="field">
          <span>入口路径</span>
          <div class="security-entry-input">
            <span>/</span>
            <input v-model="securityEntryPath" type="text" maxlength="48" autocomplete="off" placeholder="panel-xxxxxxxx" />
          </div>
        </label>
        <div class="security-entry-actions">
          <button
            v-if="!securityEntry.enabled"
            class="button button--primary"
            type="button"
            :disabled="savingSecurityEntry"
            @click="saveSecurityEntry(true, !securityEntryPath)"
          >启用安全入口</button>
          <template v-else>
            <button class="button button--secondary" type="button" :disabled="savingSecurityEntry" @click="saveSecurityEntry(true)">保存路径</button>
            <button class="button button--secondary" type="button" :disabled="savingSecurityEntry" @click="saveSecurityEntry(true, true)">重新生成</button>
            <button class="button button--secondary" type="button" @click="copySecurityEntry"><Copy :size="15" />复制入口</button>
            <button class="button button--ghost" type="button" :disabled="savingSecurityEntry" @click="saveSecurityEntry(false)">关闭</button>
          </template>
        </div>
        <code v-if="securityEntryUrl" class="security-entry-url">{{ securityEntryUrl }}</code>
      </div>
      <p v-else class="settings-note">正在读取安全入口状态…</p>
      <p class="settings-note">安全入口是登录验证前的额外门槛，不替代强密码、会话保护和登录限速；请妥善保存入口地址。</p>
    </section>

    <section v-show="isSettingsSectionVisible('totp')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><ShieldCheck :size="19" /></span>
        <div><h2>两步验证</h2><p>兼容主流身份验证器的标准 TOTP，并提供一次性恢复码</p></div>
        <StatusBadge
          :status="totpStatus?.enabled ? 'connected' : 'idle'"
          :label="totpStatus?.enabled ? '已启用' : '未启用'"
        />
      </header>

      <div v-if="!totpStatus" class="totp-panel">
        <div v-if="totpStatusError" class="inline-alert inline-alert--danger">
          <span>{{ totpStatusError }}</span>
          <button class="button-link" type="button" @click="loadTOTPStatus">重新加载</button>
        </div>
        <p v-else>正在读取两步验证状态…</p>
      </div>

      <div v-else-if="totpAction === 'recovery'" class="totp-panel totp-panel--recovery">
        <div class="inline-alert inline-alert--warning">
          恢复码只显示这一次。请保存到密码管理器或离线位置；每个恢复码只能使用一次。
        </div>
        <div class="recovery-code-grid" aria-label="恢复码">
          <code v-for="code in recoveryCodes" :key="code">{{ code }}</code>
        </div>
        <div class="totp-actions">
          <button class="button button--secondary" type="button" @click="copyRecoveryCodes"><Copy :size="15" /> 复制</button>
          <button class="button button--secondary" type="button" @click="downloadRecoveryCodes">下载文本</button>
          <button class="button button--primary" type="button" @click="finishTOTPFlow">已安全保存，重新登录</button>
        </div>
      </div>

      <form v-else-if="totpAction === 'enroll'" class="totp-panel form-stack" @submit.prevent="startTOTPEnrollment">
        <p>启用前需要重新验证当前密码。配置完成后，所有现有会话都会失效。</p>
        <label class="field">
          <span>当前密码</span>
          <input v-model="totpForm.currentPassword" type="password" autocomplete="current-password" required autofocus />
        </label>
        <div v-if="totpError" class="inline-alert inline-alert--danger">{{ totpError }}</div>
        <div class="totp-actions">
          <button class="button button--ghost" type="button" @click="resetTOTPFlow">取消</button>
          <button class="button button--primary" type="submit" :disabled="totpBusy || !totpForm.currentPassword">
            <LoaderCircle v-if="totpBusy" class="spin" :size="16" /> 继续
          </button>
        </div>
      </form>

      <form v-else-if="totpAction === 'verify' && totpEnrollment" class="totp-panel" @submit.prevent="confirmTOTPEnrollment">
        <div class="totp-enrollment">
          <img :src="totpQRCode" width="190" height="190" alt="TOTP 配置二维码" />
          <div class="form-stack">
            <p>使用 Microsoft Authenticator、Google Authenticator、1Password 等应用扫描二维码。</p>
            <label class="field">
              <span>无法扫码时手动输入</span>
              <code class="totp-secret">{{ totpEnrollment.secret }}</code>
            </label>
            <label class="field">
              <span>输入身份验证器中的 6 位验证码</span>
              <input v-model.trim="totpForm.code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="000000" required autofocus />
            </label>
          </div>
        </div>
        <div v-if="totpError" class="inline-alert inline-alert--danger">{{ totpError }}</div>
        <div class="totp-actions">
          <button class="button button--ghost" type="button" @click="resetTOTPFlow">取消</button>
          <button class="button button--primary" type="submit" :disabled="totpBusy || !/^\d{6}$/.test(totpForm.code)">
            <LoaderCircle v-if="totpBusy" class="spin" :size="16" /> 验证并启用
          </button>
        </div>
      </form>

      <form v-else-if="totpAction === 'rotate' || totpAction === 'disable'" class="totp-panel form-stack" @submit.prevent="submitTOTPManagement">
        <div class="inline-alert" :class="totpAction === 'disable' ? 'inline-alert--warning' : 'inline-alert--info'">
          {{ totpAction === 'disable' ? '关闭后账户将恢复为仅密码登录。' : '生成新恢复码后，旧恢复码会立即全部失效。' }}
        </div>
        <label class="field"><span>当前密码</span><input v-model="totpForm.currentPassword" type="password" autocomplete="current-password" required /></label>
        <label class="field"><span>当前验证码或恢复码</span><input v-model.trim="totpForm.secondFactor" type="text" autocomplete="one-time-code" maxlength="17" required /></label>
        <div v-if="totpError" class="inline-alert inline-alert--danger">{{ totpError }}</div>
        <div class="totp-actions">
          <button class="button button--ghost" type="button" @click="resetTOTPFlow">取消</button>
          <button class="button" :class="totpAction === 'disable' ? 'button--danger' : 'button--primary'" type="submit" :disabled="totpBusy">
            <LoaderCircle v-if="totpBusy" class="spin" :size="16" /> {{ totpAction === 'disable' ? '确认关闭' : '生成新恢复码' }}
          </button>
        </div>
      </form>

      <div v-else class="totp-panel totp-summary">
        <template v-if="totpStatus.enabled">
          <dl>
            <div><dt>启用时间</dt><dd>{{ formatDateTime(totpStatus.enabledAt) }}</dd></div>
            <div><dt>剩余恢复码</dt><dd>{{ totpStatus.recoveryCodesRemaining }} 个</dd></div>
          </dl>
          <div class="totp-actions">
            <button class="button button--secondary" type="button" @click="totpAction = 'rotate'">重新生成恢复码</button>
            <button class="button button--ghost" type="button" @click="totpAction = 'disable'">关闭两步验证</button>
          </div>
        </template>
        <template v-else>
          <p>登录时除密码外，还需要身份验证器生成的动态验证码。默认关闭，可随时启用。</p>
          <button class="button button--primary" type="button" @click="totpAction = 'enroll'">启用两步验证</button>
        </template>
      </div>
      <p class="settings-note">验证码每 30 秒更新，允许轻微时钟偏差；已成功使用的验证码和恢复码不能重放。</p>
    </section>

    <PasskeySettings v-show="isSettingsSectionVisible('passkeys')" />

    <section v-show="isSettingsSectionVisible('language')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Languages :size="19" /></span>
        <div><h2>{{ i18n.t('common.language') }}</h2><p>{{ i18n.t('common.languageDescription') }}</p></div>
      </header>
      <div class="theme-options" role="radiogroup" :aria-label="i18n.t('common.language')">
        <button
          v-for="option in i18n.localeOptions"
          :key="option.id"
          type="button"
          role="radio"
          :tabindex="i18n.locale.value === option.id ? 0 : -1"
          :aria-checked="i18n.locale.value === option.id"
          :class="{ 'is-active': i18n.locale.value === option.id }"
          @keydown="moveRadioFocus"
          @click="i18n.setLocale(option.id)"
        >
          <span><Languages :size="19" /></span>
          <strong>{{ localeLabel(option.id) }}</strong>
          <small>{{ option.id }}</small>
          <Check v-if="i18n.locale.value === option.id" class="theme-options__check" :size="17" aria-hidden="true" />
        </button>
      </div>
    </section>

    <section v-show="isSettingsSectionVisible('appearance')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Palette :size="19" /></span>
        <div><h2>外观与配色</h2><p>自定义颜色和明暗模式仅保存在当前浏览器</p></div>
      </header>
      <div class="appearance-group">
        <div class="appearance-group__header">
          <h3>自定义配色</h3>
          <p>选择视觉意图，系统自动生成可读的完整浅色与深色色板</p>
        </div>
        <div class="theme-color-preset-section">
          <div class="theme-color-preset-section__header">
            <strong>推荐方案</strong>
            <span>选择一套作为起点，下面仍可完全自定义每个颜色值</span>
          </div>
          <div class="theme-color-presets" role="radiogroup" aria-label="推荐配色方案">
            <button
              v-for="(preset, index) in THEME_COLOR_PRESETS"
              :key="preset.id"
              type="button"
              role="radio"
              :tabindex="activeThemePresetId === preset.id || (!activeThemePresetId && index === 0) ? 0 : -1"
              :aria-checked="activeThemePresetId === preset.id"
              :class="{ 'is-active': activeThemePresetId === preset.id }"
              @keydown="moveRadioFocus"
              @click="selectThemeColorPreset(preset.colors)"
            >
              <span class="theme-color-preset__sample" :style="themeColorPresetStyle(preset.colors)" aria-hidden="true">
                <i class="theme-color-preset__sidebar" />
                <i class="theme-color-preset__canvas"><b /><b /><em /><u /></i>
              </span>
              <span class="theme-color-preset__copy">
                <strong>{{ preset.label }}</strong>
                <small>{{ preset.description }}</small>
              </span>
              <Check v-if="activeThemePresetId === preset.id" class="theme-color-preset__check" :size="16" aria-hidden="true" />
            </button>
          </div>
        </div>
        <div class="theme-color-studio">
          <div class="theme-color-controls">
            <template v-for="field in themeColorFields" :key="field.key">
              <div v-if="field.key !== 'signature' || !colorDraft.signatureLinked" class="theme-color-field">
                <div class="theme-color-field__copy">
                  <label :for="`theme-${field.key}-hex`">{{ field.label }}</label>
                  <p>{{ field.description }}</p>
                </div>
                <div class="theme-color-field__inputs">
                  <input
                    class="theme-color-field__picker"
                    type="color"
                    :value="colorDraft[field.key]"
                    :aria-label="field.pickerLabel"
                    @input="updateThemeColor(field.key, $event)"
                  >
                  <input
                    :id="`theme-${field.key}-hex`"
                    class="input theme-color-field__hex"
                    type="text"
                    inputmode="text"
                    maxlength="7"
                    spellcheck="false"
                    :value="colorInputs[field.key]"
                    :aria-invalid="Boolean(colorErrors[field.key])"
                    :aria-describedby="colorErrors[field.key] ? `theme-${field.key}-error` : undefined"
                    @input="updateThemeColor(field.key, $event)"
                  >
                </div>
                <small v-if="colorErrors[field.key]" :id="`theme-${field.key}-error`" class="field-error">{{ colorErrors[field.key] }}</small>
              </div>
            </template>

            <label class="theme-color-link">
              <input v-model="colorDraft.signatureLinked" type="checkbox">
              <span><strong>点缀色跟随主题色</strong><small>关闭后可单独设置少量身份标记的颜色</small></span>
            </label>
          </div>

          <div class="theme-color-preview" :style="colorPreviewStyle">
            <header class="theme-color-preview__header">
              <div><strong>局部预览</strong><small>应用前不会改变整页</small></div>
              <div class="theme-color-preview__modes" role="radiogroup" aria-label="配色预览模式">
                <button
                  v-for="option in colorPreviewModes"
                  :key="option.id"
                  type="button"
                  role="radio"
                  :tabindex="colorPreviewMode === option.id ? 0 : -1"
                  :aria-checked="colorPreviewMode === option.id"
                  :class="{ 'is-active': colorPreviewMode === option.id }"
                  @keydown="moveRadioFocus"
                  @click="colorPreviewMode = option.id"
                >{{ option.label }}</button>
              </div>
            </header>
            <div class="theme-color-preview__shell" aria-hidden="true">
              <aside>
                <i class="theme-color-preview__brand">K</i>
                <span class="is-current"><i />概览</span>
                <span><i />网站</span>
                <span><i />文件</span>
              </aside>
              <main>
                <div class="theme-color-preview__title"><span><i /></span><div><strong>运行概览</strong><small>稳定、清晰的内容层级</small></div></div>
                <div class="theme-color-preview__cards"><i /><i /><i /></div>
                <div class="theme-color-preview__row"><span class="theme-color-preview__action">主要操作</span><span class="theme-color-preview__status">运行正常</span></div>
              </main>
            </div>
          </div>
        </div>
        <div class="theme-color-actions">
          <p aria-live="polite">界面基调的明暗与饱和度会分别映射到整体层级，同时自动校正文字对比度；状态绿、警告橙和危险红不受影响。</p>
          <div>
            <button class="button button--ghost" type="button" :disabled="!theme.isCustom.value && !colorDraftDirty" @click="resetThemeColors">恢复默认配色</button>
            <button class="button button--secondary" type="button" :disabled="!colorDraftDirty" @click="cancelThemeColorChanges">取消更改</button>
            <button class="button button--primary" type="button" :disabled="hasColorErrors || !colorDraftDirty" @click="applyThemeColors">应用配色</button>
          </div>
        </div>
      </div>
      <div class="appearance-group">
        <div class="appearance-group__header">
          <h3>明暗模式</h3>
          <p>自定义配色会分别生成浅色、深色，并支持跟随系统</p>
        </div>
        <div class="theme-options" role="radiogroup" aria-label="明暗模式">
          <button
            v-for="option in themeModes"
            :key="option.id"
            type="button"
            role="radio"
            :tabindex="theme.preference.value === option.id ? 0 : -1"
            :aria-checked="theme.preference.value === option.id"
            :class="{ 'is-active': theme.preference.value === option.id }"
            @keydown="moveRadioFocus"
            @click="theme.setTheme(option.id)"
          >
            <span><component :is="option.icon" :size="19" /></span>
            <strong>{{ option.label }}</strong>
            <small>{{ option.description }}</small>
            <Check v-if="theme.preference.value === option.id" class="theme-options__check" :size="17" aria-hidden="true" />
          </button>
        </div>
      </div>
    </section>

    <section v-show="isSettingsSectionVisible('wallpaper')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><ImageIcon :size="19" /></span>
        <div><h2>壁纸</h2><p>桌面模式与经典模式共用一张壁纸，选择后同时套用它的配色</p></div>
      </header>
      <div class="appearance-group">
        <DesktopWallpaperPicker
          :visible="isSettingsSectionVisible('wallpaper')"
          @select="(id, pack) => desktopWallpaper.select(id, pack)"
        />
      </div>
      <div class="appearance-group">
        <div class="appearance-group__header">
          <h3>经典模式透出</h3>
          <p>让上面的壁纸透到经典模式页面背后；3D 场景会继续播放，系统要求减少动态效果时显示封面</p>
        </div>
        <div class="theme-options" role="radiogroup" aria-label="经典模式透出">
          <button
            v-for="option in classicWallpaperLevels"
            :key="option.id"
            type="button"
            role="radio"
            :tabindex="classicWallpaper.level.value === option.id ? 0 : -1"
            :aria-checked="classicWallpaper.level.value === option.id"
            :class="{ 'is-active': classicWallpaper.level.value === option.id }"
            @keydown="moveRadioFocus"
            @click="classicWallpaper.setLevel(option.id)"
          >
            <span><component :is="option.icon" :size="19" /></span>
            <strong>{{ option.label }}</strong>
            <small>{{ option.description }}</small>
            <Check v-if="classicWallpaper.level.value === option.id" class="theme-options__check" :size="17" aria-hidden="true" />
          </button>
        </div>
      </div>
    </section>

    <BackupCenter v-show="isSettingsSectionVisible('backup')" />
    <MCPAccess v-show="isSettingsSectionVisible('mcp')" />

    <section
      id="version-updates"
      ref="automaticUpdateSection"
      v-show="isSettingsSectionVisible('version-updates')"
      class="settings-section panel-card automatic-update-section"
      data-testid="release-update-settings"
      tabindex="-1"
      aria-labelledby="version-updates-title"
    >
      <header class="settings-section__header">
        <span><RefreshCw :size="19" /></span>
        <div><h2 id="version-updates-title">版本更新</h2><p>稳定版默认，预览版自愿加入；更新通道与自动安装相互独立</p></div>
        <StatusBadge v-if="automaticUpdate" :status="automaticUpdateState.status" :label="automaticUpdateState.label" />
      </header>
      <div v-if="automaticUpdate" class="automatic-update-panel">
        <label class="automatic-update-switch automatic-update-switch--preview">
          <span>
            <strong>加入预览版计划</strong>
            <small>接收正式稳定版和更高版本的 RC 预览版；不会因为勾选而自动安装。</small>
          </span>
          <input
            data-testid="preview-program-toggle"
            type="checkbox"
            role="switch"
            :checked="automaticUpdate.channel === 'preview'"
            :disabled="savingAutomaticUpdate || checkingAutomaticUpdate || manuallyUpdatingKPanel || automaticUpdate.state === 'queued' || automaticUpdate.state === 'updating'"
            @change="togglePreviewProgram"
          />
        </label>
        <label class="automatic-update-switch">
          <span>
            <strong>自动安装更新</strong>
            <small>默认关闭；启用后只自动安装当前通道中已通过观察期的版本。</small>
          </span>
          <input
            data-testid="automatic-install-toggle"
            type="checkbox"
            role="switch"
            :checked="automaticUpdate.enabled"
            :disabled="savingAutomaticUpdate || checkingAutomaticUpdate || manuallyUpdatingKPanel || automaticUpdate.state === 'queued' || automaticUpdate.state === 'updating'"
            @change="toggleAutomaticUpdate"
          />
        </label>
        <div v-if="automaticUpdate.channel === 'preview'" class="inline-alert inline-alert--warning">
          你已加入预览版计划。预览版可能存在兼容性或稳定性问题；退出计划只切回稳定版来源，不会自动降级当前版本。
        </div>
        <dl class="settings-list automatic-update-details">
          <div><dt>更新通道</dt><dd>{{ automaticUpdateChannelLabel }}</dd></div>
          <div><dt>当前版本</dt><dd>{{ automaticUpdate.currentVersion || '—' }}</dd></div>
          <div><dt>候选版本</dt><dd>{{ automaticUpdate.candidateVersion || '—' }}</dd></div>
          <div><dt>候选镜像摘要</dt><dd class="automatic-update-digest">{{ automaticUpdate.candidateImageDigest || '—' }}</dd></div>
          <div><dt>最后检查</dt><dd>{{ formatDateTime(automaticUpdate.lastCheckedAt) }}</dd></div>
          <div><dt>最近成功更新</dt><dd>{{ formatDateTime(automaticUpdate.lastSuccessAt) }}</dd></div>
        </dl>
        <div v-if="automaticUpdateNotice" class="inline-alert" :class="automaticUpdate.state === 'failed' ? 'inline-alert--warning' : 'inline-alert--info'">
          {{ automaticUpdateNotice }}
        </div>
        <div class="automatic-update-actions">
          <button
            data-testid="check-release-update"
            class="button button--secondary"
            type="button"
            :disabled="checkingAutomaticUpdate || savingAutomaticUpdate || installingAutomaticUpdate || manuallyUpdatingKPanel || automaticUpdate.state === 'queued' || automaticUpdate.state === 'updating'"
            @click="checkAutomaticUpdate()"
          >
            <LoaderCircle v-if="checkingAutomaticUpdate" class="spin" :size="15" />
            <RefreshCw v-else :size="15" />
            立即检查
          </button>
          <button
            data-testid="manual-release-update"
            class="button button--primary"
            type="button"
            :disabled="installingAutomaticUpdate || savingAutomaticUpdate || checkingAutomaticUpdate || manuallyUpdatingKPanel || automaticUpdate.state === 'queued' || automaticUpdate.state === 'updating'"
            @click="manuallyUpdateKPanel"
          >
            <LoaderCircle v-if="installingAutomaticUpdate || manuallyUpdatingKPanel" class="spin" :size="15" />
            <Download v-else :size="15" />
            {{ manualUpdateButtonLabel }}
          </button>
        </div>
        <p class="settings-note">{{ automaticUpdateScheduleNote }}</p>
      </div>
      <div v-else-if="automaticUpdateError" class="automatic-update-unavailable">
        <div class="inline-alert inline-alert--warning">{{ automaticUpdateError }}</div>
        <RouterLink class="button button--secondary" :to="kpanelAppUpdatePath">
          <Download :size="15" />
          前往应用市场手动更新
        </RouterLink>
      </div>
      <p v-else class="settings-note">正在读取自动更新状态…</p>
    </section>

    <section v-show="isSettingsSectionVisible('agent')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Server :size="19" /></span>
        <div><h2>宿主机 Agent</h2><p>面板唯一的特权操作边界</p></div>
        <StatusBadge :status="agentState.status" :label="agentState.label" />
      </header>
      <dl class="settings-list settings-list--agent">
        <div>
          <dt>Agent 版本</dt>
          <dd>{{ panel.state.agent?.version || '—' }}</dd>
        </div>
        <div>
          <dt>协议版本</dt>
          <dd>{{ panel.state.agent?.protocolVersion || '—' }}</dd>
        </div>
        <div>
          <dt>最后检查</dt>
          <dd>{{ relativeTimeLabel(panel.state.agent?.lastSeenAt) }}</dd>
        </div>
        <div>
          <dt>已开放能力</dt>
          <dd>{{ capabilities.filter((item) => item.enabled).length }} / {{ capabilities.length }}</dd>
        </div>
      </dl>
      <div v-if="panel.state.agent?.reason" class="inline-alert inline-alert--warning">{{ panel.state.agent.reason }}</div>
      <p class="settings-note">
        Web 容器不挂载 Docker Socket 或宿主机根目录；Agent 只通过本地 Unix Socket 接收类型化动作。
      </p>
    </section>

    <section v-show="isSettingsSectionVisible('license')" class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Scale :size="19" /></span>
        <div><h2>开源许可</h2><p>GNU AGPL v3.0 only</p></div>
      </header>
      <p class="settings-note">
        KPanel 源代码采用 AGPL-3.0-only；第三方组件继续使用各自的原始许可。
      </p>
      <div class="license-actions">
        <a
          class="button button--ghost"
          href="https://github.com/kejilion/KPanel"
          target="_blank"
          rel="noopener noreferrer"
        >
          查看源码 <ExternalLink :size="15" />
        </a>
        <a
          class="button button--ghost"
          href="https://github.com/kejilion/KPanel/blob/main/LICENSE"
          target="_blank"
          rel="noopener noreferrer"
        >
          查看许可协议 <ExternalLink :size="15" />
        </a>
      </div>
    </section>

    <div v-if="visibleSettingsSectionCount === 0" class="settings-empty panel-card" role="status">
      <Search :size="24" aria-hidden="true" />
      <div>
        <strong>没有找到匹配的设置</strong>
        <p>尝试其他关键词，或清除当前分类和搜索条件。</p>
      </div>
      <button class="button button--secondary" type="button" @click="resetSettingsFilters">查看全部设置</button>
    </div>

    <KPanelUpdateDialog
      :open="kpanelUpdateDialogOpen"
      :release="kpanelRelease"
      :loading="kpanelReleaseLoading"
      :error="kpanelReleaseError"
      :current-version="automaticUpdate?.currentVersion"
      :target-version="automaticUpdate?.candidateVersion"
      :target-digest="automaticUpdate?.candidateImageDigest"
      :channel="automaticUpdate?.channel"
      :busy="installingAutomaticUpdate"
      strategy="automatic"
      @close="closeKPanelUpdateDialog"
      @retry="loadKPanelRelease()"
      @confirm="installAutomaticUpdate"
    />
  </div>
</template>

<style scoped>
.settings-browser {
  display: grid;
  gap: 13px;
  padding: 16px;
  scroll-margin-top: calc(var(--topbar-height) + 16px);
}

:global(.desktop-window__body) .settings-browser {
  scroll-margin-top: 16px;
}

.settings-browser__search {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 0 12px;
  border: 1px solid var(--control-border);
  border-radius: var(--radius-md);
  background: var(--surface-raised);
  color: var(--muted);
  transition: border-color 160ms ease, box-shadow 160ms ease;
}

.settings-browser__search:focus-within {
  border-color: var(--brand);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 14%, transparent);
}

.settings-browser__search input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
}

.settings-browser__search input::placeholder {
  color: var(--muted);
}

.settings-browser__search input::-webkit-search-cancel-button {
  display: none;
}

.settings-browser__search button {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--muted);
  cursor: pointer;
}

.settings-browser__search button:hover {
  background: var(--surface-soft);
  color: var(--text);
}

.settings-browser__categories {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.settings-browser__categories button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 36px;
  padding: 0 11px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface-raised);
  color: var(--muted-strong);
  font: inherit;
  font-size: 0.88rem;
  cursor: pointer;
  transition: border-color 160ms ease, background 160ms ease, color 160ms ease;
}

.settings-browser__categories button:hover {
  border-color: color-mix(in srgb, var(--brand) 38%, var(--line));
  color: var(--text);
}

.settings-browser__categories button.is-active {
  border-color: color-mix(in srgb, var(--brand) 45%, var(--line));
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.settings-browser__categories small {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 9%, transparent);
  color: inherit;
  font-size: 0.75rem;
  line-height: 18px;
  text-align: center;
}

.settings-empty {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 22px;
  color: var(--muted);
}

.settings-empty > svg {
  flex: 0 0 auto;
  color: var(--brand);
}

.settings-empty > div {
  flex: 1;
}

.settings-empty strong {
  color: var(--text);
}

.settings-empty p {
  margin: 4px 0 0;
}

.password-form {
  max-width: 560px;
  padding: 18px;
}

.password-form > .button {
  justify-self: start;
  min-width: 132px;
}

.license-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 14px;
}

.security-entry-form {
  display: grid;
  gap: 12px;
  max-width: 760px;
  padding: 18px;
}

.security-entry-input {
  display: flex;
  align-items: center;
  gap: 7px;
}

.security-entry-input input {
  flex: 1;
}

.security-entry-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
}

.security-entry-url {
  overflow-wrap: anywhere;
  color: var(--brand);
}

.automatic-update-panel {
  display: grid;
  gap: 14px;
}

.automatic-update-section {
  scroll-margin-top: calc(var(--topbar-height) + 16px);
}

:global(.desktop-window__body) .automatic-update-section {
  scroll-margin-top: 16px;
}

.automatic-update-section:focus {
  outline: none;
}

.automatic-update-section:focus-visible {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 34%, transparent), var(--shadow-sm);
}

.automatic-update-unavailable {
  display: grid;
  gap: 12px;
  padding: 16px 18px 18px;
}

.automatic-update-unavailable .button {
  width: fit-content;
}

.automatic-update-switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 16px 18px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--surface-soft);
}

.automatic-update-switch > span {
  display: grid;
  gap: 4px;
}

.automatic-update-switch small {
  color: var(--muted);
}

.automatic-update-switch--preview {
  border-color: color-mix(in srgb, var(--warning) 34%, var(--line));
  background: color-mix(in srgb, var(--warning-soft) 45%, var(--surface-soft));
}

.automatic-update-switch input {
  position: relative;
  flex: 0 0 auto;
  width: 44px;
  height: 24px;
  margin: 0;
  appearance: none;
  border: 1px solid var(--control-border);
  border-radius: 999px;
  background: var(--surface-raised);
  cursor: pointer;
  transition: background 160ms ease, border-color 160ms ease;
}

.automatic-update-switch input::after {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--muted);
  content: '';
  transition: transform 160ms ease, background 160ms ease;
}

.automatic-update-switch input:checked {
  border-color: var(--brand);
  background: var(--brand-soft);
}

.automatic-update-switch input:checked::after {
  background: var(--brand);
  transform: translateX(20px);
}

.automatic-update-switch input:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.automatic-update-switch input:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.automatic-update-details {
  margin: 0;
}

.automatic-update-digest {
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  font-size: 0.82rem;
}

.automatic-update-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
  justify-content: flex-start;
}

@media (max-width: 640px) {
  .settings-browser {
    padding: 14px;
  }

  .settings-browser__categories {
    flex-wrap: nowrap;
    overflow-x: auto;
    padding-bottom: 2px;
    scrollbar-width: none;
  }

  .settings-browser__categories::-webkit-scrollbar {
    display: none;
  }

  .settings-browser__categories button {
    flex: 0 0 auto;
  }

  .settings-empty {
    align-items: flex-start;
    flex-wrap: wrap;
    padding: 18px;
  }

  .settings-empty .button {
    width: 100%;
  }

  .password-form {
    max-width: none;
    padding: 14px;
  }

  .password-form > .button {
    width: 100%;
  }

  .license-actions .button {
    width: 100%;
  }

  .security-entry-form {
    padding: 14px;
  }

  .security-entry-input {
    align-items: stretch;
    flex-direction: column;
  }

  .security-entry-actions .button {
    flex: 1 1 140px;
  }

  .automatic-update-switch {
    align-items: flex-start;
    padding: 14px;
  }

  .automatic-update-actions .button {
    width: 100%;
  }

  .automatic-update-unavailable .button {
    width: 100%;
  }
}
</style>

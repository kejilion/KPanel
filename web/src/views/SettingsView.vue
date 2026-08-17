<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/SettingsView/en-US').then((module) => module.default)
  : import('@/i18n/pages/SettingsView/zh-TW').then((module) => module.default))
import QRCode from 'qrcode'
import {
  Check,
  Clock3,
  Copy,
  ExternalLink,
  KeyRound,
  Languages,
  LoaderCircle,
  Monitor,
  Moon,
  RefreshCw,
  Scale,
  Server,
  ShieldCheck,
  Sun,
  UserRound,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api, resetApiSecurityState } from '@/lib/api'
import { formatDateTime, relativeTime } from '@/lib/format'
import { usePanelState } from '@/stores/panel'
import { useSession } from '@/stores/session'
import { useTheme, type ThemePreference } from '@/stores/theme'
import { useToast } from '@/stores/toast'
import { useI18n, type SupportedLocale } from '@/i18n'
import type { TOTPEnrollment, TOTPStatus } from '@/types/api'

const router = useRouter()
const session = useSession()
const panel = usePanelState()
const theme = useTheme()
const toast = useToast()
const i18n = useI18n()

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

const themes: Array<{ id: ThemePreference; label: string; description: string; icon: typeof Sun }> = [
  { id: 'light', label: '浅色', description: '始终使用明亮界面', icon: Sun },
  { id: 'dark', label: '深色', description: '始终使用低亮度界面', icon: Moon },
  { id: 'system', label: '跟随系统', description: '随设备设置自动切换', icon: Monitor },
]

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
  const [capabilityResult, entranceResult, totpResult] = await Promise.allSettled([
    api.agent.capabilities(),
    api.settings.securityEntrance.get(),
    api.settings.totp.status(),
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
})
</script>

<template>
  <div class="page page--narrow">
    <PageHeader title="设置" description="管理账户、安全验证和当前设备偏好；宿主机策略仍由 Agent 统一执行。" />

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Languages :size="19" /></span>
        <div><h2>{{ i18n.t('common.language') }}</h2><p>{{ i18n.t('common.languageDescription') }}</p></div>
      </header>
      <div class="theme-options">
        <button
          v-for="option in i18n.localeOptions"
          :key="option.id"
          type="button"
          :class="{ 'is-active': i18n.locale.value === option.id }"
          @click="i18n.setLocale(option.id)"
        >
          <span><Languages :size="19" /></span>
          <strong>{{ localeLabel(option.id) }}</strong>
          <small>{{ option.id }}</small>
          <Check v-if="i18n.locale.value === option.id" class="theme-options__check" :size="17" />
        </button>
      </div>
    </section>

    <section class="settings-section panel-card">
      <header class="settings-section__header">
        <span><Sun :size="19" /></span>
        <div><h2>界面主题</h2><p>仅保存在当前浏览器，不上传服务器</p></div>
      </header>
      <div class="theme-options">
        <button
          v-for="option in themes"
          :key="option.id"
          type="button"
          :class="{ 'is-active': theme.preference.value === option.id }"
          @click="theme.setTheme(option.id)"
        >
          <span><component :is="option.icon" :size="19" /></span>
          <strong>{{ option.label }}</strong>
          <small>{{ option.description }}</small>
          <Check v-if="theme.preference.value === option.id" class="theme-options__check" :size="17" />
        </button>
      </div>
    </section>

    <section class="settings-section panel-card">
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

    <section class="settings-section panel-card">
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
  </div>
</template>

<style scoped>
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

@media (max-width: 640px) {
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
}
</style>

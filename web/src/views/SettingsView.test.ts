import { readFileSync } from 'node:fs'
import { createSSRApp, ssrContextKey, type ComputedRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { THEME_COLOR_PRESETS, type ThemeColorIntent, type ThemeColorKey, type ThemeMode } from '@/theme/colors'
import type { KPanelReleaseInfo } from '@/types/api'
import SettingsView from './SettingsView.vue'

const settingsSource = readFileSync(new URL('./SettingsView.vue', import.meta.url), 'utf8')

const mocks = vi.hoisted(() => {
  class MockApiError extends Error {}

  return {
    MockApiError,
    changePassword: vi.fn(),
    changeUsername: vi.fn(),
    getSecurityEntrance: vi.fn(),
    updateSecurityEntrance: vi.fn(),
    getTOTPStatus: vi.fn(),
	getAutomaticUpdate: vi.fn(),
	updateAutomaticUpdate: vi.fn(),
	checkAutomaticUpdate: vi.fn(),
	installAutomaticUpdate: vi.fn(),
	getKPanelRelease: vi.fn(),
    startTOTPEnrollment: vi.fn(),
    confirmTOTPEnrollment: vi.fn(),
    rotateRecoveryCodes: vi.fn(),
    disableTOTP: vi.fn(),
    resetApiSecurityState: vi.fn(),
    replace: vi.fn(),
    route: { query: {} as Record<string, unknown> },
    toastSuccess: vi.fn(),
    toastDanger: vi.fn(),
    toastShow: vi.fn(),
    themeSetTheme: vi.fn(),
    themeSetColors: vi.fn(),
    themeResetColors: vi.fn(),
    themePreference: { value: 'system' },
    themeResolved: { value: 'light' },
    themeColors: {
      value: {
        brand: '#0c7a60',
        neutral: '#52645f',
        signatureLinked: true,
        signature: '#0c7a60',
      },
    },
    themeIsCustom: { value: false },
    sessionState: {
      authenticated: true,
      user: { id: 'admin', username: 'admin' } as { id: string; username: string } | undefined,
      expiresAt: '2026-07-26T00:00:00Z' as string | undefined,
      agent: { connected: true } as { connected: boolean } | undefined,
    },
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('@/lib/api', () => ({
  ApiError: mocks.MockApiError,
  api: {
    agent: {
      capabilities: vi.fn().mockResolvedValue([]),
      health: vi.fn(),
    },
    settings: {
      changePassword: mocks.changePassword,
      changeUsername: mocks.changeUsername,
      securityEntrance: {
        get: mocks.getSecurityEntrance,
        update: mocks.updateSecurityEntrance,
      },
      totp: {
        status: mocks.getTOTPStatus,
        startEnrollment: mocks.startTOTPEnrollment,
        confirmEnrollment: mocks.confirmTOTPEnrollment,
        regenerateRecoveryCodes: mocks.rotateRecoveryCodes,
        disable: mocks.disableTOTP,
      },
      automaticUpdate: {
        get: mocks.getAutomaticUpdate,
        update: mocks.updateAutomaticUpdate,
        check: mocks.checkAutomaticUpdate,
        install: mocks.installAutomaticUpdate,
      },
      kpanelRelease: {
        get: mocks.getKPanelRelease,
      },
    },
  },
  resetApiSecurityState: mocks.resetApiSecurityState,
}))

vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn().mockResolvedValue('data:image/png;base64,test') } }))

vi.mock('@/stores/panel', () => ({
  usePanelState: () => ({
    state: { agent: undefined },
    setAgent: vi.fn(),
  }),
}))

vi.mock('@/stores/session', () => ({
  useSession: () => ({ state: mocks.sessionState }),
}))

vi.mock('@/stores/theme', () => ({
  useTheme: () => ({
    preference: mocks.themePreference,
    resolved: mocks.themeResolved,
    colors: mocks.themeColors,
    isCustom: mocks.themeIsCustom,
    setTheme: mocks.themeSetTheme,
    setColors: mocks.themeSetColors,
    resetColors: mocks.themeResetColors,
  }),
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
    show: mocks.toastShow,
  }),
}))

interface SettingsBindings {
  settingsSearch: Ref<string>
  activeSettingsCategory: Ref<'all' | 'account' | 'appearance' | 'data' | 'system' | 'support'>
  visibleSettingsSectionCount: ComputedRef<number>
  isSettingsSectionVisible: (id: 'help' | 'account-overview' | 'username' | 'password' | 'security-entrance' | 'totp' | 'language' | 'appearance' | 'backup' | 'mcp' | 'version-updates' | 'agent' | 'license') => boolean
  settingsCategoryCount: (category: 'all' | 'account' | 'appearance' | 'data' | 'system' | 'support') => number
  selectSettingsCategory: (category: 'all' | 'account' | 'appearance' | 'data' | 'system' | 'support') => void
  clearSettingsSearch: () => void
  resetSettingsFilters: () => void
  usernameForm: { newUsername: string; currentPassword: string }
  usernameValid: ComputedRef<boolean>
  canChangeUsername: ComputedRef<boolean>
  changingUsername: Ref<boolean>
  usernameSubmitted: Ref<boolean>
  changeUsername: () => Promise<void>
  passwordForm: {
    currentPassword: string
    newPassword: string
    confirmPassword: string
  }
  passwordChecks: ComputedRef<Array<{ label: string; valid: boolean }>>
  canChangePassword: ComputedRef<boolean>
  changingPassword: Ref<boolean>
  passwordSubmitted: Ref<boolean>
  changePassword: () => Promise<void>
  colorDraft: ThemeColorIntent
  colorInputs: Record<ThemeColorKey, string>
  colorErrors: Record<ThemeColorKey, string>
  colorPreviewMode: Ref<ThemeMode>
  colorPreviewTokens: ComputedRef<Record<string, string>>
  activeThemePresetId: ComputedRef<string | null>
  hasColorErrors: ComputedRef<boolean>
  colorDraftDirty: ComputedRef<boolean>
  selectThemeColorPreset: (colors: Readonly<ThemeColorIntent>) => void
  updateThemeColor: (key: ThemeColorKey, event: Event) => void
  applyThemeColors: () => void
  cancelThemeColorChanges: () => void
  resetThemeColors: () => void
  securityEntry: Ref<{ enabled: boolean; path?: string; resourceVersion: string } | undefined>
  securityEntryPath: Ref<string>
  saveSecurityEntry: (enabled: boolean, regenerate?: boolean) => Promise<void>
  totpForm: { currentPassword: string; code: string; secondFactor: string }
  totpAction: Ref<'idle' | 'enroll' | 'verify' | 'recovery' | 'rotate' | 'disable'>
  recoveryCodes: Ref<string[]>
  startTOTPEnrollment: () => Promise<void>
  confirmTOTPEnrollment: () => Promise<void>
  finishTOTPFlow: () => Promise<void>
	automaticUpdate: Ref<{
		enabled: boolean
		state: string
		channel: 'stable' | 'preview'
		canInstall: boolean
		resourceVersion: string
		observationHours: number
		candidateVersion?: string
	} | undefined>
	saveAutomaticUpdate: (enabled: boolean) => Promise<void>
	checkAutomaticUpdate: () => Promise<unknown>
	togglePreviewProgram: (event: Event) => Promise<void>
	installAutomaticUpdate: () => Promise<void>
	manuallyUpdateKPanel: () => Promise<void>
	kpanelUpdateDialogOpen: Ref<boolean>
	kpanelRelease: Ref<KPanelReleaseInfo | undefined>
	settingsBrowser: Ref<HTMLElement | undefined>
	automaticUpdateSection: Ref<HTMLElement | undefined>
	focusAutomaticUpdateSection: () => Promise<void>
}

function setupView(): SettingsBindings {
  const component = SettingsView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => SettingsBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({}, { expose: () => undefined }))
  } finally {
    warn.mockRestore()
  }
}

function themeColorInput(value: string, type = 'text'): Event {
  return { currentTarget: { value, type } } as unknown as Event
}

describe('SettingsView navigation', () => {
  it('selects system settings before the first render for the version route intent', () => {
    mocks.route.query = { section: 'version-updates' }

    const view = setupView()

    expect(view.activeSettingsCategory.value).toBe('system')
    expect(view.visibleSettingsSectionCount.value).toBe(2)
    expect(view.isSettingsSectionVisible('version-updates')).toBe(true)
  })

  it('starts with every settings section visible and exposes useful category counts', () => {
    const view = setupView()

    expect(view.visibleSettingsSectionCount.value).toBe(14)
    expect(view.settingsCategoryCount('account')).toBe(6)
    expect(view.settingsCategoryCount('appearance')).toBe(2)
    expect(view.settingsCategoryCount('data')).toBe(2)
    expect(view.settingsCategoryCount('system')).toBe(2)
    expect(view.settingsCategoryCount('support')).toBe(2)
  })

  it('filters sections by category without destroying their state', () => {
    const view = setupView()

    view.selectSettingsCategory('appearance')

    expect(view.visibleSettingsSectionCount.value).toBe(2)
    expect(view.isSettingsSectionVisible('language')).toBe(true)
    expect(view.isSettingsSectionVisible('appearance')).toBe(true)
    expect(view.isSettingsSectionVisible('password')).toBe(false)
    expect(settingsSource).toContain('<BackupCenter v-show=')
    expect(settingsSource).toContain('<MCPAccess v-show=')
  })

  it('searches section labels and keywords, then restores the complete list', () => {
    const view = setupView()

    view.settingsSearch.value = '恢复码'

    expect(view.visibleSettingsSectionCount.value).toBe(1)
    expect(view.isSettingsSectionVisible('totp')).toBe(true)
    expect(view.isSettingsSectionVisible('backup')).toBe(false)

    view.resetSettingsFilters()

    expect(view.settingsSearch.value).toBe('')
    expect(view.activeSettingsCategory.value).toBe('all')
    expect(view.visibleSettingsSectionCount.value).toBe(14)
  })

  it('renders an accessible search, category tabs, and empty-result recovery action', () => {
    expect(settingsSource).toContain('type="search"')
    expect(settingsSource).toContain('role="tablist"')
    expect(settingsSource).toContain(':aria-selected="activeSettingsCategory === category.id"')
    expect(settingsSource).toContain('visibleSettingsSectionCount === 0')
    expect(settingsSource).toContain('没有找到匹配的设置')
  })
})

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('confirm', vi.fn(() => true))
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
    callback(0)
    return 1
  }))
  mocks.route.query = {}
  mocks.replace.mockResolvedValue(undefined)
  mocks.changePassword.mockResolvedValue(undefined)
  mocks.changeUsername.mockResolvedValue(undefined)
  mocks.getSecurityEntrance.mockResolvedValue({ enabled: false, resourceVersion: 'sha256:initial' })
  mocks.updateSecurityEntrance.mockResolvedValue({
    enabled: true,
    path: 'panel-generated',
    resourceVersion: 'sha256:updated',
  })
  mocks.getTOTPStatus.mockResolvedValue({ enabled: false, recoveryCodesRemaining: 0 })
	mocks.getAutomaticUpdate.mockResolvedValue({
		available: true, enabled: false, state: 'disabled', channel: 'stable',
		canInstall: false, installRequested: false,
		schedule: 'daily-04:00-local', observationHours: 24, resourceVersion: 'sha256:auto-initial',
	})
	mocks.updateAutomaticUpdate.mockResolvedValue({
		available: true, enabled: true, state: 'idle', channel: 'stable',
		canInstall: false, installRequested: false,
		schedule: 'daily-04:00-local', observationHours: 24, resourceVersion: 'sha256:auto-updated',
	})
	mocks.checkAutomaticUpdate.mockResolvedValue({
		available: true, enabled: true, state: 'waiting', channel: 'stable',
		canInstall: true, installRequested: false,
		schedule: 'daily-04:00-local', observationHours: 24, candidateVersion: '1.16.0',
		candidateImageDigest: `sha256:${'a'.repeat(64)}`,
		resourceVersion: 'sha256:auto-checked',
	})
	mocks.installAutomaticUpdate.mockResolvedValue({
		available: true, enabled: false, state: 'queued', channel: 'preview',
		canInstall: false, installRequested: true,
		schedule: 'daily-04:00-local', observationHours: 24, candidateVersion: '1.17.0-rc.1',
		candidateImageDigest: `sha256:${'b'.repeat(64)}`,
		resourceVersion: 'sha256:auto-queued',
	})
	mocks.getKPanelRelease.mockResolvedValue({
		channel: 'stable',
		version: '1.16.0',
		imageDigest: `sha256:${'a'.repeat(64)}`,
		releaseUrl: 'https://github.com/kejilion/KPanel/releases/tag/v1.16.0',
		publishedAt: '2026-09-14T00:00:00Z',
		notes: [{ kind: 'added', text: '新增更新内容展示' }],
		upgradeNotes: ['更新期间服务会短暂重启。'],
		cached: true,
		stale: false,
	} satisfies KPanelReleaseInfo)
  mocks.startTOTPEnrollment.mockResolvedValue({
    id: 'enrollment-1', secret: 'JBSWY3DPEHPK3PXP', otpauthUri: 'otpauth://totp/KPanel:admin', expiresAt: '2026-08-01T00:10:00Z',
  })
  mocks.confirmTOTPEnrollment.mockResolvedValue({ recoveryCodes: ['AAAAA-BBBBB-CCCCC', 'DDDDD-EEEEE-FFFFF'] })
  mocks.sessionState.authenticated = true
  mocks.sessionState.user = { id: 'admin', username: 'admin' }
  mocks.sessionState.expiresAt = '2026-07-26T00:00:00Z'
  mocks.sessionState.agent = { connected: true }
  mocks.themePreference.value = 'system'
  mocks.themeResolved.value = 'light'
  mocks.themeColors.value = {
    brand: '#0c7a60',
    neutral: '#52645f',
    signatureLinked: true,
    signature: '#0c7a60',
  }
  mocks.themeIsCustom.value = false
  mocks.themeSetColors.mockImplementation((colors: ThemeColorIntent) => {
    mocks.themeColors.value = { ...colors }
    mocks.themeIsCustom.value = true
  })
  mocks.themeResetColors.mockImplementation(() => {
    mocks.themeColors.value = {
      brand: '#0c7a60',
      neutral: '#52645f',
      signatureLinked: true,
      signature: '#0c7a60',
    }
    mocks.themeIsCustom.value = false
  })
})

describe('SettingsView password change', () => {
  it('requires the existing password and a matching 12-character password with letters and digits', async () => {
    const view = setupView()

    view.passwordForm.currentPassword = 'old-password'
    view.passwordForm.newPassword = 'Short123'
    view.passwordForm.confirmPassword = 'Short123'
    expect(view.canChangePassword.value).toBe(false)

    view.passwordForm.newPassword = 'abcdefghijkl'
    view.passwordForm.confirmPassword = 'abcdefghijkl'
    expect(view.passwordChecks.value[1]?.valid).toBe(false)
    expect(view.canChangePassword.value).toBe(false)

    view.passwordForm.newPassword = 'StrongPassword123'
    view.passwordForm.confirmPassword = 'StrongPassword124'
    expect(view.canChangePassword.value).toBe(false)

    await view.changePassword()
    expect(view.passwordSubmitted.value).toBe(true)
    expect(mocks.changePassword).not.toHaveBeenCalled()
  })

  it('clears the local session and redirects to login after a successful change', async () => {
    const view = setupView()
    view.passwordForm.currentPassword = 'CurrentPassword123'
    view.passwordForm.newPassword = 'ReplacementPassword456'
    view.passwordForm.confirmPassword = 'ReplacementPassword456'

    await view.changePassword()

    expect(mocks.changePassword).toHaveBeenCalledWith('CurrentPassword123', 'ReplacementPassword456')
    expect(mocks.resetApiSecurityState).toHaveBeenCalledOnce()
    expect(mocks.sessionState).toMatchObject({
      authenticated: false,
      user: undefined,
      expiresAt: undefined,
      agent: undefined,
    })
    expect(mocks.toastSuccess).toHaveBeenCalledWith('密码已修改', '请使用新密码重新登录。')
    expect(mocks.replace).toHaveBeenCalledWith({ name: 'login' })
    expect(view.passwordForm).toEqual({
      currentPassword: '',
      newPassword: '',
      confirmPassword: '',
    })
  })

  it('keeps the current session when the API rejects the current password', async () => {
    mocks.changePassword.mockRejectedValueOnce(new mocks.MockApiError('当前密码不正确'))
    const view = setupView()
    view.passwordForm.currentPassword = 'WrongPassword123'
    view.passwordForm.newPassword = 'ReplacementPassword456'
    view.passwordForm.confirmPassword = 'ReplacementPassword456'

    await view.changePassword()

    expect(mocks.toastDanger).toHaveBeenCalledWith('密码修改失败', '当前密码不正确')
    expect(mocks.resetApiSecurityState).not.toHaveBeenCalled()
    expect(mocks.replace).not.toHaveBeenCalled()
    expect(mocks.sessionState.authenticated).toBe(true)
    expect(view.changingPassword.value).toBe(false)
  })
})

describe('SettingsView username change', () => {
  it('keeps the current password empty until user interaction while allowing explicit browser fill', () => {
    expect(settingsSource).toContain('name="username-current-password"')
    expect(settingsSource).toMatch(/name="username-current-password"[\s\S]*?autocomplete="current-password"/)
    expect(settingsSource).toContain(':readonly="!usernamePasswordUnlocked"')
    expect(settingsSource).toContain('@focus="unlockUsernamePassword"')
  })

  it('keeps two-step verification immediately below the security entrance', () => {
    expect(settingsSource.indexOf('<h2>登录安全入口</h2>')).toBeLessThan(settingsSource.indexOf('<h2>两步验证</h2>'))
  })

  it('keeps password change immediately below username change', () => {
    const usernameIndex = settingsSource.indexOf('<h2>修改用户名</h2>')
    const passwordIndex = settingsSource.indexOf('<h2>修改密码</h2>')
    const securityEntryIndex = settingsSource.indexOf('<h2>登录安全入口</h2>')

    expect(usernameIndex).toBeGreaterThanOrEqual(0)
    expect(passwordIndex).toBeGreaterThan(usernameIndex)
    expect(passwordIndex).toBeLessThan(securityEntryIndex)
  })
})

describe('SettingsView appearance', () => {
  it('keeps backup and restore immediately after appearance and colors', () => {
    expect(settingsSource).toMatch(/<h2>外观与配色<\/h2>[\s\S]*?<\/section>\s*<BackupCenter v-show=/)
  })

  it('provides accessible color inputs, linked accents, a local preview, and explicit actions', () => {
    expect(settingsSource).toContain('role="radiogroup" aria-label="推荐配色方案"')
    expect(settingsSource).toContain('@click="selectThemeColorPreset(preset.colors)"')
    expect(settingsSource).toContain('v-if="field.key !== \'signature\' || !colorDraft.signatureLinked"')
    expect(settingsSource).toContain('class="theme-color-field__picker"')
    expect(settingsSource).toContain('type="color"')
    expect(settingsSource).toContain(':value="colorInputs[field.key]"')
    expect(settingsSource).toContain(':aria-invalid="Boolean(colorErrors[field.key])"')
    expect(settingsSource).toContain('v-model="colorDraft.signatureLinked"')
    expect(settingsSource).toContain('role="radiogroup" aria-label="配色预览模式"')
    expect(settingsSource).toContain('@click="colorPreviewMode = option.id"')
    expect(settingsSource).toContain(':style="colorPreviewStyle"')
    expect(settingsSource).toContain('@click="resetThemeColors"')
    expect(settingsSource).toContain('@click="cancelThemeColorChanges"')
    expect(settingsSource).toContain('@click="applyThemeColors"')
    expect(settingsSource).not.toContain('theme.setSkin')
    expect(settingsSource).not.toContain('KPanel VIP')
    expect(settingsSource).not.toContain('KPanel 经典')
  })

  it('uses a preset as an editable draft and persists only the final color values', () => {
    const view = setupView()
    const preset = THEME_COLOR_PRESETS[1]!

    view.selectThemeColorPreset(preset.colors)

    expect(view.activeThemePresetId.value).toBe(preset.id)
    expect(view.colorDraft).toEqual(preset.colors)
    expect(view.colorInputs).toEqual({
      brand: preset.colors.brand,
      neutral: preset.colors.neutral,
      signature: preset.colors.signature,
    })
    expect(mocks.themeSetColors).not.toHaveBeenCalled()

    view.updateThemeColor('neutral', themeColorInput('#345678'))
    expect(view.activeThemePresetId.value).toBeNull()
    view.applyThemeColors()

    expect(mocks.themeSetColors).toHaveBeenCalledWith({
      ...preset.colors,
      neutral: '#345678',
    })
    expect(Object.keys(mocks.themeSetColors.mock.calls[0]?.[0] ?? {})).toEqual([
      'brand', 'neutral', 'signatureLinked', 'signature',
    ])
  })

  it('keeps the preview mode and interface mode as separate accessible radio groups', () => {
    expect(settingsSource).toContain('role="radiogroup" aria-label="配色预览模式"')
    expect(settingsSource).toContain(':tabindex="colorPreviewMode === option.id ? 0 : -1"')
    expect(settingsSource).toContain(':aria-checked="colorPreviewMode === option.id"')
    expect(settingsSource).toContain('role="radiogroup" aria-label="明暗模式"')
    expect(settingsSource).toContain(':tabindex="theme.preference.value === option.id ? 0 : -1"')
    expect(settingsSource).toContain(':aria-checked="theme.preference.value === option.id"')
    expect(settingsSource).toContain('@click="theme.setTheme(option.id)"')
    expect(settingsSource).toContain('role="radiogroup" aria-label="经典模式壁纸"')
    expect(settingsSource).toContain(':aria-checked="classicWallpaper.level.value === option.id"')
    expect(settingsSource.match(/@keydown="moveRadioFocus"/g)).toHaveLength(5)
  })

  it('validates and normalizes Hex input before applying a complete color intent', () => {
    const view = setupView()

    view.updateThemeColor('brand', themeColorInput('#not-a-color'))
    expect(view.colorErrors.brand).toBe('请输入 3 或 6 位 Hex 颜色，例如 #315d7d')
    expect(view.hasColorErrors.value).toBe(true)
    view.applyThemeColors()
    expect(mocks.themeSetColors).not.toHaveBeenCalled()

    view.updateThemeColor('brand', themeColorInput('#357'))
    view.updateThemeColor('neutral', themeColorInput('#65717D'))
    view.colorDraft.signatureLinked = false
    view.updateThemeColor('signature', themeColorInput('#B28C54'))
    expect(view.colorErrors.brand).toBe('')
    expect(view.colorInputs).toMatchObject({ brand: '#357', neutral: '#65717D', signature: '#B28C54' })

    view.applyThemeColors()

    expect(mocks.themeSetColors).toHaveBeenCalledWith({
      brand: '#335577',
      neutral: '#65717d',
      signatureLinked: false,
      signature: '#b28c54',
    })
    expect(mocks.toastSuccess).toHaveBeenCalledWith('配色已应用', '浅色和深色层级已自动生成。')
    expect(view.colorDraft).toEqual(mocks.themeColors.value)
    expect(view.colorInputs).toEqual({ brand: '#335577', neutral: '#65717d', signature: '#b28c54' })
  })

  it('links the preview accent to the theme color and previews light and dark without applying', () => {
    const view = setupView()
    view.updateThemeColor('brand', themeColorInput('#315d7d'))
    view.updateThemeColor('signature', themeColorInput('#b28c54'))

    const linkedAccent = view.colorPreviewTokens.value['--theme-accent']
    const lightSurface = view.colorPreviewTokens.value['--surface']
    view.colorDraft.signatureLinked = false
    const independentAccent = view.colorPreviewTokens.value['--theme-accent']
    view.colorPreviewMode.value = 'dark'

    expect(independentAccent).not.toBe(linkedAccent)
    expect(view.colorPreviewTokens.value['--surface']).not.toBe(lightSurface)
    expect(mocks.themeSetColors).not.toHaveBeenCalled()
    expect(mocks.themeSetTheme).not.toHaveBeenCalled()
  })

  it('cancels drafts and restores the default color intent explicitly', () => {
    mocks.themeColors.value = {
      brand: '#315d7d',
      neutral: '#65717d',
      signatureLinked: false,
      signature: '#b28c54',
    }
    mocks.themeIsCustom.value = true
    const view = setupView()

    view.updateThemeColor('brand', themeColorInput('#a13f49'))
    view.updateThemeColor('neutral', themeColorInput('invalid'))
    view.cancelThemeColorChanges()
    expect(view.colorDraft).toEqual(mocks.themeColors.value)
    expect(view.colorInputs).toEqual({ brand: '#315d7d', neutral: '#65717d', signature: '#b28c54' })
    expect(view.colorErrors).toEqual({ brand: '', neutral: '', signature: '' })
    expect(view.colorDraftDirty.value).toBe(false)

    view.resetThemeColors()
    expect(mocks.themeResetColors).toHaveBeenCalledOnce()
    expect(view.colorDraft).toEqual({
      brand: '#0c7a60',
      neutral: '#52645f',
      signatureLinked: true,
      signature: '#0c7a60',
    })
    expect(mocks.toastSuccess).toHaveBeenCalledWith('已恢复默认配色')
  })

  it('treats applying the default intent as the real stylesheet default', () => {
    mocks.themeColors.value = {
      brand: '#315d7d',
      neutral: '#65717d',
      signatureLinked: false,
      signature: '#b28c54',
    }
    mocks.themeIsCustom.value = true
    const view = setupView()

    Object.assign(view.colorDraft, {
      brand: '#0c7a60',
      neutral: '#52645f',
      signatureLinked: true,
      signature: '#0c7a60',
    })
    view.applyThemeColors()

    expect(mocks.themeResetColors).toHaveBeenCalledOnce()
    expect(mocks.themeSetColors).not.toHaveBeenCalled()
    expect(view.colorDraft).toEqual(mocks.themeColors.value)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('已恢复默认配色')
  })
})

describe('SettingsView username submission', () => {
  it('validates the new username and requires the current password', async () => {
    const view = setupView()
    view.usernameForm.newUsername = 'bad name'
    view.usernameForm.currentPassword = 'CurrentPassword123'
    expect(view.usernameValid.value).toBe(false)
    expect(view.canChangeUsername.value).toBe(false)

    view.usernameForm.newUsername = 'admin'
    expect(view.canChangeUsername.value).toBe(false)

    view.usernameForm.newUsername = 'operator-01'
    view.usernameForm.currentPassword = ''
    await view.changeUsername()
    expect(view.usernameSubmitted.value).toBe(true)
    expect(mocks.changeUsername).not.toHaveBeenCalled()
  })

  it('clears the local session and redirects after a successful username change', async () => {
    const view = setupView()
    view.usernameForm.newUsername = 'operator-01'
    view.usernameForm.currentPassword = 'CurrentPassword123'

    await view.changeUsername()

    expect(mocks.changeUsername).toHaveBeenCalledWith('CurrentPassword123', 'operator-01')
    expect(mocks.resetApiSecurityState).toHaveBeenCalledOnce()
    expect(mocks.sessionState.authenticated).toBe(false)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('用户名已修改', '请使用新用户名重新登录。')
    expect(mocks.replace).toHaveBeenCalledWith({ name: 'login' })
  })
})

describe('SettingsView security entrance', () => {
  it('updates from the current resource version and reflects the generated path', async () => {
    const view = setupView()
    view.securityEntry.value = { enabled: false, resourceVersion: 'sha256:initial' }
    view.securityEntryPath.value = ''

    await view.saveSecurityEntry(true, true)

    expect(mocks.updateSecurityEntrance).toHaveBeenCalledWith({
      enabled: true,
      path: '',
      regenerate: true,
      expectedResourceVersion: 'sha256:initial',
    })
    expect(view.securityEntry.value).toEqual({
      enabled: true,
      path: 'panel-generated',
      resourceVersion: 'sha256:updated',
    })
    expect(view.securityEntryPath.value).toBe('panel-generated')
  })
})

describe('SettingsView two-factor authentication', () => {
  it('requires the current password, verifies TOTP, and shows recovery codes before logout', async () => {
    const view = setupView()
    view.totpAction.value = 'enroll'
    view.totpForm.currentPassword = 'CurrentPassword123'

    await view.startTOTPEnrollment()
    expect(mocks.startTOTPEnrollment).toHaveBeenCalledWith('CurrentPassword123')
    expect(view.totpAction.value).toBe('verify')

    view.totpForm.code = '123456'
    await view.confirmTOTPEnrollment()
    expect(mocks.confirmTOTPEnrollment).toHaveBeenCalledWith('enrollment-1', '123456')
    expect(view.totpAction.value).toBe('recovery')
    expect(view.recoveryCodes.value).toEqual(['AAAAA-BBBBB-CCCCC', 'DDDDD-EEEEE-FFFFF'])
    expect(mocks.replace).not.toHaveBeenCalled()

    await view.finishTOTPFlow()
    expect(mocks.resetApiSecurityState).toHaveBeenCalled()
    expect(mocks.replace).toHaveBeenCalledWith({ name: 'login' })
  })
})

describe('SettingsView automatic updates', () => {
  it('persists an explicit opt-in and keeps check-now read-only with respect to installation', async () => {
    const view = setupView()
    view.automaticUpdate.value = {
      enabled: false,
      state: 'disabled',
      channel: 'stable',
      canInstall: false,
      observationHours: 24,
      resourceVersion: 'sha256:auto-initial',
    }

    await view.saveAutomaticUpdate(true)

    expect(mocks.updateAutomaticUpdate).toHaveBeenCalledWith({
      enabled: true,
      channel: 'stable',
      expectedResourceVersion: 'sha256:auto-initial',
    })
    expect(view.automaticUpdate.value?.resourceVersion).toBe('sha256:auto-updated')
    expect(mocks.toastSuccess).toHaveBeenCalledWith('自动安装已启用')

    await view.checkAutomaticUpdate()

    expect(mocks.checkAutomaticUpdate).toHaveBeenCalledOnce()
    expect(view.automaticUpdate.value).toMatchObject({
      state: 'waiting',
      candidateVersion: '1.16.0',
    })
    expect(mocks.updateAutomaticUpdate).toHaveBeenCalledTimes(1)
  })

  it('switches to preview only after warning and immediately checks that channel', async () => {
    mocks.updateAutomaticUpdate.mockResolvedValueOnce({
      available: true, enabled: false, state: 'disabled', channel: 'preview',
      canInstall: false, installRequested: false, schedule: 'daily-04:00-local',
      observationHours: 24, resourceVersion: 'sha256:preview-policy',
    })
    mocks.checkAutomaticUpdate.mockResolvedValueOnce({
      available: true, enabled: false, state: 'waiting', channel: 'preview',
      canInstall: true, installRequested: false, schedule: 'daily-04:00-local',
      observationHours: 24, candidateVersion: '1.17.0-rc.1',
      candidateImageDigest: `sha256:${'b'.repeat(64)}`,
      resourceVersion: 'sha256:preview-check',
    })
    const view = setupView()
    view.automaticUpdate.value = {
      enabled: false, state: 'disabled', channel: 'stable', canInstall: false,
      observationHours: 24, resourceVersion: 'sha256:auto-initial',
    }
    const input = { checked: true }

    await view.togglePreviewProgram({ currentTarget: input } as unknown as Event)

    expect(globalThis.confirm).toHaveBeenCalledOnce()
    expect(mocks.updateAutomaticUpdate).toHaveBeenCalledWith({
      enabled: false,
      channel: 'preview',
      expectedResourceVersion: 'sha256:auto-initial',
    })
    expect(mocks.checkAutomaticUpdate).toHaveBeenCalledOnce()
    expect(view.automaticUpdate.value).toMatchObject({
      channel: 'preview', candidateVersion: '1.17.0-rc.1', canInstall: true,
    })
    expect(input.checked).toBe(true)
  })

  it('queues immediate installation without enabling future automatic installs', async () => {
    const view = setupView()
    view.automaticUpdate.value = {
      enabled: false,
      state: 'waiting',
      channel: 'preview',
      canInstall: true,
      observationHours: 24,
      candidateVersion: '1.17.0-rc.1',
      resourceVersion: 'sha256:preview-check',
    }

    await view.installAutomaticUpdate()

    expect(mocks.installAutomaticUpdate).toHaveBeenCalledWith({
      expectedResourceVersion: 'sha256:preview-check',
    })
    expect(view.automaticUpdate.value).toMatchObject({
      enabled: false, state: 'queued', installRequested: true,
    })
    expect(mocks.updateAutomaticUpdate).not.toHaveBeenCalled()
  })

  it('checks the selected channel and opens the dedicated confirmation before installation', async () => {
    const view = setupView()
    view.automaticUpdate.value = {
      enabled: false,
      state: 'disabled',
      channel: 'stable',
      canInstall: false,
      observationHours: 24,
      resourceVersion: 'sha256:auto-initial',
    }

    await view.manuallyUpdateKPanel()

    expect(mocks.checkAutomaticUpdate).toHaveBeenCalledOnce()
    expect(view.kpanelUpdateDialogOpen.value).toBe(true)
    expect(mocks.getKPanelRelease).toHaveBeenCalledWith('stable', expect.any(AbortSignal))
    expect(mocks.installAutomaticUpdate).not.toHaveBeenCalled()
    expect(globalThis.confirm).not.toHaveBeenCalled()

    await view.installAutomaticUpdate()

    expect(mocks.installAutomaticUpdate).toHaveBeenCalledWith({
      expectedResourceVersion: 'sha256:auto-checked',
    })
    expect(mocks.updateAutomaticUpdate).not.toHaveBeenCalled()
    expect(view.kpanelUpdateDialogOpen.value).toBe(false)
    expect(view.automaticUpdate.value).toMatchObject({
      state: 'queued',
      installRequested: true,
    })
  })

  it('keeps the manual action non-destructive when the selected channel is current', async () => {
    mocks.checkAutomaticUpdate.mockResolvedValueOnce({
      available: true, enabled: false, state: 'idle', channel: 'stable',
      canInstall: false, installRequested: false, schedule: 'daily-04:00-local',
      observationHours: 24, currentVersion: '1.18.0', resourceVersion: 'sha256:auto-current',
    })
    const view = setupView()
    view.automaticUpdate.value = {
      enabled: false,
      state: 'disabled',
      channel: 'stable',
      canInstall: false,
      observationHours: 24,
      resourceVersion: 'sha256:auto-initial',
    }

    await view.manuallyUpdateKPanel()

    expect(mocks.installAutomaticUpdate).not.toHaveBeenCalled()
    expect(mocks.toastShow).toHaveBeenCalledWith('当前没有可手动安装的更新', {
      message: '稳定版已是最新版本。',
    })
  })

  it('focuses the version update card for the shared settings route intent', async () => {
    mocks.route.query = { section: 'version-updates' }
    const view = setupView()
    view.settingsSearch.value = '密码'
    view.activeSettingsCategory.value = 'account'
    const scrollSettingsBrowserIntoView = vi.fn()
    const scrollAutomaticUpdateIntoView = vi.fn()
    const focus = vi.fn()
    view.settingsBrowser.value = { scrollIntoView: scrollSettingsBrowserIntoView } as unknown as HTMLElement
    view.automaticUpdateSection.value = { scrollIntoView: scrollAutomaticUpdateIntoView, focus } as unknown as HTMLElement

    await view.focusAutomaticUpdateSection()

    expect(requestAnimationFrame).toHaveBeenCalledOnce()
    expect(view.settingsSearch.value).toBe('')
    expect(view.activeSettingsCategory.value).toBe('system')
    expect(view.isSettingsSectionVisible('version-updates')).toBe(true)
    expect(scrollSettingsBrowserIntoView).toHaveBeenCalledWith({ block: 'start' })
    expect(scrollAutomaticUpdateIntoView).not.toHaveBeenCalled()
    expect(focus).toHaveBeenCalledWith({ preventScroll: true })
  })

  it('documents opt-in, observation, host timer, backup, and rollback in the visible settings surface', () => {
    expect(settingsSource).toContain('加入预览版计划')
    expect(settingsSource).toContain('不会因为勾选而自动安装')
    expect(settingsSource).toContain('自动安装更新')
    expect(settingsSource).toContain('systemd 宿主机每天本地时间 04:00 检查')
    expect(settingsSource).toContain('手动立即安装可跳过等待')
    expect(settingsSource).toContain('退出预览版计划不会自动降级')
    expect(settingsSource).toContain('候选镜像摘要')
    expect(settingsSource).toContain('冷备份 Panel 与 Agent 数据')
    expect(settingsSource).toContain('失败时自动恢复原版本和数据')
    expect(settingsSource).toContain('data-testid="preview-program-toggle"')
    expect(settingsSource).toContain('data-testid="automatic-install-toggle"')
    expect(settingsSource).toContain('id="version-updates"')
    expect(settingsSource).toContain('data-testid="manual-release-update"')
    expect(settingsSource).toContain('前往应用市场手动更新')
  })
})

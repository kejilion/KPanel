// @vitest-environment jsdom
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { KPanelReleaseInfo } from '@/types/api'
import KPanelUpdateDialog from './KPanelUpdateDialog.vue'

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastDanger: vi.fn(),
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    danger: mocks.toastDanger,
  }),
}))

const digest = `sha256:${'a'.repeat(64)}`
const release: KPanelReleaseInfo = {
  channel: 'stable',
  version: '1.18.0',
  imageDigest: digest,
  releaseUrl: 'https://github.com/kejilion/KPanel/releases/tag/v1.18.0',
  publishedAt: '2026-09-14T00:00:00Z',
  notes: [
    { kind: 'added', text: '新增版本更新内容展示。' },
    { kind: 'fixed', text: '修复手动更新入口不一致的问题。' },
  ],
  upgradeNotes: ['更新期间服务会短暂重启。'],
  cached: true,
  stale: false,
}

const wrappers: VueWrapper[] = []

function mountDialog(props: Record<string, unknown> = {}): VueWrapper {
  const wrapper = mount(KPanelUpdateDialog, {
    attachTo: document.body,
    props: {
      open: true,
      release,
      currentVersion: '1.17.0',
      targetVersion: '1.18.0',
      targetDigest: digest,
      strategy: 'automatic',
      ...props,
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('KPanelUpdateDialog', () => {
  it('shows the exact target, release summary, upgrade warning, and official release link', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const wrapper = mountDialog()
    const content = document.body.textContent || ''

    expect(content).toContain('v1.17.0')
    expect(content).toContain('v1.18.0')
    expect(content).toContain('新增版本更新内容展示。')
    expect(content).toContain('更新期间服务会短暂重启。')
    expect(content).toContain('宿主机将先冷备份 Panel 与 Agent 数据')
    expect(content).toContain('不依赖当前浏览器直连 GitHub')

    const link = document.body.querySelector<HTMLAnchorElement>('.kpanel-release-link a')
    expect(link?.href).toBe('https://github.com/kejilion/KPanel/releases/tag/v1.18.0')
    expect(link?.rel).toBe('noopener noreferrer')

    document.body.querySelector<HTMLButtonElement>('[aria-label="复制发布说明地址"]')?.click()
    await nextTick()
    expect(writeText).toHaveBeenCalledWith('https://github.com/kejilion/KPanel/releases/tag/v1.18.0')
    expect(mocks.toastSuccess).toHaveBeenCalledWith('发布说明地址已复制')

    document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')?.click()
    await nextTick()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
  })

  it('does not present mismatched notes and keeps the actual update available', async () => {
    const wrapper = mountDialog({
      targetVersion: '1.19.0',
      targetDigest: `sha256:${'b'.repeat(64)}`,
      error: '当前网络无法读取发布说明。',
    })
    const content = document.body.textContent || ''

    expect(content).toContain('发布说明与目标镜像暂未匹配')
    expect(content).not.toContain('新增版本更新内容展示。')
    expect(document.body.querySelector('[data-testid="kpanel-release-url"]')?.textContent).toBe(
      'https://github.com/kejilion/KPanel/releases/tag/v1.19.0',
    )

    const confirm = document.body.querySelector<HTMLButtonElement>('.modal-panel__footer .button--primary')
    expect(confirm?.disabled).toBe(false)
    confirm?.click()
    await nextTick()
    expect(wrapper.emitted('confirm')).toHaveLength(1)

    document.body.querySelector<HTMLButtonElement>('.kpanel-release-unavailable .button')?.click()
    await nextTick()
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})

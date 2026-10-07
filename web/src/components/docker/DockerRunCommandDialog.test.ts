// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DockerRunCommandDialog from './DockerRunCommandDialog.vue'
import type { DockerContainerRunCommand } from '@/types/api'

const mocks = vi.hoisted(() => ({
  runCommand: vi.fn(),
  copyText: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status: number
    readonly code: string

    constructor(message: string, status = 0, code = 'request_failed') {
      super(message)
      this.status = status
      this.code = code
    }
  },
  api: { docker: { runCommand: mocks.runCommand } },
}))
vi.mock('@/lib/clipboard', () => ({ copyText: mocks.copyText }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const containerId = 'c'.repeat(64)

function command(overrides: Partial<DockerContainerRunCommand> = {}): DockerContainerRunCommand {
  return {
    containerId,
    name: 'db',
    image: 'mysql:8',
    options: [
      { flag: '-d' },
      { flag: '--name', value: 'db' },
      { flag: '--restart', value: 'always' },
      { flag: '-p', value: '3306:3306' },
      { flag: '-e', value: 'TZ=Asia/Shanghai' },
      { flag: '-e', value: 'MYSQL_ROOT_PASSWORD=hunter2' },
    ],
    command: [],
    networks: [],
    unsupported: [],
    imageDefaults: true,
    collectedAt: '2026-10-07T08:00:00Z',
    ...overrides,
  }
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(DockerRunCommandDialog, {
    props: { open: true, containerId, containerName: 'db', ...props },
    global: { stubs: { teleport: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.runCommand.mockResolvedValue(command())
  mocks.copyText.mockResolvedValue(true)
})

describe('DockerRunCommandDialog', () => {
  it('shows the reconstructed command with secrets hidden until asked', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(mocks.runCommand).toHaveBeenCalledWith(containerId, expect.any(AbortSignal))
    const code = wrapper.get('.run-command__code')
    expect(code.text()).toContain('docker run -d')
    expect(code.text()).toContain('--restart always')
    expect(code.text()).toContain('-e TZ=Asia/Shanghai')
    expect(code.text()).toContain('MYSQL_ROOT_PASSWORD=••••••')
    expect(code.text()).not.toContain('hunter2')
    expect(wrapper.get('.run-command__toolbar').text()).toContain('已隐藏 1 个疑似敏感值')
    expect(wrapper.findAll('.run-command__line').map((line) => line.text().endsWith('\\'))).toEqual([
      true, true, true, true, true, true, false,
    ])

    await wrapper.findAll('button').find((button) => button.text().includes('显示敏感值'))!.trigger('click')
    // The teleport stub re-creates the slot DOM, so query again after the toggle.
    expect(wrapper.get('.run-command__code').text()).toContain('MYSQL_ROOT_PASSWORD=hunter2')
    expect(wrapper.get('.run-command__toolbar button').attributes('aria-pressed')).toBe('true')
  })

  it('copies the full command and says when it holds hidden values', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('复制命令'))!.trigger('click')
    await flushPromises()

    expect(mocks.copyText).toHaveBeenCalledWith([
      'docker run -d \\',
      '  --name db \\',
      '  --restart always \\',
      '  -p 3306:3306 \\',
      '  -e TZ=Asia/Shanghai \\',
      '  -e MYSQL_ROOT_PASSWORD=hunter2 \\',
      '  mysql:8',
    ].join('\n'))
    expect(mocks.success).toHaveBeenCalledWith('完整命令已复制', '包含 1 个已隐藏的敏感值，请妥善保管。')
  })

  it('points Compose containers at their project file', async () => {
    mocks.runCommand.mockResolvedValueOnce(command({ composeProject: 'blog', composeService: 'db' }))
    const wrapper = mountDialog({ composeAvailable: true })
    await flushPromises()

    expect(wrapper.text()).toContain('该容器属于 Compose 项目 blog，原始配置在项目的 Compose 文件中。')
    await wrapper.findAll('button').find((button) => button.text() === '打开 Compose 配置')!.trigger('click')
    expect(wrapper.emitted('openCompose')).toEqual([['blog']])

    mocks.runCommand.mockResolvedValueOnce(command({ composeProject: 'blog' }))
    const unavailable = mountDialog({ composeAvailable: false })
    await flushPromises()
    expect(unavailable.text()).toContain('该容器属于 Compose 项目 blog')
    expect(unavailable.findAll('button').some((button) => button.text() === '打开 Compose 配置')).toBe(false)
  })

  it('states what the command could not capture', async () => {
    mocks.runCommand.mockResolvedValueOnce(command({
      imageDefaults: false,
      unsupported: ['--mac-address', '--storage-opt'],
      networks: [{ name: 'frontend', aliases: ['www'] }],
    }))
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('无法读取镜像默认配置')
    expect(wrapper.text()).toContain('--mac-address、--storage-opt')
    expect(wrapper.get('.run-command__code').text()).toContain('docker network connect --alias www frontend db')
  })

  it('explains an Agent that predates the endpoint', async () => {
    const { ApiError } = await import('@/lib/api')
    mocks.runCommand.mockRejectedValueOnce(new ApiError('请求方法不允许', 405, 'method_not_allowed'))
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('当前 Agent 版本不支持查看创建命令，请先更新 KPanel。')
    expect((wrapper.findAll('button').find((button) => button.text().includes('复制命令'))!.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('stops loading when closed and reloads on reopen', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.setProps({ open: false })
    expect(wrapper.find('.run-command').exists()).toBe(false)
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(mocks.runCommand).toHaveBeenCalledTimes(2)
  })
})

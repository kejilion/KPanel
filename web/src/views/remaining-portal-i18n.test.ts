// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DockerView from './DockerView.vue'
import FilesView from './FilesView.vue'
import { resetLocaleForTest, setLocale } from '@/i18n'
import dockerEnglishCatalog from '@/i18n/pages/DockerView/en-US'
import filesEnglishCatalog from '@/i18n/pages/FilesView/en-US'
import sharedEnglishCatalog from '@/i18n/pages/shared/en-US'
import { registerPhraseCatalog, resetPhraseLocalizationForTest } from '@/i18n/phrase'

const mocks = vi.hoisted(() => ({
  dockerInventory: vi.fn(),
  dockerBackups: vi.fn(),
  dockerEnvironment: vi.fn(),
  dockerJobs: vi.fn(),
  filesList: vi.fn(),
  remoteDownloadJobs: vi.fn(),
  clusterHosts: vi.fn(),
  routerPush: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: mocks.routerPush }),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {
    readonly status = 0
    readonly code = 'request_failed'
  },
  api: {
    docker: {
      inventory: mocks.dockerInventory,
      backups: mocks.dockerBackups,
      environment: mocks.dockerEnvironment,
      jobs: mocks.dockerJobs,
      job: vi.fn(),
      action: vi.fn(),
      composeProject: vi.fn(),
      exec: vi.fn(),
      logs: vi.fn(),
      stats: vi.fn(),
      task: vi.fn(),
    },
    files: {
      list: mocks.filesList,
      remoteDownloadJobs: mocks.remoteDownloadJobs,
      entry: vi.fn(),
      text: vi.fn(),
      write: vi.fn(),
      action: vi.fn(),
      transferFromPanel: vi.fn(),
      trash: vi.fn(),
      upload: vi.fn(),
      contentUrl: vi.fn(() => ''),
      archiveUrl: vi.fn(() => ''),
      createDownloadTicket: vi.fn(),
      createArchiveDownloadTicket: vi.fn(),
      thumbnailUrl: vi.fn(() => ''),
      createRemoteDownloadJob: vi.fn(),
      remoteDownloadJob: vi.fn(),
      cancelRemoteDownloadJob: vi.fn(),
      deleteRemoteDownloadJob: vi.fn(),
    },
    cluster: { hosts: mocks.clusterHosts },
    system: { publicNetwork: vi.fn(), action: vi.fn() },
  },
  setFileHostId: vi.fn(),
}))

vi.mock('@/stores/panel', () => ({
  usePanelState: () => ({ isReadOnly: { value: false } }),
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: vi.fn(), danger: vi.fn(), show: vi.fn() }),
}))

function registerEnglishCatalog(catalog: readonly (readonly [string, string])[]): void {
  registerPhraseCatalog(sharedEnglishCatalog)
  registerPhraseCatalog(catalog)
}

function dockerInventory(): Record<string, unknown> {
  return {
    available: true,
    version: '28.0.0',
    observedAt: '2026-08-31T08:00:00Z',
    containers: [],
    images: [],
    networks: [],
    volumes: [],
  }
}

function fileDirectory(): Record<string, unknown> {
  return {
    path: '/home',
    entries: [{
      name: 'site.conf',
      path: '/home/site.conf',
      kind: 'file',
      mime: 'text/plain',
      sizeBytes: 4,
      mode: '-rw-r--r--',
      owner: 'root',
      group: 'root',
      modifiedAt: '2026-08-31T08:00:00Z',
      resourceVersion: 'sha256:file',
      editable: true,
      previewable: true,
    }],
    offset: 0,
    total: 1,
    totalKnown: true,
    truncated: false,
    scanTruncated: false,
    readAt: '2026-08-31T08:00:00Z',
  }
}

describe('remaining portal dialog localization', () => {
  let wrappers: VueWrapper[] = []

  beforeEach(async () => {
    vi.clearAllMocks()
    resetPhraseLocalizationForTest()
    resetLocaleForTest()
    await setLocale('en-US', false)
    mocks.dockerInventory.mockResolvedValue(dockerInventory())
    mocks.dockerBackups.mockResolvedValue({ items: [] })
    mocks.dockerEnvironment.mockResolvedValue({
      available: true,
      containers: 0,
      images: 0,
      mirrorPreset: 'official',
      registryMirrors: [],
      ipv6Enabled: false,
      daemonConfig: 'valid',
      observedAt: '2026-08-31T08:00:00Z',
    })
    mocks.dockerJobs.mockResolvedValue({ items: [] })
    mocks.filesList.mockResolvedValue(fileDirectory())
    mocks.remoteDownloadJobs.mockResolvedValue({ items: [] })
    mocks.clusterHosts.mockResolvedValue({ nodeId: 'local-node', items: [] })
  })

  afterEach(() => {
    for (const wrapper of wrappers.splice(0)) wrapper.unmount()
    document.body.innerHTML = ''
    resetPhraseLocalizationForTest()
    resetLocaleForTest()
  })

  it('localizes the Docker deployment dialog rendered through body teleport', async () => {
    registerEnglishCatalog(dockerEnglishCatalog)
    const wrapper = mount(DockerView, {
      attachTo: document.body,
      global: {
        stubs: {
          PageHeader: true,
          EmptyState: true,
          ErrorState: true,
          LoadingState: true,
          StatusBadge: true,
          DockerDeploymentEditor: true,
        },
      },
    })
    wrappers.push(wrapper)
    await flushPromises()

    const createButton = wrapper.findAll('button')
      .find((button) => button.text().includes('新建容器'))
    expect(createButton).toBeDefined()
    await createButton?.trigger('click')
    await flushPromises()

    const panel = document.body.querySelector<HTMLElement>('.modal-panel')
    expect(panel).not.toBeNull()
    const text = panel?.textContent || ''
    expect(text).toContain('Deploy Docker application')
    expect(text).toContain('Paste deployment content')
    expect(text).toContain('Waiting for content')
    expect(text).toContain('Deploy')
    expect(text).not.toContain('部署 Docker 应用')
    expect(text).not.toContain('粘贴部署内容')
    expect(text).not.toContain('等待粘贴')
  })

  it('localizes the Files operation dialog rendered through body teleport', async () => {
    registerEnglishCatalog(filesEnglishCatalog)
    const wrapper = mount(FilesView, {
      attachTo: document.body,
      global: {
        stubs: {
          PageHeader: true,
          FileShareDialog: true,
          FileShareManagerDialog: true,
          CodeEditor: true,
        },
      },
    })
    wrappers.push(wrapper)
    await flushPromises()

    const entry = (wrapper.vm as unknown as { entries: unknown[] }).entries[0]
    ;(wrapper.vm as unknown as { openDialog: (action: 'compress', entry?: unknown) => void }).openDialog('compress', entry)
    await flushPromises()

    const panel = document.body.querySelector<HTMLElement>('.modal-panel')
    expect(panel).not.toBeNull()
    const text = panel?.textContent || ''
    expect(text).toContain('Compress File')
    expect(text).toContain('Compression Format')
    expect(text).toContain('Single maximum 100 items')
    expect(text).toContain('Start compression')
    expect(text).not.toContain('压缩文件')
    expect(text).not.toContain('压缩格式')
    expect(text).not.toContain('开始压缩')
  })
})

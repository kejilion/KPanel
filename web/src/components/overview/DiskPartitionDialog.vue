<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  CheckCircle2,
  ChevronRight,
  HardDrive,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ShieldAlert,
  Unplug,
  Wrench,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { diskProblemMessage, localizeDiskRuntimeMessage } from '@/lib/diskMessages'
import { formatBytes } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type {
  DiskDevice,
  DiskFilesystem,
  DiskManagementAction,
  DiskManagementSnapshot,
} from '@/types/api'

const props = withDefaults(defineProps<{
  open: boolean
  readable: boolean
  writable: boolean
  unavailableReason?: string
}>(), { unavailableReason: '' })
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const { locale } = useI18n()

const snapshot = ref<DiskManagementSnapshot>()
const selectedId = ref('')
const mountPoint = ref('')
const persist = ref(true)
const selectedMountPath = ref('')
const removePersistence = ref(true)
const filesystem = ref<DiskFilesystem>('ext4')
const pendingAction = ref<DiskManagementAction>()
const loading = ref(false)
const refreshing = ref(false)
const submitting = ref(false)
const error = ref('')
let controller: AbortController | undefined
let timer: number | undefined

const jobActive = computed(() => ['queued', 'running'].includes(snapshot.value?.job?.status || ''))
const selected = computed(() => snapshot.value?.devices.find((device) => device.id === selectedId.value))
const totalCapacity = computed(() => snapshot.value?.devices
  .filter((device) => !device.parentId)
  .reduce((total, device) => total + device.sizeBytes, 0) || 0)
const mountedCount = computed(() => snapshot.value?.devices.filter((device) => device.mounts.length).length || 0)
const mountPointIssue = computed(() => validateMountPoint(mountPoint.value))
const confirmationText = computed(() => pendingAction.value && selected.value
  ? phrase(confirmation(pendingAction.value, selected.value))
  : '')
const pendingDestructive = computed(() => pendingAction.value === 'format' || pendingAction.value === 'repair')

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function localizedDiskMessage(value: string): string {
  return localizeDiskRuntimeMessage(value, locale.value, phrase)
}

function localizedDiskError(reason: unknown, fallback: string): string {
  if (!(reason instanceof ApiError)) return localizedDiskMessage(fallback)
  return localizedDiskMessage(diskProblemMessage(reason.code) || reason.message || fallback)
}

function actionLabel(action: DiskManagementAction): string {
  return phrase({ mount: '挂载', unmount: '卸载', format: '格式化', check: '只读检查', repair: '自动修复' }[action])
}

function mountCountLabel(count: number): string {
  return count === 1 ? phrase('1 个挂载点') : phrase(`${count} 个挂载点`)
}

function shortDeviceID(value: string): string {
  return value.length > 24 ? `${value.slice(0, 12)}…${value.slice(-6)}` : value
}

function validateMountPoint(value: string): string {
  if (!value) return phrase('请输入挂载目录。')
  if (new TextEncoder().encode(value).length > 4096) return phrase('挂载目录不能超过 4096 字节。')
  const canonical = value.startsWith('/') && value !== '/' && !value.endsWith('/') && !value.includes('//')
    && !value.split('/').some((part, index) => index > 0 && (part === '' || part === '.' || part === '..'))
    && !/[\u0000-\u001f\u007f]/.test(value)
  const overlapsProtected = ['/dev', '/proc', '/sys', '/run', '/boot', '/boot/efi', '/home', '/var/lib/kejilion-panel', '/home/docker']
    .some((protectedPath) => value === protectedPath || value.startsWith(`${protectedPath}/`) || protectedPath.startsWith(`${value}/`))
  return canonical && !overlapsProtected
    ? ''
    : phrase('挂载目录必须是规范且非系统保护范围的 Linux 绝对路径。')
}

const deviceRows = computed<Array<{ device: DiskDevice; depth: number }>>(() => {
  const devices = snapshot.value?.devices || []
  const known = new Set(devices.map((device) => device.id))
  const children = new Map<string, DiskDevice[]>()
  for (const device of devices) {
    const parent = device.parentId && known.has(device.parentId) ? device.parentId : ''
    children.set(parent, [...(children.get(parent) || []), device])
  }
  const rank = (device: DiskDevice) => device.type === 'disk' ? 0 : device.type === 'loop' ? 1 : 2
  const compare = (left: DiskDevice, right: DiskDevice) => rank(left) - rank(right) || left.path.localeCompare(right.path, undefined, { numeric: true })
  for (const group of children.values()) group.sort(compare)
  const rows: Array<{ device: DiskDevice; depth: number }> = []
  const seen = new Set<string>()
  const visit = (device: DiskDevice, depth: number) => {
    if (seen.has(device.id)) return
    seen.add(device.id)
    rows.push({ device, depth })
    for (const child of children.get(device.id) || []) visit(child, Math.min(depth + 1, 4))
  }
  for (const root of children.get('') || []) visit(root, 0)
  for (const device of devices) visit(device, 0)
  return rows
})

function clearTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

function resetForm(device?: DiskDevice): void {
  if (!device) return
  mountPoint.value = `/mnt/${device.name.replace(/[^A-Za-z0-9._-]/g, '-')}`
  persist.value = true
  selectedMountPath.value = device.mounts[0]?.path || ''
  removePersistence.value = Boolean(device.mounts[0]?.persistent)
  filesystem.value = device.filesystem?.type === 'xfs' || device.filesystem?.type === 'ntfs' || device.filesystem?.type === 'vfat'
    ? device.filesystem.type
    : 'ext4'
}

function choose(device: DiskDevice): void {
  selectedId.value = device.id
  resetForm(device)
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    const next = await api.system.disks(controller.signal)
    snapshot.value = next
    const current = next.devices.find((device) => device.id === selectedId.value)
    if (!current) {
      const preferred = next.devices.find((device) => Object.values(device.operations).some((operation) => operation.enabled)) || next.devices[0]
      selectedId.value = preferred?.id || ''
      resetForm(preferred)
    } else if (!submitting.value) {
      selectedMountPath.value = current.mounts.some((mount) => mount.path === selectedMountPath.value)
        ? selectedMountPath.value
        : current.mounts[0]?.path || ''
    }
    clearTimer()
    if (jobActive.value) timer = window.setTimeout(() => void load(true), 1600)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = localizedDiskError(reason, '无法读取磁盘与分区状态。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function availability(action: DiskManagementAction): { enabled: boolean; reason: string } {
  const operation = selected.value?.operations[action]
  if (!props.writable) return { enabled: false, reason: '当前 Agent 仅支持查看磁盘状态。' }
  if (jobActive.value) return { enabled: false, reason: '已有磁盘任务正在执行。' }
  return { enabled: Boolean(operation?.enabled), reason: operation?.reason || '当前设备状态不支持此操作。' }
}

function confirmation(action: DiskManagementAction, device: DiskDevice): string {
  const identity = `${device.path} (${formatBytes(device.sizeBytes)})`
  if (action === 'format') return `格式化 ${identity} 为 ${filesystem.value} 会永久清除其中的数据，且无法由 KPanel 回滚。确认继续吗？`
  if (action === 'repair') return `自动修复 ${identity} 会写入文件系统元数据。请先确认已有可用备份，是否继续？`
  if (action === 'unmount') return removePersistence.value
    ? `卸载 ${selectedMountPath.value} 并移除对应的开机挂载配置，会中断正在使用该目录的服务。确认继续吗？`
    : `卸载 ${selectedMountPath.value} 会中断正在使用该目录的服务。确认继续吗？`
  if (action === 'mount') return `将 ${device.path} 挂载到 ${mountPoint.value}${persist.value ? phrase('，并写入开机挂载配置') : ''}。确认继续吗？`
  return `将对 ${identity} 执行只读文件系统检查。确认继续吗？`
}

function requestAction(action: DiskManagementAction): void {
  if (!selected.value || submitting.value || !availability(action).enabled) return
  pendingAction.value = action
}

async function submit(): Promise<void> {
  const action = pendingAction.value
  const device = selected.value
  if (!action || !device || submitting.value || !snapshot.value?.resourceVersion || !availability(action).enabled) return
  submitting.value = true
  try {
    const job = await api.system.diskAction({
      action,
      deviceId: device.id,
      expectedResourceVersion: snapshot.value.resourceVersion,
      ...(action === 'mount' ? { mountPoint: mountPoint.value, persist: persist.value } : {}),
      ...(action === 'unmount' ? { mountPoint: selectedMountPath.value, removePersistence: removePersistence.value } : {}),
      ...(action === 'format' ? { filesystem: filesystem.value } : {}),
    })
    pendingAction.value = undefined
    snapshot.value.job = job
    toast.success(phrase('磁盘任务已提交'), localizedDiskMessage(job.message))
    await load(true)
  } catch (reason) {
    toast.danger(phrase('磁盘操作未能启动'), localizedDiskError(reason, 'Agent 未能启动磁盘任务。'))
    await load(true)
  } finally {
    submitting.value = false
  }
}

function operationReason(action: DiskManagementAction): string {
  return localizedDiskMessage(availability(action).reason)
}

function mountUsage(device: DiskDevice): string {
  const mount = device.mounts[0]
  if (!mount || mount.usagePercent === undefined) return ''
  return phrase(`${Math.round(mount.usagePercent)}% 已用`)
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) void load()
  else {
    pendingAction.value = undefined
    controller?.abort()
    clearTimer()
  }
}, { immediate: true })

onBeforeUnmount(() => { controller?.abort(); clearTimer() })
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('磁盘与分区')"
    :description="phrase('查看真实块设备拓扑，并用固定流程完成挂载、卸载、格式化与文件系统检查。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="disk-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ localizedDiskMessage(unavailableReason || '当前 Agent 的磁盘读取能力尚未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="6" />
      <ErrorState v-else-if="error && !snapshot" :message="localizedDiskMessage(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="disk-summary">
          <div>
            <span>{{ phrase('物理与虚拟设备') }}</span>
            <strong>{{ snapshot.devices.filter((device) => !device.parentId).length }}</strong>
          </div>
          <div>
            <span>{{ phrase('总容量') }}</span>
            <strong>{{ formatBytes(totalCapacity) }}</strong>
          </div>
          <div>
            <span>{{ phrase('已挂载文件系统') }}</span>
            <strong>{{ mountedCount }}</strong>
          </div>
          <button class="icon-button" type="button" :disabled="refreshing || submitting" :title="phrase('刷新磁盘状态')" @click="load(true)">
            <RefreshCw :size="17" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="snapshot.platform.kind === 'wsl1' || snapshot.platform.kind === 'wsl2'" class="inline-alert inline-alert--info" role="status">
          <HardDrive :size="17" />
          <span><strong>{{ snapshot.platform.label }}</strong>{{ phrase('：系统盘与 WSL Swap 会保持保护；通过 Windows 挂入的独立磁盘仍按真实占用状态判断。') }}</span>
        </div>
        <div v-else-if="!snapshot.platform.writable" class="inline-alert inline-alert--warning" role="status">
          <ShieldAlert :size="17" /> {{ localizedDiskMessage(snapshot.platform.reason || '当前运行环境仅支持查看磁盘状态。') }}
        </div>

        <section v-if="snapshot.job" class="disk-job" :class="`is-${snapshot.job.status}`" aria-live="polite">
          <LoaderCircle v-if="jobActive" :size="18" class="spin" />
          <CheckCircle2 v-else-if="snapshot.job.status === 'succeeded'" :size="18" />
          <ShieldAlert v-else :size="18" />
          <div>
            <strong>{{ jobActive ? phrase(`正在处理 ${snapshot.job.devicePath} · ${snapshot.job.progress}%`) : localizedDiskMessage(snapshot.job.message) }}</strong>
            <span v-if="jobActive">{{ localizedDiskMessage(snapshot.job.message) }}</span>
            <span v-else>{{ snapshot.job.devicePath }} · {{ actionLabel(snapshot.job.action) }}</span>
            <span v-if="snapshot.job.status === 'needs_attention' && snapshot.job.recoveryPath" class="disk-job__recovery">
              {{ phrase('恢复快照') }}
              <code :title="snapshot.job.recoveryPath">{{ snapshot.job.recoveryPath }}</code>
            </span>
          </div>
          <span class="disk-job__progress" role="progressbar" :aria-valuenow="snapshot.job.progress" aria-valuemin="0" aria-valuemax="100">
            <span :style="{ width: `${snapshot.job.progress}%` }"></span>
          </span>
        </section>

        <div v-if="error" class="inline-alert inline-alert--warning" role="status">{{ localizedDiskMessage(error) }}</div>
        <div v-if="!snapshot.devices.length" class="disk-empty">
          <HardDrive :size="28" />
          <strong>{{ phrase('未发现可展示的块设备') }}</strong>
          <span>{{ phrase('请确认 Agent worker 可以读取宿主机 /dev 与 sysfs。') }}</span>
        </div>

        <div v-else class="disk-workspace">
          <section class="disk-list" :aria-label="phrase('磁盘设备列表')">
            <button
              v-for="row in deviceRows"
              :key="row.device.id"
              type="button"
              class="disk-row"
              :class="{ 'is-selected': selectedId === row.device.id, 'has-protection': row.device.protected }"
              :style="{ '--disk-depth': row.depth }"
              @click="choose(row.device)"
            >
              <span class="disk-row__branch"><ChevronRight v-if="row.depth" :size="14" /></span>
              <span class="disk-row__icon"><HardDrive :size="18" /></span>
              <span class="disk-row__identity">
                <strong>{{ row.device.path }}</strong>
                <small>{{ [row.device.model, row.device.type, row.device.transport].filter(Boolean).join(' · ') }}</small>
              </span>
              <span class="disk-row__filesystem">
                <strong>{{ row.device.filesystem?.type || phrase('未格式化') }}</strong>
                <small>{{ row.device.mounts.map((mount) => mount.path).join(' · ') || phrase('未挂载') }}</small>
              </span>
              <span class="disk-row__size">
                <strong>{{ formatBytes(row.device.sizeBytes) }}</strong>
                <small>{{ mountUsage(row.device) }}</small>
              </span>
              <span v-if="row.device.protected" class="disk-row__badge"><LockKeyhole :size="13" /> {{ phrase('已保护') }}</span>
              <span v-else-if="row.device.readOnly" class="disk-row__badge">{{ phrase('只读') }}</span>
            </button>
          </section>

          <aside v-if="selected" class="disk-inspector" :aria-label="phrase('当前设备操作')">
            <header>
              <div>
                <span>{{ phrase('当前设备') }}</span>
                <strong>{{ selected.path }}</strong>
              </div>
              <span class="disk-inspector__capacity">{{ formatBytes(selected.sizeBytes) }}</span>
            </header>

            <dl class="disk-facts">
              <div><dt>{{ phrase('设备标识') }}</dt><dd :title="selected.id">{{ shortDeviceID(selected.id) }}</dd></div>
              <div><dt>{{ phrase('文件系统') }}</dt><dd>{{ selected.filesystem?.type || phrase('未格式化') }}<template v-if="selected.filesystem?.label"> · {{ selected.filesystem.label }}</template></dd></div>
              <div><dt>{{ phrase('挂载状态') }}</dt><dd>{{ selected.mounts.length ? mountCountLabel(selected.mounts.length) : phrase('未挂载') }}</dd></div>
              <div><dt>{{ phrase('介质属性') }}</dt><dd>{{ phrase(selected.readOnly ? '只读' : '可写') }} · {{ phrase(selected.removable ? '可移除' : '固定') }}</dd></div>
            </dl>

            <div v-if="selected.protectionReasons.length" class="disk-protection">
              <LockKeyhole :size="16" />
              <span><strong>{{ phrase('此设备已保护') }}</strong>{{ selected.protectionReasons.map(localizedDiskMessage).join(' · ') }}</span>
            </div>

            <section v-if="selected.mounts.length" class="disk-action-card">
              <header><Unplug :size="17" /><strong>{{ phrase('卸载') }}</strong></header>
              <label>
                <span>{{ phrase('挂载点') }}</span>
                <select v-model="selectedMountPath" :disabled="submitting || jobActive">
                  <option v-for="mount in selected.mounts" :key="mount.path" :value="mount.path">{{ mount.path }}{{ mount.persistent ? ` · ${phrase('开机挂载')}` : '' }}</option>
                </select>
              </label>
              <label class="disk-check"><input v-model="removePersistence" type="checkbox" :disabled="submitting || jobActive" /> {{ phrase('同时移除对应的开机挂载配置') }}</label>
              <button class="button" type="button" :disabled="!availability('unmount').enabled || submitting" :title="operationReason('unmount')" @click="requestAction('unmount')">{{ phrase('卸载此挂载点') }}</button>
            </section>

            <section v-else-if="selected.filesystem" class="disk-action-card">
              <header><HardDrive :size="17" /><strong>{{ phrase('挂载') }}</strong></header>
              <label><span>{{ phrase('挂载目录') }}</span><input v-model.trim="mountPoint" type="text" spellcheck="false" :disabled="submitting || jobActive" :aria-invalid="Boolean(mountPointIssue)" aria-describedby="disk-mount-point-help" /></label>
              <small v-if="mountPointIssue" id="disk-mount-point-help" class="disk-field-error">{{ mountPointIssue }}</small>
              <label class="disk-check"><input v-model="persist" type="checkbox" :disabled="submitting || jobActive" /> {{ phrase('开机自动挂载（defaults,nofail）') }}</label>
              <button class="button button--primary" type="button" :disabled="!availability('mount').enabled || Boolean(mountPointIssue) || submitting" :title="mountPointIssue || operationReason('mount')" @click="requestAction('mount')">{{ phrase('挂载文件系统') }}</button>
            </section>

            <section class="disk-action-card">
              <header><Wrench :size="17" /><strong>{{ phrase('文件系统维护') }}</strong></header>
              <p>{{ phrase('先运行只读检查；只有检查结果建议修复时，再启动自动修复。') }}</p>
              <div class="disk-action-row">
                <button class="button" type="button" :disabled="!availability('check').enabled || submitting" :title="operationReason('check')" @click="requestAction('check')">{{ phrase('只读检查') }}</button>
                <button class="button" type="button" :disabled="!availability('repair').enabled || submitting" :title="operationReason('repair')" @click="requestAction('repair')">{{ phrase('自动修复') }}</button>
              </div>
            </section>

            <section class="disk-action-card disk-action-card--danger">
              <header><ShieldAlert :size="17" /><strong>{{ phrase('格式化') }}</strong></header>
              <p>{{ phrase('不可逆地清除当前设备上的文件系统和数据。') }}</p>
              <div class="disk-action-row">
                <select v-model="filesystem" :disabled="submitting || jobActive">
                  <option value="ext4">ext4</option>
                  <option value="xfs">XFS</option>
                  <option value="ntfs">NTFS</option>
                  <option value="vfat">FAT32</option>
                </select>
                <button class="button button--danger" type="button" :disabled="!availability('format').enabled || submitting" :title="operationReason('format')" @click="requestAction('format')">{{ phrase('格式化设备') }}</button>
              </div>
            </section>

            <p class="disk-boundary">{{ phrase('分区表创建、删除与扩容，以及 LVM / RAID 编排尚未接入固定适配器；现有拓扑仍会完整显示。') }}</p>
          </aside>
        </div>
      </template>
    </div>
  </ModalDialog>

  <ModalDialog
    :open="Boolean(pendingAction)"
    :title="phrase('确认磁盘操作')"
    :description="phrase('请核对目标和影响。无需输入固定确认文字。')"
    size="small"
    @close="pendingAction = undefined"
  >
    <div class="disk-confirm">
      <span class="disk-confirm__icon" :class="{ 'is-danger': pendingDestructive }">
        <ShieldAlert v-if="pendingDestructive" :size="22" />
        <HardDrive v-else :size="22" />
      </span>
      <div>
        <strong>{{ selected?.path }}</strong>
        <span>{{ selected ? formatBytes(selected.sizeBytes) : '' }} · {{ selected?.filesystem?.type || phrase('未格式化') }}</span>
      </div>
      <p>{{ confirmationText }}</p>
      <footer>
        <button class="button" type="button" :disabled="submitting" @click="pendingAction = undefined">{{ phrase('取消') }}</button>
        <button class="button" :class="pendingDestructive ? 'button--danger' : 'button--primary'" type="button" :disabled="submitting" @click="submit()">
          <LoaderCircle v-if="submitting" :size="16" class="spin" />
          {{ phrase(submitting ? '正在提交…' : '确认执行') }}
        </button>
      </footer>
    </div>
  </ModalDialog>
</template>

<style scoped>
.disk-dialog { display: grid; gap: 14px; }
.disk-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; align-items: stretch; gap: 8px; }
.disk-summary > div { display: grid; gap: 3px; padding: 10px 12px; border: 1px solid var(--border); border-radius: 11px; background: var(--surface-subtle); }
.disk-summary span { color: var(--muted); font-size: 12px; }
.disk-summary strong { font-size: 15px; }
.disk-summary .icon-button { align-self: center; }
.inline-alert { display: flex; align-items: flex-start; gap: 8px; }
.disk-job { position: relative; display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 9px; overflow: hidden; padding: 11px 12px 14px; border: 1px solid color-mix(in srgb, var(--brand) 35%, var(--border)); border-radius: 11px; background: color-mix(in srgb, var(--brand) 6%, var(--surface)); }
.disk-job > div { display: grid; gap: 2px; }
.disk-job span { color: var(--muted); font-size: 12px; }
.disk-job__recovery { display: flex; min-width: 0; gap: 6px; }
.disk-job__recovery code { overflow: hidden; color: inherit; font: inherit; text-overflow: ellipsis; white-space: nowrap; }
.disk-job__progress { position: absolute; right: 0; bottom: 0; left: 0; height: 3px; background: color-mix(in srgb, var(--brand) 10%, transparent); }
.disk-job__progress > span { display: block; height: 100%; background: var(--brand); transition: width .2s ease; }
.disk-job.is-failed, .disk-job.is-needs_attention { border-color: color-mix(in srgb, var(--danger) 45%, var(--border)); background: color-mix(in srgb, var(--danger) 7%, var(--surface)); }
.disk-empty { display: grid; place-items: center; gap: 7px; min-height: 240px; padding: 32px; color: var(--muted); text-align: center; }
.disk-workspace { display: grid; grid-template-columns: minmax(0, 1.35fr) minmax(310px, .85fr); gap: 12px; align-items: start; }
.disk-list, .disk-inspector { border: 1px solid var(--border); border-radius: 13px; background: var(--surface); }
.disk-list { overflow: hidden; }
.disk-row { --disk-depth: 0; display: grid; grid-template-columns: 18px 30px minmax(130px, 1.2fr) minmax(110px, 1fr) 86px auto; align-items: center; width: 100%; min-height: 61px; gap: 8px; padding: 9px 10px 9px calc(10px + var(--disk-depth) * 16px); border: 0; border-bottom: 1px solid var(--border); color: inherit; background: transparent; text-align: left; cursor: pointer; }
.disk-row:last-child { border-bottom: 0; }
.disk-row:hover, .disk-row.is-selected { background: color-mix(in srgb, var(--brand) 7%, var(--surface)); }
.disk-row.is-selected { box-shadow: inset 3px 0 var(--brand); }
.disk-row__branch, .disk-row__icon { display: grid; place-items: center; color: var(--muted); }
.disk-row__icon { width: 30px; height: 30px; border-radius: 9px; background: var(--surface-subtle); }
.disk-row__identity, .disk-row__filesystem, .disk-row__size { display: grid; min-width: 0; gap: 3px; }
.disk-row__identity strong, .disk-row__filesystem strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.disk-row__identity small, .disk-row__filesystem small, .disk-row__size small { overflow: hidden; color: var(--muted); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.disk-row__size { text-align: right; }
.disk-row__badge { display: inline-flex; align-items: center; gap: 4px; justify-self: end; padding: 3px 7px; border-radius: 999px; color: var(--amber); background: color-mix(in srgb, var(--amber) 12%, transparent); font-size: 12px; white-space: nowrap; }
.disk-inspector { display: grid; gap: 12px; padding: 13px; }
.disk-inspector > header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.disk-inspector > header > div { display: grid; gap: 3px; min-width: 0; }
.disk-inspector > header span { color: var(--muted); font-size: 12px; }
.disk-inspector > header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.disk-inspector__capacity { flex: 0 0 auto; padding: 4px 8px; border-radius: 999px; background: var(--surface-subtle); color: inherit !important; font-weight: 600; }
.disk-facts { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; margin: 0; }
.disk-facts div { display: grid; gap: 2px; padding: 8px 9px; border-radius: 9px; background: var(--surface-subtle); }
.disk-facts dt { color: var(--muted); font-size: 12px; }
.disk-facts dd { overflow-wrap: anywhere; margin: 0; font-size: 13px; }
.disk-protection { display: flex; align-items: flex-start; gap: 8px; padding: 9px 10px; border-radius: 10px; color: var(--amber); background: color-mix(in srgb, var(--amber) 9%, transparent); }
.disk-protection span { display: grid; gap: 2px; font-size: 12px; }
.disk-action-card { display: grid; gap: 9px; padding: 11px; border: 1px solid var(--border); border-radius: 11px; }
.disk-action-card > header { display: flex; align-items: center; gap: 7px; }
.disk-action-card p { margin: 0; color: var(--muted); font-size: 13px; line-height: 1.45; }
.disk-action-card label:not(.disk-check) { display: grid; gap: 5px; color: var(--muted); font-size: 12px; }
.disk-field-error { color: var(--danger); font-size: 12px; line-height: 1.45; }
.disk-action-card input[type='text'], .disk-action-card select { width: 100%; min-height: 38px; border: 1px solid var(--border); border-radius: 8px; padding: 7px 9px; color: inherit; background: var(--surface); font: inherit; }
.disk-check { display: flex; align-items: center; gap: 7px; color: var(--muted); font-size: 13px; }
.disk-action-row { display: flex; gap: 8px; }
.disk-action-row > * { flex: 1; }
.disk-action-card .button { min-height: 38px; justify-content: center; }
.disk-action-card--danger { border-color: color-mix(in srgb, var(--danger) 35%, var(--border)); }
.disk-boundary { margin: 0; padding: 0 2px; color: var(--muted); font-size: 12px; line-height: 1.5; }
.disk-confirm { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 10px; }
.disk-confirm__icon { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 12px; color: var(--brand); background: color-mix(in srgb, var(--brand) 10%, transparent); }
.disk-confirm__icon.is-danger { color: var(--danger); background: color-mix(in srgb, var(--danger) 10%, transparent); }
.disk-confirm > div { display: grid; gap: 3px; min-width: 0; }
.disk-confirm > div strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.disk-confirm > div span { color: var(--muted); font-size: 12px; }
.disk-confirm p { grid-column: 1 / -1; margin: 4px 0; color: var(--muted); line-height: 1.55; }
.disk-confirm footer { grid-column: 1 / -1; display: flex; justify-content: flex-end; gap: 8px; }
.disk-confirm footer .button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; min-width: 94px; }
@media (max-width: 900px) { .disk-workspace { grid-template-columns: 1fr; } .disk-inspector { position: static; } }
@media (max-width: 640px) { .disk-summary { grid-template-columns: repeat(3, 1fr); } .disk-summary .icon-button { grid-column: 1 / -1; justify-self: end; } .disk-row { grid-template-columns: 18px 30px minmax(0, 1fr) auto; } .disk-row__filesystem { grid-column: 3 / -1; } .disk-row__size { grid-column: 4; grid-row: 1; } .disk-row__badge { grid-column: 3 / -1; justify-self: start; } .disk-facts { grid-template-columns: 1fr; } }
@media (prefers-reduced-motion: reduce) { .disk-job__progress > span { transition: none; } }
</style>

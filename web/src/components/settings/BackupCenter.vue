<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Archive, Download, Upload, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { backups, type BackupInventory, type BackupModule, type BackupRecord } from '@/lib/backup'
import { formatBytes, formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }

const choices: Array<{ id: BackupModule; name: string; detail: string }> = [
  { id: 'panel', name: '面板数据', detail: '账户、集群配对、桌面及通知配置；AI 仅包含 API 接入，不含会话。' },
  { id: 'apps', name: '应用数据', detail: '应用目录、配置和数据卷，沿用应用市场的资源布局。' },
  { id: 'web', name: '网站数据', detail: 'LDNMP 网站、数据库、当前证书和环境配置。' },
  { id: 'docker', name: 'Docker 数据', detail: '其他容器的配置、挂载数据及网络；镜像按原版本获取。' },
]
const records = ref<BackupRecord[]>([])
const inventory = ref<BackupInventory>()
const dialog = ref<'export' | 'import' | 'restore' | 'delete' | 'recover' | ''>('')
const selected = ref<BackupModule[]>(choices.map(c => c.id))
const password = ref('')
const confirmation = ref('')
const file = ref<File>()
const source = ref<BackupRecord>()
const loading = ref(false)
const previewReady = ref(false)
let refreshing = false
const busy = ref(false)
const error = ref('')
const listError = ref('')
const notice = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const pending = computed(() => records.value.some(r => ['queued', 'running', 'restarting'].includes(r.status)))
const title = computed(() => ({ export: '导出备份', import: '导入恢复', restore: '确认恢复', delete: '删除备份记录', recover: '处理恢复中断', '': '' }[dialog.value]))
const available = computed(() => dialog.value === 'restore' ? choices.filter(c => source.value?.modules.includes(c.id)) : choices)
const estimate = computed(() => selected.value.reduce((total, module) => total + (module === 'panel' ? inventory.value?.panelBytes || 0 : inventory.value?.host?.modules.find(m => m.id === module)?.bytes || 0), 0))
const validPassword = computed(() => new TextEncoder().encode(password.value).length >= 10 && new TextEncoder().encode(password.value).length <= 256)
const missingDependencies = computed(() => dialog.value !== 'export' ? [] : inventory.value?.host?.modules.filter(m => selected.value.includes(m.id)).flatMap(m => m.requires).filter(m => !selected.value.includes(m)) || [])
const canSubmit = computed(() => !busy.value && !loading.value && (dialog.value === 'delete' || dialog.value === 'recover' || (dialog.value === 'import' ? !!file.value && validPassword.value : selected.value.length > 0 && missingDependencies.value.length === 0 && ((dialog.value === 'restore' && previewReady.value) || (dialog.value === 'export' && !!inventory.value && validPassword.value && password.value === confirmation.value)))))
const name = (id: BackupModule) => phrase(choices.find(c => c.id === id)?.name || id)
function unavailable(id: BackupModule) { return dialog.value === 'export' && id !== 'panel' && (!inventory.value?.hostAvailable || !!inventory.value.host?.modules.find(m => m.id === id)?.issue) }
function unavailableReason(id: BackupModule) {
  const issue = inventory.value?.host?.modules.find(m => m.id === id)?.issue
  return ({ protected_panel_data: '所选目录与面板自身数据重叠，请将业务数据与面板目录分开。', user_namespace_requires_adapter: '用户命名空间运行模式暂不支持此类备份。', auto_remove_requires_stop: '容器启用了停止后自动删除，请先调整容器配置。', container_not_stable: '有容器正在暂停或重启，请等待其恢复稳定。', volume_driver_requires_external_backup: '数据卷需要存储驱动提供的专用备份工具。', data_path_cannot_be_archived: '数据目录包含不可归档内容（如链接或独立挂载点），请先处理。' } as Record<string, string>)[issue || ''] || 'Agent 暂不可用，请检查连接。'
}
function status(record: BackupRecord) {
	if (record.errorCode === 'host_busy') return '请关闭宿主机终端，并等待已有主机任务完成后重试。'
  if (record.status === 'running') return ({ backing_up_services: '正在备份服务数据', encrypting: '正在加密备份文件', checking_services: '正在检查服务数据', restoring_services: '正在恢复服务数据' } as Record<string, string>)[record.stage] || '正在处理'
  if (record.errorCode === 'partially_restored') return '部分数据已恢复，请查看已完成类别'
  if (record.status === 'expired') return '备份文件已过期，请重新导出或上传'
  if (record.errorCode === 'cleanup_pending') return '数据已恢复，旧数据清理尚未完成'
  if (record.errorCode === 'recovery_required') return '恢复中断，需要继续回滚或清理'
  if (record.errorCode === 'rolled_back') return '恢复失败，已回滚所选数据'
  return ({ queued: '等待执行', running: '正在处理', completed: '已完成', ready: '检查通过，待确认恢复', failed: '未完成，请检查后重试', restarting: '正在重载面板，请稍后重新登录' } as Record<string, string>)[record.status] || record.status
}
function action(record: BackupRecord) { return ({ export: '备份导出', import: '备份检查', restore: '数据恢复', recover: '恢复中断处理' })[record.action] }
async function refresh() {
  if (refreshing || disposed) return
  refreshing = true; clearTimeout(timer)
  try { records.value = (await backups.list()).items; listError.value = '' } catch { listError.value = '暂时无法读取备份任务，请刷新重试。' }
  refreshing = false
  if (!disposed) timer = setTimeout(refresh, pending.value ? 2000 : 15000)
}
function refreshNow() { clearTimeout(timer); void refresh() }
function close() { if (busy.value) return; dialog.value = ''; password.value = ''; confirmation.value = ''; file.value = undefined; error.value = '' }
async function openExport() {
  dialog.value = 'export'; error.value = ''; loading.value = true; inventory.value = undefined
  try { inventory.value = await backups.inventory(); selected.value = choices.filter(c => !unavailable(c.id)).map(c => c.id) }
  catch { error.value = '无法读取备份内容，请稍后重试。' }
  finally { loading.value = false }
}
function openImport() { dialog.value = 'import'; error.value = ''; file.value = undefined }
function chooseFile(event: Event) { file.value = (event.target as HTMLInputElement).files?.[0] }
async function openRestore(record: BackupRecord) {
  source.value = record; selected.value = [...record.modules]; dialog.value = 'restore'; error.value = ''; loading.value = true; previewReady.value = false
  try { source.value = await backups.preview(record.id); previewReady.value = true } catch (reason) { error.value = reason instanceof Error ? reason.message : '操作未完成，请刷新后重试。' } finally { loading.value = false }
}
function openDelete(record: BackupRecord) { source.value = record; dialog.value = 'delete'; error.value = '' }
async function submit() {
  if (!canSubmit.value) return
  busy.value = true; error.value = ''
  try {
    if (dialog.value === 'export') await backups.export(selected.value, password.value, inventory.value?.host?.revision)
    if (dialog.value === 'import' && file.value) await backups.import(file.value, password.value)
    if (dialog.value === 'restore' && source.value) await backups.restore(source.value, selected.value)
    if (dialog.value === 'delete' && source.value) await backups.delete(source.value.id)
    if (dialog.value === 'recover' && source.value) await backups.recover(source.value.id)
    notice.value = dialog.value === 'delete' ? '备份记录已删除。' : '任务已受理，可在下方查看进度。'
    busy.value = false; close(); refreshNow()
  } catch (reason) { error.value = reason instanceof Error ? reason.message : '操作未完成，请刷新后重试。' }
  finally { busy.value = false }
}
onMounted(refresh)
onBeforeUnmount(() => { disposed = true; clearTimeout(timer) })
</script>

<template>
  <section class="backup-center panel-card" aria-labelledby="backup-center-title">
    <header class="backup-heading"><Archive :size="22" aria-hidden="true" /><div><h2 id="backup-center-title">备份与恢复</h2><p>将需要的数据带走，在新面板中恢复。</p></div></header>
    <div class="backup-actions">
      <button class="button button--primary" :disabled="pending" @click="openExport"><Download :size="16" />导出备份</button>
      <button class="button" :disabled="pending" @click="openImport"><Upload :size="16" />导入恢复</button>
      <button class="button backup-refresh" @click="refreshNow"><RefreshCw :size="16" />刷新记录</button>
    </div>
    <p class="backup-note">支持全部或按类别选择。备份包使用密码加密，请下载到其他设备妥善保存。</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <p v-if="listError" class="backup-error" role="alert">{{ listError }}</p>
    <p v-if="!records.length && !listError" class="backup-empty">还没有备份记录</p>
    <ul v-else class="backup-records" aria-label="备份任务记录">
      <li v-for="record in records" :key="record.id">
        <div class="backup-record-main"><strong>{{ action(record) }}</strong><span>{{ record.modules?.map(name).join(' · ') }}</span><small>{{ formatDateTime(record.createdAt) }}<template v-if="record.size"> · {{ formatBytes(record.size) }}</template></small></div>
        <div class="backup-record-status"><span>{{ status(record) }}</span><small v-if="record.completedModules?.length">{{ phrase('已恢复：') }} {{ record.completedModules.map(name).join(' · ') }}</small><div class="backup-actions">
          <a v-if="record.action === 'export' && record.status === 'completed'" class="button" :href="backups.download(record.id)" download>下载备份</a>
          <button v-if="record.status === 'ready'" class="button button--primary" :disabled="pending" @click="openRestore(record)">选择恢复内容</button>
          <button v-if="record.status === 'failed' && ['cleanup_pending', 'recovery_required'].includes(record.errorCode || '')" class="button" :disabled="pending" @click="source = record; dialog = 'recover'">处理恢复中断</button>
          <button v-if="!['running', 'queued', 'restarting'].includes(record.status)" class="button" :disabled="pending" @click="openDelete(record)">删除记录</button>
        </div></div>
      </li>
    </ul>
    <ModalDialog :open="!!dialog" :title="phrase(title)" size="medium" :close-disabled="busy" @close="close">
      <form class="backup-form" @submit.prevent="submit">
        <p v-if="loading" role="status">{{ phrase('正在计算备份内容…') }}</p>
        <template v-if="dialog === 'export' || dialog === 'restore'">
          <fieldset :disabled="busy || loading"><legend>{{ phrase('选择内容') }}</legend>
            <label v-for="choice in available" :key="choice.id" class="backup-choice"><input v-model="selected" type="checkbox" :value="choice.id" :disabled="unavailable(choice.id)" /><span><strong>{{ phrase(choice.name) }}</strong><small>{{ phrase(choice.detail) }}</small><small v-if="unavailable(choice.id)">{{ phrase(unavailableReason(choice.id)) }}</small></span></label>
          </fieldset>
          <p v-if="dialog === 'export' && inventory" class="backup-note">{{ phrase('源数据约') }} {{ formatBytes(estimate) }}{{ phrase('，实际文件大小取决于压缩效果。') }}</p>
          <p v-if="missingDependencies.length" class="backup-error">{{ phrase('存在共享数据，请同时选择：') }} {{ [...new Set(missingDependencies)].map(name).join('、') }}</p>
          <p class="backup-note">{{ phrase('应用、网站和 Docker 备份会短暂停止相关容器，完成后恢复原运行状态。') }}</p>
        </template>
        <template v-if="dialog === 'import'">
          <label>{{ phrase('选择备份文件') }}<input type="file" accept=".kpb" required :disabled="busy" @change="chooseFile" /></label>
          <p class="backup-note">{{ phrase('上传后先检查内容；确认恢复前不会覆盖当前数据。支持 KPanel 或脚本通用备份导出的 .kpb 文件。') }}</p>
        </template>
        <template v-if="dialog === 'export' || dialog === 'import'">
          <label>{{ phrase('备份密码') }}<input v-model="password" type="password" autocomplete="new-password" required :disabled="busy" maxlength="256" /></label>
          <label v-if="dialog === 'export'">{{ phrase('再次输入密码') }}<input v-model="confirmation" type="password" autocomplete="new-password" required :disabled="busy" maxlength="256" /></label>
          <p v-if="dialog === 'export' && confirmation && password !== confirmation" class="backup-error">{{ phrase('两次密码不一致。') }}</p>
          <p class="backup-note">{{ phrase('密码至少 10 字节，建议使用 10 个以上英文字符；忘记密码将无法恢复备份。') }}</p>
        </template>
        <template v-if="dialog === 'restore'">
          <p class="backup-impact">{{ phrase('恢复会覆盖所选数据。恢复面板数据后，需要使用备份中的账户重新登录。') }}</p>
          <p class="backup-note">{{ phrase('完整恢复面板身份和配对密钥，且域名、协议、端口不变时，可保留集群配对。迁移切换时请停止旧面板。') }}</p>
          <p class="backup-note">{{ phrase('AI 只替换 API 接入配置，不导入会话；通知恢复后默认关闭。') }}</p>
        </template>
        <p v-if="dialog === 'recover'">{{ phrase('继续完成已提交的清理，或回滚未完成的恢复；不会重新执行导入。') }}</p>
        <p v-if="dialog === 'delete'">{{ phrase('删除服务器上的备份文件与记录，已经下载的副本不受影响。') }}</p>
        <p v-if="error" class="backup-error" role="alert">{{ phrase(error) }}</p>
        <p v-if="busy" role="status">{{ phrase('正在提交，请保持页面打开…') }}</p>
        <footer class="backup-actions"><button class="button" type="button" :disabled="busy" @click="close">{{ phrase('取消') }}</button><button class="button button--primary" type="submit" :disabled="!canSubmit">{{ phrase(dialog === 'recover' ? '确认处理' : dialog === 'restore' ? '确认恢复' : dialog === 'import' ? '上传并检查' : dialog === 'delete' ? '确认删除' : '开始备份') }}</button></footer>
      </form>
    </ModalDialog>
  </section>
</template>

<style scoped>
.backup-center{padding:24px;font-size:14px;line-height:1.6}.backup-heading{display:flex;align-items:flex-start;gap:12px;margin-bottom:20px}.backup-heading h2{margin:0;font-size:18px}.backup-heading p{margin:4px 0 0;color:var(--text-soft)}.backup-actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px}.backup-actions .button{font-size:14px;min-height:40px;display:inline-flex;align-items:center;justify-content:center;gap:8px}.backup-refresh{margin-left:auto}.backup-note,.backup-empty{font-size:13px;color:var(--text-soft)}.backup-empty{padding:20px 0;margin:0}.backup-records{list-style:none;margin:16px 0 0;padding:0}.backup-records li{display:flex;justify-content:space-between;align-items:center;gap:20px;border-top:1px solid var(--border);padding:16px 0}.backup-record-main,.backup-record-status{display:flex;flex-direction:column;gap:4px;overflow-wrap:anywhere}.backup-record-main small{font-size:13px;color:var(--text-soft)}.backup-record-status{align-items:flex-end}.backup-form{display:grid;gap:16px;font-size:14px;line-height:1.6}.backup-form p{margin:0}.backup-form>label{display:grid;gap:6px}.backup-form input:not([type=checkbox]){width:100%;min-height:40px;font-size:14px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--text);padding:8px 12px}.backup-form fieldset{border:0;margin:0;padding:0;min-width:0}.backup-form legend{font-size:14px;font-weight:600;margin-bottom:8px}.backup-choice{display:flex;align-items:flex-start;gap:12px;padding:12px 0;cursor:pointer}.backup-choice input{margin-top:5px;width:18px;height:18px;accent-color:var(--brand)}.backup-choice span{display:grid;gap:3px}.backup-choice small{font-size:13px;color:var(--text-soft)}.backup-error{color:var(--danger);font-size:14px}.backup-impact{padding:12px;background:var(--surface-subtle);border-left:3px solid var(--brand);border-radius:var(--radius-sm)}.backup-form footer{justify-content:flex-end}.backup-form :focus-visible{outline:2px solid var(--brand);outline-offset:3px}@media(max-width:640px){.backup-center{padding:16px}.backup-records li{align-items:flex-start;flex-direction:column;gap:12px}.backup-record-status{align-items:flex-start}.backup-refresh{margin-left:0}}
</style>

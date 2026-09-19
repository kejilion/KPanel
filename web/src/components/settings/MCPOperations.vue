<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { mcpAccess, type MCPOperation } from '@/lib/mcp'
import { formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

const props = defineProps<{ clients: Array<{ id: string; name: string }>; hosts: Array<{ id: string; name: string }> }>()
const items = ref<MCPOperation[]>([])
const loading = ref(false)
const busy = ref('')
const error = ref('')
const reviewing = ref('')
const acknowledged = ref(false)
let disposed = false
const labels: Record<MCPOperation['state'], string> = { pending: '等待审批', approved: '已批准', rejected: '已拒绝', executing: '执行中', submitted: '任务已受理', succeeded: '已完成', failed: '执行失败', unknown: '结果待核实', expired: '审批已过期' }
function label(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
async function refreshResult(item: MCPOperation) {
  busy.value = item.operationId; error.value = ''
  try {
    const updated = await mcpAccess.operation(item.operationId)
    if (!disposed) items.value = items.value.map(row => row.operationId === updated.operationId ? { ...row, ...updated } : row)
  } catch { if (!disposed) error.value = '无法核实任务结果，请检查连接及授权后重试。' }
  finally { if (!disposed) busy.value = '' }
}
async function refresh() {
  loading.value = true; error.value = ''
  try { const result = await mcpAccess.operations(); if (!disposed) items.value = [...result.items].reverse() }
  catch { if (!disposed) error.value = '无法加载 MCP 操作，请刷新重试。' }
  finally { if (!disposed) loading.value = false }
}
async function decide(item: MCPOperation, approve: boolean) {
  if (approve && (reviewing.value !== item.operationId || !acknowledged.value)) return
  busy.value = item.operationId; error.value = ''
  try {
    const updated = await mcpAccess.decide(item.operationId, item.digest, approve)
    if (disposed) return
    items.value = items.value.map(row => row.operationId === updated.operationId ? { ...row, ...updated } : row)
    reviewing.value = ''; acknowledged.value = false
  } catch { if (!disposed) error.value = '操作结果尚未确认，请刷新记录核对；不要重复创建相同操作。' }
  finally { if (!disposed) busy.value = '' }
}
function review(id: string) { reviewing.value = id; acknowledged.value = false }
onMounted(refresh)
onBeforeUnmount(() => { disposed = true })
</script>

<template>
  <div class="mcp-operations">
    <div class="operation-header"><h3>MCP 操作与审批</h3><button type="button" class="button button--secondary" :disabled="loading || !!busy" @click="refresh">刷新操作</button></div>
    <p class="operation-note">核对主机、操作和参数后再批准。任务已受理表示后台仍在执行；结果待核实时，先检查实际资源状态。</p>
    <p v-if="error" role="alert">{{ label(error) }}</p>
    <p v-if="loading" role="status">正在加载操作…</p>
    <p v-else-if="!items.length" class="operation-note">暂无 MCP 操作。</p>
    <ol v-else>
      <li v-for="item in items" :key="item.operationId">
        <div class="operation-header"><strong>{{ item.tool }}</strong><span>{{ label(labels[item.state]) }}</span></div>
        <p>{{ props.clients.find(client => client.id === item.clientId)?.name || label('集群控制端或已撤销的客户端') }} · {{ props.hosts.find(host => host.id === item.hostId)?.name || item.hostId }} · {{ formatDateTime(item.createdAt) }}</p>
        <button v-if="['submitted', 'unknown', 'executing'].includes(item.state) && props.clients.some(client => client.id === item.clientId)" type="button" class="button button--secondary" :disabled="!!busy" @click="refreshResult(item)">核实任务结果</button>
        <details :open="reviewing === item.operationId"><summary>查看操作详情</summary><pre>{{ JSON.stringify({ operationId: item.operationId, arguments: item.arguments, result: item.result, error: item.error }, null, 2) }}</pre></details>
        <div v-if="item.state === 'pending'" class="operation-actions">
          <template v-if="reviewing !== item.operationId"><button type="button" class="button button--secondary" :disabled="!!busy" @click="review(item.operationId)">审阅并批准</button><button type="button" class="button button--secondary" :disabled="!!busy" @click="decide(item, false)">拒绝</button></template>
          <template v-else>
            <label><input v-model="acknowledged" type="checkbox" :disabled="!!busy" /><span>我已核对目标主机和具体参数，允许执行此次操作。</span></label>
            <button type="button" class="button button--primary" :disabled="!!busy || !acknowledged" @click="decide(item, true)">批准并执行</button>
            <button type="button" class="button button--secondary" :disabled="!!busy" @click="reviewing = ''">返回</button>
          </template>
        </div>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.mcp-operations { border-top: 1px solid var(--border); padding-top: 20px; font-size: 14px; line-height: 1.6; min-width: 0; }
.operation-header { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; align-items: center; }
h3 { font-size: 16px; margin: 0; }
.operation-note, p { color: var(--text-secondary); overflow-wrap: anywhere; }
ol { padding: 0; list-style: none; max-height: 640px; overflow: auto; }
li { padding: 16px 0; border-bottom: 1px solid var(--border); }
strong { overflow-wrap: anywhere; }
summary { cursor: pointer; padding: 8px 0; }
pre { padding: 12px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 13px; background: var(--surface-subtle); border-radius: var(--radius-sm); max-height: 320px; overflow: auto; }
.operation-actions { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; margin-top: 12px; }
.operation-actions label { display: flex; gap: 8px; flex-basis: 100%; }
input { width: 18px; height: 18px; flex: none; }
:focus-visible { outline: 2px solid var(--brand); outline-offset: 3px; }
[role=alert] { color: var(--danger); }
</style>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  Ban,
  Check,
  ChevronDown,
  Gauge,
  Globe2,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  ShieldOff,
  TriangleAlert,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { FirewallCountryRule, FirewallRule, FirewallSnapshot, SystemResourceActionInput } from '@/types/api'

type ResourceActionWithoutVersion = SystemResourceActionInput extends infer Action
  ? Action extends { expectedResourceVersion: string }
    ? Omit<Action, 'expectedResourceVersion'>
    : never
  : never

type FirewallPortAction = 'firewall-open-port' | 'firewall-close-port'
type FirewallAddressAction = 'firewall-allow-ip' | 'firewall-block-ip' | 'firewall-remove-ip'
type FirewallCountryAction = 'firewall-allow-country' | 'firewall-block-country' | 'firewall-remove-country'
type FirewallAllPortsAction = 'firewall-open-all' | 'firewall-close-all'
type FirewallZone = 'inbound' | 'outbound' | 'forward'
type FirewallZoneFilter = FirewallZone | 'all'
type FirewallDecision = 'allow' | 'block' | 'other'
type FirewallDecisionFilter = Exclude<FirewallDecision, 'other'> | 'all'

interface ParsedFirewallRule {
  kind: 'firewall'
  original: FirewallRule
  zone: FirewallZone | 'other'
  decision: FirewallDecision
  protocol: string
  port: string
  source: string
  destination: string
}

interface ParsedFirewallCountryRule {
  kind: 'country'
  countryCode: string
  networkCount: number
  zone: FirewallZone
  decision: Exclude<FirewallDecision, 'other'>
  protocol: string
  port: string
  source: string
  destination: string
}

const props = withDefaults(
  defineProps<{
    open: boolean
    readable: boolean
    writable: boolean
    unavailableReason?: string
  }>(),
  { unavailableReason: '' },
)
const emit = defineEmits<{ close: [] }>()
const { locale } = useI18n()
const toast = useToast()
const snapshot = ref<FirewallSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const port = ref(443)
const address = ref('')
const countryCode = ref('')
const ruleMode = ref<'address' | 'country'>('address')
const zoneFilter = ref<FirewallZoneFilter>('inbound')
const decisionFilter = ref<FirewallDecisionFilter>('all')
const zoneFilterOptions: Array<{ id: FirewallZoneFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'inbound', label: '入站' },
  { id: 'outbound', label: '出站' },
  { id: 'forward', label: '转发' },
]
const decisionFilterOptions: Array<{ id: FirewallDecisionFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'allow', label: '允许' },
  { id: 'block', label: '阻止' },
]
let controller: AbortController | undefined

const rules = computed(() => snapshot.value?.rules || [])
const countryRules = computed<FirewallCountryRule[]>(() => snapshot.value?.countryRules || [])
const inputPolicy = computed(() => snapshot.value?.inputPolicy.trim().toUpperCase() || '')
const allPortsAction = computed<FirewallAllPortsAction | undefined>(() => {
  if (inputPolicy.value === 'ACCEPT') return 'firewall-close-all'
  if (inputPolicy.value === 'DROP') return 'firewall-open-all'
  return undefined
})
const validPort = computed(() => Number.isInteger(port.value) && port.value >= 1 && port.value <= 65535)
const addressReady = computed(() => Boolean(address.value.trim()))
const countryReady = computed(() => /^[A-Za-z]{2}$/.test(countryCode.value.trim()))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const accessSummary = computed(() => {
  if (inputPolicy.value === 'DROP') {
    return {
      tone: 'is-protected',
      title: phrase('默认禁止外部访问'),
      detail: phrase('只有明确允许的端口或地址可以连接。'),
    }
  }
  if (inputPolicy.value === 'ACCEPT') {
    return {
      tone: 'is-open',
      title: phrase('默认允许外部访问'),
      detail: phrase('系统默认允许连接；单独的阻止规则仍可能生效。'),
    }
  }
  return {
    tone: 'is-unknown',
    title: phrase('暂时无法确认'),
    detail: phrase('系统没有返回明确的入站策略，请刷新后再操作。'),
  }
})

function reasonMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

function zoneForChain(chain: string): FirewallZone | 'other' {
  switch (chain.trim().toUpperCase()) {
    case 'INPUT':
      return 'inbound'
    case 'OUTPUT':
      return 'outbound'
    case 'FORWARD':
      return 'forward'
    default:
      return 'other'
  }
}

function decisionForTarget(target: string): FirewallDecision {
  switch (target.trim().toUpperCase()) {
    case 'ACCEPT':
      return 'allow'
    case 'DROP':
    case 'REJECT':
      return 'block'
    default:
      return 'other'
  }
}

function protocolLabel(rule: FirewallRule): string {
  const protocol = rule.protocol.trim().toLowerCase()
  if (!protocol || protocol === 'all') return phrase('所有协议')
  if (protocol === 'icmp') return 'ICMP'
  return protocol.toUpperCase()
}

function portLabel(rule: FirewallRule): string {
  const protocol = rule.protocol.trim().toLowerCase()
  if (protocol === 'icmp') return phrase('Ping')

  const optionText = `${rule.options.join(' ')} ${rule.raw}`
  const match = optionText.match(/(?:--(?:dports?|sports?)|(?:dports?|dpts?|dpt|spts?|spt))\s*[:=]?\s*([0-9][0-9,:-]*(?:\s+[0-9][0-9,:-]*)*)/i)
  const ports = match?.[1]?.replace(/\s+/g, '').replace(/,/g, '、')
  return ports ? `${phrase('端口')} ${ports}` : phrase('所有端口')
}

function addressLabel(value: string): string {
  const addressValue = value.trim()
  if (!addressValue || addressValue === '0.0.0.0/0' || addressValue === '::/0') return phrase('所有 IP')
  return addressValue
}

function countryLabel(value: string): string {
  const code = value.trim().toUpperCase()
  try {
    const name = new Intl.DisplayNames([locale.value], { type: 'region' }).of(code)
    if (!name) return code
    return locale.value === 'en-US' ? `${name} (${code})` : `${name}（${code}）`
  } catch {
    return code
  }
}

function decisionLabel(decision: FirewallDecision): string {
  if (decision === 'allow') return phrase('允许')
  if (decision === 'block') return phrase('阻止')
  return phrase('系统内部')
}

function zoneLabel(zone: FirewallZone | 'other'): string {
  if (zone === 'inbound') return phrase('入站')
  if (zone === 'outbound') return phrase('出站')
  if (zone === 'forward') return phrase('转发')
  return phrase('其他')
}

const parsedRules = computed<ParsedFirewallRule[]>(() => rules.value.map((rule) => ({
  kind: 'firewall',
  original: rule,
  zone: zoneForChain(rule.chain),
  decision: decisionForTarget(rule.target),
  protocol: protocolLabel(rule),
  port: portLabel(rule),
  source: addressLabel(rule.source),
  destination: addressLabel(rule.destination),
})))

const parsedCountryRules = computed<ParsedFirewallCountryRule[]>(() => countryRules.value.map((rule) => ({
  kind: 'country',
  countryCode: rule.code,
  networkCount: rule.networkCount,
  zone: 'inbound',
  decision: rule.decision,
  protocol: phrase('国家/地区'),
  port: phrase('所有服务'),
  source: countryLabel(rule.code),
  destination: phrase('所有服务'),
})))

const filteredRules = computed(() => [...parsedRules.value, ...parsedCountryRules.value].filter((rule) => {
  const matchesZone = zoneFilter.value === 'all' || rule.zone === zoneFilter.value
  const matchesDecision = decisionFilter.value === 'all' || rule.decision === decisionFilter.value
  return matchesZone && matchesDecision
}))

function pingDetail(): string {
  return snapshot.value?.pingAllowed
    ? phrase('外部设备可以 Ping 到这台服务器。')
    : phrase('服务器不会回应外部 Ping。')
}

function ddosDetail(): string {
  return snapshot.value?.ddosEnabled
    ? phrase('短时间大量 TCP / UDP 新连接会被限制。')
    : phrase('当前没有启用异常连接限制。')
}

function allPortsLabel(): string {
  if (allPortsAction.value === 'firewall-close-all') return phrase('全部禁止外部访问')
  if (allPortsAction.value === 'firewall-open-all') return phrase('全部允许外部访问')
  return phrase('无法确认当前状态')
}

function allPortsDetail(): string {
  if (allPortsAction.value) return phrase('高风险操作：会清空当前防火墙规则和自定义链（包括 Docker 链），只恢复基础规则。')
  return phrase('状态未识别，刷新后才能使用此操作。')
}

function runAllPorts(): void {
  const action = allPortsAction.value
  if (!action) return
  void execute({ action }, allPortsConfirmation())
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.firewall(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reasonMessage(reason, '无法读取防火墙状态。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function execute(input: ResourceActionWithoutVersion, confirmation = ''): Promise<void> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion) return
  if (confirmation && typeof window !== 'undefined' && !window.confirm(phrase(confirmation))) return
  running.value = true
  try {
    const result = await api.system.resourceAction({
      ...input,
      expectedResourceVersion: snapshot.value.resourceVersion,
    } as SystemResourceActionInput)
    toast.success(result.changed ? '防火墙配置已更新' : '防火墙配置无需变更', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('防火墙操作失败', reasonMessage(reason, 'Agent 未能完成该防火墙操作。'))
    await load(true)
  } finally {
    running.value = false
  }
}

function portAction(action: FirewallPortAction): void {
  if (!validPort.value) return
  void execute(
    { action, port: Number(port.value) },
    `确认${action === 'firewall-open-port' ? '允许' : '阻止'}端口 ${port.value} 的 TCP 与 UDP 访问吗？`,
  )
}

function addressAction(action: FirewallAddressAction): void {
  const value = address.value.trim()
  if (!value) return
  const labels = {
    'firewall-allow-ip': '允许',
    'firewall-block-ip': '阻止',
    'firewall-remove-ip': '清除',
  } as const
  void execute({ action, address: value }, `确认${labels[action]} IP 规则 ${value} 吗？`)
}

function countryAction(action: FirewallCountryAction): void {
  const value = countryCode.value.trim().toUpperCase()
  if (!countryReady.value) return
  const labels = {
    'firewall-allow-country': '允许',
    'firewall-block-country': '阻止',
    'firewall-remove-country': '清除',
  } as const
  const confirmation = action === 'firewall-allow-country'
    ? `确认允许${countryLabel(value)}访问入站连接吗？这会将默认入站策略设为阻止，可能影响其他来源和远程管理连接。`
    : `确认${labels[action]}${countryLabel(value)}的国家/地区规则吗？`
  void execute({ action, countryCode: value }, confirmation)
}

function allPortsConfirmation(): string {
  if (allPortsAction.value === 'firewall-close-all') {
    return '确认将全部端口设为阻止访问吗？这会清空当前 iptables filter 规则和自定义链（包括 Docker 链），只恢复基础规则，可能影响网站、容器和其他服务。'
  }
  if (allPortsAction.value === 'firewall-open-all') {
    return '确认将全部端口设为允许访问吗？这会清空当前 iptables filter 规则和自定义链（包括 Docker 链），只恢复基础规则，可能影响网站、容器和其他服务。'
  }
  return ''
}

watch(
  () => [props.open, props.readable] as const,
  ([open, readable]) => {
    if (open && readable) void load()
    else controller?.abort()
  },
  { immediate: true },
)

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('防火墙')"
    :description="phrase('用最简单的方式管理服务器的连接，不需要理解底层规则。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog firewall-manager">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ unavailableReason || phrase('当前 Agent 的防火墙适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="error" @retry="load()" />
      <template v-else-if="snapshot">
        <section class="firewall-manager__status-wrap">
          <section class="firewall-manager__status" :class="accessSummary.tone" :aria-label="phrase('防火墙当前状态')">
            <div class="firewall-manager__status-main">
              <span class="firewall-manager__status-icon">
                <ShieldCheck v-if="accessSummary.tone === 'is-protected'" :size="24" />
                <ShieldOff v-else-if="accessSummary.tone === 'is-open'" :size="24" />
                <ShieldAlert v-else :size="24" />
              </span>
              <div>
                <span class="firewall-manager__eyebrow">{{ phrase('当前状态') }}</span>
                <strong>{{ accessSummary.title }}</strong>
                <p>{{ accessSummary.detail }}</p>
              </div>
            </div>
            <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新防火墙')" :aria-label="phrase('刷新防火墙')" @click="load(true)">
              <RefreshCw :size="17" :class="{ spin: refreshing }" />
            </button>
          </section>
          <div v-if="!writable" class="inline-alert inline-alert--warning">
            {{ phrase('当前 Agent 仅支持查看防火墙，写入适配器未就绪。') }}
          </div>
        </section>

        <nav class="firewall-manager__tabs" role="tablist" :aria-label="phrase('连接方向')">
          <button
            v-for="option in zoneFilterOptions"
            :key="option.id"
            class="firewall-manager__tab"
            :class="{ 'is-active': zoneFilter === option.id }"
            type="button"
            role="tab"
            :aria-selected="zoneFilter === option.id"
            :data-firewall-zone="option.id"
            @click="zoneFilter = option.id"
          >
            {{ phrase(option.label) }}
          </button>
        </nav>

        <section class="firewall-manager__section firewall-manager__rules-section">
          <header class="firewall-manager__section-heading">
            <div>
              <h3>{{ phrase('规则列表') }}</h3>
              <p>{{ phrase('只显示允许、阻止、端口和 IP，底层规则默认隐藏。') }}</p>
            </div>
            <span class="firewall-manager__rule-count">{{ filteredRules.length }} {{ phrase('条规则') }}</span>
          </header>
          <div class="firewall-manager__filter-bar">
            <span>{{ phrase('按动作') }}</span>
            <div class="firewall-manager__filter-options" role="group" :aria-label="phrase('按动作筛选')">
              <button v-for="option in decisionFilterOptions" :key="option.id" class="firewall-manager__filter-option" :class="{ 'is-active': decisionFilter === option.id }" type="button" :aria-pressed="decisionFilter === option.id" :data-firewall-decision="option.id" @click="decisionFilter = option.id">
                {{ phrase(option.label) }}
              </button>
            </div>
          </div>
          <div v-if="!filteredRules.length" class="firewall-manager__rule-empty">
            <ShieldCheck :size="22" />
            <strong>{{ phrase('当前筛选下没有规则') }}</strong>
            <p>{{ phrase('换一个方向或动作筛选，或者先添加一条入站规则。') }}</p>
          </div>
          <div v-else class="firewall-manager__parsed-rules" role="list" :aria-label="phrase('规则列表')">
            <div class="firewall-manager__rule-list-head" aria-hidden="true">
              <span>{{ phrase('动作') }}</span>
              <span>{{ phrase('连接') }}</span>
              <span>{{ phrase('来源') }}</span>
              <span>{{ phrase('目标') }}</span>
            </div>
            <article v-for="(rule, index) in filteredRules" :key="rule.kind === 'country' ? `country-${rule.countryCode}-${rule.decision}` : `${rule.original.line ?? index}-${rule.original.raw}`" class="firewall-manager__parsed-rule" role="listitem">
              <header class="firewall-manager__parsed-rule-head">
                <div class="firewall-manager__rule-badges">
                  <span class="firewall-manager__zone-badge">{{ zoneLabel(rule.zone) }}</span>
                  <span class="firewall-manager__decision" :class="`is-${rule.decision}`">
                    <Check v-if="rule.decision === 'allow'" :size="14" />
                    <Ban v-else-if="rule.decision === 'block'" :size="14" />
                    <ShieldAlert v-else :size="14" />
                    {{ decisionLabel(rule.decision) }}
                  </span>
                </div>
              </header>
              <div class="firewall-manager__rule-connection">
                <span class="firewall-manager__rule-list-label">{{ phrase('连接') }}</span>
                <strong class="firewall-manager__parsed-rule-title">{{ rule.kind === 'country' ? countryLabel(rule.countryCode) : `${rule.protocol} · ${rule.port}` }}</strong>
              </div>
              <div class="firewall-manager__rule-address">
                <span class="firewall-manager__rule-list-label">{{ phrase('来源') }}</span>
                <strong>{{ rule.kind === 'country' ? `${rule.networkCount} ${phrase('个 IPv4 网段')}` : rule.source }}</strong>
              </div>
              <div class="firewall-manager__rule-address">
                <span class="firewall-manager__rule-list-label">{{ phrase('目标') }}</span>
                <strong>{{ rule.kind === 'country' ? phrase('所有服务') : rule.destination }}</strong>
              </div>
              <p v-if="rule.kind === 'country'" class="firewall-manager__rule-note">{{ phrase('按该国家/地区的 IPv4 网段匹配，不是实时地理定位。') }}</p>
              <p v-else-if="rule.decision === 'other'" class="firewall-manager__rule-note">{{ phrase('这是系统内部规则，通常不需要手动修改。') }}</p>
            </article>
          </div>
        </section>

        <section class="firewall-manager__section firewall-manager__other">
          <header class="firewall-manager__section-heading">
            <div>
              <h3>{{ phrase('其他功能选项') }}</h3>
            </div>
          </header>

          <section class="firewall-manager__other-group firewall-manager__other-group--quick">
            <header class="firewall-manager__section-heading">
              <div>
                <h4>{{ phrase('快速添加规则') }}</h4>
                <p>{{ phrase('只处理入站连接；端口操作会同时处理 TCP 和 UDP。') }}</p>
              </div>
            </header>
            <div class="firewall-manager__quick-grid">
              <article class="firewall-manager__quick-card">
                <header>
                  <span class="firewall-manager__quick-icon firewall-manager__quick-icon--blue"><Globe2 :size="18" /></span>
                  <div>
                    <h4>{{ phrase('端口规则') }}</h4>
                    <p>{{ phrase('让外部设备访问某个服务。') }}</p>
                  </div>
                </header>
                <label class="field">
                  <span>{{ phrase('端口号') }}</span>
                  <input v-model.number="port" type="number" min="1" max="65535" inputmode="numeric" :aria-describedby="'firewall-port-help'" />
                </label>
                <div class="firewall-manager__actions">
                  <button class="button button--secondary" type="button" :disabled="!writable || running || !validPort" @click="portAction('firewall-close-port')">{{ phrase('阻止') }}</button>
                  <button class="button button--primary" type="button" :disabled="!writable || running || !validPort" @click="portAction('firewall-open-port')">{{ phrase('允许') }}</button>
                </div>
                <p id="firewall-port-help" class="firewall-manager__hint">{{ phrase('端口范围是 1–65535，例如网站常用 80 或 443。') }}</p>
              </article>

              <article class="firewall-manager__quick-card">
                <header>
                  <span class="firewall-manager__quick-icon firewall-manager__quick-icon--amber"><ShieldAlert :size="18" /></span>
                  <div>
                    <h4>{{ phrase('IP / 网段规则') }}</h4>
                    <p>{{ phrase('按 IP、网段或国家/地区管理来源。') }}</p>
                  </div>
                </header>
                <div class="firewall-manager__rule-mode" role="group" :aria-label="phrase('规则类型')">
                  <button type="button" :class="{ 'is-active': ruleMode === 'address' }" :aria-pressed="ruleMode === 'address'" data-firewall-rule-mode="address" @click="ruleMode = 'address'">{{ phrase('IP / 网段') }}</button>
                  <button type="button" :class="{ 'is-active': ruleMode === 'country' }" :aria-pressed="ruleMode === 'country'" data-firewall-rule-mode="country" @click="ruleMode = 'country'">{{ phrase('国家/地区') }}</button>
                </div>
                <template v-if="ruleMode === 'address'">
                  <label class="field">
                    <span>{{ phrase('IP 地址或网段') }}</span>
                    <input v-model.trim="address" autocomplete="off" maxlength="43" :placeholder="phrase('例如 198.51.100.20 或 198.51.100.0/24')" />
                  </label>
                  <div class="firewall-manager__actions">
                    <button class="button button--secondary" type="button" :disabled="!writable || running || !addressReady" @click="addressAction('firewall-remove-ip')">{{ phrase('清除') }}</button>
                    <button class="button button--danger" type="button" :disabled="!writable || running || !addressReady" @click="addressAction('firewall-block-ip')">{{ phrase('阻止') }}</button>
                    <button class="button button--primary" type="button" :disabled="!writable || running || !addressReady" @click="addressAction('firewall-allow-ip')">{{ phrase('允许') }}</button>
                  </div>
                  <p class="firewall-manager__hint">{{ phrase('支持 IPv4 地址或网段，例如 198.51.100.0/24。') }}</p>
                </template>
                <template v-else>
                  <label class="field">
                    <span>{{ phrase('国家/地区代码') }}</span>
                    <input v-model.trim="countryCode" autocomplete="off" maxlength="2" autocapitalize="characters" :placeholder="phrase('例如 US（美国）')" />
                  </label>
                  <div class="firewall-manager__actions">
                    <button class="button button--secondary" type="button" :disabled="!writable || running || !countryReady" @click="countryAction('firewall-remove-country')">{{ phrase('清除') }}</button>
                    <button class="button button--danger" type="button" :disabled="!writable || running || !countryReady" @click="countryAction('firewall-block-country')">{{ phrase('阻止') }}</button>
                    <button class="button button--primary" type="button" :disabled="!writable || running || !countryReady" @click="countryAction('firewall-allow-country')">{{ phrase('允许') }}</button>
                  </div>
                  <p class="firewall-manager__hint">{{ phrase('例如 US 代表美国；按该国家的 IPv4 网段匹配。允许会将默认入站策略设为阻止。') }}</p>
                </template>
              </article>
            </div>
          </section>

          <section class="firewall-manager__other-group firewall-manager__other-group--basic">
            <header class="firewall-manager__section-heading">
              <div>
                <h4>{{ phrase('基础防护') }}</h4>
                <p>{{ phrase('这些开关只调整 Ping 和异常连接防护，不会替你开放网站端口。') }}</p>
              </div>
            </header>
            <div class="firewall-manager__control-grid">
              <article class="firewall-manager__control">
                <span class="firewall-manager__control-icon firewall-manager__control-icon--blue"><Globe2 :size="19" /></span>
                <div>
                  <strong>{{ phrase('Ping 响应') }}</strong>
                  <p>{{ pingDetail() }}</p>
                </div>
                <button class="button button--secondary button--small" type="button" :disabled="!writable || running" @click="execute({ action: snapshot.pingAllowed ? 'firewall-disable-ping' : 'firewall-enable-ping' }, `确认${snapshot.pingAllowed ? '阻止' : '允许'} Ping 响应吗？`)">
                  {{ phrase(snapshot.pingAllowed ? '阻止 Ping' : '允许 Ping') }}
                </button>
              </article>
              <article class="firewall-manager__control">
                <span class="firewall-manager__control-icon firewall-manager__control-icon--amber"><Gauge :size="19" /></span>
                <div>
                  <strong>{{ phrase('异常连接防护') }}</strong>
                  <p>{{ ddosDetail() }}</p>
                </div>
                <button class="button button--secondary button--small" type="button" :disabled="!writable || running" @click="execute({ action: snapshot.ddosEnabled ? 'firewall-disable-ddos' : 'firewall-enable-ddos' }, `确认${snapshot.ddosEnabled ? '关闭' : '开启'}异常连接防护吗？`)">
                  {{ phrase(snapshot.ddosEnabled ? '关闭防护' : '开启防护') }}
                </button>
              </article>
            </div>
          </section>

        <details class="firewall-manager__advanced">
          <summary>
            <span class="firewall-manager__advanced-title">
              <ChevronDown :size="18" />
              <strong>{{ phrase('更多操作') }}</strong>
              <small>{{ phrase('全部访问设置和原始规则') }}</small>
            </span>
          </summary>
          <div class="firewall-manager__advanced-body">
            <section class="firewall-manager__advanced-section firewall-manager__advanced-section--danger">
              <div class="firewall-manager__danger-copy">
                <span class="firewall-manager__control-icon firewall-manager__control-icon--danger"><TriangleAlert :size="19" /></span>
                <div>
                  <h3>{{ phrase('全部端口') }}</h3>
                  <p>{{ allPortsDetail() }}</p>
                </div>
              </div>
              <button class="button button--danger" type="button" :disabled="!writable || running || !allPortsAction" @click="runAllPorts">
                {{ allPortsLabel() }}
              </button>
            </section>

            <details class="firewall-manager__raw">
              <summary>
                <span class="firewall-manager__raw-title">
                  <ChevronDown :size="17" />
                  <strong>{{ phrase('查看原始规则（排查用）') }}</strong>
                  <small>{{ phrase('普通操作不需要查看这部分') }}</small>
                </span>
              </summary>
              <div class="firewall-manager__raw-body">
                <div class="firewall-manager__technical-grid">
                  <div><span>{{ phrase('防火墙后端') }}</span><code data-i18n-ignore>{{ snapshot.backend || phrase('未识别') }}</code></div>
                  <div><span>{{ phrase('默认入站策略') }}</span><code data-i18n-ignore>{{ snapshot.inputPolicy || phrase('未识别') }}</code></div>
                  <div><span>{{ phrase('底层规则数量') }}</span><strong>{{ snapshot.total }}</strong></div>
                </div>
                <div v-if="snapshot.truncated" class="inline-alert inline-alert--warning">
                  {{ phrase('规则较多，仅显示前 512 条。') }}
                </div>
                <EmptyState v-if="!rules.length" :title="phrase('暂无防火墙规则')" :description="phrase('当前后端没有返回可展示的规则。')" />
                <div v-else class="firewall-manager__raw-rules" :aria-label="phrase('底层防火墙规则')">
                  <article v-for="(rule, index) in rules" :key="`${rule.line ?? index}-${rule.raw}`" class="firewall-manager__raw-rule">
                    <div>
                      <strong data-i18n-ignore>{{ rule.chain }} · {{ rule.target || '-' }} · {{ rule.protocol }}</strong>
                      <span data-i18n-ignore>{{ rule.source }} → {{ rule.destination }}</span>
                      <code data-i18n-ignore>{{ rule.options.join(' ') || rule.raw }}</code>
                    </div>
                    <small>{{ phrase('第') }} {{ rule.line }} {{ phrase('行') }}</small>
                  </article>
                </div>
              </div>
            </details>
          </div>
        </details>
        </section>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.firewall-manager {
  gap: 16px;
}

.firewall-manager__status,
.firewall-manager__section,
.firewall-manager__advanced-section {
  display: grid;
  gap: 16px;
  padding: 16px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 12px;
}

.firewall-manager__status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.firewall-manager__status-wrap {
  display: grid;
  gap: 10px;
}

.firewall-manager__status-main,
.firewall-manager__danger-copy,
.firewall-manager__control,
.firewall-manager__advanced-title,
.firewall-manager__raw-title,
.firewall-manager__quick-card > header,
.firewall-manager__rule-badges {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.firewall-manager__status-main > div,
.firewall-manager__danger-copy > div,
.firewall-manager__control > div,
.firewall-manager__quick-card > header > div {
  min-width: 0;
}

.firewall-manager__status-icon,
.firewall-manager__control-icon,
.firewall-manager__quick-icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 10px;
}

.firewall-manager__status-icon {
  width: 44px;
  height: 44px;
}

.firewall-manager__status.is-protected .firewall-manager__status-icon,
.firewall-manager__control-icon--blue,
.firewall-manager__quick-icon--blue {
  color: var(--brand-strong);
  background: var(--brand-soft);
}

.firewall-manager__status.is-open .firewall-manager__status-icon,
.firewall-manager__control-icon--amber,
.firewall-manager__quick-icon--amber {
  color: var(--amber);
  background: var(--amber-soft);
}

.firewall-manager__status.is-unknown .firewall-manager__status-icon,
.firewall-manager__control-icon--danger {
  color: var(--danger);
  background: var(--danger-soft);
}

.firewall-manager__eyebrow,
.firewall-manager__filter-bar > span,
.firewall-manager__address-grid span,
.firewall-manager__technical-grid span {
  display: block;
  color: var(--muted);
  font-size: 13px;
}

.firewall-manager__eyebrow {
  margin-bottom: 3px;
}

.firewall-manager__status strong {
  display: block;
  color: var(--text);
  font-size: 18px;
  line-height: 1.3;
}

.firewall-manager__status p,
.firewall-manager__section-heading p,
.firewall-manager__control p,
.firewall-manager__danger-copy p,
.firewall-manager__quick-card header p,
.firewall-manager__rule-note {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}

.firewall-manager__section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.firewall-manager__other {
  gap: 20px;
}

.firewall-manager__other-group {
  display: grid;
  gap: 12px;
  order: 2;
}

.firewall-manager__other-group--quick {
  order: 1;
}

.firewall-manager__advanced {
  order: 3;
}

.firewall-manager__section-heading h3,
.firewall-manager__danger-copy h3,
.firewall-manager__other-group h4,
.firewall-manager__quick-card h4 {
  margin: 0;
  color: var(--text);
  font-size: 16px;
}

.firewall-manager__tabs {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 4px;
  padding: 4px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 12px;
}

.firewall-manager__tab {
  min-width: 0;
  min-height: 42px;
  padding: 8px 12px;
  color: var(--text-soft);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 650;
}

.firewall-manager__tab:hover,
.firewall-manager__tab:focus-visible {
  background: var(--surface);
  border-color: var(--border-strong);
}

.firewall-manager__tab:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
}

.firewall-manager__tab.is-active {
  color: var(--brand-strong);
  background: var(--surface);
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border));
  box-shadow: var(--shadow-sm);
}

.firewall-manager__rule-count {
  flex: 0 0 auto;
  padding: 4px 8px;
  color: var(--text-soft);
  background: var(--surface-raised);
  border: 1px solid var(--border);
  border-radius: 999px;
  font-size: 12px;
  line-height: 1.3;
}

.firewall-manager__hint {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}

.firewall-manager__control-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.firewall-manager__control {
  align-items: flex-start;
  min-height: 86px;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.firewall-manager__control-icon,
.firewall-manager__quick-icon {
  width: 36px;
  height: 36px;
}

.firewall-manager__control > div {
  flex: 1 1 auto;
}

.firewall-manager__control strong {
  color: var(--text);
  font-size: 15px;
}

.firewall-manager__control .button {
  flex: 0 0 auto;
  align-self: center;
}

.firewall-manager__filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.firewall-manager__filter-bar > span {
  flex: 0 0 auto;
  color: var(--muted);
  font-size: 13px;
}

.firewall-manager__filter-options {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.firewall-manager__filter-option {
  min-height: 38px;
  padding: 7px 12px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 550;
}

.firewall-manager__filter-option:hover,
.firewall-manager__filter-option:focus-visible {
  border-color: var(--border-strong);
}

.firewall-manager__filter-option:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
}

.firewall-manager__filter-option.is-active {
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border));
}

.firewall-manager__parsed-rules {
  display: grid;
  overflow: hidden;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.firewall-manager__rule-list-head,
.firewall-manager__parsed-rule {
  display: grid;
  grid-template-columns: minmax(118px, 0.78fr) minmax(150px, 1fr) minmax(150px, 1fr) minmax(150px, 1fr);
  align-items: center;
  column-gap: 14px;
}

.firewall-manager__rule-list-head {
  min-height: 38px;
  padding: 0 14px;
  color: var(--muted);
  background: var(--surface-subtle);
  border-bottom: 1px solid var(--border);
  font-size: 12px;
  font-weight: 650;
}

.firewall-manager__parsed-rule {
  min-height: 70px;
  margin: 0;
  padding: 12px 14px;
  background: var(--surface);
  border: 0;
  border-bottom: 1px solid var(--border);
  gap: 14px;
}

.firewall-manager__parsed-rule:last-child {
  border-bottom: 0;
}

.firewall-manager__parsed-rule-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

.firewall-manager__decision,
.firewall-manager__zone-badge {
  display: inline-flex;
  min-height: 30px;
  align-items: center;
  gap: 6px;
  padding: 5px 9px;
  border-radius: 7px;
  font-size: 13px;
  font-weight: 650;
}

.firewall-manager__decision.is-allow {
  color: var(--success-strong, var(--brand-strong));
  background: var(--success-soft, var(--brand-soft));
}

.firewall-manager__decision.is-block {
  color: var(--danger);
  background: var(--danger-soft);
}

.firewall-manager__decision.is-other,
.firewall-manager__zone-badge {
  color: var(--text-soft);
  background: var(--surface-raised);
  border: 1px solid var(--border);
}

.firewall-manager__parsed-rule-title {
  color: var(--text);
  font-size: 15px;
  line-height: 1.4;
}

.firewall-manager__rule-connection,
.firewall-manager__rule-address {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.firewall-manager__rule-list-label {
  display: none;
  color: var(--muted);
  font-size: 12px;
}

.firewall-manager__rule-address strong {
  overflow-wrap: anywhere;
  color: var(--text-soft);
  font-size: 13px;
  font-weight: 600;
}

.firewall-manager__rule-note {
  grid-column: 1 / -1;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

.firewall-manager__rule-empty {
  display: grid;
  justify-items: center;
  gap: 7px;
  padding: 24px 16px;
  color: var(--muted);
  text-align: center;
  background: var(--surface);
  border: 1px dashed var(--border-strong);
  border-radius: 10px;
}

.firewall-manager__rule-empty svg {
  color: var(--brand-strong);
}

.firewall-manager__rule-empty strong {
  color: var(--text);
  font-size: 15px;
}

.firewall-manager__rule-empty p {
  margin: 0;
  font-size: 13px;
  line-height: 1.55;
}

.firewall-manager__quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.firewall-manager__quick-card {
  display: grid;
  gap: 12px;
  min-width: 0;
  padding: 14px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.firewall-manager__quick-card > header {
  align-items: flex-start;
}

.firewall-manager__quick-card > header > div {
  flex: 1 1 auto;
}

.firewall-manager__quick-card h4 {
  font-size: 15px;
}

.firewall-manager__quick-card header p {
  margin-top: 3px;
}

.firewall-manager__quick-card .field {
  gap: 6px;
}

.firewall-manager__rule-mode {
  display: inline-flex;
  width: fit-content;
  padding: 3px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 8px;
}

.firewall-manager__rule-mode button {
  min-height: 32px;
  padding: 5px 10px;
  color: var(--text-soft);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}

.firewall-manager__rule-mode button:hover,
.firewall-manager__rule-mode button:focus-visible {
  border-color: var(--border-strong);
}

.firewall-manager__rule-mode button:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
}

.firewall-manager__rule-mode button.is-active {
  color: var(--brand-strong);
  background: var(--surface);
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border));
  box-shadow: var(--shadow-sm);
}

.firewall-manager__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.firewall-manager__actions .button {
  min-width: 76px;
}

.firewall-manager__advanced {
  border-top: 1px solid var(--border);
}

.firewall-manager__advanced > summary,
.firewall-manager__raw > summary {
  display: flex;
  min-height: 46px;
  align-items: center;
  color: var(--text-soft);
  cursor: pointer;
  list-style: none;
}

.firewall-manager__advanced > summary::-webkit-details-marker,
.firewall-manager__raw > summary::-webkit-details-marker {
  display: none;
}

.firewall-manager__advanced > summary:focus-visible,
.firewall-manager__raw > summary:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
  border-radius: 6px;
}

.firewall-manager__advanced-title,
.firewall-manager__raw-title {
  width: 100%;
}

.firewall-manager__advanced-title > svg,
.firewall-manager__raw-title > svg {
  flex: 0 0 auto;
  color: var(--muted);
  transition: transform 160ms ease;
}

.firewall-manager__advanced[open] > summary .firewall-manager__advanced-title > svg,
.firewall-manager__raw[open] > summary .firewall-manager__raw-title > svg {
  transform: rotate(180deg);
}

.firewall-manager__advanced-title strong,
.firewall-manager__raw-title strong {
  font-size: 15px;
}

.firewall-manager__advanced-title small,
.firewall-manager__raw-title small {
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.firewall-manager__advanced-body,
.firewall-manager__raw-body {
  display: grid;
  gap: 12px;
}

.firewall-manager__advanced-body {
  padding-bottom: 2px;
}

.firewall-manager__advanced-section--danger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  background: var(--danger-soft);
  border-color: color-mix(in srgb, var(--danger) 28%, var(--border));
}

.firewall-manager__danger-copy {
  align-items: flex-start;
}

.firewall-manager__advanced-section--danger .button {
  flex: 0 0 auto;
}

.firewall-manager__raw {
  padding: 0 12px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.firewall-manager__raw-body {
  padding-bottom: 12px;
}

.firewall-manager__technical-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.firewall-manager__technical-grid > div {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
}

.firewall-manager__technical-grid code,
.firewall-manager__technical-grid strong {
  overflow-wrap: anywhere;
  color: var(--text-soft);
  font-size: 13px;
  font-weight: 650;
}

.firewall-manager__raw-rules {
  display: grid;
  max-height: 360px;
  gap: 8px;
  overflow: auto;
}

.firewall-manager__raw-rule {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 9px;
}

.firewall-manager__raw-rule > div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.firewall-manager__raw-rule strong {
  overflow-wrap: anywhere;
  color: var(--text);
  font-size: 14px;
}

.firewall-manager__raw-rule span,
.firewall-manager__raw-rule small {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.firewall-manager__raw-rule code {
  overflow-wrap: anywhere;
  color: var(--text-soft);
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 900px) {
  .firewall-manager__tabs {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 680px) {
  .firewall-manager__status,
  .firewall-manager__advanced-section--danger {
    align-items: stretch;
    flex-direction: column;
  }

  .firewall-manager__control-grid,
  .firewall-manager__quick-grid,
  .firewall-manager__technical-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .firewall-manager__parsed-rules {
    overflow: visible;
    background: transparent;
    border: 0;
    border-radius: 0;
    gap: 8px;
  }

  .firewall-manager__rule-list-head {
    display: none;
  }

  .firewall-manager__parsed-rule {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    min-height: 0;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: 10px;
  }

  .firewall-manager__parsed-rule:last-child {
    border-bottom: 1px solid var(--border);
  }

  .firewall-manager__parsed-rule-head,
  .firewall-manager__rule-connection {
    grid-column: 1 / -1;
  }

  .firewall-manager__rule-list-label {
    display: block;
  }

  .firewall-manager__filter-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .firewall-manager__actions {
    justify-content: stretch;
  }

  .firewall-manager__actions .button {
    flex: 1 1 100px;
  }

  .firewall-manager__control .button,
  .firewall-manager__advanced-section--danger .button {
    align-self: stretch;
  }

  .firewall-manager__raw-rule {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .firewall-manager__advanced-title > svg,
  .firewall-manager__raw-title > svg {
    transition: none;
  }
}
</style>

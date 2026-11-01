<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { CheckCircle2, Download, Globe2, LoaderCircle, LockKeyhole, ShieldAlert } from '@lucide/vue'
import { api, ApiError } from '@/lib/api'
import { matchingAppProxySites } from '@/lib/appAccess'
import { formatHostDateTime } from '@/lib/format'
import { kpanelAppID } from '@/lib/kpanelUpdate'
import { currentPanelLocation, navigatePanel, panelAccessSection, panelDomainQuery } from '@/lib/panelAccess'
import { useI18n } from '@/i18n'
import DnsResolutionGuide from '@/components/common/DnsResolutionGuide.vue'
import type { MessageKey } from '@/i18n/messages/zh-CN'
import type { AppMarketItem, PanelAccessStatus, PanelDomainCheck, PublicNetworkSummary, Site } from '@/types/api'

const i18n = useI18n()
const route = useRoute()
const controller = new AbortController()
const root = ref<HTMLElement>()
const status = ref<PanelAccessStatus>()
const kpanelItem = ref<AppMarketItem>()
const boundSites = ref<Site[]>([])
const publicNetwork = ref<PublicNetworkSummary>()
const loading = ref(true)
const error = ref('')
const domainInput = ref('')
const domainError = ref('')
const binding = ref(false)
const checking = ref('')
const check = ref<PanelDomainCheck>()
const blockDirect = ref(true)
const switching = ref(false)
const switchError = ref('')
const tlsBusy = ref(false)
const tlsError = ref('')
const confirmingRegenerate = ref(false)

const pageLocation = currentPanelLocation()
const pageSecure = pageLocation.protocol === 'https:'
const currentOrigin = pageLocation.origin
// Certificates outlive a year, so the expiry must show the year.
const browserTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
const accessKind = computed<'trusted' | 'self-signed' | 'plain'>(() => {
  if (!pageSecure) return 'plain'
  return status.value?.requestIpHost ? 'self-signed' : 'trusted'
})
const kpanelPort = computed(() =>
  kpanelItem.value?.runtime.ports?.find((port) => port.type === 'tcp' && port.publicPort)?.publicPort,
)
const canBindDomain = computed(() => Boolean(
  kpanelItem.value && kpanelPort.value && kpanelItem.value.capabilities.add_domain?.enabled,
))
const canBlockDirect = computed(() => {
  const item = kpanelItem.value
  return Boolean(
    item?.capabilities.direct_access?.enabled &&
    item.runtime.accessMode !== 'domain_only' &&
    item.runtime.resourceVersion,
  )
})
const selfSigned = computed(() => status.value?.selfSignedTls)
const showSelfSigned = computed(() => Boolean(status.value?.allowIpHosts))
const limitations = computed<MessageKey[]>(() => accessKind.value === 'plain'
  ? ['panelAccess.limit.plainEncryption', 'panelAccess.limit.plainCookie', 'panelAccess.limit.plainPasskey', 'panelAccess.limit.plainLightNode', 'panelAccess.limit.plainClipboard']
  : accessKind.value === 'self-signed'
    ? ['panelAccess.limit.selfSignedTrust', 'panelAccess.limit.plainPasskey', 'panelAccess.limit.plainLightNode']
    : [])

function problemMessage(reason: unknown, fallback: MessageKey): string {
  return reason instanceof ApiError ? reason.message : i18n.t(fallback)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [access, inventory, sites, network] = await Promise.all([
      api.settings.panelAccess.get(controller.signal),
      api.apps.inventory(controller.signal).catch(() => undefined),
      api.sites.list(undefined, controller.signal).catch(() => undefined),
      api.system.publicNetwork(controller.signal).catch(() => undefined),
    ])
    status.value = access
    publicNetwork.value = network
    kpanelItem.value = inventory?.items.find((item) => item.id === kpanelAppID && item.runtime.installed)
    boundSites.value = kpanelItem.value && sites ? matchingAppProxySites(kpanelItem.value, sites.items) : []
  } catch (reason) {
    if (!controller.signal.aborted) error.value = problemMessage(reason, 'panelAccess.loadFailed')
  } finally {
    loading.value = false
  }
}

async function runCheck(domain: string): Promise<void> {
  checking.value = domain
  check.value = undefined
  switchError.value = ''
  try {
    check.value = await api.settings.panelAccess.checkDomain(domain, controller.signal)
  } catch (reason) {
    if (!controller.signal.aborted) {
      check.value = { domain, reachable: false, matched: false, problem: 'unreachable' }
      switchError.value = problemMessage(reason, 'panelAccess.checkFailed')
    }
  } finally {
    checking.value = ''
  }
}

async function bindDomain(): Promise<void> {
  const hostname = domainInput.value.trim().toLowerCase()
  if (!hostname || !kpanelPort.value || binding.value) return
  binding.value = true
  domainError.value = ''
  try {
    const created = await api.sites.create({
      primaryDomain: hostname,
      aliases: [],
      type: 'proxy',
      upstream: `http://127.0.0.1:${kpanelPort.value}`,
      enabled: true,
    })
    boundSites.value = [created, ...boundSites.value.filter((site) => site.id !== created.id)]
    domainInput.value = ''
    await runCheck(created.primaryDomain)
  } catch (reason) {
    domainError.value = problemMessage(reason, 'panelAccess.bindFailed')
  } finally {
    binding.value = false
  }
}

// Order matters: the new address is verified first, the IP block is only
// submitted afterwards and the browser leaves right away, so the current
// IP session is never cut off before a working address exists.
async function switchToDomain(): Promise<void> {
  const result = check.value
  const item = kpanelItem.value
  if (!result?.matched || switching.value) return
  switching.value = true
  switchError.value = ''
  try {
    if (blockDirect.value && canBlockDirect.value && item?.runtime.resourceVersion) {
      await api.apps.action(item.id, 'direct_access', {
        resourceVersion: item.runtime.resourceVersion,
        accessMode: 'domain_only',
      })
    }
    navigatePanel(`https://${result.domain}/`)
  } catch (reason) {
    switchError.value = problemMessage(reason, 'panelAccess.blockFailed')
    switching.value = false
  }
}

async function updateSelfSigned(enabled: boolean, regenerate = false): Promise<void> {
  if (tlsBusy.value) return
  tlsBusy.value = true
  tlsError.value = ''
  try {
    status.value = await api.settings.panelAccess.updateSelfSignedTLS({ enabled, regenerate })
    confirmingRegenerate.value = false
	if (enabled) {
	  navigatePanel(`https://${pageLocation.host}/settings?section=${panelAccessSection}`)
	  return
	}
    // Disabling stops new TLS handshakes; leave the self-signed address
    // before the current keep-alive connection is closed.
    if (!enabled && accessKind.value === 'self-signed') {
      navigatePanel(`http://${pageLocation.host}/settings?section=${panelAccessSection}`)
    } else {
      navigatePanel(`${pageLocation.origin}/settings?section=${panelAccessSection}`)
    }
  } catch (reason) {
    tlsError.value = problemMessage(reason, 'panelAccess.selfSignedFailed')
  } finally {
    tlsBusy.value = false
  }
}

function openSelfSignedAddress(): void {
  navigatePanel(`https://${pageLocation.host}/settings?section=${panelAccessSection}`)
}

function problemLabel(result: PanelDomainCheck): string {
  const key = `panelAccess.problem.${result.problem || 'unreachable'}` as MessageKey
  return i18n.t(key)
}

async function focusFromIntent(): Promise<void> {
  if (route.query.section !== panelAccessSection) return
  await nextTick()
  // Let the router finish its own scroll first. Background tabs and hidden
  // windows may never deliver a frame, so a timer bounds the wait.
  await new Promise<void>((resolve) => {
    requestAnimationFrame(() => resolve())
    setTimeout(resolve, 100)
  })
  root.value?.scrollIntoView?.({ block: 'start' })
  root.value?.focus({ preventScroll: true })
}

onMounted(async () => {
  await load()
  await focusFromIntent()
  const requested = panelDomainQuery(route.query.domain)
  if (requested && !check.value) await runCheck(requested)
})
onBeforeUnmount(() => controller.abort())
</script>

<template>
  <section
    ref="root"
    class="panel-access settings-section panel-card"
    aria-labelledby="panel-access-title"
    tabindex="-1"
  >
    <header class="panel-access__heading settings-section__header">
      <Globe2 :size="21" aria-hidden="true" />
      <div><h2 id="panel-access-title">{{ i18n.t('panelAccess.title') }}</h2><p>{{ i18n.t('panelAccess.description') }}</p></div>
    </header>
    <p v-if="loading" role="status">{{ i18n.t('panelAccess.loading') }}</p>
    <div v-else-if="error" class="inline-alert inline-alert--danger" role="alert">
      {{ error }}
      <button class="button-link" type="button" @click="load">{{ i18n.t('panelAccess.retry') }}</button>
    </div>
    <template v-else-if="status">
      <div class="panel-access__current" :data-kind="accessKind">
        <component :is="accessKind === 'trusted' ? CheckCircle2 : accessKind === 'self-signed' ? LockKeyhole : ShieldAlert" :size="18" aria-hidden="true" />
        <div>
          <strong>{{ i18n.t(`panelAccess.kind.${accessKind}`) }}</strong>
          <span class="panel-access__origin">{{ i18n.t('panelAccess.currentOrigin') }} <code>{{ currentOrigin }}</code></span>
        </div>
      </div>
      <ul v-if="limitations.length" class="panel-access__limits">
        <li v-for="item in limitations" :key="item">{{ i18n.t(item) }}</li>
      </ul>

      <section class="panel-access__block" aria-labelledby="panel-access-domain-title">
        <h3 id="panel-access-domain-title">{{ i18n.t('panelAccess.domainTitle') }}</h3>
        <p class="panel-access__note">{{ i18n.t('panelAccess.domainIntro') }}</p>
        <template v-if="canBindDomain">
          <ul v-if="boundSites.length" class="panel-access__domains">
            <li v-for="site in boundSites" :key="site.id">
              <span>{{ site.primaryDomain }}</span>
              <button
                class="button button--secondary"
                type="button"
                :disabled="Boolean(checking) || switching"
                @click="runCheck(site.primaryDomain)"
              >
                <LoaderCircle v-if="checking === site.primaryDomain" class="spin" :size="15" />
                {{ i18n.t('panelAccess.checkDomain') }}
              </button>
            </li>
          </ul>
          <form class="panel-access__form" @submit.prevent="bindDomain">
            <label class="field">
              <span>{{ i18n.t('panelAccess.domainLabel') }}</span>
              <input v-model.trim="domainInput" placeholder="panel.example.com" autocomplete="off" :disabled="binding" required />
            </label>
            <button class="button button--primary" type="submit" :disabled="binding || !domainInput || Boolean(checking)">
              <LoaderCircle v-if="binding" class="spin" :size="15" />
              {{ i18n.t('panelAccess.bindDomain') }}
            </button>
          </form>
          <p class="panel-access__note">{{ i18n.t('panelAccess.domainDnsHint') }}</p>
          <DnsResolutionGuide :ipv4="publicNetwork?.ipv4" :ipv6="publicNetwork?.ipv6" />
          <p v-if="domainError" class="inline-alert inline-alert--danger" role="alert">{{ domainError }}</p>
        </template>
        <p v-else class="inline-alert inline-alert--info">{{ i18n.t('panelAccess.domainManual') }}</p>
        <p v-if="checking" role="status">{{ i18n.t('panelAccess.checking', { domain: checking }) }}</p>

        <div v-if="check && check.matched" class="panel-access__switch" role="region" :aria-label="i18n.t('panelAccess.switchTitle')">
          <p class="inline-alert inline-alert--success"><CheckCircle2 :size="16" aria-hidden="true" />{{ i18n.t('panelAccess.matched', { origin: `https://${check.domain}` }) }}</p>
          <label v-if="canBlockDirect" class="panel-access__check">
            <input v-model="blockDirect" type="checkbox" :disabled="switching" />
            <span>{{ i18n.t('panelAccess.blockDirect') }}</span>
          </label>
          <p class="panel-access__note">{{ i18n.t('panelAccess.reloginNotice') }}</p>
          <p v-if="blockDirect && canBlockDirect" class="panel-access__note">{{ i18n.t('panelAccess.blockRecovery') }}</p>
          <button class="button button--primary" type="button" :disabled="switching" @click="switchToDomain">
            <LoaderCircle v-if="switching" class="spin" :size="15" />
            {{ i18n.t('panelAccess.switch') }}
          </button>
        </div>
        <p v-else-if="check" class="inline-alert inline-alert--warning" role="alert">
          {{ i18n.t('panelAccess.notReady', { domain: check.domain }) }} {{ problemLabel(check) }}
        </p>
        <p v-if="switchError" class="inline-alert inline-alert--danger" role="alert">{{ switchError }}</p>
      </section>

      <section v-if="showSelfSigned" class="panel-access__block" aria-labelledby="panel-access-tls-title">
        <h3 id="panel-access-tls-title">{{ i18n.t('panelAccess.selfSignedTitle') }}</h3>
        <p class="panel-access__note">{{ i18n.t('panelAccess.selfSignedIntro') }}</p>
        <p v-if="status.selfSignedError" class="inline-alert inline-alert--warning">{{ i18n.t('panelAccess.selfSignedLoadError') }}</p>
        <dl v-if="selfSigned?.fingerprintSha256" class="panel-access__cert">
          <div><dt>{{ i18n.t('panelAccess.state') }}</dt><dd>{{ i18n.t(selfSigned.enabled ? 'panelAccess.enabled' : 'panelAccess.disabled') }}</dd></div>
          <div><dt>{{ i18n.t('panelAccess.fingerprint') }}</dt><dd><code>{{ selfSigned.fingerprintSha256 }}</code></dd></div>
          <div v-if="selfSigned.notAfter"><dt>{{ i18n.t('panelAccess.validUntil') }}</dt><dd>{{ formatHostDateTime(selfSigned.notAfter, browserTimeZone) }}</dd></div>
          <div v-if="selfSigned.hosts?.length"><dt>{{ i18n.t('panelAccess.hosts') }}</dt><dd>{{ selfSigned.hosts.join(', ') }}</dd></div>
        </dl>
        <p v-if="selfSigned?.enabled" class="panel-access__note">{{ i18n.t('panelAccess.verifyFingerprint') }}</p>
        <div class="panel-access__actions">
          <button
            v-if="!selfSigned?.enabled"
            class="button button--primary"
            type="button"
            :disabled="tlsBusy"
            @click="updateSelfSigned(true)"
          >
            <LoaderCircle v-if="tlsBusy" class="spin" :size="15" />
            {{ i18n.t('panelAccess.enableSelfSigned') }}
          </button>
          <button
            v-if="selfSigned?.enabled && !pageSecure"
            class="button button--primary"
            type="button"
            @click="openSelfSignedAddress"
          >
            <LockKeyhole :size="15" aria-hidden="true" />
            {{ i18n.t('panelAccess.openHttps') }}
          </button>
          <a
            v-if="selfSigned?.fingerprintSha256"
            class="button button--secondary"
            :href="api.settings.panelAccess.certificateURL()"
            download="kpanel-self-signed.crt"
          >
            <Download :size="15" aria-hidden="true" />
            {{ i18n.t('panelAccess.downloadCertificate') }}
          </a>
          <button
            v-if="selfSigned?.enabled && !confirmingRegenerate"
            class="button button--ghost"
            type="button"
            :disabled="tlsBusy"
            @click="confirmingRegenerate = true"
          >
            {{ i18n.t('panelAccess.regenerate') }}
          </button>
          <button
            v-if="selfSigned?.enabled"
            class="button button--ghost button--danger-text"
            type="button"
            :disabled="tlsBusy"
            @click="updateSelfSigned(false)"
          >
            {{ i18n.t('panelAccess.disableSelfSigned') }}
          </button>
        </div>
        <div v-if="confirmingRegenerate" class="panel-access__confirm">
          <p class="inline-alert inline-alert--warning">{{ i18n.t('panelAccess.regenerateConfirm') }}</p>
          <div class="panel-access__actions">
            <button class="button button--ghost" type="button" :disabled="tlsBusy" @click="confirmingRegenerate = false">{{ i18n.t('panelAccess.cancel') }}</button>
            <button class="button button--danger" type="button" :disabled="tlsBusy" @click="updateSelfSigned(true, true)">{{ i18n.t('panelAccess.regenerate') }}</button>
          </div>
        </div>
        <p v-if="tlsError" class="inline-alert inline-alert--danger" role="alert">{{ tlsError }}</p>
        <p class="panel-access__note">{{ i18n.t('panelAccess.selfSignedRecovery') }}</p>
        <code class="panel-access__command">cd /home/docker/kpanel &amp;&amp; docker compose --env-file .env run --rm --no-deps panel self-signed-tls disable &amp;&amp; docker compose --env-file .env restart panel</code>
      </section>
    </template>
  </section>
</template>

<style scoped>
.panel-access { padding: 24px; font-size: 14px; line-height: 1.6; }
.panel-access:focus { outline: none; }
.panel-access:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
.panel-access__heading { display: flex; align-items: flex-start; gap: 12px; }
.panel-access__heading > svg { flex-shrink: 0; margin-top: 3px; color: var(--brand); }
.panel-access__heading h2 { margin: 0; font-size: 17px; }
.panel-access__heading p { margin: 4px 0 16px; color: var(--text-soft); }
.panel-access__current { display: flex; align-items: flex-start; gap: 10px; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--radius); }
.panel-access__current > svg { flex-shrink: 0; margin-top: 3px; }
.panel-access__current[data-kind='trusted'] > svg { color: var(--success); }
.panel-access__current[data-kind='self-signed'] > svg { color: var(--brand); }
.panel-access__current[data-kind='plain'] > svg { color: var(--warning); }
.panel-access__current div { display: grid; gap: 2px; min-width: 0; }
.panel-access__origin { color: var(--text-soft); font-size: 13px; overflow-wrap: anywhere; }
.panel-access__origin code { font-size: 13px; }
.panel-access .inline-alert--success { color: var(--success); background: var(--success-soft); border-color: color-mix(in srgb, var(--success) 22%, transparent); }
.panel-access .inline-alert { font-size: 14px; }
.panel-access .inline-alert > svg { flex-shrink: 0; margin-top: 2px; }
.panel-access__limits { margin: 12px 0 0; padding-left: 20px; color: var(--text-soft); font-size: 13px; }
.panel-access__block { margin-top: 24px; padding-top: 20px; border-top: 1px solid var(--border); }
.panel-access__block h3 { margin: 0 0 4px; font-size: 15px; }
.panel-access__note { margin: 8px 0; color: var(--text-soft); font-size: 13px; overflow-wrap: anywhere; }
.panel-access__domains { list-style: none; margin: 12px 0; padding: 0; }
.panel-access__domains li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border); overflow-wrap: anywhere; }
.panel-access__form { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; margin-top: 12px; }
.panel-access__form .field { flex: 1 1 260px; min-width: 0; }
.panel-access :deep(.dns-guide) { margin-top: 12px; font-size: 14px; }
.panel-access :deep(.dns-guide header small), .panel-access :deep(.dns-guide p), .panel-access :deep(.dns-guide nav > span) { font-size: 13px; }
.panel-access :deep(.dns-guide header code) { font-size: 12px; }
.panel-access :deep(.dns-guide nav a), .panel-access :deep(.dns-guide__records code) { font-size: 14px; }
.panel-access :deep(.dns-guide__records b), .panel-access :deep(.dns-guide nav i) { font-size: 12px; }
.panel-access__switch { display: grid; gap: 8px; justify-items: start; margin-top: 12px; }
.panel-access__check { display: flex; align-items: flex-start; gap: 8px; cursor: pointer; }
.panel-access__check input { margin-top: 5px; }
.panel-access__cert { display: grid; gap: 8px; margin: 12px 0; }
.panel-access__cert div { display: grid; grid-template-columns: minmax(96px, max-content) 1fr; gap: 12px; }
.panel-access__cert dt { color: var(--text-soft); font-size: 13px; }
.panel-access__cert dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }
.panel-access__cert code, .panel-access__command { font-size: 13px; overflow-wrap: anywhere; }
.panel-access__command { display: block; padding: 10px 12px; border-radius: var(--radius-sm); background: var(--surface-muted, var(--surface-soft)); user-select: all; }
.panel-access__actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
.panel-access__actions .button--danger-text { margin-left: auto; }
.panel-access__confirm { margin-top: 12px; }
.panel-access .button, .panel-access input, .panel-access .button-link { font-size: 14px; }
@media (max-width: 600px) {
  .panel-access { padding: 16px; }
  .panel-access__cert div { grid-template-columns: 1fr; gap: 2px; }
  .panel-access__actions .button--danger-text { margin-left: 0; }
}
</style>

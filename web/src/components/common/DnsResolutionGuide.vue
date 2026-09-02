<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, Copy, ExternalLink, Network } from '@lucide/vue'
import { siAlibabacloud, siCloudflare, siHuawei, type SimpleIcon } from 'simple-icons'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const props = defineProps<{
  ipv4?: string
  ipv6?: string
  compact?: boolean
}>()

const copied = ref('')
let copiedTimer: number | undefined

const records = computed(() =>
  [
    props.ipv4 ? { type: 'A', value: props.ipv4 } : undefined,
    props.ipv6 ? { type: 'AAAA', value: props.ipv6 } : undefined,
  ].filter((record): record is { type: string; value: string } => Boolean(record)),
)

const providers: Array<{ name: string; url: string; icon?: SimpleIcon; fallback?: string }> = [
  { name: 'Cloudflare', url: 'https://dash.cloudflare.com/', icon: siCloudflare },
  { name: '阿里云', url: 'https://dns.console.aliyun.com/', icon: siAlibabacloud },
  { name: '腾讯云', url: 'https://console.cloud.tencent.com/cns', fallback: '腾' },
  { name: '华为云', url: 'https://console.huaweicloud.com/dns/', icon: siHuawei },
]

async function writeClipboard(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
  } catch {
    const input = document.createElement('textarea')
    input.value = value
    input.setAttribute('readonly', '')
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    input.remove()
  }
}

async function copy(value: string): Promise<void> {
  await writeClipboard(value)
  copied.value = value
  if (copiedTimer) window.clearTimeout(copiedTimer)
  copiedTimer = window.setTimeout(() => {
    copied.value = ''
  }, 1600)
}
</script>

<template>
  <aside class="dns-guide" :class="{ 'dns-guide--compact': compact }">
    <header>
      <span class="dns-guide__icon"><Network :size="17" /></span>
      <div>
        <strong>{{ phrase('先完成域名解析') }}</strong>
        <small>{{ phrase('在域名托管平台添加以下记录，主机记录通常填写') }} <code>@</code> {{ phrase('或子域名前缀。') }}</small>
      </div>
    </header>

    <div v-if="records.length" class="dns-guide__records">
      <button
        v-for="record in records"
        :key="record.type"
        type="button"
        :title="phrase(`复制 ${record.type} 记录地址`)"
        @click="copy(record.value)"
      >
        <b>{{ record.type }}</b>
        <code>{{ record.value }}</code>
        <Check v-if="copied === record.value" :size="14" />
        <Copy v-else :size="14" />
      </button>
    </div>
    <p v-else>{{ phrase('暂未识别本机公网 IP，请先在概览刷新“网络与位置”。') }}</p>

    <nav :aria-label="phrase('域名托管平台')">
      <span>{{ phrase('打开 DNS 控制台') }}</span>
      <a
        v-for="provider in providers"
        :key="provider.name"
        :href="provider.url"
        target="_blank"
        rel="noopener noreferrer"
      >
        <svg v-if="provider.icon" viewBox="0 0 24 24" aria-hidden="true">
          <path :d="provider.icon.path" :style="{ fill: `#${provider.icon.hex}` }" />
        </svg>
        <i v-else aria-hidden="true">{{ provider.fallback }}</i>
        {{ phrase(provider.name) }}
        <ExternalLink :size="11" />
      </a>
    </nav>
  </aside>
</template>

<style scoped>
.dns-guide {
  display: grid;
  gap: 11px;
  padding: 14px;
  border: 1px solid color-mix(in srgb, var(--blue) 22%, var(--border));
  border-radius: 12px;
  background: color-mix(in srgb, var(--blue-soft) 58%, var(--surface));
}

.dns-guide header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.dns-guide header > div {
  display: grid;
  gap: 3px;
}

.dns-guide header small,
.dns-guide p,
.dns-guide nav > span {
  color: var(--muted);
  font-size: 11px;
  line-height: 1.5;
}

.dns-guide p {
  margin: 0;
}

.dns-guide__icon {
  display: grid;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 9px;
  color: var(--blue);
  background: var(--surface);
}

.dns-guide__records,
.dns-guide nav {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}

.dns-guide__records button,
.dns-guide nav a {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  min-height: 31px;
  padding: 5px 9px;
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text);
  background: var(--surface);
  text-decoration: none;
}

.dns-guide__records button {
  cursor: pointer;
}

.dns-guide__records b {
  color: var(--blue);
  font-size: 10px;
}

.dns-guide__records code {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: left;
  white-space: nowrap;
}

.dns-guide nav a {
  font-size: 11px;
  font-weight: 600;
}

.dns-guide nav svg,
.dns-guide nav i {
  width: 15px;
  height: 15px;
  flex: 0 0 auto;
  fill: currentColor;
}

.dns-guide nav i {
  display: grid;
  place-items: center;
  border-radius: 4px;
  color: #fff;
  background: #1766d2;
  font-size: 9px;
  font-style: normal;
}

.dns-guide__records button:hover,
.dns-guide nav a:hover {
  border-color: color-mix(in srgb, var(--blue) 40%, var(--border));
  color: var(--blue);
}

.dns-guide--compact {
  grid-template-columns: minmax(172px, 0.6fr) minmax(0, 1.6fr);
  gap: 6px 8px;
  padding: 9px 10px;
}

.dns-guide--compact header {
  grid-row: 1 / span 2;
}

.dns-guide--compact .dns-guide__records,
.dns-guide--compact nav {
  grid-column: 2;
}

.dns-guide--compact .dns-guide__records button,
.dns-guide--compact nav a {
  min-height: 26px;
  padding: 2px 5px;
}

.dns-guide--compact .dns-guide__records,
.dns-guide--compact nav {
  flex-wrap: nowrap;
  gap: 4px;
}

.dns-guide--compact nav > span,
.dns-guide--compact nav a {
  font-size: 10px;
  white-space: nowrap;
}

.dns-guide--compact nav svg,
.dns-guide--compact nav i {
  width: 13px;
  height: 13px;
}

.dns-guide--compact nav a {
  gap: 4px;
}

@media (max-width: 640px) {
  .dns-guide--compact {
    grid-template-columns: 1fr;
  }

  .dns-guide--compact header,
  .dns-guide--compact .dns-guide__records,
  .dns-guide--compact nav {
    grid-column: 1;
    grid-row: auto;
    flex-wrap: wrap;
  }

  .dns-guide__records button {
    flex: 1 1 100%;
  }
}
</style>

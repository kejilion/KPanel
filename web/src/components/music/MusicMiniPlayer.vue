<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Music2, Pause, Play, SkipForward, X } from '@lucide/vue'
import { useMusicPlayer } from '@/stores/musicPlayer'
import { useDesktopMode } from '@/stores/desktopMode'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { galleryParentPath } from '@/lib/gallery'
const player = useMusicPlayer(), state = player.state, desktop = useDesktopMode(), router = useRouter()
const failed = ref(false)
watch(player.cover, () => { failed.value = false })
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const progress = computed(() => state.duration ? `${Math.min(100, state.position / state.duration * 100)}%` : '0%')
function open() {
  const track = state.current
  if (!track) return
  const query = new URLSearchParams({ view: 'listen', path: galleryParentPath(track.entry.path), ...(track.hostId ? { hostId: track.hostId } : {}) })
  if (desktop.mode.value === 'desktop') desktop.openWindow(`/music?${query}`, 'route.music', false, true)
  else void router.push(`/music?${query}`)
}
</script>
<template>
  <aside v-if="state.current && !state.viewCount" class="music-mini" :aria-label="phrase('迷你音乐播放器')">
    <button class="music-mini__song" :aria-label="phrase('返回音乐播放器')" @click="open"><img v-if="player.cover.value && !failed" :src="player.cover.value" alt="" @error="failed = true" /><Music2 v-else :size="28" /><span><strong>{{ state.current.title }}</strong><small>{{ state.error ? phrase('播放遇到问题，返回播放器重试') : state.waiting ? phrase('正在缓冲…') : state.current.artist }}</small></span></button>
    <button :aria-label="state.playing || state.waiting ? phrase('暂停') : phrase('播放')" @click="player.toggle"><Pause v-if="state.playing || state.waiting" :size="20" /><Play v-else :size="20" /></button>
    <button :aria-label="phrase('下一首')" @click="player.next()"><SkipForward :size="20" /></button>
    <button :aria-label="phrase('停止播放并关闭')" @click="player.stop"><X :size="18" /></button>
    <div class="music-mini__progress" aria-hidden="true"><span :style="{ width: progress }" /></div>
  </aside>
</template>
<style scoped>
.music-mini { position:fixed; right:20px; bottom:72px; width:min(440px,calc(100vw - 32px)); display:flex; align-items:center; gap:6px; padding:12px; border:1px solid var(--border-strong); border-radius:var(--radius-lg); background:var(--surface-raised); color:var(--text); box-shadow:var(--shadow-md); z-index:1100; }
.music-mini button { font:inherit; font-size:14px; color:inherit; background:none; border:0; min-width:36px; min-height:40px; border-radius:var(--radius-md); cursor:pointer; }
.music-mini button:hover { background:var(--surface); } .music-mini button:focus-visible { outline:3px solid var(--brand); outline-offset:2px; }
.music-mini .music-mini__song { display:flex; gap:10px; align-items:center; text-align:start; min-width:0; flex:1; padding:0; }
.music-mini__song img { width:40px; height:40px; object-fit:cover; border-radius:var(--radius-sm); } .music-mini__song > svg { flex-shrink:0; }
.music-mini__song span { min-width:0; overflow-wrap:anywhere; } .music-mini__song strong { display:block; font-size:14px; } .music-mini__song small { display:block; color:var(--muted); font-size:13px; margin-top:4px; }
.music-mini__progress { position:absolute; left:12px; right:12px; bottom:4px; height:2px; background:var(--border); overflow:hidden; } .music-mini__progress span { display:block; height:100%; background:var(--brand); }
@media (max-width:480px) { .music-mini { right:16px; bottom:80px; } }
</style>

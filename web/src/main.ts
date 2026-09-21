import { createApp, nextTick } from 'vue'
import App from './App.vue'
import { router } from './router'
import { initializeI18n } from './i18n'
import { initializeTheme } from './stores/theme'
import { initializeDesktopMode } from './stores/desktopMode'
import './styles/main.css'

initializeTheme()
initializeDesktopMode()

async function bootstrap(): Promise<void> {
  await initializeI18n()
  createApp(App).use(router).mount('#app')
  await router.isReady()
  await nextTick()
  document.documentElement.classList.remove('desktop-boot')
  document.getElementById('desktop-boot')?.remove()
}

void bootstrap()

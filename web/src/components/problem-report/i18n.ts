import { ref, watch } from 'vue'
import { useI18n } from '@/i18n'
import zhCN from '@/i18n/pages/ProblemReport/zh-CN'

export function useProblemReportText() {
  const i18n = useI18n()
  const messages = ref(zhCN)
  let sequence = 0
  watch(i18n.locale, async (locale) => {
    const current = ++sequence
    messages.value = zhCN
    if (locale === 'zh-CN') return
    try {
      const module = locale === 'en-US' ? await import('@/i18n/pages/ProblemReport/en-US')
        : await import('@/i18n/pages/ProblemReport/zh-TW')
      if (sequence === current) messages.value = module.default
    } catch { /* Keep the usable Chinese fallback without loading any report data. */ }
  }, { immediate: true })
  return (key: keyof typeof zhCN) => messages.value[key]
}

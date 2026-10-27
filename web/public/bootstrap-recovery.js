// Independent of the module graph: a failed entry/chunk must not leave a blank page.
(() => {
  let finished = false
  let card
  let timer
  const cleanup = () => {
    clearTimeout(timer)
    window.removeEventListener('error', resourceError, true)
    window.removeEventListener('vite:preloadError', show)
    window.removeEventListener('kpanel:boot-failed', show)
    window.removeEventListener('kpanel:ready', ready)
  }
  const ready = () => {
    finished = true
    cleanup()
    card?.remove()
  }
  const show = () => {
    if (finished || card || !document.body) return
    let locale = 'zh-CN'
    try { locale = localStorage.getItem('kejilion-panel-locale') || locale } catch { /* Storage may be disabled. */ }
    const messages = locale === 'en-US' ? [
      'The page could not finish loading', 'Check your connection or wait for the service to finish restarting, then try again.', 'Reload page',
    ] : locale === 'zh-TW' ? [
      '頁面未能完成載入', '請檢查網路或等待服務重新啟動完成，然後重試。', '重新載入',
    ] : [
      '页面未能完成加载', '请检查网络或等待服务重启完成，然后重试。', '重新加载',
    ]
    card = document.createElement('section')
    card.id = 'bootstrap-recovery'
    card.setAttribute('role', 'alert')
    card.style.cssText = 'position:fixed;inset:0;z-index:2147483647;overflow:auto;display:flex;flex-direction:column;align-items:center;justify-content:center;box-sizing:border-box;padding:24px;background:Canvas;color:CanvasText;font:14px/1.6 system-ui;text-align:center'
    const title = document.createElement('h1')
    title.style.cssText = 'font-size:20px;margin:0 0 12px'
    title.textContent = messages[0]
    const detail = document.createElement('p')
    detail.style.cssText = 'max-width:34em;margin:0 0 24px'
    detail.textContent = messages[1]
    const retry = document.createElement('button')
    retry.type = 'button'
    retry.style.cssText = 'font:inherit;min-height:44px;padding:8px 20px;cursor:pointer'
    retry.textContent = messages[2]
    // Only an explicit click reloads: never replay a mutation or create reload loops.
    retry.addEventListener('click', () => location.reload())
    card.append(title, detail, retry)
    document.body.append(card)
  }
  const resourceError = event => {
    if (event.target instanceof HTMLScriptElement && event.target.type === 'module') show()
  }
  window.addEventListener('error', resourceError, true)
  window.addEventListener('vite:preloadError', show)
  window.addEventListener('kpanel:boot-failed', show)
  window.addEventListener('kpanel:ready', ready)
  timer = setTimeout(show, 12000)
})()

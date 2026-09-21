// Run before the application modules, under the existing script-src 'self' policy.
(() => {
  const root = document.documentElement
  const read = key => {
    try { return localStorage.getItem(key) } catch { return null }
  }
  const preference = read('kejilion-panel-theme')
  const systemTheme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  const theme = preference === 'light' || preference === 'dark'
    ? preference : systemTheme
  root.dataset.theme = theme
  root.style.colorScheme = theme
  const stored = read('kpanel:desktop-wallpaper:v1')
  const wallpaper = ['orbit', 'horizon', 'rift', 'prism'].includes(stored) ? `-${stored}` : ''
  root.style.setProperty('--desktop-wallpaper-image', `url("/wallpapers/kpanel-desktop${wallpaper}.webp")`)
  if (read('kejilion-panel-desktop-mode') === 'desktop' && !/^\/(login|setup|share)(\/|$)/.test(location.pathname)) {
    root.classList.add('desktop-boot')
  }
})()

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
  const readSession = key => {
    try { return sessionStorage.getItem(key) } catch { return null }
  }
  // Restore only locally generated gradient/color tokens, never CSS URLs from storage.
  try {
    const raw = readSession('kpanel:desktop-backdrop:v1')
    const cached = raw && raw.length < 12000 ? JSON.parse(raw) : null
    if (cached?.theme === theme && cached.colors === read('kejilion-panel-colors')) {
      for (const key of ['wallpaper-base', 'wallpaper-veil-light', 'wallpaper-veil-dark', 'wallpaper-vignette', 'aurora-one', 'aurora-two', 'aurora-opacity']) {
        const token = `--desktop-${key}`
        const value = cached.tokens?.[token]
        if (typeof value === 'string' && value.length < 1500 && /^[a-zA-Z0-9#.,()% /+-]+$/.test(value) && !/url/i.test(value)) root.style.setProperty(token, value)
      }
    }
  } catch { /* Invalid or unavailable cache leaves the shared theme defaults. */ }
  // Mirrors dominantChronoPhase() in web/src/lib/desktopScenes/chrono.ts.
  const chronoPhase = () => {
    const now = new Date()
    const minute = now.getHours() * 60 + now.getMinutes() + now.getSeconds() / 60
    if (minute < 315 || minute > 1125) return 'night'
    return minute <= 405 || minute >= 1035 ? 'golden' : 'day'
  }
  const selectedWallpaper = () => {
    const stored = read('kpanel:desktop-wallpaper:v1')
    // Live scenes paint their poster first; DesktopScene animates it after mount.
    if (['chrono', 'tide', 'sakura', 'neon', 'aurora'].includes(stored)) {
      const key = stored === 'chrono' ? `chrono-${chronoPhase()}` : stored
      return { id: stored, key, scene: true, url: `/wallpapers/scenes/${key}.webp` }
    }
    const id = ['orbit', 'horizon', 'rift', 'prism'].includes(stored) ? stored : 'classic'
    return { id, key: id, url: `/wallpapers/kpanel-desktop${id === 'classic' ? '' : `-${id}`}.webp` }
  }
  const cacheKey = key => `kpanel:desktop-wallpaper-cache:v1:${key}`
  const cachedImage = id => {
    const value = readSession(cacheKey(id))
    return value && value.length <= 131072 && /^data:image\/webp;base64,[A-Za-z0-9+/]+=*$/.test(value) ? value : null
  }
  const cacheWallpaper = async ({ key, scene, url }) => {
    // Scene posters exceed the per-tab bitmap budget; the HTTP cache serves them.
    if (scene || cachedImage(key)) return
    try {
      const response = await fetch(url)
      if (!response.ok) return
      const blob = await response.blob()
      if (blob.type !== 'image/webp' || blob.size > 98304) return
      const reader = new FileReader()
      reader.onload = () => {
        try { sessionStorage.setItem(cacheKey(key), reader.result) } catch { /* Cache is optional. */ }
      }
      reader.readAsDataURL(blob)
    } catch { /* Network failures never block the desktop. */ }
  }
  const wallpaper = selectedWallpaper()
  root.dataset.desktopWallpaper = wallpaper.id
  const source = cachedImage(wallpaper.key) || wallpaper.url
  root.style.setProperty('--desktop-wallpaper-image', `url("${source}")`)
  root.classList.add('desktop-wallpaper-loading')
  const image = new Image()
  image.fetchPriority = 'high'
  image.src = source
  image.decode().catch(async () => {
    if (source !== wallpaper.url) {
      try { sessionStorage.removeItem(cacheKey(wallpaper.key)) } catch { /* Optional cache. */ }
      root.style.setProperty('--desktop-wallpaper-image', `url("${wallpaper.url}")`)
      image.src = wallpaper.url
      await image.decode()
      void cacheWallpaper(wallpaper)
    } else throw new Error('wallpaper_unavailable')
  }).catch(() => { root.dataset.desktopWallpaperFailed = 'true' }).finally(() => {
    root.classList.remove('desktop-wallpaper-loading')
  })
  void cacheWallpaper(wallpaper)
  window.addEventListener('kpanel:cache-desktop-wallpaper', () => { void cacheWallpaper(selectedWallpaper()) })
  if (read('kejilion-panel-desktop-mode') === 'desktop' && !/^\/(login|setup|share)(\/|$)/.test(location.pathname)) {
    root.classList.add('desktop-boot')
  }
})()

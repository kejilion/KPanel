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
  const classicURL = '/wallpapers/kpanel-desktop.webp'
  const selectedWallpaper = () => {
    const stored = read('kpanel:desktop-wallpaper:v1')
    // An installed 3D scene pack: its poster is the still frame for reduced motion.
    const pack = typeof stored === 'string' ? /^pack:([a-z0-9][a-z0-9-]{0,39})$/.exec(stored) : null
    if (pack) return { id: stored, pack: true, url: `/api/v1/desktop/scene-packs/${pack[1]}/poster` }
    const id = ['orbit', 'horizon', 'rift', 'prism'].includes(stored) ? stored : 'classic'
    return { id, url: `/wallpapers/kpanel-desktop${id === 'classic' ? '' : `-${id}`}.webp` }
  }
  const cacheKey = id => `kpanel:desktop-wallpaper-cache:v1:${id}`
  const cachedImage = id => {
    const value = readSession(cacheKey(id))
    return value && value.length <= 131072 && /^data:image\/webp;base64,[A-Za-z0-9+/]+=*$/.test(value) ? value : null
  }
  const cacheWallpaper = async ({ id, url }) => {
    if (cachedImage(id)) return
    try {
      const response = await fetch(url)
      if (!response.ok) return
      const blob = await response.blob()
      if (blob.type !== 'image/webp' || blob.size > 98304) return
      const reader = new FileReader()
      reader.onload = () => {
        try { sessionStorage.setItem(cacheKey(id), reader.result) } catch { /* Cache is optional. */ }
      }
      reader.readAsDataURL(blob)
    } catch { /* Network failures never block the desktop. */ }
  }
  const wallpaper = selectedWallpaper()
  root.dataset.desktopWallpaper = wallpaper.id
  // Classic mode shows the same picture; a scene pack runs live there too (AppShell), the poster is its fallback.
  const classicLevel = read('kpanel:classic-wallpaper:v1')
  if (classicLevel === 'ambient' || classicLevel === 'clear') root.dataset.classicWallpaper = classicLevel
  const setClassicImage = ({ id, url }) => {
    root.style.setProperty('--classic-wallpaper-image', `url("${cachedImage(id) || url}")`)
  }
  // A live scene boots to black (desktopWallpaper.css) so its own entrance is the first thing seen.
  const liveScene = wallpaper.pack
    && !(matchMedia('(prefers-reduced-motion: reduce)').matches && read('kpanel:desktop-scene-motion:v1') !== 'always')
  if (liveScene) {
    root.dataset.desktopWallpaperScene = 'live'
  } else {
    const source = cachedImage(wallpaper.id) || wallpaper.url
    root.style.setProperty('--desktop-wallpaper-image', `url("${source}")`)
    root.classList.add('desktop-wallpaper-loading')
    const image = new Image()
    image.fetchPriority = 'high'
    image.src = source
    image.decode().catch(async () => {
      if (source !== wallpaper.url) {
        try { sessionStorage.removeItem(cacheKey(wallpaper.id)) } catch { /* Optional cache. */ }
        root.style.setProperty('--desktop-wallpaper-image', `url("${wallpaper.url}")`)
        image.src = wallpaper.url
        await image.decode()
        void cacheWallpaper(wallpaper)
      } else throw new Error('wallpaper_unavailable')
    }).catch(async (error) => {
      // A removed or unreachable pack falls back to the classic wallpaper instead of an empty desktop.
      if (!wallpaper.pack) throw error
      root.style.setProperty('--desktop-wallpaper-image', `url("${classicURL}")`)
      image.src = classicURL
      await image.decode()
    }).catch(() => { root.dataset.desktopWallpaperFailed = 'true' }).finally(() => {
      root.classList.remove('desktop-wallpaper-loading')
    })
    void cacheWallpaper(wallpaper)
  }
  setClassicImage(wallpaper)
  window.addEventListener('kpanel:cache-desktop-wallpaper', () => {
    const selected = selectedWallpaper()
    setClassicImage(selected)
    void cacheWallpaper(selected)
  })
  if (read('kejilion-panel-desktop-mode') === 'desktop' && !/^\/(login|setup|share)(\/|$)/.test(location.pathname)) {
    root.classList.add('desktop-boot')
  }
})()

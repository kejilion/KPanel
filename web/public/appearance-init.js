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
    // An uploaded picture, served by the panel to the signed-in browser.
    const custom = typeof stored === 'string' ? /^custom:([0-9a-f]{32})$/.exec(stored) : null
    if (custom) return { id: stored, custom: custom[1], url: `/api/v1/desktop/wallpapers/${custom[1]}/image` }
    const id = ['orbit', 'horizon', 'rift', 'prism'].includes(stored) ? stored : 'classic'
    return { id, url: `/wallpapers/kpanel-desktop${id === 'classic' ? '' : `-${id}`}.webp` }
  }
  const cacheKey = id => `kpanel:desktop-wallpaper-cache:v1:${id}`
  const cachedImage = id => {
    const value = readSession(cacheKey(id))
    return value && value.length <= 131072 && /^data:image\/webp;base64,[A-Za-z0-9+/]+=*$/.test(value) ? value : null
  }
  const cacheWallpaper = async ({ id, url, custom }) => {
    // Uploaded pictures are far larger than the session cache allows; the HTTP cache keeps them.
    if (custom || cachedImage(id)) return
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
  // Sign-in, first-run and share pages have no session: an uploaded picture or a scene poster
  // cannot load there and must not be shown to whoever opens them. Only the public built-in
  // wallpapers are used before sign-in (the sign-in brand panel); AppShell asks for the real
  // images again once signed in.
  const publicPage = /^\/(login|setup|share)(\/|$)/.test(location.pathname)
  const privateWallpaper = Boolean(wallpaper.pack || wallpaper.custom)
  // The desktop paints its own image when this does not name its wallpaper; a private one
  // held back here must not look already painted after an in-app sign-in.
  root.dataset.desktopWallpaper = publicPage && privateWallpaper ? 'classic' : wallpaper.id
  // A private wallpaper appears on the sign-in page only as this browser's own reduced copy
  // (web/src/lib/authWallpaperCopy.ts), never fetched from the server before sign-in.
  const authCopy = id => {
    try {
      const copy = JSON.parse(read('kpanel:auth-wallpaper:v1') || 'null')
      const inRange = value => Number.isInteger(value) && value >= 0 && value <= 1000
      if (copy?.id === id && typeof copy.image === 'string' && copy.image.length <= 400000
        && /^data:image\/(webp|jpeg);base64,[A-Za-z0-9+/]+=*$/.test(copy.image)
        && inRange(copy.focusX) && inRange(copy.focusY) && typeof copy.bright === 'boolean') return copy
    } catch { /* An unreadable copy leaves the default wallpaper. */ }
    return null
  }
  const setAuthImage = ({ id, pack, custom, url }) => {
    const copy = pack || custom ? authCopy(id) : null
    let image = url
    let name = id
    if (copy) {
      image = copy.image
      name = 'private'
    } else if (pack || custom) {
      image = classicURL
      name = 'classic'
    }
    root.dataset.authWallpaper = name
    root.style.setProperty('--auth-wallpaper-image', `url("${image}")`)
    root.style.setProperty('--auth-wallpaper-position', copy ? `${copy.focusX / 10}% ${copy.focusY / 10}%` : 'center')
    if (copy ? copy.bright : id === 'horizon') root.dataset.authWallpaperBright = 'true'
    else delete root.dataset.authWallpaperBright
  }
  setAuthImage(wallpaper)
  // An uploaded picture keeps its focal point in view and, when bright, asks for a thicker classic veil.
  if (wallpaper.custom && !publicPage) {
    try {
      const display = JSON.parse(read('kpanel:desktop-wallpaper-custom:v1') || 'null')
      const inRange = (value, max) => Number.isInteger(value) && value >= 0 && value <= max
      if (display?.id === wallpaper.custom && inRange(display.focusX, 1000) && inRange(display.focusY, 1000) && inRange(display.luminance, 100)) {
        root.style.setProperty('--desktop-wallpaper-position', `${display.focusX / 10}% ${display.focusY / 10}%`)
        if (display.luminance >= 50) root.dataset.wallpaperBright = 'true'
      }
    } catch { /* Framing is optional; the picture stays centred. */ }
  }
  // Classic mode shows the same picture; a scene pack runs live there too (AppShell), the poster is its fallback.
  const classicLevel = read('kpanel:classic-wallpaper:v1')
  if (classicLevel === 'ambient' || classicLevel === 'clear') root.dataset.classicWallpaper = classicLevel
  const setClassicImage = ({ id, url }) => {
    root.style.setProperty('--classic-wallpaper-image', `url("${cachedImage(id) || url}")`)
  }
  // A live scene boots to black (desktopWallpaper.css) so its own entrance is the first thing seen.
  const liveScene = wallpaper.pack && !publicPage
    && !(matchMedia('(prefers-reduced-motion: reduce)').matches && read('kpanel:desktop-scene-motion:v1') !== 'always')
  if (liveScene) {
    root.dataset.desktopWallpaperScene = 'live'
  } else if (!(publicPage && privateWallpaper)) {
    // Before sign-in a private wallpaper is not preloaded: the desktop and classic surfaces
    // only exist after sign-in.
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
      // A removed or unreachable pack or picture falls back to the classic wallpaper instead of an empty desktop.
      if (!wallpaper.pack && !wallpaper.custom) throw error
      root.style.setProperty('--desktop-wallpaper-image', `url("${classicURL}")`)
      image.src = classicURL
      await image.decode()
    }).catch(() => { root.dataset.desktopWallpaperFailed = 'true' }).finally(() => {
      root.classList.remove('desktop-wallpaper-loading')
    })
    void cacheWallpaper(wallpaper)
  }
  if (!(publicPage && privateWallpaper)) setClassicImage(wallpaper)
  window.addEventListener('kpanel:cache-desktop-wallpaper', () => {
    const selected = selectedWallpaper()
    setAuthImage(selected)
    setClassicImage(selected)
    void cacheWallpaper(selected)
  })
  if (read('kejilion-panel-desktop-mode') === 'desktop' && !publicPage) {
    root.classList.add('desktop-boot')
  }
})()

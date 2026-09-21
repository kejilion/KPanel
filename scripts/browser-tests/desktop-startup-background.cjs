const assert = require('node:assert/strict')
const fs = require('node:fs')
const { execFileSync } = require('node:child_process')
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
assert.equal(candidate, process.env.REPORT_CANDIDATE)
assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '')
const out = process.env.EVIDENCE_DIR
fs.mkdirSync(out, { recursive: true })
const report = { candidate, mode: 'mock-ui', cases: [], errors: [] }

;(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
  try {
    for (const [theme, wallpaper] of [['light', 'classic'], ['dark', 'prism']]) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme === 'dark' ? 'light' : 'dark' })
      await context.addInitScript(({ theme, wallpaper }) => {
        localStorage.setItem('kejilion-panel-theme', theme)
        localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
        localStorage.setItem('kejilion-panel-desktop-windows', '[]')
        localStorage.setItem('kpanel:desktop-wallpaper:v1', wallpaper)
      }, { theme, wallpaper })
      const page = await context.newPage()
      page.setDefaultTimeout(12000)
      page.on('pageerror', error => report.errors.push(error.message))
      let releaseMain, releaseDesktop
      const mainGate = new Promise(resolve => { releaseMain = resolve })
      const desktopGate = new Promise(resolve => { releaseDesktop = resolve })
      await page.route('**/src/main.ts', async route => { await mainGate; await route.continue() })
      await page.route('**/components/desktop/DesktopView.vue', async route => { await desktopGate; await route.continue() })
      await page.route(base + '/', async route => {
        const response = await route.fetch()
        await route.fulfill({ response, headers: { ...response.headers(), 'content-security-policy': "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'" } })
      })
      await page.goto(base, { waitUntil: 'commit' })
      const wallpaperPath = `/wallpapers/kpanel-desktop${wallpaper === 'classic' ? '' : `-${wallpaper}`}.webp`
      await page.waitForFunction(() => document.documentElement.classList.contains('desktop-boot'))
      const startup = await page.evaluate(() => ({ theme: document.documentElement.dataset.theme, background: getComputedStyle(document.documentElement).backgroundImage, mounted: !!document.querySelector('#app')?.children.length }))
      assert.equal(startup.theme, theme)
      assert(startup.background.includes(wallpaperPath))
      assert.equal(startup.mounted, false)
      await page.evaluate(async () => {
        const image = new Image()
        image.src = getComputedStyle(document.documentElement).getPropertyValue('--desktop-wallpaper-image').match(/url\("?([^"\)]+)"?\)/)[1]
        await image.decode()
      })
      await page.screenshot({ path: `${out}/${theme}-before-app.png` })
      releaseMain()
      await page.locator('.desktop[role="status"] .desktop__wallpaper-image').waitFor({ state: 'attached' })
      const placeholder = await page.locator('.desktop__wallpaper-image').evaluate(element => getComputedStyle(element).backgroundImage)
      assert(placeholder.includes(wallpaperPath))
      assert.equal(await page.locator('.sidebar').isVisible(), false)
      assert.equal(await page.locator('.app-shell__main').isVisible(), false)
      await page.screenshot({ path: `${out}/${theme}-lazy-desktop.png` })
      releaseDesktop()
      await page.locator('.desktop__icons:not(.desktop__icons--restoring)').waitFor()
      assert.equal(await page.evaluate(() => document.documentElement.classList.contains('desktop-boot')), false)
      assert((await page.locator('.desktop__wallpaper-image').evaluate(element => getComputedStyle(element).backgroundImage)).includes(wallpaperPath))
      report.cases.push({ theme, wallpaper, startup, placeholder, bootSurfaceRemoved: true, classicVisible: false })
      await context.close()
    }
    assert.deepEqual(report.errors, [])
  } catch (error) { report.failure = error.stack; throw error }
  finally { await browser.close(); fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify(report))
})().catch(error => { console.error(error); process.exitCode = 1 })

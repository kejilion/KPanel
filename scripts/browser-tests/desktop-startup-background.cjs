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
const deferred = () => {
  let release
  const promise = new Promise(resolve => { release = resolve })
  return { promise, release }
}

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
      let mainGate = deferred(), desktopGate = deferred(), imageGate = deferred(), imageRequests = 0
      await page.route('**/src/main.ts', async route => { await mainGate.promise; await route.continue() })
      await page.route('**/components/desktop/DesktopView.vue', async route => { await desktopGate.promise; await route.continue() })
      await page.route('**/wallpapers/*.webp', async route => { imageRequests++; await imageGate.promise; await route.continue() })
      await page.route(base + '/', async route => {
        const response = await route.fetch()
        await route.fulfill({ response, headers: { ...response.headers(), 'content-security-policy': "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'" } })
      })
      const checkHandoff = async phase => {
        await page.waitForFunction(() => document.documentElement.classList.contains('desktop-boot') && !document.documentElement.classList.contains('desktop-wallpaper-loading'))
        const before = await page.screenshot({ path: `${out}/${theme}-${phase}-before-app.png` })
        mainGate.release()
        await page.locator('.desktop[role="status"] .desktop__wallpaper-image').waitFor({ state: 'attached' })
        await page.waitForFunction(() => !document.getElementById('desktop-boot'))
        assert.equal(await page.locator('.sidebar').isVisible(), false)
        const placeholder = await page.screenshot({ path: `${out}/${theme}-${phase}-lazy-desktop.png` })
        assert(before.equals(placeholder), `${theme}/${phase}: wallpaper/veil changed at placeholder handoff`)
        desktopGate.release()
        await page.locator('.desktop__icons:not(.desktop__icons--restoring)').waitFor()
        const hideUI = await page.addStyleTag({ content: '.desktop > :not(.desktop__wallpaper) { visibility: hidden !important; }' })
        const mounted = await page.screenshot({ path: `${out}/${theme}-${phase}-mounted-backdrop.png` })
        assert(placeholder.equals(mounted), `${theme}/${phase}: wallpaper/veil changed at desktop mount`)
        await hideUI.evaluate(element => element.remove())
        report.cases.push({ theme, phase, matchingBackdropPixels: true })
      }
      await page.goto(base, { waitUntil: 'commit' })
      await page.waitForFunction(() => document.documentElement.classList.contains('desktop-wallpaper-loading'))
      assert.equal(await page.locator('#desktop-boot .desktop__wallpaper-veil').isVisible(), false)
      imageGate.release()
      await checkHandoff('cold')
      await page.waitForFunction(id => sessionStorage.getItem(`kpanel:desktop-wallpaper-cache:v1:${id}`)?.startsWith('data:image/webp;'), wallpaper)
      if (theme === 'dark') await page.evaluate(async () => {
        const { useTheme } = await import('/src/stores/theme.ts')
        useTheme().setColors({ brand: '#3bc4a0', neutral: '#000000', signatureLinked: true, signature: '#3bc4a0' })
      })
      mainGate = deferred(); desktopGate = deferred(); imageGate = deferred(); imageRequests = 0
      await page.reload({ waitUntil: 'commit' })
      await checkHandoff('cached')
      assert.equal(imageRequests, 0, 'refresh must not wait for a wallpaper network request')
      imageGate.release()
      await context.close()
    }
    assert.deepEqual(report.errors, [])
  } catch (error) { report.failure = error.stack; throw error }
  finally { await browser.close(); fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify(report))
})().catch(error => { console.error(error); process.exitCode = 1 })

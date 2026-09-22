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
const deferred = () => { let release; const promise = new Promise(resolve => { release = resolve }); return { promise, release } }

;(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
  const deadline = setTimeout(() => { void browser.close() }, 50000)
  try {
    for (const scenario of ['light', 'dark', 'failed-inventory']) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
      await context.addInitScript(theme => {
        localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
        localStorage.setItem('kejilion-panel-theme', theme === 'light' ? 'light' : 'dark')
        localStorage.setItem('kejilion-panel-desktop-windows', '[]')
      }, scenario)
      const page = await context.newPage()
      page.setDefaultTimeout(10000)
      page.on('pageerror', error => report.errors.push(error.message))
      const dataGate = deferred(), imageGate = deferred(), requested = new Set()
      let writes = 0
      await page.route('**/api/v1/desktop/workspace', route => {
        if (route.request().method() !== 'GET') writes++
        return route.fulfill({ json: { schemaVersion: 5, available: true, resourceVersion: 'sha256:' + '1'.repeat(64), groups: [{ id: 'a'.repeat(32), name: 'Saved group', columns: 4, rows: 0, collapsed: false, members: ['nav:/overview', 'app:builtin-5'] }], positions: { ['group:' + 'a'.repeat(32)]: { x: 0.3, y: 0.1 }, 'app:builtin-28': { x: 0, y: 0.4 } }, widgetPositions: {}, hiddenEntryKeys: [], hiddenWidgetKeys: [], labels: {}, shortcuts: [] } })
      })
      for (const endpoint of ['apps', 'sites', 'system/public-network']) await page.route(`**/api/v1/${endpoint}`, async route => {
        requested.add(endpoint)
        await dataGate.promise
        if (scenario === 'failed-inventory' && endpoint !== 'system/public-network') return route.fulfill({ status: 500, json: { title: 'Injected failure' } })
        const response = await route.fetch()
        if (endpoint !== 'apps') return route.fulfill({ response })
        const data = await response.json()
        for (const item of data.items) if (item.id === 'builtin-5') item.icon = '/slow-desktop-icon.webp'
        await route.fulfill({ response, json: data })
      })
      await page.route('**/slow-desktop-icon.webp', async route => {
        await imageGate.promise
        await route.fulfill({ path: 'web/public/desktop-icons/overview-kpanel-flat-v1.webp', contentType: 'image/webp' })
      })
      const snapshot = () => page.evaluate(() => Object.fromEntries([...document.querySelectorAll('.desktop__icon-slot[data-icon-key^="nav:"], .desktop-widget-slot, .desktop-group')].map(element => {
        const rect = element.getBoundingClientRect()
        return [element.dataset.iconKey || element.dataset.groupId || element.getAttribute('aria-label'), [rect.x, rect.y, rect.width, rect.height].map(value => Math.round(value * 100) / 100)]
      })))
      await page.goto(base, { waitUntil: 'domcontentloaded' })
      await page.locator('.desktop__icons:not(.desktop__icons--restoring)').waitFor()
      await page.waitForFunction(() => getComputedStyle(document.querySelector('.desktop__icons')).opacity === '1')
      assert(await page.locator('[data-icon-key="nav:/overview"]').isVisible(), 'navigation must not wait for inventory')
      assert(await page.locator('.desktop-group').isVisible(), 'group frame must not wait for inventory')
      assert(await page.locator('.desktop-widget-slot').first().isVisible(), 'widgets must not wait for inventory')
      assert.deepEqual([...requested].sort(), ['apps', 'sites', 'system/public-network'], 'data requests must start concurrently')
      const before = await snapshot()
      await page.screenshot({ path: `${out}/${scenario}-pending-data.png` })
      dataGate.release()
      await page.waitForFunction(() => document.querySelector('.desktop__icons').getAttribute('aria-busy') === 'false')
      assert.deepEqual(await snapshot(), before, 'late data must not move visible navigation, widgets or groups')
      if (scenario !== 'failed-inventory') {
        const icon = page.locator('[data-icon-key="app:builtin-5"]')
        await icon.waitFor()
        assert.equal(await icon.getAttribute('data-group-member'), 'a'.repeat(32))
        assert(await icon.locator('.desktop__icon-monogram').isVisible(), 'slow image needs an immediate fallback')
        assert.equal(await icon.locator('img').evaluate(element => getComputedStyle(element).opacity), '0')
        const rect = await icon.boundingBox()
        await icon.getByRole('button').click()
        assert(await icon.getByRole('button').evaluate(element => element.classList.contains('desktop__icon--selected')))
        await page.mouse.move(0, 0)
        imageGate.release()
        await icon.locator('img:not(.desktop__icon-img--loading)').waitFor()
        assert.equal(await icon.locator('.desktop__icon-monogram').count(), 0)
        assert.deepEqual(await icon.boundingBox(), rect, 'image replacement must not change the slot')
        assert.deepEqual(await snapshot(), before)
      } else imageGate.release()
      assert.equal(writes, 0, 'progressive loading must not save runtime anchors')
      report.cases.push({ scenario, independentSurfaces: true, stablePositions: true, noWorkspaceWrites: true })
      await page.screenshot({ path: `${out}/${scenario}-settled.png` })
      await context.close()
    }
    assert.deepEqual(report.errors, [])
  } catch (error) { report.failure = error.stack; throw error }
  finally { clearTimeout(deadline); await browser.close(); fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify(report))
})().catch(error => { console.error(error); process.exitCode = 1 })

const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const { execFileSync } = require('node:child_process')
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
const draft = process.env.PREVIEW_GRADE === 'draft'
if (!draft) {
  assert.equal(candidate, process.env.REPORT_CANDIDATE)
  assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '')
}
const out = process.env.EVIDENCE_DIR
fs.mkdirSync(out, { recursive: true })
const report = { candidate, grade: draft ? 'draft' : 'acceptance', mode: 'mock-ui', cases: [], errors: [], cleanup: false }
;(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
  report.browser = browser.version()
  let resourceFailure = false
  let page, context
  const watchdog = setInterval(() => {
    const disk = fs.statfsSync(process.cwd())
    if (os.freemem() < 512 * 1024 ** 2 || disk.bavail * disk.bsize < 1024 ** 3) { resourceFailure = true; void browser.close() }
  }, 1000)
  try {
    context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
    await context.addInitScript(() => {
      localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
      localStorage.setItem('kejilion-panel-desktop-windows', '[]')
    })
    page = await context.newPage()
    page.setDefaultTimeout(10000)
    page.on('pageerror', error => report.errors.push(error.message))
    await context.tracing.start({ screenshots: true, snapshots: true })
    let state = { schemaVersion: 5, available: true, resourceVersion: 'sha256:' + '1'.repeat(64), groups: [], positions: {}, widgetPositions: {}, hiddenEntryKeys: [], hiddenWidgetKeys: ['widget:clock', 'widget:monitor', 'widget:services'], labels: {}, shortcuts: [] }
    let writes = 0, failNext = false
    await page.route('**/api/v1/desktop/workspace', async route => {
      if (route.request().method() === 'PUT') {
        if (failNext) { failNext = false; return route.fulfill({ status: 500, json: { title: 'Injected failure', code: 'desktop_workspace_unavailable' } }) }
        state = { ...state, ...route.request().postDataJSON(), resourceVersion: 'sha256:' + (++writes + 1).toString(16).padStart(64, '0') }
      }
      await route.fulfill({ json: state })
    })
    const settle = () => page.evaluate(async () => {
      await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame)
      await Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {})))
    })
    const group = page.locator('.desktop-group').first()
    const hint = page.locator('.desktop-group__drop-label')
    await page.goto(base + '/overview'); await page.locator('[data-icon-key]').first().waitFor(); await settle()
    const keys = await page.locator('[data-icon-key]').evaluateAll(icons => icons.map(icon => icon.dataset.iconKey))
    const source = page.locator(`[data-icon-key="${keys[12]}"]`)
    const begin = async (expectHint = true) => {
      await page.locator('.desktop__icons').evaluate(el => { el.scrollTop = 0 }); await settle()
      const from = await source.boundingBox(), target = await group.locator('[data-group-cell="0"]').boundingBox()
      assert(from && target)
      report.lastDrag = { from, target, source: keys[12] }
      await page.mouse.move(from.x + from.width / 2, from.y + 24); await page.mouse.down()
      await page.mouse.move(target.x + target.width / 2, target.y + 24, { steps: 12 })
      if (expectHint) await hint.waitFor()
      await settle()
    }
    const verifyHint = async expectedSide => {
      const card = await group.boundingBox(), label = await hint.boundingBox()
      assert(card && label)
      const side = label.y >= card.y + card.height ? 'below' : 'above'
      assert.equal(side, expectedSide)
      assert(Math.abs(side === 'below' ? label.y - card.y - card.height - 8 : card.y - label.y - label.height - 8) < 1)
      const viewport = page.viewportSize()
      assert(label.x >= 7 && label.x + label.width <= viewport.width - 7)
      assert(label.y >= 7 && label.y + label.height <= viewport.height - 7)
      assert.equal(await hint.evaluate(el => getComputedStyle(el).pointerEvents), 'none')
      assert.equal(await hint.evaluate(el => getComputedStyle(el).fontSize), '14px')
      assert.equal(await hint.evaluate(el => el.parentElement === document.body), true)
      return { card, label, side }
    }
    const cdp = await context.newCDPSession(page)
    for (const [width, height, theme, scale, locale] of [[1280, 900, 'dark', 1, 'zh-CN'], [1280, 900, 'light', 1.25, 'en-US'], [768, 800, 'light', 1, 'zh-CN'], [960, 540, 'dark', 2, 'en-US'], [390, 844, 'dark', 1, 'zh-CN']]) {
      await page.setViewportSize({ width, height })
      await cdp.send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: scale, mobile: false })
      const area = await page.locator('.desktop__icons').boundingBox()
      state.groups = [{ id: 'a'.repeat(32), name: locale === 'en-US' ? 'Operations and applications · long name' : '常用运维与应用服务分组', columns: 4, rows: 0, collapsed: false, members: keys.slice(0, 12) }]
      state.positions = { [`group:${state.groups[0].id}`]: { x: 0, y: 100 / Math.max(100, area.height - 96) }, [keys[12]]: { x: 0, y: 0 } }
      await page.evaluate(locale => localStorage.setItem('kejilion-panel-locale', locale), locale)
      await page.reload(); await group.waitFor(); await settle()
      await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
      const before = await group.boundingBox(), beforeWrites = writes
      if (width <= 760) {
        await begin(false); await page.keyboard.press('Escape'); await page.mouse.up(); await settle()
        assert.equal(await hint.count(), 0); assert.equal(writes, beforeWrites)
        report.cases.push({ width, compactMode: 'existing drag-disabled contract preserved; no hint or write' })
        continue
      }
      await begin()
      const geometry = await verifyHint('below')
      assert.deepEqual(geometry.card, before)
      assert.equal(writes, beforeWrites)
      await page.screenshot({ path: `${out}/${width}-${theme}-outside.png` })
      if (width === 1280 && theme === 'dark') {
        await page.mouse.move(geometry.label.x + geometry.label.width / 2, geometry.label.y + geometry.label.height / 2, { steps: 6 })
        await hint.waitFor({ state: 'detached' })
        const cell = await group.locator('[data-group-cell="0"]').boundingBox()
        await page.mouse.move(cell.x + cell.width / 2, cell.y + 24, { steps: 6 })
        await hint.waitFor(); await verifyHint('below')
        report.cases.push('moving onto the outside hint leaves the drop target; re-entering restores it')
      }
      await page.keyboard.press('Escape'); await page.mouse.up(); await settle()
      assert.equal(await hint.count(), 0); assert.equal(writes, beforeWrites)
      report.cases.push({ width, height, theme, scale, locale, geometry, cancel: true, scaleMethod: 'CSS viewport plus device metrics emulation' })
    }
    await cdp.send('Emulation.clearDeviceMetricsOverride')
    await page.setViewportSize({ width: 1280, height: 340 }); await settle()
    // Keep the first row reachable while the card bottom has no room for a label.
    state.groups[0].members = keys.slice(0, 6)
    const shortArea = await page.locator('.desktop__icons').boundingBox()
    state.positions = { [`group:${state.groups[0].id}`]: { x: 0, y: 100 / Math.max(100, shortArea.height - 96) }, [keys[12]]: { x: .6, y: 0 } }
    await page.reload(); await group.waitFor(); await settle(); await begin()
    report.cases.push({ bottomFallback: await verifyHint('above') })
    await page.screenshot({ path: `${out}/bottom-flip.png` })
    await page.locator('.desktop__icons').evaluate(el => { el.scrollTop += 8 })
    await settle()
    report.cases.push({ scrollReposition: await verifyHint('above') })
    await page.keyboard.press('Escape'); await page.mouse.up(); await settle()
    state.groups[0].members = keys.slice(0, 12)
    state.positions = { [`group:${state.groups[0].id}`]: { x: 0, y: .14 }, [keys[12]]: { x: .6, y: 0 } }
    await page.setViewportSize({ width: 1280, height: 900 }); await page.reload(); await group.waitFor(); await settle()
    await begin()
    const failed = page.waitForResponse(r => r.url().endsWith('/api/v1/desktop/workspace') && r.status() === 500)
    failNext = true; await page.mouse.up(); await failed; await settle()
    assert.equal(state.groups[0].members.length, 12); assert.equal(await hint.count(), 0)
    await page.locator('.desktop-group-undo button').last().click()
    await begin()
    const saved = page.waitForResponse(r => r.url().endsWith('/api/v1/desktop/workspace') && r.request().method() === 'PUT')
    await page.mouse.up(); assert.equal((await saved).status(), 200); await settle()
    assert(state.groups[0].members.includes(keys[12])); assert.equal(await hint.count(), 0)
    report.cases.push('failed drop removes hint and preserves members; retry succeeds without intercepting the pointer')
    assert.deepEqual(report.errors, []); assert.equal(resourceFailure, false)
    await context.tracing.stop({ path: `${out}/trace.zip` })
    await context.close()
  } catch (error) {
    report.failure = error.stack
    await page?.screenshot({ path: `${out}/failure.png` }).catch(() => {})
    await context?.tracing.stop({ path: `${out}/failure-trace.zip` }).catch(() => {})
    throw error
  }
  finally { clearInterval(watchdog); await browser.close(); report.cleanup = true; fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify({ candidate, cases: report.cases.length, errors: report.errors, cleanup: report.cleanup }))
})().catch(error => { console.error(error); process.exitCode = 1 })

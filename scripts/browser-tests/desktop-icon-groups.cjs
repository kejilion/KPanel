const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const { execFileSync } = require('node:child_process')
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const out = process.env.EVIDENCE_DIR
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
const draft = process.env.PREVIEW_GRADE === 'draft'
if (!draft) {
  assert.equal(candidate, process.env.REPORT_CANDIDATE)
  assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '')
}
fs.mkdirSync(out, { recursive: true })
const report = { candidate, grade: draft ? 'draft' : 'acceptance', mode: 'mock-ui', cases: [], errors: [], cleanup: false }
;(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
  report.browser = browser.version()
  let resourceFailure = false
  const watchdog = setInterval(() => {
    const disk = fs.statfsSync(process.cwd())
    if (os.freemem() < 512 * 1024 ** 2 || disk.bavail * disk.bsize < 1024 ** 3) { resourceFailure = true; void browser.close() }
  }, 1000)
  try {
    const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } })
    await context.addInitScript(() => {
      localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
      localStorage.setItem('kejilion-panel-desktop-windows', '[]')
      if (!localStorage.getItem('kejilion-panel-locale')) localStorage.setItem('kejilion-panel-locale', 'zh-CN')
      if (!localStorage.getItem('kejilion-panel-theme')) localStorage.setItem('kejilion-panel-theme', 'dark')
    })
    const page = await context.newPage()
    page.setDefaultTimeout(12000)
    page.on('pageerror', error => report.errors.push(error.message))
    await context.tracing.start({ screenshots: true, snapshots: true })
    // Isolated per-context mock workspace: never overwrite the user's preview state.
    let state = { schemaVersion: 4, available: true, resourceVersion: 'sha256:' + '1'.repeat(64), groups: [], positions: {}, widgetPositions: {}, hiddenEntryKeys: [], hiddenWidgetKeys: [], labels: {}, shortcuts: [] }
    let writes = 0, failNext = false
    await page.route('**/api/v1/desktop/workspace', async route => {
      if (route.request().method() === 'PUT') {
        if (failNext) { failNext = false; return route.fulfill({ status: 500, json: { title: 'Injected save failure', code: 'desktop_workspace_unavailable' } }) }
        const input = route.request().postDataJSON()
        assert.equal(input.expectedResourceVersion, state.resourceVersion)
        state = { ...state, ...input, resourceVersion: 'sha256:' + (++writes + 1).toString(16).padStart(64, '0') }
      }
      await route.fulfill({ json: state })
    })
    const slot = key => page.locator(`[data-icon-key="${key}"]`)
    const group = page.locator('.desktop-group').first()
    const settle = async () => page.evaluate(async () => {
      await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame)
      await Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {})))
      await new Promise(requestAnimationFrame)
    })
    const save = async action => {
      const response = page.waitForResponse(r => r.url().endsWith('/api/v1/desktop/workspace') && r.request().method() === 'PUT')
      await action(); assert.equal((await response).status(), 200); await settle()
    }
    const drag = async (locator, x, y) => {
      const box = await locator.boundingBox(); assert(box)
      await page.mouse.move(box.x + box.width / 2, box.y + Math.min(24, box.height / 2))
      await page.mouse.down(); await page.mouse.move(x, y, { steps: 16 }); await page.mouse.up()
    }
    await page.goto(base + '/overview')
    await slot('nav:/overview').waitFor()
    await settle()
    assert.equal(await group.count(), 0)
    assert.equal(writes, 0)
    const looseBefore = await slot('nav:/files').boundingBox()
    await slot('nav:/overview').locator('button').click()
    await slot('nav:/terminal').locator('button').click({ modifiers: ['Control'] })
    await page.locator('.desktop__selection-actions').getByText('编为一组').click()
    await page.locator('.desktop-group-form input').fill('常用运维')
    await save(() => page.getByRole('button', { name: '保存分组', exact: true }).click())
    assert.deepEqual(state.groups[0].members, ['nav:/overview', 'nav:/terminal'])
    assert.equal(await slot('nav:/files').getAttribute('data-group-member'), null)
    const looseAfter = await slot('nav:/files').boundingBox()
    assert.equal(looseAfter.x, looseBefore.x); assert.equal(looseAfter.y, looseBefore.y)
    report.cases.push('default ungrouped; selected-only create; loose positions preserved')
    let box = await group.boundingBox()
    await save(() => drag(slot('nav:/files'), box.x + box.width - 24, box.y + 90))
    assert(state.groups[0].members.includes('nav:/files'))
    assert.equal(await slot('nav:/files').getAttribute('data-group-member'), state.groups[0].id)
    const first = await slot('nav:/overview').boundingBox()
    await save(() => drag(slot('nav:/files'), first.x + 15, first.y + 24))
    assert.equal(state.groups[0].members[0], 'nav:/files')
    await save(() => drag(slot('nav:/files'), 960, 460))
    assert(!state.groups[0].members.includes('nav:/files'))
    report.cases.push('pointer drag in, reorder and out')
    const memberBefore = await slot('nav:/overview').boundingBox()
    await save(() => drag(group.locator('.desktop-group__header strong'), 680, 140))
    const moved = await slot('nav:/overview').boundingBox()
    assert(Math.abs(moved.x - memberBefore.x) > 100)
    report.cases.push('header moves group with members')
    await slot('nav:/overview').locator('button').focus()
    await save(() => page.keyboard.press('Control+ArrowRight'))
    assert.equal(state.groups[0].members[1], 'nav:/overview')
    await group.locator('.desktop-group__menu').focus()
    await page.keyboard.press('Enter')
    await page.locator('.desktop-group-form input').waitFor()
    await page.keyboard.press('Escape')
    await settle()
    assert.equal(await group.locator('.desktop-group__menu').evaluate(el => document.activeElement === el), true)
    report.cases.push('keyboard reorder and modal focus return')
    await save(() => group.locator('.desktop-group__toggle').click())
    assert.equal(await slot('nav:/overview').getAttribute('aria-hidden'), 'true')
    assert.equal(await slot('nav:/overview').evaluate(el => el.inert), true)
    await page.reload(); await group.waitFor(); assert.equal(await group.locator('.desktop-group__toggle').getAttribute('aria-expanded'), 'false')
    await save(() => group.locator('.desktop-group__toggle').click())
    failNext = true
    const failed = page.waitForResponse(r => r.url().endsWith('/api/v1/desktop/workspace') && r.status() === 500)
    await group.locator('.desktop-group__toggle').click(); await failed; await settle()
    assert.equal(await group.locator('.desktop-group__toggle').getAttribute('aria-expanded'), 'true')
    const closeToast = page.getByRole('button', { name: '关闭通知', exact: true }).first()
    if (await closeToast.count()) await closeToast.click()
    report.cases.push('collapse inert; refresh persists; failed save restores')
    const cdp = await context.newCDPSession(page)
    for (const [width, height, theme, reduced, scale, locale] of [[1920, 1080, 'dark', false, 1, 'zh-CN'], [1280, 800, 'light', true, 1.25, 'en-US'], [960, 540, 'dark', false, 2, 'en-US'], [761, 700, 'dark', false, 1, 'zh-CN'], [760, 700, 'light', false, 1, 'zh-CN'], [390, 844, 'dark', true, 1, 'zh-CN'], [320, 568, 'light', true, 1, 'zh-CN']]) {
      const previousWrites = writes
      await page.evaluate(locale => localStorage.setItem('kejilion-panel-locale', locale), locale)
      await page.reload(); await group.waitFor()
      await page.setViewportSize({ width, height })
      await cdp.send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: scale, mobile: false })
      await page.emulateMedia({ reducedMotion: reduced ? 'reduce' : 'no-preference' })
      await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
      await settle()
      await group.scrollIntoViewIfNeeded(); await settle()
      box = await group.boundingBox()
      assert(box.x >= -1 && box.x + box.width <= width + 1)
      for (const key of state.groups[0].members) {
        const member = await slot(key).boundingBox()
        assert(member.x >= box.x && member.x + member.width <= box.x + box.width + 1)
        assert(member.y >= box.y + 47 && member.y + member.height <= box.y + box.height + 1)
      }
      if (reduced) assert(Number.parseFloat(await group.evaluate(el => getComputedStyle(el).transitionDuration)) < 0.001)
      assert.equal(writes, previousWrites)
      await page.screenshot({ path: `${out}/${width}-${theme}.png` })
      const titleSize = await group.locator('strong').evaluate(el => parseFloat(getComputedStyle(el).fontSize))
      const memberSize = await slot('nav:/overview').locator('.desktop__icon-label').evaluate(el => parseFloat(getComputedStyle(el).fontSize))
      assert(titleSize >= 14); assert(memberSize >= 14)
      report.cases.push({ width, theme, locale, reduced, titleSize, memberSize, effectiveScale: scale, scaleMethod: 'CSS viewport plus device metrics emulation', contained: true, resizeWrites: 0 })
    }
    await cdp.send('Emulation.clearDeviceMetricsOverride')
    await page.setViewportSize({ width: 1920, height: 1080 }); await settle()
    await group.locator('.desktop-group__menu').click()
    await save(() => page.getByRole('button', { name: '解散分组', exact: true }).click())
    assert.equal(state.groups.length, 0)
    assert.equal(await slot('nav:/overview').getAttribute('data-group-member'), null)
    await save(() => page.locator('.desktop-group-undo').getByText('撤销', { exact: true }).click())
    assert.equal(state.groups.length, 1)
    report.cases.push('dissolve preserves entries; undo restores group')
    assert.equal(report.errors.length, 0)
    assert.equal(resourceFailure, false)
    await context.tracing.stop({ path: `${out}/trace.zip` })
    await context.close()
  } catch (error) { report.failure = error.stack; throw error }
  finally { clearInterval(watchdog); await browser.close(); report.cleanup = true; fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify(report))
})().catch(error => { console.error(error); process.exitCode = 1 })

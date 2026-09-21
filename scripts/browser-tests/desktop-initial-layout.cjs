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
    for (const [width, theme, zoom, reducedMotion] of [[1280, 'dark', 1, 'no-preference'], [1280, 'light', 2, 'no-preference'], [390, 'light', 1, 'no-preference'], [1280, 'dark', 1, 'reduce']]) {
      // Match the CSS viewport and pixel density of browser zoom without changing application styles.
      const context = await browser.newContext({ viewport: { width: width / zoom, height: 900 / zoom }, deviceScaleFactor: zoom, reducedMotion })
      await context.addInitScript(({ theme }) => {
        localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
        localStorage.setItem('kejilion-panel-desktop-windows', '[]')
        localStorage.setItem('kejilion-panel-theme', theme)
      }, { theme })
      const page = await context.newPage()
      page.setDefaultTimeout(10000)
      page.on('pageerror', error => report.errors.push(error.message))
      let release, failed = false
      let pending = new Promise(resolve => { release = resolve })
      let workspace = { schemaVersion: 5, available: true, resourceVersion: 'sha256:' + '1'.repeat(64), groups: [{ id: 'a'.repeat(32), name: 'Saved group', columns: 4, rows: 0, collapsed: false, members: ['nav:/overview', 'nav:/terminal'] }], positions: {}, widgetPositions: {}, hiddenEntryKeys: [], hiddenWidgetKeys: [], labels: {}, shortcuts: [] }
      await page.route('**/api/v1/desktop/workspace', async route => {
        if (route.request().method() === 'PUT') {
          workspace = { ...workspace, ...route.request().postDataJSON() }
        } else await pending
        await route.fulfill(failed ? { status: 500, json: { title: 'Injected load failure' } } : { json: workspace })
      })
      const verifyReveal = async label => {
        await page.locator('.desktop__icons--initializing').waitFor({ state: 'attached' })
        await page.mouse.move(0, 0) // Keep legitimate hover transitions out of restoration samples.
        assert.equal(await page.locator('[data-icon-key="nav:/overview"]').isVisible(), false)
        assert.equal(await page.locator('.desktop__icons').evaluate(element => element.inert), true)
        // Capture every paint from the pending state through the first visible frames.
        await page.evaluate(() => {
          window.layoutFrames = []
          const capture = () => {
            const grid = document.querySelector('.desktop__icons')
            const icon = grid?.querySelector('[data-icon-key="nav:/overview"]')
            if (icon) {
              const visible = getComputedStyle(icon).visibility === 'visible'
              const rect = icon.getBoundingClientRect()
              const animations = grid.getAnimations({ subtree: true }).filter(animation => animation.effect.target.matches('.desktop__icon, .desktop__icon-slot, .desktop-group'))
              const opacity = getComputedStyle(icon.querySelector('.desktop__icon')).opacity
              const surfaceOpacity = Number(getComputedStyle(grid).opacity)
              const surfaceAnimations = grid.getAnimations().map(animation => ({ name: animation.animationName, duration: animation.effect.getTiming().duration }))
              window.layoutFrames.push({ visible, member: icon.dataset.groupMember, x: rect.x, y: rect.y, opacity, surfaceOpacity, surfaceAnimations, animations: animations.length })
            }
            if (window.layoutFrames.filter(frame => frame.visible).length < 8) requestAnimationFrame(capture)
          }
          requestAnimationFrame(capture)
        })
        release()
        await page.waitForFunction(() => window.layoutFrames.filter(frame => frame.visible).length >= 8)
        const frames = await page.evaluate(() => window.layoutFrames.filter(frame => frame.visible))
        report.cases.push({ label, width, theme, zoom, reducedMotion, zoomMode: 'effective-css-viewport', frames })
        if (reducedMotion === 'reduce') {
          assert(frames.every(frame => frame.surfaceOpacity === 1 && frame.surfaceAnimations.length === 0), `${label}: reduced motion animated`)
        } else {
          assert(frames.some(frame => frame.surfaceOpacity > 0 && frame.surfaceOpacity < 1), `${label}: missing gentle reveal`)
          assert(frames.flatMap(frame => frame.surfaceAnimations).every(animation => animation.name === 'desktop-workspace-reveal' && animation.duration === 180))
          assert(frames.every((frame, index) => index === 0 || frame.surfaceOpacity >= frames[index - 1].surfaceOpacity), `${label}: reveal restarted`)
        }
        await page.waitForFunction(() => getComputedStyle(document.querySelector('.desktop__icons')).opacity === '1')
        assert(frames.every(frame => frame.animations === 0), `${label}: initial placement animated`)
        assert(frames.every(frame => frame.opacity === '1'), `${label}: icon faded in after reveal`)
        assert(frames.every(frame => frame.x === frames[0].x && frame.y === frames[0].y), `${label}: visible coordinates changed`)
        if (!failed) assert(frames.every(frame => frame.member === 'a'.repeat(32)), `${label}: ungrouped frame`)
        assert.equal(await page.locator('.desktop__icons').evaluate(element => element.inert), false)
      }
      await page.goto(base)
      await verifyReveal('initial refresh')
      await page.screenshot({ path: `${out}/${width}-${theme}-${zoom}-${reducedMotion}.png` })
      await page.locator('.desktop__classic-button').click()
      pending = new Promise(resolve => { release = resolve })
      await page.locator('.desktop-entry-button').click()
      await verifyReveal('classic to desktop')
      await page.locator('.desktop-group__toggle').focus()
      await page.keyboard.press('Enter')
      await page.waitForFunction(() => document.querySelector('[data-icon-key="nav:/overview"]').getAttribute('aria-hidden') === 'true')
      await page.locator('.desktop-group__toggle').click()
      await page.locator('[data-icon-key="nav:/overview"]').waitFor({ state: 'visible' })
      failed = true
      pending = new Promise(resolve => { release = resolve })
      await page.reload()
      await verifyReveal('failed workspace load')
      await context.close()
    }
    assert.deepEqual(report.errors, [])
  } catch (error) { report.failure = error.stack; throw error }
  finally { await browser.close(); fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify({ candidate, cases: report.cases.length, errors: report.errors }))
})().catch(error => { console.error(error); process.exitCode = 1 })

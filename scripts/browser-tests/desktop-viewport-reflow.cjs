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
const overlap = (a, b) => a.x < b.x + b.width - 0.1 && b.x < a.x + a.width - 0.1 && a.y < b.y + b.height - 0.1 && b.y < a.y + a.height - 0.1

;(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
  const deadline = setTimeout(() => { void browser.close() }, 55000)
  try {
    for (const grouped of [false, true]) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
      await context.addInitScript(() => {
        localStorage.setItem('kejilion-panel-desktop-mode', 'desktop')
        localStorage.setItem('kejilion-panel-theme', 'dark')
        localStorage.setItem('kejilion-panel-desktop-windows', '[]')
      })
      const page = await context.newPage()
      page.setDefaultTimeout(8000)
      page.on('pageerror', error => report.errors.push(error.message))
      const groupID = 'a'.repeat(32)
      let writes = 0
      let workspace = { schemaVersion: 5, available: true, resourceVersion: 'sha256:' + '1'.repeat(64), groups: grouped ? [{ id: groupID, name: 'Saved group', columns: 4, rows: 0, collapsed: false, members: ['nav:/overview', 'nav:/terminal'] }] : [], positions: { 'nav:/overview': { x: 0, y: 0 }, 'nav:/terminal': { x: 0, y: 0.5 }, 'nav:/files': { x: 0, y: 1 }, ['group:' + groupID]: { x: 0.35, y: 0.5 } }, widgetPositions: { 'widget:clock': { x: 1, y: 0 }, 'widget:monitor': { x: 1, y: 0.4 }, 'widget:services': { x: 1, y: 0.9 } }, hiddenEntryKeys: [], hiddenWidgetKeys: [], labels: {}, shortcuts: [] }
      await page.route('**/api/v1/desktop/workspace', async route => {
        if (route.request().method() === 'PUT') { writes++; workspace = { ...workspace, ...route.request().postDataJSON() } }
        await route.fulfill({ json: workspace })
      })
      const settled = async () => {
        // Let the resize event and ResizeObserver deliver before inspecting their settled state.
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
        await page.waitForFunction(() => {
        const grid = document.querySelector('.desktop__icons')
        return grid && !grid.classList.contains('desktop__icons--resizing') && !grid.classList.contains('desktop__icons--restoring') && !grid.getAnimations({ subtree: true }).some(animation => animation instanceof CSSTransition && ['transform', 'left', 'top'].includes(animation.transitionProperty))
        })
      }
      const snapshot = () => page.evaluate(() => {
        const grid = document.querySelector('.desktop__icons'), area = grid.getBoundingClientRect()
        const entities = [...grid.querySelectorAll(':scope > .desktop__icon-slot:not([data-group-member]), :scope > .desktop-widget-slot, :scope > .desktop-group')].filter(element => getComputedStyle(element).display !== 'none').map(element => {
          const rect = element.getBoundingClientRect()
          const round = n => Math.round(n * 100) / 100
          return { key: element.dataset.iconKey || element.dataset.groupId || element.getAttribute('aria-label'), x: round(rect.left - area.left), y: round(rect.top - area.top + grid.scrollTop), width: round(rect.width), height: round(rect.height) }
        })
        return { entities, width: area.width, height: area.height, contentHeight: grid.scrollHeight }
      })
      await page.goto(base)
      await settled()
      const original = await snapshot(), originalWorkspace = JSON.stringify(workspace)
      await page.evaluate(() => {
        window.resizeFrames = []
        window.recordResize = true
        const capture = () => {
          const grid = document.querySelector('.desktop__icons')
          const animations = grid.getAnimations({ subtree: true }).filter(a => a instanceof CSSTransition && ['transform', 'left', 'top'].includes(a.transitionProperty))
          window.resizeFrames.push({ resizing: grid.classList.contains('desktop__icons--resizing'), moving: animations.length })
          if (window.recordResize) requestAnimationFrame(capture)
        }
        requestAnimationFrame(capture)
      })
      for (const [width, height] of [[1895, 995], [1100, 720], [768, 650], [390, 700], [1280, 900]]) {
        await page.setViewportSize({ width, height })
        await settled()
        const state = await snapshot()
        for (let i = 0; i < state.entities.length; i++) {
          const entity = state.entities[i]
          assert(entity.x >= -0.1 && entity.x + entity.width <= state.width + 0.1, `${entity.key} escaped viewport`)
          assert(entity.y + entity.height <= state.contentHeight + 0.1, `${entity.key} is unreachable`)
          for (let j = i + 1; j < state.entities.length; j++) assert(!overlap(entity, state.entities[j]), `${entity.key} overlaps ${state.entities[j].key}`)
        }
        if (width === 1280) assert.deepEqual(state, original, 'original viewport must restore the saved layout')
        else {
          const icons = state.entities.filter(item => item.key.startsWith('nav:') || item.width === 90)
          const rows = Math.max(1, Math.floor((state.height - 96) / 100) + 1)
          const last = Math.max(...icons.map(item => Math.round(item.x / 95) * rows + Math.round(item.y / 100)))
          for (let slot = 0; slot < last; slot++) {
            const cell = { x: Math.floor(slot / rows) * 95, y: (slot % rows) * 100, width: 90, height: 96 }
            assert(state.entities.some(item => overlap(cell, item)), `unused icon hole at ${cell.x},${cell.y}`)
          }
          if (width > 900) {
            const widgets = state.entities.filter(item => item.key.startsWith('widget:')).sort((a, b) => a.y - b.y)
            assert.deepEqual(widgets.map(item => item.y), [0, 200, 500])
          }
        }
        assert.equal(writes, 0, 'resize must not persist workspace positions')
        assert.equal(JSON.stringify(workspace), originalWorkspace)
        await page.screenshot({ path: `${out}/${grouped ? 'grouped' : 'plain'}-${width}.png` })
        report.cases.push({ grouped, width, height, compact: true, noOverlap: true, writes })
      }
      // A rapid stream of intermediate sizes must also keep placement motion disabled.
      for (const width of [1240, 1200, 1120, 1080, 1040]) await page.setViewportSize({ width, height: 800 })
      await settled()
      const frames = await page.evaluate(() => { window.recordResize = false; return window.resizeFrames })
      assert(frames.some(frame => frame.resizing), 'resize suppression was never exercised')
      assert(frames.every(frame => frame.moving === 0), 'a resize frame animated placement')
      const clock = page.locator('.desktop-widget-slot[aria-label="widget:clock"]')
      await clock.focus()
      await page.keyboard.press('Control+ArrowLeft')
      await page.waitForFunction(() => document.querySelector('.desktop-widget-slot') !== null)
      await settled()
      assert.equal(writes, 1, 'manual adjustment must still persist')
      const edited = await snapshot()
      await page.reload()
      await settled()
      assert.deepEqual(await snapshot(), edited, 'manual layout must survive reload')
      report.cases.push({ grouped, rapidResize: true, noPlacementAnimations: true, manualEditPersisted: true })
      await context.close()
    }
    assert.deepEqual(report.errors, [])
  } catch (error) { report.failure = error.stack; throw error }
  finally { clearTimeout(deadline); await browser.close(); fs.writeFileSync(`${out}/result.json`, JSON.stringify(report, null, 2)) }
  console.log(JSON.stringify({ candidate, cases: report.cases.length, errors: report.errors }))
})().catch(error => { console.error(error); process.exitCode = 1 })

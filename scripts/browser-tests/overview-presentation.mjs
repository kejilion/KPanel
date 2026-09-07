// Real-browser regression for staged first paint and the actual 20-second poll.
// Run against the canonical mock preview; no credentials or host writes.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync, statfsSync } from 'node:fs'
import { freemem } from 'node:os'
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const out = process.env.OVERVIEW_EVIDENCE
assert(out)
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
assert.equal(candidate, process.env.REPORT_CANDIDATE)
assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '')
mkdirSync(out, { recursive: true })
const result = { candidate, mode: 'mock-ui', cases: [], errors: [], regressions: [], cleanup: false }
const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
result.browserVersion = browser.version()
result.playwrightVersion = require((process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright') + '/package.json').version
let resourceFailure = false
const timeout = setTimeout(() => { resourceFailure = true; void browser.close() }, 240_000)
const watchdog = setInterval(() => {
  const fs = statfsSync(process.cwd())
  if (freemem() < 512 * 1024 ** 2 || fs.bavail * fs.bsize < 1024 ** 3) {
    resourceFailure = true; void browser.close()
  }
}, 1000)
function check(condition, description) {
  if (!condition) result.regressions.push(description)
}
function gate() {
  let release
  const promise = new Promise(resolve => { release = resolve })
  return { promise, release }
}
async function contextFor(width, theme, locale, mode) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
  await context.addInitScript(({ theme, locale, mode }) => {
    localStorage.setItem('kejilion-panel-theme', theme)
    localStorage.setItem('kejilion-panel-locale', locale)
    localStorage.setItem('kejilion-panel-desktop-mode', mode)
  }, { theme, locale, mode })
  const page = await context.newPage()
  page.setDefaultTimeout(12_000)
  page.on('pageerror', error => result.errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error' && !message.text().includes('503')) result.errors.push(message.text()) })
  return { context, page }
}
async function completeMockRoute(route) {
  const path = new URL(route.request().url()).pathname
  if (path === '/api/v1/system/public-network') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ipv4: '192.0.2.10', ipv6: '2001:db8::10', isp: 'Mock Network', country: 'US', source: 'isolated mock', updatedAt: '2026-09-07T00:00:00Z' }) })
  if (/^\/api\/v1\/sites\/c{32}\/appearance$/.test(path)) return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
  if (/^\/api\/v1\/sites\/c{32}\/icon$/.test(path)) return route.fulfill({ status: 200, contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="#64748b"/></svg>' })
  return route.continue()
}
async function openOverview(page, mode) {
  await page.goto(base + '/')
  if (mode === 'desktop') await page.locator('[data-icon-key="nav:/overview"]').dblclick()
}
async function geometry(page) {
  return page.evaluate(() => {
    const selectors = ['.realtime-monitoring', '.overview-grid', '.panel-card--resource-overview', '.overview-system-management', '.service-grid']
    const root = document.querySelector('.realtime-monitoring')?.closest('.page')
    return { height: root?.getBoundingClientRect().height,
      anchors: selectors.map(selector => { const e = document.querySelector(selector); const r = e?.getBoundingClientRect(); return r ? [r.top, r.height] : null }),
      cards: Array.from(document.querySelectorAll('.overview-system-card')).map(e => e.getBoundingClientRect().height),
      services: document.querySelectorAll('.service-item').length,
      scroll: [scrollY, document.querySelector('.desktop-window__body')?.scrollTop || 0] }
  })
}
try {
  // Cold load cannot put the lower management section ahead of upper metrics.
  for (const [width, theme, locale, scale, mode] of [
    [1280, 'light', 'zh-CN', 1, 'classic'],
    [768, 'dark', 'en-US', 1.25, 'classic'],
    [390, 'dark', 'zh-TW', 2, 'classic'],
    [1280, 'dark', 'en-US', 1, 'desktop'],
  ]) {
    const { context, page } = await contextFor(width, theme, locale, mode)
    const runtime = gate(); const management = gate()
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      if (url.origin !== base) return route.abort()
      if (url.pathname.endsWith('/system/runtime')) await runtime.promise
      if (url.pathname.includes('/system/management/')) await management.promise
      await completeMockRoute(route)
    })
    try {
      await openOverview(page, mode)
      await page.locator('.loading-state').first().waitFor()
      await page.waitForTimeout(300)
      check(await page.locator('.overview-system-management').count() === 0, `cold lower-section flash ${width}/${mode}`)
      await page.screenshot({ path: `${out}/cold-${width}-${mode}.png`, fullPage: true })
      runtime.release()
      await page.locator('.realtime-monitoring').waitFor()
      assert.equal(await page.locator('.overview-system-card').count(), 6)
      assert.equal(await page.locator('.overview-system-card[aria-busy="true"]').count(), 6)
      const ordered = await geometry(page)
      check(ordered.anchors.slice(0, 4).every((rect, i, list) => rect && (!i || rect[0] >= list[i - 1][0])), 'original section order')
      management.release()
      await page.waitForFunction(() => document.querySelectorAll('.overview-system-card[aria-busy="true"]').length === 0)
      await page.evaluate(scale => { document.documentElement.style.fontSize = `${16 * scale}px` }, scale)
      check(await page.locator('.overview-system-card__body').first().locator('small').count() <= 1, 'overview card restores one detail row')
      await page.screenshot({ path: `${out}/overview-${width}-${mode}.png`, fullPage: true })
      result.cases.push({ width, theme, locale, scale, scaleMethod: 'root text scaling', mode, ordered })
    } finally { runtime.release(); management.release(); await context.close() }
  }
  // Static catalog remains available even while all network reads are pending.
  for (const [width, theme, locale, scale] of [[1280, 'light', 'zh-CN', 1], [768, 'dark', 'en-US', 1.25], [390, 'dark', 'zh-TW', 2]]) {
    const { context, page } = await contextFor(width, theme, locale, 'classic')
    const runtime = gate(); const defense = gate(); let fail = true
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      if (url.origin !== base) return route.abort()
      if (url.pathname.endsWith('/system/runtime')) await runtime.promise
      if (url.pathname.endsWith('/management/ssh-defense')) await defense.promise
      if (url.pathname.endsWith('/management/bbrv3') && fail) return route.fulfill({ status: 503, contentType: 'application/json', body: '{"title":"isolated failure"}' })
      await completeMockRoute(route)
    })
    try {
      await page.goto(base + '/system')
      const cards = page.locator('.system-tool')
      await cards.first().waitFor()
      assert.equal(await cards.count(), 24)
      assert(await cards.first().isDisabled())
      check(!(await cards.first().innerText()).includes('对应 kejilion.sh'), 'no script-description row on catalog')
      runtime.release()
      const dns = cards.filter({ has: page.locator('strong').filter({ hasText: /^DNS/ }) }).first()
      await page.waitForFunction(() => Array.from(document.querySelectorAll('.system-tool')).some(e => e.querySelector('strong')?.textContent?.startsWith('DNS') && !e.disabled))
      await dns.click()
      await page.locator('.modal-panel').waitFor()
      assert((await page.locator('.modal-panel').innerText()).includes('kejilion.sh'))
      await page.keyboard.press('Escape')
      await page.locator('.modal-panel').waitFor({ state: 'hidden' })
      defense.release()
      await page.waitForFunction(() => document.querySelectorAll('.system-tool[aria-busy="true"]').length === 0)
      fail = false
      await page.locator('.inline-alert button').click()
      await page.waitForFunction(() => !document.querySelector('.inline-alert button'))
      await page.evaluate(scale => { document.documentElement.style.fontSize = `${16 * scale}px` }, scale)
      check(await cards.first().locator(':scope > small').count() === 1, 'system card restores one detail row')
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 2), 'no horizontal page overflow')
      await page.screenshot({ path: `${out}/system-${width}.png`, fullPage: true })
      result.cases.push({ catalog: 24, width, theme, locale, scale, independentFailureRetry: true, keyboard: true })
    } finally { runtime.release(); defense.release(); await context.close() }
  }
  // Do not accelerate timers: wait for the production 20-second polling path.
  for (const mode of ['classic', 'desktop']) {
    const { context, page } = await contextFor(1280, 'dark', 'zh-CN', mode)
    const resources = gate(); let runtimeCount = 0; let hold = false
    const optional = ['/sites', '/docker/summary', '/docker/containers', '/system/public-network', '/apps']
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      if (url.origin !== base) return route.abort()
      if (url.pathname.endsWith('/system/runtime')) runtimeCount++
      if (hold && optional.some(path => url.pathname === '/api/v1' + path)) await resources.promise
      await completeMockRoute(route)
    })
    try {
      await openOverview(page, mode)
      await page.locator('.service-item').first().waitFor()
      await page.waitForFunction(() => !document.querySelector('.realtime-monitoring .spin'))
      await page.waitForTimeout(400)
      await page.locator('.overview-system-management').scrollIntoViewIfNeeded()
      const before = await geometry(page)
      assert(before.services > 0)
      hold = true
      const start = Date.now(); const initialCount = runtimeCount
      while (runtimeCount <= initialCount && Date.now() - start < 27_000) await page.waitForTimeout(200)
      assert(runtimeCount > initialCount, 'actual poll fired')
      await page.waitForTimeout(600)
      const during = await geometry(page)
      check(JSON.stringify(before) === JSON.stringify(during), `idle geometry retained ${mode}`)
      await page.screenshot({ path: `${out}/idle-${mode}.png`, fullPage: true })
      resources.release()
      await page.waitForFunction(() => !document.querySelector('.realtime-monitoring .spin'))
      const dns = page.locator('.overview-system-card').filter({ has: page.locator('strong').filter({ hasText: /^DNS/ }) }).first()
      await dns.click()
      const input = page.locator('.modal-panel textarea')
      await input.fill('9.9.9.9\n149.112.112.112')
      const formStart = Date.now(); const formCount = runtimeCount
      while (runtimeCount <= formCount && Date.now() - formStart < 27_000) await page.waitForTimeout(200)
      assert(runtimeCount > formCount, 'poll while editing fired')
      await page.waitForFunction(() => !document.querySelector('.realtime-monitoring .spin'))
      assert.equal(await input.inputValue(), '9.9.9.9\n149.112.112.112')
      await page.keyboard.press('Escape')
      await page.locator('.modal-panel').waitFor({ state: 'hidden' })
      result.cases.push({ mode, actualPollCount: runtimeCount, elapsedMs: Date.now() - start, before, during, unsavedFormRetained: true })
    } finally { resources.release(); await context.close() }
  }
  assert.deepEqual(result.errors, [])
  assert.equal(resourceFailure, false)
  if (process.env.EXPECT_REGRESSION === '1') assert(result.regressions.length > 0, 'baseline must reproduce regression')
  else assert.deepEqual(result.regressions, [])
  result.passed = result.regressions.length === 0
} finally {
  clearTimeout(timeout); clearInterval(watchdog)
  await browser.close()
  result.cleanup = true; result.resourceFailure = resourceFailure
  writeFileSync(`${out}/summary.json`, JSON.stringify(result, null, 2))
}

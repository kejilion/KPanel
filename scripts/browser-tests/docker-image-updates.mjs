// Run against the canonical loopback mock preview through background-browser-test.mjs.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync, statfsSync } from 'node:fs'
import { freemem } from 'node:os'
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const out = process.env.DOCKER_UPDATE_EVIDENCE
assert(out)
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
assert.equal(candidate, process.env.REPORT_CANDIDATE)
assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '')
mkdirSync(out, { recursive: true })
const result = { candidate, mode: 'mock-ui', cases: [], errors: [], cleanup: false }
const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined })
let resourceFailure = false
const watchdog = setInterval(() => {
  const fs = statfsSync(process.cwd())
  if (freemem() < 512 * 1024 ** 2 || fs.bavail * fs.bsize < 1024 ** 3) {
    resourceFailure = true
    void browser.close()
  }
}, 1000)
try {
  for (const [width, theme, locale, scale, mode] of [
    [1280, 'light', 'zh-CN', 1, 'classic'],
    [768, 'dark', 'en-US', 1.25, 'classic'],
    [390, 'dark', 'zh-TW', 2, 'classic'],
    [1280, 'dark', 'zh-CN', 1, 'desktop'],
  ]) {
    const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
    await context.addInitScript(({ theme, locale, mode }) => {
      localStorage.setItem('kejilion-panel-theme', theme)
      localStorage.setItem('kejilion-panel-locale', locale)
      localStorage.setItem('kejilion-panel-desktop-mode', mode)
    }, { theme, locale, mode })
    const page = await context.newPage()
    page.setDefaultTimeout(15_000)
    page.on('pageerror', error => result.errors.push(error.message))
    let requests = 0
    page.on('request', request => { if (request.url().endsWith('/check_update')) requests++ })
    await page.goto(base + (mode === 'desktop' ? '/' : '/docker'))
    if (mode === 'desktop') await page.locator('[data-icon-key="nav:/docker"]').dblclick()
    await page.evaluate(scale => { document.documentElement.style.fontSize = `${16 * scale}px` }, scale)
    await page.locator('.docker-row').first().waitFor()
    const beforeHeights = await page.locator('.docker-row').evaluateAll(rows => rows.map(row => row.getBoundingClientRect().height))
    await page.locator('.docker-image-update').waitFor()
    await page.waitForTimeout(3_500)
    assert.equal(await page.locator('.docker-image-update').count(), 1)
    assert.equal(await page.locator('.docker-update-details, .docker-image-update__button').count(), 0)
    assert.equal(requests, 4)
    const badge = page.locator('.docker-image-update')
    const label = locale === 'en-US' ? 'Image update available' : locale === 'zh-TW' ? '有映像更新' : '有镜像更新'
    assert.equal(await badge.getAttribute('title'), label)
    assert.equal(await badge.getAttribute('aria-label'), label)
    assert.equal(await badge.getAttribute('role'), 'img')
    assert.equal(await badge.innerText(), '')
    const geometry = await badge.evaluate(element => {
      const style = getComputedStyle(element), rect = element.getBoundingClientRect(), parent = element.parentElement.getBoundingClientRect()
      return { color: style.color, position: style.position, cursor: style.cursor, height: rect.height, width: rect.width, topOffset: rect.top - parent.top, rightOffset: rect.right - parent.right }
    })
    assert.equal(geometry.position, 'absolute')
    assert.equal(geometry.cursor, 'default')
    assert(geometry.height >= 14 && geometry.width >= 14)
    const afterHeights = await page.locator('.docker-row').evaluateAll(rows => rows.map(row => row.getBoundingClientRect().height))
    assert.deepEqual(afterHeights, beforeHeights, 'update badge must not change container row heights')
    await badge.hover()
    await badge.click()
    assert.equal(requests, 4, 'passive badge must not trigger checks or expand details')
    assert.equal(await page.locator('.docker-image-update small, .docker-image-update button').count(), 0)
    await page.getByRole('button', { name: locale === 'en-US' ? 'Refresh Docker status' : locale === 'zh-TW' ? '重新整理 Docker 狀態' : '刷新 Docker 状态', exact: true }).click()
    await page.waitForTimeout(2_000)
    assert.equal(requests, 4, 'refresh must reuse unchanged image identities')
    await page.screenshot({ path: `${out}/${width}-${theme}-${mode}-quiet.png`, fullPage: true })
    result.cases.push({ width, theme, locale, scale, mode, requests, geometry, beforeHeights, afterHeights })
    await context.close()
  }
  for (const locale of ['zh-CN', 'en-US']) {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: 'reduce' })
    await context.addInitScript(locale => {
      localStorage.setItem('kejilion-panel-locale', locale)
      localStorage.setItem('kejilion-panel-theme', 'light')
      localStorage.setItem('kejilion-panel-desktop-mode', 'classic')
    }, locale)
    const page = await context.newPage()
    page.setDefaultTimeout(15_000)
    page.on('pageerror', error => result.errors.push(error.message))
    let requests = 0, inventoryRequests = 0, status = 'current'
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/apps') inventoryRequests++ })
    await page.route('**/api/v1/apps/*/check_update', async route => {
      requests++
      const failed = status === 'unavailable'
      await route.fulfill({ status: failed ? 502 : 200, contentType: 'application/json', body: JSON.stringify(failed
        ? { code: 'docker_update_registry_auth', title: 'secret-registry.example/token=never-display' }
        : { containerId: 'a'.repeat(64), image: 'ghcr.io/librespeed/speedtest:latest', resourceVersion: 'fresh-read-snapshot',
            status, updateAvailable: status === 'available', checkedAt: new Date().toISOString() }) })
    })
    await page.goto(base + '/apps')
    await page.locator('.app-card.is-installed').filter({ hasText: /LibreSpeed/i }).locator('.app-card__main').click()
    const check = page.getByRole('button', { name: locale === 'en-US' ? 'Check for updates' : '检查更新', exact: true })
    const state = page.locator('.app-control-panel__status > div').nth(1).locator('strong')
    const inventoryBefore = inventoryRequests
    const labels = locale === 'en-US'
      ? ['No update found for this tag', 'Update available', 'Fixed version', 'Registry access denied. Check image visibility and registry access permissions.']
      : ['该标签未发现更新', '发现更新', '固定版本', '仓库拒绝访问，请检查镜像是否公开及仓库访问权限。']
    for (const [index, next] of ['current', 'available', 'fixed', 'unavailable'].entries()) {
      status = next
      await check.click()
      if (next === 'unavailable') {
        await page.getByText(labels[index], { exact: true }).waitFor()
        assert(!['该标签未发现更新', '固定版本', 'No update found for this tag', 'Fixed version'].includes(await state.innerText()))
        assert(!(await page.locator('body').innerText()).includes('secret-registry.example'))
      } else {
        await page.waitForFunction(({ label }) => document.querySelector('.app-control-panel__status > div:nth-child(2) strong')?.textContent === label, { label: labels[index] })
      }
      assert.equal(requests, index + 1, 'market must make exactly one request per explicit check')
    }
    assert.equal(inventoryRequests, inventoryBefore, 'market must not multiply shared retries by reloading inventory')
    await page.screenshot({ path: `${out}/apps-${locale}-classified-failure.png`, fullPage: true })
    result.cases.push({ consumer: 'apps', locale, requests, inventoryReloads: inventoryRequests - inventoryBefore, statuses: ['current', 'available', 'fixed', 'unavailable'] })
    await context.close()
  }
  assert.equal(resourceFailure, false)
  assert.deepEqual(result.errors, [])
  result.status = 'passed'
} catch (error) {
  result.status = 'failed'
  result.failure = String(error.stack || error)
  throw error
} finally {
  clearInterval(watchdog)
  await browser.close()
  result.cleanup = true
  writeFileSync(`${out}/result.json`, JSON.stringify(result, null, 2))
}

// Real TerminalView/xterm against the local-feature-preview mock API. No shell is executed.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '../..');
const out = path.resolve(process.env.SPLIT_EVIDENCE || path.join(root, '.codex-tmp/terminal-split-browser'));
fs.mkdirSync(out, { recursive: true });
const visiblePanes = (page) => page.locator('.terminal-pane[data-session-id]:visible');
const waitPaneCount = (page, count) => page.waitForFunction((count) => [...document.querySelectorAll('.terminal-pane[data-session-id]')]
  .filter((el) => el.getBoundingClientRect().width > 0 && el.getBoundingClientRect().height > 0).length === count, count);
const settle = (page) => page.waitForFunction(() => [...document.querySelectorAll('.terminal-pane[data-session-id]')]
  .filter((el) => el.getBoundingClientRect().width > 0).every((el) => el.querySelector('.xterm-screen')?.getBoundingClientRect().height > 0));

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined });
  const result = { candidate: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(),
    mode: 'UI Mock: real TerminalView and xterm, simulated API', browser: browser.version(), cases: [], errors: [] };
  try {
    for (const [width, theme, zoom, language, mode = 'classic'] of [
      [1600, 'dark', 1, 'zh-CN'], [1600, 'light', 1, 'zh-CN'],
      [1600, 'dark', 1.25, 'zh-CN'], [1600, 'light', 2, 'zh-CN'],
      [390, 'dark', 1, 'zh-CN'], [1600, 'light', 1, 'en-US'],
      [1600, 'dark', 1, 'zh-CN', 'desktop'],
    ]) {
      // Equivalent CSS viewport and device scale for browser zoom; media queries also reflow.
      const viewport = { width: Math.floor(width / zoom), height: Math.floor(1100 / zoom) };
      const context = await browser.newContext({ viewport, deviceScaleFactor: zoom, reducedMotion: 'reduce' });
      await context.addInitScript(({ theme, language }) => {
        localStorage.setItem('kejilion-panel-theme', theme);
        localStorage.setItem('kejilion-panel-locale', language);
      }, { theme, language });
      const page = await context.newPage();
      page.setDefaultTimeout(8000);
      page.on('pageerror', (error) => result.errors.push(error.message));
      const requests = [];
      page.on('request', (request) => {
        if (/\/terminal-sessions\/[^/]+\/(input|resize|close)$/.test(request.url())) {
          requests.push({ url: request.url(), body: request.postDataJSON() });
        }
      });
      try {
        await page.goto(`${process.env.PREVIEW_URL}/terminal`);
        await page.locator('.host-terminal__composer input').first().waitFor();
        await page.evaluate(async ({ theme, language, mode }) => {
          const { useTheme } = await import('/src/stores/theme.ts');
          useTheme().setTheme(theme);
          const { setLocale } = await import('/src/i18n/index.ts');
          await setLocale(language);
          if (mode === 'desktop') {
            const { useDesktopMode } = await import('/src/stores/desktopMode.ts');
            const desktop = useDesktopMode();
            desktop.enterDesktop();
            desktop.openWindow('/terminal', 'route.terminal', false);
          }
        }, { theme, language, mode });
        await page.locator('.host-terminal__composer input').first().waitFor();
        assert.equal(await page.locator('html').getAttribute('data-theme'), theme);
        requests.length = 0;
        // Keep the exact xterm nodes to detect remounts during layout changes.
        await page.evaluate(() => { window.__splitNodes = [...document.querySelectorAll('.xterm')]; });
        const layout = page.locator('.terminal-layout-trigger');
        assert.equal(await layout.isVisible(), viewport.width > 900);
        assert.match(await layout.getAttribute('aria-label'), /Terminal layout|终端布局/);
        // Configure split layouts at desktop width, then verify responsive preservation on mobile.
        if (viewport.width <= 900) await page.setViewportSize({ ...viewport, width: 1000 });
        const chooseLayout = async (value) => {
          if (viewport.width <= 900) await page.setViewportSize({ ...viewport, width: 1000 });
          await layout.click();
          assert.equal(await page.locator('.terminal-layout-menu button svg:first-child').count(), 4);
          assert.equal(await page.locator('.terminal-layout-menu button span:visible').count(), 4);
          if (width === 1600 && theme === 'dark' && zoom === 1 && language === 'zh-CN') {
            await page.screenshot({ path: path.join(out, `layout-menu-${mode}.png`), fullPage: true });
          }
          await page.locator(`[data-layout="${value}"]`).click();
          if (viewport.width <= 900) {
            await page.setViewportSize(viewport);
            await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
            assert.equal(await layout.isVisible(), false);
          }
        };
        assert.equal(await layout.evaluate((element) => Boolean(element.closest('.terminal-tabs-bar'))), true);
        assert.equal(await page.locator('.terminal-pane-workspace .terminal-layout-controls').count(), 0);
        const controlBox = await layout.boundingBox();
        const toolbarBox = await page.locator('.terminal-tabs-bar').first().boundingBox();
        if (viewport.width > 900) assert(controlBox.x + controlBox.width <= toolbarBox.x + toolbarBox.width && controlBox.y >= toolbarBox.y);
        await layout.focus();
        await layout.press('ArrowDown');
        await page.keyboard.press('End');
        assert.equal(await page.locator('[data-layout="grid"]').evaluate((el) => el === document.activeElement), true);
        await page.keyboard.press('Escape');
        assert.equal(await layout.getAttribute('aria-expanded'), 'false');
        await chooseLayout('grid');
        await settle(page);
        const compact = await page.locator('.terminal-pane-workspace').evaluate((el) => el.classList.contains('is-single-visible'));
        assert.equal(requests.filter((r) => r.url.endsWith('/close')).length, 0);
        assert.equal(await page.locator('.terminal-pane-empty:visible').count(), compact ? 0 : 3);
        // Open three more independent sessions using the existing host-row action.
        for (let i = 0; i < 3; i++) {
          const drawer = page.locator('.terminal-tabs-bar__connections').first();
          if (await drawer.isVisible() && await drawer.getAttribute('aria-expanded') !== 'true') await drawer.click();
          await page.locator('.terminal-host__new').first().click();
          await page.waitForFunction((count) => document.querySelectorAll('.terminal-tab').length === count, i + 2);
        }
        await settle(page);
        const count = await visiblePanes(page).count();
        assert.equal(count, compact ? 1 : 4);
        if (!compact) {
          const cells = await visiblePanes(page).evaluateAll((panes) => panes.map((el) => [el.style.gridColumn, el.style.gridRow]));
          assert.deepEqual(cells, [['1', '1'], ['3', '1'], ['1', '3'], ['3', '3']]);
        }
        assert.equal(await page.evaluate(() => window.__splitNodes.every((node) => node.isConnected)), true);
        const drawer = page.locator('.terminal-tabs-bar__connections').first();
        if (await drawer.isVisible() && await drawer.getAttribute('aria-expanded') !== 'true') await drawer.click();
        const limited = page.waitForResponse((response) => response.url().endsWith('/terminal-sessions') && response.status() === 429);
        await page.locator('.terminal-host__new').first().click();
        await limited;
        assert.equal(await page.locator('.terminal-tab').count(), 4);
        await page.locator('.terminal-alert').waitFor();
        const pane = visiblePanes(page).first();
        const id = await pane.getAttribute('data-session-id');
        const input = pane.locator('.host-terminal__composer input');
        await input.fill('pwd');
        const sent = page.waitForRequest((request) => request.url().endsWith(`/${id}/input`));
        await input.press('Enter');
        await sent;
        assert.equal(await pane.evaluate((el) => el.classList.contains('is-active')), true);
        if (count === 4) {
          const divider = page.locator('[role="separator"][aria-orientation="vertical"]');
          await divider.focus();
          await divider.press('ArrowRight');
          assert.equal(await divider.getAttribute('aria-valuenow'), '55');
          await page.waitForFunction(() => {
            const grid = document.querySelector('.terminal-pane-grid');
            const divider = document.querySelector('.terminal-pane-divider.is-column');
            const rect = grid.getBoundingClientRect();
            return divider.getBoundingClientRect().left - rect.left > rect.width * .53;
          });
          const box = await divider.boundingBox();
          await page.mouse.move(box.x + box.width / 2, box.y + 30);
          await page.mouse.down();
          await page.mouse.move(box.x + 45, box.y + 30);
          await page.mouse.up();
          const draggedRatio = Number(await divider.getAttribute('aria-valuenow'));
          assert(draggedRatio > 55, JSON.stringify({ box, draggedRatio, grid: await page.locator('.terminal-pane-grid').boundingBox() }));
          await pane.locator('.terminal-pane-header button').first().click();
          await waitPaneCount(page, 1);
          assert.equal(await visiblePanes(page).count(), 1);
          await pane.locator('.terminal-pane-header button').first().click();
          await waitPaneCount(page, 4);
          assert.equal(await visiblePanes(page).count(), 4);
        }
        for (const value of ['rows', 'columns', 'single', 'grid']) await chooseLayout(value);
        await settle(page);
        assert.equal(await page.evaluate(() => window.__splitNodes.every((node) => node.isConnected)), true);
        assert.equal(requests.filter((r) => r.url.endsWith('/close')).length, 0);
        if (count === 4 && mode === 'classic') {
          if (width <= 900) {
            const quickToggle = page.locator('[aria-controls="terminal-quick-commands"]');
            await quickToggle.click();
            const quickPanel = page.locator('.terminal-quick-commands');
            await quickPanel.waitFor({ state: 'visible' });
            await layout.click();
            const option = page.locator('[data-layout="rows"]');
            const box = await option.boundingBox();
            const reachable = await page.evaluate(({ x, y }) => Boolean(document.elementFromPoint(x, y)?.closest('[data-layout="rows"]')),
              { x: box.x + box.width / 2, y: box.y + box.height / 2 });
            assert.equal(reachable, true, 'layout option must remain above the narrow quick-commands panel');
            await option.click();
            await quickToggle.click();
            await chooseLayout('grid');
          }
          await page.setViewportSize({ width: 500, height: 900 });
          await page.waitForFunction(() => document.querySelector('.terminal-pane-workspace').classList.contains('is-single-visible'));
          await waitPaneCount(page, 1);
          assert.equal(await visiblePanes(page).count(), 1);
          await page.setViewportSize(viewport);
          await page.waitForFunction(() => !document.querySelector('.terminal-pane-workspace').classList.contains('is-single-visible'));
          await waitPaneCount(page, 4);
          assert.equal(await visiblePanes(page).count(), 4);
        }
        const metrics = await page.evaluate(() => {
          const workspace = document.querySelector('.terminal-pane-workspace');
          const panes = [...workspace.querySelectorAll('.terminal-pane[data-session-id]')].filter((el) => el.getBoundingClientRect().width > 0);
          const text = [...workspace.querySelectorAll('button,select,small,header')].filter((el) => el.getBoundingClientRect().width > 0);
          return { overflow: workspace.scrollWidth > workspace.clientWidth + 1,
            minimumFont: Math.min(...text.map((el) => parseFloat(getComputedStyle(el).fontSize))),
            panes: panes.map((el) => ({ width: el.clientWidth, height: el.clientHeight,
              screen: el.querySelector('.host-terminal__screen').clientHeight,
              composer: el.querySelector('.host-terminal__composer').getBoundingClientRect().bottom <= el.getBoundingClientRect().bottom + 1 })) };
        });
        assert.equal(metrics.overflow, false);
        assert(metrics.minimumFont >= 12, JSON.stringify(metrics));
        assert(metrics.panes.every((pane) => pane.screen > 30 && pane.composer), JSON.stringify(metrics));
        await page.screenshot({ path: path.join(out, `${width}-${theme}-${zoom}-${language}-${mode}.png`), fullPage: true });
        // A failed close should move directly to cleanup and dismiss the tab.
        await page.route('**/terminal-sessions/*/close', async (route) => {
          await route.fulfill({ status: 502, json: { code: 'terminal_close_failed', title: 'Mock offline' } });
        });
        await page.route('**/terminal-sessions/*/detach', route => route.fulfill({ status: 200, json: { detached: true } }));
        const before = await page.locator('.terminal-tab').count();
        const detached = page.waitForResponse((response) => response.url().endsWith('/detach') && response.ok());
        await page.locator('.terminal-tab__close').last().click();
        await detached;
        await page.waitForFunction((n) => document.querySelectorAll('.terminal-tab').length === n, before - 1);
        assert.equal(await page.locator('.terminal-alert').count(), 0);
        await page.unroute('**/terminal-sessions/*/close');
        await page.unroute('**/terminal-sessions/*/detach');
        result.cases.push({ width, theme, zoom, language, mode, count, ...metrics });
      } catch (error) {
        await page.screenshot({ path: path.join(out, 'failure.png'), fullPage: true });
        console.error(await page.locator('.terminal-alert').allTextContents());
        console.error(requests.filter((request) => request.url.endsWith('/close')));
        throw error;
      } finally {
        for (let remaining = 4; remaining > 0 && await page.locator('.terminal-tab__close').count(); remaining--) {
          const n = await page.locator('.terminal-tab__close').count();
          await page.locator('.terminal-tab__close').last().click();
          await page.waitForFunction((n) => document.querySelectorAll('.terminal-tab__close').length < n, n);
        }
        await context.close();
      }
    }
    assert.deepEqual(result.errors, []);
    result.passed = true;
  } catch (error) { result.failure = error.stack; throw error; }
  finally {
    await browser.close();
    result.cleanup = true;
    fs.writeFileSync(path.join(out, 'result.json'), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });

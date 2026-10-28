// Real HostTerminal/xterm with synthetic log writes; no shell or remote host access.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '../../..');
const out = process.env.FOLLOW_EVIDENCE || path.join(root, '.codex-tmp/terminal-follow');
fs.mkdirSync(out, { recursive: true });
(async () => {
  const deadline = setTimeout(() => { console.error('Terminal follow regression timed out'); process.exit(1); }, 90000);
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined });
  const result = { candidate: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(),
    mode: 'UI Mock; real HostTerminal and xterm; synthetic log output', cases: [], errors: [] };
  try {
    for (const width of [1440, 390]) {
      const page = await browser.newPage({ viewport: { width, height: 900 }, deviceScaleFactor: width === 1440 ? 1.25 : 1 });
      page.on('pageerror', error => result.errors.push(error.message));
      await page.goto(`${process.env.PREVIEW_URL}/terminal`);
      await page.locator('.xterm-screen').first().waitFor();
      const checks = await page.evaluate(async () => {
        const host = document.querySelector('.host-terminal');
        const term = host.__vueParentComponent.setupState.terminal;
        if (!term) throw new Error('HostTerminal instance unavailable');
        const viewport = host.querySelector('.xterm-viewport');
        const frame = () => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        // Same write/follow contract as useTerminalController, without executing a command.
        const write = data => new Promise(resolve => {
          const follow = term.buffer.active.baseY - term.buffer.active.viewportY <= 2;
          term.write(data, () => { if (follow) term.scrollToBottom(); resolve(); });
        });
        const state = () => ({ base: term.buffer.active.baseY, view: term.buffer.active.viewportY });
        const checks = [];
        const check = (name, condition) => { checks.push({ name, ...state(), passed: condition }); };
        await write('live log\r\n'.repeat(300)); await frame();
        for (let cycle = 0; cycle < 12; cycle++) {
          term.scrollLines(-80); await frame();
          const history = term.buffer.active.viewportY;
          await write('reading history\r\n'.repeat(30)); await frame();
          check(`history-${cycle}`, term.buffer.active.viewportY === history);
          // Return on the native rail and deliver a burst BEFORE its async scroll event.
          viewport.scrollTop = viewport.scrollHeight;
          await write('racing log\r\n'.repeat(50)); await frame();
          check(`return-${cycle}`, term.buffer.active.viewportY === term.buffer.active.baseY);
          await write('continued log\r\n'.repeat(60)); await frame();
          check(`follow-${cycle}`, term.buffer.active.viewportY === term.buffer.active.baseY);
          // The opposite race must preserve the user's move into history as well.
          viewport.scrollTop = Math.max(0, viewport.scrollTop - 800);
          await write('upward race\r\n'.repeat(30)); await frame();
          check(`upward-${cycle}`, term.buffer.active.baseY - term.buffer.active.viewportY > 2);
        }
        term.scrollToBottom(); await frame();
        await write('fill scrollback\r\n'.repeat(5500)); await frame();
        term.scrollLines(-80); await frame();
        viewport.scrollTop = viewport.scrollHeight;
        await write('full buffer race\r\n'.repeat(60)); await frame();
        check('full-buffer-return', term.buffer.active.viewportY === term.buffer.active.baseY);
        await write('\x1b[?1049hfull screen\r\n'); await frame();
        check('alternate-buffer', term.buffer.active.type === 'alternate' && viewport.scrollTop === 0);
        await write('\x1b[?1049l'); await frame();
        check('normal-buffer-return', term.buffer.active.type === 'normal' && term.buffer.active.viewportY === term.buffer.active.baseY);
        host.style.display = 'none'; await frame();
        await write('hidden log\r\n'.repeat(30)); await frame();
        host.style.display = ''; await frame();
        check('visible-again', term.buffer.active.viewportY === term.buffer.active.baseY);
        term.focus();
        check('keyboard-focus', document.activeElement === term.textarea);
        return checks;
      });
      result.cases.push({ width, checks });
      await page.screenshot({ path: path.join(out, `terminal-${width}.png`) });
      assert.ok(checks.every(check => check.passed), JSON.stringify(checks.filter(check => !check.passed)));
      const screen = await page.locator('.xterm-screen').first().boundingBox();
      await page.mouse.move(screen.x + 60, screen.y + 60);
      await page.mouse.wheel(0, -900);
      await page.waitForFunction(() => {
        const term = document.querySelector('.host-terminal').__vueParentComponent.setupState.terminal;
        return term.buffer.active.baseY > term.buffer.active.viewportY;
      });
      await page.mouse.wheel(0, 100000);
      await page.waitForFunction(() => {
        const term = document.querySelector('.host-terminal').__vueParentComponent.setupState.terminal;
        return term.buffer.active.baseY === term.buffer.active.viewportY;
      });
      result.cases.at(-1).wheelReturn = true;
      await page.close();
    }
    assert.deepEqual(result.errors, []);
  } finally {
    fs.writeFileSync(path.join(out, 'result.json'), JSON.stringify(result, null, 2));
    await browser.close();
    clearTimeout(deadline);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });

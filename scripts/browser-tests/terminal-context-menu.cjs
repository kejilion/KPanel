// UI-only fixture using the real modal, interactive terminal, xterm and styles.
// Run against local-feature-preview with KPANEL_PLAYWRIGHT_MODULE and PREVIEW_URL.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright');

const root = path.resolve(__dirname, '../..');
const out = path.resolve(process.env.TERMINAL_MENU_EVIDENCE || path.join(root, '.codex-tmp/terminal-menu-browser'));
const fixture = path.join(root, 'web/node_modules/.cache/terminal-menu-preview.html');
const sample = 'OpenList installation completed\r\nDemo output for clipboard verification\r\n';
const baseline = process.env.EXPECT_MENU_OBSCURED === '1';
fs.mkdirSync(out, { recursive: true });
fs.mkdirSync(path.dirname(fixture), { recursive: true });
fs.writeFileSync(fixture, `<!doctype html><html><head><meta charset="utf-8"></head>
<body><div id="app"></div><script type="module">
import { createApp, h, ref } from '/node_modules/.vite/deps/vue.js';
import ModalDialog from '/src/components/common/ModalDialog.vue';
import AppInteractiveTerminal from '/src/components/apps/AppInteractiveTerminal.vue';
import { initializeTheme } from '/src/stores/theme.ts';
import '/src/styles/main.css';
import '/src/styles/desktop.css';
const params = new URLSearchParams(location.search);
document.body.classList.toggle('desktop-mode-open', params.get('mode') !== 'classic');
localStorage.setItem('kejilion-panel-theme', params.get('theme') || 'dark');
initializeTheme();
document.documentElement.style.zoom = params.get('zoom') || '1';
const originalFetch = window.fetch.bind(window);
window.fetch = (input, init) => String(input).includes('/app-jobs/menu-demo/terminal')
  ? Promise.resolve(new Response(JSON.stringify({
    dataBase64: ${JSON.stringify(Buffer.from(sample).toString('base64'))},
    nextOffset: ${Buffer.byteLength(sample)}, finished: true, inputOpen: false,
  }), {headers: {'Content-Type': 'application/json'}}))
  : originalFetch(input, init);
createApp({ setup() {
  const open = ref(true);
  return () => [
    h('button', {onClick: () => open.value = true}, '打开模拟安装终端'),
    h(ModalDialog, {open: open.value, title: 'OpenList 安装进度 · 模拟数据',
      description: '真实 UI 组件；仅模拟已完成任务输出，不执行安装。', size: 'large',
      onClose: () => open.value = false}, {
      default: () => h(AppInteractiveTerminal, {jobId: 'menu-demo'}),
      footer: () => h('button', {onClick: () => open.value = false}, '关闭窗口'),
    }),
  ];
}}).mount('#app');
</script></body></html>`);

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KPANEL_BROWSER_EXECUTABLE || undefined });
  const result = { mode: 'mock UI, real components', baseline, cases: [], errors: [], cleanup: false };
  try {
    const matrix = baseline ? [[1280, 'dark', 'desktop', 1]] : [
      [1280, 'dark', 'desktop', 1], [1280, 'light', 'desktop', 1],
      [1280, 'dark', 'classic', 1], [768, 'light', 'desktop', 1.25],
      [390, 'dark', 'desktop', 1], [1280, 'light', 'desktop', 2],
    ];
    for (const [width, theme, mode, zoom] of matrix) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce',
        permissions: ['clipboard-read', 'clipboard-write'] });
      const page = await context.newPage();
      page.setDefaultTimeout(8000);
      page.on('pageerror', error => result.errors.push(error.message));
      await page.goto(`${process.env.PREVIEW_URL}/node_modules/.cache/terminal-menu-preview.html?mode=${mode}&theme=${theme}&zoom=${zoom}`);
      await page.locator('.interactive-terminal').getByText('已结束', { exact: true }).waitFor();
      const screen = page.locator('.interactive-terminal__screen');
      const menu = page.locator('.terminal-context-menu');
      await screen.click({ button: 'right', position: { x: 40, y: 40 } });
      await menu.waitFor();
      const hit = await menu.evaluate(element => {
        const r = element.getBoundingClientRect();
        const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        return { exposed: element.contains(top), topClass: top?.className,
          layer: getComputedStyle(element).zIndex,
          modalLayer: getComputedStyle(document.querySelector('.modal-backdrop')).zIndex };
      });
      assert.equal(hit.exposed, !baseline, JSON.stringify(hit));
      if (!baseline) {
        assert.equal(await menu.getByRole('menuitem', { name: /^粘贴/ }).isDisabled(), true);
        await menu.getByRole('menuitem', { name: '全选', exact: true }).click();
        await screen.click({ button: 'right', position: { x: 40, y: 40 } });
        await menu.getByRole('menuitem', { name: /^复制/ }).click();
        assert.match(await page.evaluate(() => navigator.clipboard.readText()), /Demo output for clipboard verification/);
        await screen.click({ button: 'right', position: { x: 40, y: 40 } });
        await page.keyboard.press('ArrowDown');
        await page.keyboard.press('Escape');
        assert.equal(await menu.count(), 0);
        assert.equal(await page.getByRole('dialog').count(), 1);
        if (zoom === 1) {
          await page.locator('.terminal-toolbar button').last().click();
          await screen.click({ button: 'right', position: { x: 40, y: 80 } });
          await menu.getByRole('menuitem', { name: '全选', exact: true }).click();
          await page.locator('.terminal-toolbar button').last().click();
        }
        await screen.click({ button: 'right', position: { x: 40, y: 40 } });
      }
      await page.screenshot({ path: path.join(out, `${width}-${theme}-${mode}-${zoom}.png`) });
      result.cases.push({ width, theme, mode, zoom, ...hit });
      await context.close();
    }
    assert.deepEqual(result.errors, []);
    result.passed = true;
  } catch (error) {
    result.failure = error.stack;
    throw error;
  } finally {
    await browser.close();
    result.cleanup = true;
    fs.writeFileSync(path.join(out, 'result.json'), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });

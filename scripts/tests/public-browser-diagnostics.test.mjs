import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';
import test from 'node:test';

const require = createRequire(import.meta.url);
const runner = new URL('../../packaging/tests/release-browser/public-browser-complete.cjs', import.meta.url);
const { createDiagnostics } = require(fileURLToPath(runner));
const base = 'http://127.0.0.1:8080';
const request = (url, method = 'GET') => ({ url: () => url, method: () => method,
  failure: () => ({ errorText: 'net::ERR_CONNECTION_RESET token=PRIVATE' }),
  headers() { throw new Error('must never read credentials'); }, postData() { throw new Error('must never read input'); } });

function fixture(count = 1) {
  const order = [];
  const pages = Array.from({ length: count }, () => Object.assign(new EventEmitter(), {
    url: () => base + '/terminal?token=PRIVATE#PRIVATE',
    async evaluate() { order.push('capture'); return [{ selector: '.terminal-tab__close', count: 0, visible: 0 }]; },
  }));
  const context = Object.assign(new EventEmitter(), { pages: () => pages });
  return { context, pages, order };
}

test('diagnostics retain bounded recent endpoint metadata and pending ages without secrets or DOM text', async () => {
  const { context, pages } = fixture(8);
  const diagnostics = createDiagnostics(context, base);
  diagnostics.stage('terminal-first-navigation');
  for (let i = 0; i < 200; i++) {
    const r = request(base + '/api/v1/auth/session?token=PRIVATE');
    context.emit('request', r); context.emit('response', { request: () => r, status: () => 200 });
  }
  const failed = request(base + '/api/v1/terminal-sessions/PRIVATE/output?offset=PRIVATE');
  context.emit('requestfailed', failed);
  context.emit('response', { request: () => request('https://PRIVATE.invalid/PRIVATE?password=PRIVATE'), status: () => 502 });
  diagnostics.api('/settings/kpanel-release?channel=preview&token=PRIVATE', 'GET', 502);
  const result = await diagnostics.capture();
  assert.equal(diagnostics.data.events.length, 128);
  assert.equal(result.pending.length, 32);
  assert.equal(result.pages.length, 4);
  assert.equal(result.omittedPages, 4);
  assert.ok(diagnostics.data.droppedEvents > 0 && diagnostics.data.untrackedRequests > 0);
  assert.ok(result.pending.every(r => Number.isFinite(r.ageMs) && r.stage === 'terminal-first-navigation'));
  assert.equal(diagnostics.data.events.at(-1).status, 502);
  assert.equal(diagnostics.data.events.at(-3).route, '/api/v1/terminal-sessions/:id/output');
  assert.equal(diagnostics.data.events.at(-3).code, 'net::ERR_CONNECTION_RESET');
  assert.doesNotMatch(JSON.stringify({ data: diagnostics.data, result }), /PRIVATE|password=|token=|headers|cookie/i);
  // Dynamic future tabs are observed too; no page was created or navigated by the collector.
  const newPage = Object.assign(new EventEmitter(), { url: pages[0].url });
  context.emit('page', newPage); newPage.emit('pageerror', new Error('PRIVATE'));
  assert.equal(diagnostics.data.events.at(-1).kind, 'pageerror');
});

test('completed and failed requests leave the pending set; capture failures have a hard deadline', async () => {
  const { context, pages } = fixture();
  pages[0].evaluate = () => new Promise(() => {});
  const diagnostics = createDiagnostics(context, base, { captureTimeoutMs: 20 });
  const done = request(base + '/api/v1/cluster/hosts');
  const failed = request(base + '/assets/app.js?token=PRIVATE');
  context.emit('request', done); context.emit('request', failed);
  context.emit('requestfinished', done); context.emit('requestfailed', failed);
  const result = await diagnostics.capture();
  assert.equal(result.pending.length, 0);
  assert.equal(result.pages[0].status, 'unavailable');
  assert.equal(diagnostics.data.events[0].route, '/assets/*');
});

test('DOM sampling exports only bounded marker counts even when page text contains secrets', async () => {
  const { context, pages } = fixture();
  pages[0].evaluate = callback => runInNewContext('(' + callback.toString() + ')()', { document: {
    querySelectorAll() { return Array.from({ length: 200 }, () => ({
      get innerText() { throw new Error('must not export page text'); },
      get value() { throw new Error('must not export form input'); },
      getClientRects: () => [1],
    })); },
  } });
  const result = await createDiagnostics(context, base).capture();
  assert.equal(result.pages[0].status, 'captured');
  assert.equal(result.pages[0].state.length, 7);
  assert.ok(result.pages[0].state.every(marker => marker.count === 32 && marker.visible === 32));
});

test('diagnostics identify the recovery card produced by the real bootstrap failure handler', async () => {
  const nodes = [];
  let showFailure;
  const document = {
    body: { append(node) { nodes.push(node); } },
    createElement() { return { style: {}, attributes: {},
      setAttribute(key, value) { this.attributes[key] = value; },
      append() {}, addEventListener() {}, getClientRects() { return [1]; },
    }; },
    querySelectorAll(selector) {
      if (selector.startsWith('#')) return nodes.filter(node => node.id === selector.slice(1));
      if (selector === '[role="alert"]') return nodes.filter(node => node.attributes.role === 'alert');
      return [];
    },
  };
  runInNewContext(readFileSync(new URL('../../web/public/bootstrap-recovery.js', import.meta.url), 'utf8'), {
    document, localStorage: { getItem: () => 'zh-CN' },
    window: { addEventListener(name, handler) { if (name === 'kpanel:boot-failed') showFailure = handler; }, removeEventListener() {} },
    setTimeout() { return 1; }, clearTimeout() {},
  });
  showFailure();
  const { context, pages } = fixture();
  pages[0].evaluate = callback => runInNewContext('(' + callback.toString() + ')()', { document });
  const result = await createDiagnostics(context, base).capture();
  const marker = result.pages[0].state.find(entry => entry.selector === '#bootstrap-recovery');
  assert.equal(marker?.count, 1);
  assert.equal(marker.visible, 1);
});

async function navigationFailure({ captureFails = false, closeFails = false } = {}) {
  const { context, pages, order } = fixture();
  if (captureFails) pages[0].evaluate = async () => { order.push('capture'); throw new Error('capture failed'); };
  let written;
  const secret = 'PRIVATE_BOOTSTRAP_TOKEN';
  const response = (status, body) => ({ status: () => status, ok: () => status < 400, json: async () => body });
  context.addInitScript = async () => {};
  context.request = {
    post: async () => response(201, { csrfToken: secret }),
    async fetch(url) {
      if (new URL(url).pathname === '/api/v1/terminal-sessions') return response(200, { retained: true, sessionId: 'fixture-id' });
      if (new URL(url).pathname.endsWith('/output')) return response(200, { data: Buffer.from('__KPANEL_RC127_PTY__').toString('base64'), nextOffset: 1 });
      return response(200, {});
    },
  };
  context.newPage = async () => {
    context.emit('page', pages[0]);
    pages[0].goto = async () => {
      const r = request(base + '/api/v1/auth/session?token=' + secret);
      context.emit('response', { request: () => r, status: () => 503 });
      throw new Error('intentional first-navigation failure');
    };
    return pages[0];
  };
  const module = { exports: {} };
  const fakeRequire = name => {
    if (name === '/work/runtime/node_modules/playwright') return { chromium: { launch: async () => ({
      newContext: async () => context, close: async () => { order.push('close'); if (closeFails) throw new Error('close failed'); },
    }) } };
    if (name === 'node:fs') return { readFileSync: () => secret, writeFileSync(path, text) { written = JSON.parse(text); } };
    return require(name);
  };
  fakeRequire.main = module;
  const process = { argv: ['node', 'runner', '8080', 'published', 'sha256:fixture'], env: {
    EXPECTED_VERSION: '1.29.0-rc.5', EXPECTED_SHA: 'a'.repeat(40),
  } };
  await runInNewContext(readFileSync(runner, 'utf8'), {
    require: fakeRequire, module, process, Buffer, URL, setTimeout, clearTimeout, console: { log() {} },
  });
  assert.equal(process.exitCode, 1);
  assert.equal(written.status, 'failed');
  assert.match(written.error, /intentional first-navigation failure/);
  assert.equal(written.diagnostics.failure.stage, 'terminal-first-navigation');
  assert.equal(written.diagnostics.events.at(-1).status, 503);
  assert.equal(written.candidate, 'a'.repeat(40));
  assert.deepEqual(order, ['capture', 'close']);
  assert.doesNotMatch(JSON.stringify(written.diagnostics), /PRIVATE/);
  assert.ok(written.checks.includes('real PTY execution output marker'));
  return written;
}

test('runner navigation failure captures before close and preserves the original failure exit', () => navigationFailure());

test('diagnostic and browser-close failures cannot replace the original navigation failure', async () => {
  const report = await navigationFailure({ captureFails: true, closeFails: true });
  assert.equal(report.diagnostics.failure.pages[0].status, 'unavailable');
  assert.equal(report.browserCloseFailed, true);
});

import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

import { checkDistFiles, checkEntryHTML, checkManifest, checkScenePacks, LIMITS } from '../check-scene-packs.mjs';

const repoRoot = resolve(import.meta.dirname, '..', '..');

function manifest(overrides = {}) {
  return {
    schema: 1,
    id: 'neon-city',
    version: '1.0.0',
    runtime: 'kpanel-scene-pack@1',
    name: { 'zh-CN': '霓虹都市', 'en-US': 'Neon City' },
    description: { 'zh-CN': '雨夜街道车流不息', 'en-US': 'Traffic streams through a rainy neon night' },
    author: { name: 'Someone', url: 'https://github.com/someone' },
    license: 'CC-BY-4.0',
    tags: ['city', 'night'],
    entry: 'index.html',
    poster: 'poster.webp',
    thumb: 'thumb.webp',
    theme: { brand: '#356fc0', neutral: '#34465c', signature: '#23a6bd' },
    cameras: [{ id: 'street', name: { 'zh-CN': '街道', 'en-US': 'Street level' } }],
    ...overrides,
  };
}

function distFiles(extra = []) {
  return [
    { path: 'index.html', size: 200, symlink: false },
    { path: 'manifest.json', size: 900, symlink: false },
    { path: 'poster.webp', size: 50_000, symlink: false },
    { path: 'thumb.webp', size: 6_000, symlink: false },
    { path: 'scene.js', size: 600_000, symlink: false },
    ...extra,
  ];
}

test('the repository scene packs follow their own development rules', async () => {
  assert.deepEqual(await checkScenePacks(join(repoRoot, 'scene-packs')), []);
});

test('manifests accept any author, license and style within the published limits', () => {
  assert.deepEqual(checkManifest(manifest(), 'neon-city'), []);
  assert.deepEqual(checkManifest(manifest({ license: 'MIT OR Apache-2.0', dependencies: [{ name: 'babylonjs', version: '8.1.0' }] }), 'neon-city'), []);
});

test('manifests reject identity, locale and metadata mistakes', () => {
  assert.match(checkManifest(manifest(), 'other-dir').join('\n'), /match the directory/);
  assert.match(checkManifest(manifest({ id: 'Neon_City' }), 'Neon_City').join('\n'), /lowercase/);
  assert.match(checkManifest(manifest({ name: { 'zh-CN': '霓虹都市' } }), 'neon-city').join('\n'), /name\.en-US is required/);
  assert.match(checkManifest(manifest({ name: { 'zh-CN': '霓'.repeat(21), 'en-US': 'Neon' } }), 'neon-city').join('\n'), /name\.zh-CN must be at most 20/);
  assert.deepEqual(checkManifest(manifest({ description: { 'zh-CN': '夜', 'en-US': 'x'.repeat(90) } }), 'neon-city'), []);
  assert.match(checkManifest(manifest({ description: { 'zh-CN': '夜', 'en-US': 'x'.repeat(91) } }), 'neon-city').join('\n'), /description\.en-US must be at most 90/);
  assert.match(checkManifest(manifest({ author: { name: 'x', url: 'http://example.com' } }), 'neon-city').join('\n'), /https/);
  assert.match(checkManifest(manifest({ license: 'see LICENSE file' }), 'neon-city').join('\n'), /SPDX/);
  assert.match(checkManifest(manifest({ runtime: 'kpanel-scene-pack@2' }), 'neon-city').join('\n'), /runtime/);
  assert.match(checkManifest(manifest({ entry: '../index.html' }), 'neon-city').join('\n'), /entry, poster and thumb/);
  assert.match(checkManifest(manifest({ dependencies: [{ name: 'three', version: 'latest' }] }), 'neon-city').join('\n'), /exact version/);
});

test('each scene sets one panel color scheme with plain hex values', () => {
  assert.match(checkManifest(manifest({ theme: undefined }), 'neon-city').join('\n'), /theme must set brand, neutral and signature/);
  assert.match(checkManifest(manifest({ theme: { brand: 'red', neutral: '#34465c', signature: '#23a6bd' } }), 'neon-city').join('\n'), /theme must set/);
  assert.match(checkManifest(manifest({ theme: { brand: 'url(x)', neutral: '#34465c', signature: '#23a6bd' } }), 'neon-city').join('\n'), /theme must set/);
  // Cameras cannot carry their own scheme: the theme does not change with the shot.
  const warm = { brand: '#c07a2c', neutral: '#4a4038', signature: '#e8b25c' };
  assert.match(checkManifest(manifest({ cameras: [{ id: 'sunset', name: { 'zh-CN': '日落', 'en-US': 'Sunset' }, theme: warm }] }), 'neon-city').join('\n'), /cameras\[0\]\.theme is not supported/);
});

test('camera lists stay between one and six named shots', () => {
  assert.match(checkManifest(manifest({ cameras: [] }), 'neon-city').join('\n'), /cameras must list 1-6/);
  const seven = Array.from({ length: LIMITS.cameras + 1 }, (_, index) => ({ id: `shot-${index}`, name: { 'zh-CN': '机位', 'en-US': 'Shot' } }));
  assert.match(checkManifest(manifest({ cameras: seven }), 'neon-city').join('\n'), /cameras must list 1-6/);
  assert.match(checkManifest(manifest({ cameras: [{ id: 'Street View', name: { 'zh-CN': '街道', 'en-US': 'Street' } }] }), 'neon-city').join('\n'), /cameras\[0\]\.id/);
});

test('published files are limited to web content types, sizes and plain paths', () => {
  assert.deepEqual(checkDistFiles(distFiles([{ path: 'models/city.glb', size: 4_000_000, symlink: false }, { path: 'engine.wasm', size: 2_000_000, symlink: false }])), []);
  const joined = (extra) => checkDistFiles(distFiles(extra)).join('\n');
  assert.match(joined([{ path: 'payload.exe', size: 10, symlink: false }]), /\.exe files are not allowed/);
  assert.match(joined([{ path: 'logo.svg', size: 10, symlink: false }]), /\.svg files are not allowed/);
  assert.match(joined([{ path: 'link.js', size: 0, symlink: true }]), /symbolic links/);
  assert.match(joined([{ path: 'Textures/Sky.webp', size: 10, symlink: false }]), /lowercase names/);
  assert.match(joined([{ path: '.hidden/a.js', size: 10, symlink: false }]), /hidden segments/);
  assert.match(joined([{ path: 'a/b/c/d/e.js', size: 10, symlink: false }]), /at most 4 levels/);
  assert.match(joined([{ path: 'huge.bin', size: LIMITS.fileBytes + 1, symlink: false }]), /larger than/);
  assert.match(checkDistFiles(distFiles().filter((file) => file.path !== 'thumb.webp')).join('\n'), /dist\/thumb\.webp is required/);
  assert.match(checkDistFiles(distFiles().map((file) => file.path === 'poster.webp' ? { ...file, size: LIMITS.posterBytes + 1 } : file)).join('\n'), /poster\.webp is \d+ bytes/);
  const many = Array.from({ length: LIMITS.files }, (_, index) => ({ path: `chunks/${index}.js`, size: 1, symlink: false }));
  assert.match(checkDistFiles(distFiles(many)).join('\n'), /the limit is 200/);
  assert.match(checkDistFiles(distFiles([{ path: 'a.bin', size: 16_000_000, symlink: false }, { path: 'b.bin', size: 16_000_000, symlink: false }])).join('\n'), /dist is \d+ bytes/);
});

test('entry pages must load bundled external scripts only', () => {
  assert.deepEqual(checkEntryHTML('<!doctype html><script src="./scene.js"></script>'), []);
  assert.match(checkEntryHTML('<script>alert(1)</script>').join('\n'), /no inline code/);
  assert.match(checkEntryHTML('<script src="./scene.js">run()</script>').join('\n'), /no inline code/);
  assert.match(checkEntryHTML('<script src="https://cdn.example.com/three.js"></script>').join('\n'), /remote URLs/);
  assert.match(checkEntryHTML('<link rel="stylesheet" href="//fonts.example.com/a.css">').join('\n'), /remote URLs/);
});

function writePack(root, id, overrides = {}) {
  const packRoot = join(root, id);
  const dist = join(packRoot, 'dist');
  mkdirSync(join(packRoot, 'src'), { recursive: true });
  mkdirSync(dist, { recursive: true });
  const body = manifest({ id, ...overrides });
  writeFileSync(join(packRoot, 'manifest.json'), JSON.stringify(body));
  writeFileSync(join(packRoot, 'src', 'main.js'), 'export {}\n');
  const files = {
    'index.html': '<script src="./scene.js"></script>',
    'manifest.json': JSON.stringify(body),
    'poster.webp': 'poster',
    'scene.js': 'void 0\n',
    'thumb.webp': 'thumb',
  };
  for (const [name, content] of Object.entries(files)) writeFileSync(join(dist, name), content);
  return { ...body, path: `${id}/dist`, files: Object.entries(files).map(([path, content]) => ({ path, size: Buffer.byteLength(content), sha256: createHash('sha256').update(content).digest('hex') })) };
}

test('the catalog must pin every published file of every pack', async () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-scene-packs-test-'));
  try {
    const entry = writePack(root, 'neon-city');
    mkdirSync(join(root, '_template'));
    writeFileSync(join(root, 'catalog.json'), JSON.stringify({ schema: 1, packs: [entry] }));
    assert.deepEqual(await checkScenePacks(root), []);

    writeFileSync(join(root, 'neon-city', 'dist', 'scene.js'), 'fetch("https://evil.example")\n');
    assert.match((await checkScenePacks(root)).join('\n'), /catalog\.json is out of date/);

    writeFileSync(join(root, 'catalog.json'), JSON.stringify({ schema: 1, packs: [entry, { ...entry, id: 'ghost' }] }));
    assert.match((await checkScenePacks(root)).join('\n'), /lists ghost, which has no pack directory/);

    writeFileSync(join(root, 'neon-city', 'dist', 'manifest.json'), JSON.stringify(manifest({ id: 'neon-city', version: '9.9.9' })));
    assert.match((await checkScenePacks(root)).join('\n'), /dist\/manifest\.json differs/);

    rmSync(join(root, 'neon-city', 'src'), { recursive: true });
    assert.match((await checkScenePacks(root)).join('\n'), /src\/ with readable source is required/);

    writeFileSync(join(root, 'catalog.json'), '{');
    assert.match((await checkScenePacks(root)).join('\n'), /catalog\.json is missing or not valid JSON/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('the official build reproduces dist from src and the gate runs it for changed packs', () => {
  const script = readFileSync(join(repoRoot, 'scripts', 'verify-change.sh'), 'utf8');
  assert.match(script, /scene-packs\/\*\/\*\)/);
  assert.match(script, /npm run scene-packs:check -- "\$\{scene_pack_ids\[@\]\}"/);
  assert.match(script, /node scripts\/check-scene-packs\.mjs/);
  const packageJSON = JSON.parse(readFileSync(join(repoRoot, 'web', 'package.json'), 'utf8'));
  assert.match(packageJSON.scripts['scene-packs:check'], /build-scene-packs\.mjs --check/);
});

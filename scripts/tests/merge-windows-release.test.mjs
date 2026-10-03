import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { mergeWindowsRelease, windowsNodeAssets } from '../merge-windows-release.mjs';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-windows-release-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const source = join(root, 'signed');
  const destination = join(root, 'release');
  mkdirSync(source);
  mkdirSync(destination);
  const lines = windowsNodeAssets.map(name => {
    const bytes = Buffer.from('signed bytes ' + name);
    writeFileSync(join(source, name), bytes);
    return createHash('sha256').update(bytes).digest('hex') + '  ' + name;
  });
  const manifest = lines.join('\n') + '\n';
  writeFileSync(join(source, 'SHA256SUMS.windows'), manifest);
  const linux = 'a'.repeat(64) + '  kejilion-node-linux-amd64\n';
  writeFileSync(join(destination, 'SHA256SUMS'), linux);
  return { source, destination, manifest, linux };
}

test('combines only verified signed bytes without altering Linux checksums', t => {
  const f = fixture(t);
  mergeWindowsRelease(f.source, f.destination);
  assert.equal(readFileSync(join(f.destination, 'SHA256SUMS'), 'utf8'), f.linux + f.manifest);
  for (const name of windowsNodeAssets) assert.deepEqual(readFileSync(join(f.source, name)), readFileSync(join(f.destination, name)));
  assert.throws(() => mergeWindowsRelease(f.source, f.destination), /already merged/);
});

test('rejects tampering, missing entries, traversal, duplicates and unexpected assets', async t => {
  for (const scenario of ['tamper', 'missing', 'traversal', 'duplicate', 'extra']) {
    await t.test(scenario, t => {
      const f = fixture(t);
      if (scenario === 'tamper') writeFileSync(join(f.source, windowsNodeAssets[0]), 'altered after signing');
      if (scenario === 'extra') writeFileSync(join(f.source, 'unsigned.exe'), 'must not be published');
      if (scenario === 'missing') writeFileSync(join(f.source, 'SHA256SUMS.windows'), f.manifest.split('\n').slice(1).join('\n'));
      if (scenario === 'traversal') writeFileSync(join(f.source, 'SHA256SUMS.windows'), f.manifest.replace(windowsNodeAssets[0], '../' + windowsNodeAssets[0]));
      if (scenario === 'duplicate') writeFileSync(join(f.source, 'SHA256SUMS.windows'), f.manifest + f.manifest.split('\n')[0] + '\n');
      assert.throws(() => mergeWindowsRelease(f.source, f.destination));
      assert.equal(readFileSync(join(f.destination, 'SHA256SUMS'), 'utf8'), f.linux);
    });
  }
});

test('does not overwrite an existing Windows artifact', t => {
  const f = fixture(t);
  writeFileSync(join(f.destination, windowsNodeAssets[0]), 'existing');
  assert.throws(() => mergeWindowsRelease(f.source, f.destination), /EEXIST/);
  assert.equal(readFileSync(join(f.destination, windowsNodeAssets[0]), 'utf8'), 'existing');
  assert.equal(readFileSync(join(f.destination, 'SHA256SUMS'), 'utf8'), f.linux);
});

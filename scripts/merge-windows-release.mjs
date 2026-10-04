import { createHash } from 'node:crypto';
import { copyFileSync, lstatSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { basename, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const windowsNodeAssets = ['kejilion-node-windows-amd64.exe', 'kejilion-node-windows-arm64.exe', 'install-windows.ps1'];

export function mergeWindowsRelease(source, destination) {
  const manifestName = 'SHA256SUMS.windows';
  const expected = [...windowsNodeAssets, manifestName].sort();
  for (const directory of [source, destination]) {
    if (!lstatSync(directory).isDirectory() || lstatSync(directory).isSymbolicLink()) throw new Error('Release paths must be regular directories');
  }
  if (JSON.stringify(readdirSync(source).sort()) !== JSON.stringify(expected)) throw new Error('Unexpected Windows artifact contents');
  for (const name of expected) {
    const info = lstatSync(resolve(source, name));
    if (!info.isFile() || info.isSymbolicLink() || info.size === 0 || info.size > (name === manifestName ? 2048 : 256 * 1024 * 1024)) throw new Error('Invalid Windows artifact file');
  }
  const manifest = readFileSync(resolve(source, manifestName), 'utf8');
  const lines = manifest.trimEnd().split('\n');
  const hashes = new Map();
  for (const line of lines) {
    const match = /^([0-9a-f]{64})  ([A-Za-z0-9.-]+)$/.exec(line);
    if (!match || !windowsNodeAssets.includes(match[2]) || hashes.has(match[2])) throw new Error('Invalid Windows checksum manifest');
    hashes.set(match[2], match[1]);
  }
  if (hashes.size !== windowsNodeAssets.length) throw new Error('Incomplete Windows checksum manifest');
  for (const name of windowsNodeAssets) {
    const digest = createHash('sha256').update(readFileSync(resolve(source, name))).digest('hex');
    if (digest !== hashes.get(name)) throw new Error('Windows artifact checksum mismatch: ' + name);
  }
  const combinedPath = resolve(destination, 'SHA256SUMS');
  const combined = readFileSync(combinedPath, 'utf8');
  if (!combined.endsWith('\n') || combined.split('\n').some(line => windowsNodeAssets.includes(line.slice(66)))) throw new Error('Windows checksums already merged or invalid base manifest');
  for (const name of windowsNodeAssets) {
    // COPYFILE_EXCL prevents silently replacing a different product artifact.
    copyFileSync(resolve(source, name), resolve(destination, name), 1);
  }
  writeFileSync(combinedPath, combined + manifest.trimEnd() + '\n');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.length !== 4) throw new Error('Usage: ' + basename(process.argv[1]) + ' <artifact-directory> <release-directory>');
  mergeWindowsRelease(resolve(process.argv[2]), resolve(process.argv[3]));
}

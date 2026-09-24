// The pending scene-pack host supplies Vite and Three.js; --web-root also allows
// this independent asset branch to build before the host branch is merged.
import { copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { tmpdir } from 'node:os'

const packRoot = dirname(fileURLToPath(import.meta.url))
const check = process.argv.includes('--check')
const webRoot = resolve(process.argv.slice(2).find(arg => arg !== '--check') || join(packRoot, '../../web'))
const scratch = check ? await mkdtemp(join(tmpdir(), 'celestial-palace-build-')) : null
const outDir = scratch || join(packRoot, 'dist')
const { build } = await import(pathToFileURL(join(webRoot, 'node_modules/vite/dist/node/index.js')).href)
try {
await build({
  configFile: false, root: packRoot, logLevel: 'warn', publicDir: false,
  resolve: { alias: { three: join(webRoot, 'node_modules/three') } },
  build: {
    outDir, emptyOutDir: true, target: 'es2021', minify: true,
    reportCompressedSize: false,
    lib: { entry: join(packRoot, 'src/palace.ts'), formats: ['iife'], name: 'KPanelScenePack', fileName: () => 'scene.js' },
  },
})
await mkdir(outDir, { recursive: true })
for (const file of ['index.html', 'manifest.json', 'poster.webp', 'thumb.webp', 'license.txt']) {
  await copyFile(join(packRoot, file), join(outDir, file))
}
const manifest = JSON.parse(await readFile(join(packRoot, 'manifest.json'), 'utf8'))
const files = []
for (const path of (await readdir(outDir)).sort()) {
  const bytes = await readFile(join(outDir, path))
  files.push({ path, size: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex') })
}
// A single-pack catalog can be merged by the standard repository build later.
const catalog = JSON.stringify({ ...manifest, path: `${manifest.id}/dist`, files, sizeBytes: files.reduce((sum, f) => sum + f.size, 0) }, null, 2) + '\n'
if (check) {
  const existing = (await readdir(join(packRoot, 'dist'))).sort()
  if (existing.join('\n') !== files.map(f => f.path).join('\n')) throw new Error('dist file list differs from source build')
  for (const file of files) {
    const current = await readFile(join(packRoot, 'dist', file.path))
    if (createHash('sha256').update(current).digest('hex') !== file.sha256) throw new Error(`stale artifact: ${file.path}`)
  }
  if ((await readFile(join(packRoot, 'catalog-entry.json'), 'utf8')).replace(/\r\n/g, '\n') !== catalog) throw new Error('stale catalog-entry.json')
  console.log(`${manifest.id}: source, published files and hashes match`)
} else {
  await writeFile(join(packRoot, 'catalog-entry.json'), catalog)
  console.log(`built ${manifest.id}: ${files.length} files`)
}
} finally {
  if (scratch) await rm(scratch, { recursive: true, force: true })
}

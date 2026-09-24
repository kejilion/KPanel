// Vite and Three.js come from the existing KPanel web toolchain.
import { copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { tmpdir } from 'node:os'

const packRoot = dirname(fileURLToPath(import.meta.url))
const check = process.argv.includes('--check')
const webRoot = resolve(process.argv.slice(2).find(arg => arg !== '--check') || join(packRoot, '../../web'))
const model = await readFile(join(packRoot, 'assets/scene.glb'))
if (model.length < 20 || model.readUInt32LE(0) !== 0x46546c67 || model.readUInt32LE(4) !== 2 || model.readUInt32LE(8) !== model.length) {
  throw new Error('assets/scene.glb must be a complete glTF 2.0 binary')
}
if (model.readUInt32LE(16) !== 0x4e4f534a) throw new Error('The first GLB chunk must contain its JSON document')
const document = JSON.parse(model.subarray(20, 20 + model.readUInt32LE(12)).toString('utf8'))
for (const resource of [...(document.buffers || []), ...(document.images || [])]) {
  if (resource.uri !== undefined) throw new Error('The GLB must embed every buffer and image without external URIs')
}
const cameras = JSON.parse(await readFile(join(packRoot, 'assets/cameras.json'), 'utf8'))
if (!Array.isArray(cameras) || cameras.length !== 3 || cameras.some(camera =>
  !camera || ![camera.position, camera.target].every(vector => Array.isArray(vector) && vector.length === 3 && vector.every(Number.isFinite)) ||
  !Number.isFinite(camera.fov) || camera.fov <= 0 || camera.fov >= 150
)) throw new Error('assets/cameras.json must contain three finite Three.js camera poses')

const scratch = check ? await mkdtemp(join(tmpdir(), 'abyssal-ruins-build-')) : null
const outDir = scratch || join(packRoot, 'dist')
try {
  const { build } = await import(pathToFileURL(join(webRoot, 'node_modules/vite/dist/node/index.js')).href)
  await build({
    configFile: false, root: packRoot, logLevel: 'warn', publicDir: false,
    resolve: { alias: { three: join(webRoot, 'node_modules/three') } },
    define: { __ABYSS_GLB__: JSON.stringify(model.toString('base64')), __ABYSS_CAMERAS__: JSON.stringify(cameras) },
    build: {
      outDir, emptyOutDir: true, target: 'es2021', minify: true,
      reportCompressedSize: false,
      lib: { entry: join(packRoot, 'src/runtime.ts'), formats: ['iife'], name: 'KPanelScenePack', fileName: () => 'scene.js' },
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
  const catalog = JSON.stringify({ ...manifest, path: `${manifest.id}/dist`, files, sizeBytes: files.reduce((sum, file) => sum + file.size, 0) }, null, 2) + '\n'
  if (check) {
    const existing = (await readdir(join(packRoot, 'dist'))).sort()
    if (existing.join('\n') !== files.map(file => file.path).join('\n')) throw new Error('dist file list differs from source build')
    for (const file of files) {
      const current = await readFile(join(packRoot, 'dist', file.path))
      if (createHash('sha256').update(current).digest('hex') !== file.sha256) throw new Error(`stale artifact: ${file.path}`)
    }
    if ((await readFile(join(packRoot, 'catalog-entry.json'), 'utf8')).replace(/\r\n/g, '\n') !== catalog) throw new Error('stale catalog-entry.json')
    console.log(`${manifest.id}: source, embedded GLB, published files and hashes match`)
  } else {
    await writeFile(join(packRoot, 'catalog-entry.json'), catalog)
    console.log(`built ${manifest.id}: ${files.length} files, embedded model ${model.length} bytes`)
  }
} finally {
  if (scratch) await rm(scratch, { recursive: true, force: true })
}

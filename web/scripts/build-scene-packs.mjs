// Builds the desktop 3D scene packs in <repo>/scene-packs and writes the catalog
// the panel downloads from the repository (GitHub raw, or the gh.kejilion.pro mirror).
//
//   node scripts/build-scene-packs.mjs            build every pack with a src/main.* entry
//   node scripts/build-scene-packs.mjs <id>...    build only these packs
//   node scripts/build-scene-packs.mjs --check [<id>...]
//                                                 rebuild into a temporary directory and fail
//                                                 unless dist/ and the catalog match byte for byte
//
// Packs built with the standard toolchain get src/main.(ts|js) bundled into a
// single classic script dist/scene.js (Three.js from this workspace included);
// an assets/ directory (models, textures, data the scene loads at run time) is
// copied to dist/assets/ as it is.
// Packs built with their author's own toolchain only commit dist/; they are
// hashed into the catalog as they are. The catalog pins size and SHA-256 of
// every published file so the panel can verify downloads. The check mode is what
// lets reviewers trust that the dist/ the panel runs is exactly the src/ they read.
import { createHash } from 'node:crypto'
import { access, copyFile, mkdir, mkdtemp, readdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const packsRoot = resolve(webRoot, '..', 'scene-packs')
const PACK_ID = /^[a-z0-9][a-z0-9-]{0,39}$/
const PUBLISHED_ASSETS = ['index.html', 'manifest.json', 'poster.webp', 'thumb.webp', 'preview.webm']
const checkOnly = process.argv.includes('--check')
const requested = new Set(process.argv.slice(2).filter((arg) => arg !== '--check'))

async function exists(path) {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

async function listFiles(root, directory = root) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await listFiles(root, path))
    else if (entry.isFile()) files.push(relative(root, path).split(sep).join('/'))
  }
  return files.sort()
}

async function buildPack(packRoot, id, outDir) {
  const entry = ['src/main.ts', 'src/main.js'].map((name) => join(packRoot, name))
  const source = (await Promise.all(entry.map(exists))).findIndex(Boolean)
  if (source < 0) return false
  await rm(outDir, { recursive: true, force: true })
  await build({
    configFile: false,
    root: packRoot,
    logLevel: 'warn',
    publicDir: false,
    resolve: { alias: { three: join(webRoot, 'node_modules', 'three') } },
    build: {
      outDir,
      emptyOutDir: true,
      target: 'es2021',
      minify: true,
      reportCompressedSize: false,
      lib: { entry: entry[source], formats: ['iife'], name: 'KPanelScenePack', fileName: () => 'scene.js' },
    },
  })
  await mkdir(outDir, { recursive: true })
  for (const name of PUBLISHED_ASSETS) {
    if (await exists(join(packRoot, name))) await copyFile(join(packRoot, name), join(outDir, name))
  }
  if (await exists(join(packRoot, 'assets'))) {
    for (const path of await listFiles(join(packRoot, 'assets'))) {
      await mkdir(dirname(join(outDir, 'assets', path)), { recursive: true })
      await copyFile(join(packRoot, 'assets', path), join(outDir, 'assets', path))
    }
  }
  if (!checkOnly) console.log(`built ${id}`)
  return true
}

async function sameFiles(expectedDir, actualDir) {
  const [expected, actual] = await Promise.all([listFiles(expectedDir), listFiles(actualDir)])
  if (expected.join('\n') !== actual.join('\n')) return false
  for (const path of expected) {
    const [a, b] = await Promise.all([readFile(join(expectedDir, path)), readFile(join(actualDir, path))])
    if (!a.equals(b)) return false
  }
  return true
}

const problems = []
const scratch = checkOnly ? await mkdtemp(join(tmpdir(), 'kpanel-scene-packs-')) : ''
const packs = []
try {
  for (const entry of (await readdir(packsRoot, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
    if (!entry.isDirectory() || entry.name.startsWith('_') || entry.name.startsWith('.')) continue
    const packRoot = join(packsRoot, entry.name)
    const manifest = JSON.parse(await readFile(join(packRoot, 'manifest.json'), 'utf8'))
    if (!PACK_ID.test(manifest.id) || manifest.id !== entry.name) throw new Error(`pack id must match its directory: ${entry.name}`)
    const dist = join(packRoot, 'dist')
    const selected = !requested.size || requested.has(manifest.id)
    if (checkOnly && selected) {
      const rebuilt = join(scratch, manifest.id)
      if (await buildPack(packRoot, manifest.id, rebuilt) && !await sameFiles(rebuilt, dist).catch(() => false)) {
        problems.push(`${manifest.id}: dist/ is not the build of src/; run npm --prefix web run scene-packs:build`)
      }
    } else if (!checkOnly && selected) {
      await buildPack(packRoot, manifest.id, dist)
    }
    if (!await exists(dist)) throw new Error(`${manifest.id}: dist/ is missing; build it or commit your own build`)
    const files = []
    for (const path of await listFiles(dist)) {
      const body = await readFile(join(dist, path))
      files.push({ path, size: body.length, sha256: createHash('sha256').update(body).digest('hex') })
    }
    packs.push({ ...manifest, path: `${manifest.id}/dist`, files, sizeBytes: files.reduce((sum, file) => sum + file.size, 0) })
  }
} finally {
  if (scratch) await rm(scratch, { recursive: true, force: true })
}

const catalog = `${JSON.stringify({ schema: 1, packs }, null, 2)}\n`
if (checkOnly) {
  const current = await readFile(join(packsRoot, 'catalog.json'), 'utf8').catch(() => '')
  if (current.replace(/\r\n/g, '\n') !== catalog) problems.push('catalog.json is out of date; run npm --prefix web run scene-packs:build')
  for (const problem of problems) console.error(`scene-packs: ${problem}`)
  if (problems.length) process.exit(1)
  console.log(`scene-packs: ${packs.length} pack(s) reproduce from source`)
} else {
  await writeFile(join(packsRoot, 'catalog.json'), catalog)
  console.log(`catalog: ${packs.length} pack(s)`)
}

// No bundler or package code execution: publish readable, self-contained files.
import { createHash } from 'node:crypto'
import { readdir, readFile, mkdir, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../share-themes')
const check = process.argv.includes('--check')
const packs = []
const idPattern = /^[a-z0-9][a-z0-9-]{0,39}$/
const namePattern = /^[a-z0-9][a-z0-9_.-]{0,79}$/
const extensions = /\.(html|css|js|mjs|json|webp|png|jpg|jpeg|avif|woff2|txt|md)$/
async function files(dir, prefix = '') {
  const result = []
  for (const item of await readdir(dir, { withFileTypes: true })) {
    const path = prefix + item.name
    if (!namePattern.test(item.name) || item.name.includes('..') || path.split('/').length > 4 || path.length > 240) throw Error(`Invalid theme path: ${path}`)
    if (item.isDirectory()) result.push(...await files(join(dir, item.name), `${path}/`))
    else if (item.isFile() && extensions.test(item.name)) result.push(path)
    else throw Error(`Unsupported theme file: ${path}`)
  }
  return result.sort()
}
async function output(file, body) {
  if (check) {
    const previous = await readFile(file)
    if (!previous.equals(Buffer.from(body))) throw Error(`Stale theme output: ${file}`)
  } else { await mkdir(dirname(file), { recursive: true }); await writeFile(file, body) }
}
for (const entry of (await readdir(root, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
  if (entry.name.startsWith('_') || !entry.isDirectory()) continue
  if (!idPattern.test(entry.name) || packs.length >= 100) throw Error('Invalid theme ID or catalog size')
  const dir = join(root, entry.name)
  const manifest = JSON.parse(await readFile(join(dir, 'manifest.json'), 'utf8'))
  if (manifest.id !== entry.name || manifest.schema !== 1 || !['kpanel-share-theme@1', 'kpanel-share-theme@2'].includes(manifest.runtime) || manifest.entry !== 'index.html') throw Error(`Invalid theme manifest: ${entry.name}`)
  const names = await files(join(dir, 'src'))
  if (!names.includes('index.html') || names.includes('manifest.json') || names.length > 39) throw Error(`Invalid file list: ${entry.name}`)
  const bodies = new Map(await Promise.all(names.map(async name => [name, await readFile(join(dir, 'src', name))])))
  bodies.set('manifest.json', Buffer.from(JSON.stringify(manifest, null, 2) + '\n'))
  const specs = []
  for (const [name, body] of [...bodies].sort(([a], [b]) => a.localeCompare(b))) {
    if (!body.length || body.length > 5 * 1024 * 1024 || (['index.html', 'manifest.json'].includes(name) && body.length > 65536)) throw Error(`Oversized theme file: ${name}`)
    await output(join(dir, 'dist', name), body)
    specs.push({ path: name, size: body.length, sha256: createHash('sha256').update(body).digest('hex') })
  }
  if (JSON.stringify(await files(join(dir, 'dist'))) !== JSON.stringify([...bodies.keys()].sort())) throw Error(`Extra files in ${entry.name}/dist`)
  const sizeBytes = specs.reduce((n, file) => n + file.size, 0)
  if (sizeBytes > 5 * 1024 * 1024) throw Error(`Theme exceeds 5 MiB: ${entry.name}`)
  packs.push({ ...manifest, path: `${entry.name}/dist`, files: specs, sizeBytes })
}
await output(join(root, 'catalog.json'), JSON.stringify({ schema: 1, packs }, null, 2) + '\n')
console.log(`share-themes=${check ? 'verified' : 'built'} packs=${packs.length}`)

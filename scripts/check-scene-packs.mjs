#!/usr/bin/env node
// Validates the desktop 3D scene pack repository (scene-packs/) against its
// development rules: manifest fields, published file types and limits, required
// files, and that catalog.json pins the exact size and SHA-256 of every file in
// each pack's dist/. See scene-packs/README.md.
import { createHash } from 'node:crypto'
import { readdir, readFile, stat } from 'node:fs/promises'
import { join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

export const PACK_ID = /^[a-z0-9][a-z0-9-]{0,39}$/
const VERSION = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/
const FILE_NAME = /^[a-z0-9_.-]+$/
const ALLOWED_EXTENSIONS = new Set(['html', 'js', 'mjs', 'css', 'json', 'webp', 'png', 'jpg', 'jpeg', 'avif', 'ktx2', 'basis', 'glb', 'gltf', 'bin', 'hdr', 'exr', 'wasm', 'woff2', 'webm', 'mp4', 'txt', 'md'])
export const LIMITS = Object.freeze({
  files: 200,
  totalBytes: 30 * 1024 * 1024,
  fileBytes: 16 * 1024 * 1024,
  depth: 4,
  posterBytes: 200 * 1024,
  thumbBytes: 40 * 1024,
  previewBytes: 3 * 1024 * 1024,
  cameras: 6,
})
const REQUIRED_DIST = ['index.html', 'manifest.json', 'poster.webp', 'thumb.webp']

async function listFiles(root, directory = root) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isSymbolicLink()) files.push({ path: relative(root, path).split(sep).join('/'), symlink: true })
    else if (entry.isDirectory()) files.push(...await listFiles(root, path))
    else if (entry.isFile()) files.push({ path: relative(root, path).split(sep).join('/'), symlink: false })
  }
  return files.sort((a, b) => a.path.localeCompare(b.path))
}

function checkText(problems, where, value, max) {
  if (typeof value !== 'string' || !value.trim()) problems.push(`${where} is required`)
  else if ([...value].length > max) problems.push(`${where} must be at most ${max} characters`)
}

/** Chinese text is counted in characters; English gets roughly twice the room. */
function checkLocalized(problems, where, value, [chinese, english]) {
  if (!value || typeof value !== 'object') {
    problems.push(`${where} must provide zh-CN and en-US`)
    return
  }
  checkText(problems, `${where}.zh-CN`, value['zh-CN'], chinese)
  checkText(problems, `${where}.en-US`, value['en-US'], english)
  if (value['zh-TW'] !== undefined) checkText(problems, `${where}.zh-TW`, value['zh-TW'], chinese)
}

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/

/** The panel accent colors of a scene: brand, neutral and signature as #rrggbb. */
function checkTheme(problems, where, theme) {
  if (!theme || typeof theme !== 'object' || ['brand', 'neutral', 'signature'].some((key) => !HEX_COLOR.test(theme[key] ?? ''))) {
    problems.push(`${where} must set brand, neutral and signature as #rrggbb colors`)
  }
}

export function checkManifest(manifest, directoryName) {
  const problems = []
  if (manifest.schema !== 1) problems.push('schema must be 1')
  if (!PACK_ID.test(manifest.id ?? '') || manifest.id !== directoryName) problems.push('id must be lowercase letters, digits or "-" and match the directory')
  if (!VERSION.test(manifest.version ?? '')) problems.push('version must be semantic (x.y.z)')
  if (manifest.runtime !== 'kpanel-scene-pack@1') problems.push('runtime must be kpanel-scene-pack@1')
  checkLocalized(problems, 'name', manifest.name, [20, 40])
  checkLocalized(problems, 'description', manifest.description, [40, 90])
  checkText(problems, 'author.name', manifest.author?.name, 40)
  if (manifest.author?.url !== undefined && !/^https:\/\//.test(manifest.author.url)) problems.push('author.url must be https')
  if (typeof manifest.license !== 'string' || !/^[A-Za-z0-9.+-]+(?: (?:AND|OR) [A-Za-z0-9.+-]+)*$/.test(manifest.license)) problems.push('license must be an SPDX identifier')
  if (!Array.isArray(manifest.tags) || manifest.tags.length > 6 || manifest.tags.some((tag) => !/^[a-z0-9-]{1,20}$/.test(tag))) problems.push('tags must be up to 6 lowercase words')
  if (manifest.entry !== 'index.html' || manifest.poster !== 'poster.webp' || manifest.thumb !== 'thumb.webp') problems.push('entry, poster and thumb must be index.html, poster.webp and thumb.webp')
  checkTheme(problems, 'theme', manifest.theme)
  const cameras = manifest.cameras
  if (!Array.isArray(cameras) || cameras.length < 1 || cameras.length > LIMITS.cameras) {
    problems.push(`cameras must list 1-${LIMITS.cameras} shots`)
  } else {
    cameras.forEach((camera, index) => {
      if (!/^[a-z0-9-]{1,40}$/.test(camera?.id ?? '')) problems.push(`cameras[${index}].id is invalid`)
      checkLocalized(problems, `cameras[${index}].name`, camera?.name, [12, 30])
      if (camera?.theme !== undefined) problems.push(`cameras[${index}].theme is not supported: a scene has one theme for all its cameras`)
    })
  }
  if (manifest.dependencies !== undefined && (!Array.isArray(manifest.dependencies) || manifest.dependencies.some((item) => !item?.name || !VERSION.test(item?.version ?? '')))) {
    problems.push('dependencies must list name and exact version')
  }
  return problems
}

export function checkDistFiles(files) {
  const problems = []
  if (files.length > LIMITS.files) problems.push(`dist has ${files.length} files; the limit is ${LIMITS.files}`)
  const total = files.reduce((sum, file) => sum + file.size, 0)
  if (total > LIMITS.totalBytes) problems.push(`dist is ${total} bytes; the limit is ${LIMITS.totalBytes}`)
  for (const file of files) {
    const parts = file.path.split('/')
    const extension = file.path.includes('.') ? file.path.split('.').pop() : ''
    if (file.symlink) problems.push(`${file.path}: symbolic links are not allowed`)
    if (parts.length > LIMITS.depth || parts.some((part) => !FILE_NAME.test(part) || part.startsWith('.'))) problems.push(`${file.path}: use lowercase names without hidden segments, at most ${LIMITS.depth} levels deep`)
    if (!ALLOWED_EXTENSIONS.has(extension)) problems.push(`${file.path}: .${extension} files are not allowed`)
    if (file.size > LIMITS.fileBytes) problems.push(`${file.path}: larger than ${LIMITS.fileBytes} bytes`)
  }
  for (const name of REQUIRED_DIST) if (!files.some((file) => file.path === name)) problems.push(`dist/${name} is required`)
  const limits = { 'poster.webp': LIMITS.posterBytes, 'thumb.webp': LIMITS.thumbBytes, 'preview.webm': LIMITS.previewBytes }
  for (const [name, limit] of Object.entries(limits)) {
    const file = files.find((candidate) => candidate.path === name)
    if (file && file.size > limit) problems.push(`dist/${name} is ${file.size} bytes; the limit is ${limit}`)
  }
  return problems
}

/** Inline scripts never run under the pack CSP, and remote URLs are blocked: flag both early. */
export function checkEntryHTML(html) {
  const problems = []
  for (const match of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    if (!/\bsrc\s*=/.test(match[1]) || match[2].trim()) problems.push('index.html: scripts must be external files (no inline code)')
  }
  if (/\b(?:src|href)\s*=\s*["']?(?:https?:)?\/\//i.test(html)) problems.push('index.html: remote URLs are blocked; bundle every resource into dist/')
  return problems
}

export async function checkScenePacks(packsRoot) {
  const problems = []
  let catalog
  try {
    catalog = JSON.parse(await readFile(join(packsRoot, 'catalog.json'), 'utf8'))
  } catch {
    return ['catalog.json is missing or not valid JSON; run npm --prefix web run scene-packs:build']
  }
  if (catalog.schema !== 1 || !Array.isArray(catalog.packs)) return ['catalog.json must have schema 1 and a packs list']
  const seen = new Set()
  for (const entry of (await readdir(packsRoot, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
    if (!entry.isDirectory() || entry.name.startsWith('_') || entry.name.startsWith('.')) continue
    const packRoot = join(packsRoot, entry.name)
    const prefix = `${entry.name}: `
    let manifest
    try {
      manifest = JSON.parse(await readFile(join(packRoot, 'manifest.json'), 'utf8'))
    } catch {
      problems.push(`${prefix}manifest.json is missing or not valid JSON`)
      continue
    }
    problems.push(...checkManifest(manifest, entry.name).map((problem) => prefix + problem))
    try {
      if (!(await stat(join(packRoot, 'src'))).isDirectory()) throw new Error('not a directory')
    } catch {
      problems.push(`${prefix}src/ with readable source is required`)
    }
    const dist = join(packRoot, 'dist')
    let listed = []
    try {
      listed = await listFiles(dist)
    } catch {
      problems.push(`${prefix}dist/ is missing`)
      continue
    }
    const files = []
    for (const file of listed) {
      if (file.symlink) {
        files.push({ ...file, size: 0, sha256: '' })
        continue
      }
      const body = await readFile(join(dist, file.path))
      files.push({ ...file, size: body.length, sha256: createHash('sha256').update(body).digest('hex') })
    }
    problems.push(...checkDistFiles(files).map((problem) => prefix + problem))
    const entryFile = files.find((file) => file.path === 'index.html')
    if (entryFile) problems.push(...checkEntryHTML(await readFile(join(dist, 'index.html'), 'utf8')).map((problem) => prefix + problem))
    const distManifest = files.find((file) => file.path === 'manifest.json')
    if (distManifest) {
      const copy = await readFile(join(dist, 'manifest.json'), 'utf8')
      if (JSON.stringify(JSON.parse(copy)) !== JSON.stringify(manifest)) problems.push(`${prefix}dist/manifest.json differs from manifest.json; rebuild`)
    }
    const published = catalog.packs.find((pack) => pack.id === entry.name)
    seen.add(entry.name)
    if (!published) {
      problems.push(`${prefix}missing from catalog.json; rebuild`)
      continue
    }
    const expected = files.map(({ path, size, sha256 }) => ({ path, size, sha256 }))
    if (published.path !== `${entry.name}/dist` || JSON.stringify(published.files) !== JSON.stringify(expected) || published.version !== manifest.version) {
      problems.push(`${prefix}catalog.json is out of date with dist/; rebuild`)
    }
  }
  for (const pack of catalog.packs) if (!seen.has(pack.id)) problems.push(`catalog.json lists ${pack.id}, which has no pack directory`)
  return problems
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(process.argv[2] ?? join(fileURLToPath(new URL('..', import.meta.url)), 'scene-packs'))
  const problems = await checkScenePacks(root)
  if (problems.length) {
    for (const problem of problems) console.error(`scene-packs: ${problem}`)
    process.exitCode = 1
  } else {
    console.log('scene-packs: ok')
  }
}

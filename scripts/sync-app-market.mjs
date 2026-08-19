import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const sourceURL = 'https://app.kejilion.sh/'
const sourceOrigin = new URL(sourceURL).origin
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const catalogPath = join(root, 'internal', 'appmarket', 'catalog.json')
const iconRoot = join(root, 'web', 'public', 'app-icons')
const allowedCategories = new Set(['ops', 'ai', 'storage', 'media', 'netsec', 'devprod', 'commtools'])
const safeToken = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/
const safeID = /^(?:builtin|thirdparty)-[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/
const safeIcon = /^icons\/[a-z0-9][a-z0-9_-]{0,63}[.]webp$/
const safeCatalogDate = /^\d{4}-\d{2}-\d{2}$/
const maxIconBytes = 512 * 1024

function isCatalogDate(value) {
  if (typeof value !== 'string' || !safeCatalogDate.test(value)) return false
  const timestamp = Date.parse(`${value}T00:00:00Z`)
  return Number.isFinite(timestamp) && new Date(timestamp).toISOString().slice(0, 10) === value
}

async function fetchChecked(url, accept) {
  const response = await fetch(url, {
    headers: {
      Accept: accept,
      'User-Agent': 'KPanel catalog sync',
    },
    redirect: 'error',
    signal: AbortSignal.timeout(30_000),
  })
  if (!response.ok) throw new Error(`${url} returned HTTP ${response.status}`)
  return response
}

function extractCatalog(html) {
  const prefix = 'window.__APPS__ = '
  const start = html.indexOf(prefix)
  if (start < 0) throw new Error('window.__APPS__ payload was not found')
  const bodyStart = start + prefix.length
  const terminator = html.slice(bodyStart).match(/;\s*<\/script>/)
  const end = terminator?.index === undefined ? -1 : bodyStart + terminator.index
  if (end < bodyStart) throw new Error('window.__APPS__ payload is not terminated')
  return JSON.parse(html.slice(bodyStart, end))
}

function validateCatalog(raw) {
  if (!raw || !Array.isArray(raw.categories) || !Array.isArray(raw.apps)) {
    throw new Error('catalog payload has an invalid shape')
  }
  if (raw.meta?.builtin !== 116 || raw.apps.length < 116 || raw.apps.length > 500) {
    throw new Error('catalog application count is outside the audited boundary')
  }
  if (raw.categories.length !== allowedCategories.size) {
    throw new Error('catalog category count changed; audit the source before syncing')
  }

  const ids = new Set()
  const tokens = new Set()
  const slugs = new Set()
  for (const category of raw.categories) {
    if (!allowedCategories.has(category.key) || !category.zh || !category.en) {
      throw new Error(`invalid category ${JSON.stringify(category)}`)
    }
  }
  for (const app of raw.apps) {
    if (
      !safeID.test(app.id) ||
      ids.has(app.id) ||
      !safeToken.test(app.token) ||
      tokens.has(app.token) ||
      !safeToken.test(app.slug) ||
      slugs.has(app.slug) ||
      !safeIcon.test(app.icon) ||
      !allowedCategories.has(app.cat) ||
      !['builtin', 'thirdparty'].includes(app.source) ||
      typeof app.name_zh !== 'string' ||
      !app.name_zh.trim() ||
      typeof app.desc_zh !== 'string' ||
      (app.addedAt !== undefined && !isCatalogDate(app.addedAt))
    ) {
      throw new Error(`invalid or duplicate application record ${JSON.stringify(app)}`)
    }
    if (app.url) {
      const website = new URL(app.url)
      if (!['http:', 'https:'].includes(website.protocol)) {
        throw new Error(`unsupported website URL for ${app.token}`)
      }
    }
    ids.add(app.id)
    tokens.add(app.token)
    slugs.add(app.slug)
  }
}

function isWebP(bytes) {
  return (
    bytes.length >= 12 &&
    bytes.subarray(0, 4).toString('ascii') === 'RIFF' &&
    bytes.subarray(8, 12).toString('ascii') === 'WEBP'
  )
}

async function syncIcon(app) {
  const iconURL = new URL(app.icon, `${sourceOrigin}/`)
  if (iconURL.origin !== sourceOrigin) throw new Error(`cross-origin icon for ${app.token}`)
  const response = await fetchChecked(iconURL, 'image/webp')
  const bytes = Buffer.from(await response.arrayBuffer())
  if (bytes.length > maxIconBytes || !isWebP(bytes)) {
    throw new Error(`invalid WebP icon for ${app.token}`)
  }
  const filename = `${app.slug}.webp`
  await writeFile(join(iconRoot, filename), bytes, { mode: 0o644 })
  return {
    ...app,
    icon: `/app-icons/${filename}`,
    iconSha256: createHash('sha256').update(bytes).digest('hex'),
  }
}

async function readTraditionalMetadata() {
  try {
    const snapshot = JSON.parse(await readFile(catalogPath, 'utf8'))
    const categories = new Map((snapshot.categories || []).map((category) => [category.key, category]))
    const apps = new Map()
    for (const app of snapshot.apps || []) {
      if (app.id) apps.set(app.id, app)
      if (app.token) apps.set(app.token, app)
    }
    return { categories, apps }
  } catch {
    return { categories: new Map(), apps: new Map() }
  }
}

const html = await (await fetchChecked(sourceURL, 'text/html')).text()
const raw = extractCatalog(html)
validateCatalog(raw)
const existingTraditional = await readTraditionalMetadata()

await mkdir(dirname(catalogPath), { recursive: true })
await mkdir(iconRoot, { recursive: true })
const apps = []
for (const app of raw.apps) {
  const existing = existingTraditional.apps.get(app.id) || existingTraditional.apps.get(app.token)
  // Existing KPanel metadata and icon assets may contain audited product-specific refinements.
  // Remote catalog refreshes only add missing icons; they must not overwrite those local assets.
  const synced = existing?.icon && existing?.iconSha256
    ? { ...app, icon: existing.icon, iconSha256: existing.iconSha256 }
    : await syncIcon(app)
  apps.push({
    ...synced,
    ...(app.name_zh_tw || existing?.name_zh_tw ? { name_zh_tw: app.name_zh_tw || existing.name_zh_tw } : {}),
    ...(app.desc_zh_tw || existing?.desc_zh_tw ? { desc_zh_tw: app.desc_zh_tw || existing.desc_zh_tw } : {}),
    ...(existing
      ? existing.addedAt
        ? { addedAt: existing.addedAt }
        : {}
      : app.addedAt
        ? { addedAt: app.addedAt }
        : {}),
  })
}

const snapshot = {
  schemaVersion: 1,
  source: sourceURL,
  upstream: raw.meta?.source || 'https://github.com/kejilion/sh',
  categories: raw.categories.map((category) => {
    const existing = existingTraditional.categories.get(category.key)
    return existing?.zh_tw && !category.zh_tw ? { ...category, zh_tw: existing.zh_tw } : category
  }),
  apps,
}
await writeFile(catalogPath, `${JSON.stringify(snapshot, null, 2)}\n`, { encoding: 'utf8', mode: 0o644 })

const builtin = apps.filter((app) => app.source === 'builtin').length
const thirdparty = apps.filter((app) => app.source === 'thirdparty').length
process.stdout.write(`Synced ${apps.length} apps (${builtin} built-in, ${thirdparty} third-party).\n`)

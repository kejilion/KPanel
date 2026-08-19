import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'
import { compileTemplate, parse } from 'vue/compiler-sfc'

const webRoot = path.resolve(import.meta.dirname, '..')
const sourceRoot = path.join(webRoot, 'src')
const catalogRoot = path.join(sourceRoot, 'i18n', 'pages')
const chinesePattern = /[\u3400-\u9fff]/
const placeholderPattern = /\{(\d+)\}/g
const forbiddenTranslationPatterns = [
  /KPX[A-Z]+/,
  /Docker Engineering/,
  /Officer Network/,
  /medical (?:checkup|protocol|record)/i,
  /KPX/,
  /%s/,
  /Evolution's mail/,
  /Alexander!/,
  /Could not close temporary folder/,
  /Can not get folder/,
  /\bUnmount/i,
  /reverse agent/i,
  /Domainname/,
  /bullet window/i,
  /\bmission\b/i,
]

const viewFiles = fs.readdirSync(path.join(sourceRoot, 'views'))
  .filter((name) => name.endsWith('.vue') && !['LoginView.vue', 'SetupView.vue'].includes(name))
  .sort()

const sharedFiles = []
for (const directory of ['components']) {
  const root = path.join(sourceRoot, directory)
  for (const relative of fs.readdirSync(root, { recursive: true })) {
    if (relative.endsWith('.vue')) sharedFiles.push(path.join(directory, relative).replaceAll('\\', '/'))
  }
}

function normalizePhrase(value) {
  return value.replace(/\\r/g, '').replace(/\\n/g, ' ').replace(/\s+/g, ' ').trim()
}

function shouldInclude(value) {
  if (!value || !chinesePattern.test(value)) return false
  if (/[\u0000-\u001f]/.test(value) || value.includes('\\x1b') || value.includes('KPANEL_')) return false
  return value.length <= 600
}

function addPhrase(target, value) {
  const normalized = normalizePhrase(value)
  if (shouldInclude(normalized)) target.add(normalized)
}

function templateExpression(node) {
  let value = node.head.text
  node.templateSpans.forEach((span, index) => {
    value += `{${index}}${span.literal.text}`
  })
  return value
}

function extractScriptPhrases(source, fileName, target) {
  if (!source) return
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  function visit(node) {
    if (ts.isStringLiteralLike(node)) addPhrase(target, node.text)
    else if (ts.isTemplateExpression(node)) addPhrase(target, templateExpression(node))
    ts.forEachChild(node, visit)
  }
  visit(file)
}

function walkTemplate(node, target) {
  if (!node || typeof node !== 'object') return
  if (node.type === 2) addPhrase(target, node.content)
  if (node.type === 6 && node.value?.content) addPhrase(target, node.value.content)
  if (node.type === 5 && node.content?.content) extractScriptPhrases(node.content.content, 'template-expression.ts', target)
  if (node.type === 7 && node.exp?.content) extractScriptPhrases(node.exp.content, 'template-directive.ts', target)
  if (Array.isArray(node.children)) node.children.forEach((child) => walkTemplate(child, target))
  if (Array.isArray(node.props)) node.props.forEach((child) => walkTemplate(child, target))
  if (node.branches) node.branches.forEach((child) => walkTemplate(child, target))
}

function extractFile(relativeFile) {
  const source = fs.readFileSync(path.join(sourceRoot, relativeFile), 'utf8')
  const { descriptor } = parse(source, { filename: relativeFile })
  const phrases = new Set()
  extractScriptPhrases(descriptor.script?.content, relativeFile, phrases)
  extractScriptPhrases(descriptor.scriptSetup?.content, relativeFile, phrases)
  if (descriptor.template?.content) {
    const result = compileTemplate({ source: descriptor.template.content, filename: relativeFile, id: relativeFile })
    walkTemplate(result.ast, phrases)
  }
  return phrases
}

function readCatalog(group, locale) {
  const fileName = path.join(catalogRoot, group, `${locale}.ts`)
  if (!fs.existsSync(fileName)) throw new Error(`Missing ${locale} catalog: ${path.relative(webRoot, fileName)}`)
  const source = fs.readFileSync(fileName, 'utf8')
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  let entries
  function visit(node) {
    if (!entries && ts.isExportAssignment(node)) {
      let expression = node.expression
      while (ts.isAsExpression(expression) || ts.isSatisfiesExpression(expression)) expression = expression.expression
      if (ts.isArrayLiteralExpression(expression)) {
        entries = expression.elements.map((element) => {
          if (!ts.isArrayLiteralExpression(element) || element.elements.length !== 2) {
            throw new Error(`Invalid ${locale} catalog entry in ${group}`)
          }
          const [sourceNode, translationNode] = element.elements
          if (!ts.isStringLiteralLike(sourceNode) || !ts.isStringLiteralLike(translationNode)) {
            throw new Error(`${locale} catalog entries must be string pairs in ${group}`)
          }
          return [sourceNode.text, translationNode.text]
        })
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(file)
  if (!entries) throw new Error(`Unable to parse ${locale} catalog: ${group}`)
  return entries
}

function placeholders(value) {
  return [...value.matchAll(placeholderPattern)].map((match) => match[1]).sort().join(',')
}

const rawSharedSources = sharedFiles
  .map((file) => normalizePhrase(fs.readFileSync(path.join(sourceRoot, file), 'utf8')))
const rawRuntimeSources = fs.readdirSync(sourceRoot, { recursive: true })
  .filter((file) => /\.(?:ts|vue)$/.test(file))
  .filter((file) => !file.includes('i18n\\pages') && !file.includes('i18n/pages'))
  .filter((file) => !/\.test\.ts$/.test(file))
  .map((file) => normalizePhrase(fs.readFileSync(path.join(sourceRoot, file), 'utf8')))

function sourceContainsPhrase(source, phrase) {
  const parts = phrase.split(/\{\d+\}/).map(normalizePhrase).filter(Boolean)
  if (!parts.length) return false
  let cursor = 0
  for (const part of parts) {
    const index = source.indexOf(part, cursor)
    if (index < 0) return false
    cursor = index + part.length
  }
  return true
}

function phraseExistsInSource(group, phrase) {
  return rawRuntimeSources.some((source) => sourceContainsPhrase(source, phrase))
}

const groups = new Map()
const shared = new Set()
for (const file of sharedFiles) extractFile(file).forEach((phrase) => shared.add(phrase))
groups.set('shared', shared)
for (const file of viewFiles) groups.set(path.basename(file, '.vue'), extractFile(path.join('views', file)))

const errors = []
let phraseCount = 0
for (const [group, expected] of groups) {
  const catalogs = new Map()
  for (const locale of ['en-US', 'zh-TW']) {
    const entries = readCatalog(group, locale)
    catalogs.set(locale, entries)
    const actual = new Map(entries)
    phraseCount += locale === 'en-US' ? expected.size : 0
    for (const phrase of expected) {
      if (!actual.has(phrase)) errors.push(`${locale} ${group}: missing source phrase ${JSON.stringify(phrase)}`)
    }
    for (const [source, translation] of entries) {
      if (!expected.has(source) && !phraseExistsInSource(group, source)) {
        errors.push(`${locale} ${group}: stale source phrase ${JSON.stringify(source)}`)
      }
      if (!translation.trim()) errors.push(`${locale} ${group}: empty translation for ${JSON.stringify(source)}`)
      const preservedCommand = /^k\s/.test(source) && translation === source
      if (locale === 'en-US' && chinesePattern.test(translation) && !preservedCommand) {
        errors.push(`${locale} ${group}: untranslated Chinese text for ${JSON.stringify(source)}`)
      }
      if (placeholders(source) !== placeholders(translation)) {
        errors.push(`${locale} ${group}: placeholder mismatch for ${JSON.stringify(source)}`)
      }
      if (locale === 'en-US') {
        for (const pattern of forbiddenTranslationPatterns) {
          if (pattern.test(translation)) errors.push(`${locale} ${group}: suspicious translation ${JSON.stringify(translation)}`)
        }
      }
    }
  }
  const expectedSources = [...catalogs.get('en-US')].map(([source]) => source).sort()
  const traditionalSources = [...catalogs.get('zh-TW')].map(([source]) => source).sort()
  if (JSON.stringify(expectedSources) !== JSON.stringify(traditionalSources)) {
    errors.push(`${group}: en-US and zh-TW source key sets differ`)
  }
}

for (const file of viewFiles) {
  const group = path.basename(file, '.vue')
  const source = fs.readFileSync(path.join(sourceRoot, 'views', file), 'utf8')
  const expectedLoader = `@/i18n/pages/${group}/en-US`
  if (!source.includes(expectedLoader)) errors.push(`${group}: route does not register its lazy catalog`)
}
const appSource = fs.readFileSync(path.join(sourceRoot, 'App.vue'), 'utf8')
if (!appSource.includes("@/i18n/pages/shared/en-US")) errors.push('shared: App.vue does not register the lazy shared catalog')

if (errors.length) {
  console.error(errors.join('\n'))
  process.exitCode = 1
} else {
  console.log(`Verified ${phraseCount} localized phrases across ${groups.size} lazy catalogs.`)
}

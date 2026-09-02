import { readdirSync, readFileSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

export type VisualStyleBlock = {
  file: string
  content: string
  contentOffset: number
}

export type VisualStyleDeclaration = {
  file: string
  line: number
  selector: string
  property: string
  value: string
}

const STYLE_BLOCK_PATTERN = /<style\b[^>]*>([\s\S]*?)<\/style>/gi
const CSS_RULE_PATTERN = /([^{}]+)\{([^{}]*)\}/g
const CSS_DECLARATION_PATTERN = /([-\w]+)\s*:\s*([^;{}]+);?/g

// These surfaces use blur to explain a state or a distinct public hero layer,
// not to make ordinary panels look glossy.
export const ALLOWED_COMPONENT_BLUR_SELECTORS = [
  '.share-hero',
  '.drop-overlay',
  '.file-internal-drop-hint',
] as const

function normalize(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function withoutComments(value: string): string {
  // Preserve offsets so diagnostics still point at the original SFC line.
  return value.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ' '))
}

function listVueFiles(srcRoot: string): string[] {
  const files: string[] = []

  function visit(directory: string): void {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name)
      if (entry.isDirectory()) visit(path)
      else if (entry.isFile() && entry.name.endsWith('.vue')) files.push(path)
    }
  }

  visit(srcRoot)
  return files.sort()
}

export function collectVueStyleBlocks(srcRoot: string): VisualStyleBlock[] {
  const webRoot = join(srcRoot, '..')
  const blocks: VisualStyleBlock[] = []

  for (const filePath of listVueFiles(srcRoot)) {
    const source = readFileSync(filePath, 'utf8')
    const file = relative(webRoot, filePath).split(sep).join('/')

    for (const match of source.matchAll(STYLE_BLOCK_PATTERN)) {
      const content = match[1] ?? ''
      const matchOffset = match.index ?? 0
      blocks.push({
        file,
        content,
        contentOffset: matchOffset + match[0].indexOf(content),
      })
    }
  }

  return blocks
}

export function collectVueStyleDeclarations(srcRoot: string): VisualStyleDeclaration[] {
  const declarations: VisualStyleDeclaration[] = []

  for (const block of collectVueStyleBlocks(srcRoot)) {
    const source = readFileSync(join(srcRoot, '..', block.file.replaceAll('/', sep)), 'utf8')
    const scanContent = withoutComments(block.content)

    for (const rule of scanContent.matchAll(CSS_RULE_PATTERN)) {
      const selector = normalize(rule[1] ?? '')
      const body = rule[2] ?? ''
      const bodyOffset = rule[0].indexOf(body)

      for (const declaration of body.matchAll(CSS_DECLARATION_PATTERN)) {
        const property = declaration[1] ?? ''
        const value = normalize(declaration[2] ?? '')
        const offset = block.contentOffset + (rule.index ?? 0) + bodyOffset + (declaration.index ?? 0)
        const line = source.slice(0, offset).split('\n').length
        declarations.push({ file: block.file, line, selector, property, value })
      }
    }
  }

  return declarations
}

export function visualDeclarationKey(
  declaration: Pick<VisualStyleDeclaration, 'file' | 'selector' | 'property' | 'value'>,
): string {
  return [declaration.file, declaration.selector, declaration.property, normalize(declaration.value)].join('|')
}

export function numericCssPixels(value: string): number | null {
  const match = value.trim().match(/^(-?(?:\d+(?:\.\d+)?|\.\d+))(px|rem)$/i)
  if (!match) return null
  return Number(match[1]) * (match[2]!.toLowerCase() === 'rem' ? 16 : 1)
}

export function isNonTokenRadius(value: string): boolean {
  const normalized = normalize(value).toLowerCase()
  if (!/\d+(?:\.\d+)?px/.test(normalized)) return false
  if (normalized.includes('calc(') || /var\([^)]*radius/i.test(normalized)) return false
  if (normalized.includes('50%') || normalized.includes('999px')) return false
  return normalized !== '0'
}

export function isNonTokenShadow(value: string): boolean {
  const normalized = normalize(value).toLowerCase()
  return normalized !== 'none' && !normalized.includes('var(')
}

export function isBlurFilter(value: string): boolean {
  return /\bblur\s*\(/i.test(value)
}

export function isAllowedComponentBlurSelector(selector: string): boolean {
  return ALLOWED_COMPONENT_BLUR_SELECTORS.some((allowed) => selector.includes(allowed))
}

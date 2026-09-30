import type { StreamParser } from '@codemirror/language'
import type { Extension } from '@codemirror/state'

export const CODE_HIGHLIGHT_MAX_BYTES = 1024 * 1024

export interface CodeLanguage {
  id: string
  label: string
  highlighted: boolean
  extension?: Extension
  reason?: 'unsupported' | 'large-file'
}

interface DetectedLanguage {
  id: string
  label: string
}

const extensionLanguages: Record<string, DetectedLanguage> = {
  bash: { id: 'shell', label: 'Shell' },
  c: { id: 'c', label: 'C' },
  cc: { id: 'cpp', label: 'C++' },
  cfg: { id: 'properties', label: '配置文件' },
  cjs: { id: 'javascript', label: 'JavaScript' },
  clj: { id: 'clojure', label: 'Clojure' },
  cljs: { id: 'clojure', label: 'Clojure' },
  cmake: { id: 'cmake', label: 'CMake' },
  cnf: { id: 'properties', label: '配置文件' },
  conf: { id: 'properties', label: '配置文件' },
  cpp: { id: 'cpp', label: 'C++' },
  cs: { id: 'csharp', label: 'C#' },
  css: { id: 'css', label: 'CSS' },
  cts: { id: 'typescript', label: 'TypeScript' },
  cxx: { id: 'cpp', label: 'C++' },
  dart: { id: 'dart', label: 'Dart' },
  desktop: { id: 'properties', label: '配置文件' },
  diff: { id: 'diff', label: 'Diff' },
  edn: { id: 'clojure', label: 'Clojure' },
  elm: { id: 'elm', label: 'Elm' },
  env: { id: 'properties', label: '环境变量' },
  erl: { id: 'erlang', label: 'Erlang' },
  fish: { id: 'shell', label: 'Shell' },
  fs: { id: 'fsharp', label: 'F#' },
  geojson: { id: 'json', label: 'JSON' },
  go: { id: 'go', label: 'Go' },
  gradle: { id: 'groovy', label: 'Groovy' },
  groovy: { id: 'groovy', label: 'Groovy' },
  h: { id: 'c', label: 'C' },
  hh: { id: 'cpp', label: 'C++' },
  hpp: { id: 'cpp', label: 'C++' },
  hrl: { id: 'erlang', label: 'Erlang' },
  hs: { id: 'haskell', label: 'Haskell' },
  htm: { id: 'html', label: 'HTML' },
  html: { id: 'html', label: 'HTML' },
  hxx: { id: 'cpp', label: 'C++' },
  ini: { id: 'properties', label: 'INI' },
  ino: { id: 'cpp', label: 'Arduino' },
  j2: { id: 'jinja2', label: 'Jinja' },
  java: { id: 'java', label: 'Java' },
  jinja: { id: 'jinja2', label: 'Jinja' },
  jl: { id: 'julia', label: 'Julia' },
  js: { id: 'javascript', label: 'JavaScript' },
  json: { id: 'json', label: 'JSON' },
  json5: { id: 'json', label: 'JSON5' },
  jsonc: { id: 'json', label: 'JSON' },
  jsonl: { id: 'json', label: 'JSON Lines' },
  jsx: { id: 'jsx', label: 'JSX' },
  ksh: { id: 'shell', label: 'Shell' },
  kt: { id: 'kotlin', label: 'Kotlin' },
  kts: { id: 'kotlin', label: 'Kotlin' },
  less: { id: 'less', label: 'Less' },
  lua: { id: 'lua', label: 'Lua' },
  markdown: { id: 'markdown', label: 'Markdown' },
  md: { id: 'markdown', label: 'Markdown' },
  mdx: { id: 'markdown', label: 'MDX' },
  mjs: { id: 'javascript', label: 'JavaScript' },
  ml: { id: 'ocaml', label: 'OCaml' },
  mli: { id: 'ocaml', label: 'OCaml' },
  mts: { id: 'typescript', label: 'TypeScript' },
  ndjson: { id: 'json', label: 'JSON Lines' },
  nginx: { id: 'nginx', label: 'Nginx' },
  patch: { id: 'diff', label: 'Diff' },
  php: { id: 'php', label: 'PHP' },
  pl: { id: 'perl', label: 'Perl' },
  pm: { id: 'perl', label: 'Perl' },
  properties: { id: 'properties', label: 'Properties' },
  proto: { id: 'protobuf', label: 'Protocol Buffers' },
  ps1: { id: 'powershell', label: 'PowerShell' },
  psd1: { id: 'powershell', label: 'PowerShell' },
  psm1: { id: 'powershell', label: 'PowerShell' },
  pug: { id: 'pug', label: 'Pug' },
  py: { id: 'python', label: 'Python' },
  pyi: { id: 'python', label: 'Python' },
  r: { id: 'r', label: 'R' },
  rb: { id: 'ruby', label: 'Ruby' },
  repo: { id: 'properties', label: '配置文件' },
  rs: { id: 'rust', label: 'Rust' },
  sass: { id: 'sass', label: 'Sass' },
  scala: { id: 'scala', label: 'Scala' },
  scss: { id: 'scss', label: 'SCSS' },
  service: { id: 'properties', label: 'systemd' },
  sh: { id: 'shell', label: 'Shell' },
  socket: { id: 'properties', label: 'systemd' },
  sql: { id: 'sql', label: 'SQL' },
  styl: { id: 'stylus', label: 'Stylus' },
  swift: { id: 'swift', label: 'Swift' },
  timer: { id: 'properties', label: 'systemd' },
  toml: { id: 'toml', label: 'TOML' },
  ts: { id: 'typescript', label: 'TypeScript' },
  tsx: { id: 'tsx', label: 'TSX' },
  vb: { id: 'vb', label: 'Visual Basic' },
  vue: { id: 'html', label: 'Vue/HTML' },
  xhtml: { id: 'html', label: 'XHTML' },
  xml: { id: 'xml', label: 'XML' },
  xsd: { id: 'xml', label: 'XML' },
  xsl: { id: 'xml', label: 'XML' },
  xslt: { id: 'xml', label: 'XML' },
  yaml: { id: 'yaml', label: 'YAML' },
  yml: { id: 'yaml', label: 'YAML' },
  zsh: { id: 'shell', label: 'Shell' },
}

const fileNameLanguages: Record<string, DetectedLanguage> = {
  '.bash_aliases': { id: 'shell', label: 'Shell' },
  '.bash_logout': { id: 'shell', label: 'Shell' },
  '.bash_profile': { id: 'shell', label: 'Shell' },
  '.bashrc': { id: 'shell', label: 'Shell' },
  '.editorconfig': { id: 'properties', label: '配置文件' },
  '.gitconfig': { id: 'properties', label: '配置文件' },
  '.npmrc': { id: 'properties', label: '配置文件' },
  '.profile': { id: 'shell', label: 'Shell' },
  '.zprofile': { id: 'shell', label: 'Shell' },
  '.zshenv': { id: 'shell', label: 'Shell' },
  '.zshrc': { id: 'shell', label: 'Shell' },
  'cmakelists.txt': { id: 'cmake', label: 'CMake' },
  gemfile: { id: 'ruby', label: 'Ruby' },
  jenkinsfile: { id: 'groovy', label: 'Groovy' },
  rakefile: { id: 'ruby', label: 'Ruby' },
  vagrantfile: { id: 'ruby', label: 'Ruby' },
}

// Files that are backups or templates of another file keep the inner file's
// language, e.g. nginx.conf.bak or config.yml.example.
const wrapperExtensions = new Set([
  'bak', 'backup', 'default', 'dist', 'example', 'in', 'old', 'orig', 'sample', 'save',
])

type StreamParserLoader = () => Promise<StreamParser<unknown>>

const legacyModes: Record<string, StreamParserLoader> = {
  c: async () => (await import('@codemirror/legacy-modes/mode/clike')).c,
  clojure: async () => (await import('@codemirror/legacy-modes/mode/clojure')).clojure,
  cmake: async () => (await import('@codemirror/legacy-modes/mode/cmake')).cmake,
  cpp: async () => (await import('@codemirror/legacy-modes/mode/clike')).cpp,
  csharp: async () => (await import('@codemirror/legacy-modes/mode/clike')).csharp,
  dart: async () => (await import('@codemirror/legacy-modes/mode/clike')).dart,
  diff: async () => (await import('@codemirror/legacy-modes/mode/diff')).diff,
  dockerfile: async () => (await import('@codemirror/legacy-modes/mode/dockerfile')).dockerFile,
  elm: async () => (await import('@codemirror/legacy-modes/mode/elm')).elm,
  erlang: async () => (await import('@codemirror/legacy-modes/mode/erlang')).erlang,
  fsharp: async () => (await import('@codemirror/legacy-modes/mode/mllike')).fSharp,
  groovy: async () => (await import('@codemirror/legacy-modes/mode/groovy')).groovy,
  haskell: async () => (await import('@codemirror/legacy-modes/mode/haskell')).haskell,
  java: async () => (await import('@codemirror/legacy-modes/mode/clike')).java,
  jinja2: async () => (await import('@codemirror/legacy-modes/mode/jinja2')).jinja2,
  julia: async () => (await import('@codemirror/legacy-modes/mode/julia')).julia,
  kotlin: async () => (await import('@codemirror/legacy-modes/mode/clike')).kotlin,
  less: async () => (await import('@codemirror/legacy-modes/mode/css')).less,
  lua: async () => (await import('@codemirror/legacy-modes/mode/lua')).lua,
  nginx: async () => (await import('@codemirror/legacy-modes/mode/nginx')).nginx,
  ocaml: async () => (await import('@codemirror/legacy-modes/mode/mllike')).oCaml,
  perl: async () => (await import('@codemirror/legacy-modes/mode/perl')).perl,
  powershell: async () => (await import('@codemirror/legacy-modes/mode/powershell')).powerShell,
  properties: async () => (await import('@codemirror/legacy-modes/mode/properties')).properties,
  protobuf: async () => (await import('@codemirror/legacy-modes/mode/protobuf')).protobuf,
  pug: async () => (await import('@codemirror/legacy-modes/mode/pug')).pug,
  r: async () => (await import('@codemirror/legacy-modes/mode/r')).r,
  ruby: async () => (await import('@codemirror/legacy-modes/mode/ruby')).ruby,
  rust: async () => (await import('@codemirror/legacy-modes/mode/rust')).rust,
  sass: async () => (await import('@codemirror/legacy-modes/mode/sass')).sass,
  scala: async () => (await import('@codemirror/legacy-modes/mode/clike')).scala,
  scss: async () => (await import('@codemirror/legacy-modes/mode/css')).sCSS,
  shell: async () => (await import('@codemirror/legacy-modes/mode/shell')).shell,
  stylus: async () => (await import('@codemirror/legacy-modes/mode/stylus')).stylus,
  swift: async () => (await import('@codemirror/legacy-modes/mode/swift')).swift,
  toml: async () => (await import('@codemirror/legacy-modes/mode/toml')).toml,
  vb: async () => (await import('@codemirror/legacy-modes/mode/vb')).vb,
}

const mimeLanguages: Array<[RegExp, DetectedLanguage]> = [
  [/javascript|ecmascript/i, { id: 'javascript', label: 'JavaScript' }],
  [/typescript/i, { id: 'typescript', label: 'TypeScript' }],
  [/json/i, { id: 'json', label: 'JSON' }],
  [/html/i, { id: 'html', label: 'HTML' }],
  [/css/i, { id: 'css', label: 'CSS' }],
  [/markdown/i, { id: 'markdown', label: 'Markdown' }],
  [/python/i, { id: 'python', label: 'Python' }],
  [/ya?ml/i, { id: 'yaml', label: 'YAML' }],
  [/php/i, { id: 'php', label: 'PHP' }],
  [/sql/i, { id: 'sql', label: 'SQL' }],
  [/\bgo\b/i, { id: 'go', label: 'Go' }],
  [/xml/i, { id: 'xml', label: 'XML' }],
  [/shell|bash|x-sh/i, { id: 'shell', label: 'Shell' }],
]

export function detectCodeLanguage(fileName: string, mime = ''): DetectedLanguage | undefined {
  let normalized = fileName.trim().toLocaleLowerCase()
  for (let depth = 0; depth < 3; depth += 1) {
    if (/^(?:docker|container)file(?:\.|$)/.test(normalized)) {
      return { id: 'dockerfile', label: 'Dockerfile' }
    }
    if (normalized === '.env' || normalized.startsWith('.env.')) {
      return { id: 'properties', label: '环境变量' }
    }
    if (fileNameLanguages[normalized]) return fileNameLanguages[normalized]
    const separator = normalized.lastIndexOf('.')
    const extension = separator > 0 ? normalized.slice(separator + 1) : ''
    // nginx.conf and variants such as nginx.conf.default, but not nginx.service.
    if (normalized.startsWith('nginx.') && (extension === 'conf' || !extensionLanguages[extension])) {
      return { id: 'nginx', label: 'Nginx' }
    }
    if (extensionLanguages[extension]) return extensionLanguages[extension]
    if (!wrapperExtensions.has(extension)) break
    normalized = normalized.slice(0, separator)
  }
  return mimeLanguages.find(([pattern]) => pattern.test(mime))?.[1]
}

export async function loadCodeLanguage(
  fileName: string,
  mime: string | undefined,
  sizeBytes: number,
): Promise<CodeLanguage> {
  const detected = detectCodeLanguage(fileName, mime)
  if (sizeBytes > CODE_HIGHLIGHT_MAX_BYTES) {
    return {
      id: detected?.id || 'plain-text',
      label: detected?.label || '纯文本',
      highlighted: false,
      reason: 'large-file',
    }
  }
  if (!detected) {
    return {
      id: 'plain-text',
      label: '纯文本',
      highlighted: false,
      reason: 'unsupported',
    }
  }

  let extension: Extension
  switch (detected.id) {
    case 'javascript':
      extension = (await import('@codemirror/lang-javascript')).javascript()
      break
    case 'jsx':
      extension = (await import('@codemirror/lang-javascript')).javascript({ jsx: true })
      break
    case 'typescript':
      extension = (await import('@codemirror/lang-javascript')).javascript({ typescript: true })
      break
    case 'tsx':
      extension = (await import('@codemirror/lang-javascript')).javascript({
        jsx: true,
        typescript: true,
      })
      break
    case 'html':
      extension = (await import('@codemirror/lang-html')).html()
      break
    case 'css':
      extension = (await import('@codemirror/lang-css')).css()
      break
    case 'json':
      extension = (await import('@codemirror/lang-json')).json()
      break
    case 'markdown':
      extension = (await import('@codemirror/lang-markdown')).markdown()
      break
    case 'python':
      extension = (await import('@codemirror/lang-python')).python()
      break
    case 'yaml':
      extension = (await import('@codemirror/lang-yaml')).yaml()
      break
    case 'php':
      extension = (await import('@codemirror/lang-php')).php()
      break
    case 'sql':
      extension = (await import('@codemirror/lang-sql')).sql()
      break
    case 'go':
      extension = (await import('@codemirror/lang-go')).go()
      break
    case 'xml':
      extension = (await import('@codemirror/lang-xml')).xml()
      break
    default: {
      const loadMode = legacyModes[detected.id]
      if (loadMode) {
        const [{ StreamLanguage }, mode] = await Promise.all([
          import('@codemirror/language'),
          loadMode(),
        ])
        extension = StreamLanguage.define(mode)
        break
      }
      return {
        id: 'plain-text',
        label: '纯文本',
        highlighted: false,
        reason: 'unsupported',
      }
    }
  }
  return { ...detected, highlighted: true, extension }
}

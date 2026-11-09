import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./MusicView.vue', import.meta.url), 'utf8')

describe('music playback status layout', () => {
  it('positions buffering and error feedback above the sticky controls without adding a grid row', () => {
    const consoleRule = source.match(/\.music-console\s*\{([^}]*)\}/)?.[1] ?? ''
    const statusRule = source.match(/\.music-playback-status\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(consoleRule).toMatch(/position\s*:\s*sticky/)
    expect(statusRule).toMatch(/position\s*:\s*absolute/)
    expect(statusRule).toMatch(/bottom\s*:\s*calc\(100%\s*\+\s*8px\)/)
    expect(statusRule).not.toMatch(/grid-column/)
    expect(source).toContain(':role="playbackError ? \'alert\' : \'status\'"')
  })
})

it('lets touch users expand clipped playback metadata even in short layouts', () => {
  expect(source).toContain(':aria-expanded="titleExpanded"')
  expect(source).toContain('titleExpanded = !titleExpanded')
  expect(source).toMatch(/music-console__title\.is-expanded strong,.music-console__title\.is-expanded span \{[^}]*white-space:normal/)
  expect(source).toContain('.music-page--focus .music-console__title.is-expanded { max-height:min(30cqh,144px); overflow-y:auto; overscroll-behavior:contain; }')
})

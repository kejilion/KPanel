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

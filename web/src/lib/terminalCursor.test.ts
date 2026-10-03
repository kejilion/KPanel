import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { terminalCursorOptions } from './terminalCursor'

describe('terminal cursor', () => {
  it('is a steady block that turns into an outline when the terminal loses focus', () => {
    expect(terminalCursorOptions).toEqual({
      cursorBlink: false,
      cursorStyle: 'block',
      cursorInactiveStyle: 'outline',
    })
  })

  it.each([
    ['HostTerminal', new URL('../components/terminal/HostTerminal.vue', import.meta.url)],
    ['AppInteractiveTerminal', new URL('../components/apps/AppInteractiveTerminal.vue', import.meta.url)],
  ])('is the only cursor definition of %s', (_name, path) => {
    const source = readFileSync(path, 'utf8')
    expect(source).toContain('...terminalCursorOptions')
    // A cursor chosen locally would make the terminals look different again.
    expect(source).not.toMatch(/cursorStyle\s*:/)
    expect(source).not.toMatch(/cursorBlink\s*:/)
    expect(source).not.toMatch(/cursorWidth\s*:/)
  })
})

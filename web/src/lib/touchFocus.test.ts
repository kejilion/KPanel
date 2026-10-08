// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { focusTextInput, isTextEntry, isTouchPrimary } from './touchFocus'

function mockTouch(matches: boolean): void {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches })))
}

afterEach(() => vi.unstubAllGlobals())

describe('touchFocus', () => {
  it('does not focus text inputs on touch-primary devices', () => {
    mockTouch(true)
    const input = document.createElement('input')
    document.body.appendChild(input)
    expect(isTouchPrimary()).toBe(true)
    expect(focusTextInput(input)).toBe(false)
    expect(document.activeElement).not.toBe(input)
    input.remove()
  })

  it('focuses text inputs on pointer devices', () => {
    mockTouch(false)
    const input = document.createElement('input')
    document.body.appendChild(input)
    expect(focusTextInput(input)).toBe(true)
    expect(document.activeElement).toBe(input)
    input.remove()
  })

  it('classifies text-entry elements', () => {
    const checkbox = document.createElement('input')
    checkbox.type = 'checkbox'
    expect(isTextEntry(document.createElement('textarea'))).toBe(true)
    expect(isTextEntry(document.createElement('input'))).toBe(true)
    expect(isTextEntry(checkbox)).toBe(false)
    expect(isTextEntry(document.createElement('button'))).toBe(false)
  })
})

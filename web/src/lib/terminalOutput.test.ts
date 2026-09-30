import { describe, expect, it } from 'vitest'
import { TerminalOutputNormalizer } from './terminalOutput'

const decoder = new TextDecoder()

describe('TerminalOutputNormalizer', () => {
  it('maps an RGB true-black background to the terminal default background', () => {
    const normalizer = new TerminalOutputNormalizer()

    expect(decoder.decode(normalizer.transform('\x1b[48;2;0;0;0mLISAHOST\x1b[0m')))
      .toBe('\x1b[49mLISAHOST\x1b[0m')
  })

  it('preserves foreground colors and non-black backgrounds', () => {
    const normalizer = new TerminalOutputNormalizer()
    const output = '\x1b[38;2;0;140;249mblue\x1b[48;2;1;0;0mnear-black\x1b[0m'

    expect(decoder.decode(normalizer.transform(output))).toBe(output)
  })

  it('recognizes the background sequence when polling splits it across chunks', () => {
    const normalizer = new TerminalOutputNormalizer()
    const output = [
      normalizer.transform('before\x1b[48;'),
      normalizer.transform('2;0;0;'),
      normalizer.transform('0mafter'),
    ]

    expect(output.map((chunk) => decoder.decode(chunk)).join(''))
      .toBe('before\x1b[49mafter')
  })

  it('matches the reference mapping for randomly split chunks', () => {
    const pieces = ['\x1b[48;2;0;0;0m', '\x1b[48;2;1;0;0m', '\x1b', '\x1b[48;', 'x', '\x1b[0m', '0m', '中']
    let seed = 7
    const random = () => (seed = (seed * 48271) % 2147483647) / 2147483647
    for (let round = 0; round < 50; round += 1) {
      let text = ''
      for (let index = 0; index < 40; index += 1) text += pieces[Math.floor(random() * pieces.length)]
      const bytes = new TextEncoder().encode(text)
      const normalizer = new TerminalOutputNormalizer()
      const chunks: Uint8Array[] = []
      for (let start = 0; start < bytes.length;) {
        const end = Math.min(bytes.length, start + 1 + Math.floor(random() * 9))
        chunks.push(normalizer.transform(bytes.slice(start, end)))
        start = end
      }
      chunks.push(normalizer.flush())
      const output = chunks.map((chunk) => decoder.decode(chunk, { stream: true })).join('') + decoder.decode()
      expect(output).toBe(text.replaceAll('\x1b[48;2;0;0;0m', '\x1b[49m'))
    }
  })

  it('flushes an incomplete escape sequence without dropping bytes', () => {
    const normalizer = new TerminalOutputNormalizer()
    const first = normalizer.transform('before\x1b[48;2;')
    const pending = normalizer.flush()

    expect(decoder.decode(first) + decoder.decode(pending)).toBe('before\x1b[48;2;')
  })
})

const encoder = new TextEncoder()

const trueBlackBackground = encoder.encode('\x1b[48;2;0;0;0m')
const defaultBackground = encoder.encode('\x1b[49m')

export class TerminalOutputNormalizer {
  private matchLength = 0

  // Runs on every output chunk of every visible terminal, so it avoids
  // per-byte array growth: output never exceeds the carried partial match
  // plus the input, because a completed match is replaced by shorter bytes.
  transform(data: string | Uint8Array): Uint8Array {
    const input = typeof data === 'string' ? encoder.encode(data) : data
    if (this.matchLength === 0 && input.indexOf(trueBlackBackground[0]!) < 0) return input
    const output = new Uint8Array(this.matchLength + input.length)
    let length = 0

    for (const byte of input) {
      if (byte === trueBlackBackground[this.matchLength]) {
        this.matchLength += 1
        if (this.matchLength === trueBlackBackground.length) {
          output.set(defaultBackground, length)
          length += defaultBackground.length
          this.matchLength = 0
        }
        continue
      }

      if (this.matchLength > 0) {
        output.set(trueBlackBackground.subarray(0, this.matchLength), length)
        length += this.matchLength
        this.matchLength = 0
        if (byte === trueBlackBackground[0]) {
          this.matchLength = 1
          continue
        }
      }

      output[length++] = byte
    }

    return output.subarray(0, length)
  }

  flush(): Uint8Array {
    const pending = trueBlackBackground.slice(0, this.matchLength)
    this.matchLength = 0
    return pending
  }

  reset(): void {
    this.matchLength = 0
  }
}

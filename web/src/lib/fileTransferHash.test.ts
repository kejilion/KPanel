import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { FileTransferHash } from './fileTransferHash'

describe('bounded file transfer hashing', () => {
  it('matches NIST vectors and preserves the running state after reading its digest', () => {
    const hash = new FileTransferHash()
    expect(hash.hex()).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
    hash.update(new TextEncoder().encode('abc'))
    expect(hash.hex()).toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
    hash.update(new TextEncoder().encode('def'))
    expect(hash.hex()).toBe(createHash('sha256').update('abcdef').digest('hex'))
  })

  it('matches an independent implementation across padding and network chunk boundaries', () => {
    for (const size of [55, 56, 63, 64, 65, 127, 129, 1_048_627]) {
      const data = Uint8Array.from({ length: size }, (_, index) => (index * 31 + 17) % 256)
      const hash = new FileTransferHash()
      for (let offset = 0; offset < data.length; offset += 113) hash.update(data.subarray(offset, offset + 113))
      expect(hash.hex()).toBe(createHash('sha256').update(data).digest('hex'))
    }
  })
})

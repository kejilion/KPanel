// Incremental SHA-256 keeps large files bounded in memory and also works on
// authenticated HTTP/LAN panels where WebCrypto is unavailable.
const constants = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])
const rotate = (value: number, bits: number): number => (value >>> bits) | (value << (32 - bits))

export class FileTransferHash {
  private state = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19])
  private pending = new Uint8Array(64)
  private used = 0
  private length = 0
  private words = new Uint32Array(64)

  update(data: Uint8Array): this {
    this.length += data.length
    let offset = 0
    if (this.used) {
      const count = Math.min(64 - this.used, data.length)
      this.pending.set(data.subarray(0, count), this.used)
      this.used += count
      offset = count
      if (this.used === 64) { this.block(this.pending, 0); this.used = 0 }
    }
    while (offset + 64 <= data.length) { this.block(data, offset); offset += 64 }
    if (offset < data.length) { this.pending.set(data.subarray(offset), 0); this.used = data.length - offset }
    return this
  }

  hex(): string {
    const copy = new FileTransferHash()
    copy.state.set(this.state)
    copy.pending.set(this.pending)
    copy.used = this.used
    copy.length = this.length
    const padding = new Uint8Array((this.used < 56 ? 56 : 120) - this.used + 8)
    padding[0] = 0x80
    const view = new DataView(padding.buffer)
    view.setUint32(padding.length - 8, Math.floor(this.length / 0x20000000))
    view.setUint32(padding.length - 4, (this.length * 8) >>> 0)
    copy.update(padding)
    return Array.from(copy.state, (value) => value.toString(16).padStart(8, '0')).join('')
  }

  private block(data: Uint8Array, offset: number): void {
    const words = this.words
    for (let i = 0; i < 16; i++) {
      const p = offset + i * 4
      words[i] = ((data[p]! << 24) | (data[p + 1]! << 16) | (data[p + 2]! << 8) | data[p + 3]!) >>> 0
    }
    for (let i = 16; i < 64; i++) {
      const x = words[i - 15]!, y = words[i - 2]!
      words[i] = (words[i - 16]! + (rotate(x, 7) ^ rotate(x, 18) ^ (x >>> 3)) + words[i - 7]! + (rotate(y, 17) ^ rotate(y, 19) ^ (y >>> 10))) >>> 0
    }
    let [a, b, c, d, e, f, g, h] = Array.from(this.state) as [number, number, number, number, number, number, number, number]
    for (let i = 0; i < 64; i++) {
      const first = (h + (rotate(e, 6) ^ rotate(e, 11) ^ rotate(e, 25)) + ((e & f) ^ (~e & g)) + constants[i]! + words[i]!) >>> 0
      const second = ((rotate(a, 2) ^ rotate(a, 13) ^ rotate(a, 22)) + ((a & b) ^ (a & c) ^ (b & c))) >>> 0
      h = g; g = f; f = e; e = (d + first) >>> 0; d = c; c = b; b = a; a = (first + second) >>> 0
    }
    const result = [a, b, c, d, e, f, g, h]
    for (let i = 0; i < 8; i++) this.state[i] = (this.state[i]! + result[i]!) >>> 0
  }
}

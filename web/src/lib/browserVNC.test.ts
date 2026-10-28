import { describe, expect, it, vi } from 'vitest'
import { observeVNCFrame } from './browserVNC'

describe('VNC painted-frame barrier', () => {
  it('ignores handshake/resize/cursor updates and waits for decoded pixels to paint', async () => {
    let finish!: () => void
    const painted = vi.fn()
    const decoder = {
      _FBU: { encoding: -223 },
      _handleRect: () => true,
      _framebufferUpdate() { this._handleRect(); return true },
      _display: { flush: () => new Promise<void>(resolve => { finish = resolve }) },
    }
    const restore = observeVNCFrame(decoder, painted)
    decoder._framebufferUpdate()
    expect(painted).not.toHaveBeenCalled()
    decoder._FBU.encoding = -239
    decoder._framebufferUpdate()
    expect(painted).not.toHaveBeenCalled()
    decoder._FBU.encoding = 0x574d5664
    decoder._framebufferUpdate()
    expect(painted).not.toHaveBeenCalled()
    decoder._FBU.encoding = 7
    decoder._framebufferUpdate()
    expect(painted).not.toHaveBeenCalled()
    finish(); await Promise.resolve()
    expect(painted).toHaveBeenCalledTimes(1)
    decoder._framebufferUpdate()
    restore(); finish(); await Promise.resolve()
    expect(painted).toHaveBeenCalledTimes(1)
  })
  it('fails closed on an incompatible noVNC interface', () => {
    expect(() => observeVNCFrame({}, () => {})).toThrow('Unsupported VNC decoder')
  })
  it('retains pixel progress across partial framebuffer messages', async () => {
    const painted = vi.fn()
    let complete = false
    const decoder = {
      _FBU: { encoding: 0 },
      _handleRect: () => true,
      _framebufferUpdate() { if (!complete) this._handleRect(); return complete },
      _display: { flush: () => Promise.resolve() },
    }
    observeVNCFrame(decoder, painted)
    decoder._framebufferUpdate(); await Promise.resolve()
    expect(painted).not.toHaveBeenCalled()
    complete = true
    decoder._framebufferUpdate(); await Promise.resolve()
    expect(painted).toHaveBeenCalledTimes(1)
  })
})

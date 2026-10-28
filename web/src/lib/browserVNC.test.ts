import { describe, expect, it, vi } from 'vitest'
import { observeVNCFrame, suppressVNCCursor } from './browserVNC'

describe('VNC cursor suppression', () => {
  it('clears the remote cursor on connect and every later cursor update', () => {
    const clear = vi.fn()
    const update = vi.fn()
    const client = { _updateCursor: update, _cursor: { clear } }
    const restore = suppressVNCCursor(client)
    expect(clear).toHaveBeenCalledTimes(1)
    client._updateCursor('shape')
    expect(update).toHaveBeenCalledWith('shape')
    expect(clear).toHaveBeenCalledTimes(2)
    restore()
    expect(client._updateCursor).toBe(update)
    expect(clear).toHaveBeenCalledTimes(3)
  })

  it('fails closed if noVNC changes its cursor interface', () => {
    expect(() => suppressVNCCursor({})).toThrow('Unsupported VNC cursor interface')
  })
})

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

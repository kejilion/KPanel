import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

function source(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const jobs = source('../../views/JobsView.vue')
const ai = source('../../views/AiView.vue')
const terminal = source('../terminal/HostTerminal.vue')

describe('desktop background lifecycle contract', () => {
  it('pauses active-job refreshes while the activity window is inactive', () => {
    expect(jobs).toContain('desktopWindowActiveKey')
    expect(jobs).toContain('!current.signal.aborted && desktopWindowActive.value')
    expect(jobs).toMatch(/watch\(desktopWindowActive,[\s\S]*?else\s*\{\s*controller\?\.abort\(\)\s*if \(timer\) window\.clearTimeout\(timer\)/)
  })

  it('buffers AI stream deltas without scheduling background animation frames', () => {
    expect(ai).toContain('desktopWindowActiveKey')
    expect(ai).toContain('desktopWindowActive.value&&!streamFrame')
    expect(ai).toContain("if(!active){if(streamFrame)cancelAnimationFrame(streamFrame)")
  })

  it('pauses terminal long-poll traffic without closing the server session', () => {
    expect(terminal).toContain('desktopWindowActiveKey')
    expect(terminal).toContain('disposed || !desktopWindowActive.value')
    expect(terminal).toContain('pollController?.abort()')
    expect(terminal).toContain('if (desktopWindowActive.value) void poll()')
  })
})

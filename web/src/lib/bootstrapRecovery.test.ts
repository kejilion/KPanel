// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const script = readFileSync(resolve('public/bootstrap-recovery.js'), 'utf8')
beforeEach(() => { vi.useFakeTimers(); document.body.innerHTML = ''; localStorage.clear(); window.eval(script) })
afterEach(() => { window.dispatchEvent(new Event('kpanel:ready')); vi.useRealTimers() })

describe('module-independent bootstrap recovery', () => {
  it('shows a recovery action when the entry script fails and removes it on eventual readiness', () => {
    const module = document.createElement('script')
    module.type = 'module'
    document.body.append(module)
    module.dispatchEvent(new Event('error'))
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('页面未能完成加载')
    expect(document.querySelector('button')?.textContent).toBe('重新加载')
    window.dispatchEvent(new Event('kpanel:ready'))
    expect(document.getElementById('bootstrap-recovery')).toBeNull()
    vi.advanceTimersByTime(12000)
    expect(document.getElementById('bootstrap-recovery')).toBeNull()
  })
  it.each(['vite:preloadError', 'kpanel:boot-failed'])('recovers %s without duplicate cards', event => {
    window.dispatchEvent(new Event(event))
    vi.advanceTimersByTime(12000)
    expect(document.querySelectorAll('#bootstrap-recovery')).toHaveLength(1)
  })
  it('bounds a hanging bootstrap and respects the saved language', () => {
    localStorage.setItem('kejilion-panel-locale', 'en-US')
    vi.advanceTimersByTime(11999)
    expect(document.getElementById('bootstrap-recovery')).toBeNull()
    vi.advanceTimersByTime(1)
    expect(document.querySelector('button')?.textContent).toBe('Reload page')
  })
  it('ignores analytics failures and errors after the application is ready', () => {
    const analytics = document.createElement('script')
    document.body.append(analytics)
    analytics.dispatchEvent(new Event('error'))
    expect(document.getElementById('bootstrap-recovery')).toBeNull()
    window.dispatchEvent(new Event('kpanel:ready'))
    window.dispatchEvent(new Event('vite:preloadError'))
    vi.advanceTimersByTime(12000)
    expect(document.getElementById('bootstrap-recovery')).toBeNull()
  })
})

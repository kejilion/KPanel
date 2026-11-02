// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { dismissEnvironmentResult, readDismissedEnvironmentResults } from './environmentJobDismissal'
import type { WebEnvironmentJob } from '@/types/api'

const storageKey = 'kpanel:environment-dismissed-results:v1'
beforeEach(() => window.localStorage.clear())

describe('environment result preferences', () => {
  it.each(['{broken', '{}', JSON.stringify(['a'.repeat(32) + ':running']), ' '.repeat(4097)])('ignores unusable stored preferences (%#)', (raw) => {
    window.localStorage.setItem(storageKey, raw)
    expect(readDismissedEnvironmentResults()).toEqual([])
  })

  it('bounds history and merges acknowledgements from another view', () => {
    let current: string[] = []
    for (let index = 0; index < 55; index++) {
      current = dismissEnvironmentResult({ id: index.toString(16).padStart(32, '0'), status: 'failed' } as WebEnvironmentJob, current)
    }
    expect(current).toHaveLength(50)
    expect(readDismissedEnvironmentResults()).toEqual(current)
    const otherView = dismissEnvironmentResult({ id: 'b'.repeat(32), status: 'succeeded' } as WebEnvironmentJob, [])
    expect(otherView).toHaveLength(50)
    expect(otherView.slice(1)).toEqual(current.slice(0, 49))
    expect(window.localStorage.getItem(storageKey)!.length).toBeLessThan(4096)
  })
})

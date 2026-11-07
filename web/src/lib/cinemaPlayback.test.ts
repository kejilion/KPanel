// @vitest-environment jsdom
import { afterEach, expect, it } from 'vitest'
import { applyCinemaPlayback, cinemaCheckpoint, cinemaPlayback, MAX_CINEMA_CHECKPOINTS, mergeCinemaPlayback, parseCinemaPlayback, recordCinemaCheckpoint, setCinemaPlaybackOwner } from './cinemaPlayback'
import type { CinemaCheckpoint } from '@/types/api'

const point = (path = '/films/a.mp4'): CinemaCheckpoint => ({ path, hostId: 'edge', fileVersion: 'v1', position: 42, duration: 120, completed: false })
afterEach(() => setCinemaPlaybackOwner(''))
it('bounds, validates and deduplicates untrusted stored checkpoints', () => {
  expect(parseCinemaPlayback({ entries: [point()] })?.entries).toHaveLength(1)
  for (const update of [{ path: '/films/../secret' }, { hostId: 'bad/host' }, { fileVersion: '' }, { position: NaN }, { duration: Infinity }, { duration: 0 }, { position: 121 }]) {
    expect(parseCinemaPlayback({ entries: [{ ...point(), ...update }] })).toBeUndefined()
  }
  expect(parseCinemaPlayback({ entries: [point(), point()] })).toBeUndefined()
  expect(parseCinemaPlayback({ entries: Array(MAX_CINEMA_CHECKPOINTS + 1).fill(point()) })).toBeUndefined()
})
it('keeps other films and honors explicit restart instead of taking maximum progress', () => {
  const merged = mergeCinemaPlayback({ entries: [point(), point('/films/b.mp4')] }, { entries: [{ ...point(), position: 0 }] })
  expect(merged.entries.map(item => item.position)).toEqual([0, 42])
  expect(merged.entries[0]?.path).toBe('/films/a.mp4')
})
it('scopes by host and file version and isolates logout/account changes', () => {
  setCinemaPlaybackOwner('admin')
  applyCinemaPlayback({ entries: [point()] })
  expect(cinemaCheckpoint({ path: point().path, resourceVersion: 'v1' }, 'edge')?.position).toBe(42)
  expect(cinemaCheckpoint({ path: point().path, resourceVersion: 'v2' }, 'edge')).toBeUndefined()
  expect(cinemaCheckpoint({ path: point().path, resourceVersion: 'v1' }, '')).toBeUndefined()
  setCinemaPlaybackOwner('other')
  recordCinemaCheckpoint(point(), 'admin', true)
  expect(cinemaPlayback.entries).toHaveLength(0)
})

// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import {
  filesPaneDensity,
  normalizeFilesSplitPath,
  readFilesSplitPreference,
  writeFilesSplitPreference,
} from './filesSplit'

describe('files split preference', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('keeps only the directory and host of a file-manager location', () => {
    expect(normalizeFilesSplitPath('/files?path=/var/log&hostId=h1&file=/var/log/syslog'))
      .toBe('/files?path=%2Fvar%2Flog&hostId=h1')
    expect(normalizeFilesSplitPath('/files')).toBe('/files')
    expect(normalizeFilesSplitPath('/files?path=relative')).toBe('/files')
  })

  it('rejects locations outside the file manager', () => {
    for (const value of ['/overview', '/filesystem', '/files/../cluster', '//evil.example/files', 'https://evil.example/files', 42]) {
      expect(normalizeFilesSplitPath(value)).toBeUndefined()
    }
  })

  it('round-trips the pane state and ignores damaged storage', () => {
    writeFilesSplitPreference({ open: true, secondaryPath: '/files?path=/srv&file=/srv/a.txt' })
    expect(readFilesSplitPreference()).toEqual({ open: true, secondaryPath: '/files?path=%2Fsrv' })

    window.localStorage.setItem('kpanel:files:split:v1', '{broken')
    expect(readFilesSplitPreference()).toEqual({ open: false })
    window.localStorage.setItem('kpanel:files:split:v1', JSON.stringify({ open: 'yes', secondaryPath: '/settings' }))
    expect(readFilesSplitPreference()).toEqual({ open: false })
  })

  it('maps pane width to the desktop-window list densities', () => {
    expect(filesPaneDensity(0)).toBe('regular')
    expect(filesPaneDensity(560)).toBe('narrow')
    expect(filesPaneDensity(760)).toBe('compact')
    expect(filesPaneDensity(900)).toBe('regular')
  })
})

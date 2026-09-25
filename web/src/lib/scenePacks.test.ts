import { describe, expect, it } from 'vitest'
import {
  formatPackSize,
  isOfficialScenePack,
  localizedText,
  parseScenePackEvent,
  scenePackFromWallpaper,
  scenePackPageURL,
  scenePackThemeColors,
  scenePackWallpaper,
} from '@/lib/scenePacks'

describe('scene pack helpers', () => {
  it('marks only KPanel-authored packs as official', () => {
    expect(isOfficialScenePack({ author: { name: 'KPanel' } })).toBe(true)
    expect(isOfficialScenePack({ author: { name: 'kpanel-fan' } })).toBe(false)
    expect(isOfficialScenePack({})).toBe(false)
  })

  it('loads pack pages only from a same-origin file base', () => {
    const base = `/api/v1/desktop/scene-packs/orbital-station/files/${'a'.repeat(32)}/`
    expect(scenePackPageURL({ fileBase: base })).toBe(`${base}index.html`)
    for (const fileBase of [null, '', 'https://evil.example/', '//evil.example/', 'javascript:alert(1)//', '/api/v1/files', '/api/v1/files/', base.replace('/files/', '/files/%2e%2e/'), '/\\evil.example/']) {
      expect(scenePackPageURL({ fileBase })).toBeUndefined()
    }
  })

  it('reads pack wallpapers from the wallpaper key without trusting other values', () => {
    expect(scenePackWallpaper('orbital-station')).toBe('pack:orbital-station')
    expect(scenePackFromWallpaper('pack:orbital-station')).toBe('orbital-station')
    for (const value of [null, undefined, 'orbit', 'pack:', 'pack:../secrets', 'pack:Orbital', `pack:${'a'.repeat(41)}`]) {
      expect(scenePackFromWallpaper(value)).toBeUndefined()
    }
  })

  it('falls back through the published locales', () => {
    const text = { 'zh-CN': '星港轨道', 'en-US': 'Orbital Station' }
    expect(localizedText(text, 'en-US')).toBe('Orbital Station')
    expect(localizedText(text, 'zh-TW')).toBe('星港轨道')
    expect(localizedText({ 'en-US': 'Only English' }, 'zh-CN')).toBe('Only English')
    expect(localizedText({}, 'zh-CN')).toBe('')
  })

  it('turns a scene theme into panel colors from plain hex values only', () => {
    expect(scenePackThemeColors({ theme: { brand: '#2F6FD0', neutral: '#33415a', signature: '#35b8d6' } }))
      .toEqual({ brand: '#2f6fd0', neutral: '#33415a', signatureLinked: false, signature: '#35b8d6' })
    expect(scenePackThemeColors({})).toBeUndefined()
    expect(scenePackThemeColors({ theme: { brand: '#fff', neutral: '#000000', signature: '#000000' } })).toBeUndefined()
    expect(scenePackThemeColors({ theme: { brand: 'var(--x)', neutral: '#000000', signature: '#000000' } })).toBeUndefined()
  })

  it('formats download sizes', () => {
    expect(formatPackSize(650_000)).toBe('635 KB')
    expect(formatPackSize(100)).toBe('1 KB')
    expect(formatPackSize(12 * 1024 * 1024)).toBe('12.0 MB')
  })

  it('accepts only well-formed events from a pack', () => {
    expect(parseScenePackEvent({ source: 'kpanel-scene-pack', type: 'ready', cameras: ['a', 'b'] }))
      .toEqual({ source: 'kpanel-scene-pack', type: 'ready', cameras: ['a', 'b'] })
    expect(parseScenePackEvent({ source: 'kpanel-scene-pack', type: 'camera', index: 2 }))
      .toEqual({ source: 'kpanel-scene-pack', type: 'camera', index: 2 })
    expect(parseScenePackEvent({ source: 'kpanel-scene-pack', type: 'error', reason: 'x'.repeat(200) }))
      .toEqual({ source: 'kpanel-scene-pack', type: 'error', reason: 'x'.repeat(80) })
    expect(parseScenePackEvent({ source: 'kpanel-scene-pack', type: 'progress', value: 0.4 }))
      .toEqual({ source: 'kpanel-scene-pack', type: 'progress', value: 0.4 })
    expect(parseScenePackEvent({ source: 'kpanel-scene-pack', type: 'progress', value: 7 }))
      .toEqual({ source: 'kpanel-scene-pack', type: 'progress', value: 1 })
    for (const data of [
      null,
      'ready',
      { source: 'kpanel-desktop', type: 'ready', cameras: [] },
      { source: 'kpanel-scene-pack', type: 'ready', cameras: Array.from({ length: 13 }, () => 'a') },
      { source: 'kpanel-scene-pack', type: 'ready', cameras: ['x'.repeat(41)] },
      { source: 'kpanel-scene-pack', type: 'ready', cameras: [{ toString: () => 'a' }] },
      { source: 'kpanel-scene-pack', type: 'camera', index: 1.5 },
      { source: 'kpanel-scene-pack', type: 'camera', index: 12 },
      { source: 'kpanel-scene-pack', type: 'camera', index: -1 },
      { source: 'kpanel-scene-pack', type: 'error', reason: 42 },
      { source: 'kpanel-scene-pack', type: 'progress', value: Number.NaN },
      { source: 'kpanel-scene-pack', type: 'progress', value: '0.5' },
      { source: 'kpanel-scene-pack', type: 'navigate', url: '/logout' },
    ]) {
      expect(parseScenePackEvent(data)).toBeUndefined()
    }
  })
})

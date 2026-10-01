import { describe, expect, it } from 'vitest'
import {
  moveStartMenuIndex,
  normalizeStartMenuQuery,
  searchDesktopStartMenu,
  type DesktopStartMenuItem,
} from './desktopStartMenu'

const items: DesktopStartMenuItem[] = [
  { key: 'action:wallpaper', section: 'actions', label: '更换壁纸和主题', keywords: ['wallpaper'] },
  { key: 'nav:/docker', section: 'system', label: '容器', keywords: ['docker'] },
  { key: 'nav:/files', section: 'system', label: '文件管理', keywords: ['files'] },
  { key: 'app:nginx', section: 'entries', label: 'Nginx Proxy Manager', detail: '应用', keywords: ['http://10.0.0.5:81', 'nginx'] },
  { key: 'site:blog', section: 'entries', label: 'blog.example.com', detail: '网站', hidden: true },
  { key: 'shortcut:logs', section: 'entries', label: 'Logs', detail: '目录快捷方式', keywords: ['/var/log/nginx'] },
  { key: 'action:sign-out', section: 'actions', label: '退出登录', keywords: ['logout', 'sign out'] },
]

describe('desktop start menu search', () => {
  it('normalizes width, case and whitespace before matching', () => {
    expect(normalizeStartMenuQuery('  ＤＯＣＫＥＲ   Logs ')).toBe('docker logs')
  })

  it('shows launcher sections without actions for an empty query, in section order', () => {
    expect(searchDesktopStartMenu(items, '   ').map((item) => item.key)).toEqual([
      'nav:/docker',
      'nav:/files',
      'app:nginx',
      'site:blog',
      'shortcut:logs',
    ])
  })

  it('finds Chinese labels, English route aliases, hidden entries and actions', () => {
    expect(searchDesktopStartMenu(items, '文件').map((item) => item.key)).toEqual(['nav:/files'])
    expect(searchDesktopStartMenu(items, 'DOCKER').map((item) => item.key)).toEqual(['nav:/docker'])
    expect(searchDesktopStartMenu(items, 'blog').map((item) => item.key)).toEqual(['site:blog'])
    expect(searchDesktopStartMenu(items, 'logout').map((item) => item.key)).toEqual(['action:sign-out'])
  })

  it('ranks label prefixes before label substrings before keyword matches within a section', () => {
    expect(searchDesktopStartMenu(items, 'nginx').map((item) => item.key)).toEqual(['app:nginx', 'shortcut:logs'])
    expect(searchDesktopStartMenu(items, 'log').map((item) => item.key)).toEqual([
      'shortcut:logs',
      'site:blog',
      'action:sign-out',
    ])
  })

  it('requires every space-separated term to match somewhere', () => {
    expect(searchDesktopStartMenu(items, 'nginx proxy').map((item) => item.key)).toEqual(['app:nginx'])
    expect(searchDesktopStartMenu(items, 'nginx missing')).toEqual([])
  })
})

describe('desktop start menu keyboard movement', () => {
  const grid = { count: 7, columns: 3 }

  it('moves through the launcher grid by rows and into the list', () => {
    expect(moveStartMenuIndex(1, 'ArrowDown', 10, grid)).toBe(4)
    expect(moveStartMenuIndex(4, 'ArrowDown', 10, grid)).toBe(7)
    expect(moveStartMenuIndex(6, 'ArrowDown', 10, grid)).toBe(7)
    expect(moveStartMenuIndex(4, 'ArrowUp', 10, grid)).toBe(1)
    expect(moveStartMenuIndex(1, 'ArrowUp', 10, grid)).toBe(1)
    expect(moveStartMenuIndex(7, 'ArrowUp', 10, grid)).toBe(6)
    expect(moveStartMenuIndex(8, 'ArrowUp', 10, grid)).toBe(7)
  })

  it('keeps a grid-only menu in place at its last row and clamps list movement', () => {
    expect(moveStartMenuIndex(5, 'ArrowDown', 7, grid)).toBe(5)
    expect(moveStartMenuIndex(9, 'ArrowDown', 10, grid)).toBe(9)
    expect(moveStartMenuIndex(0, 'ArrowLeft', 10, grid)).toBe(0)
    expect(moveStartMenuIndex(9, 'ArrowRight', 10, grid)).toBe(9)
    expect(moveStartMenuIndex(4, 'Home', 10, grid)).toBe(0)
    expect(moveStartMenuIndex(4, 'End', 10, grid)).toBe(9)
  })

  it('treats a flat result list as single steps and reports no target when empty', () => {
    expect(moveStartMenuIndex(0, 'ArrowDown', 3)).toBe(1)
    expect(moveStartMenuIndex(2, 'ArrowUp', 3)).toBe(1)
    expect(moveStartMenuIndex(-1, 'ArrowDown', 3)).toBe(1)
    expect(moveStartMenuIndex(0, 'ArrowDown', 0)).toBe(-1)
  })
})

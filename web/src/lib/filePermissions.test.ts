import { describe, expect, it } from 'vitest'
import { commonPermissionValue, permissionValue } from './filePermissions'

describe('file permission values', () => {
  it.each([
    ['drwxr-xr-x', '755'], ['-rw-r--r--', '644'], ['drwx------', '700'],
    ['----------', '000'], ['0750', '750'], ['644', '644'], ['invalid', undefined],
  ])('reads %s as %s', (mode, expected) => {
    expect(permissionValue(mode)).toBe(expected)
  })

  it('prefills only a known permission shared by the entire selection', () => {
    expect(commonPermissionValue([{ mode: 'drwxr-xr-x' }])).toBe('755')
    expect(commonPermissionValue([{ mode: '0755' }, { mode: 'drwxr-xr-x' }])).toBe('755')
    expect(commonPermissionValue([{ mode: 'drwxr-xr-x' }, { mode: '-rw-r--r--' }])).toBe('')
    expect(commonPermissionValue([{ mode: 'unknown' }])).toBe('')
    expect(commonPermissionValue([])).toBe('')
  })
})

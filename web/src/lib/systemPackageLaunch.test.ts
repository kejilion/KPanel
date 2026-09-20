// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { systemPackageLaunchCommand } from './systemPackageLaunch'

describe('system package launch commands', () => {
	it('maps launchable package IDs to fixed commands', () => {
		expect(systemPackageLaunchCommand('htop')).toBe('htop')
	})

	it('rejects non-launchable catalog entries', () => {
		expect(systemPackageLaunchCommand('curl')).toBeUndefined()
	})

	it('does not treat prototype keys as executable commands', () => {
		expect(systemPackageLaunchCommand('__proto__' as never)).toBeUndefined()
	})
})

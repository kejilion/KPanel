import type { SystemPackageID } from '@/types/api'

const launchCommands: Partial<Record<SystemPackageID, string>> = {
	htop: 'htop',
	iftop: 'iftop',
	tmux: 'tmux',
	btop: 'btop',
	ranger: 'cd / && ranger',
	ncdu: 'ncdu /',
	fzf: 'cd / && fzf',
}

export function systemPackageLaunchCommand(id: SystemPackageID): string | undefined {
	if (!Object.prototype.hasOwnProperty.call(launchCommands, id)) return undefined
	const command = launchCommands[id]
	return typeof command === 'string' && command ? command : undefined
}

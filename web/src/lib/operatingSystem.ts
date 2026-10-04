export interface OperatingSystemIdentityInput {
  os?: string
  osId?: string
  osLike?: string[]
}

export interface OperatingSystemIdentity {
  key: string
  label: string
}

const identities: Array<OperatingSystemIdentity & { ids: string[]; names: string[] }> = [
  { key: 'windows', label: 'Windows', ids: ['windows'], names: ['windows'] },
  { key: 'ubuntu', label: 'Ubuntu', ids: ['ubuntu'], names: ['ubuntu'] },
  { key: 'debian', label: 'Debian', ids: ['debian'], names: ['debian'] },
  { key: 'centos', label: 'CentOS', ids: ['centos'], names: ['centos'] },
  { key: 'rocky', label: 'Rocky Linux', ids: ['rocky'], names: ['rocky linux'] },
  { key: 'alma', label: 'AlmaLinux', ids: ['almalinux', 'alma'], names: ['almalinux'] },
  {
    key: 'rhel',
    label: 'Red Hat Enterprise Linux',
    ids: ['rhel', 'redhat', 'redhatenterpriseserver'],
    names: ['red hat enterprise linux', 'rhel'],
  },
  {
    key: 'oracle',
    label: 'Oracle Linux',
    ids: ['ol', 'oracle', 'oraclelinux'],
    names: ['oracle linux'],
  },
  { key: 'fedora', label: 'Fedora', ids: ['fedora'], names: ['fedora'] },
  { key: 'alpine', label: 'Alpine Linux', ids: ['alpine'], names: ['alpine linux'] },
  { key: 'manjaro', label: 'Manjaro', ids: ['manjaro'], names: ['manjaro'] },
  { key: 'arch', label: 'Arch Linux', ids: ['arch', 'archlinux'], names: ['arch linux'] },
  {
    key: 'opensuse',
    label: 'openSUSE',
    ids: ['opensuse', 'opensuse-leap', 'opensuse-tumbleweed'],
    names: ['opensuse'],
  },
  {
    key: 'suse',
    label: 'SUSE Linux Enterprise',
    ids: ['sles', 'sles_sap', 'sled', 'suse'],
    names: ['suse linux enterprise', 'sles'],
  },
]

export function detectOperatingSystemIdentity(
  input?: OperatingSystemIdentityInput,
): OperatingSystemIdentity {
  const osId = input?.osId?.trim().toLowerCase() || ''
  const exact = identities.find((identity) => identity.ids.includes(osId))
  if (exact) return { key: exact.key, label: exact.label }

  const name = ` ${(input?.os || '').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim()} `
  const named = identities.find((identity) =>
    identity.names.some((candidate) => name.includes(` ${candidate} `)),
  )
  if (named) return { key: named.key, label: named.label }

  // ID_LIKE describes package/runtime compatibility, not the distribution's
  // brand. Unknown derivatives must use the neutral Linux mark instead of
  // impersonating Ubuntu, Debian, Fedora, CentOS, or Arch.
  return { key: 'linux', label: input?.os?.trim() || 'Linux' }
}

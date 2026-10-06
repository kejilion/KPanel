import { describe, expect, it } from 'vitest'
import { withoutUnsupportedLightNodes } from './nodeFeatureHosts'

describe('node feature host availability', () => {
  it('offers only supported light nodes while preserving panel hosts', () => {
    const hosts = [
      { id: 'linux', kind: 'light_node', platform: 'linux' },
      { id: 'unsupported', kind: 'light_node', platform: 'unknown' },
      { id: 'legacy-linux', kind: 'light_node' },
      { id: 'windows-panel', kind: 'panel', platform: 'windows' },
    ]

    expect(withoutUnsupportedLightNodes(hosts).map((host) => host.id))
      .toEqual(['linux', 'legacy-linux', 'windows-panel'])
  })
})

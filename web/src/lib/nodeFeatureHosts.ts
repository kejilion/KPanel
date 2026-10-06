type NodeFeatureHostIdentity = { kind?: string; platform?: string }

/** Filter explicit non-Linux light nodes while retaining v1.24-era Linux records without platform metadata. */
export function isUnsupportedLightNode(host: NodeFeatureHostIdentity): boolean {
  return host.kind === 'light_node' && host.platform !== undefined && host.platform !== 'linux'
}

/** History, files, gallery, and terminal pickers only expose supported light nodes. */
export function withoutUnsupportedLightNodes<T extends NodeFeatureHostIdentity>(hosts: readonly T[]): T[] {
  return hosts.filter((host) => !isUnsupportedLightNode(host))
}

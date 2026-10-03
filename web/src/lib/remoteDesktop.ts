let backend: Promise<typeof import('@devolutions/iron-remote-desktop-rdp')> | undefined

export function loadRemoteDesktop() {
  return backend ??= (async () => {
    const rdp = await import('@devolutions/iron-remote-desktop-rdp')
    await rdp.init('warn')
    await import('@devolutions/iron-remote-desktop')
    return rdp
  })().catch((error: unknown) => { backend = undefined; throw error })
}

# Run 12 architecture: saved Windows RDP credentials

This is a scoped, source-only incremental audit of the one-click RDP credential path frozen at 2725d51386f930d54f71ddd8fcae5445f110730a. Its exact changed-path review base is 9ea1404407dbd89d3d2eeb02934257b870278fb3; the workflow coverage comparison base remains 4c0694aa8e02e46145a775707b8d5a0355f7ce10. The direct parent is 6cac7e9e7211b25c70020658df64bf74ca547d33.

The Panel accepts a save, status, or clear request through the authenticated desktop-session route. The handlers use the authenticated user identity, current user credential version, host ID, and digest of the enrolled node Noise key to form the vault binding. The credential store persists only XChaCha20-Poly1305 ciphertext with those binding fields as associated data. Retrieval is used when opening a desktop session and returns a no-store response bound to that one-use session; the browser forwards the Windows account to IronRDP/WASM memory and clears the related JavaScript fields. The Panel and Windows node then use the existing dedicated Noise desktop stream.

Credential clearing is scoped to the current user and host. Deleting a host requests cleanup for all credential records for that host. If the credential store cannot open, credential saves/retrievals fail closed; the source hunter noted that host deletion can still proceed without vault cleanup in this unavailable-store condition, but found no application retrieval or same-identity rebind path and did not promote it as a security candidate.

This run reviewed the 16 paths changed by the one-click credential increment. It did not re-audit the full 188-path scoped history and does not claim scope_complete.

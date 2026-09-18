# FINDINGS-DETAIL — KPanel security-audit run-1

## F1 (high, confirmed): hostbackup restore payload Root.Path unconfined to data model

**Fingerprint**: `hostbackup.restore.payload-root-unconfined-to-data-model`

### Ordered repository-relative trace

1. **entrypoint** — [internal/panel/backup_handlers.go:439](C:/GitHub/kejilion-panel/internal/panel/backup_handlers.go): panel HTTP handler (authenticated session + CSRF). Admin imports an attacker-supplied .kpb; multipart upload saved to upload.kpb; password is attacker-chosen (only 10-256 bytes enforced, no creator authentication).
2. **propagation** — backup_handlers.go:474 (backupImport worker): backup.ReadContext decrypts the container; inner module payload files stream to the Agent import job; inspect triggers.
3. **propagation** — internal/hostbackup/archive.go:254 (Engine.ReadPayload): `e.validatePayload(p)` is the only validation ever applied to the payload — no second, stronger validation exists at any later stage.
4. **propagation** — archive.go:393 (validatePayload root check): Root.Path is checked only for absolute form, Clean round-trip, and `!e.excluded()`. No positive allowlist, no owner() call, no module-vs-path consistency, no ID-derivation check.
5. **propagation** — internal/hostbackup/engine.go:159 (excluded()): negative list = /proc, /sys, /dev, /run, /var/run, /etc/kejilion-panel, /var/lib/kejilion-panel, /home/docker/kpanel + .kpanel-* name filters. Bidirectional within() excludes parents (literal /etc and /var/lib caught) but /etc/ssh, /root, /usr/local/bin, /var/lib/docker pass.
6. **propagation** — internal/hostbackup/service.go:312 (Service.create action=restore): requires only import Status ready + destination Inventory revision match — the revision binds host state, never payload root paths.
7. **propagation** — internal/hostbackup/restore.go:95 (mount-coverage loop): with zero declared containers (no minimum enforced) the loop is a no-op; even with containers it checks payload mounts against payload roots only, never against the destination host.
8. **propagation** — restore.go:127 (destination checks): the only inventory-derived gates are protectionRootOverlap (Panel/Agent runtime dirs), protected container names, userns, destination-writer overlap. current.Roots is never compared to payload roots; /root, /etc/ssh, /usr/local/bin, /var/lib/docker pass whenever nothing bind-mounts them (the default).
9. **propagation** — restore.go:310 (swap): `os.Rename(entry.Target, entry.Previous)` moves the real target to `<target>.kpanel-restore-<id>`; copyTree had populated `.kpanel-next-<id>` from the archive; `os.Rename(entry.Next, entry.Target)` installs attacker content at the target.
10. **propagation** — restore.go:746 (cleanupRecovery): after success, `removeDataTree(r.Previous)` permanently deletes the original host directory.
11. **sink** — restore.go:654 + owner_unix.go:11-18 + archive.go:297,440-465: restored files receive archived uid/gid via os.Chown and full mode via os.Chmod; archiveMode maps 04000/02000 to setuid/setgid; ReadPayload accepts mode 0..07777. e.host() is filepath.Join(Engine.Root, path) with Engine.Root hard-coded to '/' in production (engine.go:99,110-112); the agent runs as host root (deploy/systemd/kejilion-agent.service:8, User=root, ProtectSystem=false).

### Dummy attacker/principal and affected dummy resource

- **Attacker**: third-party .kpb author who knows their own archive password (the only secret the import requires).
- **Victim principal**: single panel admin performing a routine import + restore via Backup Center (web) or the root-terminal `backup` CLI menu.
- **Affected resource**: any non-excluded host directory outside the /home data model — /root (including .ssh/authorized_keys), /etc/ssh (sshd_config), /etc/cron.d, /usr/local/bin, /var/lib/docker.

### Native input and exact bounded instructions (target-neutral)

**Inner payload** (gzip+tar, hostbackup layout): first tar entry `manifest.json` (≤4MiB):

```json
{"version":1,"module":"apps",
 "roots":[{"id":"<32-hex>","path":"/root","module":"apps","directory":true,"bytes":64,"entries":2}],
 "containers":[],"networks":{},"volumes":{}}
```

followed by entries `data/<id>/` (dir) and `data/<id>/.ssh/authorized_keys` (regular file, uid 0, gid 0, mode 0600). Variants: path `/etc/ssh` (replace sshd_config), `/etc/cron.d` (plant a cron file), `/usr/local/bin` (plant a binary), `/var/lib/docker` (poison Docker state).

**Outer container**: standard encrypted .kpb format (backup.Write layout) with manifest.json Parts=[{module:'apps'}] and apps.payload — or export a genuine backup and substitute the module payload before re-encrypting with the attacker's password.

**Bounded reproduction (NOT executed in this run — source-settled)**:

1. *Local harness (bounded, dummy data)*: extend the existing Docker-backed integration test pattern (internal/hostbackup/native_integration_test.go) — craft the payload above with a sacrificial target directory; run ReadPayload/validatePayload and assert acceptance; run Engine.Restore and assert the swap completes and cleanup removes the original. Repeat validation alone with Root.Path /root to prove no rejection fires.
2. *Owner-observed deployment check (disposable VM)*: Panel → Backup Center → Import, upload the crafted .kpb with the attacker password; observe inspect succeeds; click Preview (notice: no root paths shown — part of the finding); confirm Restore with module apps; verify on the host that the target directory was replaced and its original deleted. Use a sacrificial directory first; /etc-adjacent targets only on a throwaway image.

**Observed result (source-settled)**: reading the full call chain establishes deterministically that validatePayload accepts the crafted root; no later stage in Engine.Restore compares payload roots to the destination inventory or data model; the swap and cleanup deletion apply to the real host path running as host root; restored files receive archived uid/gid (owner_unix.go:11-18) and modes including setgid/sticky. setuid preservation depends on the systemd release honoring RestrictSUIDSGID=true (agent unit line 53) — but uid/gid-0 arbitrary-mode content in /root, /etc/ssh, /etc/cron.d, /usr/local/bin is unaffected and independently sufficient for host takeover via authorized_keys or cron.

### Conditions and containment

- Requires authenticated admin session (or root CLI) + operator importing and restoring the attacker-supplied file with its password.
- Target must not be a mountpoint nor contain a nested mount (default for the listed paths); no existing container bind-mount may overlap it; destination inventory revision unchanged between preview and restore.
- Containment that DOES hold: panel-owned dirs and their parents are excluded; runtime-protection (protection.go) blocks Panel/Agent impersonation; backup single-writer gates and backupMutationMu serialize the flow; the literal /etc and /var/lib are excluded as parents.

### Source-level remediation and regression case

1. **internal/hostbackup/archive.go (validatePayload)**: for each root require `r.ID == hex(sha256(r.Path)[:16])` (exporter convention, engine.go:384-385) and require the path to lie inside the declared module's data model — web: within /home/web; apps: within /home with owner()==apps; docker: path must equal a bind Source or resolved volume Mountpoint of a container declared in the same payload (zero containers ⇒ zero roots).
2. **internal/hostbackup/restore.go**: after e.Inventory (line 123), before staging, require every payload root path to be covered by the destination inventory (current.Roots, /home-derived candidates, prepareVolumes-resolved mountpoints); reject otherwise.
3. **internal/hostbackup/service.go + BackupCenter.vue + backup_bundle.go**: include payload root paths (path, module, bytes) in the inspect/preview response and display them in both confirmation UIs.
4. **internal/hostbackup/backup_test.go**: regression cases asserting validatePayload/Restore reject roots at /root, /etc/ssh and any path outside the module model, including zero-container payloads.

### Severity rationale

- **likelihood medium**: social-engineering/untrusted-backup-source precondition; no auth bypass; but the confirmation UI never shows the target path, the archive carries its own password, and the flow is indistinguishable from a routine restore.
- **impact critical**: arbitrary content placement and deletion as host root outside the backup data model with attacker-chosen uid/gid and modes; direct root code execution via authorized_keys/sshd_config/cron.
- **overall high** (cannot exceed demonstrated impact; likelihood gated by operator interaction).
- **confidence high**: full chain source-traced end-to-end; Phase-3 adversarial verification and Phase-5 file-level record check both passed with no refutation.

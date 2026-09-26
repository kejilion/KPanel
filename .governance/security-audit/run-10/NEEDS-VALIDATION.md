# Needs validation

## Failed update rollback can restore a revoked light node's report credential

- Fingerprint: kpanel:rollback-restores-revoked-light-node
- Verdict: needs_validation; no severity assigned.
- Affected boundary: app update snapshot/rollback and cluster light-node report authentication.

### Source trace

1. packaging/kejilion-app/kpanel.conf:1205 — kpanel_prepare_automatic_transaction stops writers and prepares the snapshot.
2. packaging/kejilion-app/kpanel.conf:1934 — the target panel starts before readiness and transaction completion.
3. internal/panel/cluster.go:270 — an authenticated cluster mutation can delete a light host during target verification.
4. internal/cluster/light_store.go:559 — deletion removes the host record and reporting credential.
5. packaging/kejilion-app/kpanel.conf:1397 — a failed verification invokes rollback.
6. packaging/kejilion-app/kpanel.conf:1266 — restore replaces live panel data with the snapshot directory.
7. internal/cluster/light_service.go:258 — report authentication loads the current host and secret and checks request freshness and HMAC.
8. internal/cluster/light_service.go:207 — accepted reports update host telemetry.

### Evidence

- packaging/kejilion-app/kpanel.conf:1206 snapshots data/panel and data/agent after stopping writers and checks the archive.
- packaging/kejilion-app/kpanel.conf:1934 starts the target panel before readiness checks finish.
- internal/panel/cluster.go:283 dispatches an authorized cluster-host deletion after mutation checks.
- internal/cluster/light_store.go:559 and :565–568 remove a light-host record and credentials.
- packaging/kejilion-app/kpanel.conf:1266 exchanges the current panel directory for the snapshot; it does not merge post-snapshot revocations.
- internal/cluster/light_service.go:270 compares the HMAC against the secret loaded for the current host record; :207 persists accepted telemetry.
- packaging/tests/app-conf-update-backups.sh:114 asserts generic restoration of a preseeded light-node credential file, but does not test report authentication after deletion.

### Blocker

No parent-approved OS-enforced sandbox was available. Source review cannot establish that rollback restores an executable light store which accepts a fresh dummy-node HMAC report and persists telemetry.

### Local validation plan

In an approved offline OS-enforced sandbox, use a temporary data directory and synthetic light-host record with a generated dummy HMAC key. Snapshot the data, start the target panel, delete the dummy host during readiness verification, force readiness failure, perform rollback, and submit a fresh signed dummy report. Observe whether authentication succeeds and telemetry changes. Use an empty allowlisted environment, read-only target/tools, scratch-only writes, no external networking, and low CPU, memory, process, disk, file-size, and wall-clock limits.

No deployment check is needed for this source claim; the local dummy fixture can settle it. Do not send test traffic to a deployed system.
# NEEDS VALIDATION — KPanel security-audit run-1

No needs_validation records were produced by this run.

All 45 coverage units settled at the source level. The single confirmed finding (hostbackup.restore.payload-root-unconfined-to-data-model, high) is source-settled and carries no unresolved blocker; its bounded local harness and owner-observed deployment checks are documented in FINDINGS-DETAIL.md as reproduction steps, not as open validation questions.

Host-state-dependent conditions on the confirmed finding (no nested mount under the target directory; no existing container bind-mount overlap) are characterized in the finding's conditions; they hold by default on stock hosts for /root, /etc/ssh, /etc/cron.d, /usr/local/bin, /var/lib/docker.

Future runs should re-examine: (1) the sandbox limitation noted in REPORT.md — dynamic confirmation of the confirmed finding via the local harness remains available to an owner; (2) the deployment-dependent facts flagged in hardening notes (trusted-proxy appending behavior, systemd RestrictSUIDSGID release behavior affecting setuid propagation in the finding).

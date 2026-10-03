# Needs validation — run 12

There are no source-grounded candidate findings requiring decisive dynamic validation in this increment. The source-only review did not execute code, tests, build, or browser flows because no OS-enforced sandbox was available.

Deployment fact: effective ACLs on the configured Panel `DataDir` and protection of the sibling key/ciphertext files must be checked on the supported deployment filesystem. This is not asserted as an exposure.

Hardening follow-up: if `desktopcredentials.Open` fails, host deletion can proceed while vault `DeleteHost` cleanup is skipped. No application retrieval while the vault is unavailable or same-ID/same-key rebind path was demonstrated. Preserve a pending cleanup/retry outcome; this is not reported as a vulnerability.
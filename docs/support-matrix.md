# Support Matrix

The table distinguishes the production target from an experimental published architecture. Production support for a version is established only after its Debian 13 amd64 package has complete exact-artifact evidence.

Current blockers and the mandatory evidence gate are tracked in `docs/production-readiness.md`; candidate results belong in `docs/validation-record.md`.

| Component | Supported | Notes |
| --- | --- | --- |
| Distribution | Debian 13 (`trixie`) | Exact production target. |
| Architecture | `amd64` production; `arm64` experimental | Packages and installation guards match their architecture. Runtime status reports the support tier. |
| Init/service manager | systemd | Required for activation, rollback, boot reconciliation, and DDNS scheduling. |
| Firewall | Debian 13 nftables | `inet hostfw` only. |
| Privilege | Root | Required for install and live policy operations. |
| Docker | Optional | Strict gate; Docker nftables backend migration remains operator-controlled. |
| IPv4/IPv6 | Both | DDNS IPv6 derivation supports `/56` and `/64`. |
| Remote operation | Supported with recovery | Console/rescue access and mandatory dead-man rollback are operational requirements. |

Explicitly unsupported or untested:

- Debian 12 or earlier, Debian testing beyond 13, Ubuntu, Fedora, RHEL, Alpine, and derivatives.
- `386` and architectures other than `amd64` and `arm64`.
- OpenRC, runit, containers without systemd, and non-systemd hosts.
- Arbitrary nftables profiles, multiple managed tables, or coexistence with another owner of `inet hostfw`.
- Docker backends or network modes not validated in the exact release evidence.
- Kubernetes/CNI policy, router/firewall appliances, and systems where cnftctl acts as a complete forwarding firewall.
- Binary-only installation without the matching Debian package units and integrity manifest.

An unsupported result from `status` or `doctor` is intentional and exits `1`. It does not imply that the CLI can safely apply policy on that platform.

## Experimental arm64

Debian 13 arm64 is available but is not production-supported and carries no production or security-support guarantee. It remains experimental because no exact released or candidate arm64 package has completed live activation, timeout rollback, confirmation, reboot recovery, DDNS, Docker coexistence, upgrade, and uninstall validation on a disposable Debian 13 arm64 host with independent console or rescue access. Native CI is compatibility evidence, not a substitute for that gate.

Arm64 graduates to production support only after one exact released or candidate arm64 package completes the full `docs/manual-validation.md` checklist on such a host, with its filename and SHA-256 recorded. Until then, every live arm64 validation item is `NOT EXERCISED`, never `PASS`, and use is at the operator's own risk.

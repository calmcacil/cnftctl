# Security Policy

## Supported Versions

Only the latest published release on the supported Debian 13 amd64 target receives security fixes. The repository development branch and unpublished artifacts are not production support channels.

## Reporting

Report suspected vulnerabilities privately through GitHub's **Security** tab using **Report a vulnerability** (private vulnerability reporting), when available. Include the affected version and artifact checksum, target environment, impact, reproduction steps, relevant sanitized logs, and whether remote access or rollback is affected.

Do not open a public issue for an unpatched vulnerability. Do not include credentials, private hostnames, real IP allowlists, transaction detail, or exploit traffic from systems you do not own. If private vulnerability reporting is unavailable, contact the repository owner through the private contact method listed on the GitHub profile and request a secure reporting channel; do not send exploit details until one is established.

## Response

Maintainers will acknowledge receipt when the private channel is monitored, validate scope, coordinate remediation and disclosure, and credit reporters who request it. No fixed response or remediation SLA is promised. Firewall lockout or an active incident should be handled with `docs/incident-response.md`; a vulnerability report is not an emergency recovery channel.

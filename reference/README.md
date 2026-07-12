# Reference nftables Docker firewall with DDNS SSH whitelist

> **DO NOT DEPLOY OR EXECUTE THIS DIRECTORY AS A PRODUCTION FIREWALL.** This is a sanitized compatibility reference for review only. It omits cnftctl's mandatory exact-generation validation, durable transactions, fixed rollback, boot reconciliation, ownership checks, and active-generation DDNS scheduling. The only intended delivery unit is the canonical `cnftctl-VERSION-debian13-amd64.tar.gz` bundle, whose production status remains **NOT READY** pending exact Debian 13 amd64 evidence.

This folder is a sanitized historical behavior baseline for a default-deny nftables
firewall. Its files are intentionally not an installation or operations guide.

It contains no real domains, API tokens, or personal whitelist addresses.

## Layout

```text
reference/
├── README.md
├── nftables.conf
├── nftables.d/
│   ├── open-ports.nft
│   └── whitelist.nft
├── ddns-whitelist/
│   ├── ddns-hosts.conf
│   ├── nft-ddns-whitelist.service
│   ├── nft-ddns-whitelist.timer
│   └── update-nft-ddns-whitelist
└── cloudflare-ddns.sh
```

## Firewall model

### Host input

The `input` chain is default-deny:

```nft
type filter hook input priority filter; policy drop;
```

Allowed input paths:

- ICMP/ICMPv6 for diagnostics, IPv6 NDP, and Path MTU Discovery
- trusted VPN/overlay interfaces such as `tailscale0`
- established/related connections
- loopback
- Docker bridge-originated traffic to host services
- static SSH whitelist entries from `whitelist.nft`
- dynamic DDNS SSH whitelist entries from runtime nftables sets
- explicit public WAN service ports from `open_ports.nft`

Non-whitelisted public SSH is blocked by default.

### Public host services

`open_ports.nft` defines one shared set:

```nft
set open_ports {
    typeof meta l4proto . th dport
    flags interval
    elements = {
        # empty by default
    }
}
```

For host services, the rule is scoped to the WAN interface:

```nft
iifname $WAN_IF meta l4proto . th dport @open_ports accept
```

### Docker published services

Docker may still publish ports, but WAN access is gated before Docker's normal
forwarding rules can allow traffic.

IPv4 Docker-published traffic is allowed only when the original public destination
port is listed in `open_ports`:

```nft
meta nfproto ipv4 iifname $WAN_IF oifname $DOCKER_IFS ct status dnat jump docker_wan_allow
meta l4proto . ct original proto-dst @open_ports accept
```

IPv6 Docker traffic supports both DNAT and routed container addresses:

```nft
ct status dnat meta l4proto . ct original proto-dst @open_ports accept
meta l4proto . th dport @open_ports accept
```

Anything else from WAN to Docker bridges is dropped.

### Forwarding policy

The forward hook chain uses:

```nft
policy accept;
```

This chain is a WAN-to-Docker gate, not a complete router firewall. The accept
policy preserves Docker's own forwarding behavior while still dropping unauthorized
WAN-to-Docker traffic matching the gate conditions.

## SSH access model

SSH is allowed from:

1. trusted VPN interfaces, e.g. `tailscale0`
2. static IPs/prefixes in `nftables.d/whitelist.nft`
3. dynamic DDNS-resolved IPs/prefixes in:
   - `ddns_whitelist_v4`
   - `ddns_whitelist_v6`

SSH is blocked from all other public sources.

## DDNS whitelist updater

The server-side updater is:

```text
ddns-whitelist/update-nft-ddns-whitelist
```

It resolves hostnames from:

```text
/etc/nftables.d/ddns-hosts.conf
```

Then updates runtime nftables sets without reloading the firewall:

```nft
set ddns_whitelist_v4 { type ipv4_addr; flags timeout; timeout 1h; }
set ddns_whitelist_v6 { type ipv6_addr; flags interval, timeout; timeout 1h; }
```

Behavior:

- A records are added as exact IPv4 addresses.
- AAAA records are converted into an IPv6 prefix.
- The default IPv6 prefix length is `/56`, suitable for DHCPv6-PD delegations.
- Change `IPV6_PREFIXLEN="56"` to `64` if you only want to trust one LAN `/64`.
- Entries expire after 1 hour if the updater stops.
- The systemd timer refreshes entries every 5 minutes.

Dependencies:

```text
nft getent awk sort systemd
```

No Python or jq is required.

## EdgeRouter / Cloudflare DDNS

`cloudflare-ddns.sh` is an optional EdgeOS/EdgeRouter script that updates:

- `A home.example.com` from public IPv4 via `api.ipify.org`
- `AAAA home.example.com` from a global IPv6 address on a configured LAN interface

The Cloudflare record must be DNS-only, not proxied.

The script expects the A and AAAA records to already exist in Cloudflare.

Dependencies on EdgeOS:

```text
sh curl ip awk logger
```

No Python or jq is required.

## No Supported Manual Deployment

Do not copy these files into `/etc`, enable their reference timers, or execute the updater scripts as a cnftctl deployment. They remain readable solely to compare sanitized firewall behavior. Use only a verified canonical release bundle after its exact-artifact evidence gate passes.

## Security notes

- Do not commit real Cloudflare API tokens.
- Use a scoped Cloudflare token limited to DNS edits for the intended zone.
- Any hostname in `ddns-hosts.conf` becomes part of the SSH trust boundary.
- Protect DDNS and DNS provider accounts with MFA where possible.
- Keep `open_ports.nft` minimal; every listed port is public for both host and Docker services.

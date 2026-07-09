#!/bin/sh
#
# Cloudflare DDNS updater for EdgeOS / EdgeRouter.
#
# Updates:
# - A record from public IPv4 discovered via api.ipify.org
# - AAAA record from a global IPv6 address on IPV6_IF
#
# Dependencies intentionally kept minimal for EdgeOS:
# - sh, curl, ip, awk, logger
#
# No jq/python required.
#
# For the firewall DDNS whitelist flow, the AAAA record only needs to point at
# one current IPv6 address inside your delegated prefix. The server-side updater
# derives the configured DHCPv6-PD prefix, for example /56, from that address.

TOKEN="YOUR_CLOUDFLARE_API_TOKEN"
ZONE="example.com"
RECORD="home.example.com"

# Interface that has an IPv6 address inside your delegated prefix.
# Common EdgeRouter LAN interfaces: switch0, eth1, br0.
IPV6_IF="switch0"

CACHE_V4="/config/cloudflare-ddns.ipv4"
CACHE_V6="/config/cloudflare-ddns.ipv6"
API="https://api.cloudflare.com/client/v4"

log() {
    logger -t cloudflare-ddns "$*"
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
        log "Required command not found: $1"
        exit 1
    }
}

cf_get() {
    curl -fsS \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        "$1"
}

cf_patch() {
    url="$1"
    data="$2"

    curl -fsS -X PATCH \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        "$url" \
        --data "$data"
}

json_first_id() {
    # Extract the first JSON "id" value from Cloudflare responses.
    # This is intentionally small and dependency-light for EdgeOS. It is not a
    # general JSON parser, but is sufficient for the specific API responses
    # queried by this script.
    awk -F'"' '
        {
            for (i = 1; i <= NF; i++) {
                if ($i == "id") {
                    print $(i + 2)
                    exit
                }
            }
        }
    '
}

json_success_true() {
    # Cloudflare emits compact JSON containing "success":true on success.
    awk 'BEGIN { ok = 1 } /"success"[[:space:]]*:[[:space:]]*true/ { ok = 0 } END { exit ok }'
}

get_public_ipv4() {
    curl -4 -fsS https://api.ipify.org
}

get_lan_ipv6() {
    # Pick the first global IPv6 address on IPV6_IF, excluding temporary/privacy,
    # deprecated, tentative, and ULA/link-local addresses.
    ip -6 addr show dev "$IPV6_IF" scope global 2>/dev/null | \
        awk '
            /inet6/ {
                addr = $2
                flags = $0
                sub(/\/.*$/, "", addr)

                if (addr ~ /^fc/ || addr ~ /^fd/ || addr ~ /^fe80:/) next
                if (flags ~ /temporary/) next
                if (flags ~ /deprecated/) next
                if (flags ~ /tentative/) next

                print addr
                exit
            }
        '
}

lookup_zone_id() {
    cf_get "$API/zones?name=$ZONE" | json_first_id
}

lookup_record_id() {
    record_type="$1"

    cf_get "$API/zones/$ZONE_ID/dns_records?type=$record_type&name=$RECORD" | json_first_id
}

update_record() {
    record_type="$1"
    record_id="$2"
    record_ip="$3"

    cf_patch \
        "$API/zones/$ZONE_ID/dns_records/$record_id" \
        "{\"type\":\"$record_type\",\"name\":\"$RECORD\",\"content\":\"$record_ip\",\"ttl\":1,\"proxied\":false}" | \
        json_success_true
}

require_cmd curl
require_cmd ip
require_cmd awk
require_cmd logger

if [ -z "$TOKEN" ] || [ "$TOKEN" = "YOUR_CLOUDFLARE_API_TOKEN" ]; then
    log "Cloudflare API token is not configured."
    exit 1
fi

CURRENT_IPV4=$(get_public_ipv4) || CURRENT_IPV4=""
[ -n "$CURRENT_IPV4" ] || {
    log "Unable to determine public IPv4."
    exit 1
}

CURRENT_IPV6=$(get_lan_ipv6) || CURRENT_IPV6=""
if [ -z "$CURRENT_IPV6" ]; then
    log "Unable to determine global IPv6 on interface $IPV6_IF. Will update IPv4 only."
fi

LAST_IPV4=""
LAST_IPV6=""
[ -f "$CACHE_V4" ] && LAST_IPV4=$(cat "$CACHE_V4")
[ -f "$CACHE_V6" ] && LAST_IPV6=$(cat "$CACHE_V6")

NEED_V4="no"
NEED_V6="no"

[ "$CURRENT_IPV4" != "$LAST_IPV4" ] && NEED_V4="yes"
[ -n "$CURRENT_IPV6" ] && [ "$CURRENT_IPV6" != "$LAST_IPV6" ] && NEED_V6="yes"

if [ "$NEED_V4" = "no" ] && [ "$NEED_V6" = "no" ]; then
    exit 0
fi

ZONE_ID=$(lookup_zone_id)
[ -n "$ZONE_ID" ] || {
    log "Unable to find zone '$ZONE'."
    exit 1
}

if [ "$NEED_V4" = "yes" ]; then
    RECORD_ID_A=$(lookup_record_id A)
    [ -n "$RECORD_ID_A" ] || {
        log "Unable to find A record '$RECORD'."
        exit 1
    }

    if update_record A "$RECORD_ID_A" "$CURRENT_IPV4"; then
        echo "$CURRENT_IPV4" > "$CACHE_V4"
        log "Updated A $RECORD -> $CURRENT_IPV4"
    else
        log "Cloudflare A record update failed."
        exit 1
    fi
fi

if [ "$NEED_V6" = "yes" ]; then
    RECORD_ID_AAAA=$(lookup_record_id AAAA)
    [ -n "$RECORD_ID_AAAA" ] || {
        log "Unable to find AAAA record '$RECORD'. Create it in Cloudflare first."
        exit 1
    }

    if update_record AAAA "$RECORD_ID_AAAA" "$CURRENT_IPV6"; then
        echo "$CURRENT_IPV6" > "$CACHE_V6"
        log "Updated AAAA $RECORD -> $CURRENT_IPV6"
    else
        log "Cloudflare AAAA record update failed."
        exit 1
    fi
fi

exit 0

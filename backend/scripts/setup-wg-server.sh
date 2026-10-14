#!/usr/bin/env bash
# Bootstraps a WireGuard interface on a fresh Linux VPN server so the agent
# (cmd/agent) can manage its peers. Run as root, once per server.
#
#   WG_INTERFACE=wg0 WG_SUBNET=10.8.0.0/24 WG_PORT=51820 ./setup-wg-server.sh
#
# Safe to re-run on an existing server: it keeps the keys and the IPv4
# address but adds what older versions of this script did not set (IPv6 in
# the tunnel, MSS clamping, return traffic, kernel buffers), without
# restarting the interface.
#
# WG_CLIENT_MTU must match WG_MTU on the API (default 1280), and
# WG_IPV6_PREFIX must match WG_IPV6_PREFIX on the API ("off" on both to
# disable IPv6 in the tunnel).
#
# Prints the interface public key: paste it into the `wg_public_key` field
# when creating the server in the API (POST /api/v1/server).
set -euo pipefail

WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_SUBNET="${WG_SUBNET:-10.8.0.0/24}"
WG_PORT="${WG_PORT:-51820}"
WG_CLIENT_MTU="${WG_CLIENT_MTU:-1280}"
WG_IPV6_PREFIX="${WG_IPV6_PREFIX:-fd7a:5f1c:9e42::/64}"
WG_DIR="/etc/wireguard"
CONF="$WG_DIR/$WG_INTERFACE.conf"

if [[ $EUID -ne 0 ]]; then
  echo "run as root" >&2
  exit 1
fi

if ! command -v wg >/dev/null 2>&1; then
  echo "installing wireguard-tools..."
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq wireguard-tools
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y wireguard-tools
  else
    echo "install wireguard-tools manually" >&2
    exit 1
  fi
fi

# Gateway = first usable address of the subnet (matches wireguard.Pool.Gateway()).
NETWORK="${WG_SUBNET%/*}"
PREFIX="${WG_SUBNET#*/}"
IFS=. read -r o1 o2 o3 o4 <<<"$NETWORK"
GATEWAY="$o1.$o2.$o3.$((o4 + 1))/$PREFIX"

# IPv6 gateway: first address of the prefix (the API derives device
# addresses as <prefix>::<device IPv4>, see wireguard.DeviceIPv6).
GATEWAY6=""
if [[ "$WG_IPV6_PREFIX" != off ]]; then
  NETWORK6="${WG_IPV6_PREFIX%/*}"
  if [[ "$NETWORK6" != *:: ]]; then
    echo "WG_IPV6_PREFIX must end with '::' (e.g. fd7a:5f1c:9e42::/64)" >&2
    exit 1
  fi
  GATEWAY6="${NETWORK6}1/${WG_IPV6_PREFIX#*/}"
  if ! command -v ip6tables >/dev/null 2>&1; then
    echo "ip6tables is missing (install iptables), or set WG_IPV6_PREFIX=off here and on the API" >&2
    exit 1
  fi
fi

# Default route interfaces, used for NAT.
WAN_IF="$(ip route show default | awk '/default/ {print $5; exit}')"
WAN6_IF="$(ip -6 route show default 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "dev") {print $(i + 1); exit}}' || true)"

# Can this server reach the IPv6 internet? Without a global address and a
# default route, IPv6 from the clients cannot go out.
HAS_IPV6_EGRESS=false
if [[ -n "$WAN6_IF" ]] && ip -6 addr show dev "$WAN6_IF" scope global | grep -q inet6; then
  HAS_IPV6_EGRESS=true
fi

# TCP MSS announced through the tunnel: client MTU minus the IP + TCP headers
# (40 bytes in IPv4, 60 in IPv6). Clamping both directions keeps every
# segment small enough to be encapsulated without fragmentation, even for
# clients still using an older config without MTU (Android then defaults to
# 1500). The tcpmss match only lowers the MSS, never raises it.
mss_match() {
  local mss=$((WG_CLIENT_MTU - $1))
  echo "-p tcp --tcp-flags SYN,RST SYN -m tcpmss --mss $((mss + 1)):65535 -j TCPMSS --set-mss $mss"
}

# Firewall rules owned by the interface, as "tool|table|rule" (applied with
# -A on PostUp and -D on PostDown).
RULES=(
  "iptables|nat|POSTROUTING -o $WAN_IF -j MASQUERADE"
  "iptables|filter|FORWARD -i $WG_INTERFACE -j ACCEPT"
  "iptables|filter|FORWARD -o $WG_INTERFACE -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
  "iptables|mangle|FORWARD -i $WG_INTERFACE $(mss_match 40)"
  "iptables|mangle|FORWARD -o $WG_INTERFACE $(mss_match 40)"
)
if [[ -n "$GATEWAY6" ]]; then
  if $HAS_IPV6_EGRESS; then
    # Clients have ULA addresses: NAT66 them behind the server's IPv6.
    RULES+=(
      "ip6tables|nat|POSTROUTING -s $WG_IPV6_PREFIX -o $WAN6_IF -j MASQUERADE"
      "ip6tables|filter|FORWARD -i $WG_INTERFACE -j ACCEPT"
      "ip6tables|filter|FORWARD -o $WG_INTERFACE -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
      "ip6tables|mangle|FORWARD -i $WG_INTERFACE $(mss_match 60)"
      "ip6tables|mangle|FORWARD -o $WG_INTERFACE $(mss_match 60)"
    )
  else
    # No IPv6 here: refuse it immediately so apps switch to IPv4 at once
    # instead of waiting for a timeout.
    RULES+=("ip6tables|filter|FORWARD -i $WG_INTERFACE -j REJECT --reject-with icmp6-adm-prohibited")
  fi
fi

rule_tool() { echo "${1%%|*}"; }
rule_table() {
  local rest="${1#*|}"
  echo "${rest%%|*}"
}
rule_args() { echo "${1##*|}"; }

hook_lines() {
  local rule
  for rule in "${RULES[@]}"; do
    echo "PostUp   = $(rule_tool "$rule") -t $(rule_table "$rule") -A $(rule_args "$rule")"
  done
  for rule in "${RULES[@]}"; do
    echo "PostDown = $(rule_tool "$rule") -t $(rule_table "$rule") -D $(rule_args "$rule")"
  done
}

mkdir -p "$WG_DIR"
chmod 700 "$WG_DIR"

if [[ -f "$CONF" ]]; then
  echo "$CONF already exists, keeping its keys and IPv4 address"
else
  umask 077
  PRIVATE_KEY="$(wg genkey)"
  cat >"$CONF" <<EOF
[Interface]
PrivateKey = $PRIVATE_KEY
Address = $GATEWAY${GATEWAY6:+, $GATEWAY6}
ListenPort = $WG_PORT
$(hook_lines)

# Peers are managed at runtime by the agent (cmd/agent), do not add them here.
EOF
  echo "wrote $CONF"
fi

# Existing interface created by an older version of this script: add what is
# missing so it survives a restart.
if [[ -n "$GATEWAY6" ]] && ! grep -q '^Address = .*:' "$CONF"; then
  sed -i "s|^Address = .*|&, $GATEWAY6|" "$CONF"
  echo "added $GATEWAY6 to the interface address in $CONF"
fi
missing=""
while IFS= read -r line; do
  grep -qxF -- "$line" "$CONF" || missing+="$line"$'\n'
done < <(hook_lines)
# The first version wrote MASQUERADE and FORWARD -i on one combined line:
# keep it rather than duplicating those two rules.
if grep -q 'MASQUERADE; iptables -A FORWARD' "$CONF"; then
  missing="$(grep -v -e '= iptables -t nat -. POSTROUTING' \
    -e "= iptables -t filter -. FORWARD -i $WG_INTERFACE -j ACCEPT" <<<"$missing" || true)"
fi
missing="$(sed '/^$/d' <<<"$missing")"
if [[ -n "$missing" ]]; then
  tmp="$(mktemp)"
  echo "$missing" >"$tmp"
  # After ListenPort, so the hooks stay grouped after the interface settings.
  sed -i "/^ListenPort/r $tmp" "$CONF"
  grep -qxF -- "$(head -n1 "$tmp")" "$CONF" || sed -i "/^\[Interface\]/r $tmp" "$CONF"
  rm -f "$tmp"
  echo "added $(wc -l <<<"$missing") firewall hooks to $CONF"
fi

# Apply everything now on a running interface (wg-quick only reads the file
# on start, and restarting it would drop every peer until the next agent
# sync).
if ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
  if [[ -n "$GATEWAY6" ]] && ! ip -6 addr show dev "$WG_INTERFACE" | grep -q "inet6 ${GATEWAY6%/*}/"; then
    ip -6 addr add "$GATEWAY6" dev "$WG_INTERFACE"
  fi
  for rule in "${RULES[@]}"; do
    tool="$(rule_tool "$rule")"
    table="$(rule_table "$rule")"
    # shellcheck disable=SC2046 # the rule is a list of arguments
    "$tool" -t "$table" -C $(rule_args "$rule") 2>/dev/null ||
      "$tool" -t "$table" -A $(rule_args "$rule")
  done
fi

# IP forwarding and network tuning, persistent.
cat >/etc/sysctl.d/99-wireguard.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1

# Larger socket buffers: the WireGuard UDP socket carries every client, and
# the kernel defaults (~200 KB) drop packets in bursts on fast links.
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.core.rmem_default = 1048576
net.core.wmem_default = 1048576
# Queue more packets per CPU before dropping when traffic peaks.
net.core.netdev_max_backlog = 16384

# Fair queuing with pacing on egress: smooths bursts so one heavy flow does
# not add latency to the others (bufferbloat).
net.core.default_qdisc = fq
EOF
# With forwarding on, the kernel ignores router advertisements unless
# accept_ra is 2: a server configured by SLAAC would lose its IPv6 route.
if [[ -n "$WAN6_IF" && "$WAN6_IF" != *.* ]]; then
  echo "net.ipv6.conf.$WAN6_IF.accept_ra = 2" >>/etc/sysctl.d/99-wireguard.conf
fi
sysctl -q --system

systemctl enable --now "wg-quick@$WG_INTERFACE"

echo
echo "interface : $WG_INTERFACE ($GATEWAY${GATEWAY6:+, $GATEWAY6}, udp/$WG_PORT, client MTU $WG_CLIENT_MTU)"
if [[ -n "$GATEWAY6" ]]; then
  if $HAS_IPV6_EGRESS; then
    echo "ipv6      : routed out through $WAN6_IF (NAT66)"
  else
    echo "ipv6      : no IPv6 on this server, refused at once (clients fall back to IPv4)"
  fi
fi
echo "public key: $(wg show "$WG_INTERFACE" public-key)"
echo
echo "next: start the agent on this host"
echo "  AGENT_TOKEN=... WG_INTERFACE=$WG_INTERFACE ./wg-agent"

#!/usr/bin/env bash
# Bootstraps a WireGuard interface on a fresh Linux VPN server so the agent
# (cmd/agent) can manage its peers. Run as root, once per server.
#
#   WG_INTERFACE=wg0 WG_SUBNET=10.8.0.0/24 WG_PORT=51820 ./setup-wg-server.sh
#
# Prints the interface public key: paste it into the `wg_public_key` field
# when creating the server in the API (POST /api/v1/server).
set -euo pipefail

WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_SUBNET="${WG_SUBNET:-10.8.0.0/24}"
WG_PORT="${WG_PORT:-51820}"
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

# Default route interface, used for NAT.
WAN_IF="$(ip route show default | awk '/default/ {print $5; exit}')"

mkdir -p "$WG_DIR"
chmod 700 "$WG_DIR"

if [[ -f "$CONF" ]]; then
  echo "$CONF already exists, leaving it untouched"
else
  umask 077
  PRIVATE_KEY="$(wg genkey)"
  cat >"$CONF" <<EOF
[Interface]
PrivateKey = $PRIVATE_KEY
Address = $GATEWAY
ListenPort = $WG_PORT
PostUp   = iptables -t nat -A POSTROUTING -o $WAN_IF -j MASQUERADE; iptables -A FORWARD -i $WG_INTERFACE -j ACCEPT
PostDown = iptables -t nat -D POSTROUTING -o $WAN_IF -j MASQUERADE; iptables -D FORWARD -i $WG_INTERFACE -j ACCEPT

# Peers are managed at runtime by the agent (cmd/agent), do not add them here.
EOF
  echo "wrote $CONF"
fi

# IP forwarding, persistent.
cat >/etc/sysctl.d/99-wireguard.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
sysctl -q --system

systemctl enable --now "wg-quick@$WG_INTERFACE"

echo
echo "interface : $WG_INTERFACE ($GATEWAY, udp/$WG_PORT)"
echo "public key: $(wg show "$WG_INTERFACE" public-key)"
echo
echo "next: start the agent on this host"
echo "  AGENT_TOKEN=... WG_INTERFACE=$WG_INTERFACE ./wg-agent"

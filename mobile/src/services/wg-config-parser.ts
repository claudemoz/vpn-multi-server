import type { WireGuardConfig } from 'react-native-wireguard-vpn';

import { PrivateKeyPlaceholder } from '@/services/wg-constants';

// Used when the config has no MTU (configs saved before the API sent one).
// Without it Android keeps a 1500 MTU, too large once encapsulated: big
// packets get dropped and TCP stalls. Must match WG_MTU on the API.
const DEFAULT_MTU = 1280;

type WgSections = Record<string, Record<string, string>>;

function parseSections(configText: string): WgSections {
  const sections: WgSections = { Interface: {}, Peer: {} };
  let current = 'Interface';

  for (const raw of configText.split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;

    if (line.startsWith('[') && line.endsWith(']')) {
      current = line.slice(1, -1);
      sections[current] ??= {};
      continue;
    }

    const eq = line.indexOf('=');
    if (eq <= 0) continue;

    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    sections[current] ??= {};
    sections[current][key] = value;
  }

  return sections;
}

function splitList(value?: string): string[] {
  if (!value) return [];
  return value
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);
}

/** Convertit un fichier wg-quick (.conf) en config pour le module natif. */
export function wgQuickToNativeConfig(
  configText: string,
  privateKeyOverride?: string,
): WireGuardConfig {
  const sections = parseSections(configText);
  const iface = sections.Interface ?? {};
  const peer = sections.Peer ?? {};

  const privateKey = privateKeyOverride ?? iface.PrivateKey;
  if (!privateKey || privateKey === PrivateKeyPlaceholder) {
    throw new Error('Clé privée manquante. Reconnectez-vous ou recréez le device.');
  }

  const publicKey = peer.PublicKey;
  if (!publicKey) {
    throw new Error('Config WireGuard invalide : PublicKey du serveur manquant');
  }

  const endpoint = peer.Endpoint;
  if (!endpoint) {
    throw new Error('Config WireGuard invalide : Endpoint manquant');
  }

  const [serverAddress, portStr] = endpoint.split(':');
  const serverPort = Number.parseInt(portStr, 10);
  if (!serverAddress || Number.isNaN(serverPort)) {
    throw new Error(`Endpoint invalide : ${endpoint}`);
  }

  const allowedIPs = splitList(peer.AllowedIPs);
  if (allowedIPs.length === 0) {
    allowedIPs.push('0.0.0.0/0', '::/0');
  }

  const dns = splitList(iface.DNS);
  // Dual-stack configs list an IPv4 and an IPv6 address. Android only splits
  // an array, so pass one when there are several (iOS accepts both).
  const addresses = splitList(iface.Address);
  if (addresses.length === 0) {
    throw new Error('Config WireGuard invalide : Address manquante');
  }
  const address = addresses.length === 1 ? addresses[0] : addresses;

  // Android reads optional keys with hasKey() then getInt()/getArray(): a key
  // present with a null value crashes the native module, so omit it entirely.
  const config: WireGuardConfig = {
    privateKey,
    publicKey,
    serverAddress,
    serverPort,
    address,
    allowedIPs,
  };
  if (dns.length > 0) config.dns = dns;
  const mtu = iface.MTU ? Number.parseInt(iface.MTU, 10) : Number.NaN;
  config.mtu = Number.isFinite(mtu) ? mtu : DEFAULT_MTU;
  if (peer.PresharedKey) config.presharedKey = peer.PresharedKey;
  return config;
}

/** Injecte la clé privée dans une config wg-quick (remplace le placeholder). */
export function injectPrivateKey(configText: string, privateKey: string): string {
  if (configText.includes(PrivateKeyPlaceholder)) {
    return configText.replace(PrivateKeyPlaceholder, privateKey);
  }
  return configText.replace(
    /PrivateKey = .+/,
    `PrivateKey = ${privateKey}`,
  );
}

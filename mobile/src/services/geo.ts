import { NativeModules } from 'react-native';

export type PublicLocation = {
  /** Address shown to the user: IPv4 when available, otherwise IPv6. */
  ip: string;
  ipv4?: string;
  ipv6?: string;
  city?: string;
  region?: string;
  country?: string;
  countryCode?: string;
  isp?: string;
};

type Geo = Omit<PublicLocation, 'ip' | 'ipv4' | 'ipv6'>;

const IPV4_TIMEOUT_MS = 5000;
// IPv6 is dropped inside the tunnel when the server has none: fail fast.
const IPV6_TIMEOUT_MS = 3000;
const GEO_TIMEOUT_MS = 6000;

type FreshFetcher = (url: string, timeoutMs: number) => Promise<string>;

const nativeFetchFresh: FreshFetcher | undefined =
  NativeModules.WireGuardVpnModule?.fetchFresh?.bind(NativeModules.WireGuardVpnModule);

async function fetchJson(url: string, timeoutMs: number): Promise<any> {
  const sep = url.includes('?') ? '&' : '?';
  const fullUrl = `${url}${sep}_=${Date.now()}`;

  // iOS keeps connections opened before the VPN on the physical interface;
  // a fresh native session guarantees the request goes through the tunnel.
  if (nativeFetchFresh) {
    return JSON.parse(await nativeFetchFresh(fullUrl, timeoutMs));
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(fullUrl, {
      signal: controller.signal,
      headers: { Accept: 'application/json', 'Cache-Control': 'no-cache' },
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return await res.json();
  } finally {
    clearTimeout(timer);
  }
}

// These hosts only answer over one IP family, so each request reveals the
// address of that family (or fails when it is blocked, e.g. IPv6 inside the VPN).
async function fetchIp(url: string, timeoutMs: number): Promise<string | undefined> {
  try {
    const d = await fetchJson(url, timeoutMs);
    return typeof d?.ip === 'string' && d.ip ? d.ip : undefined;
  } catch {
    return undefined;
  }
}

async function geoFromIpInfo(ip: string): Promise<Geo> {
  const d = await fetchJson(`https://ipinfo.io/${encodeURIComponent(ip)}/json`, GEO_TIMEOUT_MS);
  if (!d?.country || d.bogon) throw new Error('ipinfo failed');
  return {
    city: d.city || undefined,
    region: d.region || undefined,
    country: countryName(d.country),
    countryCode: d.country,
    isp: typeof d.org === 'string' ? d.org.replace(/^AS\d+\s+/, '') : undefined,
  };
}

async function geoFromIpWho(ip: string): Promise<Geo> {
  const d = await fetchJson(`https://ipwho.is/${encodeURIComponent(ip)}`, GEO_TIMEOUT_MS);
  if (!d?.success) throw new Error(d?.message ?? 'ipwho.is failed');
  return {
    city: d.city || undefined,
    region: d.region || undefined,
    country: d.country || undefined,
    countryCode: d.country_code || undefined,
    isp: d.connection?.isp || d.connection?.org || undefined,
  };
}

async function geoFromGeoJs(ip: string): Promise<Geo> {
  const d = await fetchJson(`https://get.geojs.io/v1/ip/geo/${encodeURIComponent(ip)}.json`, GEO_TIMEOUT_MS);
  if (!d?.ip) throw new Error('geojs failed');
  return {
    city: d.city || undefined,
    region: d.region || undefined,
    country: d.country || undefined,
    countryCode: d.country_code || undefined,
    isp: d.organization_name || undefined,
  };
}

export async function geolocate(ip: string): Promise<Geo> {
  for (const provider of [geoFromIpInfo, geoFromIpWho, geoFromGeoJs]) {
    try {
      return await provider(ip);
    } catch {
      // try the next provider
    }
  }
  return {};
}

/** Starts both lookups at once; each promise resolves to undefined on failure. */
export function lookupPublicIps(): { ipv4: Promise<string | undefined>; ipv6: Promise<string | undefined> } {
  return {
    ipv4: fetchIp('https://api4.ipify.org?format=json', IPV4_TIMEOUT_MS),
    ipv6: fetchIp('https://api6.ipify.org?format=json', IPV6_TIMEOUT_MS),
  };
}

// iOS rotates temporary IPv6 addresses, but the /64 prefix identifies the
// home network and stays stable.
function ipv6Prefix64(ip: string): string | null {
  const [head, tail = ''] = ip.toLowerCase().split('::');
  const left = head ? head.split(':') : [];
  const right = ip.includes('::') ? (tail ? tail.split(':') : []) : [];
  const groups = ip.includes('::')
    ? [...left, ...Array(Math.max(0, 8 - left.length - right.length)).fill('0'), ...right]
    : left;
  if (groups.length !== 8) return null;
  return groups
    .slice(0, 4)
    .map((g) => g.replace(/^0+(?=.)/, ''))
    .join(':');
}

/** True when `current` still exposes one of the addresses seen without the VPN. */
export function sharesAddress(current: PublicLocation, real: PublicLocation): boolean {
  if (current.ipv4 && current.ipv4 === real.ipv4) return true;
  if (current.ipv6 && real.ipv6) {
    const a = ipv6Prefix64(current.ipv6);
    return a !== null && a === ipv6Prefix64(real.ipv6);
  }
  return false;
}

export function formatPlace(loc: PublicLocation): string {
  const parts = [loc.city, loc.country].filter(Boolean);
  return parts.length > 0 ? parts.join(', ') : 'Position inconnue';
}

export function countryName(code: string): string {
  const upper = code.trim().slice(0, 2).toUpperCase();
  try {
    const names = new Intl.DisplayNames(['fr'], { type: 'region' });
    return names.of(upper) ?? upper;
  } catch {
    return upper;
  }
}

import type { Server } from '@/types/api';

/** Round-trip time to each server in ms, keyed by server id. */
export type Latencies = Record<string, number>;

// Score units are milliseconds: a fully loaded server counts as 100 ms of
// extra latency, so a nearby server wins unless it is much busier.
const LOAD_WEIGHT_MS = 100;
// Servers whose latency could not be measured rank behind every measured one.
const UNKNOWN_LATENCY_MS = 1000;
// Moving a device to another server reallocates its IP and peer, so only
// switch when the gain is noticeable.
const SWITCH_MARGIN_MS = 20;
const SATURATED = 0.85;

const PROBE_TIMEOUT_MS = 2_000;

export function loadOf(server: Server): number {
  if (server.max_peers <= 0) return 0;
  return Math.min(1, Math.max(0, server.current_peers / server.max_peers));
}

function scoreOf(server: Server, latencies: Latencies, anyMeasured: boolean): number {
  // Without any measurement the latency term is the same for everyone and the
  // choice falls back to the load alone.
  const rtt = anyMeasured ? (latencies[server.id] ?? UNKNOWN_LATENCY_MS) : 0;
  return rtt + loadOf(server) * LOAD_WEIGHT_MS;
}

/**
 * Fastest active server (lowest latency, then load), keeping `current` unless
 * another is clearly better.
 */
export function pickBestServer(
  servers: Server[],
  current: Server | null,
  latencies: Latencies = {},
): Server | null {
  const active = servers.filter((s) => s.is_active);
  if (active.length === 0) return null;

  const anyMeasured = active.some((s) => latencies[s.id] !== undefined);
  const score = (s: Server) => scoreOf(s, latencies, anyMeasured);

  const best = active.reduce((a, b) => (score(b) < score(a) ? b : a));
  const kept = current ? active.find((s) => s.id === current.id) : undefined;
  if (kept && loadOf(kept) < SATURATED && score(kept) - score(best) < SWITCH_MARGIN_MS) {
    return kept;
  }
  return best;
}

/** `grpc_endpoint` as "host:port", without scheme or path. */
function probeTarget(server: Server): string | null {
  const target = server.grpc_endpoint
    .trim()
    .replace(/^[a-z]+:\/\//i, '')
    .replace(/\/.*$/, '');
  return target || null;
}

async function timedRequest(url: string): Promise<number | null> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);
  const start = Date.now();
  try {
    // The agent is a gRPC server on the same host as WireGuard: a plain POST
    // gets an immediate 415 answer, which is all we need to time the round
    // trip. Any HTTP response counts; a network error means no measurement.
    await fetch(url, { method: 'POST', signal: controller.signal });
    return Date.now() - start;
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Measures the round trip to a server's host. The first request pays for
 * DNS, TCP and TLS; the second reuses the connection, so the minimum of the
 * two is close to one network round trip.
 */
export async function measureLatency(server: Server): Promise<number | null> {
  const target = probeTarget(server);
  if (!target) return null;
  const url = `https://${target}/`;

  const first = await timedRequest(url);
  if (first === null) return null;
  const second = await timedRequest(url);
  return second === null ? first : Math.min(first, second);
}

/** Measures every server in parallel; servers that did not answer are left out. */
export async function measureLatencies(servers: Server[]): Promise<Latencies> {
  const results = await Promise.all(
    servers.map(async (s) => [s.id, await measureLatency(s)] as const),
  );
  const latencies: Latencies = {};
  for (const [id, ms] of results) {
    if (ms !== null) latencies[id] = ms;
  }
  return latencies;
}

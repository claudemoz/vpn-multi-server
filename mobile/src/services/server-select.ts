import type { Server } from '@/types/api';

// Moving a device to another server reallocates its IP and peer, so only
// switch when the gain in load is noticeable.
const SWITCH_MARGIN = 0.15;
const SATURATED = 0.85;

export function loadOf(server: Server): number {
  if (server.max_peers <= 0) return 0;
  return Math.min(1, Math.max(0, server.current_peers / server.max_peers));
}

/** Least loaded active server, keeping `current` unless another is clearly better. */
export function pickBestServer(servers: Server[], current: Server | null): Server | null {
  const active = servers.filter((s) => s.is_active);
  if (active.length === 0) return null;

  const best = active.reduce((a, b) => (loadOf(b) < loadOf(a) ? b : a));
  const kept = current ? active.find((s) => s.id === current.id) : undefined;
  if (kept && loadOf(kept) < SATURATED && loadOf(kept) - loadOf(best) < SWITCH_MARGIN) {
    return kept;
  }
  return best;
}

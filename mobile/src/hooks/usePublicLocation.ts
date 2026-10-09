import { useCallback, useEffect, useRef, useState } from 'react';
import { AppState } from 'react-native';

import type { ConnectionStatus } from '@/contexts/VpnContext';
import { geolocate, lookupPublicIps, sharesAddress, type PublicLocation } from '@/services/geo';
import * as storage from '@/services/storage';
import { getTunnelStatus, phaseOf, type TunnelPhase } from '@/services/vpn-tunnel';

// Routes take a moment to switch after the tunnel changes state.
const SETTLE_DELAY_MS = 500;
const RETRY_DELAY_MS = 1500;
const MAX_RETRIES = 3;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

type Measurement = {
  location: PublicLocation;
  /** Tunnel phase reported by iOS when the measurement was taken. */
  phase: TunnelPhase;
  /** False while city/country are still being resolved. */
  located: boolean;
};

export function usePublicLocation(status: ConnectionStatus) {
  const [measurement, setMeasurement] = useState<Measurement | null>(null);
  const [real, setReal] = useState<PublicLocation | null>(null);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);

  const requestId = useRef(0);
  const realRef = useRef<PublicLocation | null>(null);
  const statusRef = useRef(status);
  statusRef.current = status;

  useEffect(() => {
    storage.getRealLocation<PublicLocation>().then((saved) => {
      if (saved && !realRef.current) {
        realRef.current = saved;
        setReal(saved);
      }
    });
  }, []);

  const refresh = useCallback(async (delayMs = 0, remeasures = 2): Promise<void> => {
    const id = ++requestId.current;
    const alive = () => id === requestId.current;
    const retryLater = () => {
      if (remeasures > 0) return refresh(SETTLE_DELAY_MS, remeasures - 1);
      setMeasurement(null);
      setFailed(true);
    };

    setLoading(true);
    setFailed(false);
    try {
      if (delayMs > 0) await sleep(delayMs);
      // The JS status can lag behind the system (e.g. it starts as
      // 'disconnected' while the tunnel is already up), so ask iOS directly.
      const phase = phaseOf(await getTunnelStatus());
      if (!alive()) return;
      if (phase !== statusRef.current) return retryLater();

      let lookup = lookupPublicIps();
      let ipv4 = await lookup.ipv4;

      // Right after connecting, a request can still leave through the old route.
      for (
        let attempt = 0;
        attempt < MAX_RETRIES && phase === 'connected' && !!ipv4 && ipv4 === realRef.current?.ipv4;
        attempt++
      ) {
        await sleep(RETRY_DELAY_MS);
        if (!alive()) return;
        lookup = lookupPublicIps();
        ipv4 = await lookup.ipv4;
      }
      if (!alive()) return;

      const ip = ipv4 ?? (await lookup.ipv6);
      if (!alive()) return;
      if (!ip) throw new Error('Adresse IP publique introuvable');

      // Show the address right away; city, country and the IPv6 check follow.
      let location: PublicLocation = { ip, ipv4, ipv6: ipv4 ? undefined : ip };
      let located = false;
      const publish = () => {
        if (alive()) setMeasurement({ location, phase, located });
      };
      publish();
      setLoading(false);

      await Promise.all([
        geolocate(ip).then((geo) => {
          location = { ...location, ...geo };
          located = true;
          publish();
        }),
        ipv4
          ? lookup.ipv6.then((ipv6) => {
              location = { ...location, ipv6 };
              publish();
            })
          : Promise.resolve(),
      ]);

      const phaseAfter = phaseOf(await getTunnelStatus());
      if (!alive()) return;
      if (phaseAfter !== phase) return retryLater();

      if (phase === 'disconnected' && statusRef.current === 'disconnected') {
        realRef.current = location;
        setReal(location);
        storage.setRealLocation(location).catch(() => {});
      }
    } catch {
      if (alive()) setFailed(true);
    } finally {
      if (alive()) setLoading(false);
    }
  }, []);

  useEffect(() => {
    // A measurement only describes the state it was taken in.
    setMeasurement(null);
    setFailed(false);
    if (status === 'connected' || status === 'disconnected') {
      refresh(SETTLE_DELAY_MS);
    } else {
      requestId.current++;
      setLoading(true);
    }
  }, [status, refresh]);

  useEffect(() => {
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active' && (statusRef.current === 'connected' || statusRef.current === 'disconnected')) {
        refresh();
      }
    });
    return () => sub.remove();
  }, [refresh]);

  const matchesState = !!measurement && measurement.phase === status;
  const current = matchesState ? measurement.location : null;
  const located = matchesState && measurement.located;

  const isLeaking =
    status === 'connected' && !loading && !!current && !!real && sharesAddress(current, real);
  const isMasked = status === 'connected' && !loading && !!current && !isLeaking;
  const leaksIpv6 = isLeaking && !!current?.ipv6 && !!real?.ipv6 && current.ipv4 !== real.ipv4;

  return { current, located, real, loading, failed, isMasked, isLeaking, leaksIpv6, refresh };
}

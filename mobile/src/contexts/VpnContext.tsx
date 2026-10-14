import * as Device from 'expo-device';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { AppState } from 'react-native';

import { useAuth } from '@/contexts/AuthContext';
import { ApiClientError, deviceApi, serverApi } from '@/services/api';
import { measureLatencies, pickBestServer, type Latencies } from '@/services/server-select';
import * as storage from '@/services/storage';
import {
  connectTunnelFromConfig,
  describeTunnelError,
  disconnectTunnel,
  getTunnelStatus,
  isVpnTunnelAvailable,
  mapTunnelStatus,
  NATIVE_BUILD_COMMAND,
  phaseOf,
  sleep,
  subscribeTunnelState,
  type TunnelPhase,
} from '@/services/vpn-tunnel';
import { injectPrivateKey as injectKey } from '@/services/wg-config-parser';
import type { Server } from '@/types/api';

export type ConnectionStatus = TunnelPhase;

type VpnContextValue = {
  servers: Server[];
  /** Measured round trip to each server (ms), by server id. */
  latencies: Latencies;
  /** Server used for the next connection (picked by the app when `autoSelect`). */
  selectedServer: Server | null;
  autoSelect: boolean;
  status: ConnectionStatus;
  assignedIp: string | null;
  /** Start of the current connection (local clock, ms), restored from the API. */
  connectedAt: number | null;
  isLoadingServers: boolean;
  tunnelAvailable: boolean;
  error: string | null;
  selectServer: (server: Server) => void;
  selectAuto: () => void;
  connect: () => Promise<void>;
  disconnect: () => Promise<void>;
  refreshServers: () => Promise<void>;
};

type PreparedTunnel = {
  config: string;
  privateKey: string;
  assignedIp: string | null;
};

type WaitResult = 'connected' | 'failed' | 'timeout' | 'cancelled';

const CONNECT_TIMEOUT_MS = 30_000;
const DISCONNECT_TIMEOUT_MS = 10_000;
const POLL_INTERVAL_MS = 700;
// iOS briefly reports "disconnected" right after saving the configuration,
// before the extension starts; ignore it during this window.
const START_GRACE_MS = 2_000;
const RELEASE_CONFIRM_MS = 1_500;

const NATIVE_BUILD_REQUIRED = `Tunnel natif indisponible. Lancez un build natif : ${NATIVE_BUILD_COMMAND}`;

const VpnContext = createContext<VpnContextValue | null>(null);

function deviceLabel(): string {
  const name = Device.deviceName ?? Device.modelName ?? 'Mobile';
  return name.slice(0, 100);
}

function extractIp(config: string | null): string | null {
  return config?.match(/Address = ([\d.]+)/)?.[1] ?? null;
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiClientError) return error.message;
  return describeTunnelError(error) || fallback;
}

/** The device no longer exists for this account: a new one must be created. */
function isGone(error: unknown): boolean {
  return error instanceof ApiClientError && (error.status === 404 || error.status === 403);
}

async function createDevice(token: string, server: Server): Promise<PreparedTunnel> {
  const device = await deviceApi.create(token, { server_id: server.id, name: deviceLabel() });
  if (!device.private_key) {
    throw new Error('Le serveur n’a pas renvoyé de clé privée pour cet appareil.');
  }
  const config = injectKey(device.config, device.private_key);
  await storage.saveVpnSession({
    deviceId: device.id,
    serverId: server.id,
    config,
    privateKey: device.private_key,
  });
  return { config, privateKey: device.private_key, assignedIp: device.assigned_ip };
}

/**
 * Deletes the device stored on this phone, which removes its peer, frees its
 * IP and closes its session on the API. On failure the device is kept so the
 * next release (or connection) can reuse it.
 */
async function releaseDevice(token: string): Promise<void> {
  const { deviceId } = await storage.getVpnSession();
  if (!deviceId) return;
  try {
    await deviceApi.remove(token, deviceId, 8_000);
  } catch (e) {
    if (!isGone(e)) throw e;
  }
  await storage.clearVpnSession();
}

/**
 * Returns a ready-to-use tunnel config for `server`. The device is normally
 * released at disconnect; one still stored (failed release) is reused.
 */
async function prepareTunnel(token: string, server: Server): Promise<PreparedTunnel> {
  const session = await storage.getVpnSession();
  const { deviceId, config, privateKey } = session;
  if (!deviceId || !config || !privateKey) {
    return createDevice(token, server);
  }

  if (session.serverId === server.id) {
    try {
      const device = await deviceApi.get(token, deviceId, 5_000);
      if (device.revoked_at) return createDevice(token, server);
      // Re-render the config so a device kept from an older version picks up
      // the current tunnel settings (IPv6 address, MTU, DNS).
      const fresh = await deviceApi
        .config(token, deviceId)
        .then((raw) => injectKey(raw, privateKey))
        .catch(() => config);
      if (fresh !== config) {
        await storage.saveVpnSession({ deviceId, serverId: server.id, config: fresh, privateKey });
      }
      return { config: fresh, privateKey, assignedIp: device.assigned_ip ?? extractIp(fresh) };
    } catch (e) {
      if (isGone(e)) return createDevice(token, server);
      // API unreachable: the stored config is still valid for the tunnel.
      return { config, privateKey, assignedIp: extractIp(config) };
    }
  }

  try {
    const device = await deviceApi.update(token, deviceId, { server_id: server.id });
    const raw = await deviceApi.config(token, deviceId);
    const moved = injectKey(raw, privateKey);
    await storage.saveVpnSession({ deviceId, serverId: server.id, config: moved, privateKey });
    return { config: moved, privateKey, assignedIp: device.assigned_ip ?? extractIp(moved) };
  } catch (e) {
    if (isGone(e)) return createDevice(token, server);
    throw e;
  }
}

export function VpnProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth();
  const [servers, setServers] = useState<Server[]>([]);
  const [latencies, setLatencies] = useState<Latencies>({});
  // Read by refreshServers without making it change on every measurement.
  const latenciesRef = useRef<Latencies>({});
  const [selectedServer, setSelectedServer] = useState<Server | null>(null);
  const [autoSelect, setAutoSelect] = useState(true);
  const [status, setStatus] = useState<ConnectionStatus>('disconnected');
  const [assignedIp, setAssignedIp] = useState<string | null>(null);
  const [isLoadingServers, setIsLoadingServers] = useState(false);
  const [tunnelAvailable, setTunnelAvailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Only one connect/disconnect runs at a time. Bumping `attemptRef` makes any
  // in-flight operation stale, which is how a connection gets cancelled.
  const busyRef = useRef<'connect' | 'disconnect' | null>(null);
  const attemptRef = useRef(0);
  const sawConnectingRef = useRef(false);
  const releaseRef = useRef<Promise<void> | null>(null);
  const sessionRef = useRef<'idle' | 'loading' | 'tracked'>('idle');
  const [connectedAt, setConnectedAt] = useState<number | null>(null);

  const release = useCallback((): Promise<void> => {
    if (!token) return Promise.resolve();
    releaseRef.current ??= releaseDevice(token)
      .catch(() => {})
      .finally(() => {
        releaseRef.current = null;
      });
    return releaseRef.current;
  }, [token]);

  /**
   * Releases the device once the tunnel is confirmed down: deleting it removes
   * the peer on the server, which would break a tunnel that is still up.
   */
  const releaseIfDown = useCallback(async () => {
    await sleep(RELEASE_CONFIRM_MS);
    if (busyRef.current) return;
    if (phaseOf(await getTunnelStatus()) !== 'disconnected' || busyRef.current) return;
    // Also wipes the private key from the iOS profile when the VPN was turned
    // off outside the app.
    await disconnectTunnel().catch(() => {});
    await release();
  }, [release]);

  /** Opens (or restores) the session on the API to get the connection time. */
  const trackSession = useCallback(async () => {
    if (!token || sessionRef.current !== 'idle') return;
    sessionRef.current = 'loading';
    try {
      const { deviceId } = await storage.getVpnSession();
      if (!deviceId) throw new Error('no device');
      const session = await deviceApi.startSession(token, deviceId);
      if (sessionRef.current !== 'loading') return;
      // Derived from the elapsed time rather than connected_at so that a
      // phone clock out of sync with the server does not skew the timer.
      // A local start set after an earlier failure is kept so the timer does not jump.
      const startedAt = Date.now() - session.duration_seconds * 1000;
      setConnectedAt((at) => at ?? startedAt);
      sessionRef.current = 'tracked';
    } catch {
      if (sessionRef.current !== 'loading') return;
      setConnectedAt((at) => at ?? Date.now());
      sessionRef.current = 'idle';
    }
  }, [token]);

  const resetSession = useCallback(() => {
    sessionRef.current = 'idle';
    setConnectedAt(null);
  }, []);

  const applyPhase = useCallback(
    async (phase: TunnelPhase) => {
      setStatus(phase);
      if (phase === 'connected') {
        const session = await storage.getVpnSession().catch(() => null);
        setAssignedIp((ip) => ip ?? extractIp(session?.config ?? null));
        trackSession().catch(() => {});
      } else if (phase === 'disconnected') {
        setAssignedIp(null);
        resetSession();
        releaseIfDown().catch(() => {});
      }
    },
    [releaseIfDown, trackSession, resetSession],
  );

  const syncFromNative = useCallback(async () => {
    if (busyRef.current) return;
    const nativeStatus = await getTunnelStatus();
    if (busyRef.current) return;
    await applyPhase(phaseOf(nativeStatus));
  }, [applyPhase]);

  useEffect(() => {
    isVpnTunnelAvailable().then(setTunnelAvailable, () => setTunnelAvailable(false));
  }, []);

  useEffect(() => {
    return subscribeTunnelState((event) => {
      const phase = event.isConnected ? 'connected' : mapTunnelStatus(event.status ?? event.tunnelState);
      if (phase === 'connecting') sawConnectingRef.current = true;
      // During an operation the waiting loop owns the status; otherwise follow
      // the system (e.g. the VPN was turned off from iOS Settings).
      if (!busyRef.current) {
        applyPhase(phase).catch(() => {});
      }
    });
  }, [applyPhase]);

  useEffect(() => {
    syncFromNative().catch(() => {});
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active') syncFromNative().catch(() => {});
    });
    return () => sub.remove();
  }, [syncFromNative]);

  /**
   * Measures the latency to each server and, in automatic mode, moves the
   * selection to the fastest one. Only runs while the tunnel is down: through
   * the tunnel every probe would go via the current server.
   */
  const measureServers = useCallback(async (active: Server[]) => {
    if (active.length < 2 || busyRef.current) return;
    if (phaseOf(await getTunnelStatus()) !== 'disconnected') return;

    const measured = await measureLatencies(active);
    if (Object.keys(measured).length === 0 || busyRef.current) return;
    latenciesRef.current = measured;
    setLatencies(measured);
    if (await storage.getAutoSelect()) {
      setSelectedServer((prev) => pickBestServer(active, prev, measured));
    }
  }, []);

  const refreshServers = useCallback(async () => {
    if (!token) return;
    setIsLoadingServers(true);
    setError(null);
    let active: Server[] = [];
    try {
      const list = await serverApi.list(token);
      active = list.filter((s) => s.is_active);
      setServers(active);

      const [session, auto] = await Promise.all([storage.getVpnSession(), storage.getAutoSelect()]);
      setAutoSelect(auto);
      setSelectedServer((prev) => {
        const saved = session.serverId ? active.find((s) => s.id === session.serverId) : undefined;
        const current = prev && active.some((s) => s.id === prev.id) ? prev : (saved ?? null);
        if (auto) return pickBestServer(active, current, latenciesRef.current);
        return current ?? active[0] ?? null;
      });
    } catch (e) {
      setError(errorMessage(e, 'Impossible de charger les serveurs'));
    } finally {
      setIsLoadingServers(false);
    }
    // In the background: the list is usable right away, the measurement only
    // refines the automatic choice.
    measureServers(active).catch(() => {});
  }, [token, measureServers]);

  useEffect(() => {
    refreshServers();
  }, [refreshServers]);

  const selectServer = useCallback(
    (server: Server) => {
      if (busyRef.current || status !== 'disconnected') return;
      setAutoSelect(false);
      storage.setAutoSelect(false).catch(() => {});
      setSelectedServer(server);
      setError(null);
    },
    [status],
  );

  const selectAuto = useCallback(() => {
    if (busyRef.current || status !== 'disconnected') return;
    setAutoSelect(true);
    storage.setAutoSelect(true).catch(() => {});
    setSelectedServer((prev) => pickBestServer(servers, prev, latencies));
    setError(null);
  }, [status, servers, latencies]);

  const waitForPhase = useCallback(
    async (target: TunnelPhase, timeoutMs: number, isStale: () => boolean): Promise<boolean> => {
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        if (isStale()) return false;
        if (phaseOf(await getTunnelStatus()) === target) return true;
        await sleep(POLL_INTERVAL_MS);
      }
      return false;
    },
    [],
  );

  const waitForConnected = useCallback(
    async (isStale: () => boolean): Promise<WaitResult> => {
      const startedAt = Date.now();
      const deadline = startedAt + CONNECT_TIMEOUT_MS;
      while (Date.now() < deadline) {
        if (isStale()) return 'cancelled';
        const phase = phaseOf(await getTunnelStatus());
        if (phase === 'connected') return 'connected';
        if (phase === 'connecting') sawConnectingRef.current = true;
        if (
          phase === 'disconnected' &&
          sawConnectingRef.current &&
          Date.now() - startedAt > START_GRACE_MS
        ) {
          return 'failed';
        }
        await sleep(POLL_INTERVAL_MS);
      }
      return 'timeout';
    },
    [],
  );

  const connect = useCallback(async () => {
    if (busyRef.current) return;
    const server = autoSelect ? pickBestServer(servers, selectedServer, latencies) : selectedServer;
    if (!token || !server) {
      setError('Sélectionnez un serveur');
      return;
    }
    if (server.id !== selectedServer?.id) setSelectedServer(server);

    const attempt = ++attemptRef.current;
    const isStale = () => attemptRef.current !== attempt;
    busyRef.current = 'connect';
    sawConnectingRef.current = false;
    setError(null);
    setStatus('connecting');

    let tunnelStarted = false;
    try {
      const available = tunnelAvailable || (await isVpnTunnelAvailable());
      if (!available) {
        setStatus('disconnected');
        setError(NATIVE_BUILD_REQUIRED);
        return;
      }
      setTunnelAvailable(true);

      // A tunnel left running (other server, previous session) must be stopped
      // first: iOS ignores a start request while the VPN is already up.
      if (phaseOf(await getTunnelStatus()) !== 'disconnected') {
        await disconnectTunnel().catch(() => {});
        await waitForPhase('disconnected', DISCONNECT_TIMEOUT_MS, isStale);
      }
      if (isStale()) return;

      if (releaseRef.current) await releaseRef.current;
      resetSession();
      const prepared = await prepareTunnel(token, server);
      if (isStale()) return;

      tunnelStarted = true;
      await connectTunnelFromConfig(prepared.config, prepared.privateKey);
      if (isStale()) {
        await disconnectTunnel().catch(() => {});
        return;
      }

      const result = await waitForConnected(isStale);
      if (result === 'cancelled' || isStale()) return;

      if (result !== 'connected') {
        await disconnectTunnel().catch(() => {});
        release().catch(() => {});
        setStatus('disconnected');
        setAssignedIp(null);
        setError(
          result === 'failed'
            ? 'Le serveur VPN n’a pas accepté la connexion. Vérifiez votre réseau et réessayez.'
            : 'Le serveur VPN ne répond pas. Réessayez dans un instant.',
        );
        return;
      }

      setAssignedIp(prepared.assignedIp);
      setStatus('connected');
      trackSession().catch(() => {});
    } catch (e) {
      if (isStale()) return;
      if (tunnelStarted) await disconnectTunnel().catch(() => {});
      release().catch(() => {});
      setStatus('disconnected');
      setAssignedIp(null);
      setError(errorMessage(e, 'Échec de la connexion'));
    } finally {
      if (!isStale()) busyRef.current = null;
    }
  }, [
    token,
    servers,
    latencies,
    selectedServer,
    autoSelect,
    tunnelAvailable,
    waitForPhase,
    waitForConnected,
    release,
    trackSession,
    resetSession,
  ]);

  /** Disconnects, or cancels a connection that is still in progress. */
  const disconnect = useCallback(async () => {
    if (busyRef.current === 'disconnect') return;

    const attempt = ++attemptRef.current;
    const isStale = () => attemptRef.current !== attempt;
    busyRef.current = 'disconnect';
    setError(null);
    setStatus('disconnecting');

    try {
      await disconnectTunnel();
      const stopped = await waitForPhase('disconnected', DISCONNECT_TIMEOUT_MS, isStale);
      if (!stopped && !isStale()) {
        setError('Le VPN met du temps à se couper. Vérifiez dans Réglages > VPN.');
      }
    } catch (e) {
      if (!isStale()) setError(errorMessage(e, 'Échec de la déconnexion'));
    } finally {
      if (!isStale()) {
        busyRef.current = null;
        await applyPhase(phaseOf(await getTunnelStatus()));
      }
    }
  }, [applyPhase, waitForPhase]);

  const value = useMemo(
    () => ({
      servers,
      latencies,
      selectedServer,
      autoSelect,
      status,
      assignedIp,
      connectedAt,
      isLoadingServers,
      tunnelAvailable,
      error,
      selectServer,
      selectAuto,
      connect,
      disconnect,
      refreshServers,
    }),
    [
      servers,
      latencies,
      selectedServer,
      autoSelect,
      status,
      assignedIp,
      connectedAt,
      isLoadingServers,
      tunnelAvailable,
      error,
      selectServer,
      selectAuto,
      connect,
      disconnect,
      refreshServers,
    ],
  );

  return <VpnContext.Provider value={value}>{children}</VpnContext.Provider>;
}

export function useVpn() {
  const ctx = useContext(VpnContext);
  if (!ctx) throw new Error('useVpn must be used within VpnProvider');
  return ctx;
}

import * as Device from 'expo-device';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

import { useAuth } from '@/contexts/AuthContext';
import { deviceApi, serverApi } from '@/services/api';
import * as storage from '@/services/storage';
import {
  connectTunnelFromConfig,
  disconnectTunnel,
  getTunnelStatus,
  isVpnTunnelAvailable,
  mapTunnelStatusToConnectionStatus,
  subscribeTunnelState,
} from '@/services/vpn-tunnel';
import { injectPrivateKey as injectKey } from '@/services/wg-config-parser';
import type { Server } from '@/types/api';

export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected';

type VpnContextValue = {
  servers: Server[];
  selectedServer: Server | null;
  status: ConnectionStatus;
  assignedIp: string | null;
  isLoadingServers: boolean;
  tunnelAvailable: boolean;
  error: string | null;
  selectServer: (server: Server) => void;
  connect: () => Promise<void>;
  disconnect: () => Promise<void>;
  refreshServers: () => Promise<void>;
};

const VpnContext = createContext<VpnContextValue | null>(null);

function deviceLabel(): string {
  const name = Device.deviceName ?? Device.modelName ?? 'Mobile';
  return name.slice(0, 100);
}

function extractIp(config: string | null): string | null {
  return config?.match(/Address = ([\d.]+)/)?.[1] ?? null;
}

export function VpnProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth();
  const [servers, setServers] = useState<Server[]>([]);
  const [selectedServer, setSelectedServer] = useState<Server | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>('disconnected');
  const [assignedIp, setAssignedIp] = useState<string | null>(null);
  const [isLoadingServers, setIsLoadingServers] = useState(false);
  const [tunnelAvailable, setTunnelAvailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    isVpnTunnelAvailable().then(setTunnelAvailable);
  }, []);

  useEffect(() => {
    return subscribeTunnelState((event) => {
      setStatus(mapTunnelStatusToConnectionStatus(event.status));
      if (!event.isConnected) {
        setAssignedIp(null);
      }
    });
  }, []);

  const refreshServers = useCallback(async () => {
    if (!token) return;
    setIsLoadingServers(true);
    setError(null);
    try {
      const list = await serverApi.list(token);
      const active = list.filter((s) => s.is_active);
      setServers(active);

      const session = await storage.getVpnSession();
      setSelectedServer((prev) => {
        if (prev && active.some((s) => s.id === prev.id)) return prev;
        if (session.serverId) {
          const saved = active.find((s) => s.id === session.serverId);
          if (saved) return saved;
        }
        return active[0] ?? null;
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Impossible de charger les serveurs');
    } finally {
      setIsLoadingServers(false);
    }
  }, [token]);

  useEffect(() => {
    refreshServers();
  }, [token]);

  useEffect(() => {
    (async () => {
      const tunnelStatus = await getTunnelStatus();
      if (tunnelStatus?.isConnected) {
        setStatus('connected');
        const session = await storage.getVpnSession();
        setAssignedIp(extractIp(session.config));
        return;
      }

      const session = await storage.getVpnSession();
      if (session.deviceId && session.config) {
        setAssignedIp(extractIp(session.config));
      }
    })();
  }, []);

  const selectServer = useCallback(
    (server: Server) => {
      if (status === 'connected' || status === 'connecting') return;
      setSelectedServer(server);
      setError(null);
    },
    [status],
  );

  const startNativeTunnel = useCallback(async (config: string, privateKey?: string) => {
    const fullConfig = privateKey ? injectKey(config, privateKey) : config;
    await connectTunnelFromConfig(fullConfig, privateKey);
  }, []);

  const connect = useCallback(async () => {
    if (!token || !selectedServer) {
      setError('Sélectionnez un serveur');
      return;
    }

    setStatus('connecting');
    setError(null);

    try {
      let session = await storage.getVpnSession();

      if (session.deviceId && session.serverId === selectedServer.id && session.config) {
        if (tunnelAvailable) {
          await startNativeTunnel(session.config, session.privateKey ?? undefined);
        }
        setAssignedIp(extractIp(session.config));
        setStatus('connected');
        return;
      }

      const device = await deviceApi.create(token, {
        server_id: selectedServer.id,
        name: deviceLabel(),
      });

      console.log("### device ### ", device);
      
      const configWithKey = device.private_key
        ? injectKey(device.config, device.private_key)
        : device.config;

      await storage.saveVpnSession({
        deviceId: device.id,
        serverId: selectedServer.id,
        config: configWithKey,
        privateKey: device.private_key,
      });

      if (tunnelAvailable) {
        await startNativeTunnel(configWithKey, device.private_key);
      } else {
        setError(
          'Device provisionné. Build natif requis pour le tunnel (npx expo run:ios --device).',
        );
      }

      setAssignedIp(device.assigned_ip);
      setStatus('connected');
    } catch (e) {
      setStatus('disconnected');
      setError(e instanceof Error ? e.message : 'Échec de la connexion');
    }
  }, [token, selectedServer, tunnelAvailable, startNativeTunnel]);

  const disconnect = useCallback(async () => {
    setStatus('connecting');
    try {
      if (tunnelAvailable) {
        await disconnectTunnel();
      }
    } finally {
      setStatus('disconnected');
      setAssignedIp(null);
      await storage.clearVpnSession();
    }
  }, [tunnelAvailable]);

  const value = useMemo(
    () => ({
      servers,
      selectedServer,
      status,
      assignedIp,
      isLoadingServers,
      tunnelAvailable,
      error,
      selectServer,
      connect,
      disconnect,
      refreshServers,
    }),
    [
      servers,
      selectedServer,
      status,
      assignedIp,
      isLoadingServers,
      tunnelAvailable,
      error,
      selectServer,
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

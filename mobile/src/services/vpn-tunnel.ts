import { NativeEventEmitter, NativeModules, Platform } from 'react-native';
import type {
  VpnStateChangedEvent,
  WireGuardConfig,
  WireGuardStatus,
} from 'react-native-wireguard-vpn';

import { wgQuickToNativeConfig } from '@/services/wg-config-parser';

export type TunnelStateListener = (event: VpnStateChangedEvent) => void;
export type TunnelPhase = 'disconnected' | 'connecting' | 'connected' | 'disconnecting';

const NATIVE_CALL_TIMEOUT_MS = 15_000;
// Android resolves connect only after the server answered, and may first wait
// for the user to accept the system VPN permission dialog.
const CONNECT_CALL_TIMEOUT_MS = 60_000;

export const NATIVE_BUILD_COMMAND =
  Platform.OS === 'android' ? 'npx expo run:android --device' : 'npx expo run:ios --device';

function isNativePlatform(): boolean {
  return Platform.OS === 'ios' || Platform.OS === 'android';
}

/** Rejette si `promise` ne se termine pas dans `ms` (un appel natif peut ne jamais répondre). */
export function withTimeout<T>(promise: Promise<T>, ms: number, message: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(message)), ms);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function loadModule() {
  if (!isNativePlatform()) return null;
  try {
    const mod = await import('react-native-wireguard-vpn');
    return mod.default ?? null;
  } catch {
    return null;
  }
}

/** Traduit les erreurs natives (iOS NetworkExtension, Android VpnService) en message lisible. */
export function describeTunnelError(error: unknown): string {
  const raw = error instanceof Error ? error.message : String(error ?? '');
  const text = raw.toLowerCase();
  if (text.includes('permission') || text.includes('denied') || text.includes('code=5')) {
    return 'Autorisation VPN refusée. Acceptez la configuration VPN puis réessayez.';
  }
  if (text.includes('cancel')) {
    return 'Connexion annulée.';
  }
  return raw || 'Échec du tunnel VPN';
}

/** Vrai si le module natif WireGuard est disponible (pas Expo Go / pas web). */
export async function isVpnTunnelAvailable(): Promise<boolean> {
  const mod = await loadModule();
  if (!mod) return false;
  try {
    return await withTimeout(mod.isSupported(), 5_000, 'isSupported timeout');
  } catch {
    return false;
  }
}

let initialized = false;

export async function initializeTunnel(): Promise<void> {
  const mod = await loadModule();
  if (!mod) {
    throw new Error('Tunnel VPN indisponible sur cette plateforme (utilisez un build natif)');
  }
  if (initialized) return;
  await withTimeout(mod.initialize(), NATIVE_CALL_TIMEOUT_MS, 'Initialisation du VPN trop longue');
  initialized = true;
}

export async function connectTunnelFromConfig(
  configText: string,
  privateKey?: string,
): Promise<void> {
  const nativeConfig = wgQuickToNativeConfig(configText, privateKey);
  await connectTunnel(nativeConfig);
}

export async function connectTunnel(config: WireGuardConfig): Promise<void> {
  const mod = await loadModule();
  if (!mod) {
    throw new Error(`Tunnel VPN indisponible. Lancez avec : ${NATIVE_BUILD_COMMAND}`);
  }
  await initializeTunnel();
  await withTimeout(mod.connect(config), CONNECT_CALL_TIMEOUT_MS, 'Le système n’a pas démarré le VPN');
}

export async function disconnectTunnel(): Promise<void> {
  const mod = await loadModule();
  if (!mod) return;
  await withTimeout(mod.disconnect(), NATIVE_CALL_TIMEOUT_MS, 'Le système n’a pas arrêté le VPN');
}

export async function getTunnelStatus(): Promise<WireGuardStatus | null> {
  const mod = await loadModule();
  if (!mod) return null;
  try {
    return await withTimeout(mod.getStatus(), 5_000, 'getStatus timeout');
  } catch {
    return null;
  }
}

/** Écoute les changements d'état du tunnel (Network Extension / VpnService). */
export function subscribeTunnelState(listener: TunnelStateListener): () => void {
  if (!isNativePlatform()) {
    return () => {};
  }

  const moduleRef = NativeModules.WireGuardVpnModule;
  if (!moduleRef) {
    return () => {};
  }

  try {
    const emitter = new NativeEventEmitter(moduleRef);
    const sub = emitter.addListener('vpnStateChanged', (event: VpnStateChangedEvent) => {
      try {
        listener(event);
      } catch {
        // A listener error must never crash the native event loop.
      }
    });
    return () => sub.remove();
  } catch {
    return () => {};
  }
}

export function mapTunnelStatus(
  status: WireGuardStatus['status'] | WireGuardStatus['tunnelState'] | undefined,
): TunnelPhase {
  switch (status) {
    case 'CONNECTED':
    case 'ACTIVE':
      return 'connected';
    case 'CONNECTING':
      return 'connecting';
    case 'DISCONNECTING':
      return 'disconnecting';
    default:
      return 'disconnected';
  }
}

export function phaseOf(status: WireGuardStatus | null | undefined): TunnelPhase {
  if (!status) return 'disconnected';
  if (status.isConnected) return 'connected';
  return mapTunnelStatus(status.status ?? status.tunnelState);
}

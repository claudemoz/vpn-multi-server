import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

const KEYS = {
  token: 'auth_token',
  user: 'auth_user',
  deviceId: 'vpn_device_id',
  deviceConfig: 'vpn_device_config',
  privateKey: 'vpn_private_key',
  serverId: 'vpn_server_id',
  realLocation: 'vpn_real_location_v2',
  autoSelect: 'vpn_auto_select',
} as const;

/**
 * SecureStore ne fonctionne pas sur le web Expo : fallback localStorage.
 */
async function setItem(key: string, value: string): Promise<void> {
  if (Platform.OS === 'web') {
    localStorage.setItem(key, value);
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

async function getItem(key: string): Promise<string | null> {
  if (Platform.OS === 'web') {
    return localStorage.getItem(key);
  }
  return SecureStore.getItemAsync(key);
}

async function deleteItem(key: string): Promise<void> {
  if (Platform.OS === 'web') {
    localStorage.removeItem(key);
    return;
  }
  await SecureStore.deleteItemAsync(key);
}

export async function getToken(): Promise<string | null> {
  return getItem(KEYS.token);
}

export async function setToken(token: string): Promise<void> {
  await setItem(KEYS.token, token);
}

export async function clearAuth(): Promise<void> {
  await deleteItem(KEYS.token);
  await deleteItem(KEYS.user);
}

export async function getUser<T>(): Promise<T | null> {
  const raw = await getItem(KEYS.user);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

export async function setUser(user: unknown): Promise<void> {
  await setItem(KEYS.user, JSON.stringify(user));
}

export async function saveVpnSession(data: {
  deviceId: string;
  serverId: string;
  config: string;
  privateKey?: string;
}): Promise<void> {
  await setItem(KEYS.deviceId, data.deviceId);
  await setItem(KEYS.serverId, data.serverId);
  await setItem(KEYS.deviceConfig, data.config);
  if (data.privateKey) {
    await setItem(KEYS.privateKey, data.privateKey);
  }
}

export async function getVpnSession(): Promise<{
  deviceId: string | null;
  serverId: string | null;
  config: string | null;
  privateKey: string | null;
}> {
  const [deviceId, serverId, config, privateKey] = await Promise.all([
    getItem(KEYS.deviceId),
    getItem(KEYS.serverId),
    getItem(KEYS.deviceConfig),
    getItem(KEYS.privateKey),
  ]);
  return { deviceId, serverId, config, privateKey };
}

export async function clearVpnSession(): Promise<void> {
  await Promise.all([
    deleteItem(KEYS.deviceId),
    deleteItem(KEYS.serverId),
    deleteItem(KEYS.deviceConfig),
    deleteItem(KEYS.privateKey),
  ]);
}

export async function getRealLocation<T>(): Promise<T | null> {
  const raw = await getItem(KEYS.realLocation);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

export async function setRealLocation(location: unknown): Promise<void> {
  await setItem(KEYS.realLocation, JSON.stringify(location));
}

/** Automatic server selection is on unless the user picked a server. */
export async function getAutoSelect(): Promise<boolean> {
  return (await getItem(KEYS.autoSelect)) !== '0';
}

export async function setAutoSelect(enabled: boolean): Promise<void> {
  await setItem(KEYS.autoSelect, enabled ? '1' : '0');
}

export async function clearAll(): Promise<void> {
  await deleteItem(KEYS.autoSelect);
  await clearAuth();
  await clearVpnSession();
  await deleteItem(KEYS.realLocation);
  await deleteItem('vpn_real_location').catch(() => {});
}

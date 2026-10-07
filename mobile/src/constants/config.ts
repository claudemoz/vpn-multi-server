import Constants from 'expo-constants';
import { Platform } from 'react-native';

const API_PORT = 8080;
const API_PREFIX = '/api/v1';

const PROD_HTTP_BASE = "https://apivpn.tsiang.fr/v1"
  // process.env.EXPO_PUBLIC_API_URL?.replace(/\/$/, '') ??
  // 'https://api.example.com/api/v1';

const getDevHost = (): string => {
  const host = Constants.expoConfig?.hostUri?.split(':')[0];
  if (host) return host;
  if (Platform.OS === 'android') return '10.0.2.2';
  return '127.0.0.1';
};

/** Base URL HTTP (inclut /api/v1). */
export function getHttpBaseURL(): string {
  if (!__DEV__) {
    return `http://${getDevHost()}:${API_PORT}${API_PREFIX}`;
  }
  const configured = process.env.EXPO_PUBLIC_API_URL?.replace(/\/$/, '');
  return configured ?? PROD_HTTP_BASE;
}

/** URL racine sans /api/v1 (affichage debug). */
export function getApiRootURL(): string {
  return getHttpBaseURL().replace(/\/api\/v1\/?$/, '');
}

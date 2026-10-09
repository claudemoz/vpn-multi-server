import { router } from 'expo-router';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

import { authApi, deviceApi, userApi } from '@/services/api';
import * as storage from '@/services/storage';
import { disconnectTunnel } from '@/services/vpn-tunnel';
import type { User } from '@/types/api';

type AuthContextValue = {
  user: User | null;
  token: string | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (data: {
    first_name: string;
    last_name: string;
    email: string;
    phone?: string;
    password: string;
  }) => Promise<void>;
  updateProfile: (data: { first_name: string; last_name: string; email: string }) => Promise<void>;
  changePassword: (data: { current_password: string; password: string }) => Promise<void>;
  deleteAccount: (password: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    (async () => {
      try {
        const savedToken = await storage.getToken();
        if (!savedToken) return;

        const savedUser = await storage.getUser<User>();
        if (savedUser) {
          setToken(savedToken);
          setUser(savedUser);
          return;
        }

        const me = await userApi.me(savedToken);
        setToken(savedToken);
        setUser(me);
        await storage.setUser(me);
      } catch {
        await storage.clearAuth();
      } finally {
        setIsLoading(false);
      }
    })();
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const res = await authApi.login(email.trim(), password);
    await storage.setToken(res.token);
    await storage.setUser(res.user);
    setToken(res.token);
    setUser(res.user);
    router.replace('/(app)');
  }, []);

  const register = useCallback(
    async (data: {
      first_name: string;
      last_name: string;
      email: string;
      phone?: string;
      password: string;
    }) => {
      await authApi.register({
        ...data,
        first_name: data.first_name.trim(),
        last_name: data.last_name.trim(),
        email: data.email.trim(),
        phone: data.phone?.trim() || undefined,
      });
    },
    [],
  );

  const updateProfile = useCallback(
    async (data: { first_name: string; last_name: string; email: string }) => {
      if (!token) throw new Error('unauthenticated');
      const updated = await userApi.update(token, {
        first_name: data.first_name.trim(),
        last_name: data.last_name.trim(),
        email: data.email.trim(),
      });
      await storage.setUser(updated);
      setUser(updated);
    },
    [token],
  );

  const changePassword = useCallback(
    async (data: { current_password: string; password: string }) => {
      if (!token) throw new Error('unauthenticated');
      await userApi.changePassword(token, data);
    },
    [token],
  );

  const deleteAccount = useCallback(async (password: string) => {
    await disconnectTunnel().catch(() => {});
    if (!token) throw new Error('unauthenticated');
    await userApi.remove(token, password);
    await storage.clearAll();
    setToken(null);
    setUser(null);
    router.replace('/(auth)/login');
  }, [token]);

  const logout = useCallback(async () => {
    await disconnectTunnel().catch(() => {});
    const { deviceId } = await storage.getVpnSession().catch(() => ({ deviceId: null }));
    if (token && deviceId) {
      await deviceApi.remove(token, deviceId, 5_000).catch(() => {});
    }
    await storage.clearAll();
    setToken(null);
    setUser(null);
    router.replace('/(auth)/login');
  }, [token]);

  const value = useMemo(
    () => ({ user, token, isLoading, login, register, updateProfile, changePassword, deleteAccount, logout }),
    [user, token, isLoading, login, register, updateProfile, changePassword, deleteAccount, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

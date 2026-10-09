import { getApiRootURL, getHttpBaseURL } from '@/constants/config';
import type {
  ApiError,
  CreateDeviceResponse,
  Device,
  LoginResponse,
  RegisterResponse,
  Server,
  User,
  VpnSession,
} from '@/types/api';

export class ApiClientError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiClientError';
    this.status = status;
  }
}

const DEFAULT_TIMEOUT_MS = 15_000;

type RequestOptions = RequestInit & { timeoutMs?: number };

async function request<T>(
  path: string,
  options: RequestOptions = {},
  token?: string | null,
): Promise<T> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, ...init } = options;
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init.headers as Record<string, string>),
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  let res: Response;
  try {
    res = await fetch(`${getHttpBaseURL()}${path}`, { ...init, headers, signal: controller.signal });
  } catch {
    if (controller.signal.aborted) {
      throw new ApiClientError("L'API ne répond pas. Vérifiez votre connexion et réessayez.", 408);
    }
    throw new ApiClientError(
      `Impossible de joindre l'API (${getApiRootURL()}). Vérifiez votre connexion.`,
      0,
    );
  } finally {
    clearTimeout(timer);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const isText = res.headers.get('content-type')?.startsWith('text/plain') ?? false;
  const body = isText && res.ok ? await res.text() : await res.json().catch(() => ({}));

  if (!res.ok) {
    const msg = (body as ApiError).error ?? `HTTP ${res.status}`;
    throw new ApiClientError(msg, res.status);
  }

  return body as T;
}

export const authApi = {
  register(data: {
    first_name: string;
    last_name: string;
    email: string;
    phone?: string;
    password: string;
  }) {
    return request<RegisterResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  login(email: string, password: string) {
    return request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
  },
};

export const userApi = {
  me(token: string) {
    return request<User>('/user/me', {}, token);
  },

  update(token: string, data: { first_name?: string; last_name?: string; email?: string }) {
    return request<User>('/user/me', { method: 'PUT', body: JSON.stringify(data) }, token);
  },

  changePassword(token: string, data: { current_password: string; password: string }) {
    return request<void>('/user/me/password', { method: 'PUT', body: JSON.stringify(data) }, token);
  },

  remove(token: string, password: string) {
    return request<void>('/user/me', { method: 'DELETE', body: JSON.stringify({ password }) }, token);
  },
};

export const serverApi = {
  list(token: string) {
    return request<Server[]>('/server/list', {}, token);
  },
};

export const deviceApi = {
  list(token: string) {
    return request<Device[]>('/device/list', {}, token);
  },

  create(token: string, data: { server_id: string; name: string }) {
    return request<CreateDeviceResponse>('/device', { method: 'POST', body: JSON.stringify(data)}, token);
  },

  get(token: string, id: string, timeoutMs?: number) {
    return request<Device>(`/device/${id}`, { timeoutMs }, token);
  },

  update(token: string, id: string, data: { server_id?: string; name?: string }) {
    return request<Device>(`/device/${id}`, { method: 'PUT', body: JSON.stringify(data) }, token);
  },

  /** Config wg-quick du device ; la clé privée y est remplacée par un placeholder. */
  config(token: string, id: string) {
    return request<string>(`/device/${id}/config`, {}, token);
  },

  /** Supprime le device et son peer (libère l'IP) et clôture sa session. */
  remove(token: string, id: string, timeoutMs?: number) {
    return request<void>(`/device/${id}`, { method: 'DELETE', timeoutMs }, token);
  },

  /** Ouvre la session du device, ou renvoie celle déjà ouverte. */
  startSession(token: string, id: string) {
    return request<VpnSession>(`/device/${id}/session`, { method: 'POST', timeoutMs: 8_000 }, token);
  },
};

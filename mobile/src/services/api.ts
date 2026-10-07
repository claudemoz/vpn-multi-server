import { getApiRootURL, getHttpBaseURL } from '@/constants/config';
import type {
  ApiError,
  CreateDeviceResponse,
  Device,
  LoginResponse,
  RegisterResponse,
  Server,
  User,
} from '@/types/api';

export class ApiClientError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiClientError';
    this.status = status;
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${getHttpBaseURL()}${path}`, { ...options, headers });
  } catch {
    throw new ApiClientError(
      `Impossible de joindre l'API (${getApiRootURL()}). Vérifiez que le backend tourne et EXPO_PUBLIC_API_URL dans .env`,
      0,
    );
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const body = await res.json().catch(() => ({}));

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
};

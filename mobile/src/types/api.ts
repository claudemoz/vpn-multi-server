export type User = {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  phone?: string;
  created_at: string;
  updated_at: string;
};

export type LoginResponse = {
  token: string;
  user: User;
};

export type RegisterResponse = {
  user: User;
};

export type Server = {
  id: string;
  region: string;
  name: string;
  public_endpoint: string;
  wg_public_key: string;
  grpc_endpoint: string;
  subnet: string;
  max_peers: number;
  current_peers: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

export type Device = {
  id: string;
  user_id: string;
  server_id: string;
  name: string;
  public_key: string;
  assigned_ip: string;
  last_handshake?: string;
  revoked_at?: string;
  created_at: string;
  updated_at: string;
};

export type CreateDeviceResponse = Device & {
  private_key?: string;
  config: string;
};

export type VpnSession = {
  id: string;
  device_id: string;
  server_id: string;
  assigned_ip: string;
  connected_at: string;
  disconnected_at?: string;
  duration_seconds: number;
};

export type ApiError = {
  error: string;
};

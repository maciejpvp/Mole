import { api } from './api'

export type AuthenticatedUser = {
  id: string
  username: string
  email: string
  plan: string
}

export type Authentication = {
  user: AuthenticatedUser
  access_token: string
  expires_at: string
}

export type Tunnel = {
  id: string
  proto: string
  internal_address: string
  outbound_port: number
  server_address: string
  status: string
  started_at?: string | null
  stopped_at?: string | null
  current_period_minutes?: number
  current_period_transfer_bytes?: number
  created_at?: string
}

export type UserProfile = AuthenticatedUser & {
  created_at: string
  last_login_at: string | null
  limits: {
    max_active_tunnels: number | null
    monthly_minutes: number | null
    monthly_transfer_bytes: number | null
  }
  usage: {
    period_started_at: string
    monthly_minutes_used: number
    monthly_transfer_bytes_used: number
    limit_reached_at: string | null
  }
  tunnels: Tunnel[]
}

export async function login(input: { identifier: string; password: string }) {
  const response = await api.post<Authentication>('/api/v1/auth/login', input)
  return response.data
}

export async function register(input: { username: string; email: string; password: string }) {
  const response = await api.post<Authentication>('/api/v1/auth/register', input)
  return response.data
}

export async function getCurrentUser() {
  const response = await api.get<UserProfile>('/api/v1/user/me')
  return response.data
}

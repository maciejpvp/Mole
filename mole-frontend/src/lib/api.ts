import axios from 'axios'

const accessTokenKey = 'mole.access-token'

const controlPlaneUrl = import.meta.env.VITE_CONTROL_PLANE_URL !== undefined
  ? import.meta.env.VITE_CONTROL_PLANE_URL
  : 'http://127.0.0.1:8080'

export const api = axios.create({
  baseURL: controlPlaneUrl,
  headers: { 'Content-Type': 'application/json' },
})

export function getAccessToken(): string | null {
  return localStorage.getItem(accessTokenKey)
}

export function setAccessToken(token: string | null) {
  if (token) {
    localStorage.setItem(accessTokenKey, token)
    api.defaults.headers.common.Authorization = `Bearer ${token}`
    return
  }
  localStorage.removeItem(accessTokenKey)
  delete api.defaults.headers.common.Authorization
}

export type CreateTunnelInput = {
  proto: string
  internal_address: string
}

export type CreatedTunnel = {
  id: string
  proto: string
  internal_address: string
  outbound_port: number
  endpoint: string
  server_address: string
  token: string
}

export function createTunnel(input: CreateTunnelInput) {
  return api.post<CreatedTunnel>('/api/v1/tunnels', input)
}

export function deleteTunnel(id: string) {
	return api.delete(`/api/v1/tunnels/${id}`)
}

export type AdminUser = {
	id: string
	username: string
	email: string
	plan: string
	is_admin: boolean
	is_banned: boolean
	monthly_minutes_used: number
	monthly_transfer_bytes_used: number
	created_at: string
	last_login_at: string | null
}

export type Plan = {
	id: number
	name: string
	max_active_tunnels: number | null
	monthly_minutes: number | null
	monthly_transfer_bytes: number | null
}

export type AdminUsersPage = {
	users: AdminUser[]
	next_cursor?: string
}

export type AdminUsersQuery = {
	limit: number
	search?: string
	cursor?: string
	sort: 'transfer' | 'minutes' | 'username' | 'created_at'
	direction: 'asc' | 'desc'
}

export function listAdminUsers(query: AdminUsersQuery) {
	return api.get<AdminUsersPage>('/api/v1/admin/users', { params: query })
}

export function listPlans() {
	return api.get<Plan[]>('/api/v1/plans')
}

export function changeAdminUserPlan(userId: string, planId: number) {
	return api.patch<AdminUser>(`/api/v1/admin/users/${userId}/plan`, { plan_id: planId })
}

export function setAdminUserPermission(userId: string, isAdmin: boolean) {
	return api.patch<AdminUser>(`/api/v1/admin/users/${userId}/admin`, { is_admin: isAdmin })
}

export function setAdminUserBanned(userId: string, isBanned: boolean) {
	return api.patch<AdminUser>(`/api/v1/admin/users/${userId}/ban`, { is_banned: isBanned })
}

export const deteteTunnel = deleteTunnel

const savedToken = getAccessToken()
if (savedToken) {
  api.defaults.headers.common.Authorization = `Bearer ${savedToken}`
}

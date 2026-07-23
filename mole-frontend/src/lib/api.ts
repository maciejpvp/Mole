import axios from 'axios'

const accessTokenKey = 'mole.access-token'

export const api = axios.create({
  baseURL: import.meta.env.VITE_CONTROL_PLANE_URL ?? 'http://127.0.0.1:8080',
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

export const deteteTunnel = deleteTunnel

export function formatOutboundAddress(serverAddress?: string, outboundPort?: number): string {
  if (!outboundPort) return serverAddress ?? ''
  if (!serverAddress) return `:${outboundPort}`

  let host = serverAddress.trim()
  const colonCount = (host.match(/:/g) || []).length
  if (colonCount === 1 || (colonCount > 1 && host.includes(']'))) {
    const lastColon = host.lastIndexOf(':')
    const possiblePort = host.substring(lastColon + 1)
    if (/^\d+$/.test(possiblePort)) {
      host = host.substring(0, lastColon)
    }
  }

  return `${host}:${outboundPort}`
}

const savedToken = getAccessToken()
if (savedToken) {
  api.defaults.headers.common.Authorization = `Bearer ${savedToken}`
}


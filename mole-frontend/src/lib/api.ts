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

const savedToken = getAccessToken()
if (savedToken) {
  api.defaults.headers.common.Authorization = `Bearer ${savedToken}`
}

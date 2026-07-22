import { createContext, useContext } from 'react'

export type AuthSession = {
  accessToken: string | null
  setSessionAccessToken: (token: string | null) => void
}

export const AuthSessionContext = createContext<AuthSession | null>(null)

export function useAuthSession() {
  const session = useContext(AuthSessionContext)
  if (!session) throw new Error('useAuthSession must be used within AuthSessionProvider')
  return session
}

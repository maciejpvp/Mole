import { type ReactNode, useState } from 'react'
import { getAccessToken, setAccessToken } from '../lib/api'
import { AuthSessionContext } from './authSessionContext'

export function AuthSessionProvider({ children }: { children: ReactNode }) {
  const [accessToken, setSessionToken] = useState(getAccessToken)
  const setSessionAccessToken = (token: string | null) => {
    setAccessToken(token)
    setSessionToken(token)
  }

  return (
    <AuthSessionContext.Provider value={{ accessToken, setSessionAccessToken }}>
      {children}
    </AuthSessionContext.Provider>
  )
}

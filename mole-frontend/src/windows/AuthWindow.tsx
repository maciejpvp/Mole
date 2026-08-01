import { useEffect, useState } from 'react'
import { ImGuiButton } from '../components/imgui'
import { useAuthSession } from '../auth/authSessionContext'
import { useUser, userQueryKey } from '../hooks/useUser'
import { exchangeGoogleLogin, getGoogleLoginUrl } from '../lib/auth'
import { errorMessage } from '../utils'
import { useQueryClient } from '@tanstack/react-query'

export function AuthWindow() {
  const { accessToken, setSessionAccessToken } = useAuthSession()
  const queryClient = useQueryClient()
  const userQuery = useUser(accessToken)
  const [googleStatus, setGoogleStatus] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.replace(/^#/, ''))
    const code = params.get('google_code')
    const error = params.get('google_error')
    if (!code && !error) return

    window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
    if (error) {
      setGoogleStatus('Google sign-in was cancelled or failed.')
      return
    }

    setGoogleStatus('Signing in with Google…')
    void exchangeGoogleLogin(code as string)
      .then((authentication) => {
        setSessionAccessToken(authentication.access_token)
        void queryClient.invalidateQueries({ queryKey: userQueryKey })
        setGoogleStatus('')
      })
      .catch((requestError: unknown) => {
        setGoogleStatus(errorMessage(requestError, 'Google sign-in failed'))
      })
  }, [queryClient, setSessionAccessToken])

  const logout = () => {
    setSessionAccessToken(null)
    queryClient.removeQueries({ queryKey: userQueryKey })
  }

  if (userQuery.data) {
    return (
      <div className="flex items-center gap-3">
        <span>Signed in as {userQuery.data.username}</span>
        <ImGuiButton onClick={logout}>Log Out</ImGuiButton>
      </div>
    )
  }

  if (accessToken && userQuery.isFetching) {
    return <span className="text-[14px] text-[#9ab4d2]">Loading account…</span>
  }

  return (
    <div className="space-y-3">
      <ImGuiButton onClick={() => window.location.assign(getGoogleLoginUrl())}>
        Continue with Google
      </ImGuiButton>
      {googleStatus && <span className="text-[14px] text-[#9ab4d2]">{googleStatus}</span>}
      {userQuery.error && !googleStatus && (
        <span className="text-[14px] text-[#9ab4d2]">Your saved session has expired. Please sign in again.</span>
      )}
    </div>
  )
}

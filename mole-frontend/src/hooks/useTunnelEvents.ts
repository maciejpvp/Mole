import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { userQueryKey } from './useUser'
import type { UserProfile } from '../lib/auth'
import { useAuthSession } from '../auth/authSessionContext'

const controlPlaneUrl = import.meta.env.VITE_CONTROL_PLANE_URL !== undefined
  ? import.meta.env.VITE_CONTROL_PLANE_URL
  : 'http://127.0.0.1:8080'

export function useTunnelEvents(accessToken: string | null) {
  const queryClient = useQueryClient()
  const { setSessionAccessToken } = useAuthSession()
  const [isConnected, setIsConnected] = useState(false)

  useEffect(() => {
    if (!accessToken) {
      setIsConnected(false)
      return
    }

    const sseUrl = `${controlPlaneUrl}/api/v1/tunnels/events?token=${encodeURIComponent(accessToken)}`
    const eventSource = new EventSource(sseUrl)

    eventSource.onopen = () => {
      setIsConnected(true)
    }

    const handleUpdate = (event: MessageEvent) => {
      try {
        const updatedProfile = JSON.parse(event.data) as UserProfile
        if (updatedProfile && updatedProfile.id) {
          queryClient.setQueryData(userQueryKey, updatedProfile)
        }
      } catch (err) {
        console.error('[SSE] Failed to parse tunnel_update event:', err)
      }
    }

    eventSource.addEventListener('tunnel_update', handleUpdate)

    const handleBanned = () => {
      setSessionAccessToken(null)
      queryClient.removeQueries({ queryKey: userQueryKey })
    }

    eventSource.addEventListener('user_banned', handleBanned)

    eventSource.onerror = () => {
      setIsConnected(false)
    }

    return () => {
      eventSource.removeEventListener('tunnel_update', handleUpdate)
      eventSource.removeEventListener('user_banned', handleBanned)
      eventSource.close()
      setIsConnected(false)
    }
  }, [accessToken, queryClient, setSessionAccessToken])

  return { isConnected }
}

import { useQuery } from '@tanstack/react-query'
import { getCurrentUser } from '../lib/auth'

export const userQueryKey = ['user'] as const

export function useUser(accessToken: string | null) {
  return useQuery({
    queryKey: userQueryKey,
    queryFn: getCurrentUser,
    enabled: Boolean(accessToken),
    retry: false,
  })
}

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createTunnel, type CreateTunnelInput } from '../lib/api'
import { userQueryKey } from './useUser'

export function useCreateTunnel() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (input: CreateTunnelInput) => {
      const response = await createTunnel(input)
      return response.data
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userQueryKey })
    },
  })
}

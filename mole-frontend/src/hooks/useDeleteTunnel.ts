import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteTunnel } from '../lib/api'
import { userQueryKey } from './useUser'

export function useDeleteTunnel() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (tunnelId: string) => deleteTunnel(tunnelId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userQueryKey })
    },
  })
}

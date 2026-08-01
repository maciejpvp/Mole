import { useMutation, useQueryClient } from '@tanstack/react-query'
import { resetAdminUserLimits } from '../lib/api'

export function useResetAdminUserLimits() {
	const queryClient = useQueryClient()

	return useMutation({
		mutationFn: (userId: string) => resetAdminUserLimits(userId).then((response) => response.data),
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
		},
	})
}

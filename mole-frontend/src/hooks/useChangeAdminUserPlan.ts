import { useMutation, useQueryClient } from '@tanstack/react-query'
import { changeAdminUserPlan } from '../lib/api'

export function useChangeAdminUserPlan() {
	const queryClient = useQueryClient()

	return useMutation({
		mutationFn: ({ userId, planId }: { userId: string; planId: number }) =>
			changeAdminUserPlan(userId, planId).then((response) => response.data),
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
		},
	})
}

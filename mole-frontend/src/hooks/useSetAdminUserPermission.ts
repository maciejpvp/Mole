import { useMutation, useQueryClient } from '@tanstack/react-query'
import { setAdminUserPermission } from '../lib/api'

export function useSetAdminUserPermission() {
	const queryClient = useQueryClient()

	return useMutation({
		mutationFn: ({ userId, isAdmin }: { userId: string; isAdmin: boolean }) =>
			setAdminUserPermission(userId, isAdmin).then((response) => response.data),
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
		},
	})
}

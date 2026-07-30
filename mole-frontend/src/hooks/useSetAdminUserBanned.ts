import { useMutation, useQueryClient } from '@tanstack/react-query'
import { setAdminUserBanned } from '../lib/api'

export function useSetAdminUserBanned() {
	const queryClient = useQueryClient()

	return useMutation({
		mutationFn: ({ userId, isBanned }: { userId: string; isBanned: boolean }) =>
			setAdminUserBanned(userId, isBanned).then((response) => response.data),
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
		},
	})
}

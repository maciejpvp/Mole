import { useQuery } from '@tanstack/react-query'
import { listAdminUsers, type AdminUsersQuery } from '../lib/api'

export function useAdminUsers(query: AdminUsersQuery, enabled: boolean) {
	return useQuery({
		queryKey: ['admin-users', query],
		queryFn: () => listAdminUsers(query),
		enabled,
		placeholderData: (previous) => previous,
		retry: false,
	})
}

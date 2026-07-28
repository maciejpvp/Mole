import { useQuery } from '@tanstack/react-query'
import { listPlans } from '../lib/api'

export const plansQueryKey = ['plans'] as const

export function usePlans() {
	return useQuery({
		queryKey: plansQueryKey,
		queryFn: async () => (await listPlans()).data,
	})
}

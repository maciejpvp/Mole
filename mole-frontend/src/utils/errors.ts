export function errorMessage(error: unknown, fallback: string): string {
	if (typeof error === 'object' && error !== null && 'response' in error) {
		const response = error.response as { data?: { error?: string } }
		return response.data?.error ?? fallback
	}
	return fallback
}

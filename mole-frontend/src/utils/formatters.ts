export function formatBytes(bytes: number | null | undefined, emptyValue = '0 B'): string {
	if (bytes === null || bytes === undefined || !Number.isFinite(bytes)) return emptyValue
	if (bytes < 1024) return `${bytes} B`
	if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
	if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`
	return `${(bytes / 1024 ** 3).toFixed(2)} GB`
}

export function formatMinutes(minutes: number | null | undefined): string {
	if (minutes === null || minutes === undefined || !Number.isFinite(minutes)) return '0m'
	if (minutes < 60) return `${minutes}m`
	return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

export function formatDate(value: string | null | undefined, emptyValue = '—'): string {
	if (!value) return emptyValue
	return new Date(value).toLocaleString()
}

export function formatLimit(value: number | null): string {
	return value === null ? 'unlimited' : String(value)
}

export function formatOutboundAddress(serverAddress?: string, outboundPort?: number): string {
	if (!outboundPort) return serverAddress ?? ''
	if (!serverAddress) return `:${outboundPort}`

	let host = serverAddress.trim()
	const colonCount = (host.match(/:/g) || []).length
	if (colonCount === 1 || (colonCount > 1 && host.includes(']'))) {
		const lastColon = host.lastIndexOf(':')
		const possiblePort = host.substring(lastColon + 1)
		if (/^\d+$/.test(possiblePort)) host = host.substring(0, lastColon)
	}

	return `${host}:${outboundPort}`
}

export function clamp(value: number, minimum: number, maximum: number): number {
	return Math.min(maximum, Math.max(minimum, value))
}

export function roundFraction(value: number): number {
	return Math.round(clamp(value, 0, 1) * 1_000_000) / 1_000_000
}

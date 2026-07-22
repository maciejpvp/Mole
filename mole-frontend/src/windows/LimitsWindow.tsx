import type { UserProfile } from '../lib/auth'

function formatBytes(bytes: number | null) {
  if (bytes === null) return 'unlimited'
  if (bytes >= 1024 ** 3) return `${Math.round(bytes / 1024 ** 3)}GB`
  if (bytes >= 1024 ** 2) return `${Math.round(bytes / 1024 ** 2)}MB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)}KB`
  return `${bytes}B`
}

function formatLimit(value: number | null) {
  return value === null ? 'unlimited' : String(value)
}

export function LimitsWindow({ user }: { user: UserProfile }) {
  const tunnelCount = user.tunnels.length
  const { limits, usage } = user

  return (
    <div className="p-2 font-mono text-[16px] leading-6 text-white">
      <div className="mb-1 text-[#9ab4d2]">[ usage / limits ]</div>
      <pre className="m-0 whitespace-pre">{`Tunnels          ${tunnelCount} / ${formatLimit(limits.max_active_tunnels)}
Monthly Minutes  ${usage.monthly_minutes_used} / ${formatLimit(limits.monthly_minutes)}
Monthly Transfer ${formatBytes(usage.monthly_transfer_bytes_used)} / ${formatBytes(limits.monthly_transfer_bytes)}`}</pre>
    </div>
  )
}

import type { UserProfile } from '../lib/auth'
import { formatBytes, formatLimit } from '../utils'

export function LimitsWindow({ user }: { user: UserProfile; }) {
  const tunnelCount = user.tunnels.length
  const { limits, usage } = user

  return (
    <div className="p-2 font-mono text-[16px] leading-6 text-white">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-[#9ab4d2]">[ usage / limits ]</span>
      </div>
      <pre className="m-0 whitespace-pre">{`Tunnels          ${tunnelCount} / ${formatLimit(limits.max_active_tunnels)}
Monthly Minutes  ${usage.monthly_minutes_used} / ${formatLimit(limits.monthly_minutes)}
Monthly Transfer ${formatBytes(usage.monthly_transfer_bytes_used)} / ${limits.monthly_transfer_bytes === null ? 'unlimited' : formatBytes(limits.monthly_transfer_bytes)}`}</pre>
    </div>
  )
}

import { useState } from 'react'
import type { UserProfile } from '../lib/auth'
import { formatOutboundAddress } from '../lib/api'
import { useDeleteTunnel } from '../hooks/useDeleteTunnel'

function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || isNaN(bytes)) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function formatMinutes(minutes?: number): string {
  if (minutes === undefined || minutes === null || isNaN(minutes)) return '0m'
  if (minutes < 60) return `${minutes}m`
  const hrs = Math.floor(minutes / 60)
  const mins = minutes % 60
  return `${hrs}h ${mins}m`
}

type TunnelsWindowProps = {
  user: UserProfile
  onCreateTunnel?: () => void
  onDeleteTunnel?: (tunnelId: string) => void
}

export function TunnelsWindow({ user, onCreateTunnel, onDeleteTunnel }: TunnelsWindowProps) {
  const { tunnels = [] } = user
  const deleteTunnelMutation = useDeleteTunnel()
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const handleDeleteTunnel = (tunnelId: string) => {
    if (onDeleteTunnel) {
      onDeleteTunnel(tunnelId)
    } else {
      deleteTunnelMutation.mutate(tunnelId)
    }
  }

  const handleCopy = (address: string, id: string) => {
    void navigator.clipboard.writeText(address)
    setCopiedId(id)
    setTimeout(() => setCopiedId(null), 2000)
  }

  return (
    <div className="font-mono text-[13px] leading-5 text-[#c5c5c5] select-none">
      {/* ImGui Window Title Bar with Top Right Actions */}
      <div className="mb-2 flex items-center justify-between border-b border-[#2b2f3a] pb-1">
        <span className="text-[#569cd6] text-xs font-bold tracking-wider uppercase">
          [=] Active Tunnels [{tunnels.length}]
        </span>
        <button
          onClick={onCreateTunnel}
          className="text-xs text-[#4ec9b0] hover:text-[#9cdcfe] hover:bg-[#2b2f3a] px-1 py-0.5 rounded-none transition-colors border border-transparent hover:border-[#404859]"
        >
          [ + Create Tunnel ]
        </button>
      </div>

      {tunnels.length === 0 ? (
        <div className="py-4 text-center border border-dashed border-[#2b2f3a]">
          <div className="text-[#6a9955] italic mb-2">// No active tunnels found</div>
          <button
            onClick={onCreateTunnel}
            className="text-[#4ec9b0] hover:text-[#9cdcfe] bg-[#1e222b] hover:bg-[#2b2f3a] px-2 py-1 text-xs border border-[#404859]"
          >
            [ + Create Tunnel ]
          </button>
        </div>
      ) : (
        <div className="whitespace-pre overflow-x-auto text-[#d4d4d4]">
          {/* Header Top Border */}
          <div className="text-[#404859]">╔═══════╦══════════════════╦══════════════════════════╦════════════╦══════════╦════════════╦══════════════════╗</div>
          
          {/* Table Header */}
          <div className="flex">
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> PROTO </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> INTERNAL         </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> OUTBOUND ADDRESS         </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> TRANSFER   </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> UPTIME    </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> STATUS     </span>
            <span className="text-[#404859]">║</span>
            <span className="text-[#569cd6]"> ACTIONS          </span>
            <span className="text-[#404859]">║</span>
          </div>

          {/* Header Divider */}
          <div className="text-[#404859]">╠═══════╬══════════════════╬══════════════════════════╬════════════╬══════════╬════════════╬══════════════════╣</div>

          {/* Rows */}
          {tunnels.map((tunnel) => {
            const isRowActive = tunnel.status === 'active'
            const statusColor = isRowActive
              ? 'text-[#4ec9b0]'
              : tunnel.status === 'stopped'
                ? 'text-[#f44747]'
                : 'text-[#808080]'
            const statusText = `[${tunnel.status.toUpperCase()}]`.padEnd(10)
            const fullOutboundAddress = formatOutboundAddress(tunnel.server_address, tunnel.outbound_port)
            const isDeleting = deleteTunnelMutation.isPending && deleteTunnelMutation.variables === tunnel.id
            const isCopied = copiedId === tunnel.id

            return (
              <div key={tunnel.id} className="flex items-center" title={`Tunnel ID: ${tunnel.id}`}>
                <span className="text-[#404859]">║</span>
                <span className="text-[#dcdcaa]"> {tunnel.proto.toUpperCase().padEnd(5)} </span>
                <span className="text-[#404859]">║</span>
                <span className="text-[#b5cea8]"> {tunnel.internal_address.padEnd(16)} </span>
                <span className="text-[#404859]">║</span>
                <span className="text-[#ce9178]"> {fullOutboundAddress.padEnd(24)} </span>
                <span className="text-[#404859]">║</span>
                <span className="text-[#b5cea8]"> {formatBytes(tunnel.current_period_transfer_bytes).padEnd(10)} </span>
                <span className="text-[#404859]">║</span>
                <span className="text-[#9cdcfe]"> {formatMinutes(tunnel.current_period_minutes).padEnd(8)} </span>
                <span className="text-[#404859]">║</span>
                <span className={` ${statusColor}`}> {statusText} </span>
                <span className="text-[#404859]">║</span>
                
                {/* Actions Cell */}
                <span className="inline-flex items-center justify-between w-[18ch] px-1">
                  <button
                    onClick={() => handleCopy(fullOutboundAddress, tunnel.id)}
                    className="text-[#4ec9b0] hover:text-[#9cdcfe] hover:bg-[#1e222b] px-1 transition-colors text-xs"
                    title="Copy Outbound Address"
                  >
                    {isCopied ? '[ copied ]' : '[ copy ]'}
                  </button>
                  <button
                    onClick={() => handleDeleteTunnel(tunnel.id)}
                    disabled={isDeleting}
                    className="text-[#f44747] hover:text-[#ff6666] hover:bg-[#3c1f1f] px-1 transition-colors disabled:opacity-50 text-xs"
                    title="Delete Tunnel"
                  >
                    {isDeleting ? '[ . ]' : '[ x ]'}
                  </button>
                </span>
                <span className="text-[#404859]">║</span>
              </div>
            )
          })}

          {/* Table Bottom Border */}
          <div className="text-[#404859]">╚═══════╩══════════════════╩══════════════════════════╩════════════╩══════════╩════════════╩══════════════════╝</div>
        </div>
      )}
    </div>
  )
}
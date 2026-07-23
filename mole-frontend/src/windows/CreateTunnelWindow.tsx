import { type FormEvent, useState } from 'react'
import { ImGuiButton, ImGuiInputString } from '../components/imgui'
import { formatOutboundAddress } from '../lib/api'
import { useCreateTunnel } from '../hooks/useCreateTunnel'

function errorMessage(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    const response = error.response as { data?: { error?: string } }
    return response.data?.error ?? 'Failed to create tunnel'
  }
  return 'Unable to reach the control plane'
}

type CreateTunnelWindowProps = {
  onClose?: () => void
}

export function CreateTunnelWindow({ onClose }: CreateTunnelWindowProps) {
  const [proto, setProto] = useState<'tcp' | 'udp'>('tcp')
  const [internalAddress, setInternalAddress] = useState('127.0.0.1:8080')
  const [copied, setCopied] = useState(false)
  const createTunnelMutation = useCreateTunnel()

  const handleSubmit = (e?: FormEvent) => {
    e?.preventDefault()
    if (!internalAddress.trim()) return

    createTunnelMutation.mutate({ proto, internal_address: internalAddress.trim() })
  }

  const handleCopy = (token: string) => {
    void navigator.clipboard.writeText(token)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const createdTunnel = createTunnelMutation.data

  if (createdTunnel) {
    return (
      <div className="space-y-4 font-mono text-[13px] text-[#c5c5c5]">
        <div className="p-2 border border-[#4ec9b0] bg-[#1e222b] text-[#4ec9b0]">
          <div className="font-bold text-xs uppercase mb-1">
            [!] Tunnel Created Successfully
          </div>
          <div className="text-xs text-[#d4d4d4]">
            Save your connection token now. It is only shown once and cannot be retrieved later.
          </div>
        </div>

        <div className="space-y-2 border border-[#2b2f3a] bg-[#15181e] p-3">
          <div>
            <span className="text-[#808080] text-xs">Tunnel ID: </span>
            <span className="text-[#ce9178] font-bold">{createdTunnel.id}</span>
          </div>

          <div>
            <span className="text-[#808080] text-xs">Connection Token: </span>
            <div className="mt-1 flex items-center gap-2">
              <input
                readOnly
                value={createdTunnel.token ?? ''}
                className="flex-1 bg-[#1a1a1a] border border-[#404859] px-2 py-1 text-xs text-[#dcdcaa] select-all outline-none font-mono"
              />
              <button
                type="button"
                onClick={() => handleCopy(createdTunnel.token ?? '')}
                className="text-xs text-[#4ec9b0] hover:text-[#9cdcfe] bg-[#1e222b] hover:bg-[#2b2f3a] px-2 py-1 border border-[#404859] transition-colors"
              >
                {copied ? '[ Copied! ]' : '[ Copy Token ]'}
              </button>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2 pt-2 border-t border-[#2b2f3a] text-xs">
            <div>
              <span className="text-[#808080]">Protocol: </span>
              <span className="text-[#dcdcaa]">{createdTunnel.proto?.toUpperCase()}</span>
            </div>
            <div>
              <span className="text-[#808080]">Outbound: </span>
              <span className="text-[#ce9178]">
                {formatOutboundAddress(createdTunnel.server_address, createdTunnel.outbound_port)}
              </span>
            </div>
            <div>
              <span className="text-[#808080]">Internal: </span>
              <span className="text-[#b5cea8]">{createdTunnel.internal_address}</span>
            </div>
          </div>
        </div>

        <div className="flex items-center justify-between pt-2 border-t border-[#2b2f3a]">
          <ImGuiButton
            onClick={() => {
              createTunnelMutation.reset()
              setCopied(false)
            }}
          >
            Create Another Tunnel
          </ImGuiButton>
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              className="text-xs text-[#808080] hover:text-[#c5c5c5] px-2 py-1 border border-[#404859] bg-[#1e222b]"
            >
              [ Close ]
            </button>
          )}
        </div>
      </div>
    )
  }

  const status = createTunnelMutation.isPending
    ? 'Creating tunnel…'
    : createTunnelMutation.error
      ? errorMessage(createTunnelMutation.error)
      : ''

  return (
    <form onSubmit={handleSubmit} className="space-y-4 font-mono text-[13px] text-[#c5c5c5]">
      <div className="space-y-1">
        <label className="block text-[#569cd6] text-xs font-bold uppercase">
          Protocol
        </label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setProto('tcp')}
            className={`px-3 py-1 text-xs border font-mono transition-colors ${
              proto === 'tcp'
                ? 'border-[#4ec9b0] text-[#4ec9b0] bg-[#1e222b]'
                : 'border-[#404859] text-[#808080] bg-[#1a1a1a] hover:text-[#c5c5c5]'
            }`}
          >
            [ TCP ]
          </button>
          <button
            type="button"
            onClick={() => setProto('udp')}
            className={`px-3 py-1 text-xs border font-mono transition-colors ${
              proto === 'udp'
                ? 'border-[#4ec9b0] text-[#4ec9b0] bg-[#1e222b]'
                : 'border-[#404859] text-[#808080] bg-[#1a1a1a] hover:text-[#c5c5c5]'
            }`}
          >
            [ UDP ]
          </button>
        </div>
      </div>

      <ImGuiInputString
        value={internalAddress}
        onChange={setInternalAddress}
        label="Internal Address (IP:Port)"
        ariaLabel="Internal Address"
      />

      {status && (
        <div
          className={`text-xs ${
            createTunnelMutation.error ? 'text-[#f44747]' : 'text-[#9ab4d2]'
          }`}
        >
          {status}
        </div>
      )}

      <div className="flex items-center gap-2 pt-2 border-t border-[#2b2f3a]">
        <ImGuiButton onClick={handleSubmit}>
          {createTunnelMutation.isPending ? 'Creating…' : 'Create Tunnel'}
        </ImGuiButton>
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            className="text-xs text-[#808080] hover:text-[#c5c5c5] px-2 py-1"
          >
            [ Cancel ]
          </button>
        )}
      </div>
    </form>
  )
}

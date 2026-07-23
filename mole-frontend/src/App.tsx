import { useState } from 'react'
import { ImGuiDesktop } from './components/ImGuiDesktop'
import { useAuthSession } from './auth/authSessionContext'
import { useUser } from './hooks/useUser'
import { useTunnelEvents } from './hooks/useTunnelEvents'
import { AuthWindow } from './windows/AuthWindow'
import { LimitsWindow } from './windows/LimitsWindow'
import { TunnelsWindow } from './windows/TunnelsWindow'
import { CreateTunnelWindow } from './windows/CreateTunnelWindow'

function App() {
  const { accessToken } = useAuthSession()
  const userQuery = useUser(accessToken)
  const { isConnected } = useTunnelEvents(accessToken)
  const [isCreateTunnelOpen, setIsCreateTunnelOpen] = useState(false)

  const windows = [
    {
      id: 'auth',
      title: 'Auth',
      layout: { x: 0.3, y: 0.13, width: 0.35, height: 0.35 },
      children: <AuthWindow />,
    },
    ...(userQuery.data ? [{
      id: 'limits',
      title: 'Limits',
      layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
      children: <LimitsWindow user={userQuery.data} />,
    }] : []),
    ...(userQuery.data ? [{
      id: 'tunnels',
      title: 'Tunnels',
      layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
      children: <TunnelsWindow user={userQuery.data} isLiveConnected={isConnected} onCreateTunnel={() => setIsCreateTunnelOpen(true)} />,
    }] : []),
    ...(userQuery.data && isCreateTunnelOpen ? [{
      id: 'create_tunnel',
      title: 'Create Tunnel',
      layout: { x: 0.35, y: 0.25, width: 0.3, height: 0.35 },
      showCloseBtn: true,
      onClose: () => setIsCreateTunnelOpen(false),
      children: <CreateTunnelWindow onClose={() => setIsCreateTunnelOpen(false)} />,
    }] : []),
  ]

  return <ImGuiDesktop windows={windows} />
}

export default App

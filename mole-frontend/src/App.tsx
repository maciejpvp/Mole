import { useMemo } from 'react'
import { ImGuiDesktop } from './components/ImGuiDesktop'
import { useAuthSession } from './auth/authSessionContext'
import { useUser } from './hooks/useUser'
import { useTunnelEvents } from './hooks/useTunnelEvents'
import { AuthWindow } from './windows/AuthWindow'
import { LimitsWindow } from './windows/LimitsWindow'
import { TunnelsWindow } from './windows/TunnelsWindow'
import { CreateTunnelWindow } from './windows/CreateTunnelWindow'
import type { WindowConfig } from './types/window'
import { useWindowManager } from './hooks/useWindowManager'
import { WindowProvider } from './context/WindowProvider'

function App() {
  const { accessToken } = useAuthSession()
  const userQuery = useUser(accessToken)
  useTunnelEvents(accessToken)

  const windowConfigs: WindowConfig[] = useMemo(() => {
    const user = userQuery.data
    return [
      {
        id: 'auth',
        title: 'Auth',
        layout: { x: 0.3, y: 0.13, width: 0.35, height: 0.35 },
        children: <AuthWindow />,
        enabled: true,
      },
      {
        id: 'limits',
        title: 'Limits',
        layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
        children: user ? <LimitsWindow user={user} /> : null,
        enabled: !!user,
      },
      {
        id: 'tunnels',
        title: 'Tunnels',
        layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
        children: user ? <TunnelsWindow user={user} /> : null,
        enabled: !!user,
      },
      {
        id: 'create_tunnel',
        title: 'Create Tunnel',
        layout: { x: 0.35, y: 0.25, width: 0.3, height: 0.35 },
        showCloseBtn: true,
        children: <CreateTunnelWindow />,
        enabled: !!user,
        defaultOpen: false,
      },
    ]
  }, [userQuery.data])

  const windowManager = useWindowManager(windowConfigs)

  return (
    <WindowProvider value={windowManager}>
      <ImGuiDesktop windows={windowManager.visibleWindows} />
    </WindowProvider>
  )
}

export default App

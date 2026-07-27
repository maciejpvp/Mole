import { ImGuiDesktop } from './components/ImGuiDesktop'
import { useAuthSession } from './auth/authSessionContext'
import { useUser } from './hooks/useUser'
import { useTunnelEvents } from './hooks/useTunnelEvents'
import { useAppWindows } from './hooks/useAppWindows'
import { WindowProvider } from './context/WindowProvider'

function App() {
  const { accessToken } = useAuthSession()
  const userQuery = useUser(accessToken)
  useTunnelEvents(accessToken)
  const windowManager = useAppWindows(userQuery.data)

  return (
    <WindowProvider value={windowManager}>
      <ImGuiDesktop windows={windowManager.visibleWindows} />
    </WindowProvider>
  )
}

export default App

import { ImGuiDesktop } from './components/ImGuiDesktop'
import { useAuthSession } from './auth/authSessionContext'
import { useUser } from './hooks/useUser'
import { AuthWindow } from './windows/AuthWindow'
import { LimitsWindow } from './windows/LimitsWindow'

function App() {
  const { accessToken } = useAuthSession()
  const userQuery = useUser(accessToken)
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
      layout: { x: 0.05, y: 0.4, width: 0.20, height: 0.16 },
      children: <LimitsWindow user={userQuery.data} />,
    }] : []),
  ]

  return <ImGuiDesktop windows={windows} />
}

export default App

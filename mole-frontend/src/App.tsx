import { ImGuiDesktop } from './components/ImGuiDesktop'
import { AuthWindow } from './windows/AuthWindow'

const windows = [
  {
    id: 'auth',
    title: 'Auth',
    layout: { x: 0.3, y: 0.13, width: 0.3, height: 0.35 },
    children: <AuthWindow />,
  },
]

function App() {
  return <ImGuiDesktop windows={windows} />
}

export default App

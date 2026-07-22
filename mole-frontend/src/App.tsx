import { useState } from 'react'
import { ImGuiDesktop } from './components/ImGuiDesktop'
import { ImGuiButton, ImGuiInputString } from './components/imgui'

function App() {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [message, setMessage] = useState('')

  const registerMode = mode === 'register'
  const switchMode = (nextMode: 'login' | 'register') => {
    setMode(nextMode)
    setMessage('')
  }

  const submit = () => {
    setMessage(registerMode ? 'Sign up requested' : 'Login requested')
  }

  const windows = [
    {
      id: 'Auth',
      title: 'Auth',
      layout: { x: 0.3, y: 0.13, width: 0.3, height: 0.35 },
      children: (
        <div className="space-y-3">
          {registerMode && (
            <ImGuiInputString
              value={email}
              onChange={setEmail}
              label="email"
              ariaLabel="Email"
              type="email"
            />
          )}
          <ImGuiInputString
            value={username}
            onChange={setUsername}
            label="username"
            ariaLabel="Username"
          />
          <ImGuiInputString
            value={password}
            onChange={setPassword}
            label="password"
            ariaLabel="Password"
            type="password"
          />
          <div className="flex items-center gap-3">
            <ImGuiButton onClick={submit}>{registerMode ? 'Sign Up' : 'Login'}</ImGuiButton>
            <ImGuiButton onClick={() => switchMode(registerMode ? 'login' : 'register')}>
              {registerMode ? 'Login Instead' : 'Register Instead'}
            </ImGuiButton>
            {message && <span className="text-[14px] text-[#9ab4d2]">{message}</span>}
          </div>
        </div>
      ),
    },
  ]

  return (
    <ImGuiDesktop windows={windows} />
  )
}

export default App

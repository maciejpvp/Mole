import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ImGuiButton, ImGuiInputString } from '../components/imgui'
import { useAuthSession } from '../auth/authSessionContext'
import { useUser, userQueryKey } from '../hooks/useUser'
import { login, register } from '../lib/auth'

function errorMessage(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    const response = error.response as { data?: { error?: string } }
    return response.data?.error ?? 'Authentication request failed'
  }
  return 'Unable to reach the control plane'
}

type AuthFormData = {
  email: string
  username: string
  password: string
}

const emptyForm: AuthFormData = {
  email: '',
  username: '',
  password: '',
}

export function AuthWindow() {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [formData, setFormData] = useState<AuthFormData>(emptyForm)
  const { accessToken, setSessionAccessToken } = useAuthSession()
  const queryClient = useQueryClient()

  const registerMode = mode === 'register'
  const userQuery = useUser(accessToken)
  const updateField = (field: keyof AuthFormData) => (value: string) => {
    setFormData((current) => ({ ...current, [field]: value }))
  }
  const authMutation = useMutation({
    mutationFn: () => registerMode
      ? register(formData)
      : login({ identifier: formData.username, password: formData.password }),
    onSuccess: (authentication) => {
      setSessionAccessToken(authentication.access_token)
      void queryClient.invalidateQueries({ queryKey: userQueryKey })
    },
  })

  const status = authMutation.isPending
    ? registerMode ? 'Creating account…' : 'Signing in…'
    : authMutation.error
      ? errorMessage(authMutation.error)
      : userQuery.error
        ? 'Your saved session has expired. Please sign in again.'
        : ''

  const switchMode = () => {
    const nextMode = mode === 'login' ? 'register' : 'login'
    setMode(nextMode)
    authMutation.reset()
  }

  const logout = () => {
    setSessionAccessToken(null)
    queryClient.removeQueries({ queryKey: userQueryKey })
  }

  if (userQuery.data) {
    return (
      <div className="flex items-center gap-3">
        <span>Signed in as {userQuery.data.username}</span>
        <ImGuiButton onClick={logout}>Log Out</ImGuiButton>
      </div>
    )
  }

  if (accessToken && userQuery.isFetching) {
    return <span className="text-[14px] text-[#9ab4d2]">Loading account…</span>
  }

  return (
    <div className="space-y-3">
      {registerMode && (
        <ImGuiInputString
          value={formData.email}
          onChange={updateField('email')}
          label="email"
          ariaLabel="Email"
          type="email"
        />
      )}
      <ImGuiInputString
        value={formData.username}
        onChange={updateField('username')}
        label="username"
        ariaLabel="Username"
      />
      <ImGuiInputString
        value={formData.password}
        onChange={updateField('password')}
        label="password"
        ariaLabel="Password"
        type="password"
      />
      <div className="flex items-center gap-3">
        <ImGuiButton onClick={() => authMutation.mutate()}>{registerMode ? 'Sign Up' : 'Login'}</ImGuiButton>
        <ImGuiButton onClick={switchMode}>
          {registerMode ? 'Login Instead' : 'Register Instead'}
        </ImGuiButton>
        {status && <span className="text-[14px] text-[#9ab4d2]">{status}</span>}
      </div>
    </div>
  )
}

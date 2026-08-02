import { useState } from 'react'
import { CardValidation } from '../components/CardValidation'
import { ImGuiButton } from '../components/imgui'
import { useAuthSession } from '../auth/authSessionContext'
import { useUser } from '../hooks/useUser'
import type { CardValidationConfirmation } from '../lib/billing'
import { errorMessage } from '../utils'

type VerificationState = 'idle' | 'verifying' | 'verified' | 'processing' | 'error'

export function CardVerificationWindow() {
  const { accessToken } = useAuthSession()
  const userQuery = useUser(accessToken)
  const [verificationState, setVerificationState] = useState<VerificationState>('idle')
  const [verificationMessage, setVerificationMessage] = useState('')

  const handleResult = async (result: CardValidationConfirmation) => {
    if (!result.card_verified || result.plan !== 'free') {
      setVerificationState(result.status === 'processing' ? 'processing' : 'error')
      setVerificationMessage(result.status === 'processing'
        ? 'Card validation is pending. Your account remains restricted until Stripe completes it.'
        : result.status === 'requires_action'
          ? 'Additional authentication is required to validate your card.'
          : result.status === 'canceled'
            ? 'Card validation was canceled. Please try again.'
            : 'Card validation did not complete. Please try again.')
      return
    }

    setVerificationState('verifying')
    setVerificationMessage('Card validated. Confirming your free-tier account…')
    try {
      const refreshed = await userQuery.refetch()
      if (refreshed.data?.plan === 'free') {
        setVerificationState('verified')
        setVerificationMessage('Card validated. Free-tier access enabled.')
        return
      }
      setVerificationState('processing')
      setVerificationMessage('Verification is pending. Your account remains restricted until the profile updates.')
    } catch (error: unknown) {
      setVerificationState('error')
      setVerificationMessage(errorMessage(error, 'Unable to refresh your account status. Please try again.'))
    }
  }

  const checkStatus = async () => {
    setVerificationState('verifying')
    setVerificationMessage('Checking verification status…')
    try {
      const refreshed = await userQuery.refetch()
      if (refreshed.data?.plan === 'free') {
        setVerificationState('verified')
        setVerificationMessage('Card validated. Free-tier access enabled.')
      } else {
        setVerificationState('processing')
        setVerificationMessage('Verification is still pending. Your account remains restricted.')
      }
    } catch (error: unknown) {
      setVerificationState('error')
      setVerificationMessage(errorMessage(error, 'Unable to refresh your account status. Please try again.'))
    }
  }

  if (userQuery.data?.plan === 'free' || verificationState === 'verified') {
    return <div className="text-[#4ec9b0]">{verificationMessage || 'Free-tier access enabled.'}</div>
  }

  return (
    <div className="space-y-3">
      {verificationState === 'processing' && (
        <div className="space-y-2 text-[#dcdcaa]">
          <div>{verificationMessage}</div>
          <ImGuiButton onClick={() => void checkStatus()}>Check Status</ImGuiButton>
        </div>
      )}
      <CardValidation
        onStart={() => {
          setVerificationState('verifying')
          setVerificationMessage('Validating your card…')
        }}
        onResult={(result) => void handleResult(result)}
        onError={(message) => {
          setVerificationState('error')
          setVerificationMessage(message)
        }}
      />
      {verificationState === 'error' && <div className="text-[#f44747]">{verificationMessage}</div>}
    </div>
  )
}

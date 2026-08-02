import { useEffect, useState } from 'react'
import { Elements, PaymentElement, useElements, useStripe } from '@stripe/react-stripe-js'
import { loadStripe } from '@stripe/stripe-js'
import { ImGuiButton } from './imgui'
import { type CardValidationConfirmation, confirmCardValidation, createCardValidation } from '../lib/billing'
import { errorMessage } from '../utils'

type CardValidationProps = {
  onStart: () => void
  onResult: (result: CardValidationConfirmation) => void
  onError: (message: string) => void
  onRetryNeeded?: () => void
}

const publishableKey = import.meta.env.VITE_STRIPE_PUBLISHABLE_KEY
const stripePromise = publishableKey ? loadStripe(publishableKey) : null
const stripeAppearance = {
  theme: 'night' as const,
  variables: {
    colorPrimary: '#569cd6',
    colorBackground: '#111318',
    colorText: '#d4d4d4',
    colorTextSecondary: '#9ab4d2',
    colorTextPlaceholder: '#808080',
    fontFamily: 'monospace',
    fontSizeBase: '14px',
    borderRadius: '0px',
    spacingUnit: '4px',
  },
  rules: {
    '.Tab': {
      backgroundColor: '#1a1a1a',
      border: '1px solid #404859',
      boxShadow: 'none',
    },
    '.Tab:hover': {
      backgroundColor: '#203755',
      color: '#ffffff',
    },
    '.Tab--selected': {
      backgroundColor: '#203755',
      borderColor: '#569cd6',
      boxShadow: '0 0 0 1px #569cd6',
      color: '#ffffff',
    },
    '.Input': {
      backgroundColor: '#111318',
      border: '1px solid #404859',
      boxShadow: 'none',
    },
    '.Input:focus': {
      borderColor: '#569cd6',
      boxShadow: '0 0 0 1px #569cd6',
    },
    '.Label': {
      color: '#9ab4d2',
    },
    '.Block': {
      backgroundColor: '#1a1a1a',
      borderColor: '#404859',
    },
  },
}

function CardValidationForm({ clientSecret, onStart, onResult, onError, onRetryNeeded }: CardValidationProps & { clientSecret: string }) {
  const stripe = useStripe()
  const elements = useElements()
  const [status, setStatus] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const stripeErrorMessage = (error: { message?: string; code?: string; decline_code?: string }) => {
    const code = error.code || error.decline_code
    return error.message
      ? `${error.message}${code ? ` (${code})` : ''}`
      : code || 'Card validation failed.'
  }

  const validateCard = async () => {
    if (!stripe || !elements) {
      const message = 'Stripe is still loading. Please try again.'
      setStatus(message)
      onError(message)
      return
    }

    setIsSubmitting(true)
    onStart()
    setStatus('Preparing secure card validation…')
    try {
      const submission = await elements.submit()
      if (submission.error) {
        const message = stripeErrorMessage(submission.error)
        setStatus(message)
        onError(message)
        onRetryNeeded?.()
        return
      }

      const confirmation = await stripe.confirmSetup({
        elements,
        clientSecret,
        redirect: 'if_required',
      })
      if (confirmation.error || !confirmation.setupIntent) {
        const message = confirmation.error
          ? stripeErrorMessage(confirmation.error)
          : 'Stripe did not return a SetupIntent after confirming the card. Please retry.'
        setStatus(message)
        onError(message)
        onRetryNeeded?.()
        return
      }
      setStatus('Finalizing card validation…')
      try {
        const response = await confirmCardValidation(confirmation.setupIntent.id)
        setStatus(response.data.card_verified && response.data.plan === 'free'
          ? 'Card validated. Confirming free-tier access…'
          : response.data.status === 'processing'
            ? 'Card validation is still processing…'
            : 'Card validation did not complete.')
        onResult(response.data)
      } catch (error: unknown) {
        const response = 'response' in Object(error) ? Object(error).response : undefined
        const result = response && typeof response === 'object' && 'data' in response
          ? (response as { data?: CardValidationConfirmation }).data
          : undefined
        if (result && typeof result === 'object' && 'status' in result) {
          setStatus(result.status === 'processing'
            ? 'Card validation is still processing…'
            : result.status === 'requires_action'
              ? 'Additional authentication is required to validate this card.'
              : result.status === 'canceled'
                ? 'Card validation was canceled.'
                : 'Card validation was not completed.')
          onResult(result)
          onRetryNeeded?.()
          return
        }
        throw error
      }
    } catch (error: unknown) {
      const message = errorMessage(error, 'Card validation failed.')
      setStatus(message)
      onError(message)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="min-w-0 space-y-3 border border-[#404859] p-3">
      <div className="text-[#dcdcaa]">Card validation required for free-tier access.</div>
      <div className="min-w-0 overflow-hidden border border-[#404859] bg-[#111318] p-2">
        <PaymentElement options={{ layout: 'tabs' }} />
      </div>
      <ImGuiButton onClick={() => void validateCard()} disabled={isSubmitting || !stripe}>
        {isSubmitting ? 'Validating…' : 'Validate Card'}
      </ImGuiButton>
      {status && <div className="text-[14px] text-[#9ab4d2]">{status}</div>}
    </div>
  )
}

export function CardValidation({ onStart, onResult, onError }: CardValidationProps) {
  const [clientSecret, setClientSecret] = useState('')
  const [setupError, setSetupError] = useState('')
  const [isPreparing, setIsPreparing] = useState(true)

  const prepareValidation = () => {
    setIsPreparing(true)
    setSetupError('')
    void createCardValidation()
      .then((response) => setClientSecret(response.data.client_secret))
      .catch((error: unknown) => setSetupError(errorMessage(error, 'Unable to prepare card validation.')))
      .finally(() => setIsPreparing(false))
  }

  useEffect(() => {
    if (publishableKey) prepareValidation()
  }, [])

  if (!publishableKey) {
    return <div className="text-[14px] text-[#f44747]">// Stripe card validation is not configured.</div>
  }
  if (isPreparing) {
    return <div className="text-[14px] text-[#9ab4d2]">Preparing secure card validation…</div>
  }
  if (setupError || !clientSecret) {
    return (
      <div className="space-y-2 text-[14px] text-[#f44747]">
        <div>{setupError || 'Unable to prepare card validation.'}</div>
        <ImGuiButton onClick={prepareValidation}>Retry</ImGuiButton>
      </div>
    )
  }

  return (
    <Elements stripe={stripePromise} options={{ clientSecret, appearance: stripeAppearance }}>
      <CardValidationForm
        clientSecret={clientSecret}
        onStart={onStart}
        onResult={onResult}
        onError={onError}
        onRetryNeeded={prepareValidation}
      />
    </Elements>
  )
}

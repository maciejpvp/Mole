import { api } from './api'

export type CardValidationSetup = {
  setup_intent_id: string
  client_secret: string
  status: string
  card_verified: boolean
}

export type CardValidationConfirmation = {
  setup_intent_id: string
  status: string
  card_verified: boolean
  plan: string
}

export function createCardValidation() {
  return api.post<CardValidationSetup>('/api/v1/billing/card-validation/setup')
}

export function confirmCardValidation(setupIntentId: string) {
  return api.post<CardValidationConfirmation>('/api/v1/billing/card-validation/confirm', {
    setup_intent_id: setupIntentId,
  })
}

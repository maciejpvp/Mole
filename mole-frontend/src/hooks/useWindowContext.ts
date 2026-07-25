import { useContext } from 'react'
import { WindowContext } from '../context/WindowContext'
import type { WindowContextType } from '../types/window'

export function useWindowContext(): WindowContextType {
  const context = useContext(WindowContext)
  if (!context) {
    throw new Error('useWindowContext must be used within a WindowProvider')
  }
  return context
}

export const useWindow = useWindowContext

import type { ReactNode } from 'react'
import { WindowContext } from './WindowContext'
import type { WindowContextType } from '../types/window'

export interface WindowProviderProps {
  value: WindowContextType
  children: ReactNode
}

export function WindowProvider({ value, children }: WindowProviderProps) {
  return (
    <WindowContext.Provider value={value}>
      {children}
    </WindowContext.Provider>
  )
}

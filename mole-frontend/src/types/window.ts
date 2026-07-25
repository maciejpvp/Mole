import type { ReactNode } from 'react'

export interface WindowLayout {
  x: number
  y: number
  width: number
  height: number
}

export interface WindowConfig {
  id: string
  title: string
  layout: WindowLayout
  children: ReactNode
  showCloseBtn?: boolean
  onClose?: () => void
  /** Whether the user can resize this window. Defaults to true. */
  isResizable?: boolean
  /** Controls if the window is enabled based on global state (e.g. auth check) */
  enabled?: boolean
  defaultOpen?: boolean
}

export interface WindowContextType {
  openWindow: (id: string) => void
  closeWindow: (id: string) => void
  toggleWindow: (id: string) => void
  isWindowOpen: (id: string) => boolean
  openWindows: string[]
}

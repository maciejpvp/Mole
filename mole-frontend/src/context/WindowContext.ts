import { createContext } from 'react'
import type { WindowContextType } from '../types/window'

export const WindowContext = createContext<WindowContextType | undefined>(undefined)

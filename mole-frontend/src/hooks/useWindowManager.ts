import { useState, useCallback, useMemo } from 'react'
import type { WindowConfig } from '../types/window'

export function useWindowManager(configs: WindowConfig[]) {
  const [openState, setOpenState] = useState<Record<string, boolean>>({})

  const openWindow = useCallback((id: string) => {
    setOpenState((prev) => ({ ...prev, [id]: true }))
  }, [])

  const closeWindow = useCallback((id: string) => {
    setOpenState((prev) => ({ ...prev, [id]: false }))
  }, [])

  const toggleWindow = useCallback((id: string) => {
    setOpenState((prev) => {
      const config = configs.find((item) => item.id === id)
      const currentIsOpen =
        id in prev ? prev[id] : config ? config.defaultOpen !== false : false
      return { ...prev, [id]: !currentIsOpen }
    })
  }, [configs])

  const isWindowOpen = useCallback(
    (id: string) => {
      const config = configs.find((item) => item.id === id)
      if (config?.enabled === false) return false
      if (id in openState) return openState[id]
      return config ? config.defaultOpen !== false : false
    },
    [configs, openState],
  )

  const visibleWindows = useMemo(() => {
    return configs
      .filter((config) => config.enabled !== false)
      .filter((config) => {
        if (config.id in openState) {
          return openState[config.id]
        }
        return config.defaultOpen !== false
      })
      .map((config) => ({
        ...config,
        onClose: () => {
          config.onClose?.()
          closeWindow(config.id)
        },
      }))
  }, [configs, openState, closeWindow])

  const openWindows = useMemo(
    () => visibleWindows.map((w) => w.id),
    [visibleWindows],
  )

  return {
    visibleWindows,
    openWindow,
    closeWindow,
    toggleWindow,
    isWindowOpen,
    openWindows,
  }
}

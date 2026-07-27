import { useState, useCallback, useMemo } from 'react'
import type { WindowConfig } from '../types/window'

export function useWindowManager(configs: WindowConfig[] = []) {
  const [openState, setOpenState] = useState<Record<string, boolean>>({})
  const [additionalConfigs, setAdditionalConfigs] = useState<WindowConfig[]>([])
  const allConfigs = useMemo(() => {
    const baseIDs = new Set(configs.map((config) => config.id))
    return [...configs, ...additionalConfigs.filter((config) => !baseIDs.has(config.id))]
  }, [additionalConfigs, configs])

  const addWindow = useCallback((config: WindowConfig, options: { open?: boolean } = {}) => {
    setAdditionalConfigs((current) => {
      const existingIndex = current.findIndex((item) => item.id === config.id)
      if (existingIndex === -1) return [...current, config]
      const next = [...current]
      next[existingIndex] = config
      return next
    })
    if (options.open !== false) {
      setOpenState((prev) => ({ ...prev, [config.id]: true }))
    }
  }, [])

  const removeWindow = useCallback((id: string) => {
    setAdditionalConfigs((current) => current.filter((config) => config.id !== id))
    setOpenState((prev) => {
      const next = { ...prev }
      delete next[id]
      return next
    })
  }, [])

  const openWindow = useCallback((id: string) => {
    setOpenState((prev) => ({ ...prev, [id]: true }))
  }, [])

  const closeWindow = useCallback((id: string) => {
    setOpenState((prev) => ({ ...prev, [id]: false }))
  }, [])

  const toggleWindow = useCallback((id: string) => {
    setOpenState((prev) => {
      const config = allConfigs.find((item) => item.id === id)
      const currentIsOpen =
        id in prev ? prev[id] : config ? config.defaultOpen !== false : false
      return { ...prev, [id]: !currentIsOpen }
    })
  }, [allConfigs])

  const isWindowOpen = useCallback(
    (id: string) => {
      const config = allConfigs.find((item) => item.id === id)
      if (config?.enabled === false) return false
      if (id in openState) return openState[id]
      return config ? config.defaultOpen !== false : false
    },
    [allConfigs, openState],
  )

  const visibleWindows = useMemo(() => {
    return allConfigs
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
  }, [allConfigs, openState, closeWindow])

  const openWindows = useMemo(
    () => visibleWindows.map((w) => w.id),
    [visibleWindows],
  )

  return {
    visibleWindows,
    addWindow,
    removeWindow,
    openWindow,
    closeWindow,
    toggleWindow,
    isWindowOpen,
    openWindows,
  }
}

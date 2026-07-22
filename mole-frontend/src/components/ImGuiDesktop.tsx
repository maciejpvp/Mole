import { type ReactNode, useEffect, useRef, useState } from 'react'
import { ImGuiWindowContainer, type WindowPixelLayout } from './imgui'

// Every value is a fraction of the desktop canvas: 0 is its top/left edge and
// 1 is its bottom/right edge. This makes saved layouts resolution independent.
export type NormalizedWindowLayout = {
  x: number
  y: number
  width: number
  height: number
}

export type DesktopWindow = {
  id: string
  title: string
  layout: NormalizedWindowLayout
  children: ReactNode
}

type ImGuiDesktopProps = {
  windows: DesktopWindow[]
}

type ManagedWindow = Omit<DesktopWindow, 'layout'> & {
  layout: NormalizedWindowLayout & { zIndex: number }
}

type CanvasSize = { width: number; height: number }
type PersistedDesktopState = {
  layouts: Record<string, NormalizedWindowLayout & { zIndex: number }>
  dismissedWindowIDs: string[]
}

const desktopStorageKey = 'mole.imgui.desktop'

function loadDesktopState(): PersistedDesktopState {
  try {
    const stored = localStorage.getItem(desktopStorageKey)
    if (!stored) return { layouts: {}, dismissedWindowIDs: [] }
    const parsed = JSON.parse(stored) as Partial<PersistedDesktopState>
    return {
      layouts: parsed.layouts ?? {},
      dismissedWindowIDs: parsed.dismissedWindowIDs ?? [],
    }
  } catch {
    return { layouts: {}, dismissedWindowIDs: [] }
  }
}

const clampFraction = (value: number) => Math.min(1, Math.max(0, value))
const roundedFraction = (value: number) => Math.round(clampFraction(value) * 1_000_000) / 1_000_000

export function ImGuiDesktop({ windows }: ImGuiDesktopProps) {
  const desktopRef = useRef<HTMLElement>(null)
  const persistedState = useRef(loadDesktopState())
  const [canvasSize, setCanvasSize] = useState<CanvasSize>({ width: 0, height: 0 })
  const [dismissedWindowIDs, setDismissedWindowIDs] = useState<Set<string>>(
    () => new Set(persistedState.current.dismissedWindowIDs),
  )
  const [openWindows, setOpenWindows] = useState<ManagedWindow[]>(() =>
    windows
      .filter((window) => !persistedState.current.dismissedWindowIDs.includes(window.id))
      .map((window, index) => ({
        ...window,
        layout: {
          ...window.layout,
          ...persistedState.current.layouts[window.id],
          zIndex: persistedState.current.layouts[window.id]?.zIndex ?? index + 1,
        },
      })),
  )

  // Reconcile dynamic definitions (such as user-only windows) while retaining
  // user-closed windows as dismissed instead of reopening them on each render.
  useEffect(() => {
    setOpenWindows((current) => {
      const availableIDs = new Set(windows.map((window) => window.id))
      const retained = current.filter((window) => availableIDs.has(window.id))
      const currentIDs = new Set(retained.map((window) => window.id))
      const topZIndex = Math.max(...retained.map((window) => window.layout.zIndex), 0)
      const additions = windows
        .filter((window) => !currentIDs.has(window.id) && !dismissedWindowIDs.has(window.id))
        .map((window, index) => ({
          ...window,
          layout: {
            ...window.layout,
            ...persistedState.current.layouts[window.id],
            zIndex: persistedState.current.layouts[window.id]?.zIndex ?? topZIndex + index + 1,
          },
        }))
      if (!additions.length && retained.length === current.length) return current
      return [...retained, ...additions]
    })
  }, [dismissedWindowIDs, windows])

  useEffect(() => {
    const layouts = { ...persistedState.current.layouts }
    openWindows.forEach((window) => {
      layouts[window.id] = window.layout
    })
    const nextState: PersistedDesktopState = {
      layouts,
      dismissedWindowIDs: [...dismissedWindowIDs],
    }
    persistedState.current = nextState
    try {
      localStorage.setItem(desktopStorageKey, JSON.stringify(nextState))
    } catch {
      // Storage can be unavailable in private browsing; the desktop remains usable.
    }
  }, [dismissedWindowIDs, openWindows])

  useEffect(() => {
    const desktop = desktopRef.current
    if (!desktop) return
    const updateSize = () => setCanvasSize({ width: desktop.clientWidth, height: desktop.clientHeight })
    updateSize()
    const observer = new ResizeObserver(updateSize)
    observer.observe(desktop)
    return () => observer.disconnect()
  }, [])

  const bringToFront = (id: string) => {
    setOpenWindows((current) => {
      const topZIndex = Math.max(...current.map((window) => window.layout.zIndex), 0)
      return current.map((window) =>
        window.id === id ? { ...window, layout: { ...window.layout, zIndex: topZIndex + 1 } } : window,
      )
    })
  }

  const toPixels = (layout: ManagedWindow['layout']): WindowPixelLayout => ({
    x: layout.x * canvasSize.width,
    y: layout.y * canvasSize.height,
    width: layout.width * canvasSize.width,
    height: layout.height * canvasSize.height,
    zIndex: layout.zIndex,
  })

  const toNormalized = (layout: Omit<WindowPixelLayout, 'zIndex'>): NormalizedWindowLayout => ({
    x: roundedFraction(layout.x / canvasSize.width),
    y: roundedFraction(layout.y / canvasSize.height),
    width: roundedFraction(layout.width / canvasSize.width),
    height: roundedFraction(layout.height / canvasSize.height),
  })

  return (
    <main ref={desktopRef} className="relative min-h-screen overflow-hidden bg-zinc-900 font-mono" aria-label="ImGui desktop">
      {canvasSize.width > 0 && canvasSize.height > 0 && openWindows.map((window) => {
        // Keep window geometry local to the desktop, but take the current
        // children from App. This preserves controlled widget updates.
        const definition = windows.find((item) => item.id === window.id) ?? window
        return (
          <ImGuiWindowContainer
            key={window.id}
            title={definition.title}
            layout={toPixels(window.layout)}
            onFocus={() => bringToFront(window.id)}
            onClose={() => {
              setDismissedWindowIDs((current) => new Set(current).add(window.id))
              setOpenWindows((current) => current.filter((item) => item.id !== window.id))
            }}
            onProgrammaticResize={(size) =>
              setOpenWindows((current) =>
                current.map((item) => item.id === window.id ? {
                  ...item,
                  layout: {
                    ...item.layout,
                    width: Math.min(1, Math.max(0, size.width)),
                    height: Math.min(1, Math.max(0, size.height)),
                  },
                } : item),
              )
            }
            onLayoutChange={(layout) =>
              setOpenWindows((current) =>
                current.map((item) => item.id === window.id ? {
                  ...item,
                  layout: { ...toNormalized(layout), zIndex: item.layout.zIndex },
                } : item),
              )
            }
          >
            {definition.children}
          </ImGuiWindowContainer>
        )
      })}
    </main>
  )
}

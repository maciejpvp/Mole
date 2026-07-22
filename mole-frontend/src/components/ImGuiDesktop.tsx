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

const clampFraction = (value: number) => Math.min(1, Math.max(0, value))
const roundedFraction = (value: number) => Math.round(clampFraction(value) * 10_000) / 10_000

export function ImGuiDesktop({ windows }: ImGuiDesktopProps) {
  const desktopRef = useRef<HTMLElement>(null)
  const [canvasSize, setCanvasSize] = useState<CanvasSize>({ width: 0, height: 0 })
  const [openWindows, setOpenWindows] = useState<ManagedWindow[]>(() =>
    windows.map((window, index) => ({ ...window, layout: { ...window.layout, zIndex: index + 1 } })),
  )

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
            onClose={() => setOpenWindows((current) => current.filter((item) => item.id !== window.id))}
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

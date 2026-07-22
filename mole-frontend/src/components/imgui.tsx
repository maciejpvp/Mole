import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { Rnd } from 'react-rnd'

// react-rnd requires pixels. ImGuiDesktop converts its normalized layout into
// this rendering-only shape at the edge of the windowing system.
export type WindowPixelLayout = {
  x: number
  y: number
  width: number
  height: number
  zIndex: number
}

export type NormalizedWindowSize = {
  width: number
  height: number
}

type ImGuiWindowControls = {
  setSize: (size: NormalizedWindowSize) => void
}

const ImGuiWindowContext = createContext<ImGuiWindowControls | null>(null)

export function useImGuiWindow() {
  const controls = useContext(ImGuiWindowContext)
  if (!controls) throw new Error('useImGuiWindow must be used inside ImGuiWindowContainer')
  return controls
}

type ImGuiWindowContainerProps = {
  title: string
  layout: WindowPixelLayout
  children: ReactNode
  onFocus: () => void
  onClose: () => void
  onLayoutChange: (layout: Omit<WindowPixelLayout, 'zIndex'>) => void
  showCloseBtn?: boolean
  isResizable?: boolean
  onProgrammaticResize: (size: NormalizedWindowSize) => void
}

export function ImGuiWindowContainer({
  title,
  layout,
  children,
  onFocus,
  onClose,
  onLayoutChange,
  showCloseBtn = false,
  isResizable = true,
  onProgrammaticResize,
}: ImGuiWindowContainerProps) {
  const [collapsed, setCollapsed] = useState(false)
  const resizeHandler = useRef(onProgrammaticResize)
  resizeHandler.current = onProgrammaticResize
  const setSize = useCallback((size: NormalizedWindowSize) => resizeHandler.current(size), [])
  const controls = useMemo(() => ({ setSize }), [setSize])

  return (
    <Rnd
      bounds="parent"
      position={{ x: layout.x, y: layout.y }}
      size={{ width: layout.width, height: collapsed ? 28 : layout.height }}
      minWidth={isResizable && !collapsed ? 220 : 0}
      minHeight={isResizable && !collapsed ? 132 : 28}
      enableResizing={isResizable && !collapsed}
      dragHandleClassName="imgui-window-titlebar"
      onMouseDown={onFocus}
      onDragStop={(_, position) =>
        onLayoutChange({ x: position.x, y: position.y, width: layout.width, height: layout.height })
      }
      onResizeStop={(_, __, element, ___, position) =>
        onLayoutChange({
          x: position.x,
          y: position.y,
          width: element.offsetWidth,
          height: element.offsetHeight,
        })
      }
      resizeHandleComponent={{
        bottomRight: <span className="imgui-resize-grip" aria-label="Resize window" />,
      }}
      style={{ zIndex: layout.zIndex }}
      className="overflow-hidden rounded-md border border-[#506982] bg-[#1a1a1a] shadow-[0_3px_8px_rgba(0,0,0,0.45)]"
    >
      <ImGuiWindowContext.Provider value={controls}>
      <div className="imgui-window-titlebar flex h-7 cursor-grab select-none items-center bg-[#2d4b75] px-2 font-mono text-[17px] leading-none text-white active:cursor-grabbing">
        <button
          type="button"
          aria-label={collapsed ? `Expand ${title}` : `Collapse ${title}`}
          aria-expanded={!collapsed}
          className="-ml-1 mr-2 grid h-6 w-6 cursor-pointer place-items-center text-[15px] leading-none hover:bg-white/10"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={() => setCollapsed((current) => !current)}
        >
          {collapsed ? '▲' : '▼'}
        </button>
        <span>{title}</span>
        {showCloseBtn && (
          <button
            type="button"
            aria-label={`Close ${title}`}
            className="ml-auto -mr-0.5 grid h-6 w-6 cursor-pointer place-items-center text-[21px] font-light leading-none text-white hover:bg-white/10"
            onPointerDown={(event) => event.stopPropagation()}
            onClick={onClose}
          >
            ×
          </button>
        )}
      </div>
      {!collapsed && (
        <div className="h-[calc(100%-1.75rem)] overflow-auto bg-[#1a1a1a] p-3 font-mono text-[17px] leading-6 text-white">
          {children}
        </div>
      )}
      </ImGuiWindowContext.Provider>
    </Rnd>
  )
}

export function ImGuiText({ children }: { children: ReactNode }) {
  return <p className="m-0 font-mono text-[17px] leading-6 text-white">{children}</p>
}

export function ImGuiButton({ children, onClick }: { children: ReactNode; onClick?: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="h-7 border border-[#3d608d] bg-[#2d4b75] px-1.5 font-mono text-[17px] leading-none text-white shadow-[inset_0_1px_rgba(255,255,255,0.12)] hover:bg-[#3a5d8a] active:translate-y-px active:bg-[#254165]"
    >
      {children}
    </button>
  )
}

export type LabelLocation = 'left' | 'right' | 'top'

type ImGuiInputStringProps = {
  value: string
  onChange: (value: string) => void
  label?: string
  ariaLabel?: string
  type?: 'text' | 'email' | 'password'
  labelLocation?: LabelLocation
}

export function ImGuiInputString({
  value,
  onChange,
  label = 'string',
  ariaLabel = 'String value',
  type = 'text',
  labelLocation = 'top',
}: ImGuiInputStringProps) {
  const input = (
    <input
      aria-label={ariaLabel}
      type={type}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="min-w-0 flex-1 border-0 bg-[#203755] px-1.5 text-[17px] text-white outline-none ring-1 ring-transparent focus:ring-[#607fa8]"
    />
  )

  if (labelLocation === 'top') {
    return (
      <label className="flex flex-col gap-1 font-mono text-[17px] leading-none text-white">
        <span>{label}</span>
        {input}
      </label>
    )
  }

  return (
    <label className="flex h-7 items-center gap-2 font-mono text-[17px] leading-none text-white">
      {labelLocation === 'left' && <span className="w-[70px]">{label}</span>}
      {input}
      {labelLocation === 'right' && <span className="w-[70px] text-right">{label}</span>}
    </label>
  )
}

type ImGuiDragFloatProps = {
  value: number
  onChange: (value: number) => void
  label?: string
  step?: number
}

export function ImGuiDragFloat({ value, onChange, label = 'float', step = 0.01 }: ImGuiDragFloatProps) {
  const drag = useRef<{ pointerId: number; startX: number; startValue: number } | null>(null)

  useEffect(() => {
    const updateValue = (event: PointerEvent) => {
      if (!drag.current || event.pointerId !== drag.current.pointerId) return
      const delta = Math.round((event.clientX - drag.current.startX) * step * 1000) / 1000
      onChange(Math.round((drag.current.startValue + delta) * 1000) / 1000)
    }
    const finishDrag = (event: PointerEvent) => {
      if (drag.current?.pointerId === event.pointerId) drag.current = null
    }
    window.addEventListener('pointermove', updateValue)
    window.addEventListener('pointerup', finishDrag)
    return () => {
      window.removeEventListener('pointermove', updateValue)
      window.removeEventListener('pointerup', finishDrag)
    }
  }, [onChange, step])

  return (
    <div className="flex h-7 items-center gap-2 font-mono text-[17px] leading-none text-white">
      <button
        type="button"
        aria-label={`${label}: ${value.toFixed(3)}. Drag horizontally to change.`}
        onPointerDown={(event) => {
          event.preventDefault()
          drag.current = { pointerId: event.pointerId, startX: event.clientX, startValue: value }
        }}
        className="min-w-0 flex-1 cursor-ew-resize select-none bg-[#2d4b75] px-1.5 text-right tabular-nums text-white outline-none ring-1 ring-transparent hover:bg-[#3a5d8a] focus:ring-[#607fa8]"
      >
        {value.toFixed(3)}
      </button>
      <span className="w-[70px] text-right">{label}</span>
    </div>
  )
}

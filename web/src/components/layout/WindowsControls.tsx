import React, { useState, useEffect } from 'react'

export const WindowsControls: React.FC = () => {
  const [isWindows, setIsWindows] = useState(false)
  const [isMaximized, setIsMaximized] = useState(false)

  useEffect(() => {
    const isWin = typeof navigator !== 'undefined' && /Win/.test(navigator.platform || navigator.userAgent)
    const isWails = typeof window !== 'undefined' && Boolean((window as any).runtime)
    // 如果是 Windows 或在非 Mac 的 Wails 环境下运行，则启用 Windows 样式控制按钮
    setIsWindows(isWin || (!/Mac/.test(navigator.userAgent) && isWails))

    const checkMaximized = () => {
      const rt = (window as any).runtime
      if (typeof rt?.WindowIsMaximised === 'function') {
        rt.WindowIsMaximised().then((val: boolean) => setIsMaximized(Boolean(val))).catch(() => {})
      }
    }
    checkMaximized()
    window.addEventListener('resize', checkMaximized)
    return () => window.removeEventListener('resize', checkMaximized)
  }, [])

  if (!isWindows) return null

  const handleMinimize = (e: React.MouseEvent) => {
    e.stopPropagation()
    const rt = (window as any).runtime
    if (typeof rt?.WindowMinimise === 'function') {
      rt.WindowMinimise()
    }
  }

  const handleMaximize = (e: React.MouseEvent) => {
    e.stopPropagation()
    const rt = (window as any).runtime
    if (typeof rt?.WindowToggleMaximise === 'function') {
      rt.WindowToggleMaximise()
      setIsMaximized((prev) => !prev)
    }
  }

  const handleClose = (e: React.MouseEvent) => {
    e.stopPropagation()
    const rt = (window as any).runtime
    if (typeof rt?.Quit === 'function') {
      rt.Quit()
    } else {
      window.close()
    }
  }

  return (
    <div
      className="flex items-center h-14 shrink-0 -mr-4 sm:-mr-6 border-l border-border/60 select-none ml-2"
      style={{ '--wails-draggable': 'no-drag', WebkitAppRegion: 'no-drag' } as React.CSSProperties}
    >
      {/* 最小化按钮 (固定 48px 宽，禁止挤压) */}
      <button
        type="button"
        onClick={handleMinimize}
        title="最小化"
        className="w-12 h-14 shrink-0 flex items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground transition-colors cursor-pointer focus:outline-hidden"
      >
        <svg className="w-4 h-4" viewBox="0 0 16 16" fill="currentColor">
          <path d="M2 9h12v1.2H2z" />
        </svg>
      </button>

      {/* 最大化 / 还原按钮 (固定 48px 宽，禁止挤压) */}
      <button
        type="button"
        onClick={handleMaximize}
        title={isMaximized ? '向下还原' : '最大化'}
        className="w-12 h-14 shrink-0 flex items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground transition-colors cursor-pointer focus:outline-hidden"
      >
        {isMaximized ? (
          <svg className="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2">
            <rect x="4.5" y="2.5" width="8.5" height="8.5" rx="0.5" />
            <path d="M2.5 5.5v7.5a.5.5 0 00.5.5h7.5" />
          </svg>
        ) : (
          <svg className="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2">
            <rect x="2.5" y="2.5" width="11" height="11" rx="0.5" />
          </svg>
        )}
      </button>

      {/* 关闭按钮 (固定 48px 宽，Windows 经典红色高亮，禁止挤压) */}
      <button
        type="button"
        onClick={handleClose}
        title="关闭"
        className="w-12 h-14 shrink-0 flex items-center justify-center text-muted-foreground hover:bg-[#e81123] hover:text-white active:bg-[#c42b1c] active:text-white transition-colors cursor-pointer focus:outline-hidden"
      >
        <svg className="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
          <path d="M3.5 3.5l9 9M12.5 3.5l-9 9" />
        </svg>
      </button>
    </div>
  )
}

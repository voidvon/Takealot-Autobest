import React, { useState, useEffect } from 'react'

export const WindowControls: React.FC = () => {
  const [isMac, setIsMac] = useState(false)

  useEffect(() => {
    const isMacPlatform = typeof navigator !== 'undefined' && /Mac|iPod|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
    setIsMac(isMacPlatform)
  }, [])

  // 非 macOS 环境（如 Windows）不在左侧显示占位符
  if (!isMac) {
    return null
  }

  // 在 macOS 桌面端下，系统原生 Traffic Lights 直接透显在此区域上方
  return (
    <div
      className="w-[70px] h-8 shrink-0 select-none mr-1"
      style={{ '--wails-draggable': 'drag', WebkitAppRegion: 'drag' } as React.CSSProperties}
      title="双击最大化 / 还原"
    />
  )
}

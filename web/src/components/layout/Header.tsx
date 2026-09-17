import React from 'react'
import type { EngineStatus } from '../../types'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Play, Pause, Square, Moon, Sun } from 'lucide-react'
import { formatMinutesSeconds } from '../../lib/countdown'

interface HeaderProps {
  version: string
  status: EngineStatus
  onStartReprice: () => void
  onPauseReprice: () => void
  onStopReprice: () => void
  darkMode: boolean
  onToggleDarkMode: () => void
  loadingAction: boolean
}

export const Header: React.FC<HeaderProps> = ({
  version,
  status,
  onStartReprice,
  onPauseReprice,
  onStopReprice,
  darkMode,
  onToggleDarkMode,
  loadingAction,
}) => {

  return (
    <header className="sticky top-0 z-40 w-full border-b border-border bg-background/95 backdrop-blur-sm">
      <div className="flex h-14 items-center justify-between px-4 sm:px-6">
        {/* Brand & Workspace info */}
        <div className="flex items-center gap-3">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary font-black text-primary-foreground text-sm shadow-xs">
            T
          </div>
          <div className="flex items-center gap-2">
            <h1 className="font-semibold text-sm text-foreground tracking-tight">
              Takealot 智能电商自动化中台
            </h1>
            <Badge variant="secondary" className="hidden sm:inline-flex text-[10px] font-mono px-1.5 h-4.5">
              v{version}
            </Badge>
          </div>
        </div>

        {/* Engine Live Metrics & Controls */}
        <div className="flex items-center gap-3 sm:gap-4">
          {/* Quick Metrics */}
          <div className="hidden lg:flex items-center gap-4 pr-3 border-r border-border text-xs">
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground text-[11px]">引擎态势:</span>
              {!status.is_running ? (
                <Badge variant="secondary" dot className="text-[10px] h-4.5">
                  已停止
                </Badge>
              ) : status.is_paused ? (
                <Badge variant="warning" dot className="text-[10px] h-4.5">
                  已暂停
                </Badge>
              ) : (
                <Badge variant="success" dot className="text-[10px] h-4.5">
                  巡检中
                </Badge>
              )}
            </div>

            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground text-[11px]">下次轮询:</span>
              <span className="font-mono text-foreground font-semibold text-xs">
                {status.is_running && !status.is_paused ? formatMinutesSeconds(status.countdown_seconds) : '--:--'}
              </span>
            </div>

            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground text-[11px]">今日调价:</span>
              <span className="font-mono text-primary font-semibold text-xs">
                {status.total_repriced} 次
              </span>
            </div>
          </div>

          {/* Quick Engine Actions */}
          <div className="flex items-center gap-1.5">
            {!status.is_running ? (
              <Button
                variant="default"
                size="sm"
                onClick={onStartReprice}
                loading={loadingAction}
                className="gap-1.5 text-xs h-8"
              >
                <Play className="h-3 w-3 fill-current" />
                <span>启动巡检</span>
              </Button>
            ) : (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onPauseReprice}
                  loading={loadingAction}
                  className="gap-1.5 text-xs h-8"
                >
                  {status.is_paused ? (
                    <>
                      <Play className="h-3 w-3 fill-current" />
                      <span>恢复</span>
                    </>
                  ) : (
                    <>
                      <Pause className="h-3 w-3 fill-current" />
                      <span>暂停</span>
                    </>
                  )}
                </Button>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={onStopReprice}
                  loading={loadingAction}
                  className="gap-1.5 text-xs h-8"
                >
                  <Square className="h-3 w-3 fill-current" />
                  <span>停止</span>
                </Button>
              </>
            )}

            {/* Dark Mode Toggle */}
            <Button
              variant="ghost"
              size="icon"
              onClick={onToggleDarkMode}
              className="h-8 w-8 text-muted-foreground hover:text-foreground"
              title={darkMode ? '切换为明亮模式' : '切换为暗色模式'}
            >
              {darkMode ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </Button>
          </div>
        </div>
      </div>
    </header>
  )
}

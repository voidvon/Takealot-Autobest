import type { EngineStatus, Store, LicenseStatus } from '../../types'
import { StoreSwitcher } from './StoreSwitcher'
import { WindowControls } from './WindowControls'
import { WindowsControls } from './WindowsControls'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Play, Pause, Square, Moon, Sun, KeyRound } from 'lucide-react'
import { formatMinutesSeconds } from '../../lib/countdown'

interface HeaderProps {
  version: string
  status: EngineStatus
  licenseStatus?: LicenseStatus | null
  onOpenLicense?: () => void
  onStartReprice: () => void
  onPauseReprice: () => void
  onStopReprice: () => void
  darkMode: boolean
  onToggleDarkMode: () => void
  loadingAction: boolean
  stores?: Store[]
  currentStoreId?: string
  onSelectStore?: (storeId: string) => void
  onOpenAddStore?: () => void
  onStartAllReprice?: () => void
  onStopAllReprice?: () => void
}

export const Header: React.FC<HeaderProps> = ({
  version,
  status,
  licenseStatus,
  onOpenLicense,
  onStartReprice,
  onPauseReprice,
  onStopReprice,
  darkMode,
  onToggleDarkMode,
  loadingAction,
  stores = [],
  currentStoreId = '',
  onSelectStore,
  onOpenAddStore,
  onStartAllReprice,
  onStopAllReprice,
}) => {

  const handleDoubleClick = () => {
    const rt = (window as any).runtime
    if (typeof rt?.WindowToggleMaximise === 'function') {
      rt.WindowToggleMaximise()
    }
  }

  return (
    <header
      className="sticky top-0 z-40 w-full shrink-0 border-b border-border bg-background/95 backdrop-blur-sm select-none"
      style={{ '--wails-draggable': 'drag', WebkitAppRegion: 'drag' } as React.CSSProperties}
      onDoubleClick={handleDoubleClick}
    >
      <div className="flex h-14 items-center justify-between px-4 sm:px-6">
        {/* Brand & Workspace info */}
        <div
          className="flex items-center gap-3 min-w-0"
          style={{ '--wails-draggable': 'no-drag', WebkitAppRegion: 'no-drag' } as React.CSSProperties}
        >
          {/* In-page Window Controls (Traffic Lights) */}
          <WindowControls />

          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary font-black text-primary-foreground text-sm shadow-xs shrink-0">
            T
          </div>
          <div className="flex items-center gap-2 min-w-0">
            <h1 className="font-semibold text-sm text-foreground tracking-tight hidden md:inline truncate">
              Takealot 智能电商自动化中台
            </h1>
            <Badge variant="secondary" className="hidden lg:inline-flex text-[10px] font-mono px-1.5 h-4.5 shrink-0">
              v{version}
            </Badge>

            {/* License Status Badge Button */}
            {licenseStatus && onOpenLicense && (
              <button
                type="button"
                onClick={onOpenLicense}
                className="inline-flex items-center transition-transform active:scale-95 focus:outline-hidden shrink-0"
                title="点击查看授权详情或激活"
              >
                {licenseStatus.activated && !licenseStatus.expired ? (
                  <Badge variant="success" className="text-[10px] px-1.5 h-4.5 cursor-pointer gap-1 font-medium">
                    <KeyRound className="w-2.5 h-2.5" />
                    <span>{licenseStatus.expires_at === 0 ? '永久买断' : `已激活 (${licenseStatus.days_left ?? 0}天)`}</span>
                  </Badge>
                ) : (
                  <Badge variant="destructive" className="text-[10px] px-1.5 h-4.5 cursor-pointer gap-1 font-medium animate-pulse">
                    <KeyRound className="w-2.5 h-2.5" />
                    <span>未激活</span>
                  </Badge>
                )}
              </button>
            )}
          </div>

          {/* Multi-Store Switcher */}
          {stores.length > 0 && onSelectStore && onOpenAddStore && (
            <div className="pl-1 sm:pl-2 sm:border-l sm:border-border/60 shrink-0">
              <StoreSwitcher
                stores={stores}
                currentStoreId={currentStoreId}
                onSelectStore={onSelectStore}
                onOpenAddStore={onOpenAddStore}
                onStartAllReprice={onStartAllReprice}
                onStopAllReprice={onStopAllReprice}
              />
            </div>
          )}
        </div>

        {/* Engine Live Metrics & Controls */}
        <div
          className="flex items-center gap-2 sm:gap-3 shrink-0"
          style={{ '--wails-draggable': 'no-drag', WebkitAppRegion: 'no-drag' } as React.CSSProperties}
        >
          {/* Quick Metrics */}
          <div className="hidden xl:flex items-center gap-4 pr-3 border-r border-border text-xs shrink-0">
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

          {/* Windows-style Window Controls on the right */}
          <WindowsControls />
        </div>
      </div>
    </header>
  )
}

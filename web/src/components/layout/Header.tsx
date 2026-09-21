import type { EngineStatus, Store } from '../../types'
import { StoreSwitcher } from './StoreSwitcher'
import { WindowControls } from './WindowControls'
import { WindowsControls } from './WindowsControls'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Play, Pause, Square, Sparkles } from 'lucide-react'
import { formatMinutesSeconds } from '../../lib/countdown'

interface HeaderProps {
  version: string
  status: EngineStatus
  onStartReprice: () => void
  onPauseReprice: () => void
  onStopReprice: () => void
  loadingAction: boolean
  stores?: Store[]
  currentStoreId?: string
  onSelectStore?: (storeId: string) => void
  onOpenAddStore?: () => void
  onStartAllReprice?: () => void
  onStopAllReprice?: () => void
  updateInfo?: import('../../types').UpdateInfo | null
  onOpenUpdate?: () => void
}

export const Header: React.FC<HeaderProps> = ({
  version,
  status,
  onStartReprice,
  onPauseReprice,
  onStopReprice,
  loadingAction,
  stores = [],
  currentStoreId = '',
  onSelectStore,
  onOpenAddStore,
  onStartAllReprice,
  onStopAllReprice,
  updateInfo,
  onOpenUpdate,
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
              Takealot 掌柜
            </h1>
            <Badge
              variant="secondary"
              onClick={onOpenUpdate}
              className="hidden lg:inline-flex text-[10px] font-mono px-1.5 h-4.5 shrink-0 cursor-pointer hover:bg-muted/80 transition-colors"
              title="点击查看更新"
            >
              v{version}
            </Badge>
            {updateInfo?.has_update && (
              <button
                type="button"
                onClick={onOpenUpdate}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/25 transition-all cursor-pointer animate-pulse shrink-0"
                title="发现新版本，点击查看并更新"
              >
                <Sparkles className="h-3 w-3" />
                <span>可更新 v{updateInfo.version}</span>
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
          </div>

          {/* Windows-style Window Controls on the right */}
          <WindowsControls />
        </div>
      </div>
    </header>
  )
}

import React from 'react'
import type { Store } from '../../types'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuGroup,
} from '../ui/dropdown-menu'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Store as StoreIcon, ChevronsUpDown, Check, Plus, Play, Square, Globe } from 'lucide-react'

interface StoreSwitcherProps {
  stores: Store[]
  currentStoreId: string
  onSelectStore: (storeId: string) => void
  onOpenAddStore: () => void
  onStartAllReprice?: () => void
  onStopAllReprice?: () => void
}

export const StoreSwitcher: React.FC<StoreSwitcherProps> = ({
  stores,
  currentStoreId,
  onSelectStore,
  onOpenAddStore,
  onStartAllReprice,
  onStopAllReprice,
}) => {
  const isAll = currentStoreId === 'all'
  const currentStore = stores.find((s) => s.id === currentStoreId) || (isAll ? null : stores[0])
  const anyRunning = stores.some((s) => s.status?.is_running)
  const runningCount = stores.filter((s) => s.status?.is_running).length

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          role="combobox"
          aria-label="选择店铺"
          className="h-8 gap-2 px-2.5 max-w-[200px] sm:max-w-[260px] text-xs font-normal border-border/80 hover:bg-muted/60"
        >
          <div className="flex items-center gap-1.5 min-w-0 truncate">
            {isAll ? (
              <Globe className="h-3.5 w-3.5 shrink-0 text-primary" />
            ) : (
              <StoreIcon className="h-3.5 w-3.5 shrink-0 text-primary" />
            )}
            <span className="truncate font-medium text-foreground">
              {isAll ? `全部店铺 (${stores.length})` : (currentStore ? currentStore.name : '选择店铺')}
            </span>
          </div>
          {(isAll ? anyRunning : !!currentStore) && (
            <span className="flex h-2 w-2 shrink-0 rounded-full bg-emerald-500 ring-2 ring-emerald-500/20" />
          )}
          <ChevronsUpDown className="ml-auto h-3.5 w-3.5 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent className="w-68" align="start">
        {/* 全店铺选项 */}
        <DropdownMenuItem
          onSelect={() => onSelectStore('all')}
          className={`flex items-center justify-between py-2 cursor-pointer ${
            isAll ? 'bg-primary/10 text-primary font-semibold' : ''
          }`}
        >
          <div className="flex items-center gap-2 min-w-0 truncate">
            <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground text-xs font-bold">
              <Globe className="h-3.5 w-3.5" />
            </div>
            <div className="flex flex-col min-w-0">
              <span className="text-xs font-medium truncate">
                全部店铺 (All Stores)
              </span>
              <span className="text-[10px] text-muted-foreground truncate font-mono">
                汇总聚合 • 共 {stores.length} 个店铺
              </span>
            </div>
          </div>
          <div className="flex items-center gap-1.5 shrink-0 ml-2">
            {runningCount > 0 && (
              <Badge variant="success" className="text-[9px] px-1 py-0 h-4">
                {runningCount}店巡检中
              </Badge>
            )}
            {isAll && <Check className="h-3.5 w-3.5 text-primary shrink-0" />}
          </div>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuLabel className="text-[11px] text-muted-foreground font-normal">
          单店铺切换 ({stores.length})
        </DropdownMenuLabel>
        <DropdownMenuGroup className="max-h-60 overflow-y-auto">
          {stores.map((store) => {
            const isSelected = store.id === (currentStore?.id || '')
            const isRunning = store.status?.is_running
            const isPaused = store.status?.is_paused

            return (
              <DropdownMenuItem
                key={store.id}
                onSelect={() => onSelectStore(store.id)}
                className="flex items-center justify-between py-2 cursor-pointer"
              >
                <div className="flex items-center gap-2 min-w-0 truncate">
                  <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-muted text-foreground text-xs font-bold">
                    {store.name.slice(0, 1).toUpperCase()}
                  </div>
                  <div className="flex flex-col min-w-0">
                    <span className="text-xs font-medium truncate text-foreground">
                      {store.name}
                    </span>
                    <span className="text-[10px] text-muted-foreground truncate font-mono">
                      ID: {store.id}
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-1.5 shrink-0 ml-2">
                  {isRunning ? (
                    isPaused ? (
                      <Badge variant="warning" className="text-[9px] px-1 py-0 h-4">
                        暂停
                      </Badge>
                    ) : (
                      <Badge variant="success" className="text-[9px] px-1 py-0 h-4">
                        巡检中
                      </Badge>
                    )
                  ) : null}
                  {isSelected && <Check className="h-3.5 w-3.5 text-primary shrink-0" />}
                </div>
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>

        <DropdownMenuSeparator />

        <DropdownMenuItem onSelect={onOpenAddStore} className="gap-2 text-xs cursor-pointer text-primary">
          <Plus className="h-3.5 w-3.5" />
          <span>添加新店铺...</span>
        </DropdownMenuItem>

        {(onStartAllReprice || onStopAllReprice) && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="text-[10px] text-muted-foreground">
              批量全局操作
            </DropdownMenuLabel>
            {onStartAllReprice && (
              <DropdownMenuItem onSelect={onStartAllReprice} className="gap-2 text-xs cursor-pointer text-emerald-600 dark:text-emerald-400">
                <Play className="h-3 w-3 fill-current" />
                <span>一键启动所有店铺巡检</span>
              </DropdownMenuItem>
            )}
            {onStopAllReprice && (
              <DropdownMenuItem onSelect={onStopAllReprice} className="gap-2 text-xs cursor-pointer text-amber-600 dark:text-amber-400">
                <Square className="h-3 w-3 fill-current" />
                <span>一键停止所有店铺巡检</span>
              </DropdownMenuItem>
            )}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

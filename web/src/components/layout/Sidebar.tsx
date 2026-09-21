import React from 'react'
import { cn } from '../../lib/utils'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import type { AccountStatus } from '../../types'
import {
  LayoutDashboard,
  TrendingUp,
  PackageCheck,
  ShoppingBag,
  Layers,
  Settings,
  Crown,
  User,
  Sun,
  Moon,
} from 'lucide-react'

export type TabId = 'dashboard' | 'repricer' | 'catalog' | 'sales' | 'follow' | 'settings'

interface SidebarProps {
  activeTab: TabId
  onSelectTab: (tab: TabId) => void
  repricingAlertCount?: number
  accountStatus?: AccountStatus | null
  onOpenAccount?: () => void
  darkMode?: boolean
  onToggleDarkMode?: () => void
}

export const Sidebar: React.FC<SidebarProps> = ({
  activeTab,
  onSelectTab,
  repricingAlertCount = 0,
  accountStatus,
  onOpenAccount,
  darkMode,
  onToggleDarkMode,
}) => {
  const isAuthenticated = Boolean(accountStatus?.authenticated)
  const displayName = accountStatus?.user?.display_name || accountStatus?.user?.username || '用户'
  const avatarChar = displayName ? displayName.slice(0, 1).toUpperCase() : 'U'

  const navItems: {
    id: TabId
    label: string
    icon: React.ReactNode
    badge?: number | string
    badgeVariant?: 'default' | 'secondary' | 'destructive' | 'outline' | 'success' | 'warning'
  }[] = [
    {
      id: 'dashboard',
      label: '仪表盘',
      icon: <LayoutDashboard className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'repricer',
      label: '自动竞价',
      icon: <TrendingUp className="h-4 w-4 shrink-0" />,
      badge: repricingAlertCount > 0 ? repricingAlertCount : undefined,
      badgeVariant: 'destructive',
    },
    {
      id: 'catalog',
      label: '官方商品与库存',
      icon: <PackageCheck className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'sales',
      label: '订单管理',
      icon: <ShoppingBag className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'follow',
      label: '批量跟卖与队列',
      icon: <Layers className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'settings',
      label: '店铺设置与授权',
      icon: <Settings className="h-4 w-4 shrink-0" />,
    },
  ]

  return (
    <aside className="w-full md:w-60 shrink-0 border-r border-border bg-card/95 backdrop-blur-sm flex flex-col justify-between p-3 h-full overflow-y-auto">
      <div className="space-y-4">
        {/* Navigation Group */}
        <div className="space-y-1">
          <div className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/80">
            控制中心 (Nova)
          </div>
          <nav className="space-y-0.5">
            {navItems.map((item) => {
              const isActive = activeTab === item.id
              return (
                <a
                  key={item.id}
                  href={`#/${item.id}`}
                  onClick={(e) => {
                    // 若按住 Cmd/Ctrl 键点击则允许在新标签页打开
                    if (!e.metaKey && !e.ctrlKey) {
                      e.preventDefault()
                      onSelectTab(item.id)
                    }
                  }}
                  className={cn(
                    "w-full flex items-center justify-between gap-2.5 rounded-md px-2.5 py-2 text-xs font-medium transition-colors text-left select-none cursor-pointer no-underline",
                    isActive
                      ? "bg-accent text-accent-foreground font-semibold shadow-xs"
                      : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                  )}
                >
                  <div className="flex items-center gap-2.5 min-w-0 truncate">
                    {item.icon}
                    <span className="truncate">{item.label}</span>
                  </div>
                  {item.badge !== undefined && (
                    <Badge
                      variant={item.badgeVariant || (isActive ? 'default' : 'secondary')}
                      className="h-4.5 px-1.5 text-[10px] font-mono leading-none"
                    >
                      {item.badge}
                    </Badge>
                  )}
                </a>
              )
            })}
          </nav>
        </div>
      </div>

      {/* Footer User Profile & Dark Mode Block */}
      <div className="pt-3 mt-auto border-t border-border shrink-0">
        <div
          onClick={onOpenAccount}
          className={cn(
            "group rounded-lg border border-border/80 bg-muted/30 hover:bg-muted/60 p-2 flex items-center justify-between gap-1.5 transition-colors select-none",
            onOpenAccount && "cursor-pointer"
          )}
          title={isAuthenticated ? "点击查看/管理会员账号" : "点击登录账号"}
        >
          {/* Left part: Avatar + Username + VIP Badge */}
          <div className="flex items-center gap-2 min-w-0 flex-1">
            {isAuthenticated ? (
              <>
                <div className="h-7 w-7 rounded-full bg-primary/15 text-primary border border-primary/25 flex items-center justify-center font-bold text-xs shrink-0 shadow-xs">
                  {avatarChar}
                </div>
                <div className="flex items-center gap-1.5 min-w-0 flex-1">
                  <span
                    className="text-xs font-semibold text-foreground truncate min-w-0"
                    title={displayName}
                  >
                    {displayName}
                  </span>
                  {accountStatus?.eligible ? (
                    <Badge variant="success" className="text-[10px] px-1.5 h-4.5 shrink-0 gap-1 font-medium">
                      <Crown className="w-2.5 h-2.5" />
                      <span>{accountStatus.expires_at ? 'VIP 会员' : '长期 VIP'}</span>
                    </Badge>
                  ) : (
                    <Badge variant="destructive" className="text-[10px] px-1.5 h-4.5 shrink-0 gap-1 font-medium animate-pulse">
                      <Crown className="w-2.5 h-2.5" />
                      <span>开通 VIP</span>
                    </Badge>
                  )}
                </div>
              </>
            ) : (
              <>
                <div className="h-7 w-7 rounded-full bg-muted text-muted-foreground border border-border flex items-center justify-center shrink-0">
                  <User className="h-3.5 w-3.5" />
                </div>
                <div className="flex items-center gap-1.5 min-w-0 flex-1">
                  <span className="text-xs font-semibold text-muted-foreground truncate min-w-0">
                    未登录
                  </span>
                  <Badge variant="secondary" className="text-[10px] px-1.5 h-4.5 shrink-0 font-medium">
                    登录 / 会员
                  </Badge>
                </div>
              </>
            )}
          </div>

          {/* Right part: Dark Mode Toggle */}
          {onToggleDarkMode && (
            <Button
              variant="ghost"
              size="icon"
              onClick={(e) => {
                e.stopPropagation()
                onToggleDarkMode()
              }}
              className="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground rounded-md transition-colors"
              title={darkMode ? '切换为明亮模式' : '切换为暗色模式'}
            >
              {darkMode ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
            </Button>
          )}
        </div>
      </div>
    </aside>
  )
}

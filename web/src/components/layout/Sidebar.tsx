import React from 'react'
import { cn } from '../../lib/utils'
import { Badge } from '../ui/badge'
import {
  LayoutDashboard,
  TrendingUp,
  PackageCheck,
  ShoppingBag,
  Layers,
  Terminal,
  Settings,
  ShieldCheck,
} from 'lucide-react'

export type TabId = 'dashboard' | 'repricer' | 'catalog' | 'sales' | 'follow' | 'logs' | 'settings'

interface SidebarProps {
  activeTab: TabId
  onSelectTab: (tab: TabId) => void
  unreadLogsCount?: number
  repricingAlertCount?: number
}

export const Sidebar: React.FC<SidebarProps> = ({
  activeTab,
  onSelectTab,
  unreadLogsCount = 0,
  repricingAlertCount = 0,
}) => {
  const navItems: {
    id: TabId
    label: string
    icon: React.ReactNode
    badge?: number | string
    badgeVariant?: 'default' | 'secondary' | 'destructive' | 'outline' | 'success' | 'warning'
  }[] = [
    {
      id: 'dashboard',
      label: '概览仪表盘',
      icon: <LayoutDashboard className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'repricer',
      label: '智能改价监控',
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
      label: '销售订单与明细',
      icon: <ShoppingBag className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'follow',
      label: '批量跟卖与队列',
      icon: <Layers className="h-4 w-4 shrink-0" />,
    },
    {
      id: 'logs',
      label: '实时运行日志',
      icon: <Terminal className="h-4 w-4 shrink-0" />,
      badge: unreadLogsCount > 0 ? unreadLogsCount : undefined,
      badgeVariant: 'secondary',
    },
    {
      id: 'settings',
      label: '店铺设置与授权',
      icon: <Settings className="h-4 w-4 shrink-0" />,
    },
  ]

  return (
    <aside className="w-full md:w-60 shrink-0 border-r border-border bg-card/95 backdrop-blur-sm flex flex-col justify-between p-3 h-full md:h-[calc(100vh-3.5rem)] max-h-[calc(100vh-3.5rem)] md:sticky md:top-14 self-start overflow-y-auto">
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

      {/* Footer System Status Block */}
      <div className="pt-3 mt-auto border-t border-border shrink-0">
        <div className="rounded-lg border border-border bg-muted/30 p-2.5 space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="font-semibold text-foreground flex items-center gap-1.5">
              <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
              <span>官方 API 运行</span>
            </span>
            <Badge variant="success" dot className="text-[10px] py-0 px-1.5 h-4">
              在线
            </Badge>
          </div>
          <p className="text-[10px] text-muted-foreground leading-snug">
            Seller Key 鉴权 · SQLite 事务持久化
          </p>
        </div>
      </div>
    </aside>
  )
}

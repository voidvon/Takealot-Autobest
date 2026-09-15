import React, { useEffect, useState } from 'react'
import { api } from '../../api/client'
import type { SalesSummaryItem, StockCounts, StockHealthStats, OfferViewModel } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { formatCurrency } from '../../lib/utils'
import {
  TrendingUp,
  DollarSign,
  Package,
  AlertTriangle,
  Trophy,
  Users,
  RefreshCw,
  ArrowUpRight,
  Warehouse,
  ShieldCheck,
  Clock,
  Coins,
} from 'lucide-react'

interface DashboardTabProps {
  onNavigate: (tab: any) => void
  onStartReprice: () => void
  isRunning: boolean
}

export const DashboardTab: React.FC<DashboardTabProps> = ({
  onNavigate,
  onStartReprice,
  isRunning,
}) => {
  const [loading, setLoading] = useState(true)
  const [salesSummary, setSalesSummary] = useState<SalesSummaryItem[]>([])
  const [totalOffers, setTotalOffers] = useState<number>(0)
  const [stockCounts, setStockCounts] = useState<StockCounts | null>(null)
  const [stockHealth, setStockHealth] = useState<StockHealthStats | null>(null)
  const [offers, setOffers] = useState<OfferViewModel[]>([])

  const loadData = async () => {
    setLoading(true)
    try {
      const [summaryRes, countRes, stockRes, healthRes, offersRes] = await Promise.allSettled([
        api.getOfficialSalesSummary(),
        api.getOfficialOffersCount(),
        api.getOfficialStockCounts(),
        api.getOfficialStockHealth(),
        api.getOffers(),
      ])

      if (summaryRes.status === 'fulfilled') setSalesSummary(summaryRes.value)
      if (countRes.status === 'fulfilled') setTotalOffers(countRes.value.count)
      if (stockRes.status === 'fulfilled') setStockCounts(stockRes.value)
      if (healthRes.status === 'fulfilled') setStockHealth(healthRes.value)
      if (offersRes.status === 'fulfilled') setOffers(offersRes.value.offers || [])
    } catch (e) {
      console.error('Dashboard load failed:', e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  // Calculate battleground stats
  const winningCount = offers.filter((o) => o.priority_status === 'winning').length
  const losingCount = offers.filter((o) => o.priority_status === 'losing').length
  const soloCount = offers.filter((o) => o.priority_status === 'solo').length
  const monitoredCount = offers.filter((o) => o.selected).length

  const todaySummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('today')) || {
    total: 0,
    quantity: 0,
  }
  const yesterdaySummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('yesterday')) || {
    total: 0,
    quantity: 0,
  }
  const sevenDaysSummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('7')) || {
    total: 0,
    quantity: 0,
  }
  const thirtyDaysSummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('30')) || {
    total: 0,
    quantity: 0,
  }
  const ninetyDaysSummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('90')) || {
    total: 0,
    quantity: 0,
  }
  const sixMonthsSummary = salesSummary.find((s) => s.date_range.toLowerCase().includes('6 month')) || {
    total: 0,
    quantity: 0,
  }

  return (
    <div className="space-y-6">
      {/* Top Banner & Refresh */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
            控制中心概览
          </h2>
          <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
            Takealot 官方实时销售数据、多仓库存健康度与智能调价战况
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Button
            variant="outline"
            size="sm"
            onClick={loadData}
            loading={loading}
            className="gap-1.5 text-xs"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            <span>刷新数据</span>
          </Button>
          {!isRunning ? (
            <Button
              variant="default"
              size="sm"
              onClick={onStartReprice}
              className="gap-1.5 text-xs shadow-sm"
            >
              <Trophy className="h-3.5 w-3.5" />
              <span>启动自动抢占</span>
            </Button>
          ) : (
            <Badge variant="success" dot className="py-1 px-3">
              巡检中
            </Badge>
          )}
        </div>
      </div>

      {/* Official Sales Cards - Primary Row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Today */}
        <Card className="hover:border-primary/40 transition-colors">
          <CardHeader className="p-4 pb-1">
            <div className="flex items-center justify-between text-xs text-muted-foreground font-medium">
              <span>今日销售额 (Today)</span>
              <DollarSign className="h-4 w-4 text-emerald-500" />
            </div>
          </CardHeader>
          <CardContent className="p-4 pt-1">
            <div className="text-xl sm:text-2xl font-bold text-foreground font-mono">
              {formatCurrency(todaySummary.total)}
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground mt-2 pt-2 border-t border-border">
              <span>已售销量</span>
              <span className="font-semibold text-foreground font-mono">
                {todaySummary.quantity} 件
              </span>
            </div>
          </CardContent>
        </Card>

        {/* Yesterday */}
        <Card className="hover:border-primary/40 transition-colors">
          <CardHeader className="p-4 pb-1">
            <div className="flex items-center justify-between text-xs text-muted-foreground font-medium">
              <span>昨日销售额 (Yesterday)</span>
              <TrendingUp className="h-4 w-4 text-blue-500" />
            </div>
          </CardHeader>
          <CardContent className="p-4 pt-1">
            <div className="text-xl sm:text-2xl font-bold text-foreground font-mono">
              {formatCurrency(yesterdaySummary.total)}
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground mt-2 pt-2 border-t border-border">
              <span>已售销量</span>
              <span className="font-semibold text-foreground font-mono">
                {yesterdaySummary.quantity} 件
              </span>
            </div>
          </CardContent>
        </Card>

        {/* Last 7 Days */}
        <Card className="hover:border-primary/40 transition-colors">
          <CardHeader className="p-4 pb-1">
            <div className="flex items-center justify-between text-xs text-muted-foreground font-medium">
              <span>近 7 天销售额 (7 Days)</span>
              <TrendingUp className="h-4 w-4 text-primary" />
            </div>
          </CardHeader>
          <CardContent className="p-4 pt-1">
            <div className="text-xl sm:text-2xl font-bold text-foreground font-mono">
              {formatCurrency(sevenDaysSummary.total)}
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground mt-2 pt-2 border-t border-border">
              <span>已售销量</span>
              <span className="font-semibold text-foreground font-mono">
                {sevenDaysSummary.quantity} 件
              </span>
            </div>
          </CardContent>
        </Card>

        {/* Last 30 Days */}
        <Card className="hover:border-primary/40 transition-colors">
          <CardHeader className="p-4 pb-1">
            <div className="flex items-center justify-between text-xs text-muted-foreground font-medium">
              <span>近 30 天销售额 (30 Days)</span>
              <Package className="h-4 w-4 text-purple-500" />
            </div>
          </CardHeader>
          <CardContent className="p-4 pt-1">
            <div className="text-xl sm:text-2xl font-bold text-foreground font-mono">
              {formatCurrency(thirtyDaysSummary.total)}
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground mt-2 pt-2 border-t border-border">
              <span>已售销量</span>
              <span className="font-semibold text-foreground font-mono">
                {thirtyDaysSummary.quantity} 件
              </span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Extended Period Sales & Inventory Overview */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-5">
        {/* BuyBox Status Radar */}
        <Card className="lg:col-span-2">
          <CardHeader className="p-5 pb-3">
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-base font-semibold">
                  智能抢占与购物车战况
                </CardTitle>
                <CardDescription className="text-xs">
                  当前店铺 {totalOffers} 个商品在 Takealot 买家端的竞争占优分布
                </CardDescription>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate('repricer')}
                className="gap-1 text-xs text-primary"
              >
                <span>进入改价策略</span>
                <ArrowUpRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          </CardHeader>
          <CardContent className="p-5 pt-2">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <Card className="p-3.5 border-border hover:border-emerald-500/50 transition-colors">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-foreground">胜出购物车</span>
                  <Badge variant="success" dot className="font-mono text-[10px]">
                    Winning
                  </Badge>
                </div>
                <div className="text-2xl font-bold font-mono text-emerald-600 dark:text-emerald-400 mt-2">
                  {winningCount}
                </div>
                <div className="text-[11px] text-muted-foreground mt-1">
                  当前处于最优低价，独占买家流量
                </div>
              </Card>

              <Card className="p-3.5 border-border hover:border-destructive/50 transition-colors">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-foreground">价格落后</span>
                  <Badge variant="destructive" dot className="font-mono text-[10px]">
                    In Battle
                  </Badge>
                </div>
                <div className="text-2xl font-bold font-mono text-rose-600 dark:text-rose-400 mt-2">
                  {losingCount}
                </div>
                <div className="text-[11px] text-muted-foreground mt-1">
                  对手价格更低，待系统降价击穿
                </div>
              </Card>

              <Card className="p-3.5 border-border hover:border-primary/50 transition-colors">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-foreground">独家销售</span>
                  <Badge variant="secondary" dot className="font-mono text-[10px]">
                    Solo
                  </Badge>
                </div>
                <div className="text-2xl font-bold font-mono text-foreground mt-2">
                  {soloCount}
                </div>
                <div className="text-[11px] text-muted-foreground mt-1">
                  暂无竞争对手跟卖，利润空间充裕
                </div>
              </Card>
            </div>

            {/* Extended sales summary: 90 Days and 6 Months */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-4 pt-4 border-t border-border">
              <Card className="p-3 flex items-center justify-between bg-muted/20 border-border">
                <div className="space-y-0.5">
                  <div className="text-xs text-muted-foreground">近 90 天历史销售</div>
                  <div className="text-sm font-bold font-mono text-foreground">
                    {formatCurrency(ninetyDaysSummary.total)}
                  </div>
                </div>
                <Badge variant="secondary" className="font-mono text-xs">
                  {ninetyDaysSummary.quantity} 件
                </Badge>
              </Card>

              <Card className="p-3 flex items-center justify-between bg-muted/20 border-border">
                <div className="space-y-0.5">
                  <div className="text-xs text-muted-foreground">近 6 个月累计总销</div>
                  <div className="text-sm font-bold font-mono text-foreground">
                    {formatCurrency(sixMonthsSummary.total)}
                  </div>
                </div>
                <Badge variant="secondary" className="font-mono text-xs">
                  {sixMonthsSummary.quantity} 件
                </Badge>
              </Card>
            </div>
          </CardContent>
        </Card>

        {/* Stock Health & Storage Fee Risk Card */}
        <Card>
          <CardHeader className="p-5 pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-base font-semibold">
                库存健康与仓储费预警
              </CardTitle>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate('catalog')}
                className="gap-1 text-xs text-primary"
              >
                <span>查库存</span>
                <ArrowUpRight className="h-3.5 w-3.5" />
              </Button>
            </div>
            <CardDescription className="text-xs">
              Takealot 仓库超期滞销与分仓均衡状态
            </CardDescription>
          </CardHeader>
          <CardContent className="p-5 pt-2 space-y-3">
            {/* Storage fee warning */}
            <Card className="p-3.5 border-border bg-muted/20">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 text-xs font-semibold text-foreground">
                  <Coins className="h-4 w-4 text-amber-500 shrink-0" />
                  <span>计费滞销商品数</span>
                </div>
                <Badge variant="warning" className="font-mono font-bold">
                  {stockHealth?.storage_fee_enabled_offer_count ?? 0} 款
                </Badge>
              </div>
              <p className="text-muted-foreground text-[11px] leading-relaxed mt-1.5">
                超过免租期已开始按件收取超期仓储费，建议适度降价清仓。
              </p>
            </Card>

            {/* Unbalanced stock warning */}
            <Card className="p-3.5 border-border bg-muted/20">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 text-xs font-semibold text-foreground">
                  <Warehouse className="h-4 w-4 text-primary shrink-0" />
                  <span>多仓不均衡商品数</span>
                </div>
                <Badge variant="secondary" className="font-mono font-bold">
                  {stockCounts?.unbalanced_stock_count ?? 0} 款
                </Badge>
              </div>
              <p className="text-muted-foreground text-[11px] leading-relaxed mt-1.5">
                CPT / JHB / DBN 仓库分布不均衡可能导致部分区域派送时效延误。
              </p>
            </Card>

            {/* Total Stock in Takealot */}
            <Card className="p-3.5 border-border bg-muted/20">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 text-xs font-semibold text-foreground">
                  <ShieldCheck className="h-4 w-4 text-emerald-500 shrink-0" />
                  <span>全仓在库库存款数</span>
                </div>
                <Badge variant="success" className="font-mono font-bold">
                  {stockCounts?.total_stock_count ?? 0} 款
                </Badge>
              </div>
              <p className="text-muted-foreground text-[11px] mt-1.5">
                Takealot 官方仓库现存有效库位的 Offer 统计
              </p>
            </Card>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

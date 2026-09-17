import React, { useState, useEffect, useMemo } from 'react'
import { api } from '../../api/client'
import type { OfferViewModel, PriorityStatus, TargetConfig, RepriceHistoryRecord } from '../../types'
import { Card, CardHeader, CardTitle, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Dialog } from '../ui/dialog'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { ImagePreviewModal } from '../ui/ImagePreviewModal'
import { formatCurrency, cn } from '../../lib/utils'
import {
  Search,
  Filter,
  Save,
  CheckCircle2,
  AlertCircle,
  ExternalLink,
  Edit3,
  RefreshCw,
  ImageIcon,
  ShieldAlert,
  History,
  CloudDownload,
  PackageCheck,
  Calculator,
} from 'lucide-react'

export const RepricerTab: React.FC = () => {
  const [activeSubTab, setActiveSubTab] = useState<'status' | 'logs'>('status')
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [offers, setOffers] = useState<OfferViewModel[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | PriorityStatus | 'selected'>('all')
  const [savingTargets, setSavingTargets] = useState(false)
  const [hasChanges, setHasChanges] = useState(false)

  // Running logs state
  const [historyRecords, setHistoryRecords] = useState<RepriceHistoryRecord[]>([])
  const [logsLoading, setLogsLoading] = useState(false)

  // Image preview modal state
  const [previewImage, setPreviewImage] = useState<{
    open: boolean
    imageUrl?: string
    imageLargeUrl?: string
    title?: string
    tsin?: string
    plid?: string
  }>({ open: false })

  // Quick price edit modal state
  const [editOffer, setEditOffer] = useState<{
    open: boolean
    offer?: OfferViewModel
    newPrice?: number
    newRrp?: number
    saving?: boolean
  }>({ open: false })

  const loadRepriceLogs = async () => {
    setLogsLoading(true)
    try {
      const list = await api.getRepriceHistory(200)
      setHistoryRecords(list || [])
    } catch (err: any) {
      console.error('获取调价运行日志失败:', err)
    } finally {
      setLogsLoading(false)
    }
  }

  const loadOffers = async (sync = false) => {
    if (sync) setSyncing(true)
    else setLoading(true)
    try {
      const res = await api.getOffers(sync)
      if (res.success && res.offers) {
        setOffers(res.offers)
        setHasChanges(false)
      }
    } catch (err: any) {
      alert(`获取调价商品列表失败: ${err.message}`)
    } finally {
      setLoading(false)
      setSyncing(false)
    }
  }

  useEffect(() => {
    loadOffers(false)
    loadRepriceLogs()
  }, [])

  // Filter & Search for Offers
  const filteredOffers = useMemo(() => {
    return offers.filter((item) => {
      // Search
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase()
        const matchesTitle = item.title?.toLowerCase().includes(q)
        const matchesTSIN = item.tsin_id?.toLowerCase().includes(q)
        const matchesPLID = item.plid?.toLowerCase().includes(q)
        const matchesSKU = item.sku?.toLowerCase().includes(q)
        if (!matchesTitle && !matchesTSIN && !matchesPLID && !matchesSKU) return false
      }
      // Status Filter
      if (statusFilter === 'selected') {
        if (!item.selected) return false
      } else if (statusFilter !== 'all') {
        if (item.priority_status !== statusFilter) return false
      }
      return true
    })
  }, [offers, searchQuery, statusFilter])

  // Filter & Search for Running Logs
  const filteredLogs = useMemo(() => {
    if (!searchQuery.trim()) return historyRecords
    const q = searchQuery.toLowerCase()
    return historyRecords.filter((r) =>
      (r.title && r.title.toLowerCase().includes(q)) ||
      (r.sku && r.sku.toLowerCase().includes(q)) ||
      (r.tsin_id && r.tsin_id.toLowerCase().includes(q)) ||
      (r.store_name && r.store_name.toLowerCase().includes(q)) ||
      (r.action && r.action.toLowerCase().includes(q)) ||
      (r.reason && r.reason.toLowerCase().includes(q))
    )
  }, [historyRecords, searchQuery])

  // Handlers for modifying targets
  const handleToggleSelect = (key: string) => {
    setOffers((prev) =>
      prev.map((o) => (o.key === key ? { ...o, selected: !o.selected } : o))
    )
    setHasChanges(true)
  }

  const handleMinPriceChange = (key: string, val: number) => {
    setOffers((prev) =>
      prev.map((o) => (o.key === key ? { ...o, min_price: val } : o))
    )
    setHasChanges(true)
  }

  const handleMaxPriceChange = (key: string, val: number) => {
    setOffers((prev) =>
      prev.map((o) => (o.key === key ? { ...o, max_price: val } : o))
    )
    setHasChanges(true)
  }

  const handleSelectAllFiltered = (selected: boolean) => {
    const keys = new Set(filteredOffers.map((o) => o.key))
    setOffers((prev) =>
      prev.map((o) => (keys.has(o.key) ? { ...o, selected } : o))
    )
    setHasChanges(true)
  }

  const handleSaveAllTargets = async () => {
    setSavingTargets(true)
    try {
      const targetsMap: Record<string, TargetConfig> = {}
      offers.forEach((o) => {
        targetsMap[o.key] = {
          selected: o.selected,
          min_price: o.min_price || 0,
          max_price: o.max_price || 0,
          store_id: o.store_id,
        }
      })
      const res = await api.saveTargets(targetsMap)
      if (res.success) {
        setHasChanges(false)
        alert('🎉 监控配置已成功保存并立即生效！')
      }
    } catch (err: any) {
      alert(`保存失败: ${err.message}`)
    } finally {
      setSavingTargets(false)
    }
  }

  // Handle manual price edit submit
  const handleEditPriceSubmit = async () => {
    if (!editOffer.offer || !editOffer.newPrice) return
    setEditOffer((prev) => ({ ...prev, saving: true }))
    try {
      // Find offer id if possible or use TSIN/API
      const payload: any = { selling_price: editOffer.newPrice }
      if (editOffer.newRrp) payload.rrp = editOffer.newRrp

      // Call API with specific store_id
      const offerId = editOffer.offer.key.split('/')[0] // or use official offer update
      await api.updateOfficialOffer(offerId, payload, editOffer.offer.store_id)
      alert('商品售价更新成功！')
      setEditOffer({ open: false })
      loadOffers()
    } catch (err: any) {
      alert(`修改价格失败: ${err.message}`)
    } finally {
      setEditOffer((prev) => ({ ...prev, saving: false }))
    }
  }

  // Summary counts
  const totalMonitored = offers.filter((o) => o.selected).length
  const winningCount = offers.filter((o) => o.priority_status === 'winning').length
  const losingCount = offers.filter((o) => o.priority_status === 'losing').length
  const soloCount = offers.filter((o) => o.priority_status === 'solo').length

  // Batch min-price modal state
  const [batchMinPriceModalOpen, setBatchMinPriceModalOpen] = useState(false)
  const [batchPriceBase, setBatchPriceBase] = useState<'selling_price' | 'rrp'>('rrp')
  const [batchRatio, setBatchRatio] = useState<number>(2)
  const [batchScope, setBatchScope] = useState<'all' | 'selected'>('all')

  const targetOffers = useMemo(() => {
    if (batchScope === 'selected' && totalMonitored > 0) {
      return offers.filter((o) => o.selected)
    }
    return offers
  }, [offers, batchScope, totalMonitored])

  const targetEmptyCount = useMemo(() => {
    return targetOffers.filter((o) => !o.min_price || o.min_price <= 0).length
  }, [targetOffers])

  const targetAllCount = targetOffers.length

  const handleApplyBatchMinPrice = (mode: 'empty_only' | 'overwrite_all') => {
    if (batchRatio <= 0) {
      alert('请输入大于 0 的有效折算倍数！')
      return
    }

    let updatedCount = 0
    const updated = offers.map((o) => {
      if (batchScope === 'selected' && totalMonitored > 0 && !o.selected) {
        return o
      }

      const hasExistingMin = o.min_price && o.min_price > 0
      if (mode === 'empty_only' && hasExistingMin) {
        return o
      }

      const base = batchPriceBase === 'rrp' ? (o.rrp || o.selling_price) : o.selling_price
      if (!base || base <= 0) {
        return o
      }

      const calculatedMin = Math.max(1, Math.round(base / batchRatio))
      updatedCount++
      return {
        ...o,
        min_price: calculatedMin,
      }
    })

    if (updatedCount === 0) {
      alert(mode === 'empty_only' ? '所选范围内没有空白底价的商品，无需填充。' : '所选范围内没有可更新底价的有效商品。')
      return
    }

    setOffers(updated)
    setHasChanges(true)
    setBatchMinPriceModalOpen(false)
    alert(`🎉 已成功为 ${updatedCount} 款商品计算并更新最低保护底价！\n💡 提示：底价已在本地更新，请点击顶部的【保存已修改的配置】提交生效。`)
  }

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Top Toolbar */}
      <div className="flex flex-wrap items-center justify-start gap-2.5">
        <Button variant="outline" size="sm" onClick={() => loadOffers(false)} loading={loading} className="gap-1.5 text-xs">
          <RefreshCw className="h-3.5 w-3.5" />
          <span>刷新列表 (毫秒级)</span>
        </Button>
        <Button variant="outline" size="sm" onClick={() => loadOffers(true)} loading={syncing} className="gap-1.5 text-xs border-primary/40 text-primary hover:bg-primary/5">
          <CloudDownload className="h-3.5 w-3.5" />
          <span>从店铺全量同步商品</span>
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            if (totalMonitored > 0) setBatchScope('selected')
            else setBatchScope('all')
            setBatchMinPriceModalOpen(true)
          }}
          className="gap-1.5 text-xs border-blue-500/30 text-blue-600 dark:text-blue-400 hover:bg-blue-500/10"
        >
          <Calculator className="h-3.5 w-3.5" />
          <span>最低价设置</span>
        </Button>
        <Button
          variant={hasChanges ? 'default' : 'secondary'}
          size="sm"
          onClick={handleSaveAllTargets}
          loading={savingTargets}
          disabled={!hasChanges}
          className="gap-1.5 shadow-sm"
        >
          <Save className="h-3.5 w-3.5" />
          <span>{hasChanges ? '保存已修改的配置 *' : '配置已同步'}</span>
        </Button>
      </div>

      {/* SubTabs & Search Bar */}
      <Card>
        <CardContent className="p-3 sm:p-4 flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3">
          {/* Left side: Sub-tabs: SKU状态 & 运行日志 */}
          <div className="flex items-center gap-2">
            <div className="inline-flex items-center p-1 rounded-lg bg-muted border border-border">
              <button
                type="button"
                onClick={() => setActiveSubTab('status')}
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer",
                  activeSubTab === 'status'
                    ? "bg-background text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                <PackageCheck className="h-3.5 w-3.5" />
                <span>SKU状态</span>
                <Badge variant={activeSubTab === 'status' ? "default" : "secondary"} className="ml-1 text-[10px] px-1.5 py-0 h-4">
                  {offers.length}
                </Badge>
              </button>
              <button
                type="button"
                onClick={() => {
                  setActiveSubTab('logs')
                  loadRepriceLogs()
                }}
                className={cn(
                  "flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer",
                  activeSubTab === 'logs'
                    ? "bg-background text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                <History className="h-3.5 w-3.5" />
                <span>运行日志</span>
                <Badge variant={activeSubTab === 'logs' ? "default" : "secondary"} className="ml-1 text-[10px] px-1.5 py-0 h-4">
                  {historyRecords.length}
                </Badge>
              </button>
            </div>
          </div>

          {/* Right side: Search & Refresh */}
          <div className="flex items-center gap-2.5 w-full md:w-auto justify-end">
            <div className="relative w-full md:w-80">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                type="text"
                placeholder={activeSubTab === 'status' ? "搜索商品标题、SKU、TSIN 或 PLID..." : "搜索日志标题、SKU、TSIN 或说明..."}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9 h-9 text-xs"
              />
            </div>
            {activeSubTab === 'logs' && (
              <Button
                variant="outline"
                size="sm"
                onClick={loadRepriceLogs}
                loading={logsLoading}
                className="h-9 px-3 text-xs gap-1.5 shrink-0"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                <span>刷新日志</span>
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {/* SKU Status View */}
      {activeSubTab === 'status' && (
        <>
          {/* Filter & Metric Cards using standard shadcn components */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <Card
              className={`cursor-pointer transition-colors ${
                statusFilter === 'all' ? 'border-primary shadow-xs ring-1 ring-primary/20' : 'hover:border-border/80'
              }`}
              onClick={() => setStatusFilter('all')}
            >
              <CardHeader className="p-3 pb-1">
                <CardTitle className="text-xs font-medium text-muted-foreground flex items-center justify-between">
                  <span>全量商品库</span>
                  <Badge variant="secondary" className="font-mono text-[10px] h-4 px-1">全部</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-3 pt-0">
                <div className="text-xl font-bold font-mono text-foreground">{offers.length}</div>
              </CardContent>
            </Card>

            <Card
              className={`cursor-pointer transition-colors ${
                statusFilter === 'winning'
                  ? 'border-emerald-500 shadow-xs ring-1 ring-emerald-500/20'
                  : 'hover:border-border/80'
              }`}
              onClick={() => setStatusFilter('winning')}
            >
              <CardHeader className="p-3 pb-1">
                <CardTitle className="text-xs font-medium text-emerald-600 dark:text-emerald-400 flex items-center justify-between">
                  <span>处于绝对优先</span>
                  <Badge variant="success" dot className="text-[10px] h-4 px-1.5">Winning</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-3 pt-0">
                <div className="text-xl font-bold font-mono text-emerald-600 dark:text-emerald-400">
                  {winningCount}
                </div>
              </CardContent>
            </Card>

            <Card
              className={`cursor-pointer transition-colors ${
                statusFilter === 'losing'
                  ? 'border-destructive shadow-xs ring-1 ring-destructive/20'
                  : 'hover:border-border/80'
              }`}
              onClick={() => setStatusFilter('losing')}
            >
              <CardHeader className="p-3 pb-1">
                <CardTitle className="text-xs font-medium text-rose-600 dark:text-rose-400 flex items-center justify-between">
                  <span>失去优先价格</span>
                  <Badge variant="destructive" dot className="text-[10px] h-4 px-1.5">Battle</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-3 pt-0">
                <div className="text-xl font-bold font-mono text-rose-600 dark:text-rose-400">
                  {losingCount}
                </div>
              </CardContent>
            </Card>

            <Card
              className={`cursor-pointer transition-colors ${
                statusFilter === 'selected'
                  ? 'border-primary shadow-xs ring-1 ring-primary/20'
                  : 'hover:border-border/80'
              }`}
              onClick={() => setStatusFilter('selected')}
            >
              <CardHeader className="p-3 pb-1">
                <CardTitle className="text-xs font-medium text-muted-foreground flex items-center justify-between">
                  <span>已开启自动改价</span>
                  <Badge variant="default" className="text-[10px] h-4 px-1.5">Active</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-3 pt-0">
                <div className="text-xl font-bold font-mono text-foreground">
                  {totalMonitored}
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Main Repricing Data Table */}
          <Card className="overflow-hidden border-border/80">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader className="bg-muted/30">
              <TableRow>
                <TableHead className="w-12 text-center">
                  <input
                    type="checkbox"
                    checked={filteredOffers.length > 0 && filteredOffers.every((o) => o.selected)}
                    onChange={(e) => handleSelectAllFiltered(e.target.checked)}
                    className="rounded text-primary focus:ring-primary h-4 w-4 cursor-pointer"
                  />
                </TableHead>
                <TableHead className="w-16 text-center">商品大图</TableHead>
                <TableHead className="min-w-[240px]">商品基本信息</TableHead>
                <TableHead className="text-center">当前售价 / RRP</TableHead>
                <TableHead className="text-center">竞品最优价 / 差价</TableHead>
                <TableHead className="text-center min-w-[170px]">防亏底价 / 封顶价</TableHead>
                <TableHead className="text-center">战况态势</TableHead>
                <TableHead className="text-center w-24">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredOffers.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="py-12 text-center text-muted-foreground">
                    {loading ? '正在加载商品比价数据...' : '没有找到符合条件的调价商品'}
                  </TableCell>
                </TableRow>
              ) : (
                filteredOffers.map((item) => {
                  const hasImage = !!item.image_url
                  const isWinning = item.priority_status === 'winning'
                  const isLosing = item.priority_status === 'losing'
                  const takealotUrl = item.plid
                    ? `https://www.takealot.com/x/PLID${item.plid.replace(/^PLID/i, '')}`
                    : ''

                  return (
                    <TableRow
                      key={item.key}
                      className={item.selected ? 'bg-muted/40' : undefined}
                    >
                      {/* Checkbox */}
                      <TableCell className="text-center">
                        <input
                          type="checkbox"
                          checked={item.selected}
                          onChange={() => handleToggleSelect(item.key)}
                          className="rounded text-primary focus:ring-primary h-4 w-4 cursor-pointer"
                        />
                      </TableCell>

                      {/* Image Thumbnail with Hover Zoom & Click Modal */}
                      <TableCell className="text-center">
                        <button
                          onClick={() =>
                            setPreviewImage({
                              open: true,
                              imageUrl: item.image_url,
                              imageLargeUrl: item.image_large_url,
                              title: item.title,
                              tsin: item.tsin_id,
                              plid: item.plid,
                            })
                          }
                          className="h-11 w-11 mx-auto rounded-lg bg-muted border border-border flex items-center justify-center overflow-hidden hover:ring-2 hover:ring-primary/60 transition group cursor-pointer relative"
                          title="点击查看 Takealot 官方超清大图"
                        >
                          {hasImage ? (
                            <img
                              src={item.image_url}
                              alt={item.title}
                              className="h-full w-full object-contain p-0.5 group-hover:scale-110 transition-transform duration-200"
                              loading="lazy"
                            />
                          ) : (
                            <ImageIcon className="h-4 w-4 text-muted-foreground" />
                          )}
                          <span className="absolute bottom-0 right-0 bg-black/60 text-[8px] text-white px-0.5 rounded-tl font-mono">
                            HD
                          </span>
                        </button>
                      </TableCell>

                      {/* Title & TSIN / PLID */}
                      <TableCell>
                        <div className="space-y-1">
                          <div className="flex items-start gap-1.5">
                            {item.store_name && (
                              <Badge variant="outline" className="text-[10px] h-4.5 px-1.5 shrink-0 bg-primary/5 text-primary border-primary/20 font-medium">
                                {item.store_name}
                              </Badge>
                            )}
                            <p className="font-medium text-foreground line-clamp-2 text-xs leading-snug" title={item.title}>
                              {item.title || '未知商品标题'}
                            </p>
                          </div>
                          <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground font-mono">
                            {item.sku && (
                              <span>SKU: <strong className="text-foreground">{item.sku}</strong></span>
                            )}
                            <span>TSIN: <strong className="text-foreground">{item.tsin_id}</strong></span>
                            {item.plid && (
                              <span>PLID: <strong className="text-foreground">{item.plid}</strong></span>
                            )}
                            {takealotUrl && (
                              <a
                                href={takealotUrl}
                                target="_blank"
                                rel="noreferrer"
                                className="inline-flex items-center gap-0.5 text-primary hover:underline"
                              >
                                <span>链接</span>
                                <ExternalLink className="h-2.5 w-2.5" />
                              </a>
                            )}
                          </div>
                        </div>
                      </TableCell>

                      {/* Current Selling Price & RRP */}
                      <TableCell className="text-center">
                        <div className="font-mono font-bold text-foreground text-sm">
                          {formatCurrency(item.selling_price)}
                        </div>
                        <div className="font-mono text-[11px] text-muted-foreground line-through">
                          {item.rrp ? formatCurrency(item.rrp) : '-'}
                        </div>
                        <div className="text-[10px] text-muted-foreground">
                          库存: {item.stock ?? 0}
                        </div>
                      </TableCell>

                      {/* Best Price & Difference */}
                      <TableCell className="text-center">
                        {item.best_price > 0 ? (
                          <div className="space-y-0.5 font-mono">
                            <div className="font-semibold text-foreground text-xs">
                              {formatCurrency(item.best_price)}
                            </div>
                            {isLosing && item.price_diff > 0 && (
                              <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
                                贵 {formatCurrency(item.price_diff)}
                              </Badge>
                            )}
                            {isWinning && (
                              <span className="text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold block">
                                最优领先
                              </span>
                            )}
                            <div className="text-[10px] text-muted-foreground">
                              {item.competing_offers} 位卖家在售
                            </div>
                          </div>
                        ) : (
                          <span className="text-xs text-muted-foreground">无跟卖竞品</span>
                        )}
                      </TableCell>

                      {/* Guard Min / Max Price Inputs */}
                      <TableCell className="text-center">
                        <div className="flex items-center justify-center gap-1.5">
                          <div className="w-20">
                            <Input
                              type="number"
                              min="0"
                              value={item.min_price || ''}
                              placeholder="底价"
                              onChange={(e) =>
                                handleMinPriceChange(item.key, parseInt(e.target.value) || 0)
                              }
                              className="w-full text-center h-8 text-xs font-mono font-semibold"
                              title="最低保护底价（R）：无论竞品降至何价，绝不突破此底价"
                            />
                          </div>
                          <span className="text-muted-foreground text-xs">-</span>
                          <div className="w-20">
                            <Input
                              type="number"
                              min="0"
                              value={item.max_price || ''}
                              placeholder="封顶"
                              onChange={(e) =>
                                handleMaxPriceChange(item.key, parseInt(e.target.value) || 0)
                              }
                              className="w-full text-center h-8 text-xs font-mono"
                              title="最高封顶价（R）：对手提价或独家时回升的最大上限"
                            />
                          </div>
                        </div>
                        {item.min_price > 0 && item.selling_price < item.min_price && (
                          <div className="text-[10px] text-rose-500 font-medium mt-1 flex items-center justify-center gap-1">
                            <ShieldAlert className="h-3 w-3" />
                            <span>现价已跌破保护底价!</span>
                          </div>
                        )}
                      </TableCell>

                      {/* Priority Status Badge */}
                      <TableCell className="text-center">
                        {isWinning && (
                          <Badge variant="success" dot>
                            处于优先
                          </Badge>
                        )}
                        {isLosing && (
                          <Badge variant="destructive" dot>
                            失去优先
                          </Badge>
                        )}
                        {item.priority_status === 'solo' && (
                          <Badge variant="secondary" dot>
                            独家在售
                          </Badge>
                        )}
                      </TableCell>

                      {/* Quick Edit Actions */}
                      <TableCell className="text-center">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            setEditOffer({
                              open: true,
                              offer: item,
                              newPrice: item.selling_price,
                              newRrp: item.rrp,
                            })
                          }
                          className="h-8 px-2 text-xs gap-1"
                        >
                          <Edit3 className="h-3.5 w-3.5" />
                          <span>改价</span>
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      </Card>
    </>
  )}

      {/* Running Logs Table */}
      {activeSubTab === 'logs' && (
        <Card className="overflow-hidden border-border/80">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader className="bg-muted/30">
                <TableRow>
                  <TableHead className="w-36">时间</TableHead>
                  <TableHead className="w-32">SKU</TableHead>
                  <TableHead className="w-16 text-center">主图</TableHead>
                  <TableHead className="min-w-[240px]">标题</TableHead>
                  <TableHead className="w-28 text-center">店铺</TableHead>
                  <TableHead className="w-32 text-center">动作</TableHead>
                  <TableHead className="min-w-[240px]">说明</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logsLoading ? (
                  <TableRow>
                    <TableCell colSpan={7} className="py-12 text-center text-xs text-muted-foreground">
                      正在从本地持久化数据库加载调价运行日志...
                    </TableCell>
                  </TableRow>
                ) : filteredLogs.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="py-12 text-center text-xs text-muted-foreground">
                      {searchQuery.trim()
                        ? '未搜索到匹配的调价运行日志'
                        : '暂无调价运行日志（当自动竞价引擎检测到价格变动并调价成功时将在此显式）'}
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredLogs.map((r) => {
                    const hasImage = !!r.image_url
                    const actionType = r.action || (r.new_price < r.old_price ? '跟降' : r.new_price > r.old_price ? '跟涨' : '改价')
                    const takealotUrl = r.tsin_id ? `https://www.takealot.com/x/TSIN${r.tsin_id}` : ''

                    return (
                      <TableRow key={r.id} className="hover:bg-muted/30 transition-colors">
                        {/* 1. 时间 */}
                        <TableCell className="text-xs font-mono text-muted-foreground whitespace-nowrap">
                          {r.created_at}
                        </TableCell>

                        {/* 2. SKU */}
                        <TableCell className="font-mono text-xs font-semibold text-foreground whitespace-nowrap">
                          {r.sku || (r.tsin_id ? `TSIN:${r.tsin_id}` : '-')}
                        </TableCell>

                        {/* 3. 主图 */}
                        <TableCell className="text-center">
                          <button
                            onClick={() =>
                              setPreviewImage({
                                open: true,
                                imageUrl: r.image_url,
                                title: r.title,
                                tsin: r.tsin_id,
                              })
                            }
                            className="h-10 w-10 mx-auto rounded-lg bg-muted border border-border flex items-center justify-center overflow-hidden hover:ring-2 hover:ring-primary/60 transition group cursor-pointer relative"
                            title="点击放大查看主图"
                          >
                            {hasImage ? (
                              <img
                                src={r.image_url}
                                alt={r.title}
                                className="h-full w-full object-contain p-0.5 group-hover:scale-110 transition-transform duration-200"
                                loading="lazy"
                              />
                            ) : (
                              <ImageIcon className="h-4 w-4 text-muted-foreground" />
                            )}
                          </button>
                        </TableCell>

                        {/* 4. 标题 */}
                        <TableCell>
                          <div className="space-y-1">
                            <p className="font-medium text-foreground line-clamp-2 text-xs leading-snug" title={r.title}>
                              {r.title || r.target_key || '未知商品标题'}
                            </p>
                            <div className="flex items-center gap-2 text-[11px] text-muted-foreground font-mono">
                              {r.tsin_id && <span>TSIN: <strong className="text-foreground">{r.tsin_id}</strong></span>}
                              {takealotUrl && (
                                <a
                                  href={takealotUrl}
                                  target="_blank"
                                  rel="noreferrer"
                                  className="inline-flex items-center gap-0.5 text-primary hover:underline text-[10px]"
                                >
                                  <span>链接</span>
                                  <ExternalLink className="h-2.5 w-2.5" />
                                </a>
                              )}
                            </div>
                          </div>
                        </TableCell>

                        {/* 5. 店铺 */}
                        <TableCell className="text-center whitespace-nowrap">
                          <Badge variant="outline" className="text-[11px] font-normal">
                            {r.store_name || 'Huihengxin'}
                          </Badge>
                        </TableCell>

                        {/* 6. 动作 */}
                        <TableCell className="text-center whitespace-nowrap">
                          <div className="inline-flex flex-col items-center gap-1">
                            {actionType === '跟降' ? (
                              <Badge variant="warning" className="text-[10px] px-1.5 h-4.5">
                                跟降
                              </Badge>
                            ) : actionType === '跟涨' ? (
                              <Badge variant="success" className="text-[10px] px-1.5 h-4.5">
                                跟涨
                              </Badge>
                            ) : actionType === '底价保护' ? (
                              <Badge variant="destructive" className="text-[10px] px-1.5 h-4.5">
                                底价保护
                              </Badge>
                            ) : (
                              <Badge variant="default" className="text-[10px] px-1.5 h-4.5">
                                {actionType}
                              </Badge>
                            )}
                            <div className="text-[11px] font-mono text-muted-foreground">
                              <span className="line-through">R{r.old_price}</span>
                              <span className="text-foreground font-bold ml-1">→ R{r.new_price}</span>
                            </div>
                          </div>
                        </TableCell>

                        {/* 7. 说明 */}
                        <TableCell>
                          <div className="space-y-1">
                            <p className="text-xs text-foreground/90 font-sans leading-relaxed">
                              {r.reason || '自动根据最优竞品价格策略执行调价'}
                            </p>
                            {r.competitor_price > 0 && (
                              <div className="text-[10px] font-mono text-muted-foreground">
                                竞品最优价: <span className="text-primary font-semibold">R{r.competitor_price}</span>
                              </div>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </Card>
      )}

      {/* Image Preview Modal */}
      <ImagePreviewModal
        open={previewImage.open}
        onClose={() => setPreviewImage((prev) => ({ ...prev, open: false }))}
        imageUrl={previewImage.imageUrl}
        imageLargeUrl={previewImage.imageLargeUrl}
        title={previewImage.title}
        tsin={previewImage.tsin}
        plid={previewImage.plid}
      />

      {/* Manual Quick Edit Modal */}
      <Dialog
        open={editOffer.open}
        onClose={() => setEditOffer({ open: false })}
        title="手动修改商品售价"
        description={editOffer.offer?.title}
      >
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              实时售价 Selling Price (R)
            </label>
            <Input
              type="number"
              value={editOffer.newPrice || ''}
              onChange={(e) =>
                setEditOffer((prev) => ({ ...prev, newPrice: parseInt(e.target.value) || 0 }))
              }
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              建议零售价 RRP (R)
            </label>
            <Input
              type="number"
              value={editOffer.newRrp || ''}
              onChange={(e) =>
                setEditOffer((prev) => ({ ...prev, newRrp: parseInt(e.target.value) || 0 }))
              }
            />
          </div>

          <div className="flex justify-end gap-2 pt-3 border-t border-border">
            <Button variant="outline" size="sm" onClick={() => setEditOffer({ open: false })}>
              取消
            </Button>
            <Button
              variant="default"
              size="sm"
              onClick={handleEditPriceSubmit}
              loading={editOffer.saving}
            >
              提交修改
            </Button>
          </div>
        </div>
      </Dialog>

      {/* Batch Min Price Calculation Modal */}
      <Dialog
        open={batchMinPriceModalOpen}
        onClose={() => setBatchMinPriceModalOpen(false)}
        title="最低价设置"
        description="根据商品价格与设置的倍数批量计算最低保护底价（底价 = 原价 ÷ 倍数），防止竞价跌破利润底线。"
        maxWidth="md"
      >
        <div className="space-y-4 pt-1">
          {/* Base Price & Ratio */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-foreground">
                原价基准来源
              </label>
              <select
                value={batchPriceBase}
                onChange={(e) => setBatchPriceBase(e.target.value as 'selling_price' | 'rrp')}
                className="w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-xs text-foreground shadow-xs focus:outline-none focus:ring-1 focus:ring-ring cursor-pointer"
              >
                <option value="rrp">建议零售价 RRP (划线价，默认)</option>
                <option value="selling_price">当前售价 Selling Price</option>
              </select>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-foreground">
                底价折算倍数 (除数)
              </label>
              <Input
                type="number"
                min="0.1"
                step="0.1"
                value={batchRatio || ''}
                onChange={(e) => setBatchRatio(parseFloat(e.target.value) || 0)}
                placeholder="例如 2"
                className="h-9 text-xs font-mono"
              />
            </div>
          </div>

          {/* Quick Ratio Pills */}
          <div className="flex items-center gap-1.5 text-xs">
            <span className="text-muted-foreground text-[11px]">常用快捷倍数:</span>
            {[1.2, 1.5, 1.8, 2.0, 2.5, 3.0].map((r) => (
              <button
                key={r}
                type="button"
                onClick={() => setBatchRatio(r)}
                className={cn(
                  "px-2 py-0.5 rounded border text-[11px] font-mono transition cursor-pointer",
                  batchRatio === r
                    ? "bg-primary text-primary-foreground border-primary"
                    : "bg-muted/50 border-border text-foreground hover:bg-muted"
                )}
              >
                {r}倍
              </button>
            ))}
          </div>

          {/* Calculation Formula Preview */}
          <div className="rounded-lg bg-blue-50/60 dark:bg-blue-950/30 border border-blue-200/60 dark:border-blue-800/40 p-3 text-xs space-y-1.5">
            <div className="font-semibold text-blue-900 dark:text-blue-300 flex items-center gap-1.5">
              <span>📐 计算公式：</span>
              <code className="bg-blue-100 dark:bg-blue-900/60 px-1.5 py-0.5 rounded text-[11px]">
                最低保护底价 = {batchPriceBase === 'selling_price' ? '当前售价' : '建议零售价'} ÷ {batchRatio > 0 ? batchRatio : '?'}
              </code>
            </div>
            <div className="text-blue-700 dark:text-blue-400 text-[11px] leading-relaxed">
              {batchRatio > 0 ? (
                <span>
                  💡 示例：商品原价为 <strong>R 300</strong>，设置 <strong>{batchRatio} 倍</strong>，则最低保护底价将设置为{' '}
                  <strong className="text-blue-950 dark:text-blue-200 text-xs">
                    R {Math.max(1, Math.round(300 / batchRatio))}
                  </strong>
                  。自动竞价引擎跟价降低到此价格后将不会再降，守住利润底线。
                </span>
              ) : (
                <span className="text-rose-500 font-medium">⚠️ 请输入大于 0 的有效倍数</span>
              )}
            </div>
          </div>

          {/* Target Scope Selection */}
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              应用范围
            </label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setBatchScope('all')}
                className={cn(
                  "flex items-center justify-between p-2.5 rounded-lg border text-xs text-left transition cursor-pointer",
                  batchScope === 'all'
                    ? "border-primary bg-primary/5 font-medium text-foreground"
                    : "border-border hover:bg-muted/50 text-muted-foreground"
                )}
              >
                <span>全部商品</span>
                <Badge variant="secondary" className="text-[10px]">
                  {offers.length} 款
                </Badge>
              </button>

              <button
                type="button"
                disabled={totalMonitored === 0}
                onClick={() => setBatchScope('selected')}
                className={cn(
                  "flex items-center justify-between p-2.5 rounded-lg border text-xs text-left transition",
                  totalMonitored === 0
                    ? "opacity-50 cursor-not-allowed border-border"
                    : batchScope === 'selected'
                    ? "border-primary bg-primary/5 font-medium text-foreground cursor-pointer"
                    : "border-border hover:bg-muted/50 text-muted-foreground cursor-pointer"
                )}
              >
                <span>仅选中的商品</span>
                <Badge variant={totalMonitored > 0 ? "default" : "secondary"} className="text-[10px]">
                  {totalMonitored} 款
                </Badge>
              </button>
            </div>
          </div>

          {/* Action Buttons: Empty Only vs Overwrite All */}
          <div className="pt-3 border-t border-border flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setBatchMinPriceModalOpen(false)}
            >
              取消
            </Button>
            <div className="flex items-center gap-2 justify-end">
              <Button
                variant="secondary"
                size="sm"
                disabled={batchRatio <= 0 || targetEmptyCount === 0}
                onClick={() => handleApplyBatchMinPrice('empty_only')}
                className="gap-1 text-xs"
                title="只为当前底价未设置(为空或为0)的商品填充底价，保留已有的底价设置"
              >
                <span>仅填充空白底价 ({targetEmptyCount}款)</span>
              </Button>
              <Button
                variant="default"
                size="sm"
                disabled={batchRatio <= 0 || targetAllCount === 0}
                onClick={() => handleApplyBatchMinPrice('overwrite_all')}
                className="gap-1 text-xs"
                title="无论已有底价是多少，全部按当前倍数重新计算覆盖"
              >
                <span>全部强制覆盖 ({targetAllCount}款)</span>
              </Button>
            </div>
          </div>
        </div>
      </Dialog>
    </div>
  )
}

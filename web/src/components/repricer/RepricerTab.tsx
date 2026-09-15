import React, { useState, useEffect, useMemo } from 'react'
import { api } from '../../api/client'
import type { OfferViewModel, PriorityStatus, TargetConfig } from '../../types'
import { Card, CardHeader, CardTitle, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Dialog } from '../ui/dialog'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { ImagePreviewModal } from '../ui/ImagePreviewModal'
import { formatCurrency } from '../../lib/utils'
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
} from 'lucide-react'

export const RepricerTab: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [offers, setOffers] = useState<OfferViewModel[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | PriorityStatus | 'selected'>('all')
  const [savingTargets, setSavingTargets] = useState(false)
  const [hasChanges, setHasChanges] = useState(false)

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

  // History modal state
  const [historyModal, setHistoryModal] = useState<{
    open: boolean
    records: any[]
    loading?: boolean
    title?: string
  }>({ open: false, records: [] })

  const loadHistory = async () => {
    setHistoryModal((prev) => ({ ...prev, open: true, loading: true }))
    try {
      const list = await api.getRepriceHistory(100)
      setHistoryModal({ open: true, records: list || [], loading: false })
    } catch (err: any) {
      alert(`获取调价历史失败: ${err.message}`)
      setHistoryModal({ open: false, records: [] })
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
  }, [])

  // Filter & Search
  const filteredOffers = useMemo(() => {
    return offers.filter((item) => {
      // Search
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase()
        const matchesTitle = item.title?.toLowerCase().includes(q)
        const matchesTSIN = item.tsin_id?.toLowerCase().includes(q)
        const matchesPLID = item.plid?.toLowerCase().includes(q)
        if (!matchesTitle && !matchesTSIN && !matchesPLID) return false
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

      // Call API
      const offerId = editOffer.offer.key.split('/')[0] // or use official offer update
      await api.updateOfficialOffer(offerId, payload)
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

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
            智能调价与防亏保护管理
          </h2>
          <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
            实时竞品雷达巡检 · 智能 Buybox 优先权争夺 · 针对单品配置独立底价防亏护城河
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Button variant="outline" size="sm" onClick={loadHistory} className="gap-1.5 text-xs">
            <History className="h-3.5 w-3.5 text-primary" />
            <span>调价历史</span>
          </Button>
          <Button variant="outline" size="sm" onClick={() => loadOffers(false)} loading={loading} className="gap-1.5 text-xs">
            <RefreshCw className="h-3.5 w-3.5" />
            <span>刷新列表 (毫秒级)</span>
          </Button>
          <Button variant="outline" size="sm" onClick={() => loadOffers(true)} loading={syncing} className="gap-1.5 text-xs border-primary/40 text-primary hover:bg-primary/5">
            <CloudDownload className="h-3.5 w-3.5" />
            <span>从店铺全量同步商品</span>
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
      </div>

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

      {/* Search & Actions Bar */}
      <Card>
        <CardContent className="p-4 flex flex-col md:flex-row items-center justify-between gap-3">
          <div className="relative w-full md:w-80">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="搜索商品标题、TSIN 或 PLID..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 h-9 text-xs"
            />
          </div>

          <div className="flex items-center gap-2 w-full md:w-auto justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleSelectAllFiltered(true)}
              className="text-xs"
            >
              勾选当前筛选
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleSelectAllFiltered(false)}
              className="text-xs"
            >
              取消勾选
            </Button>
          </div>
        </CardContent>
      </Card>

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
                          <p className="font-medium text-foreground line-clamp-2 text-xs leading-snug" title={item.title}>
                            {item.title || '未知商品标题'}
                          </p>
                          <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground font-mono">
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

      {/* SQLite Reprice History Modal */}
      <Dialog
        open={historyModal.open}
        onClose={() => setHistoryModal({ open: false, records: [] })}
        title={
          <div className="flex items-center gap-2">
            <History className="h-4 w-4 text-primary" />
            <span>SQLite 自动调价历史流水</span>
            <Badge variant="success" dot className="text-[10px]">持久化数据库</Badge>
          </div>
        }
        description="记录自动化巡检引擎历次价格变动的商品、原价、新价与竞争决策原因"
        maxWidth="2xl"
      >
        <div className="space-y-4">
          {historyModal.loading ? (
            <div className="py-12 text-center text-xs text-muted-foreground">正在从 SQLite 数据库载入记录...</div>
          ) : historyModal.records.length === 0 ? (
            <div className="py-12 text-center text-xs text-muted-foreground">
              SQLite 数据库中暂无改价历史（当自动化引擎检测到价格变动并调价成功时将自动写入）
            </div>
          ) : (
            <div className="max-h-[440px] overflow-y-auto rounded-xl border border-border">
              <Table>
                <TableHeader className="bg-muted/40">
                  <TableRow>
                    <TableHead className="w-36">时间</TableHead>
                    <TableHead>商品信息</TableHead>
                    <TableHead className="text-center w-20">原价</TableHead>
                    <TableHead className="text-center w-20 font-semibold text-foreground">新价</TableHead>
                    <TableHead className="text-center w-24">对手最低价</TableHead>
                    <TableHead>决策原因</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody className="font-mono">
                  {historyModal.records.map((r) => (
                    <TableRow key={r.id} className="hover:bg-muted/30">
                      <TableCell className="text-muted-foreground text-[11px] whitespace-nowrap">{r.created_at}</TableCell>
                      <TableCell className="font-sans max-w-[200px] truncate" title={r.title}>
                        <div className="text-foreground font-medium truncate">{r.title || r.target_key}</div>
                        <div className="text-[10px] text-muted-foreground font-mono">TSIN: {r.tsin_id}</div>
                      </TableCell>
                      <TableCell className="text-center text-muted-foreground line-through">R {r.old_price}</TableCell>
                      <TableCell className="text-center font-bold text-emerald-600 dark:text-emerald-400">R {r.new_price}</TableCell>
                      <TableCell className="text-center text-foreground">R {r.competitor_price}</TableCell>
                      <TableCell className="text-[11px] font-sans text-muted-foreground">{r.reason}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          <div className="flex justify-end pt-2">
            <Button variant="outline" size="sm" onClick={() => setHistoryModal({ open: false, records: [] })}>
              关闭
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}

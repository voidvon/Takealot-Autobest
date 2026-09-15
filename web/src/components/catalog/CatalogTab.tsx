import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { OfficialOfferItem } from '../../types'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Dialog } from '../ui/dialog'
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from '../ui/table'
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs'
import { ImagePreviewModal } from '../ui/ImagePreviewModal'
import { formatCurrency } from '../../lib/utils'
import {
  Search,
  RefreshCw,
  Edit,
  ExternalLink,
  ImageIcon,
  Warehouse,
  ChevronLeft,
  ChevronRight,
  Filter,
  Plus,
  PowerOff,
  Power,
  Info,
  Package,
} from 'lucide-react'

export const CatalogTab: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [offers, setOffers] = useState<OfficialOfferItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(30)
  const [filterQuery, setFilterQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'disabled'>('all')

  // Image preview state
  const [previewImage, setPreviewImage] = useState<{
    open: boolean
    imageUrl?: string
    imageLargeUrl?: string
    title?: string
    tsin?: number
  }>({ open: false })

  // Edit Offer state
  const [editOffer, setEditOffer] = useState<{
    open: boolean
    item?: OfficialOfferItem
    sellingPrice?: number
    rrp?: number
    leadtimeDays?: number
    status?: string
    saving?: boolean
  }>({ open: false })

  // Single Offer Inspector state
  const [inspectModal, setInspectModal] = useState<{
    open: boolean
    query: string
    loading?: boolean
    result?: any
    error?: string
  }>({ open: false, query: '' })

  // Create Offer by Barcode state
  const [createModal, setCreateModal] = useState<{
    open: boolean
    barcode: string
    sku: string
    sellingPrice: string
    rrp: string
    leadtimeDays: string
    submitting?: boolean
  }>({
    open: false,
    barcode: '',
    sku: '',
    sellingPrice: '',
    rrp: '',
    leadtimeDays: '7',
  })

  // Status toggle loading state
  const [statusLoadingId, setStatusLoadingId] = useState<number | null>(null)

  const loadOffers = async (targetPage = page) => {
    setLoading(true)
    try {
      const res = await api.getOfficialOffers(targetPage, pageSize, filterQuery)
      setOffers(res.offers || [])
      setTotal(res.total_results || 0)
      setPage(res.page_number || targetPage)
    } catch (err: any) {
      alert(`获取官方商品目录失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadOffers(1)
  }, [filterQuery])

  const handleEditSubmit = async () => {
    if (!editOffer.item) return
    setEditOffer((prev) => ({ ...prev, saving: true }))
    try {
      await api.updateOfficialOffer(editOffer.item.offer_id, {
        selling_price: editOffer.sellingPrice,
        rrp: editOffer.rrp,
        leadtime_days: editOffer.leadtimeDays,
        status: editOffer.status,
      })
      alert('商品参数已更新生效！')
      setEditOffer({ open: false })
      loadOffers(page)
    } catch (err: any) {
      alert(`更新失败: ${err.message}`)
    } finally {
      setEditOffer((prev) => ({ ...prev, saving: false }))
    }
  }

  const handleToggleStatus = async (item: OfficialOfferItem) => {
    const isCurrentlyDisabled = item.status.toLowerCase().includes('disabled')
    const action = isCurrentlyDisabled ? 'Re-enable' : 'Disable'
    const confirmMsg = isCurrentlyDisabled
      ? `确认要重新启用/上架商品 "${item.title}" 吗？`
      : `确认要下架/停用商品 "${item.title}" 吗？(停用后买家端不可购买)`

    if (!confirm(confirmMsg)) return

    setStatusLoadingId(item.offer_id)
    try {
      await api.setOfficialOfferStatus(String(item.offer_id), action)
      // Update locally
      setOffers((prev) =>
        prev.map((o) =>
          o.offer_id === item.offer_id
            ? { ...o, status: isCurrentlyDisabled ? 'Buyable' : 'Disabled by Seller' }
            : o
        )
      )
    } catch (err: any) {
      alert(`操作失败: ${err.message}`)
    } finally {
      setStatusLoadingId(null)
    }
  }

  const handleInspectSubmit = async () => {
    if (!inspectModal.query.trim()) return
    setInspectModal((prev) => ({ ...prev, loading: true, result: undefined, error: undefined }))
    try {
      let identifier = inspectModal.query.trim()
      // If user typed only numbers, could be offer id. Or if starts with BARCODE or SKU
      const res = await api.getOfficialSingleOffer(identifier)
      setInspectModal((prev) => ({ ...prev, loading: false, result: res }))
    } catch (err: any) {
      setInspectModal((prev) => ({ ...prev, loading: false, error: err.message }))
    }
  }

  const handleCreateSubmit = async () => {
    if (!createModal.barcode.trim()) {
      alert('请输入商品的官方条形码 (Barcode)')
      return
    }
    const price = parseInt(createModal.sellingPrice)
    if (isNaN(price) || price <= 0) {
      alert('请输入有效的售价 (正整数字)')
      return
    }
    setCreateModal((prev) => ({ ...prev, submitting: true }))
    try {
      await api.createOfficialOffer({
        barcode: createModal.barcode.trim(),
        sku: createModal.sku.trim() || undefined,
        selling_price: price,
        rrp: createModal.rrp ? parseInt(createModal.rrp) : undefined,
        leadtime_days: createModal.leadtimeDays ? parseInt(createModal.leadtimeDays) : 7,
      })
      alert(`成功创建商品 Offer！条码: ${createModal.barcode}`)
      setCreateModal({
        open: false,
        barcode: '',
        sku: '',
        sellingPrice: '',
        rrp: '',
        leadtimeDays: '7',
      })
      loadOffers(1)
    } catch (err: any) {
      alert(`创建 Offer 失败: ${err.message}`)
    } finally {
      setCreateModal((prev) => ({ ...prev, submitting: false }))
    }
  }

  const filteredItems = offers.filter((o) => {
    if (statusFilter === 'active') return !o.status.toLowerCase().includes('disabled')
    if (statusFilter === 'disabled') return o.status.toLowerCase().includes('disabled')
    return true
  })

  const totalPages = Math.ceil(total / pageSize) || 1

  return (
    <div className="space-y-6">
      {/* Header controls using standard shadcn Card */}
      <Card>
        <CardContent className="p-4 sm:p-5 flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-bold tracking-tight text-foreground">官方目录商品库</h2>
              <Badge variant="secondary" className="font-mono">
                {total} 个 Offers
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              Takealot 官方 Seller API 全量目录检索、多仓库存分布与单品上下架管理
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setInspectModal({ open: true, query: '' })}
              className="gap-1.5 text-xs"
            >
              <Search className="h-3.5 w-3.5 text-primary" />
              <span>单品速查</span>
            </Button>
            <Button
              variant="default"
              size="sm"
              onClick={() => setCreateModal((prev) => ({ ...prev, open: true }))}
              className="gap-1.5 text-xs"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>条码新建 Offer</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => loadOffers(page)}
              loading={loading}
              className="gap-1.5 text-xs"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              <span>刷新</span>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Filters bar */}
      <div className="flex flex-col sm:flex-row items-center gap-3">
        <div className="relative flex-1 w-full">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="搜索商品标题、SKU、TSIN 或条形码 (Barcode)..."
            value={filterQuery}
            onChange={(e) => setFilterQuery(e.target.value)}
            className="pl-9 h-9 text-xs"
          />
        </div>

        <Tabs value={statusFilter} onValueChange={(v) => setStatusFilter(v as any)} className="self-end sm:self-auto">
          <TabsList className="h-9 p-0.5">
            <TabsTrigger value="all" className="text-xs h-8 px-3">
              全部 ({offers.length})
            </TabsTrigger>
            <TabsTrigger value="active" className="text-xs h-8 px-3">
              可售中
            </TabsTrigger>
            <TabsTrigger value="disabled" className="text-xs h-8 px-3">
              已停售
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* Catalog Table using standard shadcn Table */}
      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader className="bg-muted/40">
              <TableRow>
                <TableHead className="w-12 text-center">图示</TableHead>
                <TableHead className="min-w-[240px]">商品标题 / TSIN</TableHead>
                <TableHead className="w-28 text-center">条形码 / SKU</TableHead>
                <TableHead className="w-24 text-center">在售价格</TableHead>
                <TableHead className="w-20 text-center">原价 (RRP)</TableHead>
                <TableHead className="w-28 text-center">官方仓储量</TableHead>
                <TableHead className="w-24 text-center">状态</TableHead>
                <TableHead className="w-28 text-right pr-4">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={8} className="py-16 text-center text-xs text-muted-foreground">
                    正在同步官方商品目录...
                  </TableCell>
                </TableRow>
              ) : filteredItems.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="py-16 text-center text-xs text-muted-foreground">
                    未检索到符合条件的官方商品
                  </TableCell>
                </TableRow>
              ) : (
                filteredItems.map((item) => {
                  const cptStock = item.stock_at_takealot?.find((s) => s.warehouse?.name === 'CPT')?.quantity_available || 0
                  const jhbStock = item.stock_at_takealot?.find((s) => s.warehouse?.name === 'JHB')?.quantity_available || 0
                  const dbnStock = item.stock_at_takealot?.find((s) => s.warehouse?.name === 'DBN')?.quantity_available || 0
                  const totalWhStock = item.stock_at_takealot_total ?? (cptStock + jhbStock + dbnStock)
                  const isDisabled = item.status.toLowerCase().includes('disabled')

                  return (
                    <TableRow key={item.offer_id} className="hover:bg-muted/40 transition-colors">
                      {/* Thumbnail with HD modal trigger */}
                      <TableCell className="text-center py-2.5">
                        {item.image_url ? (
                          <div
                            onClick={() =>
                              setPreviewImage({
                                open: true,
                                imageUrl: item.image_url,
                                imageLargeUrl: item.image_large_url,
                                title: item.title,
                                tsin: item.tsin_id,
                              })
                            }
                            className="relative group cursor-pointer h-10 w-10 mx-auto rounded-lg overflow-hidden border border-border/80 bg-background flex items-center justify-center transition-all shadow-xs hover:ring-2 hover:ring-primary/40"
                          >
                            <img
                              src={item.image_url}
                              alt={item.title}
                              className="h-full w-full object-contain p-0.5 group-hover:scale-110 transition-transform duration-200"
                              loading="lazy"
                            />
                            <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                              <ImageIcon className="h-3 w-3 text-white" />
                            </div>
                          </div>
                        ) : (
                          <div className="h-10 w-10 mx-auto rounded-lg bg-muted flex items-center justify-center text-muted-foreground text-[10px]">
                            无图
                          </div>
                        )}
                      </TableCell>

                      {/* Title & TSIN */}
                      <TableCell className="py-2.5">
                        <div className="space-y-1 max-w-[320px]">
                          <div className="font-medium text-foreground text-xs leading-snug line-clamp-2" title={item.title}>
                            {item.title}
                          </div>
                          <div className="flex items-center gap-2 text-[11px] text-muted-foreground font-mono">
                            <span>TSIN: {item.tsin_id}</span>
                            <span>OfferID: {item.offer_id}</span>
                            {item.offer_url && (
                              <a
                                href={item.offer_url}
                                target="_blank"
                                rel="noreferrer"
                                className="text-primary hover:underline inline-flex items-center gap-0.5"
                              >
                                <span>官网页面</span>
                                <ExternalLink className="h-2.5 w-2.5" />
                              </a>
                            )}
                          </div>
                        </div>
                      </TableCell>

                      {/* Barcode & SKU */}
                      <TableCell className="py-2.5 text-center font-mono text-[11px]">
                        <div className="text-foreground">{item.barcode || '-'}</div>
                        <div className="text-[10px] text-muted-foreground">{item.sku || '-'}</div>
                      </TableCell>

                      {/* Selling Price */}
                      <TableCell className="py-2.5 text-center">
                        <span className="font-bold font-mono text-xs text-foreground">
                          {formatCurrency(item.selling_price)}
                        </span>
                      </TableCell>

                      {/* RRP */}
                      <TableCell className="py-2.5 text-center font-mono text-xs text-muted-foreground">
                        {item.rrp ? formatCurrency(item.rrp) : '-'}
                      </TableCell>

                      {/* Warehouse Stock breakdown */}
                      <TableCell className="py-2.5 text-center font-mono">
                        <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-muted/60 text-xs">
                          <Warehouse className="h-3 w-3 text-muted-foreground" />
                          <span className="font-semibold text-foreground">{totalWhStock}</span>
                        </div>
                        <div className="text-[10px] text-muted-foreground space-x-1.5 mt-0.5">
                          <span title="开普敦仓">C:{cptStock}</span>
                          <span title="约翰内斯堡仓">J:{jhbStock}</span>
                          <span title="德班仓">D:{dbnStock}</span>
                        </div>
                      </TableCell>

                      {/* Status */}
                      <TableCell className="py-2.5 text-center">
                        {isDisabled ? (
                          <Badge variant="destructive" className="text-[10px] whitespace-nowrap">
                            已停售
                          </Badge>
                        ) : (
                          <Badge variant="success" dot className="text-[10px] whitespace-nowrap">
                            可售中
                          </Badge>
                        )}
                      </TableCell>

                      {/* Actions */}
                      <TableCell className="py-2.5 text-right pr-4">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-foreground"
                            onClick={() =>
                              setEditOffer({
                                open: true,
                                item,
                                sellingPrice: item.selling_price,
                                rrp: item.rrp,
                                leadtimeDays: item.leadtime_days,
                                status: item.status,
                              })
                            }
                            title="修改售价与库存"
                          >
                            <Edit className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className={`h-7 w-7 ${
                              isDisabled
                                ? 'text-emerald-600 hover:text-emerald-700'
                                : 'text-amber-600 hover:text-amber-700'
                            }`}
                            loading={statusLoadingId === item.offer_id}
                            onClick={() => handleToggleStatus(item)}
                            title={isDisabled ? '重新启用/上架' : '停售/下架商品'}
                          >
                            {isDisabled ? <Power className="h-3.5 w-3.5" /> : <PowerOff className="h-3.5 w-3.5" />}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>

        {/* Pagination Footer */}
        <div className="flex items-center justify-between p-3 border-t border-border bg-muted/20 text-xs text-muted-foreground">
          <div>
            共 {total} 个商品 · 第 {page} / {totalPages} 页
          </div>
          <div className="flex items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || loading}
              onClick={() => loadOffers(page - 1)}
              className="h-7 px-2"
            >
              <ChevronLeft className="h-3.5 w-3.5" />
              <span>上一页</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages || loading}
              onClick={() => loadOffers(page + 1)}
              className="h-7 px-2"
            >
              <span>下一页</span>
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      </Card>

      {/* Edit Offer Modal */}
      <Dialog
        open={editOffer.open}
        onClose={() => setEditOffer({ open: false })}
        title="编辑官方商品 Offer 参数"
        description={`正在修改商品 TSIN: ${editOffer.item?.tsin_id} · OfferID: ${editOffer.item?.offer_id}`}
        maxWidth="md"
      >
        <div className="space-y-4 text-xs">
          <div>
            <div className="font-medium text-foreground mb-1">商品标题</div>
            <div className="p-2.5 rounded-lg bg-muted text-muted-foreground">{editOffer.item?.title}</div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-foreground font-medium mb-1">售价 (Selling Price - R)</label>
              <Input
                type="number"
                value={editOffer.sellingPrice ?? ''}
                onChange={(e) => setEditOffer((prev) => ({ ...prev, sellingPrice: parseInt(e.target.value) || 0 }))}
                className="h-8 text-xs font-mono font-bold text-primary"
              />
            </div>
            <div>
              <label className="block text-foreground font-medium mb-1">原价 (RRP - R)</label>
              <Input
                type="number"
                value={editOffer.rrp ?? ''}
                onChange={(e) => setEditOffer((prev) => ({ ...prev, rrp: parseInt(e.target.value) || 0 }))}
                className="h-8 text-xs font-mono"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-foreground font-medium mb-1">发货时限 (Leadtime Days)</label>
              <Input
                type="number"
                value={editOffer.leadtimeDays ?? ''}
                onChange={(e) => setEditOffer((prev) => ({ ...prev, leadtimeDays: parseInt(e.target.value) || 0 }))}
                className="h-8 text-xs font-mono"
              />
            </div>
            <div>
              <label className="block text-foreground font-medium mb-1">当前状态</label>
              <Input
                disabled
                value={editOffer.status || ''}
                className="h-8 text-xs bg-muted text-muted-foreground"
              />
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setEditOffer({ open: false })}>
              取消
            </Button>
            <Button variant="default" size="sm" loading={editOffer.saving} onClick={handleEditSubmit}>
              保存并提交官方
            </Button>
          </div>
        </div>
      </Dialog>

      {/* Single Offer Inspector Dialog */}
      <Dialog
        open={inspectModal.open}
        onClose={() => setInspectModal({ open: false, query: '' })}
        title="官方单品速查 (Single Offer Inspector)"
        description="支持通过 Offer ID、条形码 (Barcode) 或商家 SKU 秒级调取 Takealot 官方实时 Offer 详情"
        maxWidth="xl"
      >
        <div className="space-y-4 text-xs">
          <div className="flex items-center gap-2">
            <Input
              placeholder="输入 Offer ID (如 226816605)、Barcode (如 MPTALX...) 或 SKU..."
              value={inspectModal.query}
              onChange={(e) => setInspectModal((prev) => ({ ...prev, query: e.target.value }))}
              onKeyDown={(e) => e.key === 'Enter' && handleInspectSubmit()}
              className="h-9 text-xs"
            />
            <Button
              variant="default"
              size="sm"
              onClick={handleInspectSubmit}
              loading={inspectModal.loading}
              className="whitespace-nowrap"
            >
              查询
            </Button>
          </div>

          {inspectModal.error && (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive border border-destructive/20">
              查询失败: {inspectModal.error}
            </div>
          )}

          {inspectModal.result && (
            <div className="rounded-xl border border-border p-4 bg-muted/20 space-y-3">
              <div className="flex items-start gap-3">
                {inspectModal.result.image_url && (
                  <img
                    src={inspectModal.result.image_url}
                    alt=""
                    className="h-14 w-14 object-contain rounded-lg border border-border bg-background p-1"
                  />
                )}
                <div className="flex-1 min-w-0">
                  <div className="font-semibold text-foreground text-sm line-clamp-2">
                    {inspectModal.result.title}
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground mt-1 font-mono">
                    <span>TSIN: {inspectModal.result.tsin_id}</span>
                    <span>Offer ID: {inspectModal.result.offer_id}</span>
                    <span>Barcode: {inspectModal.result.barcode}</span>
                    <span>SKU: {inspectModal.result.sku}</span>
                  </div>
                </div>
              </div>

              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-2">
                <div className="p-2.5 rounded-lg bg-card border border-border text-center">
                  <div className="text-[10px] text-muted-foreground">官方售价</div>
                  <div className="font-bold text-sm text-foreground font-mono mt-0.5">
                    R {inspectModal.result.selling_price}
                  </div>
                </div>
                <div className="p-2.5 rounded-lg bg-card border border-border text-center">
                  <div className="text-[10px] text-muted-foreground">建议零售价 (RRP)</div>
                  <div className="font-bold text-sm text-muted-foreground font-mono mt-0.5">
                    R {inspectModal.result.rrp || '-'}
                  </div>
                </div>
                <div className="p-2.5 rounded-lg bg-card border border-border text-center">
                  <div className="text-[10px] text-muted-foreground">全仓在库总量</div>
                  <div className="font-bold text-sm text-foreground font-mono mt-0.5">
                    {inspectModal.result.stock_at_takealot_total ?? 0}
                  </div>
                </div>
                <div className="p-2.5 rounded-lg bg-card border border-border text-center">
                  <div className="text-[10px] text-muted-foreground">状态</div>
                  <div className="font-bold text-xs mt-0.5 text-foreground">
                    {inspectModal.result.status}
                  </div>
                </div>
              </div>
            </div>
          )}

          <div className="flex justify-end pt-2">
            <Button variant="outline" size="sm" onClick={() => setInspectModal({ open: false, query: '' })}>
              关闭
            </Button>
          </div>
        </div>
      </Dialog>

      {/* Create Offer by Barcode Dialog */}
      <Dialog
        open={createModal.open}
        onClose={() => setCreateModal((prev) => ({ ...prev, open: false }))}
        title="官方条形码新建 Offer (Create Offer by Barcode)"
        description="根据 Takealot 官方目录库中已有商品的 Barcode，直接创建该商品并在你的店铺上架"
        maxWidth="md"
      >
        <div className="space-y-3.5 text-xs">
          <div>
            <label className="block text-foreground font-medium mb-1">商品官方条形码 (Barcode) *</label>
            <Input
              placeholder="例如: MPTALX18729328-0 或厂商 EAN 条码"
              value={createModal.barcode}
              onChange={(e) => setCreateModal((prev) => ({ ...prev, barcode: e.target.value }))}
              className="h-8 text-xs font-mono"
            />
          </div>

          <div>
            <label className="block text-foreground font-medium mb-1">自定义商家 SKU (可选)</label>
            <Input
              placeholder="例如: MY-STORE-SKU-001 (留空则系统自动分配)"
              value={createModal.sku}
              onChange={(e) => setCreateModal((prev) => ({ ...prev, sku: e.target.value }))}
              className="h-8 text-xs font-mono"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-foreground font-medium mb-1">销售价格 (R) *</label>
              <Input
                type="number"
                placeholder="例如: 399"
                value={createModal.sellingPrice}
                onChange={(e) => setCreateModal((prev) => ({ ...prev, sellingPrice: e.target.value }))}
                className="h-8 text-xs font-mono font-bold"
              />
            </div>
            <div>
              <label className="block text-foreground font-medium mb-1">建议零售价 RRP (R)</label>
              <Input
                type="number"
                placeholder="例如: 699"
                value={createModal.rrp}
                onChange={(e) => setCreateModal((prev) => ({ ...prev, rrp: e.target.value }))}
                className="h-8 text-xs font-mono"
              />
            </div>
          </div>

          <div>
            <label className="block text-foreground font-medium mb-1">发货准备天数 (Leadtime Days)</label>
            <Input
              type="number"
              value={createModal.leadtimeDays}
              onChange={(e) => setCreateModal((prev) => ({ ...prev, leadtimeDays: e.target.value }))}
              className="h-8 text-xs font-mono"
            />
            <p className="text-[11px] text-muted-foreground mt-1">
              默认 7 天；若设置为 -1 则取消 Leadtime 限制
            </p>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setCreateModal((prev) => ({ ...prev, open: false }))}>
              取消
            </Button>
            <Button variant="default" size="sm" loading={createModal.submitting} onClick={handleCreateSubmit}>
              确认创建并上架
            </Button>
          </div>
        </div>
      </Dialog>

      {/* HD Image Preview Modal */}
      <ImagePreviewModal
        open={previewImage.open}
        onClose={() => setPreviewImage({ open: false })}
        imageUrl={previewImage.imageUrl}
        imageLargeUrl={previewImage.imageLargeUrl}
        title={previewImage.title}
        tsin={previewImage.tsin}
      />
    </div>
  )
}

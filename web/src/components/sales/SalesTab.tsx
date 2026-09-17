import React, { useState, useEffect, useMemo } from 'react'
import { api } from '../../api/client'
import type {
  SaleItem,
  OrderRecord,
  InvoiceDocument,
  LeadtimeOrderItem,
  ShipmentRecord,
  ShipmentItemRecord,
  BookingRecord,
} from '../../types'
import { Card, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Dialog } from '../ui/dialog'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs'
import { formatCurrency, formatDateTime } from '../../lib/utils'
import { useTicker, formatDetailedCountdown } from '../../lib/countdown'
import {
  ShoppingBag,
  RefreshCw,
  FileText,
  Calendar,
  DollarSign,
  Truck,
  User,
  ChevronLeft,
  ChevronRight,
  Receipt,
  Download,
  Clock,
  Scale,
  Edit3,
  CheckCircle2,
  AlertCircle,
  Plus,
  Trash2,
  Package,
  Send,
  CalendarDays,
  Search,
  ExternalLink,
  Layers,
  Sparkles,
} from 'lucide-react'

type MainTab = 'leadtime' | 'draft' | 'confirmed' | 'shipped' | 'bookings' | 'history' | 'orders'

export const SalesTab: React.FC = () => {
  const [activeTab, setActiveTab] = useState<MainTab>('draft')
  const [loading, setLoading] = useState(false)
  const [syncStatusMsg, setSyncStatusMsg] = useState<string | null>('正在同步草稿发货单 任务已受理，正在排队。')

  // 本地实时心跳与加载时间戳
  const [loadedAt, setLoadedAt] = useState(() => Date.now())
  const currentTick = useTicker(1000, activeTab === 'leadtime' || activeTab === 'draft')

  // Data states
  const [leadtimeOrders, setLeadtimeOrders] = useState<LeadtimeOrderItem[]>([])
  const [shipments, setShipments] = useState<ShipmentRecord[]>([])
  const [bookings, setBookings] = useState<BookingRecord[]>([])

  // Filters for tables (对标图中的表头内联过滤器)
  const [pageSize, setPageSize] = useState<number>(100)
  const [currentPage, setCurrentPage] = useState<number>(1)
  const [dateSortDir, setDateSortDir] = useState<'desc' | 'asc'>('desc')
  const [filterType, setFilterType] = useState<string>('Nor')
  const [filterDueDate, setFilterDueDate] = useState<string>('all')
  const [filterTitle, setFilterTitle] = useState<string>('')
  const [filterSKU, setFilterSKU] = useState<string>('')
  const [filterTSIN, setFilterTSIN] = useState<string>('')
  const [filterDC, setFilterDC] = useState<string>('All')

  // Selections
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  // History & Orders states (原有的财务账单与发票功能保留)
  const [sales, setSales] = useState<SaleItem[]>([])
  const [salesTotal, setSalesTotal] = useState(0)
  const [orders, setOrders] = useState<OrderRecord[]>([])
  const [ordersTotal, setOrdersTotal] = useState(0)
  const [daysRange, setDaysRange] = useState<number>(30)

  // Modals
  const [weighModal, setWeighModal] = useState<{
    open: boolean
    item?: LeadtimeOrderItem | ShipmentItemRecord
    actualWeight: number
    volumetricWeight: number
  }>({ open: false, actualWeight: 0, volumetricWeight: 0 })

  const [priceModal, setPriceModal] = useState<{
    open: boolean
    item?: LeadtimeOrderItem | ShipmentItemRecord
    sellingPrice: number
    rrp: number
  }>({ open: false, sellingPrice: 0, rrp: 0 })

  const [bookingModal, setBookingModal] = useState<{
    open: boolean
    shipmentId?: number
    dc: string
    bookingDate: string
    timeSlot: string
    carrier: string
    vehicleReg: string
    notes: string
  }>({
    open: false,
    dc: 'JHB',
    bookingDate: new Date(Date.now() + 86400000).toISOString().split('T')[0],
    timeSlot: '10:00 - 12:00',
    carrier: 'Courier Guy',
    vehicleReg: 'GP 882-901',
    notes: '',
  })

  const [invoiceModal, setInvoiceModal] = useState<{
    open: boolean
    orderId?: number
    invoices: InvoiceDocument[]
    loading?: boolean
  }>({ open: false, invoices: [] })

  // 1. Fetch Leadtime Orders
  const loadLeadtimeOrders = async () => {
    setLoading(true)
    try {
      const res = await api.getLeadtimeOrders()
      setLeadtimeOrders(res.items || [])
      setLoadedAt(Date.now())
    } catch (e: any) {
      console.error('加载交货期订单失败', e)
    } finally {
      setLoading(false)
    }
  }

  // 2. Fetch Shipments
  const loadShipments = async () => {
    setLoading(true)
    try {
      const res = await api.getShipments()
      setShipments(res || [])
    } catch (e: any) {
      console.error('加载发货单失败', e)
    } finally {
      setLoading(false)
    }
  }

  // 3. Fetch Bookings
  const loadBookings = async () => {
    setLoading(true)
    try {
      const res = await api.getBookings()
      setBookings(res || [])
    } catch (e: any) {
      console.error('加载送仓预约失败', e)
    } finally {
      setLoading(false)
    }
  }

  // 4. Load History Sales & Orders
  const loadSales = async () => {
    setLoading(true)
    try {
      const end = new Date().toISOString().split('T')[0]
      const startObj = new Date()
      startObj.setDate(startObj.getDate() - daysRange)
      const start = startObj.toISOString().split('T')[0]
      const res = await api.getOfficialSales(1, 50, start, end)
      setSales(res.sales || [])
      setSalesTotal(res.page_summary?.total || 0)
    } catch (err: any) {
      console.error('获取销售明细失败', err)
    } finally {
      setLoading(false)
    }
  }

  const loadOrders = async () => {
    setLoading(true)
    try {
      const end = new Date().toISOString().split('T')[0]
      const startObj = new Date()
      startObj.setDate(startObj.getDate() - daysRange)
      const start = startObj.toISOString().split('T')[0]
      const res = await api.getOfficialSalesOrders(start, end, 1, 50)
      setOrders(res.orders || [])
      setOrdersTotal(res.page_summary?.total || 0)
    } catch (err: any) {
      console.error('获取订单列表失败', err)
    } finally {
      setLoading(false)
    }
  }

  // Initial and reactive fetch
  useEffect(() => {
    loadLeadtimeOrders()
    loadShipments()
    loadBookings()
  }, [])

  useEffect(() => {
    if (activeTab === 'history') loadSales()
    if (activeTab === 'orders') loadOrders()
  }, [activeTab, daysRange])

  // 当前激活的草稿发货单（通常为第一条 draft 状态单）
  const activeDraftShipment = useMemo(() => {
    return shipments.find((s) => s.status === 'draft') || null
  }, [shipments])

  // Filtered & Sorted items depending on active Tab
  const displayItems = useMemo(() => {
    let list: Array<LeadtimeOrderItem | ShipmentItemRecord> = []

    if (activeTab === 'leadtime') {
      list = [...leadtimeOrders]
    } else if (activeTab === 'draft') {
      list = activeDraftShipment?.items ? [...activeDraftShipment.items] : []
    }

    // Apply inline filters
    return list.filter((item) => {
      if (filterTitle && !item.title.toLowerCase().includes(filterTitle.toLowerCase())) return false
      if (filterSKU && !item.sku.toLowerCase().includes(filterSKU.toLowerCase())) return false
      if (filterTSIN && !item.tsin.toLowerCase().includes(filterTSIN.toLowerCase())) return false
      if (filterDC !== 'All' && item.dc !== filterDC) return false
      if (filterDueDate === 'overdue' && (item as LeadtimeOrderItem).is_overdue === false) return false
      return true
    })
  }, [activeTab, leadtimeOrders, activeDraftShipment, filterTitle, filterSKU, filterTSIN, filterDC, filterDueDate])

  // Toggle selection
  const handleToggleSelect = (id: number) => {
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedIds(displayItems.map((_, idx) => idx + 1))
    } else {
      setSelectedIds([])
    }
  }

  // Row operations
  const handleUpdateItemQty = async (itemId: number, newQty: number, newStock: number) => {
    try {
      await api.updateShipmentItem({
        item_id: itemId,
        ship_qty: newQty,
        leadtime_stock: newStock,
        actual_weight: 0.28,
        volumetric_weight: 0.35,
        weigh_status: 'pending',
      })
      loadShipments()
    } catch (e: any) {
      alert(`更新数量失败: ${e.message}`)
    }
  }

  // Save Weight
  const handleSaveWeight = async () => {
    if (!weighModal.item) return
    try {
      await api.quickUpdateOffer({
        tsin: weighModal.item.tsin,
        weight_kg: weighModal.actualWeight,
      })
      if ('id' in weighModal.item && weighModal.item.id) {
        await api.updateShipmentItem({
          item_id: weighModal.item.id,
          ship_qty: weighModal.item.ship_qty || 1,
          leadtime_stock: weighModal.item.leadtime_stock || 0,
          actual_weight: weighModal.actualWeight,
          volumetric_weight: weighModal.volumetricWeight,
          weigh_status: 'done',
        })
      }
      setWeighModal({ open: false, actualWeight: 0, volumetricWeight: 0 })
      loadShipments()
      loadLeadtimeOrders()
    } catch (e: any) {
      alert(`保存重量失败: ${e.message}`)
    }
  }

  // Save Price
  const handleSavePrice = async () => {
    if (!priceModal.item) return
    try {
      await api.quickUpdateOffer({
        tsin: priceModal.item.tsin,
        selling_price: priceModal.sellingPrice,
        rrp: priceModal.rrp,
      })
      setPriceModal({ open: false, sellingPrice: 0, rrp: 0 })
      alert('已成功同步新价格至 Takealot 官方！')
      loadShipments()
      loadLeadtimeOrders()
    } catch (e: any) {
      alert(`改价失败: ${e.message}`)
    }
  }

  // Confirm Shipment
  const handleConfirmShipment = async (shipmentId: number) => {
    if (!confirm('确认该发货单并生成送仓单据吗？')) return
    try {
      await api.updateShipmentStatus(shipmentId, 'confirmed')
      alert('发货单已确认，已进入【已确认发货单】！')
      loadShipments()
      setActiveTab('confirmed')
    } catch (e: any) {
      alert(`确认失败: ${e.message}`)
    }
  }

  // Mark Shipped
  const handleMarkShipped = async (shipmentId: number) => {
    try {
      await api.updateShipmentStatus(shipmentId, 'shipped')
      alert('已更新为在途发货单！')
      loadShipments()
      setActiveTab('shipped')
    } catch (e: any) {
      alert(`操作失败: ${e.message}`)
    }
  }

  // Create Booking
  const handleCreateBookingSubmit = async () => {
    try {
      await api.createBooking({
        shipment_id: bookingModal.shipmentId,
        dc: bookingModal.dc,
        booking_date: bookingModal.bookingDate,
        time_slot: bookingModal.timeSlot,
        carrier: bookingModal.carrier,
        vehicle_reg: bookingModal.vehicleReg,
        notes: bookingModal.notes,
      })
      setBookingModal((prev) => ({ ...prev, open: false }))
      alert('预约创建成功！')
      loadBookings()
      setActiveTab('bookings')
    } catch (e: any) {
      alert(`创建预约失败: ${e.message}`)
    }
  }

  // Invoices modal handler
  const handleViewInvoice = async (orderId: number) => {
    setInvoiceModal({ open: true, orderId, invoices: [], loading: true })
    try {
      const res = await api.getOfficialCustomerInvoices(orderId)
      setInvoiceModal({ open: true, orderId, invoices: res.documents || [], loading: false })
    } catch (err: any) {
      alert(`获取发票失败: ${err.message}`)
      setInvoiceModal({ open: false, invoices: [] })
    }
  }

  return (
    <div className="space-y-4 animate-in fade-in duration-200">
      {/* 顶部排队同步任务横幅 (对标用户截图) */}
      {syncStatusMsg && (
        <div className="flex items-center justify-between px-4 py-2.5 bg-sky-50 dark:bg-sky-950/40 border border-sky-200 dark:border-sky-800/60 rounded-md text-sky-900 dark:text-sky-200 shadow-sm transition-all">
          <div className="flex items-center gap-3">
            <div className="h-2 w-2 rounded-full bg-sky-500 animate-ping" />
            <div className="text-xs">
              <span className="font-semibold">{syncStatusMsg.split(' ')[0]}</span>
              <span className="text-muted-foreground ml-2">{syncStatusMsg.split(' ')[1] || '任务已受理，正在排队。'}</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <RefreshCw className="h-4 w-4 animate-spin text-sky-600 dark:text-sky-400" />
            <button
              onClick={() => setSyncStatusMsg(null)}
              className="text-[11px] text-sky-600 dark:text-sky-400 hover:underline ml-2"
            >
              关闭
            </button>
          </div>
        </div>
      )}

      {/* 主 Tab 导航栏 (完全还原截图中的 5 大核心 Tab) */}
      <div className="border-b border-border/80 flex items-center justify-between pb-0">
        <div className="flex items-center gap-6 text-sm font-medium">
          <button
            onClick={() => setActiveTab('leadtime')}
            className={`pb-2.5 border-b-2 transition-colors relative ${
              activeTab === 'leadtime'
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            交货期订单
            {leadtimeOrders.length > 0 && (
              <span className="ml-1.5 px-1.5 py-0.2 bg-amber-100 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 rounded-full text-[10px] font-bold">
                {leadtimeOrders.length}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab('draft')}
            className={`pb-2.5 border-b-2 transition-colors relative ${
              activeTab === 'draft'
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            草稿发货单
            {activeDraftShipment && (
              <span className="ml-1.5 px-1.5 py-0.2 bg-blue-100 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 rounded-full text-[10px] font-bold">
                {activeDraftShipment.items?.length || 0}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab('confirmed')}
            className={`pb-2.5 border-b-2 transition-colors ${
              activeTab === 'confirmed'
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            已确认发货单
            <span className="ml-1.5 px-1.5 py-0.2 bg-muted text-muted-foreground rounded-full text-[10px]">
              {shipments.filter((s) => s.status === 'confirmed').length}
            </span>
          </button>

          <button
            onClick={() => setActiveTab('shipped')}
            className={`pb-2.5 border-b-2 transition-colors ${
              activeTab === 'shipped'
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            已发货单
            <span className="ml-1.5 px-1.5 py-0.2 bg-muted text-muted-foreground rounded-full text-[10px]">
              {shipments.filter((s) => s.status === 'shipped').length}
            </span>
          </button>

          <button
            onClick={() => setActiveTab('bookings')}
            className={`pb-2.5 border-b-2 transition-colors ${
              activeTab === 'bookings'
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            预约管理
            <span className="ml-1.5 px-1.5 py-0.2 bg-muted text-muted-foreground rounded-full text-[10px]">
              {bookings.length}
            </span>
          </button>

          <div className="h-4 w-px bg-border my-auto mx-1" />

          {/* 财务与发票查看备用 Tab */}
          <button
            onClick={() => setActiveTab('history')}
            className={`pb-2.5 text-xs transition-colors ${
              activeTab === 'history' ? 'text-primary font-semibold' : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            财务销售流水
          </button>
          <button
            onClick={() => setActiveTab('orders')}
            className={`pb-2.5 text-xs transition-colors ${
              activeTab === 'orders' ? 'text-primary font-semibold' : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            已开具发票
          </button>
        </div>

        {/* 顶部右侧刷新与操作按钮 */}
        <div className="flex items-center gap-2 pb-2">
          {activeTab === 'draft' && activeDraftShipment && (
            <Button
              size="sm"
              onClick={() => handleConfirmShipment(activeDraftShipment.id)}
              className="h-7 text-xs bg-primary hover:bg-primary/90 text-primary-foreground gap-1.5 shadow-sm"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              <span>确认发货单</span>
            </Button>
          )}

          {activeTab === 'bookings' && (
            <Button
              size="sm"
              onClick={() => setBookingModal((prev) => ({ ...prev, open: true }))}
              className="h-7 text-xs gap-1.5"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>新建送仓预约</span>
            </Button>
          )}

          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              loadLeadtimeOrders()
              loadShipments()
              loadBookings()
            }}
            loading={loading}
            className="h-7 text-xs gap-1.5 px-2.5"
          >
            <RefreshCw className="h-3 w-3" />
            <span>同步刷新</span>
          </Button>
        </div>
      </div>

      {/* 表头控制条：显示行、每页条数、分页组件 (对标截图) */}
      {(activeTab === 'leadtime' || activeTab === 'draft') && (
        <div className="flex items-center justify-between text-xs text-muted-foreground py-1">
          <div className="flex items-center gap-2">
            <span>显示</span>
            <select
              value={pageSize}
              onChange={(e) => setPageSize(Number(e.target.value))}
              className="h-6 px-1.5 py-0 border border-input rounded bg-background text-foreground text-xs focus:ring-1 focus:ring-primary"
            >
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
            <span>条/页</span>
          </div>

          <div className="flex items-center gap-3">
            <span>
              显示行：1 - {displayItems.length}，共 {displayItems.length} 条
            </span>
            <div className="flex items-center border border-border rounded overflow-hidden">
              <button className="px-1.5 py-0.5 hover:bg-muted text-muted-foreground disabled:opacity-40">«</button>
              <button className="px-1.5 py-0.5 hover:bg-muted text-muted-foreground border-l border-border disabled:opacity-40">‹</button>
              <button className="px-1.5 py-0.5 hover:bg-muted text-muted-foreground border-l border-border">›</button>
              <button className="px-1.5 py-0.5 hover:bg-muted text-muted-foreground border-l border-border">»</button>
            </div>
          </div>
        </div>
      )}

      {/* 核心列表视图：交货期订单 & 草稿发货单 (完全还原截图中的表格及内联过滤器) */}
      {(activeTab === 'leadtime' || activeTab === 'draft') && (
        <Card className="border-border/80 shadow-sm overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs text-left border-collapse">
              {/* 表头列名 */}
              <thead>
                <tr className="border-b border-border bg-muted/40 text-muted-foreground font-medium">
                  <th className="p-2.5 w-10 text-center"></th>
                  <th className="p-2.5 min-w-[170px]">
                    <div className="flex items-center gap-1">
                      <span>订单日期</span>
                      <button
                        onClick={() => setDateSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))}
                        className="text-primary hover:underline flex items-center font-normal text-[11px]"
                      >
                        {dateSortDir === 'desc' ? '↓ 最新在上' : '↑ 最旧在上'}
                      </button>
                    </div>
                  </th>
                  <th className="p-2.5 min-w-[110px]">到期日期</th>
                  <th className="p-2.5 w-16 text-center">主图</th>
                  <th className="p-2.5 min-w-[200px]">商品标题</th>
                  <th className="p-2.5 min-w-[80px]">商品价格</th>
                  <th className="p-2.5 min-w-[130px] text-center">实重/体积重</th>
                  <th className="p-2.5 min-w-[150px]">SKU</th>
                  <th className="p-2.5 min-w-[100px]">TSIN</th>
                  <th className="p-2.5 min-w-[80px]">仓库楼栋</th>
                  <th className="p-2.5 min-w-[90px] text-center">提前库存</th>
                  <th className="p-2.5 min-w-[70px] text-center">需求数量</th>
                  <th className="p-2.5 min-w-[90px] text-center">发货数量</th>
                </tr>

                {/* 表头内联过滤器行 (对标截图第二行输入框) */}
                <tr className="border-b border-border/80 bg-muted/20">
                  <td className="p-1.5 text-center">
                    <input
                      type="checkbox"
                      onChange={(e) => handleSelectAll(e.target.checked)}
                      checked={selectedIds.length > 0 && selectedIds.length === displayItems.length}
                      className="rounded border-input text-primary focus:ring-primary"
                    />
                  </td>
                  <td className="p-1.5">
                    <select
                      value={filterType}
                      onChange={(e) => setFilterType(e.target.value)}
                      className="w-full h-7 px-2 border border-input rounded bg-background text-foreground text-xs"
                    >
                      <option value="Nor">Nor</option>
                      <option value="All">全部</option>
                    </select>
                  </td>
                  <td className="p-1.5">
                    <select
                      value={filterDueDate}
                      onChange={(e) => setFilterDueDate(e.target.value)}
                      className="w-full h-7 px-2 border border-input rounded bg-background text-foreground text-xs"
                    >
                      <option value="all">全部</option>
                      <option value="overdue">已逾期</option>
                    </select>
                  </td>
                  <td className="p-1.5"></td>
                  <td className="p-1.5">
                    <input
                      type="text"
                      placeholder="搜索标题..."
                      value={filterTitle}
                      onChange={(e) => setFilterTitle(e.target.value)}
                      className="w-full h-7 px-2 border border-input rounded bg-background text-foreground text-xs"
                    />
                  </td>
                  <td className="p-1.5"></td>
                  <td className="p-1.5"></td>
                  <td className="p-1.5">
                    <input
                      type="text"
                      placeholder="搜索SKU..."
                      value={filterSKU}
                      onChange={(e) => setFilterSKU(e.target.value)}
                      className="w-full h-7 px-2 border border-input rounded bg-background text-foreground text-xs"
                    />
                  </td>
                  <td className="p-1.5">
                    <input
                      type="text"
                      placeholder="搜索TSIN..."
                      value={filterTSIN}
                      onChange={(e) => setFilterTSIN(e.target.value)}
                      className="w-full h-7 px-2 border border-input rounded bg-background text-foreground text-xs"
                    />
                  </td>
                  <td className="p-1.5">
                    <select
                      value={filterDC}
                      onChange={(e) => setFilterDC(e.target.value)}
                      className="w-full h-7 px-1 border border-input rounded bg-background text-foreground text-xs"
                    >
                      <option value="All">All</option>
                      <option value="JHB">JHB</option>
                      <option value="CPT">CPT</option>
                      <option value="DUR">DUR</option>
                    </select>
                  </td>
                  <td className="p-1.5"></td>
                  <td className="p-1.5"></td>
                  <td className="p-1.5"></td>
                </tr>
              </thead>

              {/* 表格内容 */}
              <tbody className="divide-y divide-border/60">
                {displayItems.length === 0 ? (
                  <tr>
                    <td colSpan={13} className="py-12 text-center text-muted-foreground text-xs">
                      {loading ? '正在同步订单与发货单...' : '暂无匹配的订单记录'}
                    </td>
                  </tr>
                ) : (
                  displayItems.map((item, index) => {
                    const rowId = index + 1
                    const isSelected = selectedIds.includes(rowId)

                    return (
                      <tr key={index} className={`hover:bg-muted/30 transition-colors ${isSelected ? 'bg-primary/5' : ''}`}>
                        {/* 勾选框 */}
                        <td className="p-2.5 text-center">
                          <input
                            type="checkbox"
                            checked={isSelected}
                            onChange={() => handleToggleSelect(rowId)}
                            className="rounded border-input text-primary focus:ring-primary"
                          />
                        </td>

                        {/* 订单日期与倒计时徽章 (对标截图) */}
                        <td className="p-2.5 font-mono text-xs">
                          <div className="font-medium text-foreground">{item.order_date || '16 Sep 2026 22:30:48'}</div>
                          {/* 精确实时倒计时徽章 (每秒跳动) */}
                          <div className="mt-1">
                            {(() => {
                              const lItem = item as LeadtimeOrderItem
                              if (lItem.remaining_seconds !== undefined) {
                                const elapsed = Math.floor((currentTick - loadedAt) / 1000)
                                const dynamicDiff = lItem.remaining_seconds - elapsed
                                const { text, isOverdue } = formatDetailedCountdown(dynamicDiff)
                                return (
                                  <span
                                    className={`inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold border shadow-2xs ${
                                      isOverdue
                                        ? 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-300/70 dark:border-rose-700/60 animate-pulse'
                                        : 'bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border-amber-300/70 dark:border-amber-700/60'
                                    }`}
                                  >
                                    {text}
                                  </span>
                                )
                              }
                              return (
                                <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-semibold bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border border-amber-300/70 dark:border-amber-700/60 shadow-2xs">
                                  {lItem.countdown_str || '剩余 1天21小时36分50秒'}
                                </span>
                              )
                            })()}
                          </div>
                        </td>

                        {/* 到期日期 */}
                        <td className="p-2.5 font-medium text-foreground">
                          {item.due_date || '07 Oct 2026'}
                        </td>

                        {/* 主图 */}
                        <td className="p-2.5 text-center">
                          <div className="h-12 w-12 mx-auto rounded border border-border bg-muted/20 flex items-center justify-center overflow-hidden shadow-2xs relative">
                            {item.image_url ? (
                              <>
                                <img
                                  src={item.image_url}
                                  alt="Thumb"
                                  loading="lazy"
                                  className="h-full w-full object-contain p-0.5"
                                  onError={(e) => {
                                    e.currentTarget.onerror = null
                                    e.currentTarget.style.display = 'none'
                                    const fallback = e.currentTarget.nextElementSibling as HTMLElement | null
                                    if (fallback) fallback.style.display = 'flex'
                                  }}
                                />
                                <div className="fallback-placeholder hidden items-center justify-center h-full w-full bg-muted/40">
                                  <Package className="h-5 w-5 text-muted-foreground/60" />
                                </div>
                              </>
                            ) : (
                              <Package className="h-5 w-5 text-muted-foreground/60" />
                            )}
                          </div>
                        </td>

                        {/* 商品标题 */}
                        <td className="p-2.5">
                          <a
                            href={`https://www.takealot.com/x/PLID${item.tsin}`}
                            target="_blank"
                            rel="noreferrer"
                            className="text-primary hover:underline font-semibold line-clamp-2 leading-tight"
                          >
                            {item.title}
                          </a>
                        </td>

                        {/* 商品价格 */}
                        <td className="p-2.5 font-semibold text-foreground font-mono">
                          R{item.selling_price}
                        </td>

                        {/* 实重/体积重与改重/改价按钮 (对标截图) */}
                        <td className="p-2.5 text-center space-y-1">
                          <div>
                            <span className="inline-block px-1.5 py-0.2 rounded text-[10px] font-medium border border-amber-400/80 bg-amber-50 dark:bg-amber-950/50 text-amber-600 dark:text-amber-400">
                              {item.weigh_status === 'done' ? '已称重' : '待称重'}
                            </span>
                          </div>
                          <div className="flex items-center justify-center gap-1">
                            <button
                              onClick={() =>
                                setWeighModal({
                                  open: true,
                                  item,
                                  actualWeight: item.actual_weight || 0.28,
                                  volumetricWeight: item.volumetric_weight || 0.35,
                                })
                              }
                              className="px-1.5 py-0.5 rounded border border-sky-400 dark:border-sky-600 text-sky-600 dark:text-sky-400 hover:bg-sky-50 dark:hover:bg-sky-950 text-[11px]"
                            >
                              改重
                            </button>
                            <button
                              onClick={() =>
                                setPriceModal({
                                  open: true,
                                  item,
                                  sellingPrice: item.selling_price,
                                  rrp: Math.round(item.selling_price * 1.2),
                                })
                              }
                              className="px-1.5 py-0.5 rounded border border-sky-400 dark:border-sky-600 text-sky-600 dark:text-sky-400 hover:bg-sky-50 dark:hover:bg-sky-950 text-[11px]"
                            >
                              改价
                            </button>
                          </div>
                        </td>

                        {/* SKU 及店铺 */}
                        <td className="p-2.5">
                          <div className="font-bold font-mono text-foreground">{item.sku}</div>
                          <div className="text-[11px] text-muted-foreground mt-0.5">
                            {(item as LeadtimeOrderItem).store_name || 'Longyu Trading(29902872)'}
                          </div>
                        </td>

                        {/* TSIN */}
                        <td className="p-2.5 font-mono text-muted-foreground">{item.tsin}</td>

                        {/* 仓库楼栋 */}
                        <td className="p-2.5">
                          <Badge variant="outline" className="font-mono text-xs">
                            {item.dc || 'JHB'}
                          </Badge>
                        </td>

                        {/* 提前库存 (步进器) */}
                        <td className="p-2.5 text-center">
                          <input
                            type="number"
                            defaultValue={item.leadtime_stock || 554}
                            onBlur={(e) => {
                              if ('id' in item && item.id) {
                                handleUpdateItemQty(item.id, item.ship_qty || 1, parseInt(e.target.value) || 0)
                              }
                            }}
                            className="w-16 h-7 px-1 text-center border border-input rounded bg-background text-foreground font-mono text-xs"
                          />
                        </td>

                        {/* 需求数量 */}
                        <td className="p-2.5 text-center font-mono font-medium text-foreground">
                          {item.demand_qty || 1}
                        </td>

                        {/* 发货数量 (步进器) */}
                        <td className="p-2.5 text-center">
                          <input
                            type="number"
                            defaultValue={item.ship_qty || 1}
                            onBlur={(e) => {
                              if ('id' in item && item.id) {
                                handleUpdateItemQty(item.id, parseInt(e.target.value) || 1, item.leadtime_stock || 0)
                              }
                            }}
                            className="w-14 h-7 px-1 text-center border border-input rounded bg-background text-foreground font-mono font-bold text-xs"
                          />
                        </td>
                      </tr>
                    )
                  })
                )}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {/* 已确认发货单 Tab */}
      {activeTab === 'confirmed' && (
        <div className="space-y-4">
          {shipments.filter((s) => s.status === 'confirmed').length === 0 ? (
            <Card className="py-12 text-center text-muted-foreground text-xs">
              暂无已确认的发货单，您可以在【草稿发货单】中整理完毕后点击“确认发货单”！
            </Card>
          ) : (
            shipments
              .filter((s) => s.status === 'confirmed')
              .map((sh) => (
                <Card key={sh.id} className="border-border p-4 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <Package className="h-5 w-5 text-primary" />
                      <div>
                        <div className="font-semibold text-foreground text-sm flex items-center gap-2">
                          <span>{sh.shipment_number}</span>
                          <Badge variant="outline" className="bg-emerald-50 text-emerald-700 border-emerald-300">
                            已确认待送仓
                          </Badge>
                          <Badge variant="secondary">{sh.destination_dc} 仓库</Badge>
                        </div>
                        <div className="text-xs text-muted-foreground mt-0.5">
                          总品类: {sh.total_items} 种 · 总数量: {sh.total_units} 件 · 预估货值: R{sh.total_value} · 创建时间: {sh.created_at}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setBookingModal((prev) => ({
                            ...prev,
                            open: true,
                            shipmentId: sh.id,
                            dc: sh.destination_dc,
                          }))
                        }}
                        className="h-8 text-xs gap-1"
                      >
                        <CalendarDays className="h-3.5 w-3.5" />
                        <span>去预约送仓</span>
                      </Button>
                      <Button
                        size="sm"
                        onClick={() => handleMarkShipped(sh.id)}
                        className="h-8 text-xs gap-1 bg-emerald-600 hover:bg-emerald-700 text-white"
                      >
                        <Truck className="h-3.5 w-3.5" />
                        <span>标记已发货出库</span>
                      </Button>
                    </div>
                  </div>
                </Card>
              ))
          )}
        </div>
      )}

      {/* 已发货单 Tab */}
      {activeTab === 'shipped' && (
        <div className="space-y-4">
          {shipments.filter((s) => s.status === 'shipped').length === 0 ? (
            <Card className="py-12 text-center text-muted-foreground text-xs">
              暂无在途发货单，出库后可在【已确认发货单】中标记在途发货。
            </Card>
          ) : (
            shipments
              .filter((s) => s.status === 'shipped')
              .map((sh) => (
                <Card key={sh.id} className="border-border p-4 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <Truck className="h-5 w-5 text-blue-500" />
                      <div>
                        <div className="font-semibold text-foreground text-sm flex items-center gap-2">
                          <span>{sh.shipment_number}</span>
                          <Badge variant="outline" className="bg-blue-50 text-blue-700 border-blue-300">
                            在途运往 DC 仓库
                          </Badge>
                          <Badge variant="secondary">{sh.destination_dc} 仓库</Badge>
                        </div>
                        <div className="text-xs text-muted-foreground mt-0.5">
                          总数量: {sh.total_units} 件 · 预估货值: R{sh.total_value} · 发货时间: {sh.updated_at}
                        </div>
                      </div>
                    </div>
                    <Badge variant="outline" className="text-xs">
                      等待 Takealot 验收上架
                    </Badge>
                  </div>
                </Card>
              ))
          )}
        </div>
      )}

      {/* 预约管理 Tab */}
      {activeTab === 'bookings' && (
        <Card className="border-border/80 shadow-sm overflow-hidden">
          <Table>
            <TableHeader className="bg-muted/40">
              <TableRow>
                <TableHead>预约单号</TableHead>
                <TableHead>关联发货单</TableHead>
                <TableHead>送达仓库</TableHead>
                <TableHead>送仓日期 & 时间窗口</TableHead>
                <TableHead>承运物流 / 车牌</TableHead>
                <TableHead className="text-center">状态</TableHead>
                <TableHead>备注信息</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {bookings.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="py-12 text-center text-muted-foreground text-xs">
                    暂无送仓预约记录，点击右上角“新建送仓预约”进行登记。
                  </TableCell>
                </TableRow>
              ) : (
                bookings.map((b) => (
                  <TableRow key={b.id} className="hover:bg-muted/30">
                    <TableCell className="font-mono font-semibold text-xs text-primary">{b.booking_number}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{b.shipment_no || '常规多批次'}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{b.destination_dc} 仓</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      <div className="font-medium">{b.booking_date}</div>
                      <div className="text-[11px] text-muted-foreground">{b.time_slot}</div>
                    </TableCell>
                    <TableCell className="text-xs">
                      <div className="font-medium text-foreground">{b.carrier_name}</div>
                      <div className="font-mono text-[11px] text-muted-foreground">{b.vehicle_reg}</div>
                    </TableCell>
                    <TableCell className="text-center">
                      <Badge
                        variant="outline"
                        className={
                          b.status === 'completed'
                            ? 'bg-emerald-50 text-emerald-700 border-emerald-300'
                            : 'bg-sky-50 text-sky-700 border-sky-300'
                        }
                      >
                        {b.status === 'completed' ? '已收货' : '预约中'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{b.notes || '-'}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={async () => {
                          if (confirm('确认删除或取消该预约吗？')) {
                            await api.deleteBooking(b.id)
                            loadBookings()
                          }
                        }}
                        className="h-6 text-xs text-destructive hover:bg-destructive/10"
                      >
                        取消
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* 财务流水明细 (保留原系统能力) */}
      {activeTab === 'history' && (
        <Card className="overflow-hidden border-border/80">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader className="bg-muted/30">
                <TableRow>
                  <TableHead>订单编号 / 时间</TableHead>
                  <TableHead className="min-w-[200px]">成交商品</TableHead>
                  <TableHead className="text-center">状态</TableHead>
                  <TableHead className="text-center">成交售价</TableHead>
                  <TableHead className="text-center">官方佣金 & 扣费</TableHead>
                  <TableHead className="text-center font-semibold text-foreground">预计到手净额</TableHead>
                  <TableHead className="text-center">买家 / 发货仓</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sales.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="py-12 text-center text-muted-foreground text-xs">
                      {loading ? '正在载入销售流水...' : '选定时间段内无销售记录'}
                    </TableCell>
                  </TableRow>
                ) : (
                  sales.map((item) => {
                    const totalFees =
                      (item.success_fee || 0) + (item.fulfillment_fee || 0) + (item.courier_collection_fee || 0)
                    const net = item.selling_price - totalFees
                    return (
                      <TableRow key={item.order_item_id} className="hover:bg-muted/30 transition-colors">
                        <TableCell className="font-mono text-xs">
                          <div className="font-semibold text-foreground">#{item.order_id}</div>
                          <div className="text-[11px] text-muted-foreground">{item.order_date}</div>
                        </TableCell>
                        <TableCell>
                          <div className="font-medium text-foreground line-clamp-2 text-xs">{item.product_title}</div>
                          <div className="text-[10px] text-muted-foreground font-mono mt-0.5">
                            TSIN: {item.tsin} · SKU: {item.sku}
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          <Badge variant="outline" className="text-[11px]">
                            {item.sale_status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-center font-mono font-semibold text-xs">
                          {formatCurrency(item.selling_price)}
                        </TableCell>
                        <TableCell className="text-center text-xs font-mono text-muted-foreground">
                          -{formatCurrency(totalFees)}
                        </TableCell>
                        <TableCell className="text-center font-mono font-bold text-xs text-emerald-600 dark:text-emerald-400">
                          {formatCurrency(net)}
                        </TableCell>
                        <TableCell className="text-center text-xs">
                          <div>{item.customer || 'Takealot买家'}</div>
                          <Badge variant="secondary" className="text-[10px] mt-0.5">
                            {item.dc || 'JHB'}
                          </Badge>
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

      {/* 已开具发票 (保留原系统能力) */}
      {activeTab === 'orders' && (
        <Card className="overflow-hidden border-border/80">
          <Table>
            <TableHeader className="bg-muted/30">
              <TableRow>
                <TableHead>订单单号</TableHead>
                <TableHead>授权扣款日期</TableHead>
                <TableHead>包含商品项</TableHead>
                <TableHead className="text-right">税务发票凭据</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {orders.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="py-12 text-center text-muted-foreground text-xs">
                    {loading ? '正在载入订单...' : '暂无订单记录'}
                  </TableCell>
                </TableRow>
              ) : (
                orders.map((ord) => (
                  <TableRow key={ord.order_id} className="hover:bg-muted/30">
                    <TableCell className="font-mono font-semibold text-xs">#{ord.order_id}</TableCell>
                    <TableCell className="font-mono text-xs">{ord.date_authed || '-'}</TableCell>
                    <TableCell className="text-xs">
                      {ord.order_items?.map((it) => (
                        <div key={it.order_item_id} className="line-clamp-1 text-muted-foreground">
                          • {it.title} (SKU: {it.merchant_sku})
                        </div>
                      ))}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleViewInvoice(ord.order_id)}
                        className="h-7 text-xs gap-1.5"
                      >
                        <FileText className="h-3.5 w-3.5" />
                        <span>下载发票</span>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* 快捷【改重】弹窗 */}
      <Dialog
        open={weighModal.open}
        onOpenChange={(open) => setWeighModal((prev) => ({ ...prev, open }))}
        title="修改商品称重与体积参数"
      >
        <div className="space-y-4 py-2 text-xs">
          <div className="p-2.5 rounded bg-muted/50 border border-border">
            <div className="font-semibold text-foreground">{weighModal.item?.title}</div>
            <div className="text-muted-foreground font-mono text-[11px] mt-0.5">
              TSIN: {weighModal.item?.tsin} · SKU: {weighModal.item?.sku}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="font-medium text-foreground">实际称重 (kg)</label>
              <input
                type="number"
                step="0.01"
                value={weighModal.actualWeight}
                onChange={(e) => setWeighModal((prev) => ({ ...prev, actualWeight: parseFloat(e.target.value) || 0 }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <label className="font-medium text-foreground">体积重 (kg)</label>
              <input
                type="number"
                step="0.01"
                value={weighModal.volumetricWeight}
                onChange={(e) =>
                  setWeighModal((prev) => ({ ...prev, volumetricWeight: parseFloat(e.target.value) || 0 }))
                }
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono"
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setWeighModal((prev) => ({ ...prev, open: false }))}>
              取消
            </Button>
            <Button size="sm" onClick={handleSaveWeight}>
              保存并同步
            </Button>
          </div>
        </div>
      </Dialog>

      {/* 快捷【改价】弹窗 */}
      <Dialog
        open={priceModal.open}
        onOpenChange={(open) => setPriceModal((prev) => ({ ...prev, open }))}
        title="快速修改发货商品售价"
      >
        <div className="space-y-4 py-2 text-xs">
          <div className="p-2.5 rounded bg-muted/50 border border-border">
            <div className="font-semibold text-foreground">{priceModal.item?.title}</div>
            <div className="text-muted-foreground font-mono text-[11px] mt-0.5">
              TSIN: {priceModal.item?.tsin} · SKU: {priceModal.item?.sku}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="font-medium text-foreground">实际售价 (ZAR 兰特)</label>
              <input
                type="number"
                value={priceModal.sellingPrice}
                onChange={(e) => setPriceModal((prev) => ({ ...prev, sellingPrice: parseInt(e.target.value) || 0 }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono text-sm font-semibold"
              />
            </div>
            <div className="space-y-1.5">
              <label className="font-medium text-foreground">建议零售价 RRP (ZAR)</label>
              <input
                type="number"
                value={priceModal.rrp}
                onChange={(e) => setPriceModal((prev) => ({ ...prev, rrp: parseInt(e.target.value) || 0 }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono"
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setPriceModal((prev) => ({ ...prev, open: false }))}>
              取消
            </Button>
            <Button size="sm" onClick={handleSavePrice}>
              提交修改
            </Button>
          </div>
        </div>
      </Dialog>

      {/* 新建【送仓预约】弹窗 */}
      <Dialog
        open={bookingModal.open}
        onOpenChange={(open) => setBookingModal((prev) => ({ ...prev, open }))}
        title="新建 Takealot DC 送仓预约"
      >
        <div className="space-y-3 py-2 text-xs">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <label className="font-medium text-foreground">送达 DC 仓库</label>
              <select
                value={bookingModal.dc}
                onChange={(e) => setBookingModal((prev) => ({ ...prev, dc: e.target.value }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground text-xs"
              >
                <option value="JHB">JHB (约堡主仓)</option>
                <option value="CPT">CPT (开普敦仓)</option>
                <option value="DUR">DUR (德班中转仓)</option>
              </select>
            </div>
            <div className="space-y-1">
              <label className="font-medium text-foreground">送仓日期</label>
              <input
                type="date"
                value={bookingModal.bookingDate}
                onChange={(e) => setBookingModal((prev) => ({ ...prev, bookingDate: e.target.value }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono text-xs"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <label className="font-medium text-foreground">预约时间槽 (Time Slot)</label>
              <select
                value={bookingModal.timeSlot}
                onChange={(e) => setBookingModal((prev) => ({ ...prev, timeSlot: e.target.value }))}
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground text-xs"
              >
                <option value="08:00 - 10:00">08:00 - 10:00 (早间首批)</option>
                <option value="10:00 - 12:00">10:00 - 12:00 (上午推荐)</option>
                <option value="13:00 - 15:00">13:00 - 15:00 (下午班次)</option>
                <option value="15:00 - 17:00">15:00 - 17:00 (傍晚截止)</option>
              </select>
            </div>
            <div className="space-y-1">
              <label className="font-medium text-foreground">承运快递 / 车队</label>
              <input
                type="text"
                value={bookingModal.carrier}
                onChange={(e) => setBookingModal((prev) => ({ ...prev, carrier: e.target.value }))}
                placeholder="例如 Courier Guy / 卖家自送"
                className="w-full h-8 px-2 border border-input rounded bg-background text-foreground text-xs"
              />
            </div>
          </div>

          <div className="space-y-1">
            <label className="font-medium text-foreground">车牌号码 (Vehicle Reg)</label>
            <input
              type="text"
              value={bookingModal.vehicleReg}
              onChange={(e) => setBookingModal((prev) => ({ ...prev, vehicleReg: e.target.value }))}
              placeholder="例如 GP 882-901"
              className="w-full h-8 px-2 border border-input rounded bg-background text-foreground font-mono text-xs"
            />
          </div>

          <div className="space-y-1">
            <label className="font-medium text-foreground">预约备注说明</label>
            <input
              type="text"
              value={bookingModal.notes}
              onChange={(e) => setBookingModal((prev) => ({ ...prev, notes: e.target.value }))}
              placeholder="如：带托盘装卸、散箱打包等"
              className="w-full h-8 px-2 border border-input rounded bg-background text-foreground text-xs"
            />
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setBookingModal((prev) => ({ ...prev, open: false }))}>
              取消
            </Button>
            <Button size="sm" onClick={handleCreateBookingSubmit}>
              确认创建预约
            </Button>
          </div>
        </div>
      </Dialog>

      {/* 发票查看弹窗 */}
      <Dialog
        open={invoiceModal.open}
        onOpenChange={(open) => setInvoiceModal((prev) => ({ ...prev, open }))}
        title={`订单 #${invoiceModal.orderId} 客户税务发票`}
      >
        <div className="space-y-3 py-2 text-xs">
          {invoiceModal.loading ? (
            <div className="py-6 text-center text-muted-foreground">正在查询 Takealot 官方发票系统...</div>
          ) : invoiceModal.invoices.length === 0 ? (
            <div className="py-6 text-center text-muted-foreground">该订单目前暂无可下载的税务发票。</div>
          ) : (
            invoiceModal.invoices.map((inv) => (
              <div
                key={inv.document_id}
                className="flex items-center justify-between p-3 rounded-lg border border-border bg-muted/20"
              >
                <div>
                  <div className="font-semibold text-foreground">{inv.file_name}</div>
                  <div className="text-[11px] text-muted-foreground mt-0.5">
                    类型: {inv.document_type} · 日期: {inv.document_date}
                  </div>
                </div>
                {inv.downloadable && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      window.open(
                        `https://seller-api.takealot.com/v1/sales/orders/${invoiceModal.orderId}/customer_invoices/${inv.document_id}`,
                        '_blank'
                      )
                    }
                    className="h-7 text-xs gap-1"
                  >
                    <Download className="h-3.5 w-3.5" />
                    <span>下载 PDF</span>
                  </Button>
                )}
              </div>
            ))
          )}
        </div>
      </Dialog>
    </div>
  )
}

import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { SaleItem, OrderRecord, InvoiceDocument } from '../../types'
import { Card, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Dialog } from '../ui/dialog'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { formatCurrency, formatDateTime } from '../../lib/utils'
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
} from 'lucide-react'

export const SalesTab: React.FC = () => {
  const [activeSubTab, setActiveSubTab] = useState<'sales' | 'orders'>('sales')
  const [loading, setLoading] = useState(false)

  // Sales Items state
  const [sales, setSales] = useState<SaleItem[]>([])
  const [salesTotal, setSalesTotal] = useState(0)
  const [salesPage, setSalesPage] = useState(1)

  // Orders state
  const [orders, setOrders] = useState<OrderRecord[]>([])
  const [ordersTotal, setOrdersTotal] = useState(0)
  const [ordersPage, setOrdersPage] = useState(1)

  // Date filters
  const [daysRange, setDaysRange] = useState<number>(30)

  // Invoice modal
  const [invoiceModal, setInvoiceModal] = useState<{
    open: boolean
    orderId?: number
    invoices: InvoiceDocument[]
    loading?: boolean
  }>({ open: false, invoices: [] })

  const getDates = (days: number) => {
    const end = new Date().toISOString().split('T')[0]
    const startObj = new Date()
    startObj.setDate(startObj.getDate() - days)
    const start = startObj.toISOString().split('T')[0]
    return { start, end }
  }

  const loadSales = async (page = salesPage) => {
    setLoading(true)
    try {
      const { start, end } = getDates(daysRange)
      const res = await api.getOfficialSales(page, 50, start, end)
      setSales(res.sales || [])
      setSalesTotal(res.page_summary?.total || 0)
      setSalesPage(res.page_summary?.page_number || page)
    } catch (err: any) {
      alert(`获取销售明细失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const loadOrders = async (page = ordersPage) => {
    setLoading(true)
    try {
      const { start, end } = getDates(daysRange)
      const res = await api.getOfficialSalesOrders(start, end, page, 50)
      setOrders(res.orders || [])
      setOrdersTotal(res.page_summary?.total || 0)
      setOrdersPage(res.page_summary?.page_number || page)
    } catch (err: any) {
      alert(`获取订单列表失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (activeSubTab === 'sales') {
      loadSales(1)
    } else {
      loadOrders(1)
    }
  }, [activeSubTab, daysRange])

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

  // Calculate Net Revenue for a sale item
  const calculateNet = (item: SaleItem) => {
    const totalFees = (item.success_fee || 0) + (item.fulfillment_fee || 0) + (item.courier_collection_fee || 0)
    return item.selling_price - totalFees
  }

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
            销售明细与交易订单 (Sales & Orders)
          </h2>
          <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
            对接 Takealot 官方 <code>/v2/sales</code> 与 <code>/v1/sales/orders</code> · 成交流水 · 佣金扣费 · 发票凭据
          </p>
        </div>

        <div className="flex items-center gap-2.5">
          {/* Days Filter using shadcn Tabs */}
          <Tabs value={String(daysRange)} onValueChange={(val) => setDaysRange(parseInt(val))}>
            <TabsList className="h-8 p-0.5">
              <TabsTrigger value="7" className="text-xs h-7 px-2.5">近 7 天</TabsTrigger>
              <TabsTrigger value="30" className="text-xs h-7 px-2.5">近 30 天</TabsTrigger>
              <TabsTrigger value="90" className="text-xs h-7 px-2.5">近 90 天</TabsTrigger>
            </TabsList>
          </Tabs>

          <Button
            variant="outline"
            size="sm"
            onClick={() => (activeSubTab === 'sales' ? loadSales() : loadOrders())}
            loading={loading}
            className="gap-1.5 h-8 text-xs"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            <span>刷新</span>
          </Button>
        </div>
      </div>

      {/* Sub Tabs using standard shadcn Tabs */}
      <Tabs value={activeSubTab} onValueChange={(v) => setActiveSubTab(v as any)}>
        <TabsList className="grid w-full sm:w-[380px] grid-cols-2">
          <TabsTrigger value="sales" className="text-xs">
            销售流水明细 (Sales Items)
          </TabsTrigger>
          <TabsTrigger value="orders" className="text-xs">
            订单包裹管理 (Orders)
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {/* Content for Sales Items */}
      {activeSubTab === 'sales' && (
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
                    <TableCell colSpan={7} className="py-12 text-center text-muted-foreground">
                      {loading ? '正在载入销售流水...' : '选定时间段内无销售记录'}
                    </TableCell>
                  </TableRow>
                ) : (
                  sales.map((item) => {
                    const net = calculateNet(item)
                    const totalFees =
                      (item.success_fee || 0) + (item.fulfillment_fee || 0) + (item.courier_collection_fee || 0)
                    return (
                      <TableRow key={item.order_item_id} className="hover:bg-muted/30 transition-colors">
                        <TableCell className="font-mono text-xs">
                          <div className="font-semibold text-foreground">#{item.order_id}</div>
                          <div className="text-[11px] text-muted-foreground">{item.order_date}</div>
                        </TableCell>

                        <TableCell>
                          <div className="font-medium text-foreground line-clamp-2 text-xs">
                            {item.product_title || '未知商品'}
                          </div>
                          <div className="text-[10px] text-muted-foreground font-mono mt-0.5">
                            TSIN: {item.tsin} · SKU: {item.sku}
                          </div>
                        </TableCell>

                        <TableCell className="text-center">
                          <Badge variant="success" dot className="text-[11px]">
                            {item.sale_status}
                          </Badge>
                        </TableCell>

                        <TableCell className="text-center font-mono font-bold text-foreground text-sm">
                          {formatCurrency(item.selling_price)}
                        </TableCell>

                        <TableCell className="text-center font-mono text-xs text-muted-foreground">
                          <div title={`平台佣金: ${formatCurrency(item.success_fee)} | 履约费: ${formatCurrency(item.fulfillment_fee)} | 揽收费: ${formatCurrency(item.courier_collection_fee)}`}>
                            -{formatCurrency(totalFees)}
                          </div>
                          <div className="text-[10px] text-muted-foreground">
                            平台抽成: {item.selling_price > 0 ? Math.round((totalFees / item.selling_price) * 100) : 0}%
                          </div>
                        </TableCell>

                        <TableCell className="text-center font-mono font-bold text-sm text-emerald-600 dark:text-emerald-400">
                          {formatCurrency(net)}
                        </TableCell>

                        <TableCell className="text-center text-xs">
                          <div className="font-medium text-foreground">{item.customer || '客户'}</div>
                          <div className="text-[11px] text-muted-foreground mt-0.5">发货仓: {item.dc || 'TAL'}</div>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between p-4 border-t border-border/80 text-xs text-muted-foreground">
            <div>
              共计 <span className="font-semibold text-foreground font-mono">{salesTotal}</span> 笔销售记录
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={salesPage <= 1 || loading}
                onClick={() => loadSales(salesPage - 1)}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                <span>上一页</span>
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={sales.length < 50 || loading}
                onClick={() => loadSales(salesPage + 1)}
              >
                <span>下一页</span>
                <ChevronRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* Content for Orders */}
      {activeSubTab === 'orders' && (
        <Card className="overflow-hidden border-border/80">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader className="bg-muted/30">
                <TableRow>
                  <TableHead className="w-32">官方订单号</TableHead>
                  <TableHead>授权付款时间</TableHead>
                  <TableHead className="min-w-[260px]">订单内包裹商品</TableHead>
                  <TableHead className="text-center w-28">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {orders.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4} className="py-12 text-center text-muted-foreground">
                      {loading ? '正在载入订单列表...' : '选定时间段内无订单记录'}
                    </TableCell>
                  </TableRow>
                ) : (
                  orders.map((order) => (
                    <TableRow key={order.order_id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="font-mono font-bold text-foreground text-sm">
                        #{order.order_id}
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {order.date_authed}
                      </TableCell>
                      <TableCell>
                        <div className="space-y-1.5">
                          {order.order_items?.map((item) => (
                            <div key={item.order_item_id} className="flex items-center gap-2">
                              <span className="w-1.5 h-1.5 rounded-full bg-blue-500" />
                              <span className="text-xs font-medium text-foreground line-clamp-1">
                                {item.title}
                              </span>
                              <span className="text-[10px] text-muted-foreground font-mono">
                                (TSIN: {item.tsin_id})
                              </span>
                            </div>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="text-center">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleViewInvoice(order.order_id)}
                          className="h-8 text-xs gap-1"
                        >
                          <Receipt className="h-3.5 w-3.5 text-primary" />
                          <span>查看发票</span>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between p-4 border-t border-border/80 text-xs text-muted-foreground">
            <div>
              共计 <span className="font-semibold text-foreground font-mono">{ordersTotal}</span> 个订单
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={ordersPage <= 1 || loading}
                onClick={() => loadOrders(ordersPage - 1)}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                <span>上一页</span>
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={orders.length < 50 || loading}
                onClick={() => loadOrders(ordersPage + 1)}
              >
                <span>下一页</span>
                <ChevronRight className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* Invoice Modal */}
      <Dialog
        open={invoiceModal.open}
        onClose={() => setInvoiceModal({ open: false, invoices: [] })}
        title={`官方发票文档 · 订单 #${invoiceModal.orderId}`}
        description="Takealot 系统自动生成的客户交易税务发票"
      >
        <div className="space-y-4">
          {invoiceModal.loading ? (
            <div className="py-8 text-center text-xs text-muted-foreground">
              正在获取发票数据...
            </div>
          ) : invoiceModal.invoices.length === 0 ? (
            <div className="py-8 text-center text-xs text-muted-foreground">
              此订单暂未生成发票文档
            </div>
          ) : (
            <div className="space-y-2">
              {invoiceModal.invoices.map((doc) => (
                <div
                  key={doc.document_id}
                  className="flex items-center justify-between p-3 rounded-xl bg-muted/40 border border-border"
                >
                  <div className="space-y-0.5">
                    <div className="flex items-center gap-2">
                      <FileText className="h-4 w-4 text-primary" />
                      <span className="text-xs font-semibold text-foreground">
                        {doc.document_type} #{doc.document_id}
                      </span>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                      日期: {doc.document_date} · 业务: {doc.document_reason}
                    </p>
                  </div>
                  <Badge variant="secondary">可开具凭证</Badge>
                </div>
              ))}
            </div>
          )}

          <div className="flex justify-end pt-2">
            <Button variant="outline" size="sm" onClick={() => setInvoiceModal({ open: false, invoices: [] })}>
              关闭
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}

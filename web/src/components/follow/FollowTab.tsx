import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { FollowItem, BatchJobRecord, BatchStatusResponse } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Input } from '../ui/input'
import { Dialog } from '../ui/dialog'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { toast } from '../ui/use-toast'
import {
  UploadCloud,
  FileSpreadsheet,
  CheckCircle2,
  AlertCircle,
  Play,
  Download,
  Trash2,
  ExternalLink,
  Layers,
  RefreshCw,
  Clock,
  ListOrdered,
  Eye,
  Send,
} from 'lucide-react'

interface FollowTabProps {
  onNavigateLogs: () => void
}

export const FollowTab: React.FC<FollowTabProps> = ({ onNavigateLogs }) => {
  const [activeTab, setActiveTab] = useState('excel-follow')

  // --- Excel Follow State ---
  const [items, setItems] = useState<FollowItem[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [dragActive, setDragActive] = useState(false)
  const [fileName, setFileName] = useState('')

  // --- Official Batch Queue State ---
  const [batchJobs, setBatchJobs] = useState<BatchJobRecord[]>([])
  const [batchLoading, setBatchLoading] = useState(false)
  const [batchDetailModal, setBatchDetailModal] = useState<{
    open: boolean
    batchId: string
    loading?: boolean
    data?: BatchStatusResponse
    error?: string
  }>({ open: false, batchId: '' })

  const [createBatchModal, setCreateBatchModal] = useState<{
    open: boolean
    jsonContent: string
    submitting?: boolean
  }>({
    open: false,
    jsonContent: JSON.stringify(
      [
        {
          offer_id: 226816605,
          selling_price: 650,
          rrp: 1200,
          leadtime_days: 7,
          status_action: 'Re-enable',
        },
      ],
      null,
      2
    ),
  })

  const loadBatchJobs = async () => {
    setBatchLoading(true)
    try {
      const list = await api.getOfficialBatchList(50)
      setBatchJobs(list || [])
    } catch (e: any) {
      console.error('Failed to load batch jobs:', e)
    } finally {
      setBatchLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'official-batch') {
      loadBatchJobs()
    }
  }, [activeTab])

  const handleFileUpload = async (file: File) => {
    if (!file.name.endsWith('.xlsx') && !file.name.endsWith('.xls')) {
      toast.warning('请上传 Excel 格式文件 (.xlsx)')
      return
    }
    setLoading(true)
    setFileName(file.name)
    try {
      const res = await api.uploadFollowExcel(file)
      if (res.success) {
        setItems(res.items || [])
        toast.success('表格解析成功！', `已读取 ${res.items?.length ?? 0} 条待跟卖记录`)
      } else {
        toast.error('文件解析失败')
      }
    } catch (err: any) {
      toast.error('上传解析失败', err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true)
    } else if (e.type === 'dragleave') {
      setDragActive(false)
    }
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setDragActive(false)
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleFileUpload(e.dataTransfer.files[0])
    }
  }

  const handleStartFollow = async () => {
    if (items.length === 0) return
    if (!confirm(`确认立即对这 ${items.length} 款商品执行批量变体解析与自动上架跟卖吗？`)) return

    setSubmitting(true)
    try {
      const res = await api.startFollowBatch(items)
      if (res.success) {
        toast.success('批量跟卖任务已在后台启动！', '将自动跳转至运行日志查看实时进度')
        onNavigateLogs()
      } else {
        toast.error('启动失败', res.message)
      }
    } catch (err: any) {
      toast.error('提交任务失败', err.message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleInspectBatch = async (batchId: string) => {
    setBatchDetailModal({ open: true, batchId, loading: true })
    try {
      const res = await api.getOfficialBatchStatus(batchId)
      setBatchDetailModal({ open: true, batchId, loading: false, data: res })
      // Refresh list to show updated status
      loadBatchJobs()
    } catch (err: any) {
      setBatchDetailModal({ open: true, batchId, loading: false, error: err.message })
    }
  }

  const handleCreateBatchSubmit = async () => {
    try {
      const parsed = JSON.parse(createBatchModal.jsonContent)
      if (!Array.isArray(parsed)) {
        toast.warning('批处理数据格式必须为 JSON 数组 (Array)')
        return
      }
      setCreateBatchModal((prev) => ({ ...prev, submitting: true }))
      const res = await api.createOfficialBatch(parsed, 'bulk_api_submission')
      toast.success('批处理已提交至官方队列！', `Batch ID: ${res.batch_id}`)
      setCreateBatchModal({ open: false, jsonContent: '' })
      loadBatchJobs()
    } catch (err: any) {
      toast.error('批处理提交失败', err.message)
    } finally {
      setCreateBatchModal((prev) => ({ ...prev, submitting: false }))
    }
  }

  return (
    <div className="space-y-6">
      {/* Top Banner */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
            批量操作与上架中台
          </h2>
          <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
            支持 Excel 表格自动变体穿透跟卖 · 官方异步 Batch 批处理队列并发调价
          </p>
        </div>
      </div>

      {/* Tabs navigation using standard shadcn Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList className="grid w-full sm:w-[400px] grid-cols-2">
          <TabsTrigger value="excel-follow" className="gap-1.5 text-xs">
            <FileSpreadsheet className="h-3.5 w-3.5" />
            <span>Excel 变体跟卖</span>
          </TabsTrigger>
          <TabsTrigger value="official-batch" className="gap-1.5 text-xs">
            <Layers className="h-3.5 w-3.5" />
            <span>官方 Batch 批处理队列</span>
          </TabsTrigger>
        </TabsList>

        {/* Tab 1: Excel Follow Selling */}
        <TabsContent value="excel-follow" className="space-y-5 mt-4">
          {/* Upload Drop Area */}
          <Card>
            <CardContent className="p-6">
              <div
                onDragEnter={handleDrag}
                onDragOver={handleDrag}
                onDragLeave={handleDrag}
                onDrop={handleDrop}
                className={`relative border-2 border-dashed rounded-xl p-8 sm:p-12 text-center transition-all cursor-pointer ${
                  dragActive
                    ? 'border-primary bg-primary/5 scale-[1.005]'
                    : 'border-border/80 hover:border-primary/50 hover:bg-muted/30'
                }`}
              >
                <input
                  type="file"
                  accept=".xlsx,.xls"
                  onChange={(e) => e.target.files?.[0] && handleFileUpload(e.target.files[0])}
                  className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
                  disabled={loading}
                />
                <div className="flex flex-col items-center justify-center space-y-3">
                  <div className="h-14 w-14 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shadow-xs">
                    <UploadCloud className="h-7 w-7" />
                  </div>
                  <div className="space-y-1">
                    <div className="text-sm font-semibold text-foreground">
                      {loading
                        ? '正在解析 Excel 表格与变体链接...'
                        : fileName
                        ? `当前文件: ${fileName}`
                        : '点击或拖拽 Excel 文件到此处上传'}
                    </div>
                    <p className="text-xs text-muted-foreground max-w-sm mx-auto">
                      表格首列为商品链接（例如 https://www.takealot.com/x/PLID...），系统将自动嗅探全部颜色与尺码变体
                    </p>
                  </div>
                  <div className="flex items-center gap-2 pt-2">
                    <Button variant="outline" size="sm" className="gap-1.5 text-xs pointer-events-none">
                      <FileSpreadsheet className="h-3.5 w-3.5" />
                      <span>选择表格 (.xlsx)</span>
                    </Button>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Table Preview */}
          {items.length > 0 && (
            <Card>
              <CardHeader className="p-4 sm:p-5 flex flex-row items-center justify-between border-b border-border">
                <div>
                  <div className="flex items-center gap-2">
                    <CardTitle className="text-base font-bold">待上架任务清单</CardTitle>
                    <Badge variant="secondary" className="font-mono text-xs">
                      已识别 {items.length} 个商品
                    </Badge>
                  </div>
                  <CardDescription className="text-xs">
                    请核对商品链接及预设的初始跟卖库存和保底价格
                  </CardDescription>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setItems([])
                      setFileName('')
                    }}
                    className="gap-1 text-xs text-destructive hover:text-destructive"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    <span>清空</span>
                  </Button>
                  <Button
                    variant="default"
                    size="sm"
                    loading={submitting}
                    onClick={handleStartFollow}
                    className="gap-1.5 text-xs"
                  >
                    <Play className="h-3.5 w-3.5" />
                    <span>启动批量自动跟卖</span>
                  </Button>
                </div>
              </CardHeader>
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow>
                      <TableHead className="w-12 text-center">#</TableHead>
                      <TableHead>商品目标链接 (Source URL)</TableHead>
                      <TableHead className="w-28 text-center">初始库存</TableHead>
                      <TableHead className="w-32 text-center">保底价格 (R)</TableHead>
                      <TableHead className="w-20 text-center">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {items.map((item, index) => (
                      <TableRow key={index}>
                        <TableCell className="text-center font-mono text-xs text-muted-foreground">
                          {index + 1}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1.5 font-mono text-xs max-w-xl truncate">
                            <span className="truncate text-foreground">{item.url}</span>
                            <a
                              href={item.url}
                              target="_blank"
                              rel="noreferrer"
                              className="text-primary shrink-0 hover:text-primary/80"
                            >
                              <ExternalLink className="h-3 w-3" />
                            </a>
                          </div>
                        </TableCell>
                        <TableCell className="text-center font-mono font-semibold text-xs">
                          {item.stock}
                        </TableCell>
                        <TableCell className="text-center font-mono font-semibold text-xs text-emerald-600 dark:text-emerald-400">
                          R {item.min_price}
                        </TableCell>
                        <TableCell className="text-center">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-destructive"
                            onClick={() => setItems((prev) => prev.filter((_, i) => i !== index))}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </Card>
          )}
        </TabsContent>

        {/* Tab 2: Official Takealot Batch Queue */}
        <TabsContent value="official-batch" className="space-y-5 mt-4">
          <Card>
            <CardHeader className="p-4 sm:p-5 flex flex-row items-center justify-between border-b border-border">
              <div>
                <div className="flex items-center gap-2">
                  <CardTitle className="text-base font-bold">官方异步批处理任务 (Takealot Batch)</CardTitle>
                  <Badge variant="secondary" className="font-mono text-xs">
                    官方接口 /v2/offers/batch
                  </Badge>
                </div>
                <CardDescription className="text-xs">
                  支持单次提交最多 10,000 件商品的价格更新、上下架与新建任务，由 Takealot 官方排队处理
                </CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={loadBatchJobs}
                  loading={batchLoading}
                  className="gap-1.5 text-xs"
                >
                  <RefreshCw className="h-3.5 w-3.5" />
                  <span>刷新任务状态</span>
                </Button>
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => setCreateBatchModal((prev) => ({ ...prev, open: true }))}
                  className="gap-1.5 text-xs"
                >
                  <Send className="h-3.5 w-3.5" />
                  <span>新建批处理提交</span>
                </Button>
              </div>
            </CardHeader>

            <div className="overflow-x-auto">
              <Table>
                <TableHeader className="bg-muted/40">
                  <TableRow>
                    <TableHead className="w-32">提交时间</TableHead>
                    <TableHead className="w-36">Batch ID</TableHead>
                    <TableHead className="w-32">类型</TableHead>
                    <TableHead className="w-24 text-center">商品数</TableHead>
                    <TableHead className="w-28 text-center">官方状态</TableHead>
                    <TableHead className="text-right pr-4">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batchLoading ? (
                    <TableRow>
                      <TableCell colSpan={6} className="py-12 text-center text-xs text-muted-foreground">
                        正在从 SQLite 数据库载入批处理流水...
                      </TableCell>
                    </TableRow>
                  ) : batchJobs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="py-12 text-center text-xs text-muted-foreground">
                        暂无已提交的官方 Batch 批处理任务（点击上方「新建批处理提交」即可向 Takealot 官方提交批量任务）
                      </TableCell>
                    </TableRow>
                  ) : (
                    batchJobs.map((job) => (
                      <TableRow key={job.id} className="hover:bg-muted/30">
                        <TableCell className="text-xs font-mono text-muted-foreground whitespace-nowrap">
                          {job.created_at}
                        </TableCell>
                        <TableCell className="text-xs font-mono font-bold text-foreground">
                          {job.batch_id}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {job.action_type || 'batch_update'}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-center font-semibold">
                          {job.item_count}
                        </TableCell>
                        <TableCell className="text-center">
                          <Badge
                            variant={
                              job.status.toLowerCase().includes('complete')
                                ? 'success'
                                : job.status.toLowerCase().includes('fail')
                                ? 'destructive'
                                : 'warning'
                            }
                            className="text-[10px]"
                          >
                            {job.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-right pr-4">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleInspectBatch(job.batch_id)}
                            className="gap-1 text-xs text-primary"
                          >
                            <Eye className="h-3.5 w-3.5" />
                            <span>追踪详情</span>
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Batch Details Modal */}
      <Dialog
        open={batchDetailModal.open}
        onClose={() => setBatchDetailModal({ open: false, batchId: '' })}
        title={`官方批处理进度与校验详情 · Batch ID: ${batchDetailModal.batchId}`}
        description="实时调取 Takealot 官方 /v2/offers/batch/{id} 接口返回的处理状态与每个商品的错误诊断"
        maxWidth="2xl"
      >
        <div className="space-y-4 text-xs">
          {batchDetailModal.loading ? (
            <div className="py-12 text-center text-muted-foreground">
              正在同步 Takealot 官方批处理队列实时结果...
            </div>
          ) : batchDetailModal.error ? (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive border border-destructive/20">
              查询失败: {batchDetailModal.error}
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 rounded-xl bg-muted/40 border border-border">
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground font-medium">当前处理状态:</span>
                  <Badge variant="default" className="text-xs">
                    {batchDetailModal.data?.status?.description || 'Unknown'}
                  </Badge>
                </div>
                <div className="text-muted-foreground font-mono">
                  处理完成条目: {batchDetailModal.data?.results?.length || 0}
                </div>
              </div>

              {/* Items / Errors List */}
              <div className="max-h-[360px] overflow-y-auto rounded-xl border border-border">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow>
                      <TableHead className="w-24">Offer ID</TableHead>
                      <TableHead className="w-28">SKU</TableHead>
                      <TableHead className="w-20 text-center">状态</TableHead>
                      <TableHead>结果与校验错误 (Validation Errors)</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(!batchDetailModal.data?.results || batchDetailModal.data.results.length === 0) ? (
                      <TableRow>
                        <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                          官方队列排队中或暂无结果条目
                        </TableCell>
                      </TableRow>
                    ) : (
                      batchDetailModal.data.results.map((r, i) => {
                        const hasErrors = r.validation_errors && r.validation_errors.length > 0
                        return (
                          <TableRow key={i}>
                            <TableCell className="font-mono text-xs text-foreground">
                              {r.offer?.offer_id || '-'}
                            </TableCell>
                            <TableCell className="font-mono text-xs text-muted-foreground">
                              {r.offer?.sku || '-'}
                            </TableCell>
                            <TableCell className="text-center font-mono text-xs">
                              {r.offer?.status || '-'}
                            </TableCell>
                            <TableCell>
                              {hasErrors ? (
                                <div className="space-y-1">
                                  {r.validation_errors!.map((err, eIdx) => (
                                    <div key={eIdx} className="text-destructive flex items-start gap-1">
                                      <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                      <span>
                                        [{err.code}] {err.message}
                                      </span>
                                    </div>
                                  ))}
                                </div>
                              ) : (
                                <div className="text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
                                  <CheckCircle2 className="h-3 w-3" />
                                  <span>处理成功</span>
                                </div>
                              )}
                            </TableCell>
                          </TableRow>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          )}

          <div className="flex justify-end pt-2">
            <Button variant="outline" size="sm" onClick={() => setBatchDetailModal({ open: false, batchId: '' })}>
              关闭
            </Button>
          </div>
        </div>
      </Dialog>

      {/* Create Official Batch Modal */}
      <Dialog
        open={createBatchModal.open}
        onClose={() => setCreateBatchModal((prev) => ({ ...prev, open: false }))}
        title="提交 Takealot 官方批处理队列 (Batch Create / Update)"
        description="支持通过 JSON 数组一次性提交最多 10,000 个商品的价格、库存调整与上下架指令"
        maxWidth="lg"
      >
        <div className="space-y-3.5 text-xs">
          <div>
            <label className="block text-foreground font-medium mb-1">
              批处理 JSON 数组 (格式请符合官方 V2OfferBatchCreateUpdate 协议)
            </label>
            <textarea
              rows={12}
              value={createBatchModal.jsonContent}
              onChange={(e) => setCreateBatchModal((prev) => ({ ...prev, jsonContent: e.target.value }))}
              className="w-full font-mono text-xs p-3 rounded-lg border border-border bg-muted/40 focus:outline-none focus:ring-1 focus:ring-primary"
              placeholder={`[{"offer_id": 12345, "selling_price": 499, "rrp": 899, "status_action": "Re-enable"}]`}
            />
          </div>

          <div className="p-3 rounded-lg bg-muted/50 text-[11px] text-muted-foreground space-y-1">
            <div className="font-semibold text-foreground">支持的字段参数:</div>
            <div>• <code className="font-mono">offer_id</code> 或 <code className="font-mono">sku</code> 或 <code className="font-mono">barcode</code>：商品识别标志</div>
            <div>• <code className="font-mono">selling_price</code> (正整数售价), <code className="font-mono">rrp</code> (建议零售价)</div>
            <div>• <code className="font-mono">leadtime_days</code> (发货准备天数, -1为移除)</div>
            <div>• <code className="font-mono">status_action</code>: "Disable" (停售) 或 "Re-enable" (重新上架)</div>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={() => setCreateBatchModal((prev) => ({ ...prev, open: false }))}>
              取消
            </Button>
            <Button
              variant="default"
              size="sm"
              loading={createBatchModal.submitting}
              onClick={handleCreateBatchSubmit}
            >
              提交至官方批处理队列
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}

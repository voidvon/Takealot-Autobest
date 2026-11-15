import React, { useEffect, useState } from 'react'
import type { FollowItem, FollowVariant } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription } from '../ui/card'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Input } from '../ui/input'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { Play } from 'lucide-react'

type ReviewRow = FollowVariant & { selected: boolean }

function priceError(row: FollowItem): string {
  if (!Number.isInteger(row.min_price) || row.min_price <= 0) return '请设置大于 0 的底价'
  if (!Number.isInteger(row.stock) || row.stock <= 0) return '库存须为正整数'
  const max = row.max_price || 0
  const price = row.selling_price || 0
  if (!Number.isInteger(max) || max < 0 || !Number.isInteger(price) || price < 0) return '价格须为非负整数'
  if (max > 0 && max < row.min_price) return '封顶价不能低于底价'
  if (price > 0 && (price < row.min_price || (max > 0 && price > max))) return '初始售价超出保护范围'
  return ''
}

interface Props {
  variants: FollowVariant[]
  blocked: boolean
  submitting: boolean
  onSubmit: (items: FollowItem[]) => Promise<void>
}

export const FollowVariantReview: React.FC<Props> = ({ variants, blocked, submitting, onSubmit }) => {
  const [rows, setRows] = useState<ReviewRow[]>([])
  useEffect(() => {
    setRows(variants.map((variant) => ({ ...variant, selected: variant.can_follow })))
  }, [variants])

  const selected = rows.filter((row) => row.selected && row.can_follow)
  const invalidCount = selected.filter((row) => priceError(row)).length
  const allSelected = rows.some((row) => row.can_follow) && rows.every((row) => !row.can_follow || row.selected)
  const update = (index: number, changes: Partial<ReviewRow>) => {
    setRows((previous) => previous.map((row, i) => i === index ? { ...row, ...changes } : row))
  }

  return (
    <Card>
      <CardHeader className="p-4 sm:p-5 border-b border-border">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-base">规格价格核对</CardTitle>
            <CardDescription className="text-xs">
              每个新规格分别核对底价、封顶价和初始售价；已有商品的底价配置请前往「自动竞价」。初始售价留空时，按该规格市场价减 R1 计算并限制在保护范围内；无市场价时使用底价。计划备货量仅作记录，上架成功不代表平台已有可售库存。
            </CardDescription>
          </div>
          <Button size="sm" loading={submitting}
            disabled={blocked || selected.length === 0 || invalidCount > 0}
            onClick={() => void onSubmit(selected.map(({ url, tsin_id, stock, min_price, max_price, selling_price }) => ({
              url, tsin_id, stock, min_price, max_price, selling_price,
            })))} className="gap-1.5">
            <Play className="h-3.5 w-3.5" />
            上架已勾选的 {selected.length} 个规格
          </Button>
        </div>
        <div className="text-xs text-muted-foreground pt-2" role="status">
          已解析 {rows.length} 个规格，已勾选 {selected.length} 个
          {invalidCount > 0 && <span className="text-destructive"> · {invalidCount} 个规格需修正价格或库存</span>}
          {blocked && <span className="text-destructive"> · 请先修正导入行中的解析错误并重新解析</span>}
        </div>
      </CardHeader>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader className="bg-muted/40">
            <TableRow>
              <TableHead className="w-10 text-center">
                <input type="checkbox" aria-label="勾选所有可跟卖规格" checked={allSelected} disabled={submitting || !rows.some((row) => row.can_follow)}
                  onChange={(event) => setRows((previous) => previous.map((row) => ({ ...row, selected: row.can_follow && event.target.checked })))} />
              </TableHead>
              <TableHead className="min-w-64">商品与规格</TableHead>
              <TableHead className="w-24 text-center">市场参考价</TableHead>
              <TableHead className="w-20 text-center">计划备货量</TableHead>
              <TableHead className="w-28 text-center">防亏底价 (R)</TableHead>
              <TableHead className="w-28 text-center">封顶价 (R)</TableHead>
              <TableHead className="w-32 text-center">初始售价 (R)</TableHead>
              <TableHead className="min-w-36">状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, index) => {
              const error = priceError(row)
              return (
                <TableRow key={`${row.plid}/${row.tsin_id}`} className={!row.can_follow ? 'opacity-60' : ''}>
                  <TableCell className="text-center">
                    <input type="checkbox" aria-label={`勾选规格 ${row.tsin_id}`} checked={row.selected}
                      disabled={!row.can_follow || submitting} onChange={(event) => update(index, { selected: event.target.checked })} />
                  </TableCell>
                  <TableCell className="text-xs space-y-1">
                    <div className="font-medium">{row.title || `规格 ${row.tsin_id}`}</div>
                    <div>{[row.colour, row.size].filter(Boolean).join(' / ') || '未返回颜色、尺码，请按 TSIN 核对'}</div>
                    <div className="font-mono text-muted-foreground">TSIN: {row.tsin_id} · PLID: {row.plid}</div>
                  </TableCell>
                  <TableCell className="text-center text-xs font-mono">{row.best_price > 0 ? `R ${row.best_price}` : '未获取'}</TableCell>
                  {(['stock', 'min_price', 'max_price', 'selling_price'] as const).map((field) => (
                    <TableCell key={field}>
                      <Input type="number" step="1" min={field === 'stock' || field === 'min_price' ? 1 : 0}
                        aria-label={`TSIN ${row.tsin_id} ${field}`}
                        value={row[field] || ''}
                        placeholder={field === 'selling_price' ? '自动' : field === 'max_price' ? '不限' : '必填'}
                        disabled={!row.can_follow || submitting}
                        onChange={(event) => update(index, { [field]: event.target.value === '' ? 0 : Number(event.target.value) })}
                        className="min-w-20 h-8 text-center text-xs font-mono" />
                    </TableCell>
                  ))}
                  <TableCell className="text-xs">
                    {!row.can_follow ? <Badge variant="secondary">{row.skip_reason}</Badge>
                      : error ? <span className="text-destructive">{error}</span>
                      : <Badge variant="success">可上架</Badge>}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </Card>
  )
}

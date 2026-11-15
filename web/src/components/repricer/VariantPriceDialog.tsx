import React, { useState } from 'react'
import type { OfferViewModel, VariantPriceConfig } from '../../types'
import { Dialog } from '../ui/dialog'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from '../ui/table'
import { formatCurrency } from '../../lib/utils'
import { Save } from 'lucide-react'

interface Props {
  offers: OfferViewModel[]
  onClose: () => void
  onSave: (prices: VariantPriceConfig[]) => Promise<void>
}

function priceError(row: VariantPriceConfig): string {
  if (!Number.isSafeInteger(row.min_price) || row.min_price <= 0) return '底价须为大于 0 的整数'
  if (!Number.isSafeInteger(row.max_price) || row.max_price < 0) return '封顶价须为非负整数'
  if (row.max_price > 0 && row.max_price < row.min_price) return '封顶价不能低于底价'
  return ''
}

export const VariantPriceDialog: React.FC<Props> = ({ offers, onClose, onSave }) => {
  const [rows, setRows] = useState(() => offers.map((offer) => ({ ...offer })))
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const changed = rows.filter((row, index) => row.min_price !== offers[index].min_price || row.max_price !== offers[index].max_price)
  const invalid = changed.some((row) => priceError(row))

  const save = async () => {
    if (changed.length === 0 || invalid || saving) return
    setSaving(true)
    setSaveError('')
    try {
      await onSave(changed.map(({ key, min_price, max_price }) => ({ key, min_price, max_price })))
      onClose()
    } catch (err: any) {
      setSaveError(err.message || '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onClose={() => { if (!saving) onClose() }} maxWidth="4xl"
      title="配置规格价格" description="每个规格分别设置防亏底价和封顶价，保存后用于自动竞价。当前售价仅供核对。">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {offers[0].store_name && <Badge variant="outline">{offers[0].store_name}</Badge>}
        <span className="font-mono">PLID: {offers[0].plid}</span>
        <span>共 {rows.length} 个在售规格</span>
      </div>
      <div className="max-h-[55vh] overflow-auto rounded-lg border border-border">
        <Table>
          <TableHeader className="bg-muted/40">
            <TableRow>
              <TableHead className="min-w-64">规格 / SKU</TableHead>
              <TableHead className="text-center whitespace-nowrap">当前售价 (R)</TableHead>
              <TableHead className="min-w-28 text-center">防亏底价 (R)</TableHead>
              <TableHead className="min-w-28 text-center">封顶价 (R)</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, index) => {
              const dirty = changed.some((item) => item.key === row.key)
              const error = dirty ? priceError(row) : ''
              return (
                <TableRow key={row.key}>
                  <TableCell className="max-w-sm space-y-1 text-xs">
                    <div className="font-medium">{row.title || `规格 ${row.tsin_id}`}</div>
                    <div className="font-mono text-muted-foreground">TSIN: {row.tsin_id}{row.sku && ` · SKU: ${row.sku}`}</div>
                    {error ? <div role="alert" className="text-destructive">{error}</div>
                      : dirty ? <div className="text-primary">待保存</div>
                      : row.min_price <= 0 && <div className="text-muted-foreground">尚未设置底价</div>}
                  </TableCell>
                  <TableCell className="text-center text-sm font-mono font-semibold">{formatCurrency(row.selling_price)}</TableCell>
                  {(['min_price', 'max_price'] as const).map((field) => (
                    <TableCell key={field}>
                      <Input type="number" step="1" min={field === 'min_price' ? 1 : 0}
                        aria-label={`TSIN ${row.tsin_id} ${field === 'min_price' ? '防亏底价' : '封顶价'}`}
                        value={row[field] || ''} placeholder={field === 'min_price' ? '必填' : '不限'} disabled={saving}
                        onChange={(event) => {
                          const value = event.target.value === '' ? 0 : Number(event.target.value)
                          setRows((previous) => previous.map((item, i) => i === index ? { ...item, [field]: value } : item))
                        }} className="h-9 text-center font-mono text-sm" />
                    </TableCell>
                  ))}
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
      {saveError && <p role="alert" className="text-sm text-destructive">{saveError}</p>}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p role="status" className="text-xs text-muted-foreground">已修改 {changed.length} 个规格 · 封顶价留空表示不限</p>
        <div className="flex items-center gap-2">
          <Button variant="outline" disabled={saving} onClick={onClose}>取消</Button>
          <Button loading={saving} disabled={changed.length === 0 || invalid} onClick={() => void save()}>
            <Save className="h-4 w-4" />保存规格配置
          </Button>
        </div>
      </div>
    </Dialog>
  )
}

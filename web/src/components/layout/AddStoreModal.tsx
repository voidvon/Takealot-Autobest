import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { Store } from '../../types'
import { Dialog, DialogFooter } from '../ui/dialog'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { ExternalLink, CheckCircle2, AlertCircle } from 'lucide-react'

interface AddStoreModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: (newStore: Store) => void
}

export const AddStoreModal: React.FC<AddStoreModalProps> = ({
  open,
  onOpenChange,
  onSuccess,
}) => {
  const [storeForm, setStoreForm] = useState<Partial<Store>>({
    id: '',
    name: '',
    authorization: '',
    proxy_url: '',
    is_active: true,
    price_decrease_step: 1,
    price_increase_step: 1,
    rrp_percentage: 120,
    interval_minutes: 5,
    bulk_stock: 1,
    max_fetch_offers: 1000,
  })

  const [testingToken, setTestingToken] = useState(false)
  const [testResult, setTestResult] = useState<{
    success: boolean
    message: string
    display_name?: string
    total_offers?: number
  } | null>(null)
  const [savingStore, setSavingStore] = useState(false)

  useEffect(() => {
    if (open) {
      setStoreForm({
        id: `store_${Date.now()}`,
        name: '',
        authorization: '',
        proxy_url: '',
        is_active: true,
        price_decrease_step: 1,
        price_increase_step: 1,
        rrp_percentage: 120,
        interval_minutes: 5,
        bulk_stock: 1,
        max_fetch_offers: 1000,
      })
      setTestResult(null)
    }
  }, [open])

  const handleTestToken = async () => {
    if (!storeForm.authorization) {
      alert('请先输入 Authorization 凭据')
      return
    }
    setTestingToken(true)
    setTestResult(null)
    try {
      const res = await api.testStore(storeForm.authorization, storeForm.proxy_url)
      setTestResult(res)
      if (res.success && res.display_name && !storeForm.name) {
        setStoreForm((prev) => ({ ...prev, name: res.display_name }))
      }
    } catch (err: any) {
      setTestResult({
        success: false,
        message: err.message,
      })
    } finally {
      setTestingToken(false)
    }
  }

  const handleSaveNewStore = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!storeForm.authorization) {
      alert('店铺 Authorization 为必填项')
      return
    }
    setSavingStore(true)
    try {
      const res = await api.createStore(storeForm)
      if (res.success) {
        onOpenChange(false)
        onSuccess(res.store)
      }
    } catch (err: any) {
      alert(`添加店铺失败: ${err.message}`)
    } finally {
      setSavingStore(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="添加 Takealot 卖家店铺"
      description="填写店铺授权凭据以接入多店铺自动化管理，系统支持自动校验并获取官方店铺名称。"
      maxWidth="lg"
    >
      <form onSubmit={handleSaveNewStore} className="space-y-4 pt-2">
        <div className="space-y-1.5">
          <label className="text-xs font-semibold text-foreground flex items-center justify-between">
            <span>店铺授权凭据 (Authorization) *</span>
            <a
              href="https://sellers.takealot.com"
              target="_blank"
              rel="noreferrer"
              className="text-primary hover:underline text-[11px] flex items-center gap-1 font-normal"
            >
              <span>前往卖家中心</span>
              <ExternalLink className="h-3 w-3" />
            </a>
          </label>
          <Input
            required
            value={storeForm.authorization || ''}
            onChange={(e) => setStoreForm({ ...storeForm, authorization: e.target.value })}
            placeholder="Key 389bxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            className="font-mono text-xs"
          />
        </div>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleTestToken}
            loading={testingToken}
            className="text-xs h-8"
          >
            验证连通性并自动获取店铺名
          </Button>
          {testResult && (
            <span
              className={`text-xs flex items-center gap-1 ${
                testResult.success ? 'text-emerald-600' : 'text-destructive'
              }`}
            >
              {testResult.success ? (
                <CheckCircle2 className="h-3.5 w-3.5" />
              ) : (
                <AlertCircle className="h-3.5 w-3.5" />
              )}
              <span>{testResult.message}</span>
            </span>
          )}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              店铺显示名称 *
            </label>
            <Input
              required
              value={storeForm.name || ''}
              onChange={(e) => setStoreForm({ ...storeForm, name: e.target.value })}
              placeholder="例如: 约堡旗舰店"
              className="text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              店铺独立代理 Proxy (可选)
            </label>
            <Input
              value={storeForm.proxy_url || ''}
              onChange={(e) => setStoreForm({ ...storeForm, proxy_url: e.target.value })}
              placeholder="http://127.0.0.1:7890"
              className="font-mono text-xs"
            />
            <p className="text-[10px] text-muted-foreground">留空则走本机直连网络</p>
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3 pt-2">
          <div className="space-y-1">
            <label className="text-[11px] text-muted-foreground">降价步长 (R)</label>
            <Input
              type="number"
              min="1"
              value={storeForm.price_decrease_step || 1}
              onChange={(e) =>
                setStoreForm({ ...storeForm, price_decrease_step: parseInt(e.target.value) || 1 })
              }
              className="text-xs font-mono h-8"
            />
          </div>
          <div className="space-y-1">
            <label className="text-[11px] text-muted-foreground">提价步长 (R)</label>
            <Input
              type="number"
              min="1"
              value={storeForm.price_increase_step || 1}
              onChange={(e) =>
                setStoreForm({ ...storeForm, price_increase_step: parseInt(e.target.value) || 1 })
              }
              className="text-xs font-mono h-8"
            />
          </div>
          <div className="space-y-1">
            <label className="text-[11px] text-muted-foreground">巡检周期 (分钟)</label>
            <Input
              type="number"
              min="1"
              value={storeForm.interval_minutes || 5}
              onChange={(e) =>
                setStoreForm({ ...storeForm, interval_minutes: parseInt(e.target.value) || 5 })
              }
              className="text-xs font-mono h-8"
            />
          </div>
        </div>

        <DialogFooter className="pt-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="text-xs"
          >
            取消
          </Button>
          <Button type="submit" loading={savingStore} className="text-xs">
            确认添加
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  )
}

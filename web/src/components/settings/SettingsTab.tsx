import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { SystemConfig } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import {
  Key,
  Sliders,
  ShieldCheck,
  Save,
  CheckCircle2,
  AlertCircle,
  ExternalLink,
  Info,
  Server,
  Zap,
} from 'lucide-react'

export const SettingsTab: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testResult, setTestResult] = useState<{
    success: boolean
    message: string
    total_offers?: number
  } | null>(null)

  const [form, setForm] = useState<SystemConfig>({
    authorization: '',
    price_decrease_step: 1,
    price_increase_step: 1,
    rrp_percentage: 120,
    interval_minutes: 5,
    bulk_stock: 1,
    max_fetch_offers: 1000,
    targets: {},
  })

  const loadConfig = async () => {
    setLoading(true)
    try {
      const cfg = await api.getConfig()
      setForm(cfg)
    } catch (err: any) {
      alert(`读取系统配置失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadConfig()
  }, [])

  const handleTestConnection = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      // First save if updated
      await api.updateConfig(form)
      const res = await api.testAuth()
      setTestResult(res)
    } catch (err: any) {
      setTestResult({
        success: false,
        message: `测试异常: ${err.message}`,
      })
    } finally {
      setTesting(false)
    }
  }

  const handleSaveConfig = async (e?: React.FormEvent) => {
    if (e) e.preventDefault()
    setSaving(true)
    try {
      const res = await api.updateConfig(form)
      if (res.success) {
        alert('🎉 系统配置已成功保存并立即生效！')
      }
    } catch (err: any) {
      alert(`保存配置失败: ${err.message}`)
    } finally {
      setSaving(false)
    }
  }

  const isKeyFormat = form.authorization.startsWith('Key ')
  const isBearerFormat = form.authorization.startsWith('Bearer ')

  return (
    <div className="space-y-6 max-w-5xl animate-in fade-in duration-200">
      {/* Header */}
      <div>
        <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
          店铺授权与系统参数设置 (Settings)
        </h2>
        <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
          配置 Takealot 官方 Seller API Key · 自定义智能调价安全边际与轮询策略
        </p>
      </div>

      <form onSubmit={handleSaveConfig} className="space-y-6">
        {/* 1. Store Authorization */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Key className="h-5 w-5 text-primary" />
                  <span>店铺 API 授权凭证 (Takealot Seller API Key)</span>
                </CardTitle>
                <CardDescription>
                  在 Takealot 卖家中心 (Seller Portal &gt; API Integrations &gt; Seller API) 生成的官方密钥
                </CardDescription>
              </div>
              <div>
                {isKeyFormat && (
                  <Badge variant="success" dot>官方 API Key</Badge>
                )}
                {isBearerFormat && (
                  <Badge variant="warning" dot>网页 Bearer Token</Badge>
                )}
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-foreground flex items-center justify-between">
                <span>Authorization Header (带 Key 前缀)</span>
                <span className="text-[11px] font-normal text-muted-foreground">
                  格式示范: <code>Key 966bf894...</code>
                </span>
              </label>
              <textarea
                rows={3}
                value={form.authorization}
                onChange={(e) => setForm({ ...form, authorization: e.target.value })}
                placeholder="请输入以 Key 开头的官方 API Key，例如: Key 966bf894c2ae2bfb..."
                className="w-full rounded-lg border border-input bg-background p-3 text-xs font-mono shadow-xs focus-visible:outline-none focus-visible:ring-1.5 focus-visible:ring-ring"
                required
              />
            </div>

            {/* Test Connection Button & Alert */}
            <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
              <div className="text-xs text-muted-foreground flex items-center gap-1.5">
                <Info className="h-4 w-4 text-blue-500 shrink-0" />
                <span>系统后端已自动兼容 API Key 前缀补全与安全性防错校验</span>
              </div>

              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleTestConnection}
                loading={testing}
                className="gap-1.5 text-xs"
              >
                <Zap className="h-3.5 w-3.5 text-amber-500 fill-current" />
                <span>立即测试授权连通性</span>
              </Button>
            </div>

            {testResult && (
              <div
                className={`p-3.5 rounded-xl border flex items-center justify-between gap-3 animate-in fade-in text-xs ${
                  testResult.success
                    ? 'bg-emerald-50 text-emerald-800 border-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:border-emerald-800/60'
                    : 'bg-rose-50 text-rose-800 border-rose-200 dark:bg-rose-950/40 dark:text-rose-300 dark:border-rose-800/60'
                }`}
              >
                <div className="flex items-center gap-2">
                  {testResult.success ? (
                    <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                  ) : (
                    <AlertCircle className="h-4 w-4 text-rose-600 shrink-0" />
                  )}
                  <span className="font-medium">{testResult.message}</span>
                </div>
                {testResult.success && testResult.total_offers !== undefined && (
                  <Badge variant="success" className="font-mono">
                    有效商品: {testResult.total_offers} 件
                  </Badge>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        {/* 2. Repricing Strategy Parameters */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Sliders className="h-5 w-5 text-primary" />
              <span>自动化调价策略与步长控制</span>
            </CardTitle>
            <CardDescription>
              细粒度控制巡检轮询频率、竞价跟进幅度以及自动加价保护机制
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {/* Poll Interval */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  巡检轮询间隔 (分钟)
                </label>
                <Input
                  type="number"
                  min="1"
                  max="60"
                  value={form.interval_minutes}
                  onChange={(e) =>
                    setForm({ ...form, interval_minutes: parseInt(e.target.value) || 5 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  建议设为 3 ~ 10 分钟，兼顾竞价时效与 API 限流安全
                </p>
              </div>

              {/* Price Decrease Step */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  降价抢位步长 (Rands)
                </label>
                <Input
                  type="number"
                  min="1"
                  value={form.price_decrease_step}
                  onChange={(e) =>
                    setForm({ ...form, price_decrease_step: parseInt(e.target.value) || 1 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  当失去优先时，比竞品最低价低 R{form.price_decrease_step} 以夺取 Buybox
                </p>
              </div>

              {/* Price Increase Step */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  回升加价步长 (Rands)
                </label>
                <Input
                  type="number"
                  min="1"
                  value={form.price_increase_step}
                  onChange={(e) =>
                    setForm({ ...form, price_increase_step: parseInt(e.target.value) || 1 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  当对手提价或独家时，每次自动回升 R{form.price_increase_step} 恢复利润
                </p>
              </div>

              {/* RRP Percentage */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  建议零售价浮动比率 (%)
                </label>
                <Input
                  type="number"
                  min="100"
                  max="300"
                  value={form.rrp_percentage}
                  onChange={(e) =>
                    setForm({ ...form, rrp_percentage: parseInt(e.target.value) || 120 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  RRP 相对售价的默认比率，用于向买家展示划线折扣
                </p>
              </div>

              {/* Bulk Follow Stock */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  跟卖上架默认库存 (件)
                </label>
                <Input
                  type="number"
                  min="1"
                  value={form.bulk_stock}
                  onChange={(e) =>
                    setForm({ ...form, bulk_stock: parseInt(e.target.value) || 1 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  批量跟卖上传未填写库存时所采用的默认上架数量
                </p>
              </div>

              {/* Max Fetch Offers */}
              <div className="space-y-1.5 p-3.5 rounded-xl bg-muted/30 border border-border/60">
                <label className="text-xs font-semibold text-foreground">
                  单次最大同步商品数 (件)
                </label>
                <Input
                  type="number"
                  min="50"
                  max="5000"
                  value={form.max_fetch_offers}
                  onChange={(e) =>
                    setForm({ ...form, max_fetch_offers: parseInt(e.target.value) || 1000 })
                  }
                />
                <p className="text-[11px] text-muted-foreground">
                  单次巡检调价时，最多拉取并监控的店铺商品上限
                </p>
              </div>
            </div>

            {/* Bottom Actions */}
            <div className="flex justify-end pt-4 border-t border-border">
              <Button type="submit" variant="default" size="default" loading={saving} className="gap-1.5 shadow-sm">
                <Save className="h-4 w-4" />
                <span>保存全部系统配置</span>
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}

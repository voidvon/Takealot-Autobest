import React, { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { Store, SystemConfig } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Switch } from '../ui/switch'
import { Dialog, DialogFooter } from '../ui/dialog'
import { toast } from '../ui/use-toast'
import {
  Key,
  Sliders,
  ShieldCheck,
  Save,
  Info,
  Server,
  Zap,
  Plus,
  Trash2,
  Edit2,
  Store as StoreIcon,
  RefreshCw,
  Globe,
  Radio,
} from 'lucide-react'

interface SettingsTabProps {
  stores: Store[]
  currentStoreId: string
  onRefreshStores: () => Promise<void>
  onSelectStore: (storeId: string) => void
  onOpenAddStore?: () => void
}

export const SettingsTab: React.FC<SettingsTabProps> = ({
  stores,
  currentStoreId,
  onRefreshStores,
  onSelectStore,
  onOpenAddStore,
}) => {
  const [editingStore, setEditingStore] = useState<Store | null>(null)
  const [isEditOpen, setIsEditOpen] = useState(false)

  // Store Form State (used for both Add and Edit)
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

  const [savingStore, setSavingStore] = useState(false)
  const [syncingStoreId, setSyncingStoreId] = useState<string | null>(null)
  const [testingStoreId, setTestingStoreId] = useState<string | null>(null)

  // Active store config form
  const [activeForm, setActiveForm] = useState<SystemConfig>({
    authorization: '',
    price_decrease_step: 1,
    price_increase_step: 1,
    rrp_percentage: 120,
    interval_minutes: 5,
    bulk_stock: 1,
    max_fetch_offers: 1000,
    targets: {},
  })
  const [savingActive, setSavingActive] = useState(false)

  const currentStore = stores.find((s) => s.id === currentStoreId) || stores[0]

  useEffect(() => {
    if (currentStore) {
      setActiveForm({
        authorization: currentStore.authorization,
        price_decrease_step: currentStore.price_decrease_step,
        price_increase_step: currentStore.price_increase_step,
        rrp_percentage: currentStore.rrp_percentage,
        interval_minutes: currentStore.interval_minutes,
        bulk_stock: currentStore.bulk_stock,
        max_fetch_offers: currentStore.max_fetch_offers,
        targets: {},
      })
    }
  }, [currentStore?.id, currentStore?.authorization])

  const openAddModal = () => {
    if (onOpenAddStore) {
      onOpenAddStore()
    }
  }

  const openEditModal = (store: Store) => {
    setEditingStore(store)
    setStoreForm({
      id: store.id,
      name: store.name,
      authorization: store.authorization,
      proxy_url: store.proxy_url || '',
      is_active: store.is_active,
      price_decrease_step: store.price_decrease_step,
      price_increase_step: store.price_increase_step,
      rrp_percentage: store.rrp_percentage,
      interval_minutes: store.interval_minutes,
      bulk_stock: store.bulk_stock,
      max_fetch_offers: store.max_fetch_offers,
    })
    setIsEditOpen(true)
  }

  const handleSaveEditStore = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editingStore) return
    setSavingStore(true)
    try {
      const res = await api.updateStore(storeForm)
      if (res.success) {
        setIsEditOpen(false)
        await onRefreshStores()
        toast.success('店铺配置更新成功！')
      }
    } catch (err: any) {
      toast.error('更新店铺失败', err.message)
    } finally {
      setSavingStore(false)
    }
  }

  const handleDeleteStore = async (store: Store) => {
    if (stores.length <= 1) {
      toast.warning('无法删除', '系统中至少保留一个店铺，无法删除最后一个店铺！')
      return
    }
    if (!confirm(`确定要删除店铺 [${store.name}] (ID: ${store.id}) 吗？\n删除后该店铺的历史监控记录将被清理，此操作不可撤回。`)) {
      return
    }
    try {
      const res = await api.deleteStore(store.id)
      if (res.success) {
        await onRefreshStores()
        toast.success('店铺已成功删除', store.name)
        if (currentStoreId === store.id) {
          const remaining = stores.filter((s) => s.id !== store.id)
          if (remaining.length > 0) {
            onSelectStore(remaining[0].id)
          }
        }
      }
    } catch (err: any) {
      toast.error('删除店铺失败', err.message)
    }
  }

  const handleTestExistingStore = async (store: Store) => {
    setTestingStoreId(store.id)
    try {
      const res = await api.testStore(store.authorization, store.proxy_url)
      if (res.success) {
        toast.success(`店铺 [${store.name}] 授权测试通过！`, `Takealot 响应: ${res.message}\n当前在售商品: ${res.total_offers ?? '已联通'}`)
      } else {
        toast.error(`店铺 [${store.name}] 连接测试失败`, res.message)
      }
    } catch (err: any) {
      toast.error('测试异常', err.message)
    } finally {
      setTestingStoreId(null)
    }
  }

  const handleSyncStoreName = async (store: Store) => {
    setSyncingStoreId(store.id)
    try {
      const res = await api.syncStoreName()
      if (res.success) {
        await onRefreshStores()
        toast.success('官方店铺名已同步', res.name)
      }
    } catch (err: any) {
      toast.error('同步官方名称失败', err.message)
    } finally {
      setSyncingStoreId(null)
    }
  }

  const handleToggleStoreActive = async (store: Store, isActive: boolean) => {
    try {
      await api.updateStore({ id: store.id, is_active: isActive })
      await onRefreshStores()
      toast.info(`店铺 [${store.name}] 已${isActive ? '开启监控' : '停用'}`)
    } catch (err: any) {
      toast.error('更新店铺状态失败', err.message)
    }
  }

  const handleSaveActiveStoreConfig = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!currentStore) return
    setSavingActive(true)
    try {
      const res = await api.updateConfig(activeForm)
      if (res.success) {
        await onRefreshStores()
        toast.success('店铺参数已成功保存并立即生效！')
      }
    } catch (err: any) {
      toast.error('保存失败', err.message)
    } finally {
      setSavingActive(false)
    }
  }

  return (
    <div className="space-y-6 max-w-5xl animate-in fade-in duration-200 pb-12">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
            多店铺管理与系统参数设置 (Multi-Store Settings)
          </h2>
          <p className="text-xs text-muted-foreground mt-1">
            在单一后台统一管理多个 Takealot 卖家店铺，各店铺授权、改价底价规则与巡检调度完全物理隔离。
          </p>
        </div>
        <Button onClick={openAddModal} className="gap-2 shadow-xs shrink-0 h-9 text-xs">
          <Plus className="h-4 w-4" />
          <span>添加新店铺</span>
        </Button>
      </div>

      {/* 1. 多店铺中心卡片 */}
      <Card className="border-border shadow-xs">
        <CardHeader className="pb-3 border-b border-border/60">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <StoreIcon className="h-4 w-4 text-primary" />
              <CardTitle className="text-base font-semibold">
                店铺中心 ({stores.length})
              </CardTitle>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onRefreshStores()}
              className="h-7 text-xs gap-1.5"
            >
              <RefreshCw className="h-3 w-3" />
              <span>刷新店铺列表</span>
            </Button>
          </div>
          <CardDescription className="text-xs">
            系统支持同时接入任意数量的 Takealot 店铺，可通过专属代理 IP 防风控关联。
          </CardDescription>
        </CardHeader>
        <CardContent className="p-4 sm:p-6 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {stores.map((store) => {
              const isCurrent = store.id === (currentStore?.id || '')
              const isTesting = testingStoreId === store.id
              const isSyncing = syncingStoreId === store.id

              return (
                <div
                  key={store.id}
                  className={`relative flex flex-col justify-between rounded-xl border p-4 transition-all duration-200 ${
                    isCurrent
                      ? 'border-primary/60 bg-primary/5 shadow-xs ring-1 ring-primary/20'
                      : 'border-border bg-card hover:border-border/80 hover:shadow-xs'
                  }`}
                >
                  <div className="space-y-3">
                    {/* Top Row: Store Name & Status Badges */}
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-sm text-foreground truncate">
                            {store.name}
                          </h3>
                          {isCurrent && (
                            <Badge variant="default" className="text-[10px] h-4.5 px-1.5">
                              当前操作中
                            </Badge>
                          )}
                        </div>
                        <div className="flex items-center gap-2 mt-0.5 text-[11px] text-muted-foreground font-mono">
                          <span>ID: {store.id}</span>
                          <span>•</span>
                          <span>{store.interval_minutes}分钟轮询</span>
                        </div>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        <span className="text-[11px] text-muted-foreground">巡检:</span>
                        <Switch
                          checked={store.is_active}
                          onCheckedChange={(val) => handleToggleStoreActive(store, val)}
                        />
                      </div>
                    </div>

                    {/* Parameters Details */}
                    <div className="grid grid-cols-3 gap-2 py-2 px-2.5 rounded-lg bg-muted/50 text-[11px]">
                      <div>
                        <span className="text-muted-foreground block text-[10px]">降/提步长</span>
                        <span className="font-semibold font-mono text-foreground">
                          R{store.price_decrease_step} / R{store.price_increase_step}
                        </span>
                      </div>
                      <div>
                        <span className="text-muted-foreground block text-[10px]">建议零售价</span>
                        <span className="font-semibold font-mono text-foreground">
                          {store.rrp_percentage}%
                        </span>
                      </div>
                      <div>
                        <span className="text-muted-foreground block text-[10px]">代理隔离</span>
                        <span className="font-mono text-foreground truncate block">
                          {store.proxy_url ? '独立代理' : '直连'}
                        </span>
                      </div>
                    </div>

                    {/* Proxy display if present */}
                    {store.proxy_url && (
                      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground truncate">
                        <Globe className="h-3 w-3 text-primary shrink-0" />
                        <span className="truncate font-mono">{store.proxy_url}</span>
                      </div>
                    )}
                  </div>

                  {/* Card Bottom Actions */}
                  <div className="flex items-center justify-between pt-3 mt-3 border-t border-border/50 gap-2">
                    <div className="flex items-center gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleTestExistingStore(store)}
                        disabled={isTesting}
                        className="h-7 text-[11px] px-2"
                      >
                        {isTesting ? <RefreshCw className="h-3 w-3 animate-spin" /> : '连通测试'}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => openEditModal(store)}
                        className="h-7 text-[11px] px-2 text-muted-foreground hover:text-foreground"
                      >
                        <Edit2 className="h-3 w-3 mr-1" />
                        编辑
                      </Button>
                      {stores.length > 1 && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDeleteStore(store)}
                          className="h-7 text-[11px] px-2 text-destructive hover:bg-destructive/10"
                        >
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      )}
                    </div>

                    {!isCurrent && (
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => onSelectStore(store.id)}
                        className="h-7 text-[11px] px-2.5"
                      >
                        切换至该店
                      </Button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </CardContent>
      </Card>

      {/* 2. 当前选中店铺独立参数与授权设置 */}
      {currentStore && (
        <Card className="border-border shadow-xs">
          <CardHeader className="pb-3 border-b border-border/60">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Sliders className="h-4 w-4 text-primary" />
                <CardTitle className="text-base font-semibold">
                  当前店铺专属参数设置: [{currentStore.name}]
                </CardTitle>
              </div>
              <Badge variant="outline" className="font-mono text-xs">
                ID: {currentStore.id}
              </Badge>
            </div>
            <CardDescription className="text-xs">
              针对当前所选店铺单独配置改价幅度、轮询周期与默认库存。
            </CardDescription>
          </CardHeader>

          <CardContent className="p-4 sm:p-6">
            <form onSubmit={handleSaveActiveStoreConfig} className="space-y-5">
              {/* Authorization Key input */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                    <Key className="h-3.5 w-3.5 text-primary" />
                    店铺授权凭证 (Authorization Header / API Key)
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => handleSyncStoreName(currentStore)}
                    disabled={syncingStoreId === currentStore.id}
                    className="h-6 text-[11px] text-primary gap-1"
                  >
                    <RefreshCw className={`h-3 w-3 ${syncingStoreId === currentStore.id ? 'animate-spin' : ''}`} />
                    <span>同步官方店铺名称</span>
                  </Button>
                </div>
                <Input
                  type="password"
                  value={activeForm.authorization}
                  onChange={(e) => setActiveForm({ ...activeForm, authorization: e.target.value })}
                  placeholder="例如: Key 389bxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                  className="font-mono text-xs"
                />
              </div>

              {/* Repricing Parameters */}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 pt-2">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    竞品低价时降价幅度 (R)
                  </label>
                  <Input
                    type="number"
                    min="1"
                    value={activeForm.price_decrease_step}
                    onChange={(e) => setActiveForm({ ...activeForm, price_decrease_step: parseInt(e.target.value) || 1 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    当竞品更低时，调降的比竞品少 R 几元以取得 Buy Box
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    竞品涨价时提价幅度 (R)
                  </label>
                  <Input
                    type="number"
                    min="1"
                    value={activeForm.price_increase_step}
                    onChange={(e) => setActiveForm({ ...activeForm, price_increase_step: parseInt(e.target.value) || 1 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    当竞品涨价或无竞品时，自动上浮提价额度
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    零售价 (RRP) 浮动比例 (%)
                  </label>
                  <Input
                    type="number"
                    min="100"
                    step="1"
                    value={activeForm.rrp_percentage}
                    onChange={(e) => setActiveForm({ ...activeForm, rrp_percentage: parseFloat(e.target.value) || 120 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    以改价后售价为基准上浮比例 (例如 120%)
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    自动调价巡检轮询间隔 (分钟)
                  </label>
                  <Input
                    type="number"
                    min="1"
                    value={activeForm.interval_minutes}
                    onChange={(e) => setActiveForm({ ...activeForm, interval_minutes: parseInt(e.target.value) || 5 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    建议设置为 3~10 分钟之间
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    批量跟卖默认铺货库存
                  </label>
                  <Input
                    type="number"
                    min="1"
                    value={activeForm.bulk_stock}
                    onChange={(e) => setActiveForm({ ...activeForm, bulk_stock: parseInt(e.target.value) || 1 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    Excel 批量跟卖未指定库存时的默认库存量
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-foreground">
                    单次拉取商品上限 (Offers)
                  </label>
                  <Input
                    type="number"
                    min="100"
                    step="100"
                    value={activeForm.max_fetch_offers}
                    onChange={(e) => setActiveForm({ ...activeForm, max_fetch_offers: parseInt(e.target.value) || 1000 })}
                    className="text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    拉取自身有效在售商品的最大件数
                  </p>
                </div>
              </div>

              <div className="flex items-center justify-end pt-3 border-t border-border/60">
                <Button type="submit" loading={savingActive} className="gap-2 text-xs">
                  <Save className="h-3.5 w-3.5" />
                  <span>保存当前店铺参数</span>
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {/* 2. 编辑店铺 Dialog */}
      <Dialog
        open={isEditOpen}
        onOpenChange={setIsEditOpen}
        title={`编辑店铺: ${editingStore?.name}`}
        description="修改店铺配置、代理网络及改价规则。"
        maxWidth="lg"
      >
        <form onSubmit={handleSaveEditStore} className="space-y-4 pt-2">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              店铺显示名称
            </label>
            <Input
              required
              value={storeForm.name || ''}
              onChange={(e) => setStoreForm({ ...storeForm, name: e.target.value })}
              className="text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              店铺授权凭据 (Authorization)
            </label>
            <Input
              required
              value={storeForm.authorization || ''}
              onChange={(e) => setStoreForm({ ...storeForm, authorization: e.target.value })}
              className="font-mono text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-foreground">
              独立代理 Proxy URL
            </label>
            <Input
              value={storeForm.proxy_url || ''}
              onChange={(e) => setStoreForm({ ...storeForm, proxy_url: e.target.value })}
              placeholder="http://user:pass@ip:port 或 socks5://..."
              className="font-mono text-xs"
            />
          </div>

          <div className="grid grid-cols-3 gap-3 pt-2">
            <div className="space-y-1">
              <label className="text-[11px] text-muted-foreground">降价步长 (R)</label>
              <Input
                type="number"
                min="1"
                value={storeForm.price_decrease_step || 1}
                onChange={(e) => setStoreForm({ ...storeForm, price_decrease_step: parseInt(e.target.value) || 1 })}
                className="text-xs font-mono h-8"
              />
            </div>
            <div className="space-y-1">
              <label className="text-[11px] text-muted-foreground">提价步长 (R)</label>
              <Input
                type="number"
                min="1"
                value={storeForm.price_increase_step || 1}
                onChange={(e) => setStoreForm({ ...storeForm, price_increase_step: parseInt(e.target.value) || 1 })}
                className="text-xs font-mono h-8"
              />
            </div>
            <div className="space-y-1">
              <label className="text-[11px] text-muted-foreground">巡检周期 (分钟)</label>
              <Input
                type="number"
                min="1"
                value={storeForm.interval_minutes || 5}
                onChange={(e) => setStoreForm({ ...storeForm, interval_minutes: parseInt(e.target.value) || 5 })}
                className="text-xs font-mono h-8"
              />
            </div>
          </div>

          <DialogFooter className="pt-3">
            <Button type="button" variant="outline" onClick={() => setIsEditOpen(false)} className="text-xs">
              取消
            </Button>
            <Button type="submit" loading={savingStore} className="text-xs">
              保存修改
            </Button>
          </DialogFooter>
        </form>
      </Dialog>
    </div>
  )
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { CheckCircle2, ShieldCheck, X, RefreshCw } from 'lucide-react'
import { Button } from '../ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../ui/select'
import { toast } from '../ui/use-toast'
import type { Store } from '../../types'

type SessionStatus = {
  store_id: string
  available: boolean
  enabled: boolean
  phase: string
  message: string
  revision: number
  origins?: { origin: string; phase: string }[]
}
const SELLER = 'https://seller-api.takealot.com'
const PUBLIC = 'https://api.takealot.com'
const bridge = () => (window as any).go?.main?.App

export function BrowserSessions({ stores, currentStoreId, eligible }: {
  stores: Store[]; currentStoreId: string; eligible: boolean
}) {
  const [statuses, setStatuses] = useState<SessionStatus[]>([])
  const [open, setOpen] = useState(false)
  const [storeId, setStoreId] = useState('')
  const [storeMenuOpen, setStoreMenuOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [viewError, setViewError] = useState('')
  const sellerViewport = useRef<HTMLDivElement>(null)
  const publicViewport = useRef<HTMLDivElement>(null)
  const seen = useRef(new Set<string>())
  const selected = statuses.find(s => s.store_id === storeId)
  const pending = statuses.filter(s => s.phase === 'direct_blocked' || (s.enabled && s.phase !== 'ready' && s.phase !== 'disabled'))
  const desktop = !!bridge()

  const refresh = useCallback(async () => {
    if (bridge()) setStatuses(await bridge().BrowserSessionStatuses())
  }, [])

  useEffect(() => {
    if (!eligible || !desktop) { setOpen(false); setStatuses([]); bridge()?.HideBrowserSession(); return }
    refresh().catch(() => {})
    const timer = setInterval(() => refresh().catch(() => {}), 2000)
    return () => clearInterval(timer)
  }, [eligible, desktop, refresh])

  useEffect(() => {
    if (open || !eligible) return
    const next = pending.find(s => !seen.current.has(`${s.store_id}:${s.revision}:${s.phase}`))
    if (next) {
      seen.current.add(`${next.store_id}:${next.revision}:${next.phase}`)
      setStoreId(next.store_id)
      setOpen(true)
    }
  }, [statuses, open, eligible])

  useEffect(() => {
    if (open && selected) seen.current.add(`${selected.store_id}:${selected.revision}:${selected.phase}`)
  }, [open, selected])

  useEffect(() => {
    if (!open || storeMenuOpen || !selected?.enabled || !sellerViewport.current || !publicViewport.current) return
    let cancelled = false
    let frame = 0
    setViewError('')
    const measure = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const sellerRect = sellerViewport.current?.getBoundingClientRect()
        const publicRect = publicViewport.current?.getBoundingClientRect()
        if (!sellerRect || !publicRect || cancelled) return
        const bounds = (rect: DOMRect) => ({
          x: rect.x + 1, y: rect.y + 1, width: rect.width - 2, height: rect.height - 2,
          viewport_width: window.innerWidth, viewport_height: window.innerHeight,
        })
        bridge().ShowBrowserSessionPanels(storeId, bounds(sellerRect), bounds(publicRect))
          .catch((err: any) => { if (!cancelled) setViewError(String(err)) })
      })
    }
    const observer = new ResizeObserver(measure)
    observer.observe(sellerViewport.current)
    observer.observe(publicViewport.current)
    window.addEventListener('resize', measure)
    measure()
    return () => {
      cancelled = true
      cancelAnimationFrame(frame)
      observer.disconnect()
      window.removeEventListener('resize', measure)
      bridge()?.HideBrowserSession()
    }
  }, [open, storeMenuOpen, storeId, selected?.enabled])

  const close = () => { setOpen(false); setStoreMenuOpen(false); bridge()?.HideBrowserSession() }
  useEffect(() => {
    if (!open) return
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') close() }
    window.addEventListener('keydown', escape)
    return () => window.removeEventListener('keydown', escape)
  }, [open])

  const action = async (work: () => Promise<unknown>, success?: string) => {
    setBusy(true)
    try { await work(); if (success) toast.success(success) }
    catch (err: any) { toast.error('访问会话', String(err)) }
    finally { await refresh().catch(() => {}); setBusy(false) }
  }

  if (!desktop || !eligible) return null
  return <>
      <Button variant="outline" size="sm"
        className={`h-8 text-xs gap-1.5 shrink-0 ${pending.length ? 'border-amber-500 text-amber-700 bg-amber-50 hover:bg-amber-100 dark:text-amber-300 dark:bg-amber-950 dark:hover:bg-amber-900' : ''}`}
        title={pending.length ? `${pending.length} 个店铺需要检查访问会话` : '在应用内完成网页验证并保持连接'}
        aria-label={pending.length ? `访问会话，${pending.length} 个店铺待验证` : '访问会话'}
        onClick={() => {
        setStoreId(stores.some(s => s.id === currentStoreId) ? currentStoreId : stores[0]?.id || '')
        setOpen(true)
      }}><ShieldCheck className="h-3.5 w-3.5" />访问会话
        {!!pending.length && <span className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-amber-600 px-1 text-[10px] font-semibold text-white">{pending.length}</span>}
      </Button>
    {open && createPortal(<div className="fixed inset-0 z-[60] bg-black/60 flex items-center justify-center p-5" role="presentation" onDoubleClick={event => event.stopPropagation()}>
      <section role="dialog" aria-modal="true" aria-label="店铺访问会话" className="bg-background text-foreground border border-border rounded-xl shadow-xl w-full max-w-6xl h-[85vh] min-h-[480px] flex flex-col overflow-hidden">
        <div className="flex items-center justify-between gap-4 px-5 py-4 border-b border-border">
          <div><h2 className="font-semibold">店铺访问会话</h2><p className="text-xs text-muted-foreground mt-1">完成网页验证后检查连接；运行中的巡检会重新比价并继续。</p></div>
          <button aria-label="关闭访问会话" className="p-2 rounded-md hover:bg-muted" onClick={close}><X className="w-4 h-4" /></button>
        </div>
        <div className="px-5 py-3 flex items-center gap-3 flex-wrap">
          <Select value={storeId} open={storeMenuOpen} onOpenChange={setStoreMenuOpen} onValueChange={value => { bridge()?.HideBrowserSession(); setStoreId(value) }} disabled={busy || !stores.length}>
            <SelectTrigger aria-label="验证店铺" className="h-8 w-44">
              <SelectValue placeholder="选择店铺" />
            </SelectTrigger>
            <SelectContent className="z-[70]" onEscapeKeyDown={event => event.stopPropagation()}>
              {stores.map(store => <SelectItem key={store.id} value={store.id}>{store.name}</SelectItem>)}
            </SelectContent>
          </Select>
          <div className="flex-1" />
          <Button size="sm" variant="outline" loading={busy} disabled={!selected?.available || !storeId} onClick={() => action(() => bridge().ConfigureBrowserSession(storeId, !selected?.enabled))}>
            {selected?.enabled ? '切换为直接请求' : '启用持久会话'}
          </Button>
        </div>
        {selected?.enabled && selected.phase === 'ready' ? (
          <div role="status" aria-live="polite" className="mx-5 mb-4 flex shrink-0 items-center gap-3 rounded-lg border-2 border-emerald-500 bg-emerald-50 px-4 py-3 text-emerald-950 dark:bg-emerald-950 dark:text-emerald-50">
            <CheckCircle2 aria-hidden="true" className="h-8 w-8 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <div className="flex-1 min-w-0">
              <p className="text-base font-semibold">验证通过，可以关闭此面板</p>
              <p className="mt-1 text-sm">卖家接口和商品接口均已验证。关闭后会保留会话，供后台继续使用。</p>
            </div>
            <Button className="shrink-0 bg-emerald-700 text-white hover:bg-emerald-800" onClick={close}>完成并关闭</Button>
          </div>
        ) : (
          <div className="px-5 pb-3 text-xs text-muted-foreground" aria-live="polite">{selected?.message || '正在读取会话状态…'}</div>
        )}
        <div className="mx-5 grid grid-cols-2 gap-4 flex-1 min-h-[200px]">
          {[{ origin: SELLER, title: '卖家接口', viewport: sellerViewport }, { origin: PUBLIC, title: '商品接口', viewport: publicViewport }].map(panel => (
            <section key={panel.origin} aria-label={panel.title} className="flex flex-col min-w-0 min-h-0 gap-2">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0"><h3 className="text-sm font-medium">{panel.title}</h3><p className="text-xs text-muted-foreground truncate">{panel.origin.replace('https://', '')}</p></div>
                <Button variant="outline" size="sm" disabled={!selected?.enabled || busy} aria-label={`重载${panel.title}`} onClick={() => action(() => bridge().ReloadBrowserSession(storeId, panel.origin))}><RefreshCw className="w-3 h-3 mr-1" />重载</Button>
              </div>
              <div ref={panel.viewport} className="flex-1 min-h-0 rounded-md border border-border bg-muted/30 flex items-center justify-center p-3 text-center text-sm text-muted-foreground">
                {viewError || (!selected?.enabled ? '启用持久会话后，在此完成人工验证。' : storeMenuOpen ? '请选择店铺' : '正在载入接口页面…')}
              </div>
            </section>
          ))}
        </div>
        <div className="px-5 py-4 flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">页面出现权限提示时，请点击“检查并恢复”校验店铺授权。关闭面板会保留会话。</p>
          <div className="flex gap-2 shrink-0">
            <Button size="sm" loading={busy} disabled={!selected?.enabled} onClick={() => action(() => bridge().CheckBrowserSession(storeId), '卖家接口和商品接口均已验证，可以关闭此面板')}>检查并恢复</Button>
          </div>
        </div>
      </section>
    </div>, document.body)}
  </>
}

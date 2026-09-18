import React, { useState, useEffect, useRef } from 'react'
import { api } from '../../api/client'
import type { LogEntry } from '../../types'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../ui/card'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs'
import { Switch } from '../ui/switch'
import {
  Terminal,
  Search,
  Trash2,
  Copy,
  Check,
  ArrowDown,
  RefreshCw,
  Radio,
} from 'lucide-react'

export const LogsTab: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [levelFilter, setLevelFilter] = useState<'ALL' | 'SUCCESS' | 'INFO' | 'WARN' | 'ERROR'>('ALL')
  const [searchQuery, setSearchQuery] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [connected, setConnected] = useState(false)
  const [copied, setCopied] = useState(false)

  const logContainerRef = useRef<HTMLDivElement>(null)

  // Fetch initial history
  useEffect(() => {
    api.getLogsHistory().then((data) => {
      if (Array.isArray(data)) setLogs(data)
    }).catch(console.error)
  }, [])

  // Establish SSE stream
  useEffect(() => {
    let eventSource: EventSource | null = null
    try {
      eventSource = new EventSource('/api/logs/stream')
      eventSource.onopen = () => setConnected(true)
      eventSource.onerror = () => setConnected(false)

      eventSource.onmessage = (e) => {
        try {
          const entry: LogEntry = JSON.parse(e.data)
          setLogs((prev) => {
            // deduplicate if same time & message
            const last = prev[prev.length - 1]
            if (last && last.time === entry.time && last.message === entry.message) {
              return prev
            }
            return [...prev.slice(-499), entry]
          })
        } catch {
          // ignore non-json
        }
      }
    } catch (e) {
      console.error('SSE connection error:', e)
    }

    return () => {
      if (eventSource) eventSource.close()
    }
  }, [])

  // Fallback polling if SSE is disconnected
  useEffect(() => {
    if (connected) return
    const interval = setInterval(() => {
      api.getLogsHistory().then((data) => {
        if (Array.isArray(data)) setLogs(data)
      }).catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [connected])

  // Auto scroll to bottom
  useEffect(() => {
    if (autoScroll && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [logs, autoScroll])

  const filteredLogs = logs.filter((log) => {
    if (levelFilter !== 'ALL' && log.level !== levelFilter) return false
    if (searchQuery.trim() && !log.message.toLowerCase().includes(searchQuery.toLowerCase())) {
      return false
    }
    return true
  })

  const handleCopyLogs = () => {
    const text = filteredLogs.map((l) => `[${l.time}] [${l.level}] ${l.message}`).join('\n')
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const getLevelBadge = (level: string) => {
    switch (level) {
      case 'SUCCESS':
        return <Badge variant="success" className="text-[10px] px-1 py-0">SUCCESS</Badge>
      case 'WARN':
        return <Badge variant="warning" className="text-[10px] px-1 py-0">WARN</Badge>
      case 'ERROR':
        return <Badge variant="destructive" className="text-[10px] px-1 py-0">ERROR</Badge>
      default:
        return <Badge variant="secondary" className="text-[10px] px-1 py-0">INFO</Badge>
    }
  }

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
              实时运行引擎日志 (Live Logs)
            </h2>
            <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-mono bg-muted border border-border">
              <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500 animate-pulse' : 'bg-rose-500'}`} />
              <span className="text-[11px] text-muted-foreground">{connected ? '实时流式接收中' : '连接离线'}</span>
            </div>
          </div>
          <p className="text-xs sm:text-sm text-muted-foreground mt-0.5">
            Server-Sent Events (SSE) 毫秒级推送调价巡检、竞价雷达与批量跟卖上架日志
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleCopyLogs} className="gap-1.5 text-xs">
            {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
            <span>{copied ? '已复制' : '复制日志'}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setLogs([])}
            className="gap-1.5 text-xs text-rose-500"
          >
            <Trash2 className="h-3.5 w-3.5" />
            <span>清屏</span>
          </Button>
        </div>
      </div>

      {/* Filter & Controls Bar */}
      <Card>
        <CardContent className="p-3 sm:p-4 flex flex-col md:flex-row items-center justify-between gap-3">
          <div className="relative w-full md:w-80">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="搜索日志关键词..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 h-9 text-xs"
            />
          </div>

          <div className="flex flex-wrap items-center gap-4 w-full md:w-auto justify-between md:justify-end">
            {/* Level Filter Tabs */}
            <Tabs value={levelFilter} onValueChange={(v) => setLevelFilter(v as any)}>
              <TabsList className="h-8 p-0.5">
                {(['ALL', 'SUCCESS', 'INFO', 'WARN', 'ERROR'] as const).map((lvl) => (
                  <TabsTrigger key={lvl} value={lvl} className="text-[11px] h-7 px-2.5 font-mono">
                    {lvl}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>

            {/* Auto Scroll Switch */}
            <div className="flex items-center gap-2">
              <Switch
                id="auto-scroll-toggle"
                checked={autoScroll}
                onCheckedChange={setAutoScroll}
              />
              <label
                htmlFor="auto-scroll-toggle"
                className="text-xs text-muted-foreground cursor-pointer select-none"
              >
                锁定吸底滚动
              </label>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Terminal View Container */}
      <Card className="border-border/90 bg-slate-950 text-slate-100 overflow-hidden shadow-xl rounded-2xl">
        {/* Terminal Header */}
        <div className="flex items-center justify-between px-4 py-2.5 bg-slate-900 border-b border-slate-800 text-xs font-mono text-slate-400">
          <div className="flex items-center gap-2">
            <div className="flex gap-1.5">
              <div className="w-3 h-3 rounded-full bg-rose-500/80" />
              <div className="w-3 h-3 rounded-full bg-amber-500/80" />
              <div className="w-3 h-3 rounded-full bg-emerald-500/80" />
            </div>
            <span className="text-slate-300 font-medium ml-2">takealot-engine.log</span>
          </div>
          <div>共 {filteredLogs.length} 条记录</div>
        </div>

        {/* Terminal Body */}
        <div
          ref={logContainerRef}
          className="p-4 h-[480px] sm:h-[540px] overflow-y-auto font-mono text-xs leading-relaxed space-y-1"
        >
          {filteredLogs.length === 0 ? (
            <div className="h-full flex items-center justify-center text-slate-500">
              暂无日志输出，等待自动化调价引擎触发...
            </div>
          ) : (
            filteredLogs.map((log, index) => (
              <div
                key={index}
                className="flex items-start gap-2 hover:bg-slate-900/60 px-1.5 py-0.5 rounded transition-colors group"
              >
                <span className="text-slate-500 shrink-0 select-none text-[11px]">
                  {log.time}
                </span>
                <span className="shrink-0">{getLevelBadge(log.level)}</span>
                <span className="text-slate-200 break-all select-text">{log.message}</span>
              </div>
            ))
          )}
        </div>
      </Card>
    </div>
  )
}

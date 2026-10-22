import type { LogEntry } from '../types'

export function formatLogEntry(entry: LogEntry): string {
  const store = [entry.store_name, entry.store_id].filter(Boolean).join(' / ')
  return `[${entry.timestamp || entry.time}] [${entry.level}]${store ? ` [店铺: ${store}]` : ''} ${entry.message}`
}

export function buildLogExport(entries: LogEntry[], level: string, query: string, now: Date): string {
  return [
    'Takealot 掌柜运行日志',
    `导出时间: ${now.toISOString()}`,
    `筛选级别: ${level}`,
    `搜索关键词: ${query.trim() || '无'}`,
    `记录数: ${entries.length}`,
    '范围: 当前界面筛选结果，最多保留最近 500 条运行日志，重启后清空。',
    '',
    ...entries.map(formatLogEntry),
    '',
  ].join('\r\n')
}

export async function saveLogExport(content: string, now: Date): Promise<string | null> {
  const desktop = window as Window & {
    go?: { main?: { App?: { ExportLogs?: (content: string) => Promise<string> } } }
  }
  const nativeExport = desktop.go?.main?.App?.ExportLogs
  if (nativeExport) {
    return (await nativeExport(content)) || null
  }

  const stamp = now.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z')
  const filename = `takealot-logs-${stamp}.txt`
  const blob = new Blob(['\uFEFF', content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.style.display = 'none'
  document.body.appendChild(link)
  try {
    link.click()
  } finally {
    link.remove()
    // Leave time for the browser to consume the object URL before releasing it.
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return filename
}

import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatCurrency(amount: number | string | undefined | null): string {
  if (amount === undefined || amount === null) return "R 0"
  const val = typeof amount === "string" ? parseFloat(amount) : amount
  if (isNaN(val)) return "R 0"
  return `R ${val.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 })}`
}

export function formatDateTime(dateStr: string | undefined | null): string {
  if (!dateStr) return "-"
  try {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return dateStr
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  } catch {
    return dateStr
  }
}

/**
 * Open URL in the default system browser across Wails desktop GUI and Web modes
 */
export function openExternalURL(url: string) {
  if (!url) return
  const fullUrl = /^https?:\/\//i.test(url) ? url : `https://${url}`

  // 1. Wails desktop runtime (runs in Wails GUI on macOS and Windows)
  const rt = (window as any).runtime
  if (typeof rt?.BrowserOpenURL === 'function') {
    rt.BrowserOpenURL(fullUrl)
    return
  }

  // 2. Call backend open_browser API to launch native system browser
  fetch(`/api/open_browser?url=${encodeURIComponent(fullUrl)}`).catch(() => {})

  // 3. Fallback for pure browser mode
  try {
    window.open(fullUrl, '_blank', 'noopener,noreferrer')
  } catch {
    // ignore
  }
}


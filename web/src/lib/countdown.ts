import { useState, useEffect } from 'react'

/**
 * 格式化简单分秒倒计时 (如 01:23)
 */
export function formatMinutesSeconds(seconds: number): string {
  if (seconds <= 0) return '00:00'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

/**
 * 格式化详细倒计时 (如 剩余 1天21小时36分50秒 / 已逾期 02小时15分)
 */
export function formatDetailedCountdown(diffSeconds: number): {
  text: string
  isOverdue: boolean
  remainingSeconds: number
} {
  const isOverdue = diffSeconds <= 0
  const absSec = Math.abs(Math.floor(diffSeconds))

  const days = Math.floor(absSec / 86400)
  const hours = Math.floor((absSec % 86400) / 3600)
  const minutes = Math.floor((absSec % 3600) / 60)
  const seconds = Math.floor(absSec % 60)

  let text = ''
  if (isOverdue) {
    if (days > 0) {
      text = `已逾期 ${days}天${hours.toString().padStart(2, '0')}小时${minutes.toString().padStart(2, '0')}分`
    } else {
      text = `已逾期 ${hours.toString().padStart(2, '0')}小时${minutes.toString().padStart(2, '0')}分${seconds.toString().padStart(2, '0')}秒`
    }
  } else {
    if (days > 0) {
      text = `剩余 ${days}天${hours.toString().padStart(2, '0')}小时${minutes.toString().padStart(2, '0')}分${seconds.toString().padStart(2, '0')}秒`
    } else {
      text = `剩余 ${hours.toString().padStart(2, '0')}小时${minutes.toString().padStart(2, '0')}分${seconds.toString().padStart(2, '0')}秒`
    }
  }

  return { text, isOverdue, remainingSeconds: diffSeconds }
}

/**
 * 通用单值倒计时 Hook（本地每秒递减 1 秒）
 */
export function useCountdown(initialSeconds: number, active = true) {
  const [seconds, setSeconds] = useState(initialSeconds)

  // 当外部初始值发生校准时同步
  useEffect(() => {
    setSeconds(initialSeconds)
  }, [initialSeconds])

  useEffect(() => {
    if (!active || seconds <= 0) return

    const timer = setInterval(() => {
      setSeconds((prev) => Math.max(0, prev - 1))
    }, 1000)

    return () => clearInterval(timer)
  }, [active, seconds])

  return {
    seconds,
    formatted: formatMinutesSeconds(seconds),
    setSeconds,
  }
}

/**
 * 通用全局心跳 Tick Hook（驱动整页时间戳实时计算，每秒跳动一次）
 */
export function useTicker(intervalMs = 1000, enabled = true) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!enabled) return
    const timer = setInterval(() => {
      setNow(Date.now())
    }, intervalMs)
    return () => clearInterval(timer)
  }, [intervalMs, enabled])

  return now
}

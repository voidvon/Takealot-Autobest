import React, { useState } from 'react'
import { api } from '../../api/client'
import type { LicenseStatus } from '../../types'
import { Dialog, DialogFooter } from '../ui/dialog'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { KeyRound, Copy, Check, ShieldCheck, AlertTriangle, Clock, Store, UserCheck } from 'lucide-react'

interface LicenseModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  status: LicenseStatus | null
  onActivated: (newStatus: LicenseStatus) => void
}

export const LicenseModal: React.FC<LicenseModalProps> = ({
  open,
  onOpenChange,
  status,
  onActivated,
}) => {
  const [copied, setCopied] = useState(false)
  const [licenseInput, setLicenseInput] = useState('')
  const [activating, setActivating] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [showRenew, setShowRenew] = useState(false)

  const isActivated = status?.activated && !status?.expired

  const handleCopyMachineID = () => {
    if (!status?.machine_id) return
    navigator.clipboard.writeText(status.machine_id)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleActivate = async (e: React.FormEvent) => {
    e.preventDefault()
    const key = licenseInput.trim()
    if (!key) {
      setErrorMsg('请输入完整的软件激活码')
      return
    }

    setActivating(true)
    setErrorMsg('')
    try {
      const res = await api.activateLicense(key)
      if (res.success && res.license) {
        onActivated(res.license)
        setLicenseInput('')
        setShowRenew(false)
        if (res.license.activated) {
          // If valid, close after a brief moment
          setTimeout(() => {
            onOpenChange(false)
          }, 800)
        }
      } else {
        setErrorMsg(res.error || '激活失败，请核对激活码')
      }
    } catch (err: any) {
      setErrorMsg(err.message || '激活失败，请核对激活码')
    } finally {
      setActivating(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(val) => {
        // If unactivated or expired, prevent accidental clicking away
        if (!isActivated && !val) {
          // allow closing only if user explicitly clicks cancel, but warn
          onOpenChange(val)
          return
        }
        onOpenChange(val)
      }}
      title="软件授权与激活中心"
      description="基于硬件绑定的全离线商业授权机制，安全保障与版本管理。"
      maxWidth="lg"
    >
      <div className="space-y-4 pt-1">
        {/* Machine ID Box */}
        <div className="rounded-lg border border-border bg-muted/40 p-3.5 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-foreground flex items-center gap-1.5">
              <KeyRound className="w-3.5 h-3.5 text-primary" />
              本机唯一机器识别码 (Machine ID)
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleCopyMachineID}
              className="h-7 text-xs gap-1"
            >
              {copied ? (
                <>
                  <Check className="w-3.5 h-3.5 text-emerald-500" />
                  <span className="text-emerald-600 font-medium">已复制</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5" />
                  <span>复制机器码</span>
                </>
              )}
            </Button>
          </div>
          <div className="font-mono text-sm font-bold tracking-wider text-primary bg-background border border-border rounded-md px-3 py-2 text-center select-all">
            {status?.machine_id || '正在读取硬件识别码...'}
          </div>
          <p className="text-[11px] text-muted-foreground leading-tight">
            💡 本识别码由主板与系统安全指纹生成，不包含任何个人隐私数据。
          </p>
        </div>

        {/* Activated State Display */}
        {isActivated && !showRenew ? (
          <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <ShieldCheck className="w-5 h-5 text-emerald-600" />
                <span className="font-semibold text-sm text-emerald-950 dark:text-emerald-200">
                  软件已完成正版激活
                </span>
              </div>
              <Badge variant="success" className="text-xs">
                授权有效
              </Badge>
            </div>

            <div className="grid grid-cols-2 gap-2.5 text-xs pt-1">
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <UserCheck className="w-3.5 h-3.5" />
                <span>授权对象:</span>
                <span className="font-medium text-foreground">{status?.customer || '尊贵客户'}</span>
              </div>
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <Store className="w-3.5 h-3.5" />
                <span>店铺配额:</span>
                <span className="font-medium text-foreground">
                  {status?.max_stores && status.max_stores > 0 ? `${status.max_stores} 家` : '不限制'}
                </span>
              </div>
              <div className="flex items-center gap-1.5 text-muted-foreground col-span-2">
                <Clock className="w-3.5 h-3.5" />
                <span>有效期至:</span>
                <span className="font-mono font-medium text-foreground">
                  {status?.expires_at_formatted || '永久买断'}
                </span>
                {status?.days_left !== undefined && status.days_left >= 0 && (
                  <span className="text-primary font-semibold">({status.days_left} 天后到期)</span>
                )}
              </div>
            </div>

            <div className="pt-2 border-t border-emerald-500/20 flex justify-end">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-xs text-muted-foreground hover:text-foreground h-7"
                onClick={() => setShowRenew(true)}
              >
                输入新激活码续期/更换
              </Button>
            </div>
          </div>
        ) : (
          /* Unactivated / Expired / Renewal Form */
          <div className="space-y-3">
            {status?.expired && (
              <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 p-3 flex items-start gap-2.5 text-xs text-rose-800 dark:text-rose-300">
                <AlertTriangle className="w-4 h-4 shrink-0 text-rose-600 mt-0.5" />
                <div>
                  <div className="font-semibold">当前授权已到期或异常</div>
                  <div className="text-[11px] opacity-90">{status?.message || '请获取新的激活码后重新激活'}</div>
                </div>
              </div>
            )}

            {!isActivated && !status?.expired && (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-900 dark:text-amber-200 space-y-1">
                <div className="font-semibold flex items-center gap-1.5">
                  <span>📋 离线激活流程指南:</span>
                </div>
                <ol className="list-decimal list-inside space-y-0.5 text-[11px] opacity-90 pl-1">
                  <li>点击上方【复制机器码】，发送给软件作者/管理员。</li>
                  <li>管理员将为您签发专属的离线激活码。</li>
                  <li>将收到的激活码完整粘贴至下方，点击【立即激活】。</li>
                </ol>
              </div>
            )}

            <form onSubmit={handleActivate} className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-xs font-semibold text-foreground flex items-center justify-between">
                  <span>离线激活码 (Activation Key) *</span>
                  {showRenew && isActivated && (
                    <button
                      type="button"
                      onClick={() => setShowRenew(false)}
                      className="text-[11px] text-muted-foreground hover:underline"
                    >
                      返回当前授权状态
                    </button>
                  )}
                </label>
                <textarea
                  value={licenseInput}
                  onChange={(e) => setLicenseInput(e.target.value)}
                  placeholder="在此处粘贴激活码 (形如: TKACT-eyJtaWQiOi...)"
                  rows={3}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-xs font-mono ring-offset-background placeholder:text-muted-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                  required
                />
              </div>

              {errorMsg && (
                <div className="text-xs text-rose-600 dark:text-rose-400 flex items-center gap-1.5">
                  <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                  <span>{errorMsg}</span>
                </div>
              )}

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => onOpenChange(false)}
                >
                  关闭
                </Button>
                <Button
                  type="submit"
                  variant="default"
                  size="sm"
                  loading={activating}
                  className="gap-1.5"
                >
                  <KeyRound className="w-3.5 h-3.5" />
                  <span>立即激活</span>
                </Button>
              </DialogFooter>
            </form>
          </div>
        )}
      </div>
    </Dialog>
  )
}

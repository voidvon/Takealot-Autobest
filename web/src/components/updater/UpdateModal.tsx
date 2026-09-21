import React, { useState, useEffect, useRef } from 'react'
import { api } from '../../api/client'
import type { UpdateInfo, UpdateProgress } from '../../types'
import { Dialog } from '../ui/dialog'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Switch } from '../ui/switch'
import { openExternalURL } from '../../lib/utils'
import {
  Rocket,
  Download,
  RotateCw,
  ExternalLink,
  AlertCircle,
  CheckCircle2,
  Zap,
  Calendar,
  HardDrive,
} from 'lucide-react'

interface UpdateModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  updateInfo: UpdateInfo | null
  currentVersion: string
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '未知大小'
  const mb = bytes / (1024 * 1024)
  return `${mb.toFixed(2)} MB`
}

export const UpdateModal: React.FC<UpdateModalProps> = ({
  open,
  onOpenChange,
  updateInfo,
  currentVersion,
}) => {
  const [useProxy, setUseProxy] = useState(true)
  const [isUpdating, setIsUpdating] = useState(false)
  const [progress, setProgress] = useState<UpdateProgress | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const pollTimerRef = useRef<number | null>(null)

  // Clear state when modal closes
  useEffect(() => {
    if (!open) {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
        pollTimerRef.current = null
      }
      setIsUpdating(false)
      setProgress(null)
      setErrorMsg(null)
    }
  }, [open])

  const handleStartUpdate = async () => {
    if (!updateInfo) return
    setIsUpdating(true)
    setErrorMsg(null)

    try {
      await api.applyUpdate({
        proxy: useProxy,
        download_url: updateInfo.asset_url,
      })

      // Start polling progress
      if (pollTimerRef.current) clearInterval(pollTimerRef.current)
      pollTimerRef.current = setInterval(async () => {
        try {
          const prog = await api.getUpdateProgress()
          setProgress(prog)

          if (prog.status === 'error') {
            if (pollTimerRef.current) clearInterval(pollTimerRef.current)
            setIsUpdating(false)
            setErrorMsg(prog.error || prog.message || '更新出现异常')
          } else if (prog.status === 'restarting') {
            if (pollTimerRef.current) clearInterval(pollTimerRef.current)
          }
        } catch (e) {
          // If server restarted and request failed, it means process exited to launch new version!
          if (progress?.status === 'ready' || progress?.status === 'restarting') {
            if (pollTimerRef.current) clearInterval(pollTimerRef.current)
          }
        }
      }, 300)
    } catch (err: any) {
      setIsUpdating(false)
      setErrorMsg(err.message || '触发自动更新失败')
    }
  }

  const handleOpenBrowser = () => {
    const targetUrl = updateInfo?.asset_url || updateInfo?.html_url
    if (targetUrl) {
      openExternalURL(targetUrl)
    }
  }

  const newVer = updateInfo?.version || updateInfo?.tag_name || '新版本'
  const isBusy = isUpdating && progress?.status !== 'error'

  return (
    <Dialog
      open={open}
      onOpenChange={(val) => {
        if (!isBusy) {
          onOpenChange(val)
        }
      }}
      title="🚀 发现新版本更新"
      description={`Takealot 掌柜已发布最新版本，建议及时更新以获得最新特性与稳定性改进。`}
      maxWidth="md"
    >
      <div className="space-y-4 pt-1">
        {/* Version Compare Banner */}
        <div className="flex items-center justify-between p-3.5 rounded-xl border border-primary/20 bg-primary/5 text-xs">
          <div className="flex items-center gap-3 min-w-0">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-xs shrink-0">
              <Rocket className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-foreground text-sm truncate">
                  v{newVer.replace(/^v/, '')}
                </span>
                <Badge variant="success" className="text-[10px] px-1.5 py-0 h-4 shrink-0">
                  最新发布
                </Badge>
              </div>
              <div className="text-[11px] text-muted-foreground flex items-center gap-2 mt-0.5">
                <span>当前版本: v{currentVersion.replace(/^v/, '')}</span>
                {updateInfo?.asset_size ? (
                  <>
                    <span>•</span>
                    <span className="flex items-center gap-1">
                      <HardDrive className="h-3 w-3 inline" />
                      {formatBytes(updateInfo.asset_size)}
                    </span>
                  </>
                ) : null}
              </div>
            </div>
          </div>

          {updateInfo?.published_at && (
            <div className="hidden sm:flex flex-col items-end text-[11px] text-muted-foreground shrink-0 pl-2">
              <span className="flex items-center gap-1">
                <Calendar className="h-3 w-3" />
                {updateInfo.published_at.split(' ')[0]}
              </span>
            </div>
          )}
        </div>

        {/* Release Notes */}
        <div className="space-y-1.5">
          <div className="text-xs font-semibold text-foreground flex items-center justify-between">
            <span>更新日志与特性:</span>
            {updateInfo?.html_url && (
              <button
                type="button"
                onClick={handleOpenBrowser}
                className="text-[11px] text-primary hover:underline flex items-center gap-1 cursor-pointer"
              >
                <span>在 GitHub 查看</span>
                <ExternalLink className="h-3 w-3" />
              </button>
            )}
          </div>
          <div className="rounded-lg border border-border/80 bg-muted/30 p-3 text-xs text-muted-foreground max-h-44 overflow-y-auto whitespace-pre-wrap leading-relaxed font-sans select-text">
            {updateInfo?.release_notes
              ? updateInfo.release_notes
              : '• 性能优化与已知问题修复\n• 提升跨平台运行稳定性与比价轮询效率'}
          </div>
        </div>

        {/* Network Acceleration Toggle */}
        {!isBusy && (
          <div className="flex items-center justify-between p-2.5 rounded-lg border border-border/60 bg-card text-xs">
            <div className="space-y-0.5">
              <div className="flex items-center gap-1.5 font-medium text-foreground">
                <Zap className="h-3.5 w-3.5 text-amber-500 fill-amber-500/20" />
                <span>使用国内下载加速通道</span>
              </div>
              <p className="text-[11px] text-muted-foreground">
                推荐开启。使用高速镜像节点代理 GitHub 资产，防止网络超时中断
              </p>
            </div>
            <Switch
              checked={useProxy}
              onCheckedChange={setUseProxy}
            />
          </div>
        )}

        {/* Error Notice */}
        {errorMsg && (
          <div className="flex items-start gap-2 p-3 text-xs rounded-lg border border-destructive/30 bg-destructive/10 text-destructive animate-in fade-in-50">
            <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
            <div className="space-y-1">
              <p className="font-semibold">更新中断</p>
              <p className="text-[11px] leading-snug">{errorMsg}</p>
            </div>
          </div>
        )}

        {/* Live Download & Restart Progress Bar */}
        {isBusy && (
          <div className="rounded-xl border border-primary/30 bg-primary/5 p-4 space-y-3 animate-in fade-in-50">
            <div className="flex items-center justify-between text-xs">
              <div className="flex items-center gap-2 font-medium text-foreground">
                <RotateCw className="h-3.5 w-3.5 animate-spin text-primary" />
                <span>
                  {progress?.status === 'ready' || progress?.status === 'restarting'
                    ? '准备完成，正在重启应用...'
                    : progress?.status === 'extracting'
                    ? '正在解压并替换更新文件...'
                    : '正在下载最新版本更新包...'}
                </span>
              </div>
              <span className="font-mono font-semibold text-primary">
                {progress?.percent ? `${progress.percent.toFixed(1)}%` : '0.0%'}
              </span>
            </div>

            {/* Progress track */}
            <div className="w-full bg-muted rounded-full h-2.5 overflow-hidden">
              <div
                className="bg-primary h-2.5 rounded-full transition-all duration-300 ease-out relative overflow-hidden"
                style={{ width: `${Math.min(100, Math.max(0, progress?.percent || 0))}%` }}
              >
                <div className="absolute inset-0 bg-white/20 animate-[pulse_1.5s_infinite]" />
              </div>
            </div>

            {/* Meta stats */}
            <div className="flex items-center justify-between text-[11px] text-muted-foreground font-mono">
              <span>
                {progress?.downloaded ? formatBytes(progress.downloaded) : '0 MB'} /{' '}
                {progress?.total ? formatBytes(progress.total) : formatBytes(updateInfo?.asset_size || 0)}
              </span>
              {progress?.speed && <span>{progress.speed}</span>}
            </div>

            <p className="text-[11px] text-muted-foreground text-center">
              💡 下载解压完毕后，软件将平滑退出并自动拉起全新版本
            </p>
          </div>
        )}

        {/* Action Buttons */}
        <div className="flex items-center justify-between pt-2 border-t border-border/60">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={handleOpenBrowser}
            disabled={isBusy}
            className="text-xs text-muted-foreground hover:text-foreground gap-1.5 h-8"
          >
            <Download className="h-3.5 w-3.5" />
            <span>浏览器下载</span>
          </Button>

          <div className="flex items-center gap-2">
            {!isBusy && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => onOpenChange(false)}
                className="text-xs h-8"
              >
                稍后提醒
              </Button>
            )}

            <Button
              type="button"
              variant="default"
              size="sm"
              onClick={handleStartUpdate}
              loading={isBusy}
              disabled={isBusy}
              className="text-xs gap-1.5 h-8 px-4 font-semibold shadow-xs"
            >
              {isBusy ? (
                <span>正在自动更新...</span>
              ) : (
                <>
                  <RotateCw className="h-3.5 w-3.5" />
                  <span>立即更新并重启</span>
                </>
              )}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  )
}

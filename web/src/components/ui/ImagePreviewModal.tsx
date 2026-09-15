import React, { useState } from 'react'
import { Dialog } from './dialog'
import { Button } from './button'
import { Badge } from './badge'
import { ExternalLink, Copy, Check, ZoomIn, Image as ImageIcon } from 'lucide-react'

interface ImagePreviewModalProps {
  open: boolean
  onClose: () => void
  imageUrl?: string
  imageLargeUrl?: string
  title?: string
  tsin?: string | number
  plid?: string | number
}

export const ImagePreviewModal: React.FC<ImagePreviewModalProps> = ({
  open,
  onClose,
  imageUrl,
  imageLargeUrl,
  title,
  tsin,
  plid,
}) => {
  const [copied, setCopied] = useState(false)
  const [useZoom, setUseZoom] = useState(false)

  // Derive large image URL
  const hdUrl = imageLargeUrl || imageUrl || ''
  const displayUrl = useZoom && hdUrl.includes('s-pdpxl.file')
    ? hdUrl.replace('s-pdpxl.file', 's-zoom.file')
    : hdUrl

  const handleCopy = () => {
    if (!displayUrl) return
    navigator.clipboard.writeText(displayUrl)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const takealotUrl = plid ? `https://www.takealot.com/x/PLID${String(plid).replace(/^PLID/i, '')}` : ''

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={
        <div className="flex items-center gap-2">
          <ImageIcon className="h-4 w-4 text-primary" />
          <span>商品高清大图预览</span>
          <Badge variant="success" dot className="text-[10px]">Takealot CDN 原图</Badge>
        </div>
      }
      description={title || (tsin ? `TSIN: ${tsin}` : undefined)}
      maxWidth="lg"
    >
      <div className="flex flex-col items-center space-y-4">
        {/* Image Display Box */}
        <div className="w-full h-80 sm:h-96 rounded-xl bg-slate-50 dark:bg-zinc-900/60 border border-border flex items-center justify-center p-3 relative overflow-hidden group">
          {displayUrl ? (
            <img
              src={displayUrl}
              alt={title || 'Product Preview'}
              className="max-h-full max-w-full object-contain rounded-md transition-transform duration-200"
              loading="lazy"
            />
          ) : (
            <div className="flex flex-col items-center justify-center text-muted-foreground gap-2">
              <ImageIcon className="h-10 w-10 stroke-1" />
              <span className="text-xs">暂无可用商品大图</span>
            </div>
          )}

          {/* Size Tag */}
          <div className="absolute top-3 left-3 flex items-center gap-1.5 bg-background/80 backdrop-blur-xs border border-border/80 px-2 py-0.5 rounded-md text-[11px] font-mono text-muted-foreground">
            <span>规格: {useZoom ? 'zoom (超清放大图)' : 'pdpxl (官方高清详情图)'}</span>
          </div>
        </div>

        {/* Action Buttons & Links */}
        <div className="w-full flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-border/40">
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setUseZoom(!useZoom)}
              className="gap-1.5"
            >
              <ZoomIn className="h-3.5 w-3.5" />
              <span>{useZoom ? '切换 pdpxl 规格' : '切换 zoom 超大图'}</span>
            </Button>

            <Button
              variant="secondary"
              size="sm"
              onClick={handleCopy}
              className="gap-1.5"
            >
              {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
              <span>{copied ? '已复制大图链接' : '复制大图链接'}</span>
            </Button>
          </div>

          <div className="flex items-center gap-2">
            {takealotUrl && (
              <a
                href={takealotUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                <span>前往 Takealot 查看</span>
                <ExternalLink className="h-3 w-3" />
              </a>
            )}
            <Button variant="default" size="sm" onClick={onClose}>
              关闭
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  )
}

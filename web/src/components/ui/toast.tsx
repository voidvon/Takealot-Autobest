import * as React from "react"
import { AlertCircle, CheckCircle2, AlertTriangle, Info, X } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ToastData } from "./use-toast"

interface ToastProps extends ToastData {
  onDismiss: (id: string) => void
}

export const ToastItem: React.FC<ToastProps> = ({
  id,
  title,
  description,
  variant = "default",
  onDismiss,
}) => {
  const variantStyles = {
    default: "border-border bg-background/95 text-foreground shadow-lg dark:bg-card/95",
    destructive:
      "border-rose-500/40 bg-rose-50/95 text-rose-950 shadow-lg dark:bg-rose-950/90 dark:text-rose-100 dark:border-rose-800/80",
    success:
      "border-emerald-500/40 bg-emerald-50/95 text-emerald-950 shadow-lg dark:bg-emerald-950/90 dark:text-emerald-100 dark:border-emerald-800/80",
    warning:
      "border-amber-500/40 bg-amber-50/95 text-amber-950 shadow-lg dark:bg-amber-950/90 dark:text-amber-100 dark:border-amber-800/80",
    info: "border-sky-500/40 bg-sky-50/95 text-sky-950 shadow-lg dark:bg-sky-950/90 dark:text-sky-100 dark:border-sky-800/80",
  }

  const renderIcon = () => {
    switch (variant) {
      case "destructive":
        return <AlertCircle className="h-5 w-5 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" />
      case "success":
        return <CheckCircle2 className="h-5 w-5 text-emerald-600 dark:text-emerald-400 shrink-0 mt-0.5" />
      case "warning":
        return <AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
      case "info":
        return <Info className="h-5 w-5 text-sky-600 dark:text-sky-400 shrink-0 mt-0.5" />
      default:
        return <Info className="h-5 w-5 text-muted-foreground shrink-0 mt-0.5" />
    }
  }

  return (
    <div
      role="alert"
      className={cn(
        "pointer-events-auto flex w-full max-w-[500px] items-start gap-3 rounded-xl border p-3.5 backdrop-blur-md transition-all duration-200 animate-in fade-in slide-in-from-top-4",
        variantStyles[variant]
      )}
    >
      {renderIcon()}

      <div className="flex-1 min-w-0 pr-1">
        {title && (
          <div className="text-xs sm:text-sm font-semibold leading-snug break-words">
            {title}
          </div>
        )}
        {description && (
          <div className="text-[11px] sm:text-xs opacity-90 leading-relaxed whitespace-pre-line mt-1 break-words">
            {description}
          </div>
        )}
      </div>

      <button
        type="button"
        onClick={() => onDismiss(id)}
        className="rounded-md p-1 opacity-60 transition-opacity hover:opacity-100 focus:outline-none shrink-0 cursor-pointer"
        aria-label="关闭提示"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}

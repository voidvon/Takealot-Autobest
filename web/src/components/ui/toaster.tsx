import * as React from "react"
import { useToast } from "./use-toast"
import { ToastItem } from "./toast"

export const Toaster: React.FC = () => {
  const { toasts, dismiss } = useToast()

  if (toasts.length === 0) {
    return null
  }

  return (
    <div
      aria-live="assertive"
      className="fixed top-5 left-1/2 -translate-x-1/2 z-[9999] flex flex-col items-center gap-2.5 pointer-events-none w-full max-w-[540px] px-4"
    >
      {toasts.map((toast) => (
        <ToastItem key={toast.id} {...toast} onDismiss={dismiss} />
      ))}
    </div>
  )
}

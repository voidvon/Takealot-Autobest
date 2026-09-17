import * as React from "react"

export type ToastVariant = "default" | "destructive" | "success" | "warning" | "info"

export interface ToastData {
  id: string
  title?: React.ReactNode
  description?: React.ReactNode
  variant?: ToastVariant
  duration?: number
}

type ActionType =
  | { type: "ADD_TOAST"; toast: ToastData }
  | { type: "DISMISS_TOAST"; toastId?: string }
  | { type: "REMOVE_TOAST"; toastId?: string }

interface State {
  toasts: ToastData[]
}

const TOAST_LIMIT = 5
const TOAST_REMOVE_DELAY = 1000

let count = 0
function genId() {
  count = (count + 1) % Number.MAX_SAFE_INTEGER
  return count.toString()
}

const toastTimeouts = new Map<string, ReturnType<typeof setTimeout>>()

const listeners: Array<(state: State) => void> = []
let memoryState: State = { toasts: [] }

function dispatch(action: ActionType) {
  memoryState = reducer(memoryState, action)
  listeners.forEach((listener) => {
    listener(memoryState)
  })
}

function reducer(state: State, action: ActionType): State {
  switch (action.type) {
    case "ADD_TOAST":
      return {
        ...state,
        toasts: [action.toast, ...state.toasts].slice(0, TOAST_LIMIT),
      }

    case "DISMISS_TOAST": {
      const { toastId } = action

      if (toastId) {
        addToRemoveQueue(toastId)
      } else {
        state.toasts.forEach((t) => {
          addToRemoveQueue(t.id)
        })
      }

      return {
        ...state,
        toasts: state.toasts.filter((t) => (toastId ? t.id !== toastId : false)),
      }
    }

    case "REMOVE_TOAST":
      if (action.toastId === undefined) {
        return {
          ...state,
          toasts: [],
        }
      }
      return {
        ...state,
        toasts: state.toasts.filter((t) => t.id !== action.toastId),
      }
  }
}

function addToRemoveQueue(toastId: string) {
  if (toastTimeouts.has(toastId)) {
    return
  }

  const timeout = setTimeout(() => {
    toastTimeouts.delete(toastId)
    dispatch({
      type: "REMOVE_TOAST",
      toastId,
    })
  }, TOAST_REMOVE_DELAY)

  toastTimeouts.set(toastId, timeout)
}

export function toast(props: Omit<ToastData, "id">) {
  const id = genId()
  const duration = props.duration ?? 4000

  const dismiss = () => dispatch({ type: "DISMISS_TOAST", toastId: id })

  dispatch({
    type: "ADD_TOAST",
    toast: {
      ...props,
      id,
      duration,
    },
  })

  if (duration > 0) {
    setTimeout(() => {
      dismiss()
    }, duration)
  }

  return {
    id,
    dismiss,
  }
}

// Convenience helper methods
toast.error = (title: React.ReactNode, description?: React.ReactNode, duration?: number) => {
  return toast({
    title,
    description,
    variant: "destructive",
    duration: duration ?? 5000,
  })
}

toast.success = (title: React.ReactNode, description?: React.ReactNode, duration?: number) => {
  return toast({
    title,
    description,
    variant: "success",
    duration: duration ?? 3500,
  })
}

toast.warning = (title: React.ReactNode, description?: React.ReactNode, duration?: number) => {
  return toast({
    title,
    description,
    variant: "warning",
    duration: duration ?? 4000,
  })
}

toast.info = (title: React.ReactNode, description?: React.ReactNode, duration?: number) => {
  return toast({
    title,
    description,
    variant: "info",
    duration: duration ?? 3500,
  })
}

export function useToast() {
  const [state, setState] = React.useState<State>(memoryState)

  React.useEffect(() => {
    listeners.push(setState)
    return () => {
      const index = listeners.indexOf(setState)
      if (index > -1) {
        listeners.splice(index, 1)
      }
    }
  }, [state])

  return {
    ...state,
    toast,
    dismiss: (toastId?: string) => dispatch({ type: "DISMISS_TOAST", toastId }),
  }
}

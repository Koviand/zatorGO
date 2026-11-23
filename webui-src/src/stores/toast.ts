import { ref } from 'vue'
import { status } from './status'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface ToastState {
  id: number
  message: string
  type: ToastType
}

const queue: ToastState[] = []

/** Текущий (показываемый) тост; очередь — в module-scope. */
export const toast = ref<ToastState | null>(null)

function priority(t: ToastType): number {
  switch (t) {
    case 'error': return 3
    case 'warning': return 2
    case 'info': return 1
    default: return 0
  }
}

/** Показать тост; error/warning не перекрываются success. */
export function showToast(message: string, type: ToastType = 'success') {
  const item: ToastState = { id: Date.now() + Math.random(), message, type }
  const cur = toast.value
  if (!cur) {
    toast.value = item
    return
  }
  // Более важный сразу сменяет текущий; иначе — в очередь.
  if (priority(type) > priority(cur.type)) {
    queue.unshift(cur)
    toast.value = item
    return
  }
  queue.push(item)
}

/** Следующий из очереди после закрытия текущего. */
export function advanceToast() {
  toast.value = queue.shift() ?? null
}

export function announceRestart() {
  if (status.value?.zapret2_running) showToast('Применяем настройку — перезапускаем zapret2…', 'info')
}

export function restartSuffix(payload: { restarted?: boolean; restart_required?: boolean } | null) {
  if (payload?.restarted) return ' zapret2 перезапущен.'
  if (payload?.restart_required === false) return ''
  return ' Применится при следующем запуске zapret2.'
}

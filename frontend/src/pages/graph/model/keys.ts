// Горячие клавиши карты: общие для полей — в widgets/graph-canvas (V, F, + − 0, Ctrl+Z, Ctrl+A, Delete, Esc), здесь то, что есть только у карты.
// Пока курсор в поле ввода или открыт диалог, до карты доходит только сохранение.
import { dialogOpen, handleCanvasKey, typing, type CanvasKeyEnv, type CanvasKeyEvent } from '@/widgets/graph-canvas'

/** Ctrl+S сохранить, R распутать, Shift+Del очистить карту; остальное — общие клавиши поля. */
export function handleGraphKey(
  e: CanvasKeyEvent,
  env: CanvasKeyEnv & {
    hasLit?: boolean
    clearLit?: () => void
    active: boolean
    ownerless: number
    canClear: boolean
    save: () => void
    relayout: () => void
    clear: () => void
  }
) {
  if (e.key === 'Escape') {
    if (!env.linking && env.hasLit) env.clearLit?.()
    else handleCanvasKey(e, env)
    return
  }
  if (!env.active || e.altKey) return
  const mod = e.ctrlKey || e.metaKey
  if (mod && e.code === 'KeyS') {
    e.preventDefault()
    if (!env.ownerless && !dialogOpen()) env.save()
    return
  }
  if (typing(e)) return
  if (!mod && e.code === 'KeyR' && !e.shiftKey) {
    e.preventDefault()
    env.relayout()
    return
  }
  if (e.code === 'Delete' && e.shiftKey) {
    if (env.canClear) {
      e.preventDefault()
      env.clear()
    }
    return
  }
  handleCanvasKey(e, env)
}

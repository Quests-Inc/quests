// Горячие клавиши карты (по физической клавише, чтобы работали в любой раскладке).
// Пока курсор в поле ввода или открыт диалог, до карты доходит только сохранение.

type KeyEvent = {
  key: string
  altKey: boolean
  ctrlKey: boolean
  metaKey: boolean
  code: string
  shiftKey: boolean
  target?: EventTarget | null
  preventDefault: () => void
}

function dialogOpen() {
  return typeof document !== 'undefined' && Boolean(document.querySelector('.backdrop [role="dialog"]'))
}

function typing(e: KeyEvent) {
  const target = e.target
  if (typeof Element === 'undefined' || !(target instanceof Element)) return false
  return Boolean(target.closest('input, textarea, select, [contenteditable="true"]') || dialogOpen())
}

/** Ctrl+S сохранить, Ctrl+Z отменить последнюю правку, V выделение, F вписать, R распутать, Shift+Del очистить. */
export function handleGraphKey(
  e: KeyEvent,
  env: {
    active: boolean
    linking: boolean
    selectMode: boolean
    full: boolean
    menuOpen: boolean
    ownerless: number
    canClear: boolean
    cancelLink: () => void
    toggleSelect: () => void
    toggleFull: () => void
    save: () => void
    undo: () => void
    fit: () => void
    relayout: () => void
    clear: () => void
  }
) {
  if (e.key === 'Escape') {
    if (env.linking) env.cancelLink()
    else if (env.selectMode) env.toggleSelect()
    else if (env.full && !env.menuOpen && !typing(e)) env.toggleFull()
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
  if (mod && e.code === 'KeyZ' && !e.shiftKey) {
    e.preventDefault()
    env.undo()
    return
  }
  if (mod) return
  if (e.code === 'KeyV' && !e.shiftKey) env.toggleSelect()
  else if (e.code === 'KeyF' && !e.shiftKey) env.fit()
  else if (e.code === 'KeyR' && !e.shiftKey) env.relayout()
  else if (e.code === 'Delete' && e.shiftKey && env.canClear) env.clear()
  else return
  e.preventDefault()
}

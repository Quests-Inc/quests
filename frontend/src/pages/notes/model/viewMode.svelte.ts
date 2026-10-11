// Режим просмотра заметки (текст / оба / просмотр): запоминается в браузере; на узком экране «Оба» не помещается.
export type ViewMode = 'raw' | 'combined' | 'formatted'

export const VIEW_MODE_OPTIONS = [
  { id: 'raw', label: 'Текст' },
  { id: 'combined', label: 'Оба' },
  { id: 'formatted', label: 'Просмотр' },
] as const

const KEY = 'quests.notes.viewMode'

export function loadViewMode(store: Pick<Storage, 'getItem'> | null): ViewMode {
  try {
    const v = store?.getItem(KEY)
    if (v === 'raw' || v === 'combined' || v === 'formatted') return v
  } catch {
    /* хранилище недоступно */
  }
  return 'combined'
}

export function createViewMode(
  store: Pick<Storage, 'getItem' | 'setItem'> | null = typeof localStorage === 'undefined' ? null : localStorage
) {
  let mode = $state<ViewMode>(loadViewMode(store))

  function set(next: ViewMode) {
    mode = next
    try {
      store?.setItem(KEY, next)
    } catch {
      /* выбор просто не запомнится */
    }
  }

  // «Оба» — это раскладка в две колонки, на экране уже 480px её нет (сама кнопка скрыта стилями):
  // уходим в «Просмотр», чтобы заметка не открылась в режиме без кнопки возврата.
  $effect(() => {
    const mq = window.matchMedia('(max-width: 480px)')
    const apply = () => {
      if (mq.matches && mode === 'combined') set('formatted')
    }
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  })

  return {
    get mode() {
      return mode
    },
    set,
  }
}

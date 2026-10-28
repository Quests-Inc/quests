// Настройки карты, которые помнит браузер: раскладка, скрытые карточки, фильтры квестов и видов, раскрытые связи.
import { ENTITY_TYPES, LAYOUTS } from '@/entities/graph'
import { QUEST_STATUSES, QUEST_STATUS_LABELS } from '@/shared/api/questConsts.ts'

type Store = Pick<Storage, 'getItem' | 'setItem'>

const KEYS = {
  layout: 'quests.graph.layout',
  hidden: 'quests.graph.hidden',
  statuses: 'quests.graph.statuses',
  types: 'quests.graph.types',
  expand: 'quests.graph.expand',
  sizes: 'quests.graph.sizes',
}

function readJSON(store: Store | null, key: string): unknown {
  try {
    return JSON.parse(store?.getItem(key) ?? 'null')
  } catch {
    return null
  }
}

function write(store: Store | null, key: string, value: unknown) {
  try {
    store?.setItem(key, JSON.stringify(value))
  } catch {
    /* хранилище может быть недоступно: настройка работает до перезагрузки */
  }
}

const strings = (raw: unknown) => (Array.isArray(raw) ? raw.filter((k): k is string => typeof k === 'string') : [])

export function createGraphPrefs(store: Store | null) {
  const layoutRaw = (() => {
    try {
      return store?.getItem(KEYS.layout) ?? null
    } catch {
      return null
    }
  })()
  let layoutMode = layoutRaw && layoutRaw in LAYOUTS ? layoutRaw : 'tree'

  let hidden = $state(new Set<string>(strings(readJSON(store, KEYS.hidden))))

  let questStatuses = $state(
    (() => {
      const ok = strings(readJSON(store, KEYS.statuses)).filter((x) => QUEST_STATUSES.includes(x))
      return new Set<string>(ok.length ? ok : ['active'])
    })()
  )

  let shownTypes = $state(
    (() => {
      const ok = strings(readJSON(store, KEYS.types)).filter((x) => ENTITY_TYPES.includes(x))
      return new Set<string>(ok.length ? ok : ENTITY_TYPES)
    })()
  )

  let expandKeys = $state<string[]>(strings(readJSON(store, KEYS.expand)))

  /** Размеры, которые человек задал растягиванием: ключ карточки → ширина и высота. */
  const readSizes = (raw: unknown) => {
    const out: Record<string, { w: number; h: number }> = {}
    if (raw && typeof raw === 'object') {
      for (const [k, v] of Object.entries(raw as Record<string, { w?: unknown; h?: unknown }>)) {
        if (typeof v?.w === 'number' && typeof v.h === 'number' && v.w > 0 && v.h > 0) out[k] = { w: v.w, h: v.h }
      }
    }
    return out
  }
  let sizes = $state<Record<string, { w: number; h: number }>>(readSizes(readJSON(store, KEYS.sizes)))
  const MAX_SIZES = 400

  const toggled = (set: Set<string>, v: string) => {
    const next = new Set(set)
    if (next.has(v)) next.delete(v)
    else next.add(v)
    return next
  }

  return {
    /** Раскладка: 'tree' | 'down' | 'radial' | 'layers' | 'free'. */
    get layoutMode() {
      return layoutMode
    },
    set layoutMode(v: string) {
      layoutMode = v
      try {
        store?.setItem(KEYS.layout, v)
      } catch {
        /* выбор просто не запомнится */
      }
    },
    /** Карточки, которые пользователь убрал с карты (ничего не удаляется). */
    get hidden() {
      return hidden
    },
    saveHidden(next: Set<string>) {
      hidden = next
      write(store, KEYS.hidden, [...next])
    },
    /** Какие квесты показывает карта; хотя бы один статус остаётся включённым. */
    get questStatuses() {
      return questStatuses
    },
    toggleStatus(st: string) {
      const next = toggled(questStatuses, st)
      if (!next.size) return
      questStatuses = next
      write(store, KEYS.statuses, [...next])
    },
    get statusLabel() {
      return questStatuses.size === QUEST_STATUSES.length
        ? 'все'
        : questStatuses.size === 1
          ? QUEST_STATUS_LABELS[[...questStatuses][0]]
          : `${questStatuses.size} статуса`
    },
    /** Какие виды сущностей показывает карта (центр и черновик остаются всегда). */
    get shownTypes() {
      return shownTypes
    },
    toggleType(t: string) {
      const next = toggled(shownTypes, t)
      if (!next.size) return
      shownTypes = next
      write(store, KEYS.types, [...next])
    },
    get typesLabel() {
      return shownTypes.size === ENTITY_TYPES.length ? 'все' : `${shownTypes.size} из ${ENTITY_TYPES.length}`
    },
    /** Карточки, у которых попросили показать все связи («quest:5» — все, «quest:5>note» — только заметки). */
    get expandKeys() {
      return expandKeys
    },
    setExpand(next: string[]) {
      expandKeys = next
      write(store, KEYS.expand, next)
    },
    get sizes() {
      return sizes
    },
    /** Запомнить размер карточки (null — вернуть автоматический). Хранится не больше 400 последних. */
    setSize(key: string, size: { w: number; h: number } | null) {
      const next = { ...sizes }
      delete next[key]
      if (size) next[key] = size
      const keys = Object.keys(next)
      for (const k of keys.slice(0, Math.max(0, keys.length - MAX_SIZES))) delete next[k]
      sizes = next
      write(store, KEYS.sizes, next)
    },
    /** Поставить размеры целиком (расклад): прежние заменяются. */
    restoreSizes(next: unknown) {
      sizes = readSizes(next)
      write(store, KEYS.sizes, sizes)
    },
    ownExpand: (key: string) => expandKeys.filter((k) => k === key || k.startsWith(`${key}>`)),
    /** Встать на сохранённый расклад: фильтры, скрытое и раскрытое (непустые фильтры, неизвестное отбрасывается). */
    restore(s: { statuses?: string[]; types?: string[]; hidden?: string[]; expand?: string[] }) {
      const st = strings(s.statuses).filter((x) => QUEST_STATUSES.includes(x))
      if (st.length) {
        questStatuses = new Set(st)
        write(store, KEYS.statuses, st)
      }
      const ty = strings(s.types).filter((x) => ENTITY_TYPES.includes(x))
      if (ty.length) {
        shownTypes = new Set(ty)
        write(store, KEYS.types, ty)
      }
      hidden = new Set(strings(s.hidden))
      write(store, KEYS.hidden, [...hidden])
      expandKeys = strings(s.expand)
      write(store, KEYS.expand, expandKeys)
    },
  }
}

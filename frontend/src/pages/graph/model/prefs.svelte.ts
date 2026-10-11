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
    ownExpand: (key: string) => expandKeys.filter((k) => k === key || k.startsWith(`${key}>`)),
  }
}

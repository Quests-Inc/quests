// Пункты контекстных меню журнала: квестлайн, квест, шаг.
import { QUEST_SIGNIFICANCES, QUEST_STATUSES, QUEST_STATUS_LABELS } from '@/shared/api/questConsts.ts'

export interface MenuItem {
  id: string
  label?: string
  sep?: boolean
  danger?: boolean
  checked?: boolean
  children?: MenuItem[]
  /** Подменю с полем поиска: полный список для запроса. */
  search?: MenuItem[]
}

/** «Без квестлайна» и все линии: цели для переноса группы квестов. */
function moveTargets(lines: { id: number; title: string }[]): MenuItem[] {
  return [{ id: 'move:none', label: 'Без квестлайна' }, ...lines.map((l) => ({ id: `move:${l.id}`, label: l.title }))]
}

export function contextMenuItems({
  kind,
  lineId,
  quest,
  questId,
  stepId,
  step,
  count = 0,
  lines = [],
  allPinned = false,
}: {
  kind: string
  lineId: number | null
  quest?: { status?: string; significance?: string; failure_inbox?: boolean; pinned?: boolean }
  questId: number | null
  stepId: number | null
  step?: { progress_total?: number }
  /** сколько квестов в группе (kind === 'quests') */
  count?: number
  /** квестлайны для «Переместить» в меню группы */
  lines?: { id: number; title: string }[]
  /** все квесты группы уже закреплены */
  allPinned?: boolean
}): MenuItem[] {
  if (kind === 'quests') {
    return [
      { id: 'copy-ids', label: `Копировать ссылки (${count})` },
      { id: 'clear-picked', label: 'Снять выделение' },
      { id: 'sep-copy', sep: true },
      {
        id: 'status',
        label: 'Изменить статус',
        children: QUEST_STATUSES.map((s) => ({ id: `status:${s}`, label: QUEST_STATUS_LABELS[s] || s })),
      },
      {
        id: 'delay',
        label: 'Отложить',
        children: [15, 30, 60, 90, 120].map((m) => ({ id: `delay-${m}`, label: `${m} мин` })),
      },
      {
        id: 'sig',
        label: 'Значимость',
        children: QUEST_SIGNIFICANCES.map((s) => ({ id: `sig:${s.id}`, label: s.label })),
      },
      { id: 'pin', label: allPinned ? 'Снять закреп' : 'Закрепить' },
      { id: 'move', label: 'Переместить в квестлайн', children: moveTargets(lines), search: moveTargets(lines) },
      { id: 'sep-danger', sep: true },
      { id: 'delete-many', label: `Удалить (${count})`, danger: true },
    ]
  }
  if (kind === 'line') {
    return [
      { id: 'copy-id', label: `Копировать questline=${lineId}` },
      { id: 'graph', label: 'Открыть на карте' },
      { id: 'sep-copy', sep: true },
      { id: 'add', label: 'Добавить квест' },
      { id: 'edit', label: 'Редактировать' },
      { id: 'icon', label: 'Иконка' },
      { id: 'secrets', label: 'Секреты' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить', danger: true },
    ]
  }
  if (kind === 'quest') {
    const status = quest?.status
    const significance = quest?.significance || 'common'
    const pinned = Boolean(quest?.pinned)
    const delayChildren: { id: string; label?: string; sep?: boolean; checked?: boolean }[] = [
      { id: 'delay-15', label: '15 мин' },
      { id: 'delay-30', label: '30 мин' },
      { id: 'delay-60', label: '60 мин' },
      { id: 'delay-90', label: '90 мин' },
      { id: 'delay-120', label: '120 мин' },
    ]
    if (status === 'frozen') {
      delayChildren.push({ id: 'sep-undelay', sep: true }, { id: 'status:active', label: 'Вернуть в активные' })
    }
    const items: MenuItem[] = [
      { id: 'copy-id', label: `Копировать quest=${questId}` },
      { id: 'graph', label: 'Открыть на карте' },
      { id: 'sep-copy', sep: true },
      { id: 'edit', label: 'Редактировать' },
      { id: 'add-step', label: 'Добавить шаг' },
      { id: 'pin', label: pinned ? 'Снять закреп' : 'Закрепить' },
      { id: 'sep-groups', sep: true },
      {
        id: 'status',
        label: 'Изменить статус',
        children: QUEST_STATUSES.map((s) => ({
          id: `status:${s}`,
          label: QUEST_STATUS_LABELS[s] || s,
          checked: s === status,
        })),
      },
      { id: 'delay', label: 'Отложить', children: delayChildren },
      {
        id: 'sig',
        label: 'Значимость',
        children: QUEST_SIGNIFICANCES.map((s) => ({
          id: `sig:${s.id}`,
          label: s.label,
          checked: s.id === significance,
        })),
      },
      { id: 'secrets', label: 'Секреты' },
      {
        id: 'export',
        label: 'Экспорт',
        children: [
          { id: 'export-md', label: 'Markdown (.md)' },
          { id: 'export-pdf', label: 'PDF' },
          { id: 'export-pdf-simple', label: 'PDF (просто)' },
        ],
      },
    ]
    if (quest?.failure_inbox) {
      items.push({ id: 'sep-ack', sep: true }, { id: 'ack-failure', label: 'Скрыть из провалов' })
    }
    items.push({ id: 'sep-danger', sep: true }, { id: 'delete', label: 'Удалить', danger: true })
    return items
  }
  if (kind === 'step') {
    const total = Math.max(1, Number(step?.progress_total) || 1)
    const items: {
      id: string
      label?: string
      sep?: boolean
      danger?: boolean
      children?: { id: string; label?: string; sep?: boolean }[]
    }[] = [
      { id: 'copy-id', label: `Копировать step=${stepId}` },
      { id: 'graph', label: 'Открыть на карте' },
    ]
    if (total > 1) {
      items.push({
        id: 'progress',
        label: 'Прогресс',
        children: [
          { id: 'inc', label: '+1' },
          { id: 'dec', label: '−1' },
          { id: 'sep-done', sep: true },
          { id: 'complete', label: 'Выполнить' },
          { id: 'reset', label: 'Обнулить' },
        ],
      })
    } else {
      items.push(
        { id: 'sep-done', sep: true },
        { id: 'complete', label: 'Выполнить' },
        { id: 'reset', label: 'Обнулить' }
      )
    }
    items.push(
      { id: 'sep-edit', sep: true },
      { id: 'edit', label: 'Редактировать' },
      { id: 'add-step', label: 'Добавить шаг' },
      { id: 'secrets', label: 'Секреты' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить шаг', danger: true }
    )
    return items
  }
  return []
}

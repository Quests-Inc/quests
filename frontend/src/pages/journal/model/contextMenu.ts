// Пункты контекстных меню журнала: квестлайн, квест, шаг.
import { QUEST_SIGNIFICANCES, QUEST_STATUSES, QUEST_STATUS_LABELS } from '@/shared/api/questConsts.ts'

export interface MenuItem {
  id: string
  label?: string
  sep?: boolean
  danger?: boolean
  checked?: boolean
  children?: MenuItem[]
}

export function contextMenuItems({
  kind,
  lineId,
  quest,
  questId,
  stepId,
  step,
}: {
  kind: string
  lineId: number | null
  quest?: { status?: string; significance?: string; failure_inbox?: boolean }
  questId: number | null
  stepId: number | null
  step?: { progress_total?: number }
}): MenuItem[] {
  if (kind === 'line') {
    return [
      { id: 'copy-id', label: `Копировать questline=${lineId}` },
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
      { id: 'sep-copy', sep: true },
      { id: 'edit', label: 'Редактировать' },
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
    }[] = [{ id: 'copy-id', label: `Копировать step=${stepId}` }]
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
      { id: 'secrets', label: 'Секреты' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить шаг', danger: true }
    )
    return items
  }
  return []
}

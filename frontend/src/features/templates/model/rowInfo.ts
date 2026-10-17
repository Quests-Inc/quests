// Подписи строки шаблона в списке и пункты его контекстного меню — чистые функции.
import { daysLabel, parseWeekdays } from '@/entities/template'
import type { TemplateRead } from '@/shared/api/automation.gen'
import { QUEST_SIGNIFICANCES } from '@/shared/api/questConsts.ts'
import { plural } from '@/shared/lib/duration.ts'

export type Streak = { closed?: number; close_rate?: number; current_streak?: number; longest_streak?: number }

export function streakLabel(st: Streak | undefined): string {
  if (!st || !st.closed) return ''
  const pct = Math.round((st.close_rate || 0) * 100)
  return `серия ${st.current_streak} · лучшая ${st.longest_streak} · ${pct}%`
}

export function rowMeta(t: TemplateRead, streak?: Streak): string {
  const mode = t.emit_mode === 'surprise' ? 'случайно' : 'по расписанию'
  const days = t.freq === 'weekly' ? daysLabel(parseWeekdays(t.weekdays)) : 'каждый день'
  return [
    days,
    mode,
    t.script_id && 'скрипт',
    t.emit_pool_command && 'команда',
    t.questline_title,
    t.category_label,
    QUEST_SIGNIFICANCES.find((x) => x.id === t.significance)?.label || 'обычное',
    t.pinned && 'закреплён',
    t.overlay_hidden && 'не в оверлее',
    streakLabel(streak),
    `${t.steps?.length ?? 0} ${plural(t.steps?.length ?? 0, ['шаг', 'шага', 'шагов'])}`,
  ]
    .filter(Boolean)
    .join(' · ')
}

export function templateMenuItems(t: { id: number; enabled?: boolean } | null) {
  if (!t) return []
  return [
    { id: 'copy-id', label: `Копировать template=${t.id}` },
    { id: 'sep-copy', sep: true },
    { id: 'edit', label: 'Редактировать' },
    { id: 'emit', label: 'Эмитировать сейчас' },
    { id: 'duplicate', label: 'Дублировать (копия выключена)' },
    { id: 'toggle', label: t.enabled ? 'Выключить' : 'Включить' },
    { id: 'sep-secrets', sep: true },
    { id: 'secrets', label: 'Секреты' },
  ]
}

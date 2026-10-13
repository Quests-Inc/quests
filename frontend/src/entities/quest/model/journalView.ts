// Что показывает список квестов в журнале: открытые или все, сужение до раздела и квестлайна, варианты сужения.
import { OPEN_STATUSES } from './questFormat.ts'

type QuestLike = { status: string; category_id?: number | null; questline_id?: number | null }

/** Сужение по разделу: id раздела, 'none' — квесты без раздела, null — без сужения. */
export type CategoryScope = number | 'none' | null

const categoryOf = (scope: Exclude<CategoryScope, null>) => (scope === 'none' ? null : scope)

/** Без фильтра по статусу в запросе и без «показывать завершённые» — только открытые (active/expired). */
export function listOpenOrAll<Q extends QuestLike>(
  quests: Q[],
  opts: { showAll: boolean; statusTokens: string[] }
): Q[] {
  return opts.showAll || opts.statusTokens.length > 0 ? quests : quests.filter((q) => OPEN_STATUSES.has(q.status))
}

/** Квесты выбранного раздела и/или квестлайна; null — без сужения. */
export function inScope<Q extends QuestLike>(
  quests: Q[],
  scope: { categoryId: CategoryScope; questlineId: number | null }
): Q[] {
  return quests.filter((q) => {
    if (scope.categoryId != null && (q.category_id ?? null) !== categoryOf(scope.categoryId)) return false
    if (scope.questlineId != null && q.questline_id !== scope.questlineId) return false
    return true
  })
}

/**
 * Квестлайны, до которых можно сузить список внутри раздела: свои по разделу плюс те, где лежат квесты
 * этого раздела (у квестлайна раздела может не быть, а у его квестов — быть). Без «показывать завершённые» —
 * только те, где есть открытый квест: квестлайн без живых дел не «сессия», до которой стоит сужать.
 */
export function scopeQuestlineChoices<L extends { id: number; category_id?: number | null }>({
  scopeCategoryId,
  questlines,
  quests,
  showAll,
}: {
  scopeCategoryId: CategoryScope
  questlines: L[]
  quests: (QuestLike & { questline_id?: number | null })[]
  showAll: boolean
}): L[] {
  if (scopeCategoryId == null) return []
  const cat = categoryOf(scopeCategoryId)
  const ids = new Set(questlines.filter((l) => (l.category_id ?? null) === cat).map((l) => l.id))
  for (const q of quests) {
    if (q.questline_id != null && (q.category_id ?? null) === cat) ids.add(q.questline_id)
  }
  const inCategory = questlines.filter((l) => ids.has(l.id))
  if (showAll) return inCategory
  const live = new Set(
    quests.filter((q) => OPEN_STATUSES.has(q.status) && q.questline_id != null).map((q) => q.questline_id)
  )
  return inCategory.filter((l) => live.has(l.id))
}

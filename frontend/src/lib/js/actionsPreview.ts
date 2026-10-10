const ACTION_LABELS = {
  create_questline: 'создать квестлайн',
  create_quest: 'создать квест',
  add_step: 'добавить шаг',
  update_step: 'изменить шаг',
  delete_step: 'удалить шаг',
  update_quest: 'изменить квест',
}

const FIELD_LABELS = {
  title: 'название',
  description: 'описание',
  status: 'статус',
  significance: 'значимость',
  pinned: 'закреплён',
  sort_order: 'порядок',
  deadline_at: 'дедлайн',
  duration_seconds: 'окно, сек',
  category_id: 'категория',
  progress_current: 'прогресс',
  progress_total: 'из',
}

export function actionLabel(action) {
  return ACTION_LABELS[action] || action
}

// `before` is now the flat quest object (GET /api/quests/{id}) — the Go
// executor fetches it directly, no more nested /api/context shape.
function findStepIn(before, id) {
  return before?.steps?.find((st) => st.id === id) || null
}

/**
 * Flat preview[] (from POST /api/llm/actions/preview) → questline > quest >
 * step tree for "было → стало" rendering. `after.id`/`after.questline_id`/
 * `after.quest_id` are already ref-resolved server-side (real ids, or the
 * same negative placeholder id shared by the creating action in this same
 * batch), so grouping is just matching those numbers up.
 *
 * `quests`/`questlines` (already loaded by App.svelte) fill in labels/parent
 * links for existing entities this batch references but doesn't itself
 * create — e.g. an update_quest that only flips status, with no questline
 * touched at all.
 */
export function buildPreviewTree(preview, { quests = [], questlines = [] } = {}) {
  const questlineById = new Map(questlines.map((l) => [l.id, l]))
  const questById = new Map(quests.map((q) => [q.id, q]))

  const lineNodes = new Map()
  const questNodes = new Map()

  function ensureLineNode(id) {
    if (lineNodes.has(id)) return lineNodes.get(id)
    const row = questlineById.get(id)
    const node = {
      id,
      kind: 'questline',
      label: row?.title || `Квестлайн #${id}`,
      isNew: false,
      quests: [],
    }
    lineNodes.set(id, node)
    return node
  }

  function ensureQuestNode(id, { label, questlineId } = {}) {
    let node = questNodes.get(id)
    if (!node) {
      const row = questById.get(id)
      node = {
        id,
        kind: 'quest',
        label: label || row?.title || `Квест #${id}`,
        isNew: false,
        questlineId: questlineId !== undefined ? questlineId : (row?.questline_id ?? null),
        changes: [],
        steps: [],
      }
      questNodes.set(id, node)
      return node
    }
    if (label) node.label = label
    if (questlineId !== undefined) node.questlineId = questlineId
    return node
  }

  for (const item of preview) {
    const { action, after, before } = item

    if (action === 'create_questline') {
      lineNodes.set(after.id, {
        id: after.id,
        kind: 'questline',
        label: after.title,
        isNew: true,
        quests: [],
      })
      continue
    }

    if (action === 'create_quest') {
      const node = ensureQuestNode(after.id, {
        label: after.title,
        questlineId: after.questline_id ?? null,
      })
      node.isNew = true
      continue
    }

    if (action === 'update_quest') {
      const node = ensureQuestNode(after.id, {
        label: before?.title,
        questlineId: 'questline_id' in after ? after.questline_id : undefined,
      })
      for (const [k, v] of Object.entries(after)) {
        if (k === 'id' || k === 'questline_id') continue
        node.changes.push({ field: FIELD_LABELS[k] || k, from: before?.[k], to: v })
      }
      continue
    }

    if (action === 'add_step' || action === 'update_step' || action === 'delete_step') {
      const questNode = ensureQuestNode(after.quest_id)
      const ctxStep = after.step_id != null ? findStepIn(before, after.step_id) : null
      const stepNode = {
        id: after.step_id ?? `new:${item.index}`,
        kind: 'step',
        label: after.title || ctxStep?.title || `Шаг #${after.step_id}`,
        isNew: action === 'add_step',
        deleted: action === 'delete_step',
        changes: [],
      }
      if (action === 'update_step') {
        for (const [k, v] of Object.entries(after)) {
          if (k === 'quest_id' || k === 'step_id') continue
          stepNode.changes.push({ field: FIELD_LABELS[k] || k, from: ctxStep?.[k], to: v })
        }
      }
      questNode.steps.push(stepNode)
      continue
    }
  }

  const looseQuests = []
  for (const quest of questNodes.values()) {
    if (quest.questlineId == null) {
      looseQuests.push(quest)
      continue
    }
    ensureLineNode(quest.questlineId).quests.push(quest)
  }

  return { questlines: [...lineNodes.values()], looseQuests }
}

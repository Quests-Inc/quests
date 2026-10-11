// Общий список квестов. Его читают журнал, карта и заметки; оболочка только запускает загрузку.
import type { QuestRead } from '@/shared/api/journal.gen'
import { listQuests } from '@/shared/api/quests.ts'

export const questCatalog = $state({ list: [] as QuestRead[] })

/** С сервера, без записи в каталог: журнал подменяет список, только когда пришли все соседние данные. */
export function fetchQuestList() {
  return listQuests({})
}

export function setQuests(list: QuestRead[]) {
  questCatalog.list = list
}

/** Подменить одну карточку целиком. false — такого id нет, список не тронут. */
export function upsertQuest(quest: { id: number }) {
  if (!questCatalog.list.some((q) => q.id === quest.id)) return false
  questCatalog.list = questCatalog.list.map((q) => (q.id === quest.id ? (quest as QuestRead) : q))
  return true
}

export function clearQuests() {
  questCatalog.list = []
}

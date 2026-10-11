// Общий список квестов. Оболочка записывает его после совместной загрузки; страницы пока получают его пропом.
import type { QuestRead } from '@/shared/api/journal.gen'

export const questCatalog = $state({ list: [] as QuestRead[] })

export function setQuests(list: QuestRead[]) {
  questCatalog.list = list
}

/** Заменить карточку с тем же id. false — такого id нет, новая карточка не добавляется. */
export function replaceQuest(quest: { id: number }) {
  if (!questCatalog.list.some((q) => q.id === quest.id)) return false
  questCatalog.list = questCatalog.list.map((q) => (q.id === quest.id ? (quest as QuestRead) : q))
  return true
}

export function clearQuests() {
  questCatalog.list = []
}

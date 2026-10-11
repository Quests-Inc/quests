// Справочники квестов: статусы, значимость, иконки и цвета квестлайнов.

export const QUESTLINE_ICONS = ['document', 'flag', 'map', 'layers', 'target', 'scroll']

export const QUESTLINE_COLORS = ['#5a8a9a', '#8a8578', '#7a9e3a', '#6a7ab8', '#c47a20', '#c9a227', '#b54a3a', '#9a9a9a']

export const QUEST_STATUSES = ['active', 'expired', 'frozen', 'completed', 'failed', 'archived']

export const QUEST_STATUS_LABELS: Record<string, string> = {
  active: 'активен',
  expired: 'просрочен',
  frozen: 'заморожен',
  completed: 'выполнен',
  failed: 'провален',
  archived: 'архив',
}

export const QUEST_SIGNIFICANCES: { id: string; label: string }[] = [
  { id: 'insignificant', label: 'незначительное' },
  { id: 'common', label: 'обычное' },
  { id: 'uncommon', label: 'необычное' },
  { id: 'epic', label: 'эпическое' },
  { id: 'legendary', label: 'легендарное' },
]

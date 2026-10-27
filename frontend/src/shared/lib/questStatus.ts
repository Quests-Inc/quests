// Как интерфейс называет статус квеста. Коды (`active`, `expired`…) — только в данных и в подсказках, не на экране.

const STATUS_WORDS: Record<string, string> = {
  active: 'активен',
  expired: 'просрочен',
  frozen: 'заморожен',
  completed: 'выполнен',
  failed: 'провален',
  archived: 'в архиве',
}

/** The word a quest's status is shown with; an unknown code is shown as it is. */
export function statusWord(status: string | null | undefined): string {
  return status ? (STATUS_WORDS[status] ?? status) : ''
}

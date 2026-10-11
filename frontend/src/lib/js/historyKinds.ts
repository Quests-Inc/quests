// Presentation of change-log kinds, shared by the History screen and per-entity timelines.

/** kind → [label, icon, tone] */
export const KINDS = {
  quest_created: ['создан квест', 'add', 'new'],
  quest_appeared: ['появился квест', 'add', 'new'],
  quest_completed: ['квест выполнен', 'checkmark', 'ok'],
  quest_failed: ['квест провален', 'alert', 'bad'],
  quest_expired: ['квест просрочен', 'alert', 'bad'],
  quest_delayed: ['квест отложен', 'renew', 'warn'],
  quest_deleted: ['квест удалён', 'delete', 'bad'],
  quest_updated: ['квест изменён', 'edit', ''],
  status_changed: ['смена статуса', 'flag', ''],
  pin_changed: ['закрепление', 'pin', ''],
  step_completed: ['шаг выполнен', 'check', 'ok'],
  step_reopened: ['шаг открыт снова', 'undo', 'warn'],
  note_created: ['создана заметка', 'document', 'new'],
  note_updated: ['заметка изменена', 'edit', ''],
  note_deleted: ['заметка удалена', 'delete', 'bad'],
  attachment_added: ['добавлено вложение', 'attachment', 'new'],
  attachment_version: ['версия вложения', 'layers', ''],
  attachment_hidden: ['вложение скрыто/возвращено', 'eye-off', ''],
  attachment_deleted: ['вложение удалено', 'delete', 'bad'],
}

/** Details that only repeat what the kind label already says. */
const GENERIC = new Set([
  'создано задание',
  'изменено',
  'удалено',
  'удалена',
  'изменена',
  'создана',
  'загружено',
  'просрочено',
  'провалено',
  'завершено',
  'создано',
])

/** @param {any} r */
export function detailOf(r: any) {
  const d = (r.detail || '').trim()
  if (!d || d === r.title || GENERIC.has(d.toLowerCase())) return ''
  return d
}

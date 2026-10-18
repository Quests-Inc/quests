/** What a script is for: chosen by hand, it sorts the scripts tab and narrows the pickers. '' = unsorted. */
export const PURPOSES = [
  { id: 'step', title: 'Шаги', color: '#4aa3df' },
  { id: 'note', title: 'Заметки', color: '#7a9e3a' },
  { id: 'template', title: 'Шаблоны', color: '#c47a20' },
  { id: 'hook', title: 'Хуки', color: '#b06ad8' },
] as const

export const PURPOSE_NONE = { id: '', title: 'Общие', color: '#8a8a8a' }

/** The purpose a script made for this kind of entity gets (a picker's `kind`, a sample's subject). */
export function purposeOfKind(kind: string): string {
  return kind === 'quest' || kind === 'workspace' ? 'hook' : kind
}

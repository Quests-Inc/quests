import { expect, test } from 'bun:test'

import { createJournalDesk } from '@/pages/journal/model/desk.svelte.ts'

function desk() {
  const selectedIds: (number | null)[] = []
  const reloads: string[] = []
  return createJournalDesk({
    getQuests: () => [{ id: 1, title: 'квест', pinned: false }],
    getQuestlines: () => [{ id: 4, title: 'линия' }],
    getSelected: () => ({ id: 1, title: 'квест' }),
    getSelectedId: () => 1,
    setSelectedId: (id) => selectedIds.push(id),
    selectQuest: () => {},
    reload: () => reloads.push('load'),
    setError: () => {},
    getRefLabels: () => ({}),
    openSecrets: () => {},
    onGraphDirty: () => {},
  })
}

test('новый квест открывает форму создания, чужой id квестлайна не редактируется', () => {
  const d = desk()
  expect(d.modalOpen).toBe(false)
  d.openCreate({ description: 'из меню' })
  expect(d.modalOpen).toBe(true)
  expect(d.modalMode).toBe('create')
  expect(d.modalDefaults?.description).toBe('из меню')
  d.openCreateQuestline()
  expect(d.lineModalOpen).toBe(true)
  expect(d.lineModalMode).toBe('create')
})

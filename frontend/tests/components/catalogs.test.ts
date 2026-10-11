import { expect, test } from 'bun:test'

import {
  attachmentCatalog,
  clearAttachmentIndex,
  setAttachmentIndex,
} from '@/entities/attachment/model/catalog.svelte.ts'
import { clearNotes, noteCatalog, setNotes } from '@/entities/note/model/catalog.svelte.ts'
import { clearQuests, questCatalog, replaceQuest, setQuests } from '@/entities/quest/model/catalog.svelte.ts'

const quest = (id: number, title: string) => ({ id, title }) as never

test('квесты: подмена списка, правка одной карточки, сброс', () => {
  setQuests([quest(1, 'а'), quest(2, 'б')])
  expect(questCatalog.list.map((q) => q.title)).toEqual(['а', 'б'])
  expect(replaceQuest(quest(2, 'было'))).toBe(true)
  expect(questCatalog.list.map((q) => q.title)).toEqual(['а', 'было'])
  expect(replaceQuest(quest(9, 'нет'))).toBe(false)
  expect(questCatalog.list).toHaveLength(2)
  clearQuests()
  expect(questCatalog.list).toEqual([])
})

test('заметки и вложения: запись и пустой каталог', () => {
  setNotes([{ id: 3, title: 'н' } as never])
  setAttachmentIndex({ quest: { '1': [] } })
  expect(noteCatalog.list).toHaveLength(1)
  expect(attachmentCatalog.index?.quest?.['1']).toEqual([])
  clearNotes()
  clearAttachmentIndex()
  expect(noteCatalog.list).toEqual([])
  expect(attachmentCatalog.index).toBeNull()
})

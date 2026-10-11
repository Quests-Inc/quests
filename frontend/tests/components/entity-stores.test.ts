import { beforeEach, expect, mock, test } from 'bun:test'

let runners: any[] = []
let templates: any[] = []
let runnerCalls = 0
let templateFail = false
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getRunners: async () => {
    runnerCalls++
    return runners
  },
  listTemplates: async () => {
    if (templateFail) throw new Error('нет связи')
    return templates
  },
}))

const { runnersList, loadRunners } = await import('@/entities/runner/model/runnersList.svelte.ts')
const { templateStore, refreshTemplates } = await import('@/entities/template/model/templatesStore.svelte.ts')
const drafts = await import('@/entities/note/model/noteDrafts.svelte.ts')
const cache = await import('@/entities/attachment/model/attachmentCache.ts')

beforeEach(() => {
  runnerCalls = 0
  templateFail = false
})

test('runnersList: берёт только неотозванные раннеры, не чаще раза в 20 с, force и параллельные вызовы', async () => {
  runners = [
    { label: 'home', online: true, registered: true },
    { label: 'old', revoked: true },
    { label: 'new', online: 0, registered: 0 },
  ]
  await Promise.all([loadRunners(), loadRunners()])
  expect(runnerCalls).toBe(1)
  expect(runnersList.items).toEqual([
    { label: 'home', online: true, registered: true },
    { label: 'new', online: false, registered: false },
  ])
  await loadRunners()
  expect(runnerCalls).toBe(1)
  await loadRunners(true)
  expect(runnerCalls).toBe(2)
})

test('templateStore: загружает список, при ошибке оставляет прежний', async () => {
  templates = [{ id: 1, title: 'Дейлик' }]
  await refreshTemplates()
  expect(templateStore.list).toEqual(templates)
  expect(templateStore.loaded).toBe(true)
  templateFail = true
  await refreshTemplates()
  expect(templateStore.list).toEqual(templates)
})

test('noteDrafts: ключи, запись копией, очистка', () => {
  expect(drafts.noteDraftKey(null)).toBe('')
  expect(drafts.getNoteDraft(null)).toBeNull()
  drafts.putNoteDraft(5, { title: 'Т', description: 'Д', parentId: '2', pinned: 1 as any })
  expect(drafts.getNoteDraft('5')).toEqual({ title: 'Т', description: 'Д', parentId: '2', pinned: true })
  expect(drafts.noteDrafts['5']).toBeTruthy()
  drafts.clearNoteDraft(5)
  expect(drafts.getNoteDraft(5)).toBeNull()
  drafts.clearNoteDraft(null)
})

test('attachmentCache: засев по владельцам, пустые владельцы получают [], сброс флагов зовёт слушателей', () => {
  const index = { quest: { 1: [{ id: 10, filename: 'a' }] }, note: {}, questline: {} }
  cache.seedAttachmentIndex(index, { questIds: [1, 2], noteIds: [7] })
  expect(cache.peekAttachmentList('quest', 1)).toEqual({ items: [{ id: 10, filename: 'a' }], probed: false })
  expect(cache.peekAttachmentList('quest', 2)).toEqual({ items: [], probed: true })
  expect(cache.peekAttachmentList('note', 7)).toEqual({ items: [], probed: true })
  expect(cache.peekAttachmentList('quest', null as any)).toBeNull()
  // повторный засев того же состава сохраняет «живые» флаги, найденные WebDAV
  cache.setAttachmentList('quest', 1, [{ id: 10, available: true }], { probed: true })
  cache.seedAttachmentIndex({ quest: { 1: [{ id: 10 }] } }, { questIds: [1] })
  expect(cache.peekAttachmentList('quest', 1)?.items[0]).toMatchObject({ id: 10, available: true })
  let notified = 0
  const off = cache.onAttachmentLiveInvalidate(() => notified++)
  cache.invalidateAttachmentLiveFlags()
  expect(notified).toBe(1)
  expect(cache.peekAttachmentList('quest', 1)?.probed).toBe(false)
  off()
  cache.invalidateAttachmentLiveFlags()
  expect(notified).toBe(1)
})

test('flattenAttachmentIndex: плоский список с типом владельца', () => {
  expect(cache.flattenAttachmentIndex(null)).toEqual([])
  const flat = cache.flattenAttachmentIndex({
    quest: { 1: [{ id: 1, owner_id: 1 }] },
    note: { 2: [{ id: 2, owner_id: 2 }] },
    questline: 'bad',
  })
  expect(flat.map((a) => [a.id, a.owner_type])).toEqual([
    [1, 'quest'],
    [2, 'note'],
  ])
})

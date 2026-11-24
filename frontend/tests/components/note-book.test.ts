import { expect, test, mock } from 'bun:test'
const reads: number[] = []
const bakes: number[] = []
let failRead = false
const catalog = [
  { id: 1, parent_id: null },
  { id: 2, parent_id: 1, sort_order: 10 },
  { id: 3, parent_id: 1, sort_order: 0 },
  { id: 4, parent_id: 2 },
  { id: 5, parent_id: null },
]
mock.module('@/shared/api/notes.ts', () => ({
  listNotes: async () => catalog,
  getNote: async (id: number) => {
    reads.push(id)
    if (failRead) throw new Error('read failed')
    return { id, title: `Chapter ${id}`, description: `Full body ${id}` }
  },
}))
mock.module('@/shared/lib/scriptBake.ts', () => ({
  bakeAutoScripts: async (source: string, host: { id: number }) => {
    bakes.push(host.id)
    return `${source} baked in ${host.id}`
  },
}))
const { buildNoteBook, noteBookOrder } = await import('@/entities/note/model/noteExport.ts')
test('book includes every descendant in tree order and bakes each in its own context', async () => {
  const book = await buildNoteBook({ id: 1, title: 'Draft title', description: 'Unsaved draft' })
  expect(book.chapters.map((c) => [c.note_id, c.depth])).toEqual([
    [1, 0],
    [2, 1],
    [4, 2],
    [3, 1],
  ])
  expect(reads).toEqual([2, 4, 3])
  expect(bakes).toEqual([1, 2, 4, 3])
  expect(book.chapters[0].markdown).toBe('Unsaved draft baked in 1')
  expect(book.chapters[1].markdown).toBe('Full body 2 baked in 2')
  expect(book.labels['note:4']).toBe('Chapter 4')
})
test('cycle cannot hang traversal or repeat notes', () => {
  expect(
    noteBookOrder(
      [
        { id: 1, parent_id: 2 },
        { id: 2, parent_id: 1 },
      ],
      1
    )
  ).toEqual([
    { id: 1, depth: 0 },
    { id: 2, depth: 1 },
  ])
})
test('failed child read aborts export instead of silently losing a chapter', async () => {
  failRead = true
  await expect(buildNoteBook({ id: 1, title: 'Root', description: '' })).rejects.toThrow('read failed')
})

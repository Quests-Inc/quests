import { expect, test } from 'bun:test'

import { createScriptGroup } from '@/pages/scripts/model/scriptGroup.svelte.ts'

import { inRoot } from './root.svelte.ts'

function make() {
  const log: any[] = []
  const store: Record<number, any> = {
    1: { id: 1, title: 'А', description: 'д', purpose: 'hook', version: 3, graph: { nodes: [], edges: [] } },
    2: { id: 2, title: 'Б', purpose: '', version: 1, graph: { nodes: [], edges: [] } },
    3: { id: 3, title: 'В', purpose: '', version: 1, graph: { nodes: [], edges: [] } },
  }
  let grp!: ReturnType<typeof createScriptGroup>
  const stop = inRoot(() => {
    grp = createScriptGroup({
      scripts: () => Object.values(store),
      selectedId: () => 2,
      api: {
        get: async (id) => store[id],
        create: async (body) => (log.push(['create', body.title, body.purpose]), { id: 9 }),
        update: async (id, body) => void log.push(['update', id, body.version, body.purpose]),
        remove: async (id) => {
          if (id === 3) throw new Error('подключён')
          log.push(['remove', id])
        },
      },
      reload: async () => void log.push(['reload']),
      onRemoved: (ids) => log.push(['removed', ids]),
    })
  })
  return { grp, log, stop }
}

test('дублирование: копия с тем же графом и принадлежностью, имя с «(копия)»', async () => {
  const t = make()
  expect(await t.grp.duplicate(1)).toBe(9)
  expect(t.log).toEqual([['create', 'А (копия)', 'hook'], ['reload']])
  t.stop()
})

test('выделение: Ctrl+клик и Ctrl+A; меню группы со счётчиком', () => {
  const t = make()
  t.grp.click({ ctrlKey: true }, 1, [1, 2, 3])
  expect(t.grp.multi.has(1)).toBe(true)
  t.grp.selectAll([1, 2, 3])
  expect(t.grp.multi.size).toBe(3)
  expect(t.grp.items([1, 2, 3]).map((i) => i.label)).toEqual([
    'Принадлежность (3)',
    'Снять выделение',
    undefined,
    'Удалить (3)…',
  ])
  t.stop()
})

test('принадлежность пачкой идёт с версией каждого, пропуская те, где уже так; удаление — с подтверждением, отказ сервера считается', async () => {
  const t = make()
  t.grp.run('gp:hook', [1, 2])
  await new Promise((r) => setTimeout(r, 10))
  expect(t.log).toEqual([['update', 2, 1, 'hook'], ['reload']])
  t.log.length = 0
  t.grp.run('group-delete', [1, 3])
  expect(t.grp.ask).toEqual([1, 3])
  await t.grp.confirmDelete()
  expect(t.log).toEqual([['remove', 1], ['reload'], ['removed', [1]]])
  expect(t.grp.ask).toEqual([])
  t.stop()
})

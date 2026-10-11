import { beforeEach, expect, mock, test } from 'bun:test'

const calls: string[] = []
let fail: string | null = null
const real = await import('@/shared/api/client.ts')
const guard = async (name: string, value: unknown = {}) => {
  calls.push(name)
  if (fail === name) throw new Error('отказ сервера')
  return value
}
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  fetchWorkspaces: () =>
    guard('list', {
      workspaces: [
        { id: 1, name: 'Мой' },
        { id: 2, name: 'Чужой' },
      ],
      current: 1,
      attachments_used: 5,
      attachments_quota: 10,
    }),
  fetchMembers: () =>
    guard('members', {
      members: [
        { user_id: 1, username: 'я' },
        { user_id: 2, username: 'он' },
      ],
      role: 'owner',
    }),
  switchWorkspace: (id: number) => guard(`switch:${id}`),
  createWorkspace: (name: string) => guard(`create:${name}`, { id: 9 }),
  renameWorkspace: (id: number, name: string) => guard(`rename:${id}:${name}`),
  deleteWorkspace: (id: number, name: string) => guard(`delete:${id}:${name}`),
  removeMember: (ws: number, user: number) => guard(`remove:${ws}:${user}`),
  createInvite: (ws: number) => guard(`invite:${ws}`, { path: '/?invite=abc' }),
  importWorkspace: () =>
    guard('import', { name: 'Копия', workspace_id: 3, report: { stripped_commands: 2, dangling_refs: 1 } }),
}))
const ws = await import('@/features/settings/model/workspaceState.svelte.ts')

let reloads = 0
beforeEach(() => {
  calls.length = 0
  fail = null
  reloads = 0
  ws.env.reload = () => void reloads++
  ws.env.origin = () => 'http://h'
  ws.wsx.error = ''
  ws.wsx.busy = false
  ws.wsx.notice = ''
})

test('loadWorkspaceState: пространства, текущее, квота и участники с ролью', async () => {
  await ws.loadWorkspaceState()
  expect(ws.wsx).toMatchObject({ loaded: true, current: 1, attachmentsUsed: 5, attachmentsQuota: 10, myRole: 'owner' })
  expect(ws.wsx.list).toHaveLength(2)
  expect(ws.wsx.members).toHaveLength(2)
  expect(ws.currentWorkspaceName()).toBe('Мой')
})

test('переключение и создание перезагружают страницу, пустое имя и то же пространство — нет', async () => {
  await ws.pickWorkspace(1)
  expect(calls).toEqual([])
  await ws.pickWorkspace(2)
  expect(calls).toEqual(['switch:2'])
  expect(reloads).toBe(1)
  expect(await ws.addWorkspace('  ')).toBe(false)
  expect(await ws.addWorkspace(' Новое ')).toBe(true)
  expect(calls).toEqual(['switch:2', 'create:Новое', 'switch:9'])
  expect(reloads).toBe(2)
})

test('переименование обновляет список; то же имя — без запроса; ошибка показывается и не перезагружает', async () => {
  await ws.loadWorkspaceState()
  calls.length = 0
  await ws.saveName('Мой')
  expect(calls).toEqual([])
  await ws.saveName(' Новое имя ')
  expect(calls).toEqual(['rename:1:Новое имя'])
  expect(ws.currentWorkspaceName()).toBe('Новое имя')
  fail = 'delete:1:Новое имя'
  await ws.removeCurrent(' Новое имя ')
  expect(ws.wsx.error).toBe('отказ сервера')
  expect(reloads).toBe(0)
  expect(ws.wsx.busy).toBe(false)
})

test('приглашение возвращает ссылку, сбой — пустую строку; импорт пишет отчёт и перезагружает позже', async () => {
  expect(await ws.newInvite()).toBe('http://h/?invite=abc')
  fail = 'invite:1'
  expect(await ws.newInvite()).toBe('')
  fail = null
  await ws.doImport(new Blob(['x']), false)
  expect(ws.wsx.notice).toContain('«Копия»')
  expect(ws.wsx.notice).toContain('Команд без раннера убрано: 2')
  expect(calls.at(-1)).toBe('switch:3')
})

test('участники: свой выход подтверждается и перезагружает, чужого убирают из списка', async () => {
  await ws.loadWorkspaceState()
  ;(globalThis as any).confirm = () => true
  await ws.dropMember({ username: 'он', user_id: 2 }, 'я')
  expect(ws.wsx.members.map((m: any) => m.user_id)).toEqual([1])
  expect(reloads).toBe(0)
  await ws.dropMember({ username: 'я', user_id: 1 }, 'я')
  expect(reloads).toBe(1)
  ;(globalThis as any).confirm = () => false
  calls.length = 0
  await ws.dropMember({ username: 'он', user_id: 2 }, 'я')
  expect(calls).toEqual([])
})

import { afterEach, expect, test } from 'bun:test'

import { checkPayload, questStepDraft } from '@/entities/quest'
import { loadScripts, scriptsFor, scriptsList } from '@/entities/script/model/scriptsList.svelte.ts'
import {
  bindScript,
  createScript,
  deleteScript,
  getNodePorts,
  unbindScript,
  updateScript,
  validateScript,
} from '@/shared/api/client.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
  scriptsList.items = []
  scriptsList.loaded = false
})

function fakeFetch(routes: Record<string, unknown>) {
  const calls: { key: string; body: unknown }[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    calls.push({ key, body: init?.body ? JSON.parse(String(init.body)) : undefined })
    if (!(key in routes)) return new Response(JSON.stringify({ detail: `no route ${key}` }), { status: 404 })
    const body = routes[key]
    return body === null ? new Response(null, { status: 204 }) : new Response(JSON.stringify(body), { status: 200 })
  }) as typeof fetch
  return calls
}

const graph = { nodes: [], edges: [] }

test('скрипты: создание, правка с версией, проверка и удаление идут в свои маршруты', async () => {
  const calls = fakeFetch({
    'POST /api/scripts': { id: 5, version: 1 },
    'PUT /api/scripts/5': { id: 5, version: 2 },
    'POST /api/scripts/validate': { ok: false, problems: [{ node: 1, message: 'x' }] },
    'POST /api/script-nodes/ports': { inputs: [], outputs: [{ name: 'bad_output', type: 'str' }] },
    'DELETE /api/scripts/5': null,
  })
  await createScript({ title: 't', graph })
  await updateScript(5, { version: 1, graph })
  expect((await validateScript(graph)).problems[0].message).toBe('x')
  expect((await getNodePorts('parse', { schema: [] })).outputs[0].name).toBe('bad_output')
  await deleteScript(5)
  expect(calls.map((c) => c.key)).toEqual([
    'POST /api/scripts',
    'PUT /api/scripts/5',
    'POST /api/scripts/validate',
    'POST /api/script-nodes/ports',
    'DELETE /api/scripts/5',
  ])
  expect(calls[1].body).toEqual({ version: 1, graph })
})

test('привязка скрипта: шаг и заметка — разные пути, тело несёт id скрипта', async () => {
  const calls = fakeFetch({
    'PUT /api/steps/7/script': null,
    'PUT /api/notes/3/script': null,
    'DELETE /api/steps/7/script': null,
  })
  await bindScript('step', 7, 11)
  await bindScript('note', 3, 12)
  await unbindScript('step', 7)
  expect(calls.map((c) => c.key)).toEqual([
    'PUT /api/steps/7/script',
    'PUT /api/notes/3/script',
    'DELETE /api/steps/7/script',
  ])
  expect(calls[0].body).toEqual({ script_id: 11 })
})

test('список скриптов для выбора: общий, и показывает только подходящие виду сущности', async () => {
  const calls = fakeFetch({
    'GET /api/scripts': [
      { id: 1, title: 'для шага', subjects: ['step'], needs_runner: true },
      { id: 2, title: 'для заметки', subjects: ['note'], needs_runner: true },
      { id: 3, title: 'на оба', subjects: ['step', 'note'], needs_runner: false },
    ],
  })
  await Promise.all([loadScripts(), loadScripts()])
  await loadScripts()
  expect(calls.length).toBe(1) // один запрос, пока список свежий
  expect(scriptsFor('step').map((s) => s.id)).toEqual([1])
  expect(scriptsFor('note').map((s) => s.id)).toEqual([2])
})

test('шаг со скриптом сохраняет раннер и расписание, хотя команды у него нет', () => {
  const draft = {
    ...questStepDraft({
      id: 1,
      title: 's',
      script_id: 9,
      check_interval_seconds: 30,
      run_mode: 'poll',
      run_on: 'Home',
    }),
  }
  expect(draft.script_id).toBe('9')
  const payload = checkPayload(draft)
  expect(payload.check_command).toBeNull()
  expect(payload.check_interval_seconds).toBe(30)
  expect(payload.run_on).toBe('home')

  // Без команды и без скрипта проверки нет совсем.
  const bare = checkPayload({ ...draft, script_id: '', check_command: '' })
  expect(bare.check_interval_seconds).toBeNull()
  expect(bare.run_on).toBe('')
})

import { afterEach, expect, mock, test } from 'bun:test'

import { mount, unmount } from 'svelte'

import type { RunnerJob } from '@/shared/api/execution.gen.ts'
import { setOpenTarget, type OpenTarget } from '@/shared/lib/openTarget.ts'

const real = await import('@/shared/api/client.ts')
const job = (id: number, fields: Partial<RunnerJob>): RunnerJob => ({
  id,
  runner_label: 'home',
  kind: 'poll',
  title: `Задание ${id}`,
  command: 'echo ok',
  state: 'success',
  attempt_no: 1,
  timeout_seconds: 30,
  valid_until: '2026-10-29T12:00:00Z',
  created_at: '2026-10-29T10:00:00Z',
  finished_at: '2026-10-29T10:01:00Z',
  timed_out: false,
  stdout: 'ok',
  stderr: '',
  log_tail: '',
  reason: '',
  cancel: false,
  ...fields,
})
const jobs = [
  job(1, { note_id: 120, title: 'Поиск: фильтры (quest=353, quest=354, quest=371)' }),
  job(2, { note_id: 120, title: 'Поиск: фильтры (quest=353, quest=354, quest=371)' }),
  job(3, { step_id: 787, quest_id: 227, state: 'running' }),
  job(4, { template_id: 5, state: 'queued' }),
  job(5, { quest_id: 230 }),
  job(6, { title: 'Проверка <img src=x onerror=alert(1)> quest=353' }),
]
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getRunners: async () => [],
  getRunnerJobs: async () => jobs,
}))
const { default: RunnersPage } = await import('@/pages/runners/ui/RunnersPage.svelte')
let app: ReturnType<typeof mount> | undefined
let target: HTMLDivElement
const seen: OpenTarget[] = []
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(() => {
  if (app) unmount(app)
  target?.remove()
  setOpenTarget(null)
  seen.length = 0
})
async function open() {
  target = document.createElement('div')
  document.body.append(target)
  setOpenTarget((t) => seen.push(t))
  app = mount(RunnersPage, { target })
  await tick()
  await tick()
}

test('job titles open their source across running, queued and recent jobs; steps take priority over quests', async () => {
  await open()
  for (const [kind, id] of [
    ['note', 120],
    ['step', 787],
    ['template', 5],
    ['quest', 230],
  ] as const) {
    const link = target.querySelector<HTMLAnchorElement>(`.job__title a[href="?${kind}=${id}"]`)
    expect(link).not.toBeNull()
    link!.click()
    expect(seen.at(-1)).toEqual({ kind: 'ref', ref: kind, id })
  }
  expect(target.querySelector('button a')).toBeNull()
  expect(target.querySelector('.jobs--inner')).toBeNull()
})

test('history expands independently of source links, and individual output remains accessible', async () => {
  await open()
  const toggle = target.querySelector<HTMLButtonElement>('[aria-label^="История заданий: Поиск"]')!
  expect(toggle.textContent).toContain('2')
  toggle.click()
  await tick()
  expect(toggle.getAttribute('aria-expanded')).toBe('true')
  expect(target.querySelectorAll('.jobs--inner > li')).toHaveLength(2)
  target.querySelector<HTMLButtonElement>('.jobs--inner .job__row--btn')!.click()
  await tick()
  expect(target.querySelector('.job__out .log')?.textContent).toBe('ok')
  target.querySelector<HTMLAnchorElement>('a[href="?note=120"]')!.click()
  await tick()
  expect(toggle.getAttribute('aria-expanded')).toBe('true')
})

test('jobs without a known source retain safe inline reference links', async () => {
  await open()
  expect(target.querySelector('.job__title img')).toBeNull()
  target.querySelector<HTMLAnchorElement>('a[href="?quest=353"]')!.click()
  expect(seen.at(-1)).toEqual({ kind: 'ref', ref: 'quest', id: 353 })
})

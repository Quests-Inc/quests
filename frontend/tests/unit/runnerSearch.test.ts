import { expect, test } from 'bun:test'

import { filterRunners } from '../../src/pages/runners/model/runnerSearch.ts'
import type { RunnerJob } from '../../src/shared/api/execution.gen.ts'
import type { RunnerView } from '../../src/shared/api/execution_http.gen.ts'
const devices = [
  { label: 'pc', hostname: 'workstation', online: true },
  { label: 'laptop', hostname: 'notebook', online: false },
] as RunnerView[]
const jobs = [
  { id: 1, runner_label: 'pc', state: 'queued', title: 'Диагностика сервисов' },
  { id: 2, runner_label: 'laptop', state: 'success', title: 'Проверка диска' },
  { id: 3, runner_label: 'retired', state: 'fail', title: 'Старая проверка' },
] as RunnerJob[]
test('runner search combines equals/colon filters, exclusion and words', () => {
  expect(filterRunners('device=pc status:queued диагностика', devices, jobs).jobs.map((j) => j.id)).toEqual([1])
  expect(filterRunners('status=offline', devices, jobs).devices.map((d) => d.label)).toEqual(['laptop'])
  expect(filterRunners('status=offline', devices, jobs).jobs.map((j) => j.id)).toEqual([2])
  expect(filterRunners('-device=laptop', devices, jobs).jobs.map((j) => j.id)).toEqual([1, 3])
  expect(filterRunners('workstation', devices, jobs).jobs.map((j) => j.id)).toEqual([1])
})
test('job status filters devices by related jobs and retains retired device labels', () => {
  expect(filterRunners('status=queued', devices, jobs).devices.map((d) => d.label)).toEqual(['pc'])
  expect(filterRunners('device=retired status=fail', devices, jobs).jobs.map((j) => j.id)).toEqual([3])
  expect(filterRunners('status=bogus', devices, jobs).parsed.unknown).toEqual(['status:bogus'])
})

test('runner time labels show elapsed execution and tolerate absent timestamps', async () => {
  const { span, took, at } = await import('../../src/pages/runners/model/runnerWords.ts')
  expect(span(65_000)).toBe('1 мин')
  expect(took({ started_at: '2026-11-10T10:00:00Z', finished_at: '2026-11-10T10:02:00Z' } as RunnerJob)).toBe('2 мин')
  expect(took({} as RunnerJob)).toBe('')
  expect(at('invalid')).toBeNull()
})

test('runner view slider and query share one filter without losing text or device filters', async () => {
  const { setRunnerView, runnerView } = await import('../../src/pages/runners/model/runnerSearch.ts')
  const query = setRunnerView('device=pc диагностика', 'active')
  expect(runnerView(filterRunners(query, devices, jobs).parsed)).toBe('active')
  expect(query).toContain('диагностика')
  expect(query).toContain('device:pc')
  expect(runnerView(filterRunners('view=recent', devices, jobs).parsed)).toBe('recent')
  const cleared = setRunnerView(query, 'all')
  expect(cleared).not.toContain('view=')
  expect(cleared).toContain('диагностика')
})

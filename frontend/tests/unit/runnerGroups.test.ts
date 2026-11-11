import { expect, test } from 'bun:test'

import { runnerGroups } from '../../src/pages/runners/model/runnerGroups.ts'
import type { RunnerJob } from '../../src/shared/api/execution.gen.ts'
import type { RunnerView } from '../../src/shared/api/execution_http.gen.ts'
const devices = [{ label: 'pc' }, { label: 'laptop' }, { label: 'empty' }] as RunnerView[]
const jobs = [
  { id: 1, runner_label: 'pc', state: 'success', note_id: 7, title: 'Проверка' },
  { id: 2, runner_label: 'laptop', state: 'fail', note_id: 7, title: 'Проверка' },
  { id: 3, runner_label: 'pc', state: 'fail', note_id: 7, title: 'Проверка' },
  { id: 4, runner_label: 'pc', state: 'queued', note_id: 7, title: 'Проверка' },
  { id: 5, runner_label: 'removed', state: 'running', title: 'Задание' },
] as RunnerJob[]
test('device grouping preserves separate histories and puts active jobs above finished checks', () => {
  const groups = runnerGroups(devices, jobs, 'all')
  expect(groups.map((g) => g.label)).toEqual(['pc', 'laptop', 'empty', 'removed'])
  expect(groups[0].rows.map((r) => r.last.id)).toEqual([4, 3])
  expect(groups[0].rows[1].list.map((j) => j.id)).toEqual([3, 1])
  expect(groups[1].rows[0].list.map((j) => j.id)).toEqual([2])
  expect(groups[2].total).toBe(0)
  expect(groups[3].device).toBeUndefined()
})
test('view filtering retains devices and selects the right jobs', () => {
  expect(runnerGroups(devices, jobs, 'active')[0].rows.map((r) => r.last.id)).toEqual([4])
  expect(runnerGroups(devices, jobs, 'recent')[0].rows[0].list.map((j) => j.id)).toEqual([3, 1])
})

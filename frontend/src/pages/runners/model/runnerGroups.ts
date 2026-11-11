import type { RunnerJob } from '@/shared/api/execution.gen.ts'
import type { RunnerView } from '@/shared/api/execution_http.gen.ts'

export const OPEN_STATES = ['queued', 'assigned', 'running']
const sourceKey = (j: RunnerJob) =>
  j.step_id
    ? `step:${j.step_id}`
    : j.note_id
      ? `note:${j.note_id}`
      : j.template_id
        ? `template:${j.template_id}`
        : j.quest_id
          ? `quest:${j.quest_id}`
          : `title:${j.title}`
const priority = (j: RunnerJob) => (j.state === 'running' || j.state === 'assigned' ? 0 : j.state === 'queued' ? 1 : 2)

/** Group by device first; repeated finished checks of one source remain a compact history row. */
export function runnerGroups(devices: RunnerView[], jobs: RunnerJob[], view: string) {
  const visible = jobs.filter(
    (j) => view === 'all' || (view === 'active' ? OPEN_STATES.includes(j.state) : !OPEN_STATES.includes(j.state))
  )
  const labels = [...new Set([...devices.map((d) => d.label), ...visible.map((j) => j.runner_label)])]
  return labels.map((label) => {
    const rows = new Map<string, RunnerJob[]>()
    for (const j of visible.filter((j) => j.runner_label === label).sort((a, b) => b.id - a.id)) {
      const key = OPEN_STATES.includes(j.state) ? `job:${j.id}` : sourceKey(j)
      const list = rows.get(key) ?? []
      list.push(j)
      rows.set(key, list)
    }
    return {
      label,
      device: devices.find((d) => d.label === label),
      rows: [...rows]
        .map(([key, list]) => ({
          key: `${label}:${key}`,
          list,
          last: list[0],
          failed: list.filter((j) => j.state !== 'success' && j.state !== 'canceled').length,
        }))
        .sort((a, b) => priority(a.last) - priority(b.last) || b.last.id - a.last.id),
      total: visible.filter((j) => j.runner_label === label).length,
    }
  })
}

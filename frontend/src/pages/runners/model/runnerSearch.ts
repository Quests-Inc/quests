import type { RunnerJob } from '@/shared/api/execution.gen.ts'
import type { RunnerView } from '@/shared/api/execution_http.gen.ts'
import { parseQuery, passes, tokenValues, setToken, type ParsedQuery, type FlagSpec } from '@/shared/lib/searchFlags.ts'

import { STATE } from './runnerWords.ts'

type SearchRow = { device?: RunnerView; job?: RunnerJob; related: RunnerJob[] }
const JOB_LABELS = Object.fromEntries(Object.entries(STATE).map(([id, value]) => [id, value.text]))
export const normalizeRunnerQuery = (value: string) => value.replace(/(^|\s)(-?[\p{L}]+)=/gu, '$1$2:')

export function runnerSearchSpec(devices: RunnerView[], jobs: RunnerJob[] = []): FlagSpec {
  return {
    view: {
      label: 'Задания',
      values: [
        { id: 'all', label: 'Все задания' },
        { id: 'active', label: 'В работе' },
        { id: 'recent', label: 'Недавние' },
      ],
      test: () => true,
    },
    device: {
      label: 'Устройство',
      values: [...new Set([...devices.map((d) => d.label), ...jobs.map((j) => j.runner_label)])].map((label) => ({
        id: label,
        label: devices.find((d) => d.label === label)?.hostname || label,
      })),
      test: (row: SearchRow, value: string) => (row.device?.label ?? row.job?.runner_label) === value,
    },
    status: {
      label: 'Статус',
      values: [
        { id: 'online', label: 'в сети' },
        { id: 'offline', label: 'офлайн' },
        ...Object.entries(JOB_LABELS).map(([id, label]) => ({ id, label })),
      ],
      test: (row: SearchRow, value: string) =>
        value === 'online'
          ? !!row.device?.online
          : value === 'offline'
            ? !!row.device && !row.device.online
            : row.job
              ? row.job.state === value
              : row.related.some((j) => j.state === value),
    },
  }
}

export function filterRunners(query: string, devices: RunnerView[], jobs: RunnerJob[]) {
  const spec = runnerSearchSpec(devices, jobs)
  const parsed = parseQuery(normalizeRunnerQuery(query), spec)
  const words = parsed.text.toLocaleLowerCase('ru').split(/\s+/).filter(Boolean)
  const matches = (row: SearchRow) => {
    const text = [
      row.device?.label,
      row.device?.hostname,
      row.job?.runner_label,
      row.job?.title,
      row.job?.state,
      row.device ? (row.device.online ? 'online в сети' : 'offline офлайн') : '',
      row.job ? JOB_LABELS[row.job.state] : '',
      ...row.related.map((j) => j.title),
    ]
      .join(' ')
      .toLocaleLowerCase('ru')
    return passes(parsed, spec, row) && words.every((word) => text.includes(word))
  }
  return {
    spec,
    parsed,
    devices: devices.filter((device) =>
      matches({ device, related: jobs.filter((j) => j.runner_label === device.label) })
    ),
    jobs: jobs.filter((job) =>
      matches({ job, device: devices.find((d) => d.label === job.runner_label), related: [] })
    ),
  }
}

export const runnerView = (parsed: ParsedQuery) => tokenValues(parsed, 'view').at(-1) ?? 'all'
export const setRunnerView = (query: string, view: string) =>
  setToken(normalizeRunnerQuery(query), 'view', view === 'all' ? [] : [view]).replace(/(^|\s)view:/g, '$1view=')

import { parseRefs } from '@/shared/lib/refs.ts'

/** Step drafts for the step editors (quest modal, template modal, step modal)
 * and their API payloads. One shape for the auto-check part everywhere:
 * check_command / check_interval_seconds (string, '' = default) /
 * wait_previous / run_mode, plus `open` for whether the row's details
 * (description, auto-check) are expanded. */

export function newStepKey() {
  try {
    const id = globalThis.crypto?.randomUUID?.()
    if (id) return id
  } catch {
    /* http://host is not a secure context — randomUUID throws */
  }
  return `s-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 9)}`
}

function normalizeRunMode(mode: any) {
  return mode === 'once' || mode === 'watch' ? mode : 'poll'
}

function checkDraft(s: {
  check_command: any
  check_interval_seconds: null
  wait_previous: any
  run_mode: any
  run_on: any
  script_id?: any
}) {
  return {
    check_command: s?.check_command ?? '',
    // The script a step runs instead of a command ('' = none). It is bound through its own call, not the step's payload.
    script_id: s?.script_id != null ? String(s.script_id) : '',
    check_interval_seconds: s?.check_interval_seconds != null ? String(s.check_interval_seconds) : '',
    wait_previous: Boolean(s?.wait_previous),
    run_mode: normalizeRunMode(s?.run_mode),
    run_on: s?.run_on ?? '',
  }
}

/** @param s API quest step, or nothing for a blank one. */
export function questStepDraft(s: any = null) {
  const description = s?.description ?? ''
  const check = checkDraft(s)
  return {
    key: s?.id != null ? String(s.id) : newStepKey(),
    id: s?.id ?? null,
    title: s?.title ?? '',
    description,
    progress_current: s?.progress_current ?? 0,
    progress_total: s?.progress_total ?? 1,
    ...check,
    open: Boolean(description.trim() || check.check_command.trim() || check.script_id),
  }
}

/** @param s API template step, or nothing for a blank one. */
export function templateStepDraft(s: any = null) {
  const check = checkDraft(s)
  return {
    key: s?.id != null ? String(s.id) : newStepKey(),
    title: s?.title ?? '',
    progress_range: s
      ? formatProgressRange(s.progress_min ?? s.progress_total, s.progress_max ?? s.progress_total)
      : '1',
    ...check,
    open: Boolean(check.check_command.trim()),
  }
}

/** Auto-check fields as the API wants them; all off without a command. */
export function checkPayload(s: {
  id?: null
  title?: string
  description?: any
  progress_current?: any
  progress_total?: any
  progress_range?: any
  check_command?: any
  check_interval_seconds?: any
  wait_previous?: any
  run_mode?: any
  run_on?: any
  script_id?: any
}) {
  const cmd = String(s.check_command || '').trim()
  // A step that runs a script has no command, but it keeps its schedule and its device.
  const checked = Boolean(cmd) || Boolean(String(s.script_id ?? '').trim())
  const intervalRaw = String(s.check_interval_seconds ?? '').trim()
  const interval = intervalRaw === '' ? null : Math.max(15, Number(intervalRaw) || 15)
  return {
    check_command: cmd || null,
    check_interval_seconds: checked ? interval : null,
    wait_previous: checked ? Boolean(s.wait_previous) : false,
    run_mode: checked ? normalizeRunMode(s.run_mode) : 'poll',
    // '' tells the server to clear it; no command or script, no device.
    run_on: checked
      ? String(s.run_on || '')
          .trim()
          .toLowerCase()
      : '',
  }
}

/** Untitled steps are dropped. */
export function questStepsPayload(
  steps: { id: null; title: string; description: any; progress_current: any; progress_total: any }[]
) {
  return steps
    .map((s: { id: null; title: string; description: any; progress_current: any; progress_total: any }, i: any) => ({
      id: s.id != null ? Number(s.id) : null,
      title: s.title.trim(),
      description: String(s.description || '').trim(),
      progress_current: Math.max(0, Number(s.progress_current) || 0),
      progress_total: Math.max(1, Number(s.progress_total) || 1),
      sort_order: i,
      ...checkPayload(s),
    }))
    .filter((s: { title: any }) => s.title)
}

/** Untitled steps are dropped. */
export function templateStepsPayload(steps: { title: string; progress_range: any }[]) {
  return steps
    .map((s: { title: string; progress_range: any }, i: any) => ({
      title: s.title.trim(),
      description: '',
      ...parseProgressRange(s.progress_range),
      sort_order: i,
      ...checkPayload(s),
    }))
    .filter((s: { title: any }) => s.title)
}

export function formatProgressRange(min: any, max: any) {
  const lo = Math.max(1, Number(min) || 1)
  const hi = Math.max(lo, Number(max) || lo)
  return lo === hi ? String(lo) : `${lo}..${hi}`
}

/** "5" → 5..5, "5..10" or "5-10" → 5..10 (swapped if reversed). */
export function parseProgressRange(raw: any) {
  const text = String(raw ?? '').trim()
  if (!text) return { progress_min: 1, progress_max: 1 }
  const sep = text.includes('..') ? '..' : text.includes('-') ? '-' : null
  if (sep) {
    const [a, b] = text.split(sep, 2)
    let lo = Math.max(1, Number(a) || 1)
    let hi = Math.max(1, Number(b) || lo)
    if (hi < lo) [lo, hi] = [hi, lo]
    return { progress_min: lo, progress_max: hi }
  }
  const n = Math.max(1, Number(text) || 1)
  return { progress_min: n, progress_max: n }
}

/**
 * First quest=N in the given texts (a step's title and description): that step
 * can mirror the quest with `quests progress N`.
 */
export function mirrorQuestId(...texts: string[]): number | null {
  const hit = parseRefs(texts.map((t) => String(t || '')).join('\n')).find((r) => r.kind === 'quest')
  return hit ? hit.id : null
}

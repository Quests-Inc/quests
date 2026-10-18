/** Prefer aliases over raw kinds when showing a hook's events. */
export function displayEvents(h: { events_raw?: string[]; events?: string[] }): string[] {
  if (h.events_raw?.length) return h.events_raw
  return h.events ?? []
}

export type HookDraft = {
  /** Negative local id while the quest is not saved yet. */
  localId: number
  name: string
  enabled: boolean
  events: string[]
  script_id: string
  run_on: string
  /** false = whole workspace (quest_id null). */
  thisQuest: boolean
}

let localSeq = -1
export function newHookDraft(partial: Partial<HookDraft> = {}): HookDraft {
  return {
    localId: localSeq--,
    name: '',
    enabled: true,
    events: ['complete'],
    script_id: '',
    run_on: '',
    thisQuest: true,
    ...partial,
  }
}

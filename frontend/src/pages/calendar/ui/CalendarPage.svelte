<script lang="ts">
  import { onMount } from 'svelte'
  import { listEvents } from '@/shared/api/client.ts'
  import type { EventRead } from '@/shared/api/journal.gen'
  import { parseApiDate } from '@/shared/lib/time.ts'
  import { questCatalog } from '@/entities/quest'
  import { ActivityCalendar } from '@/widgets/activity-calendar'
  import { dayKey } from '@/widgets/activity-calendar/model/day.ts'
  import EventModal from './EventModal.svelte'

  let {
    labels = {},
    onRef,
    onSelectQuest,
    onQuestContextMenu,
    onOpenHistory,
    onChanged,
  }: {
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    onSelectQuest?: (id: number) => void
    onQuestContextMenu?: (event: MouseEvent, id: number) => void
    onOpenHistory?: (dayKey: string) => void
    onChanged?: () => void
  } = $props()

  let quests = $derived(questCatalog.list)
  let events: EventRead[] = $state([])
  let modalOpen = $state(false)
  let editing: EventRead | null = $state(null)
  let createDay = $state('')
  let focusReq: { key: string; n: number } | null = $state(null)
  let focusN = 0

  async function reload() {
    try {
      const list = await listEvents()
      events = Array.isArray(list) ? list : []
    } catch {
      events = []
    }
    if (pendingId) openById(pendingId)
  }

  let pendingId = 0

  function focusEventDay(row: EventRead) {
    const start = parseApiDate(row.starts_at)
    if (!start) return
    focusN += 1
    focusReq = { key: dayKey(start), n: focusN }
  }

  function openById(id: number) {
    const row = events.find((e) => e.id === id)
    if (!row) {
      pendingId = id
      return
    }
    pendingId = 0
    focusEventDay(row)
  }

  onMount(() => {
    void reload()
    const onLive = () => { void reload() }
    const onOpen = (e: Event) => openById(Number((e as CustomEvent).detail))
    window.addEventListener('quests:events-changed', onLive)
    window.addEventListener('quests:open-event', onOpen)
    return () => {
      window.removeEventListener('quests:events-changed', onLive)
      window.removeEventListener('quests:open-event', onOpen)
    }
  })

  function openCreate(dayKey: string) {
    editing = null
    createDay = dayKey
    modalOpen = true
  }

  function openEdit(event: EventRead) {
    editing = event
    createDay = ''
    modalOpen = true
  }
</script>

<div class="journal__calendar">
  <ActivityCalendar
    {quests}
    {events}
    {labels}
    {focusReq}
    {onRef}
    {onSelectQuest}
    {onQuestContextMenu}
    {onOpenHistory}
    onCreateEvent={openCreate}
    onEditEvent={openEdit}
  />
</div>

<EventModal
  open={modalOpen}
  event={editing}
  dayKey={createDay}
  {labels}
  onClose={() => (modalOpen = false)}
  onSaved={() => {
    void reload()
    onChanged?.()
  }}
  {quests}
  onOpenQuest={onSelectQuest}
  {onRef}
/>

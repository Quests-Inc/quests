<script lang="ts">
  import { onMount } from 'svelte'
  import { listEvents } from '@/shared/api/client.ts'
  import type { EventRead } from '@/shared/api/journal.gen'
  import { questCatalog } from '@/entities/quest'
  import { ActivityCalendar } from '@/widgets/activity-calendar'
  import EventModal from './EventModal.svelte'

  let {
    labels = {},
    onRef,
    onSelectQuest,
    onQuestContextMenu,
    onOpenHistory,
  }: {
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    onSelectQuest?: (id: number) => void
    onQuestContextMenu?: (event: MouseEvent, id: number) => void
    onOpenHistory?: (dayKey: string) => void
  } = $props()

  let quests = $derived(questCatalog.list)
  let events: EventRead[] = $state([])
  let modalOpen = $state(false)
  let editing: EventRead | null = $state(null)
  let createDay = $state('')

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

  function openById(id: number) {
    const row = events.find((e) => e.id === id)
    if (!row) {
      pendingId = id
      return
    }
    pendingId = 0
    openEdit(row)
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
  onClose={() => (modalOpen = false)}
  onSaved={reload}
  {quests}
  onOpenQuest={onSelectQuest}
/>

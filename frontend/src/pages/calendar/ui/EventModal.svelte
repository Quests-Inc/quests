<script lang="ts">
  import { createEvent, deleteEvent, updateEvent } from '@/shared/api/client.ts'
  import { EVENT_COLOR, EVENT_ICONS, eventPhrase } from '@/shared/lib/event.ts'
  import { ColorIconPicker } from '@/entities/questline'
  import { localInputToUtcIso, parseApiDate, toLocalInputValue } from '@/shared/lib/time.ts'
  import type { EventRead } from '@/shared/api/journal.gen'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import DateInput from '@/shared/ui/DateInput.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import TimeSelect from '@/shared/ui/TimeSelect.svelte'

  let {
    open = false,
    event = null,
    dayKey = '',
    onClose,
    onSaved,
    quests = [],
    onOpenQuest,
  }: {
    open: boolean
    event?: EventRead | null
    dayKey?: string
    onClose: () => void
    onSaved: () => void
    quests?: {
      id: number
      title?: string
      event_id?: number | null
      event_edge?: string
      event_dir?: string
      event_offset_seconds?: number | null
      event_title?: string | null
    }[]
    onOpenQuest?: (id: number) => void
  } = $props()

  let title = $state('')
  let description = $state('')
  let color = $state(EVENT_COLOR)
  let icon = $state('calendar')
  let allDay = $state(true)
  let startDate = $state('')
  let startHour = $state('00')
  let startMinute = $state('00')
  let endDate = $state('')
  let endHour = $state('00')
  let endMinute = $state('00')
  let saving = $state(false)
  let deleting = $state(false)
  let deleteOpen = $state(false)
  let formError = $state('')

  function pad(n: number) {
    return String(n).padStart(2, '0')
  }

  function parts(iso: string | undefined) {
    const local = toLocalInputValue(iso)
    if (!local.includes('T')) return { date: '', hour: '00', minute: '00' }
    const [date, t] = local.split('T')
    const [hour = '00', minute = '00'] = (t || '').slice(0, 5).split(':')
    return { date, hour, minute }
  }

  /** Inclusive last day of an all-day interval stored as [start, next midnight). */
  function inclusiveEndDate(iso: string) {
    const d = parseApiDate(iso)
    if (!d) return ''
    if (d.getHours() === 0 && d.getMinutes() === 0) d.setDate(d.getDate() - 1)
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }

  $effect(() => {
    if (!open) return
    formError = ''
    if (event?.id) {
      title = event.title
      description = event.description || ''
      color = event.color || EVENT_COLOR
      icon = event.icon || 'calendar'
      allDay = Boolean(event.all_day)
      const start = parts(event.starts_at)
      startDate = start.date
      startHour = start.hour
      startMinute = start.minute
      if (allDay) {
        endDate = inclusiveEndDate(event.ends_at) || start.date
        endHour = '00'
        endMinute = '00'
      } else {
        const end = parts(event.ends_at)
        endDate = end.date
        endHour = end.hour
        endMinute = end.minute
      }
      return
    }
    title = ''
    description = ''
    color = EVENT_COLOR
    icon = 'calendar'
    allDay = true
    startDate = dayKey || ''
    endDate = dayKey || ''
    startHour = '12'
    startMinute = '00'
    endHour = '13'
    endMinute = '00'
  })

  function bounds() {
    if (allDay) {
      const start = localInputToUtcIso(`${startDate}T00:00`)
      const endDay = new Date(`${endDate}T00:00`)
      endDay.setDate(endDay.getDate() + 1)
      const end = localInputToUtcIso(
        `${endDay.getFullYear()}-${pad(endDay.getMonth() + 1)}-${pad(endDay.getDate())}T00:00`,
      )
      return { start, end }
    }
    return {
      start: localInputToUtcIso(`${startDate}T${startHour}:${startMinute}`),
      end: localInputToUtcIso(`${endDate}T${endHour}:${endMinute}`),
    }
  }

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!title.trim()) {
      formError = 'Нужно название'
      return
    }
    if (!startDate || !endDate) {
      formError = 'Нужны начало и конец'
      return
    }
    const { start, end } = bounds()
    if (!start || !end) {
      formError = 'Не разобрать время'
      return
    }
    saving = true
    formError = ''
    try {
      const payload = {
        title: title.trim(),
        description: description.trim(),
        starts_at: start,
        ends_at: end,
        all_day: allDay,
        color,
        icon,
      }
      if (event?.id) await updateEvent(event.id, payload)
      else await createEvent(payload)
      onSaved()
      onClose()
    } catch (err) {
      formError = err instanceof Error ? err.message : 'Не сохранилось'
    } finally {
      saving = false
    }
  }

  async function confirmDelete() {
    if (!event?.id) return
    deleting = true
    try {
      await deleteEvent(event.id)
      deleteOpen = false
      onSaved()
      onClose()
    } catch (err) {
      formError = err instanceof Error ? err.message : 'Не удалилось'
      deleteOpen = false
    } finally {
      deleting = false
    }
  }
</script>

<ModalShell {open} onClose={onClose} labelledby="event-title" closeDisabled={saving || deleting}>
  <form class="modal__form" onsubmit={onSubmit}>
    <ModalHead id="event-title" title={event?.id ? 'Событие' : 'Новое событие'} icon="calendar" onClose={onClose} closeDisabled={saving || deleting} />
    {#if formError}
      <p class="form-error" role="alert">{formError}</p>
    {/if}
    <label class="field">
      <span class="label">Название</span>
      <input type="text" bind:value={title} required />
    </label>
    <label class="field">
      <span class="label">Описание</span>
      <textarea rows="3" bind:value={description}></textarea>
    </label>
    <ColorIconPicker bind:color bind:icon icons={EVENT_ICONS} allowUpload={false} ownerLabel="этого события" />
    {#if event?.id}
      {@const linked = quests.filter((q) => q.event_id === event.id)}
      {#if linked.length}
        <div class="field">
          <span class="label">Квесты</span>
          <ul class="event-quests">
            {#each linked as q (q.id)}
              <li>
                <button type="button" class="event-quests__quest" onclick={() => onOpenQuest?.(q.id)}>{q.title}</button>
                <span class="event-quests__phrase">{eventPhrase({ ...q, event_title: title || event.title })}</span>
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    {/if}
    <label class="check">
      <input type="checkbox" bind:checked={allDay} />
      Весь день
    </label>
    <div class="field-row">
      <div class="field">
        <span class="label">Начало</span>
        <div class="event-when">
          <DateInput bind:value={startDate} label="Начало" />
          {#if !allDay}
            <TimeSelect bind:hour={startHour} bind:minute={startMinute} label="Время начала" />
          {/if}
        </div>
      </div>
      <div class="field">
        <span class="label">Конец</span>
        <div class="event-when">
          <DateInput bind:value={endDate} label="Конец" />
          {#if !allDay}
            <TimeSelect bind:hour={endHour} bind:minute={endMinute} label="Время конца" />
          {/if}
        </div>
      </div>
    </div>
    <ModalFoot
      onCancel={onClose}
      submitLabel={event?.id ? 'Сохранить' : 'Создать'}
      submitIcon={event?.id ? 'save' : 'checkmark'}
      busy={saving}
      disabled={deleting}
    >
      {#snippet left()}
        {#if event?.id}
          <button type="button" class="btn btn--danger" onclick={() => (deleteOpen = true)} disabled={saving || deleting}>
            Удалить
          </button>
        {/if}
      {/snippet}
    </ModalFoot>
  </form>
</ModalShell>

<ConfirmModal
  open={deleteOpen}
  title="Удалить событие?"
  message={title ? `Удалить «${title}»? У квестов останется уже вычисленный срок.` : ''}
  busy={deleting}
  onCancel={() => {
    if (!deleting) deleteOpen = false
  }}
  onConfirm={confirmDelete}
/>

<style>
  .event-when {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .event-when :global(.date-input) {
    flex: 1 1 auto;
    min-width: 0;
  }

  .event-when :global(.date-input__btn) {
    width: 100%;
    text-align: start;
  }

  .event-quests {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .event-quests li {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem 0.75rem;
    align-items: baseline;
  }

  .event-quests__quest {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    cursor: pointer;
  }

  .event-quests__quest:hover {
    color: var(--color-accent, #c9a227);
  }

  .event-quests__phrase {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.85rem;
  }
</style>

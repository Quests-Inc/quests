<script lang="ts">
  import {
    QUEST_SIGNIFICANCES,
    QUEST_STATUSES,
    QUEST_STATUS_LABELS,
    createQuest,
    createHook,
    updateQuest,
    deleteQuest,
    addQuestStep,
    updateQuestStep,
    deleteQuestStep,
    listCategories,
    listQuestlines,
    listTags,
    bindScript,
    unbindScript,
  } from '@/shared/api/client.ts'
  import {
    defaultLocalDeadlineParts,
    localInputToUtcIso,
    localTimeZone,
    toLocalInputValue,
  } from '@/shared/lib/time.ts'
  import { durationLabel, partsToSeconds, plural, secondsToParts } from '@/shared/lib/duration.ts'
  import { questStepDraft, questStepsPayload, StepsEditor } from '@/entities/quest'
  import { HooksField, type HookDraft } from '@/entities/hook/@x/quest'
  import Icon from '@/shared/ui/Icon.svelte'
  import { MentionTextarea } from '@/entities/template'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import Picker from '@/shared/ui/Picker.svelte'
  import FormSection from '@/shared/ui/FormSection.svelte'
  import HelpTip from '@/shared/ui/HelpTip.svelte'
  import TimeSelect from '@/shared/ui/TimeSelect.svelte'
  import DateInput from '@/shared/ui/DateInput.svelte'
  import DurationInput from '@/shared/ui/DurationInput.svelte'
  import SuggestChip from '@/shared/ui/SuggestChip.svelte'
  import { TagField } from '@/entities/tag'
  import { buildSuggestIndex } from '@/shared/lib/suggest.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import { untrack } from 'svelte'

  let {
    open = false,
    mode = 'create',
    quest = null,
    defaults = null,
    quests = [],
    notes = [],
    attachments = [],
    onClose,
    onSaved,
    onDeleted,
    onDraft = null,
  }: { open: boolean, mode: 'create' | 'edit', quest?: any, defaults?: { questline_id?: number | null, category_id?: number | null, description?: string } | null, quests?: any[], notes?: any[], attachments?: any[], onClose: () => void, onSaved: (q: any, meta?: { mode: string }) => void, onDeleted?: (id: number) => void, onDraft?: (payload: any) => void } = $props()

  let title = $state('')
  let description = $state('')
  let status = $state('active')
  let significance = $state('common')
  /** HUD presence: 'default' | 'always' (pinned) | 'never' (overlay_hidden). */
  let overlayMode = $state('default')
  const OVERLAY_OPTIONS = [
    { id: 'default', label: 'По умолчанию' },
    { id: 'always', label: 'Всегда' },
    { id: 'never', label: 'Никогда' },
  ]
  let automated = $state(false)
  let sortOrder = $state(0)
  /** Empty string = no category. */
  let categoryId = $state('')
  /** Empty string = no questline. */
  let questlineId = $state('')
  let categories: { id: number, slug: string, label: string, color?: string }[] = $state([])
  let questlines: { id: number, title: string, category_id?: number | null, color?: string }[] = $state([])
  let tagCatalog: { id: number, slug: string, label: string, color?: string }[] = $state([])
  let selectedTags: { id: number, slug: string, label: string, color?: string }[] = $state([])
  /** Local date YYYY-MM-DD + 24h clock. */
  let deadlineDate = $state('')
  let deadlineHour = $state('12')
  let deadlineMinute = $state('00')
  let durationHours = $state('')
  let durationMinutes = $state('')
  /** Off = no deadline. */
  let deadlineOn = $state(false)
  /** Unchecked = duration_seconds explicitly 0 ("no window", no auto-expire). */
  let windowEnabled = $state(true)
  let steps = $state(([] as ReturnType<typeof questStepDraft>[]))
  let saving = $state(false)
  let deleting = $state(false)
  let deleteConfirmOpen = $state(false)
  let formError = $state('')

  let secProps = $state(false)
  let secDeadline = $state(false)
  let secSteps = $state(true)
  let secHooks = $state(false)
  let hookDrafts = $state<HookDraft[]>([])

  let suggestOn = $derived(mode === 'create')
  // Questline/section suggestion (quest=205): only while creating, and only
  // until the user (or the context the dialog was opened from) has chosen.
  let lineTouched = $state(false)
  let catTouched = $state(false)
  let suggestIndex = $derived(open && suggestOn ? buildSuggestIndex(quests) : null)

  const STATUS_OPTIONS = QUEST_STATUSES.map((s) => ({
    id: s,
    label: QUEST_STATUS_LABELS[s] ?? s,
    kind: ('status' as const),
  }))
  const SIG_OPTIONS = QUEST_SIGNIFICANCES.map((s) => ({ ...s, kind: ('sig' as const) }))

  let lineOptions = $derived(
    questlines.map((l) => ({ id: String(l.id), label: l.title, color: l.color || '' })),
  )
  let categoryOptions = $derived(
    categories.map((c) => ({ id: String(c.id), label: c.label, color: c.color || '' })),
  )
  let lineCategory = $derived(
    categoryId === '' ? null : categories.find((c) => String(c.id) === categoryId) ?? null,
  )

  let propsSummary = $derived(
    [
      QUEST_STATUS_LABELS[status] ?? status,
      QUEST_SIGNIFICANCES.find((s) => s.id === significance)?.label,
      overlayMode === 'always' && 'закреплён',
      overlayMode === 'never' && 'не в оверлее',
      automated && 'автоквест',
    ]
      .filter(Boolean)
      .join(' · '),
  )

  let deadlineSummary = $derived.by(() => {
    if (!deadlineOn || !deadlineDate) return 'без срока'
    const d = new Date(`${deadlineDate}T00:00`)
    const day = Number.isNaN(d.getTime())
      ? deadlineDate
      : d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
    const when = `до ${day} ${deadlineHour}:${deadlineMinute}`
    if (!windowEnabled) return `${when} · без окна`
    const dur = durationLabel(durationHours, durationMinutes)
    return dur ? `${when} · окно ${dur}` : when
  })

  let stepsSummary = $derived.by(() => {
    const n = steps.filter((s) => s.title.trim()).length
    return n ? `${n} ${plural(n, ['шаг', 'шага', 'шагов'])}` : 'нет'
  })

  function applyQuestline(idStr: string) {
    questlineId = idStr
    if (idStr === '') return
    const line = questlines.find((l) => String(l.id) === idStr)
    if (!line) return
    categoryId = line.category_id != null ? String(line.category_id) : ''
  }

  function setDeadlineOn(on: boolean) {
    deadlineOn = on
    if (on && !deadlineDate) {
      const parts = defaultLocalDeadlineParts()
      deadlineDate = parts.date
      deadlineHour = parts.hour
      deadlineMinute = parts.minute
    }
  }

  function resetFromQuest(q: { title: string; description: any; status: string; significance: string; pinned: any; overlay_hidden: any; automated: any; sort_order: number; category_id: any; questline_id: any; deadline_at: any; duration_seconds: number; steps: any[]; tags: any[] }) {
    title = q?.title ?? ''
    description = q?.description ?? defaults?.description ?? ''
    status = q?.status ?? 'active'
    significance = q?.significance ?? 'common'
    overlayMode = q?.pinned ? 'always' : q?.overlay_hidden ? 'never' : 'default'
    automated = Boolean(q?.automated)
    sortOrder = q?.sort_order ?? 0
    const cat = q ? q.category_id : defaults?.category_id
    const line = q ? q.questline_id : defaults?.questline_id
    categoryId = cat != null ? String(cat) : ''
    questlineId = line != null ? String(line) : ''
    lineTouched = line != null
    catTouched = cat != null

    const local = toLocalInputValue(q?.deadline_at)
    deadlineOn = Boolean(local && local.includes('T'))
    if (deadlineOn) {
      const [d, t] = local.split('T')
      deadlineDate = d || ''
      const [hh = '12', mm = '00'] = (t || '').slice(0, 5).split(':')
      deadlineHour = String(Math.min(23, Math.max(0, Number(hh) || 0))).padStart(2, '0')
      deadlineMinute = String(Math.min(59, Math.max(0, Number(mm) || 0))).padStart(2, '0')
    } else {
      deadlineDate = ''
    }
    // duration_seconds === 0 is an explicit "no window" (distinct from
    // null/undefined, which just means "let the server auto-compute").
    windowEnabled = q?.duration_seconds !== 0
    ;({ hours: durationHours, minutes: durationMinutes } = secondsToParts(q?.duration_seconds))

    steps = q?.steps?.length ? q.steps.map((s: any) => questStepDraft(s)) : [questStepDraft()]
    selectedTags = Array.isArray(q?.tags) ? q.tags.map((t: any) => ({ ...t })) : []
    secProps = false
    secDeadline = false
    secSteps = true
    secHooks = false
    hookDrafts = []
  }

  // Init when `open` becomes true. Only track `open` — reading quest/mode
  // without untrack would re-run this on every silent refresh and wipe steps.
  $effect(() => {
    if (!open) return
    untrack(() => {
      formError = ''
      saving = false
      deleting = false
      deleteConfirmOpen = false
      resetFromQuest(mode === 'edit' ? quest : null)
      Promise.all([listCategories(), listQuestlines(), listTags()])
        .then(([cats, lines, tags]) => {
          categories = Array.isArray(cats) ? cats : []
          questlines = Array.isArray(lines) ? lines : []
          tagCatalog = Array.isArray(tags) ? tags : []
          if (mode === 'create' && defaults?.questline_id != null) {
            applyQuestline(String(defaults.questline_id))
          }
        })
        .catch(() => {
          categories = []
          questlines = []
          tagCatalog = []
        })
    })
  })

  /** Sync step list via CRUD (PATCH quest no longer replaces steps[]). */
  async function syncQuestSteps(questId: number, desired: any[], existing: any) {
    const desiredIds = new Set(desired.filter((s: { id: null }) => s.id != null).map((s: { id: any }) => Number(s.id)))
    let saved = null
    for (const s of desired) {
      if (s.id == null) continue
      const { id, script_id, ...body } = s
      // The script of a step that exists is bound through its own call, before the rest is saved (so the step keeps its device
      // and schedule); a command typed over it replaces it on the server.
      const was = (existing || []).find((o: { id: number }) => Number(o.id) === Number(id))?.script_id ?? null
      if (script_id && script_id !== was) await bindScript('step', id, script_id)
      else if (!script_id && was && !body.check_command) await unbindScript('step', id)
      saved = await updateQuestStep(questId, id, body)
    }
    for (const s of desired) {
      if (s.id != null) continue
      const { id: _id, ...body } = s
      saved = await addQuestStep(questId, body)
    }
    for (const old of existing || []) {
      if (desiredIds.has(Number(old.id))) continue
      saved = await deleteQuestStep(questId, old.id)
    }
    return saved
  }

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!title.trim()) {
      formError = 'Нужен заголовок'
      return
    }
    const stepsPayload = questStepsPayload(steps)
    saving = true
    formError = ''
    try {
      const deadline_at = deadlineOn
        ? localInputToUtcIso(`${deadlineDate}T${deadlineHour}:${deadlineMinute}`)
        : null
      const payload: Record<string, any> = {
        title: title.trim(),
        description: description.trim(),
        status,
        significance,
        pinned: overlayMode === 'always',
        overlay_hidden: overlayMode === 'never',
        automated,
        sort_order: Number(sortOrder) || 0,
        category_id: categoryId === '' ? null : Number(categoryId),
        questline_id: questlineId === '' ? null : Number(questlineId),
        tag_ids: selectedTags.map((t) => t.id),
        deadline_at,
      }
      if (!deadline_at) {
        payload.duration_seconds = null
      } else if (!windowEnabled) {
        // Explicit 0 = no urgency window, no auto-expire (see NormalizeDeadline).
        payload.duration_seconds = 0
      } else {
        const dur = partsToSeconds(durationHours, durationMinutes)
        if (dur != null) payload.duration_seconds = dur
      }
      let saved
      if (mode === 'edit' && onDraft) {
        // From the relations map: the change goes into the draft, steps are
        // edited separately there.
        onDraft(payload)
        onClose()
        return
      }
      if (mode === 'create' && onDraft) {
        // From the relations map: goes into the draft, nothing is created yet.
        onDraft({ ...payload, steps: stepsPayload })
        onClose()
        return
      }
      if (mode === 'create') {
        saved = await createQuest({ ...payload, steps: stepsPayload })
        const qid = Number(saved?.id)
        if (qid > 0 && hookDrafts.length) {
          for (const d of hookDrafts) {
            const sid = Number(d.script_id)
            if (!(sid > 0) || !d.events.length) continue
            await createHook({
              name: d.name.trim(),
              enabled: d.enabled,
              events: d.events,
              script_id: sid,
              run_on: d.run_on.trim(),
              quest_id: d.thisQuest ? qid : null,
            })
          }
          hookDrafts = []
        }
      } else {
        saved = await updateQuest(quest.id, payload)
        const afterSteps = await syncQuestSteps(quest.id, stepsPayload, quest?.steps || [])
        if (afterSteps) saved = afterSteps
      }
      onSaved(saved, { mode })
      onClose()
    } catch (e) {
      formError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  async function confirmDelete() {
    if (!quest?.id) return
    deleting = true
    formError = ''
    try {
      await deleteQuest(quest.id)
      deleteConfirmOpen = false
      onDeleted?.(quest.id)
      onClose()
    } catch (e) {
      deleteConfirmOpen = false
      formError = e.message || String(e)
    } finally {
      deleting = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="quest-modal-title" zIndex={40} maxWidth="36rem">
  <ModalHead
    id="quest-modal-title"
    title={mode === 'create' ? 'Новый квест' : 'Редактировать квест'}
    icon={mode === 'create' ? 'add' : 'edit'}
    {onClose}
  />

  {#if formError}
    <p class="modal__error">{formError}</p>
  {/if}

  <form class="modal__form" onsubmit={onSubmit}>
    <label class="field">
      <span class="label">Заголовок</span>
      <input type="text" bind:value={title} required />
    </label>

    <div class="field">
      <span class="label">Описание</span>
      <MentionTextarea
        bind:value={description}
        {quests}
        {questlines}
        {notes}
        {attachments}
        rows={3}
        placeholder="@название — квест, заметка, файл, шаг, квестлайн"
      />
    </div>

    <div class="field">
      <span class="label">Теги</span>
      <TagField
        catalog={tagCatalog}
        value={selectedTags}
        {title}
        {description}
        {quests}
        enabled={open}
        onChange={(tags) => (selectedTags = tags)}
        onCatalog={(tags) => (tagCatalog = tags)}
      />
    </div>

    <div class="field-row">
      <div class="field">
        <span class="label">Квестлайн</span>
        <Picker
          options={lineOptions}
          bind:value={questlineId}
          label="Квестлайн"
          onChange={(id) => {
            lineTouched = true
            applyQuestline(id)
          }}
        />
        <SuggestChip
          index={suggestIndex}
          {title}
          {description}
          target="questline"
          options={lineOptions}
          enabled={suggestOn && questlineId === '' && !lineTouched}
          onAccept={(id) => {
            lineTouched = true
            applyQuestline(id)
          }}
        />
        {#if questlineId !== '' && lineCategory}
          <span class="hint">раздел «{lineCategory.label}» — от квестлайна</span>
        {/if}
      </div>
      {#if questlineId === ''}
        <div class="field">
          <span class="label">Раздел</span>
          <Picker options={categoryOptions} bind:value={categoryId} label="Раздел" onChange={() => (catTouched = true)} />
          <SuggestChip
            index={suggestIndex}
            {title}
            {description}
            target="category"
            options={categoryOptions}
            enabled={suggestOn && categoryId === '' && !catTouched}
            onAccept={(id) => {
              catTouched = true
              categoryId = id
            }}
          />
        </div>
      {/if}
    </div>

    <FormSection title="Свойства" summary={propsSummary} bind:open={secProps}>
      <div class="field">
        <span class="label">Статус</span>
        <OptionPills options={STATUS_OPTIONS} bind:value={status} label="Статус" wrap />
      </div>
      <div class="field">
        <span class="label">Значимость</span>
        <OptionPills options={SIG_OPTIONS} bind:value={significance} label="Значимость" wrap />
      </div>
      <div class="check-line">
        <span class="hint">В оверлее</span>
        <OptionPills options={OVERLAY_OPTIONS} bind:value={overlayMode} label="Показ в оверлее" compact />
        <HelpTip label="Что такое показ в оверлее">
          По умолчанию — срочные и по категориям. Всегда — закреплён в HUD. Никогда — квест не попадает в HUD вообще (тосты это не меняет).
        </HelpTip>
      </div>
      <div class="check-line">
        <label class="check">
          <input type="checkbox" bind:checked={automated} />
          Автоквест
        </label>
        <HelpTip label="Что такое автоквест">
          Появление и старт такого квеста — тихий тост, а не оповещение на весь экран. Для квестов, которые создают скрипты и шаблоны.
        </HelpTip>
      </div>
    </FormSection>

    <FormSection title="Срок" summary={deadlineSummary} bind:open={secDeadline}>
      <label class="check">
        <input type="checkbox" checked={deadlineOn} onchange={(e) => setDeadlineOn(e.currentTarget.checked)} />
        Указать срок
        <span class="hint">({localTimeZone()})</span>
      </label>
      {#if deadlineOn}
        <div class="inline-line">
          <DateInput bind:value={deadlineDate} label="Дата срока" />
          <TimeSelect bind:hour={deadlineHour} bind:minute={deadlineMinute} label="Время срока" />
        </div>
        <div class="inline-line">
          <label class="check">
            <input type="checkbox" bind:checked={windowEnabled} />
            Окно срочности
          </label>
          {#if windowEnabled}
            <DurationInput bind:hours={durationHours} bind:minutes={durationMinutes} label="Окно срочности" />
          {/if}
          <HelpTip label="Что такое окно срочности">
            <p>Окно — сколько времени до срока квест считается срочным: таймер, напоминание в HUD и Telegram. По истечении срока квест просрочен.</p>
            <p>Пустая длительность — окно от создания квеста до срока. Без окна срок — просто ориентир: без таймера и без автопросрочки.</p>
          </HelpTip>
        </div>
      {/if}
    </FormSection>

    {#if !(onDraft && mode === 'edit')}
      <FormSection title="Шаги" summary={stepsSummary} bind:open={secSteps}>
        <StepsEditor bind:steps variant="quest" {quests} {questlines} {notes} {attachments} />
      </FormSection>
    {/if}

    {#if !onDraft}
      <FormSection
        title="Хуки"
        summary={mode === 'create' ? (hookDrafts.length ? `${hookDrafts.length}` : '') : ''}
        bind:open={secHooks}
      >
        <HooksField
          questId={mode === 'edit' ? quest?.id : null}
          bind:drafts={hookDrafts}
          defaultEvents={['complete']}
          showScope={true}
          open={secHooks}
        />
      </FormSection>
    {/if}

    <ModalFoot
      onCancel={onClose}
      submitLabel={onDraft ? 'В черновик' : mode === 'create' ? 'Создать' : 'Сохранить'}
      submitIcon={mode === 'create' ? 'checkmark' : 'save'}
      busy={saving}
      disabled={deleting}
    >
      {#snippet left()}
        {#if mode === 'edit' && !onDraft}
          <button
            type="button"
            class="btn btn--danger"
            onclick={() => (deleteConfirmOpen = true)}
            disabled={saving || deleting}
            title="Удалить квест"
          >
            <Icon name="delete" size={14} />
            <span class="btn__text">{deleting ? '…' : 'Удалить'}</span>
          </button>
        {/if}
      {/snippet}
    </ModalFoot>
  </form>
</ModalShell>

<ConfirmModal
  open={deleteConfirmOpen}
  title="Удалить квест?"
  message={quest ? `Удалить квест «${quest.title}»?` : ''}
  busy={deleting}
  onCancel={() => {
    if (!deleting) deleteConfirmOpen = false
  }}
  onConfirm={confirmDelete}
/>

<style>
  .check-line {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .inline-line .deadline-date {
    width: auto;
  }
</style>

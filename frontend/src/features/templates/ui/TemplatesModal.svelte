<script lang="ts">
  import {
    WEEKDAY_LABELS,
    QUEST_SIGNIFICANCES,
    copyTemplate,
    emitTemplate,
    createTemplate,
    deleteTemplate,
    getTemplateStreak,
    listCategories,
    listQuestlines,
    listTags,
    listTemplates,
    updateTemplate,
  } from '@/shared/api/client.ts'
  import { rowMeta, templateMenuItems } from '../model/rowInfo.ts'
  import { createTemplateActions } from '../model/templateActions.ts'
  import { defaultLocalDeadlineParts, localTimeZone } from '@/shared/lib/time.ts'
  import { durationLabel, partsToSeconds, plural, secondsToParts } from '@/shared/lib/duration.ts'
  import { attachmentCatalog, flattenAttachmentIndex } from '@/entities/attachment'
  import { noteCatalog } from '@/entities/note'
  import { questCatalog, templateStepDraft, templateStepsPayload, StepsEditor } from '@/entities/quest'
  import type { TemplateRead } from '@/shared/api/automation.gen'
  import Icon from '@/shared/ui/Icon.svelte'
  import { MentionTextarea, WORKDAYS, daysLabel, parseClock, parseLimits, parseWeekdays } from '@/entities/template'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import Picker from '@/shared/ui/Picker.svelte'
  import FormSection from '@/shared/ui/FormSection.svelte'
  import HelpTip from '@/shared/ui/HelpTip.svelte'
  import TimeSelect from '@/shared/ui/TimeSelect.svelte'
  import DurationInput from '@/shared/ui/DurationInput.svelte'
  import SuggestChip from '@/shared/ui/SuggestChip.svelte'
  import { TagField } from '@/entities/tag'
  import { buildSuggestIndex } from '@/shared/lib/suggest.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import TemplateList from './TemplateList.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import { untrack } from 'svelte'
  import { loadRunners, runnersList } from '@/entities/runner'

  /**
   * `focusId` opens that template for editing (a template=N link);
   * `onOpenSecrets(id)` opens the secrets manager on a template.
   */
  let { open = false, focusId = null, onClose, onChanged, onOpenSecrets }: { open: boolean, focusId?: number | null, onClose: () => void, onChanged: () => void, onOpenSecrets?: (id: number) => void } = $props()

  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)
  let attachments = $derived(flattenAttachmentIndex(attachmentCatalog.index).filter((a: { hidden?: boolean }) => !a.hidden))

  const LOCAL_TZ = localTimeZone() || 'Europe/Moscow'

  let templates = $state(([] as any[]))
  let loading = $state(false)
  let error = $state('')
  let view = $state(('list' as 'list' | 'create' | 'edit'))
  let editing = $state((null as any | null))

  let title = $state('')
  let description = $state('')
  let tagCatalog: { id: number, slug: string, label: string, color?: string }[] = $state([])
  let selectedTags: { id: number, slug: string, label: string, color?: string }[] = $state([])
  /** HUD presence of created quests: 'default' | 'always' | 'never'. */
  let overlayMode = $state('default')
  const OVERLAY_OPTIONS = [
    { id: 'default', label: 'По умолчанию' },
    { id: 'always', label: 'Всегда' },
    { id: 'never', label: 'Никогда' },
  ]
  let significance = $state('common')
  let enabled = $state(true)
  let automated = $state(false)
  let freq = $state('daily')
  let emitMode = $state('fixed')
  /** Empty string = no category / no questline. */
  let categoryId = $state('')
  let questlineId = $state('')
  let categories: { id: number, slug: string, label: string, color?: string }[] = $state([])
  let questlines: { id: number, title: string, category_id?: number | null, color?: string }[] = $state([])
  /** 0..100 for UI; sent as 0..1 */
  let emitChancePct = $state(100)
  let windowStartHour = $state('09')
  let windowStartMinute = $state('00')
  let windowEndHour = $state('18')
  let windowEndMinute = $state('00')
  let weekdays: Set<number> = $state(new Set(WORKDAYS))
  let timezone = $state(LOCAL_TZ)
  let tzEditing = $state(false)
  /** Empty hour = no deadline. */
  let deadlineHour = $state('')
  let deadlineMinute = $state('00')
  /** Shell command run per roll; stdout must be a JSON array of
   * {title, description?, weight?, ref?}. Empty = no pool (normal template). */
  let emitPoolCommand = $state('')
  /** JSON text of the emit limits (empty = defaults) */
  let emitLimitsText = $state('')
  let emitRunOn = $state('')
  let durationHours = $state('')
  let durationMinutes = $state('')
  let steps = $state(([] as ReturnType<typeof templateStepDraft>[]))
  let saving = $state(false)
  let deleting = $state(false)
  let deleteConfirmOpen = $state(false)
  let formError = $state('')

  let secSchedule = $state(true)
  let secProps = $state(false)
  let secPool = $state(false)
  let secSteps = $state(true)

  let suggestOn = $derived(view === 'create')
  // Questline/section suggestion (quest=205): only while creating, and only
  // until the user (or the context the dialog was opened from) has chosen.
  let lineTouched = $state(false)
  let catTouched = $state(false)
  let suggestIndex = $derived(open && suggestOn ? buildSuggestIndex(quests) : null)

  const FREQ_OPTIONS = [
    { id: 'daily', label: 'каждый день' },
    { id: 'weekly', label: 'по дням недели' },
  ]
  const MODE_OPTIONS = [
    { id: 'fixed', label: 'по расписанию' },
    { id: 'surprise', label: 'случайно' },
  ]
  const DAY_OPTIONS = WEEKDAY_LABELS.map((d) => ({ id: String(d.id), label: d.label }))
  const SIG_OPTIONS = QUEST_SIGNIFICANCES.map((s) => ({ ...s, kind: ('sig' as const) }))

  let isSurprise = $derived(emitMode === 'surprise')
  let hasDeadline = $derived(!isSurprise && Boolean(deadlineHour))
  let hasPool = $derived(Boolean(emitPoolCommand.trim()))
  $effect(() => {
    if (hasPool) {
      void loadRunners()
    }
  })

  let weekdayIds = $derived(new Set([...weekdays].map(String)))

  let lineOptions = $derived(
    questlines.map((l) => ({ id: String(l.id), label: l.title, color: l.color || '' })),
  )
  let categoryOptions = $derived(
    categories.map((c) => ({ id: String(c.id), label: c.label, color: c.color || '' })),
  )
  let lineCategory = $derived(
    categoryId === '' ? null : categories.find((c) => String(c.id) === categoryId) ?? null,
  )

  let scheduleSummary = $derived.by(() => {
    const days = freq === 'weekly' ? daysLabel(weekdays) : 'каждый день'
    const dur = durationLabel(durationHours, durationMinutes)
    const parts = [days]
    if (isSurprise) {
      parts.push(
        `${emitChancePct}% между ${windowStartHour}:${windowStartMinute}–${windowEndHour}:${windowEndMinute}`,
      )
      if (dur) parts.push(`длится ${dur}`)
    } else if (hasDeadline) {
      parts.push(`срок ${deadlineHour}:${deadlineMinute}`)
      if (dur) parts.push(`окно ${dur}`)
    } else {
      parts.push('без срока')
    }
    if (timezone.trim() && timezone.trim() !== LOCAL_TZ) parts.push(timezone.trim())
    return parts.join(' · ')
  })

  let propsSummary = $derived(
    [
      QUEST_SIGNIFICANCES.find((s) => s.id === significance)?.label,
      enabled ? 'включён' : 'выключен',
      overlayMode === 'always' && 'закреплён',
      overlayMode === 'never' && 'не в оверлее',
      automated && 'автоквест',
    ]
      .filter(Boolean)
      .join(' · '),
  )

  let poolSummary = $derived(!hasPool ? 'без команды' : (emitRunOn.trim() ? `команда на «${emitRunOn.trim()}»` : 'команда') + (emitLimitsText.trim() ? ' · свои лимиты' : ''))

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

  function resetForm(t: any = null) {
    formError = ''
    title = t?.title ?? ''
    description = t?.description ?? ''
    overlayMode = t?.pinned ? 'always' : t?.overlay_hidden ? 'never' : 'default'
    significance = t?.significance ?? 'common'
    enabled = t ? t.enabled !== false : true
    automated = Boolean(t?.automated)
    freq = t?.freq ?? 'daily'
    emitMode = t?.emit_mode === 'surprise' ? 'surprise' : 'fixed'
    categoryId = t?.category_id != null ? String(t.category_id) : ''
    questlineId = t?.questline_id != null ? String(t.questline_id) : ''
    lineTouched = false
    catTouched = false
    emitChancePct = t ? Math.round(Math.max(0, Math.min(1, Number(t.emit_chance) || 1)) * 100) : 100
    emitPoolCommand = t?.emit_pool_command ?? ''
    emitLimitsText = t?.emit_limits ? JSON.stringify(t.emit_limits) : ''
    emitRunOn = t?.emit_run_on ?? ''
    const ws = parseClock(t?.emit_window_start, '09', '00')
    const we = parseClock(t?.emit_window_end, '18', '00')
    windowStartHour = ws.hour
    windowStartMinute = ws.minute
    windowEndHour = we.hour
    windowEndMinute = we.minute
    weekdays = t ? parseWeekdays(t.weekdays) : new Set(WORKDAYS)
    timezone = t?.timezone || LOCAL_TZ
    tzEditing = false
    if (!t) {
      const parts = defaultLocalDeadlineParts()
      deadlineHour = parts.hour
      deadlineMinute = parts.minute
    } else {
      const d = parseClock(t.deadline_time, '', '00')
      deadlineHour = d.hour
      deadlineMinute = d.minute
    }
    ;({ hours: durationHours, minutes: durationMinutes } = secondsToParts(t?.duration_seconds))
    steps = t?.steps?.length ? t.steps.map((s: any) => templateStepDraft(s)) : [templateStepDraft()]
    selectedTags = Array.isArray(t?.tags) ? t.tags.map((x: any) => ({ ...x })) : []
    // A new template starts at its schedule; an existing one reads fine
    // from the one-line summary.
    secSchedule = !t
    secProps = false
    secPool = false
    secSteps = true
  }

  /** Per-template close streaks, loaded after the list so rows render at once. */
  let streaks = $state(({} as Record<number, any>))

  async function loadStreaks(tpls: any[]) {
    const out = ({} as Record<number, any>)
    await Promise.all(
      tpls.map(async (t) => {
        try {
          out[t.id] = await getTemplateStreak(t.id)
        } catch {
          /* no streak for this template */
        }
      }),
    )
    streaks = out
  }


  async function refresh() {
    loading = true
    error = ''
    try {
      const [tpls, cats, lines, tags] = await Promise.all([
        listTemplates({}),
        listCategories(),
        listQuestlines(),
        listTags(),
      ])
      templates = tpls
      void loadStreaks(tpls)
      categories = Array.isArray(cats) ? cats : []
      questlines = Array.isArray(lines) ? lines : []
      tagCatalog = Array.isArray(tags) ? tags : []
    } catch (e) {
      error = e.message || String(e)
      templates = []
    } finally {
      loading = false
    }
  }

  // Reset list when modal opens. Only track `open` (untrack the rest).
  $effect(() => {
    if (!open) return
    untrack(() => {
      view = 'list'
      editing = null
      void refresh()
    })
  })

  function backToList() {
    view = 'list'
    editing = null
  }

  function handleClose() {
    if (view !== 'list') backToList()
    else onClose()
  }

  function openCreate() {
    editing = null
    view = 'create'
    resetForm(null)
  }

  function openEdit(t: any) {
    editing = t
    view = 'edit'
    resetForm(t)
  }

  function toggleDay(idStr: any) {
    const id = Number(idStr)
    const next = new Set(weekdays)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    if (next.size === 0) next.add(id)
    weekdays = next
  }

  function buildPayload() {
    const duration_seconds = partsToSeconds(durationHours, durationMinutes)
    const common = {
      title: title.trim(),
      description: description.trim(),
      pinned: overlayMode === 'always',
      overlay_hidden: overlayMode === 'never',
      enabled,
      automated,
      significance,
      freq,
      weekdays: [...weekdays].sort((a, b) => a - b).join(','),
      timezone: timezone.trim() || LOCAL_TZ,
      emit_pool_command: emitPoolCommand.trim() || null,
      emit_limits: emitLimitsText.trim() ? parseLimits(emitLimitsText) : null,
      emit_run_on: emitPoolCommand.trim() ? emitRunOn.trim().toLowerCase() || null : null,
      category_id: categoryId === '' ? null : Number(categoryId),
      questline_id: questlineId === '' ? null : Number(questlineId),
      tag_ids: selectedTags.map((t) => t.id),
      steps: templateStepsPayload(steps),
    }
    if (isSurprise) {
      return {
        ...common,
        emit_mode: 'surprise',
        emit_chance: Math.max(0, Math.min(100, Number(emitChancePct) || 0)) / 100,
        emit_window_start: `${windowStartHour}:${windowStartMinute}`,
        emit_window_end: `${windowEndHour}:${windowEndMinute}`,
        deadline_time: null as string | null,
        duration_seconds,
      }
    }
    const deadline_time = hasDeadline ? `${deadlineHour}:${deadlineMinute}` : null
    return {
      ...common,
      emit_mode: 'fixed',
      emit_chance: 1,
      emit_window_start: null,
      emit_window_end: null,
      deadline_time,
      duration_seconds: deadline_time ? duration_seconds : null,
    }
  }

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!title.trim()) {
      formError = 'Нужен заголовок'
      return
    }
    let payload
    try {
      payload = buildPayload()
    } catch (e) {
      formError = e.message || String(e)
      secPool = true
      return
    }
    // The command's quest brings its own steps, so the "constant" steps below
    // are just a fallback while a command is set — don't force filling one in.
    if (!payload.steps.length && !hasPool) {
      formError = 'Нужен хотя бы один шаг'
      secSteps = true
      return
    }
    saving = true
    formError = ''
    try {
      if (view === 'create') await createTemplate(payload)
      else await updateTemplate(editing.id, payload)
      onChanged()
      await refresh()
      backToList()
    } catch (e) {
      formError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  // Right-click menu on a template row: the buttons' actions plus id/secrets.
  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxId = $state((null as number | null))
  let ctxTemplate = $derived(templates.find((t) => t.id === ctxId) ?? null)
  let ctxItems = $derived(templateMenuItems(ctxTemplate))

  function openTemplateMenu(event: MouseEvent, t: any) {
    event.preventDefault()
    event.stopPropagation()
    ctxId = t.id
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  async function onTemplateMenuSelect(action: string) {
    const t = ctxTemplate
    if (!t) return
    if (action === 'copy-id') await actions.copyRef(t)
    else if (action === 'edit') openEdit(t)
    else if (action === 'emit') await onEmit(t)
    else if (action === 'duplicate') await onCopy(t)
    else if (action === 'toggle') await onToggleEnabled(t)
    else if (action === 'secrets') onOpenSecrets?.(t.id)
  }

  // A template=N link opens the modal straight on that template's edit form —
  // once per opening, so going back to the list stays on the list.
  let focusApplied = false
  $effect(() => {
    if (!open) {
      focusApplied = false
      return
    }
    if (focusId == null || focusApplied || view !== 'list') return
    const t = templates.find((x) => x.id === focusId)
    if (!t) return
    focusApplied = true
    untrack(() => openEdit(t))
  })

  const actions = createTemplateActions({
    api: { update: updateTemplate, copy: copyTemplate, emit: emitTemplate },
    refresh: () => refresh(),
    onChanged: () => onChanged(),
    setError: (m) => (error = m),
    notify: (m, kind, ttl) => toast(m, { kind, ttl }),
    copyText,
  })
  const onToggleEnabled = actions.toggleEnabled
  function onCopy(t: { id: number }, event?: Event) {
    event?.stopPropagation?.()
    return actions.duplicate(t)
  }
  function onEmit(t: any, event?: Event) {
    event?.stopPropagation?.()
    return actions.emit(t)
  }

  async function confirmDelete() {
    if (!editing?.id) return
    deleting = true
    formError = ''
    try {
      await deleteTemplate(editing.id)
      deleteConfirmOpen = false
      onChanged()
      await refresh()
      backToList()
    } catch (e) {
      // Close the confirm dialog so the edit form behind it — where
      // formError actually renders — is visible instead of a silent no-op.
      deleteConfirmOpen = false
      formError = e.message || String(e)
    } finally {
      deleting = false
    }
  }

  const metaOf = (t: TemplateRead) => rowMeta(t, streaks[t.id])
</script>

<ModalShell {open} onClose={handleClose} labelledby="templates-modal-title" zIndex={40} maxWidth="36rem">
  <ModalHead
    id="templates-modal-title"
    title={view === 'list' ? 'Шаблоны' : view === 'create' ? 'Новый шаблон' : 'Редактировать шаблон'}
    icon="repeat"
    {onClose}
  />

  {#if view === 'list'}
    <TemplateList {error} {loading} {templates} rowMeta={metaOf} onCreate={openCreate} onOpen={openEdit} onMenu={openTemplateMenu} {onEmit} {onCopy} {onToggleEnabled} />
  {:else}
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
          rows={2}
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
          enabled={view === 'create' || view === 'edit'}
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

      <FormSection title="Расписание" summary={scheduleSummary} bind:open={secSchedule}>
        <div class="inline-line">
          <OptionPills options={FREQ_OPTIONS} bind:value={freq} label="Частота" compact />
          {#if freq === 'weekly'}
            <OptionPills options={DAY_OPTIONS} selected={weekdayIds} onToggle={toggleDay} label="Дни недели" compact />
          {/if}
        </div>
        <div class="inline-line">
          <OptionPills options={MODE_OPTIONS} bind:value={emitMode} label="Как появляется" compact />
          <HelpTip label="По расписанию или случайно">
            <p><b>По расписанию</b> — квест появляется в начале каждого дня периода, срок — в указанное время.</p>
            <p><b>Случайно</b> — одно событие в день: с заданным шансом квест появляется в случайный момент внутри окна и живёт указанное время.</p>
          </HelpTip>
        </div>

        {#if isSurprise}
          <div class="inline-line">
            с шансом
            <input type="number" min="0" max="100" step="1" bind:value={emitChancePct} aria-label="Шанс появления, %" />
            % между
            <TimeSelect bind:hour={windowStartHour} bind:minute={windowStartMinute} label="Начало окна появления" />
            и
            <TimeSelect bind:hour={windowEndHour} bind:minute={windowEndMinute} label="Конец окна появления" />
          </div>
          <div class="inline-line">
            длится
            <DurationInput bind:hours={durationHours} bind:minutes={durationMinutes} label="Сколько живёт после появления" />
            <span class="hint">пусто — без срока</span>
          </div>
        {:else}
          <div class="inline-line">
            срок
            <TimeSelect bind:hour={deadlineHour} bind:minute={deadlineMinute} allowNone label="Срок" />
            {#if hasDeadline}
              окно
              <DurationInput bind:hours={durationHours} bind:minutes={durationMinutes} label="Окно срочности" />
            {/if}
            <HelpTip label="Срок и окно">
              <p>Срок — время дня; «—» означает квест без срока.</p>
              <p>Окно — сколько до срока квест срочный. Пустое окно — от полуночи до срока.</p>
            </HelpTip>
          </div>
        {/if}

        <div class="inline-line tz-line">
          <span class="hint">часовой пояс</span>
          {#if tzEditing}
            <input class="tz-input" type="text" bind:value={timezone} placeholder={LOCAL_TZ} aria-label="Часовой пояс" />
            <button
              type="button"
              class="btn btn--link hint"
              onclick={() => {
                timezone = LOCAL_TZ
                tzEditing = false
              }}
            >
              системный
            </button>
          {:else}
            <span class="hint">{timezone}{timezone === LOCAL_TZ ? ' (системный)' : ''}</span>
            <button type="button" class="btn btn--link hint" onclick={() => (tzEditing = true)}>изменить</button>
          {/if}
        </div>
      </FormSection>

      <FormSection title="Свойства" summary={propsSummary} bind:open={secProps}>
        <div class="field">
          <span class="label">Значимость</span>
          <OptionPills options={SIG_OPTIONS} bind:value={significance} label="Значимость" wrap />
        </div>
        <label class="check">
          <input type="checkbox" bind:checked={enabled} />
          Включён — создавать квесты по расписанию
        </label>
        <div class="check-line">
          <span class="hint">Созданные квесты в оверлее</span>
          <OptionPills options={OVERLAY_OPTIONS} bind:value={overlayMode} label="Показ созданных квестов в оверлее" compact />
        </div>
        <label class="check">
          <input type="checkbox" bind:checked={automated} />
          Автоквест — тихий тост вместо оповещения
        </label>
      </FormSection>

      <FormSection title="Контент" summary={poolSummary} bind:open={secPool}>
        <div class="field">
          <span class="label">
            Команда или скрипт пула
            <HelpTip label="Как работает команда">
              <p>Запускается при каждом появлении. stdout — один квест в JSON: <code>{'{title?, description?, significance?, questline?, tags?, deadline_at?, steps: [{title, check_command?, …}]}'}</code>. Заданное в нём перекрывает поля шаблона, остальное берётся из шаблона. Пустой вывод, <code>null</code> или <code>{'{}'}</code> — квеста в этот день нет.</p>
              <p>Повторы команда не отслеживает: пока письмо не прочитано, квест про него будет появляться снова.</p>
              <p>Текст с <code>#!</code> в первой строке — целый скрипт, запускается своим интерпретатором. Секреты — в меню «Секреты», в скрипт приходят переменными окружения.</p>
            </HelpTip>
          </span>
          <textarea
            class="mono pool-command"
            rows="4"
            placeholder={'find ~/reading -name "*.md"\nили скрипт: #!/usr/bin/env python3 …'}
            bind:value={emitPoolCommand}
            spellcheck="false"
          ></textarea>
        </div>
        {#if hasPool}
          <label class="field">
            <span class="label">
              Устройство
              <HelpTip label="Устройство">
                <p>Метка раннера (<code>quests-runner</code>), на котором выполнить команду (обязательно). Устройство офлайн — команда ждёт его до суток, шаблон при этом не считается сломанным: период просто пропускается.</p>
              </HelpTip>
            </span>
            <select bind:value={emitRunOn} aria-label="Устройство">
              <option value="" disabled>выберите устройство</option>
              {#each runnersList.items as r (r.label)}
                <option value={r.label}>{r.label}{r.online ? '' : r.registered ? ' (офлайн)' : ' (не подключён)'}</option>
              {/each}
              {#if emitRunOn && !runnersList.items.some((r) => r.label === emitRunOn)}
                <option value={emitRunOn}>{emitRunOn} (нет такого раннера)</option>
              {/if}
            </select>
          </label>
          <label class="field">
            <span class="label">
              Лимиты ответа (JSON, необязательно)
              <HelpTip label="Лимиты ответа">
                <p>Ограничения на квест, который печатает команда. По умолчанию: <code>max_steps</code> 30, <code>max_title</code> 200, <code>max_description</code> 20000, <code>max_command</code> 2000. Указывайте только то, что меняете.</p>
              </HelpTip>
            </span>
            <input type="text" class="mono" placeholder={'{"max_steps": 50}'} bind:value={emitLimitsText} spellcheck="false" />
          </label>
        {/if}
      </FormSection>

      <FormSection title="Шаги" summary={stepsSummary} bind:open={secSteps}>
        {#if hasPool}
          <p class="hint">Пока задана команда, шаги берутся из её квеста — эти запасные, на случай если она не напечатала шаги.</p>
        {/if}
        <StepsEditor bind:steps variant="template" />
      </FormSection>

      <ModalFoot
        onCancel={backToList}
        submitLabel={view === 'create' ? 'Создать' : 'Сохранить'}
        submitIcon={view === 'create' ? 'checkmark' : 'save'}
        busy={saving}
        disabled={deleting}
      >
        {#snippet left()}
          {#if view === 'edit'}
            <button
              type="button"
              class="btn btn--danger"
              onclick={() => (deleteConfirmOpen = true)}
              disabled={saving || deleting}
              title="Удалить шаблон"
            >
              <Icon name="delete" size={14} />
              <span class="btn__text">Удалить</span>
            </button>
            <button
              type="button"
              class="btn btn--ghost"
              onclick={() => onCopy(editing)}
              disabled={saving || deleting}
              title="Копировать (копия будет выключена)"
            >
              <Icon name="copy" size={14} />
              <span class="btn__text">Копировать</span>
            </button>
          {/if}
        {/snippet}
      </ModalFoot>
    </form>
  {/if}
</ModalShell>

<ContextMenu
  open={ctxOpen}
  x={ctxX}
  y={ctxY}
  items={ctxItems}
  onSelect={onTemplateMenuSelect}
  onClose={() => (ctxOpen = false)}
/>

<ConfirmModal
  open={deleteConfirmOpen}
  title="Удалить шаблон?"
  message={editing ? `Удалить шаблон «${editing.title}»? Созданные квесты останутся.` : ''}
  busy={deleting}
  onCancel={() => {
    if (!deleting) deleteConfirmOpen = false
  }}
  onConfirm={confirmDelete}
/>

<style>

  .field .pool-command {
    font-size: var(--text-xs, 0.75rem);
    resize: vertical;
  }

  .tz-line {
    gap: 0.35rem;
  }

  .tz-line .tz-input {
    width: 12rem;
    padding-block: 0.25rem;
  }
</style>

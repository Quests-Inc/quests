<script lang="ts">
  import { untrack } from 'svelte'
  import Icon from '../ui/Icon.svelte'
  import QuestlineIcon from '../ui/QuestlineIcon.svelte'
  import MarkdownBody from '../ui/MarkdownBody.svelte'
  import RefText from '../ui/RefText.svelte'
  import AttachmentsBlock from './AttachmentsBlock.svelte'
  import EntityHistory from './EntityHistory.svelte'
  import TemplateStreak from './TemplateStreak.svelte'
  import ContextMenu from '../ui/ContextMenu.svelte'
  import { formatLocal, localTimeZone } from '../js/time.ts'
  import { parseRefs } from '../js/refs.ts'
  import {
    OPEN_STATUSES,
    periodBadge,
    questTimer,
    quantifiedProgress,
    significanceLabel,
    statusColor,
  } from '../js/questFormat.ts'
  import { downloadQuestPdfSimple } from '../js/questPdf.ts'
  import { downloadQuestPdf as downloadQuestPdfRich } from '../js/questExport.ts'
  import { toastDone, toastProgress } from '../js/toasts.svelte.ts'
  import { animateText } from '../js/typewriter.ts'

  /** @type {{
   *   selected: any | null,
   *   quests?: any[],
   *   showAllQuests?: boolean,
   *   nowMs: number,
   *   statusBusy: boolean,
   *   deleting: boolean,
   *   deleteAnimActive?: boolean,
   *   consumeRemoteEdit?: (questId: number | null) => boolean,
   *   onDeleteAnimDone?: () => void,
   *   stepBusyId: number | null,
   *   stepEditId: number | null,
   *   stepEditValue: string,
   *   onToggleCompleted: (quest?: any) => void,
   *   onOpenEdit: (quest?: any) => void,
   *   onRequestDelete: (quest?: any) => void,
   *   kbStepId?: number | null,   // step the keyboard cursor is on (highlight only)
   *   onBumpStep: (step: any, delta: number, questId?: number | null) => void,
   *   onBeginEditStep: (step: any, questId?: number | null) => void,
   *   onStepEditKeydown: (event: KeyboardEvent, step: any) => void,
   *   onStepEditBlur: (step: any) => void,
   *   onStepEditInput: (value: string) => void,
   *   onQuestTitleContextMenu?: (event: MouseEvent, quest: any) => void,
   *   onLineHeadContextMenu?: (event: MouseEvent) => void,
   *   onStepContextMenu?: (event: MouseEvent, step: any) => void,
   *   onSelectQuest?: (id: number) => void,
   *   notes?: any[],
   *   questlines?: any[],
   *   labels?: Record<string, string>,
   *   onRef?: (kind: string, id: number) => void,
   *   onOpenGraph?: (type: string, id: number) => void,
   * }} */
  let {
    selected,
    quests = [],
    questlines = [],
    showAllQuests = false,
    nowMs,
    statusBusy,
    deleting,
    deleteAnimActive = false,
    consumeRemoteEdit = undefined,
    onDeleteAnimDone,
    stepBusyId,
    stepEditId,
    stepEditValue,
    onToggleCompleted,
    onOpenEdit,
    onRequestDelete,
    kbStepId = null,
    onBumpStep,
    onBeginEditStep,
    onStepEditKeydown,
    onStepEditBlur,
    onStepEditInput,
    onQuestTitleContextMenu,
    onLineHeadContextMenu,
    onStepContextMenu,
    onSelectQuest,
    notes = [],
    labels = {},
    onRef,
    onOpenGraph,
  }: {
    selected: any | null,
    quests?: any[],
    showAllQuests?: boolean,
    nowMs: number,
    statusBusy: boolean,
    deleting: boolean,
    deleteAnimActive?: boolean,
    consumeRemoteEdit?: (questId: number | null) => boolean,
    onDeleteAnimDone?: () => void,
    stepBusyId: number | null,
    stepEditId: number | null,
    stepEditValue: string,
    onToggleCompleted: (quest?: any) => void,
    onOpenEdit: (quest?: any) => void,
    onRequestDelete: (quest?: any) => void,
    kbStepId?: number | null,   // step the keyboard cursor is on (highlight only)
    onBumpStep: (step: any, delta: number, questId?: number | null) => void,
    onBeginEditStep: (step: any, questId?: number | null) => void,
    onStepEditKeydown: (event: KeyboardEvent, step: any) => void,
    onStepEditBlur: (step: any) => void,
    onStepEditInput: (value: string) => void,
    onQuestTitleContextMenu?: (event: MouseEvent, quest: any) => void,
    onLineHeadContextMenu?: (event: MouseEvent) => void,
    onStepContextMenu?: (event: MouseEvent, step: any) => void,
    onSelectQuest?: (id: number) => void,
    notes?: any[],
    questlines?: any[],
    labels?: Record<string, string>,
    onRef?: (kind: string, id: number) => void,
    onOpenGraph?: (type: string, id: number) => void,
  } = $props()

  const tzLabel = localTimeZone()
  let pdfBusy = $state(false)
  let pdfMenu = $state({ open: false, x: 0, y: 0, quest: (null as any) })

  async function exportPdf(q: any, mode = 'rich') {
    if (!q || pdfBusy) return
    pdfBusy = true
    const tid = 'pdf-quest'
    toastProgress(tid, 'Генерация PDF…')
    try {
      if (mode === 'simple') {
        await downloadQuestPdfSimple(q)
      } else {
        await downloadQuestPdfRich(q, { labels, tzLabel })
      }
      toastDone(tid, 'PDF сохранён')
    } catch (e) {
      console.error(e)
      toastDone(tid, e?.message || 'Не удалось сохранить PDF', 'error')
    } finally {
      pdfBusy = false
    }
  }

  function openPdfMenu(event: { clientX: any; clientY: any }, q: any) {
    pdfMenu = { open: true, x: event.clientX, y: event.clientY, quest: q }
  }

  function selectPdfMode(id: string) {
    exportPdf(pdfMenu.quest, id)
  }

  let lineQuests = $derived.by(() => {
    if (!selected?.questline_id) return []
    const lid = Number(selected.questline_id)
    const selectedId = selected.id
    const pool = showAllQuests
      ? quests
      : quests.filter((q) => OPEN_STATUSES.has(q.status) || q.id === selectedId)
    return pool
      .filter((q) => q.questline_id != null && Number(q.questline_id) === lid)
      .sort((a, b) => {
        const ta = a.created_at || ''
        const tb = b.created_at || ''
        if (ta !== tb) return ta < tb ? -1 : 1
        return (a.id || 0) - (b.id || 0)
      })
  })

  let inQuestline = $derived(Boolean(selected?.questline_id))

  let lineRow = $derived(
    questlines.find((l) => l.id === selected?.questline_id) ?? null,
  )

  let lineMeta = $derived({
    title: lineRow?.title || selected?.questline_title || 'Квестлайн',
    color: lineRow?.color || selected?.questline_color || '#9a9a9a',
    icon: lineRow?.icon || selected?.questline_icon || 'document',
    iconUrl: lineRow?.icon_url || selected?.questline_icon_url || null,
    description: String(lineRow?.description || '').trim(),
  })

  let lineIndex = $derived(lineQuests.findIndex((q) => q.id === selected?.id))
  let linePrev = $derived(lineIndex > 0 ? lineQuests[lineIndex - 1] : null)
  let lineNext = $derived(
    lineIndex >= 0 && lineIndex < lineQuests.length - 1 ? lineQuests[lineIndex + 1] : null,
  )

  /** Блок «Заметки» свёрнут по умолчанию (рисуется, только если есть заметки). */
  let notesOpen = $state(false)

  let linkedNotes = $derived.by(() => {
    if (!selected) return []
    const blob = [
      selected.title,
      selected.description,
      ...(selected.steps || []).flatMap((s: { title: any; description: any }) => [s.title, s.description]),
    ].join('\n')
    const ids = parseRefs(blob)
      .filter((r) => r.kind === 'note')
      .map((r) => r.id)
    const seen = new Set()
    const out = []
    for (const id of ids) {
      if (seen.has(id)) continue
      seen.add(id)
      const row = notes.find((n) => n.id === id)
      out.push({ id, title: row?.title || `note=${id}` })
    }
    return out
  })

  function selectLineQuest(id: number) {
    if (id == null || id === selected?.id) return
    onSelectQuest?.(id)
  }

  /** Only jump to top when switching quests — not on every silent refresh / step bump. */
  let lastDetailQuestId = (null as number | null)

  /** What's actually rendered — equals selected.title/description except mid-animation. */
  let displayTitle = $state('')
  let displayDescription = $state('')
  let prevAutomatedTitle = (null as string | null)
  let prevAutomatedDescription = (null as string | null)
  /** @type {AbortController | null} */
  let titleAnim: AbortController | null = null
  /** @type {AbortController | null} */
  let descAnim: AbortController | null = null

  /** Same idea as displayTitle/displayDescription, per step.id — step
   * descriptions (comments) get typed in too, not just the quest's own. */
  let stepDisplayDesc = $state(new Map())
  let prevStepAutomatedDesc = new Map()
  let stepDescAnims = new Map()
  /** Which texts are being typed in right now — drives the fading tail. */
  let typingTitle = $state(false)
  let typingDesc = $state(false)
  let typingSteps = $state(new Set())

  /** Erase the old text (if any) then type the new one — only for a change
   * that arrived from outside the web UI (consumeRemoteEdit); never for the
   * user's own edits. */
  async function retype(oldText: string, newText: string, granularity: string, setDisplay: { (t: string): string; (t: string): string; (t: any): Map<any,any> }, getCtrl: { (): AbortController; (): AbortController; (): any; (): AbortController }, setCtrl: { (c: AbortController): AbortController; (c: AbortController): AbortController; (c: any): void; (arg0: AbortController): void }, setTyping: { (v: boolean): boolean; (v: boolean): boolean; (v: any): void; (arg0: boolean): void }) {
    getCtrl()?.abort()
    const ac = new AbortController()
    setCtrl(ac)
    if (oldText) {
      await animateText(oldText, { mode: 'erase', granularity, onUpdate: setDisplay, signal: ac.signal })
    }
    setTyping(true)
    await animateText(newText, { mode: 'reveal', granularity, onUpdate: setDisplay, signal: ac.signal })
    if (getCtrl() === ac) {
      setCtrl(null)
      setTyping(false)
    }
  }

  $effect(() => {
    const q = selected
    const id = q?.id ?? null
    const title = q?.title || ''
    const description = q?.description || ''

    const steps = q?.steps || []

    if (id !== lastDetailQuestId) {
      lastDetailQuestId = id
      titleAnim?.abort()
      descAnim?.abort()
      for (const ac of stepDescAnims.values()) ac.abort()
      stepDescAnims = new Map()
      typingTitle = false
      typingDesc = false
      typingSteps = new Set()
      displayTitle = title
      displayDescription = description
      prevAutomatedTitle = title
      prevAutomatedDescription = description
      const freshDesc = new Map()
      const freshPrev = new Map()
      for (const s of steps) {
        freshDesc.set(s.id, s.description || '')
        freshPrev.set(s.id, s.description || '')
      }
      stepDisplayDesc = freshDesc
      prevStepAutomatedDesc = freshPrev
      if (id != null) {
        queueMicrotask(() => {
          document.querySelector('.detail')?.scrollTo({ top: 0 })
        })
      }
      return
    }
    if (!q) return

    const stepsChanged = steps.some(
      (s: { id: any; description: any }) => prevStepAutomatedDesc.has(s.id) && (s.description || '') !== prevStepAutomatedDesc.get(s.id),
    )
    // Animate by who made the change, not by the quest: only an update that
    // came in from outside the web UI (mcp, cli, scheduler…) is typed in.
    const textChanged =
      (prevAutomatedTitle != null && title !== prevAutomatedTitle) ||
      (prevAutomatedDescription != null && description !== prevAutomatedDescription) ||
      stepsChanged
    const remote = textChanged && !!consumeRemoteEdit?.(id)

    if (remote) {
      if (prevAutomatedTitle != null && title !== prevAutomatedTitle) {
        void retype(prevAutomatedTitle, title, 'char', (t: string) => (displayTitle = t),
          () => titleAnim, (c: AbortController) => (titleAnim = c), (v: boolean) => (typingTitle = v))
      } else if (!titleAnim) {
        displayTitle = title
      }
      if (prevAutomatedDescription != null && description !== prevAutomatedDescription) {
        void retype(prevAutomatedDescription, description, 'word', (t: string) => (displayDescription = t),
          () => descAnim, (c: AbortController) => (descAnim = c), (v: boolean) => (typingDesc = v))
      } else if (!descAnim) {
        displayDescription = description
      }
      for (const s of steps) {
        const sDesc = s.description || ''
        const prev = prevStepAutomatedDesc.has(s.id) ? prevStepAutomatedDesc.get(s.id) : null
        if (prev != null && sDesc !== prev) {
          void retype(prev, sDesc, 'word',
            (t: any) => (stepDisplayDesc = new Map(stepDisplayDesc).set(s.id, t)),
            () => stepDescAnims.get(s.id) ?? null,
            (c: any) => {
              if (c) stepDescAnims.set(s.id, c)
              else stepDescAnims.delete(s.id)
            },
            (v: any) => {
              const next = new Set(typingSteps)
              if (v) next.add(s.id)
              else next.delete(s.id)
              typingSteps = next
            })
        } else if (!stepDescAnims.has(s.id) && stepDisplayDesc.get(s.id) !== sDesc) {
          // Only write when the value actually changes — reassigning this
          // $state Map on every effect run (e.g. a step-done event that
          // doesn't touch the description) re-triggers reactivity each time
          // and, under back-to-back live events, blows past Svelte's
          // effect-depth guard (effect_update_depth_exceeded, quest=231).
          stepDisplayDesc = new Map(stepDisplayDesc).set(s.id, sDesc)
        }
        prevStepAutomatedDesc.set(s.id, sDesc)
      }
    } else if (!titleAnim && !descAnim && stepDescAnims.size === 0) {
      displayTitle = title
      displayDescription = description
      const freshDesc = new Map()
      for (const s of steps) freshDesc.set(s.id, s.description || '')
      stepDisplayDesc = freshDesc
      for (const s of steps) prevStepAutomatedDesc.set(s.id, s.description || '')
    }
    prevAutomatedTitle = title
    prevAutomatedDescription = description
  })


  /** Erase-before-delete: App.svelte flips deleteAnimActive after the
   * confirm dialog is accepted and awaits onDeleteAnimDone before actually
   * calling the delete API — so the title/description visibly erase first
   * instead of the quest just vanishing from the list. */
  $effect(() => {
    if (!deleteAnimActive) return
    // untrack: onUpdate below writes displayTitle/displayDescription — a
    // tracked read here would re-run this same effect on every animation
    // frame (abort+restart from the new, shorter text each time), finishing
    // in a handful of ms instead of erasing at the intended pace.
    const { titleText, descText } = untrack(() => ({
      titleText: displayTitle,
      descText: displayDescription,
    }))
    titleAnim?.abort()
    descAnim?.abort()
    const tAc = new AbortController()
    const dAc = new AbortController()
    titleAnim = tAc
    descAnim = dAc
    Promise.all([
      animateText(titleText, {
        mode: 'erase', granularity: 'char',
        onUpdate: (t) => (displayTitle = t), signal: tAc.signal,
      }),
      animateText(descText, {
        mode: 'erase', granularity: 'word',
        onUpdate: (t) => (displayDescription = t), signal: dAc.signal,
      }),
    ]).then(() => {
      if (titleAnim === tAc) titleAnim = null
      if (descAnim === dAc) descAnim = null
      onDeleteAnimDone?.()
    })
  })
</script>

{#snippet questActions(q)}
  <div class="detail__actions">
    <button
      type="button"
      class="btn btn--icon"
      onclick={() => onToggleCompleted(q)}
      disabled={statusBusy}
      title={q.status === 'completed' ? 'Сделать активным' : 'Выполнено'}
      aria-label={q.status === 'completed' ? 'Сделать активным' : 'Отметить выполненным'}
    >
      {#if statusBusy}
        …
      {:else}
        <Icon name={q.status === 'completed' ? 'undo' : 'checkmark'} />
      {/if}
    </button>
    <button
      type="button"
      class="btn btn--icon"
      onclick={() => onOpenEdit(q)}
      title="Править"
      aria-label="Править"
    >
      <Icon name="edit" />
    </button>
    <button
      type="button"
      class="btn btn--icon"
      onclick={() => onOpenGraph?.('quest', q.id)}
      title="Связи"
      aria-label="Показать связи квеста"
    >
      <Icon name="map" />
    </button>
    <button
      type="button"
      class="btn btn--icon"
      onclick={(e) => openPdfMenu(e, q)}
      disabled={pdfBusy}
      title={pdfBusy ? 'PDF…' : 'В PDF'}
      aria-label={pdfBusy ? 'Выгрузка PDF…' : 'Выгрузить в PDF'}
    >
      {#if pdfBusy}
        …
      {:else}
        <Icon name="file-download" />
      {/if}
    </button>
    <button
      type="button"
      class="btn btn--icon btn--danger"
      onclick={() => onRequestDelete(q)}
      disabled={deleting}
      title={deleting ? 'Удаление…' : 'Удалить'}
      aria-label={deleting ? 'Удаление…' : 'Удалить'}
    >
      {#if deleting}
        …
      {:else}
        <Icon name="delete" />
      {/if}
    </button>
  </div>
{/snippet}

{#snippet questEyebrow(q)}
  {@const frac = quantifiedProgress(q)}
  <p class="detail__colophon">
    <span class="status" style:color={statusColor(q.status)}>{q.status}</span>
    {#if q.automated}
      <span title="Автоквест: создание и старт без полноэкранного тоста">авто</span>
    {/if}
    {#if q.significance && q.significance !== 'common'}
      <span class="detail__sig" data-sig={q.significance}>{significanceLabel(q)}</span>
    {/if}
    {#if q.category_label}
      <span title="Раздел">{q.category_label}</span>
    {/if}
    {#if periodBadge(q)}
      <span title="Период">{periodBadge(q)}</span>
    {/if}
    {#if frac}
      <span class="progress">{frac}</span>
    {/if}
  </p>
  {#if q.tags?.length}
    <p class="detail__tags">
      {#each q.tags as t (t.id)}
        <span class="detail__tag" style:--tag={t.color || '#9a9a9a'}>{t.slug}</span>
      {/each}
    </p>
  {/if}
{/snippet}

{#snippet stepList(q)}
  {#if q.steps?.length}
    <!-- A pipeline is steps chained with wait_previous; auto-checks alone (poll/once/watch) are not one. -->
    {@const pipe = q.steps.some((s) => s.wait_previous)}
    {@const currentId = pipe ? q.steps.find((s) => !s.done)?.id : null}
    <ol class="step-list" class:step-list--pipe={pipe}>
      {#each q.steps as step, i (step.id)}
        {@const waiting =
          Boolean(step.wait_previous) && i > 0 && !q.steps[i - 1].done && !step.done}
        {@const stepDesc = stepDisplayDesc.get(step.id) ?? step.description}
        <li
          class="step"
          class:step--done={step.done}
          class:step--current={pipe && step.id === currentId}
          class:step--kb={step.id === kbStepId}
          class:step--current-run={pipe && step.id === currentId && step.run_status === 'running'}
          oncontextmenu={(e) => onStepContextMenu?.(e, step)}
        >
          <span class="step__mark">{step.done ? '✓' : step.run_status === 'running' ? '▶' : '○'}</span>
          <span class="step__main">
            <span class="step__title"><span class="strike" class:strike--done={step.done}><RefText source={step.title} {labels} {onRef} /></span></span>
            {#if step.check_command}
              <span
                class="step__auto"
                class:step__auto--run={step.run_status === 'running'}
                class:step__auto--wait={waiting}
                title={step.check_command}
              >
                {#if waiting}
                  ждёт
                {:else if step.run_status === 'running'}
                  идёт
                {:else if step.run_status === 'fail'}
                  сбой
                {:else if step.run_mode === 'once'}
                  разово
                {:else if step.run_mode === 'watch'}
                  наблюдение {step.check_interval_seconds || '?'}s
                {:else}
                  auto {step.check_interval_seconds || '?'}s
                {/if}
                {#if step.run_on}
                  · {step.run_on}
                {/if}
              </span>
            {/if}
          </span>
          <div class="step__controls">
            {#if step.progress_current > 0}
              <button
                type="button"
                class="step__btn"
                aria-label="Уменьшить прогресс"
                disabled={stepBusyId != null}
                onclick={() => onBumpStep(step, -1, q.id)}
              >
                <Icon name="subtract" size={14} />
              </button>
            {:else}
              <span class="step__btn-slot" aria-hidden="true"></span>
            {/if}
            {#if stepEditId === step.id}
              <input
                class="step__progress-input"
                type="number"
                min="0"
                max={step.progress_total}
                step="1"
                value={stepEditValue}
                disabled={stepBusyId != null}
                aria-label="Прогресс шага"
                autofocus
                oninput={(e) => onStepEditInput(e.currentTarget.value)}
                onkeydown={(e) => onStepEditKeydown(e, step)}
                onblur={() => onStepEditBlur(step)}
                onfocus={(e) => e.currentTarget.select()}
              />
              <span class="step__progress-total">/{step.progress_total}</span>
            {:else}
              <button
                type="button"
                class="step__progress"
                title="Двойной клик — задать значение"
                disabled={stepBusyId != null}
                ondblclick={() => onBeginEditStep(step, q.id)}
              >
                {step.progress_current}/{step.progress_total}
              </button>
            {/if}
            {#if step.progress_current < step.progress_total}
              <button
                type="button"
                class="step__btn"
                aria-label="Увеличить прогресс"
                disabled={stepBusyId != null}
                onclick={() => onBumpStep(step, 1, q.id)}
              >
                <Icon name="add" size={14} />
              </button>
            {:else}
              <span class="step__btn-slot" aria-hidden="true"></span>
            {/if}
          </div>
          {#if step.description || stepDesc}
            <MarkdownBody class="step__desc" source={stepDesc} {labels} {onRef} typing={typingSteps.has(step.id)} />
          {/if}
        </li>
      {/each}
    </ol>
  {:else}
    <p class="block__body block__body--muted">Шагов нет</p>
  {/if}
{/snippet}

{#snippet questBody(q)}
  {@const timer = questTimer(q, nowMs)}
  {#if q.description || displayDescription}
    <div class="block block--prose">
      <MarkdownBody class="block__body" source={displayDescription} {labels} {onRef} typing={typingDesc} />
    </div>
  {/if}

  <div class="block">
    <h3 class="block__label">Шаги</h3>
    {@render stepList(q)}
  </div>

  <div class="block">
    <AttachmentsBlock ownerType="quest" ownerId={q.id} onOpenOwner={onRef} />
  </div>

  <div class="block">
    <EntityHistory entityType="quest" entityId={q.id} version={q.updated_at} />
  </div>

  {#if linkedNotes.length}
    <div class="block">
      <button
        type="button"
        class="block__label detail__fold"
        onclick={() => (notesOpen = !notesOpen)}
        aria-expanded={notesOpen}
      >
        Заметки ({linkedNotes.length})
        <Icon name={notesOpen ? 'chevron-down' : 'chevron-right'} size={12} />
      </button>
      {#if notesOpen}
        <ul class="detail__notes">
          {#each linkedNotes as n (n.id)}
            <li>
              <button type="button" class="detail__note-link" onclick={() => onRef?.('note', n.id)}>
                {n.title}
                <span>note={n.id}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <dl class="dates">
    <div>
      <dt>Создан ({tzLabel})</dt>
      <dd>{formatLocal(q.created_at)}</dd>
    </div>
    <div>
      <dt>Обновлён ({tzLabel})</dt>
      <dd>{formatLocal(q.updated_at)}</dd>
    </div>
    {#if q.deadline_at}
      <div>
        <dt>Срок ({tzLabel})</dt>
        <dd>{formatLocal(q.deadline_at)}</dd>
      </div>
    {/if}
    {#if q.source}
      <div>
        <dt>Источник</dt>
        <dd>
          {#if q.source.startsWith('template:')}
            <RefText source={`template=${q.source.slice('template:'.length)}`} {labels} {onRef} />
          {:else}
            {q.source}
          {/if}
        </dd>
      </div>
    {/if}
    {#if q.source?.startsWith('template:')}
      <TemplateStreak templateId={Number(q.source.slice('template:'.length))} status={q.status} />
    {/if}
  </dl>

  {#if timer}
    <div class="deadline-timer" data-tone={timer.tone}>
      <span class="deadline-timer__label">До срока</span>
      <span class="deadline-timer__value">{timer.detailLabel}</span>
    </div>
  {/if}
{/snippet}

<section class="detail" aria-live="polite">
  {#if !selected}
    <div class="detail__empty">
      <p class="detail__empty-prompt">Выберите квест слева</p>
    </div>
  {:else if inQuestline}
    <div class="detail__line" style="--line-color: {lineMeta.color}">
      <div class="detail__line-wash" aria-hidden="true"></div>
      <header
        class="detail__line-head"
        class:detail__line-head--custom={Boolean(lineMeta.iconUrl)}
        oncontextmenu={(e) => onLineHeadContextMenu?.(e)}
      >
        <span
          class="detail__line-icon"
          class:detail__line-icon--custom={Boolean(lineMeta.iconUrl)}
          aria-hidden="true"
        >
          <QuestlineIcon
            icon={lineMeta.icon}
            iconUrl={lineMeta.iconUrl}
            size="lg"
          />
        </span>
        <div class="detail__line-text">
          <p class="detail__line-eyebrow">Квестлайн</p>
          <h2 class="detail__line-title">
            <RefText source={lineMeta.title} {labels} {onRef} />
          </h2>
        </div>
        <span class="detail__line-count">{lineQuests.length}</span>
      </header>

      {#if lineMeta.description}
        <div class="detail__line-desc block block--prose">
          <MarkdownBody class="block__body" source={lineMeta.description} {labels} {onRef} />
        </div>
      {/if}

      <div class="detail__line-attach">
        <AttachmentsBlock
          ownerType="questline"
          ownerId={selected.questline_id}
          onOpenOwner={onRef}
          label="Вложения квестлайна"
        />
      </div>

      {#if lineQuests.length > 1}
        <nav class="detail__toc" aria-label="Оглавление квестлайна">
          <ol class="detail__toc-list">
            {#each lineQuests as q, i (q.id)}
              <li>
                <button
                  type="button"
                  class="detail__toc-item"
                  class:detail__toc-item--on={q.id === selected.id}
                  class:detail__toc-item--inactive={!OPEN_STATUSES.has(q.status)}
                  aria-current={q.id === selected.id ? 'page' : undefined}
                  onclick={() => selectLineQuest(q.id)}
                  oncontextmenu={(e) => onQuestTitleContextMenu?.(e, q)}
                >
                  <span class="detail__toc-num">{i + 1}</span>
                  <RefText class="detail__toc-title" source={q.title} {labels} {onRef} />
                  {#if q.status !== 'active'}
                    <span class="status" style:color={statusColor(q.status)}>{q.status}</span>
                  {/if}
                </button>
              </li>
            {/each}
          </ol>
        </nav>
      {/if}

      <article id="quest-{selected.id}" class="detail__quest">
        <header class="detail__head" data-sig={selected.significance || 'common'}>
          <div class="detail__head-row">
            <h2
              class="detail__title"
              oncontextmenu={(e) => onQuestTitleContextMenu?.(e, selected)}
            >
              {#key selected.id}<span class="strike" class:strike--done={selected.status === 'completed'}><RefText source={displayTitle} {labels} {onRef} typing={typingTitle} /></span>{/key}
            </h2>
            {@render questActions(selected)}
          </div>
          {@render questEyebrow(selected)}
        </header>
        {@render questBody(selected)}
      </article>

      {#if lineQuests.length > 1}
        <footer class="detail__line-foot">
          <nav class="detail__pager" aria-label="Соседние квесты">
            {#if linePrev}
              <button
                type="button"
                class="detail__pager-link"
                onclick={() => selectLineQuest(linePrev.id)}
              >
                ← {linePrev.title}
              </button>
            {:else}
              <span class="detail__pager-link detail__pager-link--empty"></span>
            {/if}
            <span class="detail__pager-pos">{lineIndex + 1} / {lineQuests.length}</span>
            {#if lineNext}
              <button
                type="button"
                class="detail__pager-link detail__pager-link--next"
                onclick={() => selectLineQuest(lineNext.id)}
              >
                {lineNext.title} →
              </button>
            {:else}
              <span class="detail__pager-link detail__pager-link--empty"></span>
            {/if}
          </nav>
        </footer>
      {/if}
    </div>
  {:else}
    <article id="quest-{selected.id}" class="detail__quest">
      <header class="detail__head" data-sig={selected.significance || 'common'}>
        <div class="detail__head-row">
          <h2
            class="detail__title"
            oncontextmenu={(e) => onQuestTitleContextMenu?.(e, selected)}
          >
            {#key selected.id}<span class="strike" class:strike--done={selected.status === 'completed'}><RefText source={displayTitle} {labels} {onRef} typing={typingTitle} /></span>{/key}
          </h2>
          {@render questActions(selected)}
        </div>
        {@render questEyebrow(selected)}
      </header>
      {@render questBody(selected)}
    </article>
  {/if}
</section>

<ContextMenu
  open={pdfMenu.open}
  x={pdfMenu.x}
  y={pdfMenu.y}
  items={[
    { id: 'rich', label: 'PDF' },
    { id: 'simple', label: 'PDF (просто)' },
  ]}
  onSelect={selectPdfMode}
  onClose={() => (pdfMenu = { ...pdfMenu, open: false })}
/>
